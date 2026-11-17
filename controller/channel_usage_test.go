/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type channelUsageResponse struct {
	Success bool                 `json:"success"`
	Message string               `json:"message"`
	Data    ChannelUsageResponse `json:"data"`
}

func setupChannelUsageControllerTest(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelUsageData{}))
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: 5, Name: "multi-key", Type: 1, Key: "sk-first\nsk-second",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: 6, Name: "single-key", Type: 1, Key: "sk-only",
	}).Error)
}

// insertChannelUsage stores one aggregate row, bypassing the buffer so the
// reader is tested against stored hours.
func insertChannelUsage(t *testing.T, row model.ChannelUsageData) {
	t.Helper()
	require.NoError(t, model.DB.Create(&row).Error)
}

func callChannelUsage(t *testing.T, query string) channelUsageResponse {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/data/channels?"+query, nil)
	GetChannelUsage(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload channelUsageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, payload.Message)
	return payload
}

func TestGetChannelUsageNamesKeysAndSumsTheirModels(t *testing.T) {
	setupChannelUsageControllerTest(t)
	now := common.GetTimestamp()
	hour := now - now%3600
	firstFingerprint := model.ChannelKeyFingerprint("sk-first")
	secondFingerprint := model.ChannelKeyFingerprint("sk-second")

	insertChannelUsage(t, model.ChannelUsageData{
		ChannelId: 5, KeyFingerprint: firstFingerprint, ModelName: "gpt-4o", NodeName: "n1", CreatedAt: hour,
		RequestCount: 4, SuccessCount: 3, FailureCount: 1,
		PromptTokens: 100, CompletionTokens: 40, CacheReadTokens: 25, Quota: 900,
	})
	insertChannelUsage(t, model.ChannelUsageData{
		ChannelId: 5, KeyFingerprint: firstFingerprint, ModelName: "claude-3", NodeName: "n1", CreatedAt: hour,
		RequestCount: 2, SuccessCount: 2, PromptTokens: 50, CompletionTokens: 20, Quota: 100,
	})
	insertChannelUsage(t, model.ChannelUsageData{
		ChannelId: 5, KeyFingerprint: secondFingerprint, ModelName: "gpt-4o", NodeName: "n1", CreatedAt: hour,
		RequestCount: 1, SuccessCount: 1, Quota: 10,
	})

	payload := callChannelUsage(t, fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", hour, now))
	require.Len(t, payload.Data.Channels, 1)
	channel := payload.Data.Channels[0]
	assert.Equal(t, 5, channel.ChannelId)
	assert.Equal(t, "multi-key", channel.ChannelName)
	assert.Equal(t, 7, channel.RequestCount, "the channel total sums its keys")
	assert.Equal(t, 6, channel.SuccessCount)
	assert.Equal(t, 1, channel.FailureCount)
	assert.EqualValues(t, 150, channel.PromptTokens)
	assert.EqualValues(t, 60, channel.CompletionTokens)
	assert.EqualValues(t, 25, channel.CacheReadTokens)
	assert.Equal(t, 1010, channel.Quota)

	require.Len(t, channel.Keys, 2, "one row per current key")
	first := channel.Keys[0]
	assert.Equal(t, 0, first.KeyIndex)
	assert.Equal(t, model.MaskTokenKey("sk-first"), first.KeyLabel)
	assert.False(t, first.Removed)
	assert.Equal(t, 6, first.RequestCount)
	require.Len(t, first.Models, 2, "each key keeps its models")
	assert.Equal(t, "gpt-4o", first.Models[0].ModelName, "models are ordered by requests")
	assert.Equal(t, 4, first.Models[0].RequestCount)
	assert.Equal(t, "claude-3", first.Models[1].ModelName)

	second := channel.Keys[1]
	assert.Equal(t, 1, second.KeyIndex)
	assert.Equal(t, 1, second.RequestCount)
}

func TestGetChannelUsageKeepsUsageOfDeletedKeysAndChannels(t *testing.T) {
	setupChannelUsageControllerTest(t)
	now := common.GetTimestamp()
	hour := now - now%3600

	// A current key, a key removed from the channel after it served traffic,
	// and a channel deleted after it served traffic.
	insertChannelUsage(t, model.ChannelUsageData{
		ChannelId: 5, KeyFingerprint: model.ChannelKeyFingerprint("sk-first"), ModelName: "gpt-4o",
		CreatedAt: hour, NodeName: "n1", RequestCount: 4, SuccessCount: 4,
	})
	insertChannelUsage(t, model.ChannelUsageData{
		ChannelId: 5, KeyFingerprint: model.ChannelKeyFingerprint("sk-removed"), ModelName: "gpt-4o",
		CreatedAt: hour, NodeName: "n1", RequestCount: 3, SuccessCount: 3,
	})
	// A channel deleted after it served traffic.
	insertChannelUsage(t, model.ChannelUsageData{
		ChannelId: 99, KeyFingerprint: "", ModelName: "gpt-4o", CreatedAt: hour, NodeName: "n1",
		RequestCount: 5, SuccessCount: 5,
	})

	payload := callChannelUsage(t, fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", hour, now))
	require.Len(t, payload.Data.Channels, 2)

	byId := map[int]ChannelUsageChannel{}
	for _, channel := range payload.Data.Channels {
		byId[channel.ChannelId] = channel
	}

	live := byId[5]
	require.Len(t, live.Keys, 2)
	assert.False(t, live.Keys[0].Removed, "a current key is listed first")
	assert.Equal(t, 0, live.Keys[0].KeyIndex)
	assert.True(t, live.Keys[1].Removed, "usage of a deleted key stays visible")
	assert.Equal(t, -1, live.Keys[1].KeyIndex)

	gone := byId[99]
	assert.True(t, gone.Removed, "usage of a deleted channel stays visible")
	assert.Equal(t, 5, gone.RequestCount)
}

func TestGetChannelUsageMarksAttemptsWithoutAKey(t *testing.T) {
	setupChannelUsageControllerTest(t)
	now := common.GetTimestamp()
	hour := now - now%3600
	insertChannelUsage(t, model.ChannelUsageData{
		ChannelId: 5, KeyFingerprint: "", ModelName: "mj-task", CreatedAt: hour, NodeName: "n1",
		RequestCount: 2, SuccessCount: 2,
	})

	payload := callChannelUsage(t, fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", hour, now))
	require.Len(t, payload.Data.Channels, 1)
	require.Len(t, payload.Data.Channels[0].Keys, 1)
	assert.Equal(t, -1, payload.Data.Channels[0].Keys[0].KeyIndex)
	assert.False(t, payload.Data.Channels[0].Keys[0].Removed, "task settlement is not a deleted key")
}

func TestGetChannelUsageClampsTheWindowToTheRetention(t *testing.T) {
	setupChannelUsageControllerTest(t)
	now := common.GetTimestamp()

	payload := callChannelUsage(t, fmt.Sprintf("start_timestamp=%d&end_timestamp=%d",
		now-90*24*3600, now+3600))

	assert.Equal(t, now, payload.Data.EndTimestamp, "the window never reaches the future")
	assert.Equal(t, now-model.ChannelUsageRetentionSeconds, payload.Data.StartTimestamp,
		"the window never reaches past the retention")
	assert.Empty(t, payload.Data.Channels)
}

// A caller that asks for a window starting in the future must not be answered
// with a future range.
func TestGetChannelUsageClampsAFutureStart(t *testing.T) {
	setupChannelUsageControllerTest(t)
	now := common.GetTimestamp()

	payload := callChannelUsage(t, fmt.Sprintf("start_timestamp=%d&end_timestamp=%d",
		now+7200, now+3600))

	assert.LessOrEqual(t, payload.Data.EndTimestamp, now)
	assert.Less(t, payload.Data.StartTimestamp, payload.Data.EndTimestamp)
	assert.Empty(t, payload.Data.Channels)
}

func TestGetChannelUsageDefaultsToTheLastSevenDays(t *testing.T) {
	setupChannelUsageControllerTest(t)
	now := common.GetTimestamp()

	payload := callChannelUsage(t, "")

	assert.Equal(t, now, payload.Data.EndTimestamp)
	assert.Equal(t, now-7*24*3600, payload.Data.StartTimestamp)
}

func TestGetChannelUsageFiltersOneChannel(t *testing.T) {
	setupChannelUsageControllerTest(t)
	now := common.GetTimestamp()
	hour := now - now%3600
	insertChannelUsage(t, model.ChannelUsageData{
		ChannelId: 5, KeyFingerprint: "", ModelName: "gpt-4o", CreatedAt: hour, NodeName: "n1",
		RequestCount: 1, SuccessCount: 1,
	})
	insertChannelUsage(t, model.ChannelUsageData{
		ChannelId: 6, KeyFingerprint: "", ModelName: "gpt-4o", CreatedAt: hour, NodeName: "n1",
		RequestCount: 2, SuccessCount: 2,
	})

	payload := callChannelUsage(t, fmt.Sprintf("start_timestamp=%d&end_timestamp=%d&channel_id=6", hour, now))
	require.Len(t, payload.Data.Channels, 1)
	assert.Equal(t, 6, payload.Data.Channels[0].ChannelId)
}
