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
	"errors"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func resetChannelUsageCache(t *testing.T) {
	t.Helper()
	channelUsageCacheLock.Lock()
	channelUsageCache = make(map[string]*ChannelUsageData)
	channelUsageCacheLock.Unlock()
	t.Cleanup(func() {
		channelUsageCacheLock.Lock()
		channelUsageCache = make(map[string]*ChannelUsageData)
		channelUsageCacheLock.Unlock()
	})
}

func setupChannelUsageDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	var driver gorm.Dialector
	switch dialect {
	case "sqlite":
		driver = sqlite.Open(":memory:")
	case "mysql":
		dsn := os.Getenv("TEST_MYSQL_DSN")
		if dsn == "" {
			t.Skip("TEST_MYSQL_DSN not configured")
		}
		driver = mysql.Open(dsn)
	case "postgres":
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("TEST_POSTGRES_DSN not configured")
		}
		driver = postgres.Open(dsn)
	default:
		t.Fatalf("unsupported dialect %q", dialect)
	}

	db, err := gorm.Open(driver, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "cud_test_"},
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		// Pruning reads the channel row, so every test needs that table too.
		require.NoError(t, db.Migrator().DropTable(&ChannelUsageData{}, &Channel{}))
		DB = previousDB
		require.NoError(t, sqlDB.Close())
	})

	// Startup migrates on every boot, so a second pass must be a no-op.
	require.NoError(t, db.AutoMigrate(&ChannelUsageData{}, &Channel{}))
	require.NoError(t, db.AutoMigrate(&ChannelUsageData{}, &Channel{}))
	return db
}

func TestChannelKeyFingerprintIdentifiesWithoutKeepingTheKey(t *testing.T) {
	first := ChannelKeyFingerprint("sk-first-secret")
	second := ChannelKeyFingerprint("sk-second-secret")

	assert.Len(t, first, 16)
	assert.Equal(t, first, ChannelKeyFingerprint("sk-first-secret"), "the same key keeps its fingerprint")
	assert.NotEqual(t, first, second)
	assert.NotContains(t, first, "sk-")
	assert.Empty(t, ChannelKeyFingerprint(""), "a channel with no key names no key")
}

func TestLogChannelUsageBuffersEveryAttempt(t *testing.T) {
	resetChannelUsageCache(t)
	hourStart := int64(1_700_000_000) - 1_700_000_000%3600

	base := ChannelUsageLogParams{
		ChannelId: 7, KeyFingerprint: "aaaa", ModelName: "gpt-4o", NodeName: "n1",
		CreatedAt: hourStart + 120,
	}
	success := base
	success.Success = true
	success.PromptTokens = 100
	success.CompletionTokens = 40
	success.CacheReadTokens = 25
	success.Quota = 900
	LogChannelUsage(success)
	LogChannelUsage(success)

	failure := base
	failure.Success = false
	LogChannelUsage(failure)

	// A different model, key, node and hour each keep their own row.
	otherModel := success
	otherModel.ModelName = "claude-3"
	LogChannelUsage(otherModel)
	otherKey := success
	otherKey.KeyFingerprint = "bbbb"
	LogChannelUsage(otherKey)
	otherNode := success
	otherNode.NodeName = "n2"
	LogChannelUsage(otherNode)
	nextHour := success
	nextHour.CreatedAt = hourStart + 3700
	LogChannelUsage(nextHour)

	channelUsageCacheLock.Lock()
	defer channelUsageCacheLock.Unlock()

	require.Len(t, channelUsageCache, 5, "one row per distinct channel, key, model, hour and node")
	row, ok := channelUsageCache[channelUsageKey(7, "aaaa", "gpt-4o", hourStart, "n1")]
	require.True(t, ok)
	assert.Equal(t, 3, row.RequestCount)
	assert.Equal(t, 2, row.SuccessCount)
	assert.Equal(t, 1, row.FailureCount)
	assert.EqualValues(t, 200, row.PromptTokens)
	assert.EqualValues(t, 80, row.CompletionTokens)
	assert.EqualValues(t, 50, row.CacheReadTokens)
	assert.Equal(t, 1800, row.Quota)
}

func TestLogChannelUsageIgnoresAttemptsWithoutChannel(t *testing.T) {
	resetChannelUsageCache(t)
	LogChannelUsage(ChannelUsageLogParams{ChannelId: 0, ModelName: "gpt-4o", Success: true})
	LogChannelUsage(ChannelUsageLogParams{ChannelId: -1, ModelName: "gpt-4o", Success: true})

	channelUsageCacheLock.Lock()
	defer channelUsageCacheLock.Unlock()
	assert.Empty(t, channelUsageCache)
}

