package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestCalculateAudioQuotaUsesOutputModelGroupRatio(t *testing.T) {
	original := ratio_setting.CompletionRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(original))
	})
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"item-ratio-test":2}`))

	quota, clamp := calculateAudioQuota(QuotaInfo{
		InputDetails:     TokenDetails{TextTokens: 100},
		OutputDetails:    TokenDetails{TextTokens: 100},
		ModelName:        "item-ratio-test",
		ModelRatio:       1,
		GroupRatio:       0.3,
		OutputGroupRatio: common.GetPointer(0.4),
	})

	require.Nil(t, clamp)
	require.Equal(t, 110, quota)
}
