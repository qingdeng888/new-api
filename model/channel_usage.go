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

package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ChannelUsageRetentionSeconds bounds how far back channel usage is aggregated.
// Older rows are deleted, so the table never exceeds the window a console reader
// can ask for.
const ChannelUsageRetentionSeconds int64 = 30 * 24 * 3600

const (
	// channelUsageFlushInterval is how often buffered counters reach the
	// database. A crash loses at most this window, matching quota_data.
	channelUsageFlushInterval = 5 * time.Minute
	// channelUsageDeleteBatchSize keeps the retention delete from holding a
	// large transaction on a long-neglected table.
	channelUsageDeleteBatchSize = 5000
)

// ChannelUsageData sums one channel's traffic on one key, model and node for one
// hour. It is written from the log funnels, so it counts every upstream attempt
// rather than every client request.
type ChannelUsageData struct {
	Id int `json:"id"`
	// The five dimension columns form the uniqueness key, so a second node
	// writing the same hour adds to the existing row instead of duplicating it.
	ChannelId        int    `json:"channel_id" gorm:"uniqueIndex:idx_cud_dimension,priority:1;index:idx_cud_channel_time,priority:1"`
	KeyFingerprint   string `json:"key_fingerprint" gorm:"size:16;default:'';uniqueIndex:idx_cud_dimension,priority:2;index:idx_cud_channel_key,priority:2"`
	ModelName        string `json:"model_name" gorm:"size:64;default:'';uniqueIndex:idx_cud_dimension,priority:3;index:idx_cud_channel_time,priority:3"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;uniqueIndex:idx_cud_dimension,priority:4;index:idx_cud_channel_time,priority:2;index:idx_cud_created_at"`
	NodeName         string `json:"node_name" gorm:"size:64;default:'';uniqueIndex:idx_cud_dimension,priority:5;index:idx_cud_channel_key,priority:3"`
	RequestCount     int    `json:"request_count"`
	SuccessCount     int    `json:"success_count"`
	FailureCount     int    `json:"failure_count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	Quota            int    `json:"quota"`
}

// ChannelUsageLogParams is one upstream attempt as the log funnels see it.
type ChannelUsageLogParams struct {
	ChannelId        int
	KeyFingerprint   string
	ModelName        string
	NodeName         string
	CreatedAt        int64
	Success          bool
	PromptTokens     int
	CompletionTokens int
	CacheReadTokens  int
	Quota            int
}

// recordChannelUsage captures the attempt behind a relay log. The key comes from
// the channel selection the distributor made, so it names the key that actually
// served or failed the attempt.
func recordChannelUsage(c *gin.Context, channelId int, modelName string, success bool,
	promptTokens, completionTokens, cacheReadTokens, quota int) {
	keyFingerprint := ""
	if c != nil {
		keyFingerprint = ChannelKeyFingerprint(common.GetContextKeyString(c, constant.ContextKeyChannelKey))
	}
	LogChannelUsage(ChannelUsageLogParams{
		ChannelId:        channelId,
		KeyFingerprint:   keyFingerprint,
		ModelName:        modelName,
		CreatedAt:        common.GetTimestamp(),
		Success:          success,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		CacheReadTokens:  cacheReadTokens,
		Quota:            quota,
	})
}

// ChannelKeyFingerprint identifies a channel key without keeping it. Keys are
// secrets, so the aggregate stores a truncated hash: enough to tell a channel's
// keys apart and to match them against the current key list, not enough to
// recover anything.
func ChannelKeyFingerprint(key string) string {
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

// channelUsageKey is the buffer key of one aggregate row: the same five columns
// the table's unique index covers.
func channelUsageKey(channelId int, keyFingerprint, modelName string, createdAt int64, nodeName string) string {
	return fmt.Sprintf("%d\x00%s\x00%s\x00%d\x00%s", channelId, keyFingerprint, modelName, createdAt, nodeName)
}

var (
	channelUsageCache     = make(map[string]*ChannelUsageData)
	channelUsageCacheLock sync.Mutex
)

// LogChannelUsage buffers one attempt. Requests that never reached a channel
// carry no channel id and are not channel usage.
func LogChannelUsage(params ChannelUsageLogParams) {
	if params.ChannelId <= 0 {
		return
	}
	createdAt := params.CreatedAt
	if createdAt == 0 {
		createdAt = common.GetTimestamp()
	}
	// Bucket to the hour, like quota_data: a row per hour per dimension.
	createdAt -= createdAt % 3600
	nodeName := params.NodeName
	if nodeName == "" {
		nodeName = common.NodeName
	}
	row := &ChannelUsageData{
		ChannelId:        params.ChannelId,
		KeyFingerprint:   params.KeyFingerprint,
		ModelName:        params.ModelName,
		CreatedAt:        createdAt,
		NodeName:         nodeName,
		RequestCount:     1,
		PromptTokens:     max(0, int64(params.PromptTokens)),
		CompletionTokens: max(0, int64(params.CompletionTokens)),
		CacheReadTokens:  max(0, int64(params.CacheReadTokens)),
		Quota:            params.Quota,
	}
	if params.Success {
		row.SuccessCount = 1
	} else {
		row.FailureCount = 1
	}

	key := channelUsageKey(row.ChannelId, row.KeyFingerprint, row.ModelName, row.CreatedAt, row.NodeName)

	channelUsageCacheLock.Lock()
	defer channelUsageCacheLock.Unlock()
	if buffered, ok := channelUsageCache[key]; ok {
		buffered.RequestCount += row.RequestCount
		buffered.SuccessCount += row.SuccessCount
		buffered.FailureCount += row.FailureCount
		buffered.PromptTokens += row.PromptTokens
		buffered.CompletionTokens += row.CompletionTokens
		buffered.CacheReadTokens += row.CacheReadTokens
		buffered.Quota += row.Quota
		return
	}
	channelUsageCache[key] = row
}

// SaveChannelUsageCache writes the buffered rows and empties the buffer. A row
// that grows during the write is written again by the next flush.
func SaveChannelUsageCache() error {
	channelUsageCacheLock.Lock()
	pending := channelUsageCache
	channelUsageCache = make(map[string]*ChannelUsageData)
	channelUsageCacheLock.Unlock()

	if len(pending) == 0 {
		return nil
	}
	var firstErr error
	touched := make(map[int]bool, len(pending))
	for _, row := range pending {
		touched[row.ChannelId] = true
		if err := saveChannelUsageRow(row); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	// Another node may have deleted a key in the meantime and left this batch
	// with rows for it; drop what the channel no longer has.
	for channelId := range touched {
		if _, err := PruneChannelUsageKeys(channelId); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	common.SysLog(common.LogText("saved channel usage data, %d records", len(pending)))
	return firstErr
}

// saveChannelUsageRow adds to the matching row, inserting it when this is the
// first write for the hour. Two nodes can reach the insert together; the unique
// dimension index turns the loser's insert into an error it resolves by adding
// to the winner's row.
func saveChannelUsageRow(row *ChannelUsageData) error {
	increase := func() *gorm.DB {
		return DB.Model(&ChannelUsageData{}).
			Where("channel_id = ? AND key_fingerprint = ? AND model_name = ? AND created_at = ? AND node_name = ?",
				row.ChannelId, row.KeyFingerprint, row.ModelName, row.CreatedAt, row.NodeName).
			Updates(map[string]any{
				"request_count":     gorm.Expr("request_count + ?", row.RequestCount),
				"success_count":     gorm.Expr("success_count + ?", row.SuccessCount),
				"failure_count":     gorm.Expr("failure_count + ?", row.FailureCount),
				"prompt_tokens":     gorm.Expr("prompt_tokens + ?", row.PromptTokens),
				"completion_tokens": gorm.Expr("completion_tokens + ?", row.CompletionTokens),
				"cache_read_tokens": gorm.Expr("cache_read_tokens + ?", row.CacheReadTokens),
				"quota":             gorm.Expr("quota + ?", row.Quota),
			})
	}
	result := increase()
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	if err := DB.Create(row).Error; err != nil {
		return increase().Error
	}
	return nil
}

// DeleteExpiredChannelUsage removes rows older than the retention window.
func DeleteExpiredChannelUsage(ctx context.Context, now int64) (int64, error) {
	before := now - ChannelUsageRetentionSeconds
	var deleted int64
	for {
		// MySQL rejects LIMIT inside an IN subquery, so the batch of ids is
		// collected first and deleted by value.
		var ids []int
		if err := DB.Model(&ChannelUsageData{}).
			Where("created_at < ?", before).
			Limit(channelUsageDeleteBatchSize).
			Pluck("id", &ids).Error; err != nil {
			return deleted, err
		}
		if len(ids) == 0 {
			return deleted, nil
		}
		result := DB.WithContext(ctx).Where("id IN ?", ids).Delete(&ChannelUsageData{})
		if result.Error != nil {
			return deleted, result.Error
		}
		deleted += result.RowsAffected
		if len(ids) < channelUsageDeleteBatchSize {
			return deleted, nil
		}
	}
}

// DeleteOrphanedChannelUsage removes usage of channels that no longer exist, so
// a channel deleted on another node stops being counted even when this process
// never handled the deletion.
func DeleteOrphanedChannelUsage(ctx context.Context) (int64, error) {
	var channelIds []int
	if err := DB.Model(&Channel{}).Pluck("id", &channelIds).Error; err != nil {
		return 0, err
	}
	if len(channelIds) == 0 {
		return DB.WithContext(ctx).Where("1 = 1").Delete(&ChannelUsageData{}).RowsAffected, nil
	}
	result := DB.WithContext(ctx).Where("channel_id NOT IN ?", channelIds).Delete(&ChannelUsageData{})
	return result.RowsAffected, result.Error
}

// UpdateChannelUsageData flushes buffered counters and prunes expired rows once
// a day. It runs for the life of the process.
func UpdateChannelUsageData() {
	var lastCleanupDay int64
	for {
		time.Sleep(channelUsageFlushInterval)
		if err := SaveChannelUsageCache(); err != nil {
			common.SysLog(common.LogText("failed to save channel usage data: %s", err.Error()))
		}
		day := common.GetTimestamp() / 86400
		if day == lastCleanupDay {
			continue
		}
		lastCleanupDay = day
		if orphaned, err := DeleteOrphanedChannelUsage(context.Background()); err != nil {
			common.SysLog(common.LogText("failed to delete orphaned channel usage data: %s", err.Error()))
			lastCleanupDay = 0
		} else if orphaned > 0 {
			common.SysLog(common.LogText("deleted %d channel usage records of removed channels", orphaned))
		}
		deleted, err := DeleteExpiredChannelUsage(context.Background(), common.GetTimestamp())
		if err != nil {
			common.SysLog(common.LogText("failed to delete expired channel usage data: %s", err.Error()))
			// Retry on the next flush rather than waiting for tomorrow.
			lastCleanupDay = 0
			continue
		}
		if deleted > 0 {
			common.SysLog(common.LogText("deleted %d expired channel usage records", deleted))
		}
	}
}

// ChannelUsageAggregate is one channel key's total for one model over a window.
type ChannelUsageAggregate struct {
	ChannelId        int    `json:"channel_id"`
	KeyFingerprint   string `json:"key_fingerprint"`
	ModelName        string `json:"model_name"`
	RequestCount     int    `json:"request_count"`
	SuccessCount     int    `json:"success_count"`
	FailureCount     int    `json:"failure_count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	Quota            int    `json:"quota"`
}

