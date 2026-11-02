package service

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupProxyPoolTestDB(t *testing.T) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousMaster, previousCache, previousRedis, previousSQLite := common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled, common.SQLitePath
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousType, previousLogType)
		common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled, common.SQLitePath = previousMaster, previousCache, previousRedis, previousSQLite
	})
	t.Setenv("SQL_DSN", "")
	common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled = false, false, false
	common.SQLitePath = filepath.Join(t.TempDir(), "proxy-pool.db")
	require.NoError(t, model.InitDB())
	require.NoError(t, model.DB.AutoMigrate(&model.ProxyPool{}))
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = model.DB
	common.SetLogDatabaseType(common.MainDatabaseType())
}

func createTestProxyPool(t *testing.T, name string, proxies []string) *model.ProxyPool {
	t.Helper()
	pool := &model.ProxyPool{
		Name:        name,
		Proxies:     joinProxyLines(proxies),
		CreatedTime: common.GetTimestamp(),
	}
	require.NoError(t, model.CreateProxyPool(pool))
	model.RefreshProxyPoolCache()
	return pool
}

func joinProxyLines(proxies []string) string {
	lines := ""
	for i, proxy := range proxies {
		if i > 0 {
			lines += "\n"
		}
		lines += proxy
	}
	return lines
}

func TestResolvePoolProxyBindingOrderedCyclesAndTransfers(t *testing.T) {
	setupProxyPoolTestDB(t)
	members := []string{
		"socks5://10.0.0.1:1080",
		"socks5://10.0.0.2:1080",
		"socks5://10.0.0.3:1080",
	}
	pool := createTestProxyPool(t, "ordered-pool", members)
	binding := "pool:" + strconv.Itoa(pool.Id) + ":ordered"

	// A full cycle covers every member exactly once.
	firstRound := make([]string, 0, len(members))
	for range members {
		resolved, ok := ResolvePoolProxyBinding(binding)
		require.True(t, ok)
		firstRound = append(firstRound, resolved)
	}
	assert.ElementsMatch(t, members, firstRound)

	// Cooling the next scheduled member transfers traffic to another one.
	cooled := firstRound[0]
	MarkProxyTransportResult(cooled, false)
	for range 2 {
		resolved, ok := ResolvePoolProxyBinding(binding)
		require.True(t, ok)
		assert.NotEqual(t, cooled, resolved)
	}

	// Success clears the cool-down and the member rejoins the rotation.
	MarkProxyTransportResult(cooled, true)
	resolved, ok := ResolvePoolProxyBinding(binding)
	require.True(t, ok)
	assert.Contains(t, members, resolved)
}

func TestResolvePoolProxyBindingFixedTransfersAndRecovers(t *testing.T) {
	setupProxyPoolTestDB(t)
	members := []string{
		"http://10.1.0.1:8080",
		"http://10.1.0.2:8080",
		"http://10.1.0.3:8080",
	}
	pool := createTestProxyPool(t, "fixed-pool", members)
	binding := "pool:" + strconv.Itoa(pool.Id) + ":fixed:0"

	resolved, ok := ResolvePoolProxyBinding(binding)
	require.True(t, ok)
	assert.Equal(t, members[0], resolved)

	// The pinned member cooling transfers traffic to a sibling...
	MarkProxyTransportResult(members[0], false)
	resolved, ok = ResolvePoolProxyBinding(binding)
	require.True(t, ok)
	assert.NotEqual(t, members[0], resolved)

	// ...and its recovery returns the traffic.
	MarkProxyTransportResult(members[0], true)
	resolved, ok = ResolvePoolProxyBinding(binding)
	require.True(t, ok)
	assert.Equal(t, members[0], resolved)
}

func TestResolvePoolProxyBindingRandomAndFallbacks(t *testing.T) {
	setupProxyPoolTestDB(t)
	members := []string{
		"socks5://10.2.0.1:1080",
		"socks5://10.2.0.2:1080",
	}
	pool := createTestProxyPool(t, "random-pool", members)
	randomBinding := "pool:" + strconv.Itoa(pool.Id) + ":random"
	for range 8 {
		resolved, ok := ResolvePoolProxyBinding(randomBinding)
		require.True(t, ok)
		assert.Contains(t, members, resolved)
	}

	// Every member cooled: resolution still returns the earliest-expiring
	// one instead of failing outright.
	for _, member := range members {
		MarkProxyTransportResult(member, false)
	}
	resolved, ok := ResolvePoolProxyBinding(randomBinding)
	require.True(t, ok)
	assert.Contains(t, members, resolved)

	// Missing pool and non-pool bindings are reported as unhandled.
	_, ok = ResolvePoolProxyBinding("pool:999:random")
	assert.False(t, ok)
	_, ok = ResolvePoolProxyBinding("socks5://10.2.0.9:1080")
	assert.False(t, ok)
}
