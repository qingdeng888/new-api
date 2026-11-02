package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMultiKeyManageTestDB boots an isolated SQLite database for one test
// and returns the root user, restoring the previous globals on cleanup.
func setupMultiKeyManageTestDB(t *testing.T) *model.User {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousMaster, previousCache, previousRedis, previousSQLite := common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled, common.SQLitePath
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousType, previousLogType)
		common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled, common.SQLitePath = previousMaster, previousCache, previousRedis, previousSQLite
	})
	t.Setenv("SQL_DSN", os.Getenv("TEST_CHANNEL_SQL_DSN"))
	t.Setenv("LOG_SQL_DSN", "")
	common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled = false, false, false
	common.SQLitePath = filepath.Join(t.TempDir(), "channel.db")
	require.NoError(t, model.InitDB())
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = database
	common.SetLogDatabaseType(common.MainDatabaseType())
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}, &model.Log{}, &model.AuditLog{}))
	root := &model.User{Username: "multi-key-review-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, database.Create(root).Error)
	t.Cleanup(func() { require.NoError(t, database.Unscoped().Delete(root).Error) })
	versionQuery := "SELECT VERSION()"
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		versionQuery = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, database.Raw(versionQuery).Scan(&version).Error)
	t.Logf("database=%s version=%s", common.MainDatabaseType(), version)
	return root
}

func TestMultiKeyEnableRestoresOnlyExhaustedChannels(t *testing.T) {
	root := setupMultiKeyManageTestDB(t)

	for _, cacheEnabled := range []bool{false, true} {
		for _, action := range []string{"enable_key", "enable_all_keys"} {
			for _, tc := range []struct {
				name           string
				initialStatus  int
				manualOverride string
				wantStatus     int
			}{
				{name: "key exhaustion restores", initialStatus: common.ChannelStatusEnabled, wantStatus: common.ChannelStatusEnabled},
				{name: "manual disable is preserved", initialStatus: common.ChannelStatusManuallyDisabled, wantStatus: common.ChannelStatusManuallyDisabled},
				{name: "manual disable after exhaustion is preserved", initialStatus: common.ChannelStatusEnabled, manualOverride: "status", wantStatus: common.ChannelStatusManuallyDisabled},
				{name: "tag disable after exhaustion is preserved", initialStatus: common.ChannelStatusEnabled, manualOverride: "tag", wantStatus: common.ChannelStatusManuallyDisabled},
			} {
				t.Run(fmt.Sprintf("cache=%t/%s/%s", cacheEnabled, action, tc.name), func(t *testing.T) {
					common.MemoryCacheEnabled = cacheEnabled
					tag := t.Name()
					channel := &model.Channel{Name: t.Name(), Type: 1, Key: "key-one\nkey-two", Status: tc.initialStatus, Models: "test-model", Group: "default", Tag: &tag,
						ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyStatusList: map[int]int{1: common.ChannelStatusManuallyDisabled}},
					}
					require.NoError(t, channel.Insert())
					t.Cleanup(func() {
						require.NoError(t, channel.Delete())
						model.InitChannelCache()
					})
					for _, operation := range []string{"disable_key", action} {
						if operation == action {
							if tc.manualOverride == "status" {
								model.UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual operation")
							} else if tc.manualOverride == "tag" {
								require.NoError(t, model.DisableChannelByTag(tag))
							}
						}
						payload, err := common.Marshal(MultiKeyManageRequest{ChannelId: channel.Id, Action: operation, KeyIndex: common.GetPointer(0)})
						require.NoError(t, err)
						recorder := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(recorder)
						c.Set("id", root.Id)
						c.Set("role", common.RoleRootUser)
						c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key", bytes.NewReader(payload))
						c.Request.Header.Set("Content-Type", "application/json")
						ManageMultiKeys(c)
						var result struct {
							Success bool `json:"success"`
						}
						require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
						require.True(t, result.Success, recorder.Body.String())
					}
					loaded, err := model.GetChannelById(channel.Id, true)
					require.NoError(t, err)
					assert.Equal(t, tc.wantStatus, loaded.Status)
					assert.NotContains(t, loaded.ChannelInfo.MultiKeyStatusList, 0)
					assert.NotContains(t, loaded.ChannelInfo.MultiKeyDisabledReason, 0)
					assert.NotContains(t, loaded.ChannelInfo.MultiKeyDisabledTime, 0)
					var ability model.Ability
					require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
					assert.Equal(t, tc.wantStatus == common.ChannelStatusEnabled, ability.Enabled)
				})
			}
		}
	}
}