func TestRecordChannelUsageNamesTheSelectedChannelKey(t *testing.T) {
	resetChannelUsageCache(t)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "sk-channel-key")

	RecordChannelUsageAttempt(ctx, 3, "gpt-4o", true, 10, 5, 0, 1)

	channelUsageCacheLock.Lock()
	defer channelUsageCacheLock.Unlock()
	require.Len(t, channelUsageCache, 1)
	for _, row := range channelUsageCache {
		assert.Equal(t, ChannelKeyFingerprint("sk-channel-key"), row.KeyFingerprint)
	}
}

func TestSaveChannelUsageCacheRetriesFailedWritesWithNewAttempts(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupChannelUsageDB(t, dialect)
			resetChannelUsageCache(t)
			params := ChannelUsageLogParams{
				ChannelId: 51, ModelName: "gpt-4o", NodeName: "n1",
				CreatedAt: 1_700_000_000, Success: true, PromptTokens: 10, Quota: 100,
			}
			LogChannelUsage(params)
			LogChannelUsage(params)

			writeErr := errors.New("temporary channel usage write failure")
			var newAttempt sync.Once
			const callback = "test:channel_usage_write_failure"
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Schema == nil || tx.Statement.Schema.Name != "ChannelUsageData" {
					return
				}
				newAttempt.Do(func() {
					failure := params
					failure.Success = false
					failure.PromptTokens = 0
					failure.Quota = 0
					LogChannelUsage(failure)
				})
				tx.AddError(writeErr)
			}))
			t.Cleanup(func() { db.Callback().Update().Remove(callback) })
			require.ErrorIs(t, SaveChannelUsageCache(), writeErr)
			require.NoError(t, db.Callback().Update().Remove(callback))

			require.NoError(t, SaveChannelUsageCache())
			// A further flush must not count the recovered batch again.
			require.NoError(t, SaveChannelUsageCache())
			var rows []ChannelUsageData
			require.NoError(t, db.Find(&rows).Error)
			require.Len(t, rows, 1)
			assert.Equal(t, 3, rows[0].RequestCount)
			assert.Equal(t, 2, rows[0].SuccessCount)
			assert.Equal(t, 1, rows[0].FailureCount)
			assert.EqualValues(t, 20, rows[0].PromptTokens)
			assert.Equal(t, 200, rows[0].Quota)
		})
	}
}

func TestSaveChannelUsageRowReportsInsertFailure(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupChannelUsageDB(t, dialect)
			writeErr := errors.New("channel usage insert rejected")
			const callback = "test:channel_usage_insert_failure"
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "ChannelUsageData" {
					tx.AddError(writeErr)
				}
			}))
			t.Cleanup(func() { db.Callback().Create().Remove(callback) })

			err := saveChannelUsageRow(&ChannelUsageData{
				ChannelId: 52, ModelName: "gpt-4o", CreatedAt: 1_700_000_000,
				RequestCount: 1, SuccessCount: 1,
			})
			assert.ErrorIs(t, err, writeErr)
		})
	}
}

func TestSaveChannelUsageCacheAddsToTheExistingHour(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupChannelUsageDB(t, dialect)
			resetChannelUsageCache(t)
			hourStart := int64(1_700_000_000) - 1_700_000_000%3600

			params := ChannelUsageLogParams{
				ChannelId: 9, KeyFingerprint: "cccc", ModelName: "gpt-4o", NodeName: "n1",
				CreatedAt: hourStart, Success: true, PromptTokens: 10, CompletionTokens: 5, Quota: 100,
			}
			LogChannelUsage(params)
			require.NoError(t, SaveChannelUsageCache())
			// A later flush for the same hour adds to the stored row rather than
			// inserting a second one.
			LogChannelUsage(params)
			require.NoError(t, SaveChannelUsageCache())

			var rows []ChannelUsageData
			require.NoError(t, db.Find(&rows).Error)
			require.Len(t, rows, 1)
			assert.Equal(t, 2, rows[0].RequestCount)
			assert.Equal(t, 2, rows[0].SuccessCount)
			assert.EqualValues(t, 20, rows[0].PromptTokens)
			assert.EqualValues(t, 10, rows[0].CompletionTokens)
			assert.Equal(t, 200, rows[0].Quota)
		})
	}
}

