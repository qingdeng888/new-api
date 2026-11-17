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
	"net/http"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// ChannelUsageTotals is what one channel, key or model row displays.
type ChannelUsageTotals struct {
	RequestCount     int   `json:"request_count"`
	SuccessCount     int   `json:"success_count"`
	FailureCount     int   `json:"failure_count"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	Quota            int   `json:"quota"`
}

// ChannelUsageModel is one model inside a key.
type ChannelUsageModel struct {
	ModelName string `json:"model_name"`
	ChannelUsageTotals
}

// ChannelUsageKey is one channel key. Removed marks usage whose key is no longer
// in the channel, so the row stays visible after a key is deleted. A key index
// of -1 means the attempt carried no key (task settlement).
type ChannelUsageKey struct {
	KeyIndex    int    `json:"key_index"`
	KeyLabel    string `json:"key_label"`
	Fingerprint string `json:"fingerprint"`
	Removed     bool   `json:"removed"`
	ChannelUsageTotals
	Models []ChannelUsageModel `json:"models"`
}

// ChannelUsageChannel is one channel with its keys.
type ChannelUsageChannel struct {
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	ChannelType int    `json:"channel_type"`
	// Removed marks a channel whose usage outlived the channel itself.
	Removed bool `json:"removed"`
	ChannelUsageTotals
	Keys []ChannelUsageKey `json:"keys"`
}

type ChannelUsageResponse struct {
	StartTimestamp int64                 `json:"start_timestamp"`
	EndTimestamp   int64                 `json:"end_timestamp"`
	Channels       []ChannelUsageChannel `json:"channels"`
}

// GetChannelUsage serves the channel statistics tab: every channel with its
// keys and the models each key served over the requested window. The window is
// clamped to the retention period, so a caller asking for a year still reads a
// bounded range and is told which range answered.
func GetChannelUsage(c *gin.Context) {
	now := common.GetTimestamp()
	earliest := now - model.ChannelUsageRetentionSeconds

	startTime := parseInt64Query(c, "start_timestamp", now-7*24*3600)
	endTime := parseInt64Query(c, "end_timestamp", now)
	// Clamp both bounds into the answerable window, then keep the interval
	// forward, so no caller can make the answer describe a future range or one
	// longer than the retention.
	endTime = min(endTime, now)
	startTime = max(min(startTime, now), earliest)
	if startTime >= endTime {
		startTime = max(earliest, endTime-1)
	}
	if endTime <= startTime {
		endTime = startTime + 1
	}
	if endTime-startTime > model.ChannelUsageRetentionSeconds {
		startTime = endTime - model.ChannelUsageRetentionSeconds
	}
	channelId, _ := strconv.Atoi(c.Query("channel_id"))

	rows, err := model.GetChannelUsageAggregates(startTime, endTime, channelId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	channels, err := buildChannelUsage(rows)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": ChannelUsageResponse{
			StartTimestamp: startTime,
			EndTimestamp:   endTime,
			Channels:       channels,
		},
	})
}

func parseInt64Query(c *gin.Context, name string, fallback int64) int64 {
	raw := c.Query(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

// buildChannelUsage folds the aggregate rows into the channel → key → model tree
// the console shows, naming keys from the channel's current key list.
func buildChannelUsage(rows []*model.ChannelUsageAggregate) ([]ChannelUsageChannel, error) {
	channelIds := make([]int, 0, len(rows))
	seenChannel := make(map[int]bool, len(rows))
	for _, row := range rows {
		if !seenChannel[row.ChannelId] {
			seenChannel[row.ChannelId] = true
			channelIds = append(channelIds, row.ChannelId)
		}
	}

	channelsById := make(map[int]*model.Channel, len(channelIds))
	if len(channelIds) > 0 {
		stored, err := model.GetChannelsByIds(channelIds)
		if err != nil {
			return nil, err
		}
		for _, channel := range stored {
			channelsById[channel.Id] = channel
		}
	}
	// Resolve each channel's keys once; a channel can hold many keys and every
	// aggregate row would otherwise rescan them.
	keySlotsByChannel := make(map[int]*channelKeySlots, len(channelsById))
	for channelId, channel := range channelsById {
		keys := channel.GetKeys()
		slots := &channelKeySlots{
			index: make(map[string]int, len(keys)),
			label: make(map[string]string, len(keys)),
		}
		for index, key := range keys {
			fingerprint := model.ChannelKeyFingerprint(key)
			if _, seen := slots.index[fingerprint]; seen {
				continue
			}
			slots.index[fingerprint] = index
			slots.label[fingerprint] = model.MaskTokenKey(key)
		}
		keySlotsByChannel[channelId] = slots
	}

	type keyBucket struct {
		key     ChannelUsageKey
		models  map[string]*ChannelUsageModel
		removed bool
	}
	// channel id → key fingerprint (or "" ) → bucket
	byChannel := make(map[int]map[string]*keyBucket, len(channelIds))

	for _, row := range rows {
		keyIndex := -1
		keyLabel := ""
		removed := false
		if slots, exists := keySlotsByChannel[row.ChannelId]; exists {
			if index, found := slots.index[row.KeyFingerprint]; found {
				keyIndex = index
				keyLabel = slots.label[row.KeyFingerprint]
			} else {
				removed = row.KeyFingerprint != ""
			}
		}

		keys, ok := byChannel[row.ChannelId]
		if !ok {
			keys = make(map[string]*keyBucket)
			byChannel[row.ChannelId] = keys
		}
		bucket, ok := keys[row.KeyFingerprint]
		if !ok {
			bucket = &keyBucket{
				key: ChannelUsageKey{
					KeyIndex:    keyIndex,
					KeyLabel:    keyLabel,
					Fingerprint: row.KeyFingerprint,
					Removed:     removed,
				},
				models: make(map[string]*ChannelUsageModel),
			}
			keys[row.KeyFingerprint] = bucket
		}
		addAggregate(&bucket.key.ChannelUsageTotals, row)

		modelRow, ok := bucket.models[row.ModelName]
		if !ok {
			modelRow = &ChannelUsageModel{ModelName: row.ModelName}
			bucket.models[row.ModelName] = modelRow
		}
		addAggregate(&modelRow.ChannelUsageTotals, row)
	}

	result := make([]ChannelUsageChannel, 0, len(byChannel))
	for channelId, keys := range byChannel {
		entry := ChannelUsageChannel{ChannelId: channelId}
		if channel, exists := channelsById[channelId]; exists {
			entry.ChannelName = channel.Name
			entry.ChannelType = channel.Type
		} else {
			entry.Removed = true
		}
		for _, bucket := range keys {
			for _, modelRow := range bucket.models {
				bucket.key.Models = append(bucket.key.Models, *modelRow)
			}
			sort.Slice(bucket.key.Models, func(i, j int) bool {
				return bucket.key.Models[i].RequestCount > bucket.key.Models[j].RequestCount
			})
			addTotals(&entry.ChannelUsageTotals, bucket.key.ChannelUsageTotals)
			entry.Keys = append(entry.Keys, bucket.key)
		}
		// Current keys first in their list order, then attempts that named no
		// key, then usage whose key no longer exists.
		sort.Slice(entry.Keys, func(i, j int) bool {
			left, right := entry.Keys[i], entry.Keys[j]
			if left.Removed != right.Removed {
				return !left.Removed
			}
			if (left.KeyIndex < 0) != (right.KeyIndex < 0) {
				return left.KeyIndex >= 0
			}
			if left.KeyIndex != right.KeyIndex {
				return left.KeyIndex < right.KeyIndex
			}
			return left.Fingerprint < right.Fingerprint
		})
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].RequestCount != result[j].RequestCount {
			return result[i].RequestCount > result[j].RequestCount
		}
		return result[i].ChannelId < result[j].ChannelId
	})
	return result, nil
}

// channelKeySlots maps a channel's key fingerprints to their position and
// masked label.
type channelKeySlots struct {
	index map[string]int
	label map[string]string
}

func addAggregate(target *ChannelUsageTotals, row *model.ChannelUsageAggregate) {
	addTotals(target, ChannelUsageTotals{
		RequestCount:     row.RequestCount,
		SuccessCount:     row.SuccessCount,
		FailureCount:     row.FailureCount,
		PromptTokens:     row.PromptTokens,
		CompletionTokens: row.CompletionTokens,
		CacheReadTokens:  row.CacheReadTokens,
		Quota:            row.Quota,
	})
}

func addTotals(target *ChannelUsageTotals, added ChannelUsageTotals) {
	target.RequestCount += added.RequestCount
	target.SuccessCount += added.SuccessCount
	target.FailureCount += added.FailureCount
	target.PromptTokens += added.PromptTokens
	target.CompletionTokens += added.CompletionTokens
	target.CacheReadTokens += added.CacheReadTokens
	target.Quota += added.Quota
}
