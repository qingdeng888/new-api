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
	"net/http/httptest"
	"os"
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
		require.NoError(t, db.Migrator().DropTable(&ChannelUsageData{}))
		DB = previousDB
		require.NoError(t, sqlDB.Close())
	})

	// Startup migrates on every boot, so a second pass must be a no-op.
	require.NoError(t, db.AutoMigrate(&ChannelUsageData{}))
	require.NoError(t, db.AutoMigrate(&ChannelUsageData{}))
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

	recordChannelUsage(ctx, 3, "gpt-4o", true, 10, 5, 0, 1)

	channelUsageCacheLock.Lock()
	defer channelUsageCacheLock.Unlock()
	require.Len(t, channelUsageCache, 1)
	for _, row := range channelUsageCache {
		assert.Equal(t, ChannelKeyFingerprint("sk-channel-key"), row.KeyFingerprint)
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
