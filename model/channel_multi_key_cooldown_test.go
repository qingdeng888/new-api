package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiKeyCooldownSecondsBackoff(t *testing.T) {
	assert.EqualValues(t, 60, MultiKeyCooldownSeconds(1))
	assert.EqualValues(t, 120, MultiKeyCooldownSeconds(2))
	assert.EqualValues(t, 240, MultiKeyCooldownSeconds(3))
	assert.EqualValues(t, MultiKeyCooldownMaxSeconds, MultiKeyCooldownSeconds(6))
	assert.EqualValues(t, MultiKeyCooldownMaxSeconds, MultiKeyCooldownSeconds(50))
	assert.EqualValues(t, MultiKeyCooldownBaseSeconds, MultiKeyCooldownSeconds(0))
}

func newCooldownTestChannel(t *testing.T) Channel {
	t.Helper()
	channel := Channel{
		Name:   "cooldown-channel",
		Key:    "key-a\nkey-b\nkey-c",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 3,
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	require.NoError(t, DB.Create(&channel).Error)
	return channel
}

func TestRecordMultiKeyKeyFailureAndSuccessPersistsCooldown(t *testing.T) {
	setupChannelStatusTest(t)
	channel := newCooldownTestChannel(t)

	require.True(t, RecordMultiKeyKeyFailure(channel.Id, "key-a"))
	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.EqualValues(t, 1, stored.ChannelInfo.MultiKeyFailStreak[0])
	assert.GreaterOrEqual(t, stored.ChannelInfo.MultiKeyCooldownUntil[0], common.GetTimestamp())
	assert.LessOrEqual(t, stored.ChannelInfo.MultiKeyCooldownUntil[0]-common.GetTimestamp(), MultiKeyCooldownSeconds(1))

	// A second consecutive failure doubles the cool-down.
	require.True(t, RecordMultiKeyKeyFailure(channel.Id, "key-a"))
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.EqualValues(t, 2, stored.ChannelInfo.MultiKeyFailStreak[0])

	// Success clears both the streak and the cool-down.
	require.True(t, RecordMultiKeyKeySuccess(channel.Id, "key-a"))
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.NotContains(t, stored.ChannelInfo.MultiKeyFailStreak, 0)
	assert.NotContains(t, stored.ChannelInfo.MultiKeyCooldownUntil, 0)

	// Unknown keys are ignored without error.
	assert.False(t, RecordMultiKeyKeySuccess(channel.Id, "missing-key"))
}

func TestGetNextEnabledKeySkipsCoolingKeys(t *testing.T) {
	channel := Channel{
		Key: "key-a\nkey-b\nkey-c",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 3,
			MultiKeyMode: constant.MultiKeyModeRandom,
			// key-b is the only status-enabled key but cooling down.
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				2: common.ChannelStatusAutoDisabled,
			},
			MultiKeyCooldownUntil: map[int]int64{1: common.GetTimestamp() + 600},
		},
	}
	_, _, err := channel.GetNextEnabledKey()
	require.NotNil(t, err)

	// Once the deadline passes the key returns to rotation on its own.
	channel.ChannelInfo.MultiKeyCooldownUntil = map[int]int64{1: common.GetTimestamp() - 1}
	key, idx, err := channel.GetNextEnabledKey()
	require.Nil(t, err)
	assert.Equal(t, "key-b", key)
	assert.Equal(t, 1, idx)
}

func TestUpdateChannelStatusReEnablesSingleKeyOnEnabledChannel(t *testing.T) {
	setupChannelStatusTest(t)
	channel := newCooldownTestChannel(t)

	require.True(t, UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "boom"))
	require.True(t, UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusEnabled, ""))

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.NotContains(t, stored.ChannelInfo.MultiKeyStatusList, 0)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
}

func TestUpdateChannelStatusDisableSupersedesCooldown(t *testing.T) {
	setupChannelStatusTest(t)
	channel := newCooldownTestChannel(t)
	require.True(t, RecordMultiKeyKeyFailure(channel.Id, "key-a"))

	require.True(t, UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "quota exhausted"))

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.NotContains(t, stored.ChannelInfo.MultiKeyCooldownUntil, 0)
	assert.NotContains(t, stored.ChannelInfo.MultiKeyFailStreak, 0)
}

func TestValidateProxyBinding(t *testing.T) {
	setupChannelStatusTest(t)
	previous := operation_setting.GetProxyPoolSetting().Proxies
	t.Cleanup(func() { operation_setting.GetProxyPoolSetting().Proxies = previous })
	operation_setting.GetProxyPoolSetting().Proxies = "socks5://10.9.0.1:1080\nhttp://10.9.0.2:8080"

	// Empty inherits, direct dials without a proxy, plain URLs must parse.
	require.NoError(t, ValidateProxyBinding(""))
	require.NoError(t, ValidateProxyBinding(MultiKeyProxyDirect))
	require.NoError(t, ValidateProxyBinding("socks5://user:pass@10.0.0.1:1080"))
	require.Error(t, ValidateProxyBinding("ftp://nope:21"))

	// The global pool is addressed as pool (random) or pool:<n> (pinned).
	require.NoError(t, ValidateProxyBinding("pool"))
	require.NoError(t, ValidateProxyBinding("pool:1"))
	require.Error(t, ValidateProxyBinding("pool:2"))
	require.Error(t, ValidateProxyBinding("pool:notanumber"))
	require.Error(t, ValidateProxyBinding("pool:-1"))

	// An empty pool cannot be referenced.
	operation_setting.GetProxyPoolSetting().Proxies = ""
	require.Error(t, ValidateProxyBinding("pool"))
}
