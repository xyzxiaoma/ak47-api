package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertUpstreamPricingCatalogUsesPeakOriginalPricesAndPerItemMarkups(t *testing.T) {
	var catalog upstreamPricingCatalog
	require.NoError(t, json.Unmarshal([]byte(`{
		"items": [{
			"model_id": "deepseek-v4-pro",
			"series": "DeepSeek",
			"is_active": true,
			"prices": {
				"input": {"original": 9, "platform": 1.8},
				"output": {"original": 27, "platform": 5.4},
				"cache_hit": {"original": 0.3, "platform": 0.18},
				"cache_creation": {"original": 0.6, "platform": 0.12},
				"time_of_day_enabled": true,
				"off_peak": {
					"input": {"original": 4.5, "platform": 0.9},
					"output": {"original": 13.5, "platform": 2.7},
					"cache_hit": {"original": 0.15, "platform": 0.09},
					"cache_creation": {"original": 0.3, "platform": 0.06}
				}
			}
		}]
	}`), &catalog))

	data, skipped, err := convertUpstreamPricingCatalog(catalog)
	require.NoError(t, err)
	assert.Equal(t, 0, skipped)
	assert.Equal(t, 4.5, valueMap(data["model_ratio"])["deepseek-v4-pro"])
	assert.Equal(t, 3.0, valueMap(data["completion_ratio"])["deepseek-v4-pro"])
	assert.Equal(t, 0.03333333333333333, valueMap(data["cache_ratio"])["deepseek-v4-pro"])
	assert.Equal(t, 0.06666666666666667, valueMap(data["create_cache_ratio"])["deepseek-v4-pro"])
	assert.Equal(t, 0.3, valueMap(data["model_group_ratio"])["deepseek-v4-pro"])
	assert.Equal(t, 0.3, valueMap(data["model_completion_group_ratio"])["deepseek-v4-pro"])
	assert.Equal(t, 0.7, valueMap(data["model_cache_group_ratio"])["deepseek-v4-pro"])
	assert.Equal(t, 0.3, valueMap(data["model_create_cache_group_ratio"])["deepseek-v4-pro"])
}

func TestConvertUpstreamPricingCatalogSkipsPublicAndInvalidItems(t *testing.T) {
	catalog := upstreamPricingCatalog{Items: []upstreamPricingItem{
		{ModelID: "free/ds-v4-flash", Series: "DeepSeek", Active: true},
		{ModelID: "qwen3.7-flash", Series: "Qwen", Active: true},
		{ModelID: "unknown", Series: "Other", Active: true},
	}}
	catalog.Items[1].Prices.Input.Original = 0
	catalog.Items[1].Prices.Output.Original = 0
	_, skipped, err := convertUpstreamPricingCatalog(catalog)
	require.Error(t, err)
	assert.Equal(t, 1, skipped)
}

func TestConvertUpstreamPricingCatalogKeepsDeepSeekCacheDiscountsIndependent(t *testing.T) {
	buildItem := func(modelName string, cachePlatform float64) upstreamPricingItem {
		item := upstreamPricingItem{ModelID: modelName, Series: "DeepSeek", Active: true}
		item.Prices.Input.Original = 3
		item.Prices.Input.Platform = 0.6
		item.Prices.Output.Original = 9
		item.Prices.Output.Platform = 1.8
		item.Prices.CacheHit.Original = 0.1
		item.Prices.CacheHit.Platform = cachePlatform
		item.Prices.TimeOfDayEnabled = true
		item.Prices.OffPeak.Input.Original = 1.5
		item.Prices.OffPeak.Output.Original = 4.5
		item.Prices.OffPeak.CacheHit.Original = 0.05
		return item
	}
	catalog := upstreamPricingCatalog{Items: []upstreamPricingItem{
		buildItem("deepseek-v4-flash-0731", 0.06),
		buildItem("deepseek-v4-flash", 0.02),
	}}

	data, skipped, err := convertUpstreamPricingCatalog(catalog)
	require.NoError(t, err)
	require.Equal(t, 0, skipped)
	require.Equal(t, 1.5, valueMap(data["model_ratio"])["deepseek-v4-flash-0731"])
	require.Equal(t, 1.5, valueMap(data["model_ratio"])["deepseek-v4-flash"])
	require.Equal(t, 0.7, valueMap(data["model_cache_group_ratio"])["deepseek-v4-flash-0731"])
	require.Equal(t, 0.3, valueMap(data["model_cache_group_ratio"])["deepseek-v4-flash"])
}

func TestConvertUpstreamPricingCatalogPreservesExistingGPTDiscounts(t *testing.T) {
	item := upstreamPricingItem{ModelID: "gpt-5.6-sol", Series: "GPT", Active: true}
	item.Prices.Input.Original = 10
	item.Prices.Input.Platform = 1
	item.Prices.Output.Original = 20
	item.Prices.Output.Platform = 2

	data, _, err := convertUpstreamPricingCatalog(upstreamPricingCatalog{Items: []upstreamPricingItem{item}})
	require.NoError(t, err)
	require.Equal(t, 5.0, valueMap(data["model_ratio"])["gpt-5.6-sol"])
	_, hasDiscountUpdate := valueMap(data["model_group_ratio"])["gpt-5.6-sol"]
	require.False(t, hasDiscountUpdate)
}

func TestFetchUpstreamPricingCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"model_id":"glm-5.2","series":"GLM","is_active":true,"prices":{"input":{"original":8},"output":{"original":28}}}]}`))
	}))
	t.Cleanup(server.Close)
	catalog, err := fetchUpstreamPricingCatalog(t.Context(), server.URL)
	require.NoError(t, err)
	require.Len(t, catalog.Items, 1)
	assert.Equal(t, "glm-5.2", strings.TrimSpace(catalog.Items[0].ModelID))
}
