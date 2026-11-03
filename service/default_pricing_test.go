package service

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupDefaultPricingTest swaps in an isolated SQLite database (which also
// initializes the dialect-specific column names) and wipes the options table
// so each test starts from a clean pricing state.
func setupDefaultPricingTest(t *testing.T) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousSQLite := common.SQLitePath
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SQLitePath = previousSQLite
	})
	common.SQLitePath = filepath.Join(t.TempDir(), "pricing.db")
	require.NoError(t, model.InitDB())
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	model.InitOptionMap()
	t.Cleanup(model.InitOptionMap)
	t.Cleanup(func() {
		sqlDB, err := model.DB.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
}

func TestEnsureDefaultPricingForModelsCreatesEntries(t *testing.T) {
	setupDefaultPricingTest(t)

	created := EnsureDefaultPricingForModels([]string{"  ", "custom-model-x", "custom-model-x"})
	require.Equal(t, []string{"custom-model-x"}, created)

	snapshot, err := model.GetModelPricingSnapshot([]string{"custom-model-x"})
	require.NoError(t, err)
	require.Len(t, snapshot.Entries, 1)
	configured := snapshot.Entries[0].Configured
	assert.Equal(t, "ratio", configured["billing_setting.billing_mode"])
	assert.EqualValues(t, defaultPricingModelRatio(), configured["ModelRatio"])
	assert.EqualValues(t, defaultPricingCompletionRatio(), configured["CompletionRatio"])
	assert.EqualValues(t, defaultPricingCacheRatio(), configured["CacheRatio"])

	// Idempotent: a second run creates nothing new.
	assert.Nil(t, EnsureDefaultPricingForModels([]string{"custom-model-x"}))
}

func TestEnsureDefaultPricingSkipsConfiguredModels(t *testing.T) {
	setupDefaultPricingTest(t)
	// An admin-configured price for "priced-model" must not be overwritten.
	pricedSnapshot, err := model.GetModelPricingSnapshot([]string{"priced-model"})
	require.NoError(t, err)
	require.NoError(t, model.UpdateModelPricing([]model.ModelPricingChange{{
		ModelName:       "priced-model",
		ExpectedVersion: pricedSnapshot.EmptyVersion,
		Pricing:         model.PricingValues{"ModelRatio": 99.0},
	}}))

	created := EnsureDefaultPricingForModels([]string{"priced-model"})
	assert.Empty(t, created)

	stored, err := model.GetModelPricingSnapshot([]string{"priced-model"})
	require.NoError(t, err)
	require.Len(t, stored.Entries, 1)
	assert.EqualValues(t, 99.0, stored.Entries[0].Configured["ModelRatio"])
}

func TestDefaultPricingRatioDerivation(t *testing.T) {
	assert.Equal(t, DefaultPricingInputUSDPerMillion*common.QuotaPerUnit/1e6, defaultPricingModelRatio())
	assert.Equal(t, DefaultPricingCompletionUSDPerMillion/DefaultPricingInputUSDPerMillion, defaultPricingCompletionRatio())
	assert.Equal(t, DefaultPricingCacheReadUSDPerMillion/DefaultPricingInputUSDPerMillion, defaultPricingCacheRatio())
}
