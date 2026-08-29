package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelGroupRatioOverrideAndFallback(t *testing.T) {
	original := ModelGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateModelGroupRatioByJSONString(original))
	})

	require.NoError(t, UpdateModelGroupRatioByJSONString(`{"glm-5.2":0.28}`))
	require.Equal(t, 0.28, GetEffectiveGroupRatio("glm-5.2", 0.4))
	require.Equal(t, 0.4, GetEffectiveGroupRatio("glm-5.3", 0.4))
}

func TestCheckModelGroupRatioRejectsInvalidValues(t *testing.T) {
	require.Error(t, CheckModelGroupRatio(`{"glm-5.2":-0.1}`))
	require.Error(t, CheckModelGroupRatio(`{"glm-5.2":NaN}`))
	require.NoError(t, CheckModelGroupRatio(`{"glm-5.2":0,"glm-5.3":0.4}`))
}

func TestEffectiveModelPricingGroupRatiosUseItemOverridesAndFallbacks(t *testing.T) {
	originalInput := ModelGroupRatio2JSONString()
	originalCompletion := ModelCompletionGroupRatio2JSONString()
	originalCache := ModelCacheGroupRatio2JSONString()
	originalCreateCache := ModelCreateCacheGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateModelGroupRatioByJSONString(originalInput))
		require.NoError(t, UpdateModelCompletionGroupRatioByJSONString(originalCompletion))
		require.NoError(t, UpdateModelCacheGroupRatioByJSONString(originalCache))
		require.NoError(t, UpdateModelCreateCacheGroupRatioByJSONString(originalCreateCache))
	})

	require.NoError(t, UpdateModelGroupRatioByJSONString(`{"deepseek-v4-pro":0.3}`))
	require.NoError(t, UpdateModelCompletionGroupRatioByJSONString(`{"deepseek-v4-pro":0.4}`))
	require.NoError(t, UpdateModelCacheGroupRatioByJSONString(`{"deepseek-v4-pro":0.7}`))
	require.NoError(t, UpdateModelCreateCacheGroupRatioByJSONString(`{}`))

	ratios := GetEffectiveModelPricingGroupRatios("deepseek-v4-pro", 0.2)
	require.Equal(t, 0.3, ratios.Input)
	require.Equal(t, 0.4, ratios.Completion)
	require.Equal(t, 0.7, ratios.Cache)
	require.Equal(t, 0.3, ratios.CreateCache)

	fallback := GetEffectiveModelPricingGroupRatios("deepseek-v4-flash", 0.2)
	require.Equal(t, ModelPricingGroupRatios{Input: 0.2, Completion: 0.2, Cache: 0.2, CreateCache: 0.2}, fallback)
}

func TestItemModelGroupRatiosRejectInvalidValues(t *testing.T) {
	require.Error(t, UpdateModelCompletionGroupRatioByJSONString(`{"model":-0.1}`))
	require.Error(t, UpdateModelCacheGroupRatioByJSONString(`{"model":NaN}`))
	require.Error(t, UpdateModelCreateCacheGroupRatioByJSONString(`{"model":-1}`))
}
