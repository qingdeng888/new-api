package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func newMultiKeyTestChannel() *model.Channel {
	channel := &model.Channel{
		Id:   7,
		Type: constant.ChannelTypeOpenAI,
		Key:  "k0\nk1\nk2",
	}
	channel.ChannelInfo = model.ChannelInfo{
		IsMultiKey:   true,
		MultiKeySize: 3,
		MultiKeyMode: constant.MultiKeyModeRandom,
	}
	return channel
}

func newMultiKeyTestContext() *gin.Context {
	c, _ := gin.CreateTestContext(nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

func TestSetupContextForSelectedChannelPinsRequestedMultiKey(t *testing.T) {
	channel := newMultiKeyTestChannel()
	c := newMultiKeyTestContext()
	common.SetContextKey(c, constant.ContextKeyChannelPinnedMultiKeyIndex, 1)

	require.Nil(t, SetupContextForSelectedChannel(c, channel, "gpt-4o"))
	assert.Equal(t, "k1", common.GetContextKeyString(c, constant.ContextKeyChannelKey))
	assert.Equal(t, 1, common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex))
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey))
}

func TestSetupContextForSelectedChannelPinnedIndexIgnoresDisabledStatus(t *testing.T) {
	channel := newMultiKeyTestChannel()
	// A disabled key stays selectable when pinned, so a manual test can
	// probe whether it recovered.
	channel.ChannelInfo.MultiKeyStatusList = map[int]int{1: 3}
	c := newMultiKeyTestContext()
	common.SetContextKey(c, constant.ContextKeyChannelPinnedMultiKeyIndex, 1)

	require.Nil(t, SetupContextForSelectedChannel(c, channel, "gpt-4o"))
	assert.Equal(t, "k1", common.GetContextKeyString(c, constant.ContextKeyChannelKey))
}

func TestSetupContextForSelectedChannelRejectsOutOfRangePinnedIndex(t *testing.T) {
	channel := newMultiKeyTestChannel()
	c := newMultiKeyTestContext()
	common.SetContextKey(c, constant.ContextKeyChannelPinnedMultiKeyIndex, 3)

	err := SetupContextForSelectedChannel(c, channel, "gpt-4o")
	require.NotNil(t, err)
	assert.Equal(t, types.ErrorCodeChannelNoAvailableKey, err.GetErrorCode())
}

func TestSetupContextForSelectedChannelSkipsDisabledKeysWithoutPin(t *testing.T) {
	channel := newMultiKeyTestChannel()
	channel.ChannelInfo.MultiKeyStatusList = map[int]int{0: 2}
	c := newMultiKeyTestContext()

	require.Nil(t, SetupContextForSelectedChannel(c, channel, "gpt-4o"))
	assert.Contains(
		t,
		[]string{"k1", "k2"},
		common.GetContextKeyString(c, constant.ContextKeyChannelKey),
	)
}