func TestSaveChannelUsageResolvesAPeerInsert(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupChannelUsageDB(t, dialect)
			hourStart := int64(1_700_000_000) - 1_700_000_000%3600
			// Another node stored this hour first.
			require.NoError(t, db.Create(&ChannelUsageData{
				ChannelId: 4, KeyFingerprint: "dddd", ModelName: "gpt-4o", CreatedAt: hourStart,
				NodeName: "n1", RequestCount: 1, SuccessCount: 1,
			}).Error)

			require.NoError(t, saveChannelUsageRow(&ChannelUsageData{
				ChannelId: 4, KeyFingerprint: "dddd", ModelName: "gpt-4o", CreatedAt: hourStart,
				NodeName: "n1", RequestCount: 1, SuccessCount: 1,
			}))

			var rows []ChannelUsageData
			require.NoError(t, db.Find(&rows).Error)
			require.Len(t, rows, 1, "the second writer must not duplicate the hour")
			assert.Equal(t, 2, rows[0].RequestCount)
		})
	}
}

func TestDeleteExpiredChannelUsageKeepsTheRetentionWindow(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupChannelUsageDB(t, dialect)
			now := int64(1_700_000_000)
			expired := now - ChannelUsageRetentionSeconds - 3600
			recent := now - ChannelUsageRetentionSeconds + 3600
			require.NoError(t, db.Create(&ChannelUsageData{ChannelId: 1, ModelName: "old", CreatedAt: expired}).Error)
			require.NoError(t, db.Create(&ChannelUsageData{ChannelId: 1, ModelName: "new", CreatedAt: recent}).Error)

			deleted, err := DeleteExpiredChannelUsage(context.Background(), now)
			require.NoError(t, err)
			assert.EqualValues(t, 1, deleted)

			var remaining []ChannelUsageData
			require.NoError(t, db.Find(&remaining).Error)
			require.Len(t, remaining, 1)
			assert.Equal(t, "new", remaining[0].ModelName)
		})
	}
}

// The attempts the console counts come from the settlement path, and a failure
// reaches it through the relay error handler rather than the error log, so the
// error log itself must leave the counters alone.
func TestLogFunnelsRecordChannelUsage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previousDB := DB
	previousLogDB := LOG_DB
	previousType := common.MainDatabaseType()
	DB = db
	LOG_DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
		common.SetMainDatabaseType(previousType)
	})
	require.NoError(t, db.AutoMigrate(&Log{}, &ChannelUsageData{}, &Channel{}))
	resetChannelUsageCache(t)

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "sk-first")

	other := NewLogOther()
	other.SetPublic("cache_tokens", 25)
	RecordConsumeLog(ctx, 1, RecordConsumeLogParams{
		ChannelId:        5,
		ModelName:        "gpt-4o",
		PromptTokens:     100,
		CompletionTokens: 40,
		Quota:            900,
		Group:            "default",
		Other:            other,
	})
	RecordErrorLog(ctx, 1, 5, "gpt-4o", "token-name", "upstream refused", 0, 1, false, "default", NewLogOther())

	require.NoError(t, SaveChannelUsageCache())

	var rows []ChannelUsageData
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1, "the error log alone adds no traffic")
	row := rows[0]
	assert.Equal(t, 5, row.ChannelId)
	assert.Equal(t, ChannelKeyFingerprint("sk-first"), row.KeyFingerprint)
	assert.Equal(t, 1, row.RequestCount)
	assert.Equal(t, 1, row.SuccessCount)
	assert.Zero(t, row.FailureCount)
	assert.EqualValues(t, 100, row.PromptTokens)
	assert.EqualValues(t, 40, row.CompletionTokens)
	assert.EqualValues(t, 25, row.CacheReadTokens, "cache tokens come from the log's public scope")
	assert.Equal(t, 900, row.Quota)
}

func TestPruneChannelUsageKeysDropsDeletedKeysOnly(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupChannelUsageDB(t, dialect)
			resetChannelUsageCache(t)
			require.NoError(t, db.Create(&Channel{Id: 21, Name: "keys", Key: "sk-keep\nsk-also-keep"}).Error)

			hourStart := int64(1_700_000_000) - 1_700_000_000%3600
			rows := []ChannelUsageData{
				{ChannelId: 21, KeyFingerprint: ChannelKeyFingerprint("sk-keep"), ModelName: "m", CreatedAt: hourStart, RequestCount: 2},
				{ChannelId: 21, KeyFingerprint: ChannelKeyFingerprint("sk-deleted"), ModelName: "m", CreatedAt: hourStart, RequestCount: 3},
				{ChannelId: 21, KeyFingerprint: "", ModelName: "m", CreatedAt: hourStart, RequestCount: 1},
			}
			for i := range rows {
				require.NoError(t, db.Create(&rows[i]).Error)
			}
			// A buffered row for the deleted key must go as well, or the next
			// flush would bring it back.
			LogChannelUsage(ChannelUsageLogParams{
				ChannelId: 21, KeyFingerprint: ChannelKeyFingerprint("sk-deleted"), ModelName: "m",
				CreatedAt: hourStart, Success: true,
			})

			deleted, err := PruneChannelUsageKeys(21)
			require.NoError(t, err)
			assert.EqualValues(t, 1, deleted)

			var remaining []ChannelUsageData
			require.NoError(t, db.Find(&remaining).Error)
			require.Len(t, remaining, 2, "the kept key and the key-less row survive")
			for _, row := range remaining {
				assert.NotEqual(t, ChannelKeyFingerprint("sk-deleted"), row.KeyFingerprint)
			}
			channelUsageCacheLock.Lock()
			assert.Empty(t, channelUsageCache, "the buffer no longer holds the deleted key")
			channelUsageCacheLock.Unlock()
		})
	}
}

