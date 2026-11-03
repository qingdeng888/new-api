package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

// Default prices applied to brand-new model IDs exposed by a channel, in USD
// per million tokens. Ratios are derived from these so billing stays correct
// if QuotaPerUnit changes.
const (
	DefaultPricingInputUSDPerMillion      = 2.0
	DefaultPricingCompletionUSDPerMillion = 8.0
	DefaultPricingCacheReadUSDPerMillion  = 0.04
)

// EnsureDefaultPricingForModels creates per-token default pricing (input
// $2/1M, completion $8/1M, cache read $0.04/1M) for models that have no
// billing configuration yet, so a model added through a channel is usable
// without a manual trip to the pricing page. Models that already resolve a
// price (admin-configured or built-in defaults) are left untouched. Failures
// are logged and never block the caller.
func EnsureDefaultPricingForModels(modelNames []string) []string {
	targets := make([]string, 0, len(modelNames))
	seen := make(map[string]bool, len(modelNames))
	for _, raw := range modelNames {
		name := strings.TrimSpace(raw)
		if name == "" || seen[name] || billing_setting.HasModelBillingConfig(name) {
			continue
		}
		seen[name] = true
		targets = append(targets, name)
	}
	if len(targets) == 0 {
		return nil
	}

	snapshot, err := model.GetModelPricingSnapshot(targets)
	if err != nil {
		common.SysLog(common.LogText("default pricing: failed to load pricing snapshot: %v", err))
		return nil
	}
	versionByModel := make(map[string]string, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		versionByModel[entry.ModelName] = entry.Version
	}

	changes := make([]model.ModelPricingChange, 0, len(targets))
	created := make([]string, 0, len(targets))
	for _, name := range targets {
		version, ok := versionByModel[name]
		if !ok {
			version = snapshot.EmptyVersion
		}
		changes = append(changes, model.ModelPricingChange{
			ModelName:       name,
			ExpectedVersion: version,
			Pricing: model.PricingValues{
				"billing_setting.billing_mode": "ratio",
				"ModelRatio":                   defaultPricingModelRatio(),
				"CompletionRatio":              defaultPricingCompletionRatio(),
				"CacheRatio":                   defaultPricingCacheRatio(),
			},
		})
		created = append(created, name)
	}
	if err := model.UpdateModelPricing(changes); err != nil {
		common.SysLog(common.LogText("default pricing: failed to create pricing for %q: %v", strings.Join(created, ", "), err))
		return nil
	}
	common.SysLog(common.LogText("default pricing created for: %s", strings.Join(created, ", ")))
	return created
}

func defaultPricingModelRatio() float64 {
	return DefaultPricingInputUSDPerMillion * common.QuotaPerUnit / 1e6
}

func defaultPricingCompletionRatio() float64 {
	return DefaultPricingCompletionUSDPerMillion / DefaultPricingInputUSDPerMillion
}

func defaultPricingCacheRatio() float64 {
	return DefaultPricingCacheReadUSDPerMillion / DefaultPricingInputUSDPerMillion
}