func TestOldestAutoDisabledKeyIndexRotatesThroughDeadKeys(t *testing.T) {
	channel := &model.Channel{
		Key: "k0\nk1\nk2\nk3",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 4,
			MultiKeyStatusList: map[int]int{
				1: common.ChannelStatusAutoDisabled,
				3: common.ChannelStatusAutoDisabled,
			},
			MultiKeyDisabledTime: map[int]int64{1: 100, 3: 50},
		},
	}
	// The key that failed longest ago is probed first.
	assert.Equal(t, 3, oldestAutoDisabledKeyIndex(channel))

	// Keys without a recorded time count as the oldest.
	channel.ChannelInfo.MultiKeyDisabledTime = map[int]int64{1: 100}
	assert.Equal(t, 3, oldestAutoDisabledKeyIndex(channel))

	// No auto-disabled keys at all means nothing to probe.
	channel.ChannelInfo.MultiKeyStatusList = nil
	assert.Equal(t, -1, oldestAutoDisabledKeyIndex(channel))
}

func TestMultiKeyGetKeyStatusReturnsFullKeys(t *testing.T) {
	root := setupMultiKeyManageTestDB(t)

	channel := &model.Channel{
		Name:   t.Name(),
		Type:   1,
		Key:    "sk-alpha-1234567890abcdef\nsk-beta-0987654321fedcba",
		Status: common.ChannelStatusEnabled,
		Models: "test-model",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, channel.Insert())
	t.Cleanup(func() {
		require.NoError(t, channel.Delete())
		model.InitChannelCache()
	})

	payload, err := common.Marshal(MultiKeyManageRequest{ChannelId: channel.Id, Action: "get_key_status"})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", root.Id)
	c.Set("role", common.RoleRootUser)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	ManageMultiKeys(c)

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Keys []struct {
				Index int    `json:"index"`
				Key   string `json:"key"`
			} `json:"keys"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
	require.True(t, result.Success, recorder.Body.String())
	require.Len(t, result.Data.Keys, 2)
	assert.Equal(t, "sk-alpha-1234567890abcdef", result.Data.Keys[0].Key)
	assert.Equal(t, "sk-beta-0987654321fedcba", result.Data.Keys[1].Key)
}

func TestMultiKeySetKeyProxyRoundTrip(t *testing.T) {
	root := setupMultiKeyManageTestDB(t)

	channel := &model.Channel{
		Name:   t.Name(),
		Type:   1,
		Key:    "sk-a\nsk-b\nsk-c",
		Status: common.ChannelStatusEnabled,
		Models: "test-model",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 3,
		},
	}
	require.NoError(t, channel.Insert())
	t.Cleanup(func() {
		require.NoError(t, channel.Delete())
		model.InitChannelCache()
	})

	callManage := func(request MultiKeyManageRequest) map[string]any {
		payload, err := common.Marshal(request)
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("id", root.Id)
		c.Set("role", common.RoleRootUser)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key", bytes.NewReader(payload))
		c.Request.Header.Set("Content-Type", "application/json")
		ManageMultiKeys(c)
		var result map[string]any
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
		return result
	}

	// A valid proxy is stored and reported back.
	result := callManage(MultiKeyManageRequest{
		ChannelId: channel.Id, Action: "set_key_proxy",
		KeyIndex: common.GetPointer(1), Proxy: "socks5://user:pass@10.0.0.8:1080",
	})
	assert.Equal(t, true, result["success"])

	// An unsupported scheme is rejected before touching the channel.
	result = callManage(MultiKeyManageRequest{
		ChannelId: channel.Id, Action: "set_key_proxy",
		KeyIndex: common.GetPointer(0), Proxy: "ftp://nope:21",
	})
	assert.Equal(t, false, result["success"])

	// get_key_status reports the configured proxy and omits the unset ones.
	result = callManage(MultiKeyManageRequest{ChannelId: channel.Id, Action: "get_key_status"})
	require.Equal(t, true, result["success"])
	data := result["data"].(map[string]any)
	keys := data["keys"].([]any)
	first := keys[0].(map[string]any)
	second := keys[1].(map[string]any)
	assert.NotContains(t, first, "proxy")
	assert.Equal(t, "socks5://user:pass@10.0.0.8:1080", second["proxy"])

	// An empty value clears the per-key proxy.
	result = callManage(MultiKeyManageRequest{
		ChannelId: channel.Id, Action: "set_key_proxy",
		KeyIndex: common.GetPointer(1), Proxy: "",
	})
	assert.Equal(t, true, result["success"])
	loaded, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.NotContains(t, loaded.ChannelInfo.MultiKeyProxies, 1)
}

func TestSingleKeyChannelManagesKeyProxy(t *testing.T) {
	root := setupMultiKeyManageTestDB(t)
	previous := operation_setting.GetProxyPoolSetting().Proxies
	t.Cleanup(func() { operation_setting.GetProxyPoolSetting().Proxies = previous })
	operation_setting.GetProxyPoolSetting().Proxies =
		`[{"url":"socks5://10.7.0.1:1080","note":"pool-a","enabled":true}]`

	channel := &model.Channel{
		Name:   t.Name(),
		Type:   1,
		Key:    "only-key",
		Status: common.ChannelStatusEnabled,
		Models: "test-model",
		Group:  "default",
	}
	require.NoError(t, channel.Insert())
	t.Cleanup(func() {
		require.NoError(t, channel.Delete())
		model.InitChannelCache()
	})

	callManage := func(request MultiKeyManageRequest) map[string]any {
		payload, err := common.Marshal(request)
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("id", root.Id)
		c.Set("role", common.RoleRootUser)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key", bytes.NewReader(payload))
		c.Request.Header.Set("Content-Type", "application/json")
		ManageMultiKeys(c)
		var result map[string]any
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
		return result
	}

	// A single-key channel is accepted and reports its one key.
	result := callManage(MultiKeyManageRequest{ChannelId: channel.Id, Action: "get_key_status"})
	require.Equal(t, true, result["success"], result["message"])
	keys := result["data"].(map[string]any)["keys"].([]any)
	require.Len(t, keys, 1)

	// Pinning a pool entry by URL is stored for index 0.
	pinned := operation_setting.ProxyPoolBindingPinnedPrefix + "socks5://10.7.0.1:1080"
	result = callManage(MultiKeyManageRequest{
		ChannelId: channel.Id, Action: "set_key_proxy",
		KeyIndex: common.GetPointer(0), Proxy: pinned,
	})
	assert.Equal(t, true, result["success"], result["message"])

	loaded, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, pinned, loaded.ChannelInfo.MultiKeyProxies[0])

	// The panel reports the binding back.
	result = callManage(MultiKeyManageRequest{ChannelId: channel.Id, Action: "get_key_status"})
	keys = result["data"].(map[string]any)["keys"].([]any)
	assert.Equal(t, pinned, keys[0].(map[string]any)["proxy"])

	// A binding to an entry missing from the pool is rejected.
	result = callManage(MultiKeyManageRequest{
		ChannelId: channel.Id, Action: "set_key_proxy",
		KeyIndex: common.GetPointer(0),
		Proxy:    operation_setting.ProxyPoolBindingPinnedPrefix + "socks5://10.7.9.9:1080",
	})
	assert.Equal(t, false, result["success"])
}