func TestPruneChannelUsageKeysLeavesAChannelBeingEdited(t *testing.T) {
	db := setupChannelUsageDB(t, "sqlite")
	resetChannelUsageCache(t)
	// No keys: a channel mid-edit must not lose its history.
	require.NoError(t, db.Create(&Channel{Id: 22, Name: "editing", Key: ""}).Error)
	hourStart := int64(1_700_000_000) - 1_700_000_000%3600
	require.NoError(t, db.Create(&ChannelUsageData{
		ChannelId: 22, KeyFingerprint: "abcd", ModelName: "m", CreatedAt: hourStart, RequestCount: 1,
	}).Error)

	deleted, err := PruneChannelUsageKeys(22)
	require.NoError(t, err)
	assert.EqualValues(t, 0, deleted)

	var count int64
	require.NoError(t, db.Model(&ChannelUsageData{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestDeleteChannelUsageByChannelIdsRemovesWholeChannels(t *testing.T) {
	db := setupChannelUsageDB(t, "sqlite")
	resetChannelUsageCache(t)
	hourStart := int64(1_700_000_000) - 1_700_000_000%3600
	for _, channelId := range []int{31, 32, 33} {
		require.NoError(t, db.Create(&ChannelUsageData{
			ChannelId: channelId, KeyFingerprint: "aaaa", ModelName: "m", CreatedAt: hourStart, RequestCount: 1,
		}).Error)
	}
	LogChannelUsage(ChannelUsageLogParams{ChannelId: 31, KeyFingerprint: "aaaa", ModelName: "m", CreatedAt: hourStart, Success: true})

	deleted, err := DeleteChannelUsageByChannelIds(context.Background(), []int{31, 32})
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted)

	var remaining []ChannelUsageData
	require.NoError(t, db.Find(&remaining).Error)
	require.Len(t, remaining, 1)
	assert.Equal(t, 33, remaining[0].ChannelId)
	channelUsageCacheLock.Lock()
	assert.Empty(t, channelUsageCache, "buffered rows of a deleted channel go too")
	channelUsageCacheLock.Unlock()
}

func TestDeleteOrphanedChannelUsageReportsDeleteFailureWithoutChannels(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupChannelUsageDB(t, dialect)
			require.NoError(t, db.Create(&ChannelUsageData{ChannelId: 42, ModelName: "m"}).Error)
			deleteErr := errors.New("channel usage cleanup rejected")
			const callback = "test:channel_usage_delete_failure"
			require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "ChannelUsageData" {
					tx.AddError(deleteErr)
				}
			}))
			t.Cleanup(func() { db.Callback().Delete().Remove(callback) })

			deleted, err := DeleteOrphanedChannelUsage(context.Background())
			assert.ErrorIs(t, err, deleteErr)
			assert.Zero(t, deleted)
		})
	}
}

func TestDeleteOrphanedChannelUsageRemovesDeletedChannels(t *testing.T) {
	db := setupChannelUsageDB(t, "sqlite")
	require.NoError(t, db.Create(&Channel{Id: 41, Name: "alive", Key: "sk-a"}).Error)
	hourStart := int64(1_700_000_000) - 1_700_000_000%3600
	require.NoError(t, db.Create(&ChannelUsageData{ChannelId: 41, ModelName: "m", CreatedAt: hourStart}).Error)
	require.NoError(t, db.Create(&ChannelUsageData{ChannelId: 42, ModelName: "m", CreatedAt: hourStart}).Error)

	deleted, err := DeleteOrphanedChannelUsage(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)

	var remaining []ChannelUsageData
	require.NoError(t, db.Find(&remaining).Error)
	require.Len(t, remaining, 1)
	assert.Equal(t, 41, remaining[0].ChannelId)
}
