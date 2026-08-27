package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertUpstreamPricingCatalogUsesOffPeakOriginalPrices(t *testing.T) {
	catalog := upstreamPricingCatalog{}
	catalog.Items = []upstreamPricingItem{
		{
			ModelID: "deepseek-v4-pro",
			Series:  "DeepSeek",
			Active:  true,
		},
	}
	catalog.Items[0].Prices.Input.Original = 9
	catalog.Items[0].Prices.Output.Original = 27
	catalog.Items[0].Prices.CacheHit.Original = 0.3
	catalog.Items[0].Prices.TimeOfDayEnabled = true
	catalog.Items[0].Prices.OffPeak.Input.Original = 4.5
	catalog.Items[0].Prices.OffPeak.Output.Original = 13.5
	catalog.Items[0].Prices.OffPeak.CacheHit.Original = 0.15

	data, skipped, err := convertUpstreamPricingCatalog(catalog)
	require.NoError(t, err)
	assert.Equal(t, 0, skipped)
	assert.Equal(t, 2.25, valueMap(data["model_ratio"])["deepseek-v4-pro"])
	assert.Equal(t, 3.0, valueMap(data["completion_ratio"])["deepseek-v4-pro"])
	assert.Equal(t, 0.03333333333333333, valueMap(data["cache_ratio"])["deepseek-v4-pro"])
}

func TestConvertUpstreamPricingCatalogSkipsPublicAndInvalidItems(t *testing.T) {
	catalog := upstreamPricingCatalog{Items: []upstreamPricingItem{
		{ModelID: "free/ds-v4-flash", Series: "DeepSeek", Category: "public", Active: true},
		{ModelID: "qwen3.7-flash", Series: "Qwen", Active: true},
		{ModelID: "unknown", Series: "Other", Active: true},
	}}
	catalog.Items[1].Prices.Input.Original = 0
	catalog.Items[1].Prices.Output.Original = 0
	_, skipped, err := convertUpstreamPricingCatalog(catalog)
	require.Error(t, err)
	assert.Equal(t, 1, skipped)
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