// GetChannelUsageAggregates sums the stored hours of a window, one row per
// channel, key and model. An empty channelId keeps every channel.
func GetChannelUsageAggregates(startTime, endTime int64, channelId int) ([]*ChannelUsageAggregate, error) {
	query := DB.Model(&ChannelUsageData{}).
		Select("channel_id, key_fingerprint, model_name,"+
			" sum(request_count) as request_count, sum(success_count) as success_count,"+
			" sum(failure_count) as failure_count, sum(prompt_tokens) as prompt_tokens,"+
			" sum(completion_tokens) as completion_tokens, sum(cache_read_tokens) as cache_read_tokens,"+
			" sum(quota) as quota").
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Group("channel_id, key_fingerprint, model_name")
	if channelId > 0 {
		query = query.Where("channel_id = ?", channelId)
	}
	var rows []*ChannelUsageAggregate
	err := query.Find(&rows).Error
	return rows, err
}

// PruneChannelUsageKeys deletes usage whose key is no longer in the channel, and
// the same rows still held in this process's buffer, so deleting a key removes
// its statistics rather than leaving them behind as an orphan.
//
// A channel with no keys is left alone: that state is a channel being edited,
// not evidence that every key was deleted.
func PruneChannelUsageKeys(channelId int) (int64, error) {
	channel, err := GetChannelById(channelId, true)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return 0, nil
	}
	keep := make([]string, 0, len(keys))
	keepSet := make(map[string]bool, len(keys))
	for _, key := range keys {
		fingerprint := ChannelKeyFingerprint(key)
		keep = append(keep, fingerprint)
		keepSet[fingerprint] = true
	}

	channelUsageCacheLock.Lock()
	for bufferKey, row := range channelUsageCache {
		if row.ChannelId == channelId && row.KeyFingerprint != "" && !keepSet[row.KeyFingerprint] {
			delete(channelUsageCache, bufferKey)
		}
	}
	channelUsageCacheLock.Unlock()

	result := DB.Where("channel_id = ? AND key_fingerprint <> ''", channelId).
		Where("key_fingerprint NOT IN ?", keep).
		Delete(&ChannelUsageData{})
	return result.RowsAffected, result.Error
}

// DeleteChannelUsageByChannelIds removes every usage row of the given channels,
// for a deleted channel leaving no statistics behind.
func DeleteChannelUsageByChannelIds(ctx context.Context, channelIds []int) (int64, error) {
	if len(channelIds) == 0 {
		return 0, nil
	}
	channelUsageCacheLock.Lock()
	for bufferKey, row := range channelUsageCache {
		if slices.Contains(channelIds, row.ChannelId) {
			delete(channelUsageCache, bufferKey)
		}
	}
	channelUsageCacheLock.Unlock()

	result := DB.WithContext(ctx).Where("channel_id IN ?", channelIds).Delete(&ChannelUsageData{})
	return result.RowsAffected, result.Error
}
