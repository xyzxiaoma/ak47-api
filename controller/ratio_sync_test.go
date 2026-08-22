package controller

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelaxyCodeModelListURL(t *testing.T) {
	url, ok := relaxyCodeModelListURL("https://www.relaxycode.com/v1")
	require.True(t, ok)
	assert.Equal(t, "https://www.relaxycode.com/prod-api/api/v1/model/list", url)

	_, ok = relaxyCodeModelListURL("https://example.com")
	assert.False(t, ok)
}

func TestRelaxyCodePricingEndpoint(t *testing.T) {
	assert.True(t, isRelaxyCodePricingEndpoint("https://www.relaxycode.com/api/pricing"))
	assert.True(t, isRelaxyCodePricingEndpoint("https://www.relaxycode.com/zh/dashboard/pricing"))
	assert.False(t, isRelaxyCodePricingEndpoint("https://www.relaxycode.com/prod-api/api/v1/model/list"))
	assert.False(t, isRelaxyCodePricingEndpoint("https://example.com/zh/dashboard/pricing"))
}

func TestConvertRelaxyCodeModelListToRatioData(t *testing.T) {
	data, err := convertRelaxyCodeModelListToRatioData(strings.NewReader(`{
  "code": "0",
  "message": "操作成功",
  "data": [
    {
      "modelId": "gpt-5.6-terra",
      "inputPrice": 2,
      "outputPrice": 12,
      "cacheCreatePrice": 2.5,
      "cacheReadPrice": 0.2,
      "priceTiers": [
        {
          "contextThreshold": 272000,
          "inputPrice": 4,
          "outputPrice": 18,
          "cacheCreatePrice": 5,
          "cacheReadPrice": 0.4
        }
      ]
    },
    {
      "modelId": "gpt-5.5",
      "inputPrice": 5,
      "outputPrice": 30,
      "cacheCreatePrice": 0,
      "cacheReadPrice": 0.5,
      "priceTiers": null
    }
  ]
}`))
	require.NoError(t, err)

	modelRatios := valueMap(data["model_ratio"])
	completionRatios := valueMap(data["completion_ratio"])
	cacheRatios := valueMap(data["cache_ratio"])
	createCacheRatios := valueMap(data["create_cache_ratio"])
	billingModes := valueMap(data[billing_setting.BillingModeField])
	billingExprs := valueMap(data[billing_setting.BillingExprField])

	assert.Equal(t, 1.0, modelRatios["gpt-5.6-terra"])
	assert.Equal(t, 6.0, completionRatios["gpt-5.6-terra"])
	assert.Equal(t, 0.1, cacheRatios["gpt-5.6-terra"])
	assert.Equal(t, 1.25, createCacheRatios["gpt-5.6-terra"])
	assert.Equal(t, billing_setting.BillingModeTieredExpr, billingModes["gpt-5.6-terra"])
	expr := billingExprs["gpt-5.6-terra"]
	assert.Equal(t,
		`len < 272000 ? tier("base", p * 2 + c * 12 + cr * 0.2 + cc * 2.5) : tier("tier-272000", p * 4 + c * 18 + cr * 0.4 + cc * 5)`,
		expr,
	)
	_, err = billingexpr.CompileFromCache(expr.(string))
	require.NoError(t, err)
	assert.NotContains(t, billingExprs, "gpt-5.5")
}

func TestConvertRelaxyCodeModelListToRatioDataRejectsFailedResponse(t *testing.T) {
	_, err := convertRelaxyCodeModelListToRatioData(strings.NewReader(`{
  "code": "-1",
  "message": "unauthorized",
  "data": []
}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}
