package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func senseNovaPartialBillingFixture(t *testing.T) (*gin.Context, *relaycommon.RelayInfo, *BillingSession) {
	t.Helper()
	truncate(t)
	seedUser(t, 911, 99_500)
	seedToken(t, 911, 911, "synthetic-partial-billing-token", 99_500)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 911).Update("used_quota", 500).Error)
	seedChannel(t, 911)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set(senseNovaAttemptContext, &senseNovaAttempt{})
	info := &relaycommon.RelayInfo{
		UserId: 911, TokenId: 911, TokenKey: "synthetic-partial-billing-token", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 911},
		UserQuota: 100_000_000, OriginModelName: "deepseek-v4-pro", RelayFormat: types.RelayFormatOpenAI,
		StartTime: time.Now(), IsStream: true, FinalPreConsumedQuota: 500,
		PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
	}
	info.SetEstimatePromptTokens(9000)
	session := &BillingSession{relayInfo: info, funding: &WalletFunding{userId: 911, consumed: 500}, preConsumedQuota: 500, tokenConsumed: 500}
	info.Billing = session
	return c, info, session
}

func TestSenseNovaPartialBillingSettlesDeliveredUsageWithoutRefund(t *testing.T) {
	c, info, session := senseNovaPartialBillingFixture(t)
	_, err := c.Writer.WriteString("data: partial text\n\n")
	require.NoError(t, err)
	PostSenseNovaPartialConsumeQuota(c, info, &dto.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110})
	require.False(t, session.NeedsRefund())
	session.Refund(c) // controller keeps the stream error and runs this deferred cleanup
	userQuota, err := model.GetUserQuota(911, false)
	require.NoError(t, err)
	assert.Equal(t, 99_880, userQuota, "charge only 100 input + 10 emitted output at the existing 1:2 ratios")
	var token model.Token
	require.NoError(t, model.DB.First(&token, 911).Error)
	assert.Equal(t, 99_880, token.RemainQuota)
	assert.Equal(t, 120, token.UsedQuota)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, 120, logs[0].Quota)
	assert.Equal(t, 10, logs[0].CompletionTokens)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
	assert.Equal(t, true, other["incomplete"], "a consume record for partial work cannot imply a completed answer")
	assert.Contains(t, logs[0].Content, "partial usage")
}

func TestSenseNovaPartialBillingDoesNotChargeAbsentOrUncommittedOutput(t *testing.T) {
	for _, tc := range []struct {
		name               string
		usage              *dto.Usage
		written, senseNova bool
	}{
		{name: "nil usage", written: true, senseNova: true},
		{name: "empty usage", usage: &dto.Usage{}, written: true, senseNova: true},
		{name: "uncommitted", usage: &dto.Usage{PromptTokens: 100, CompletionTokens: 10}, senseNova: true},
		{name: "other provider", usage: &dto.Usage{PromptTokens: 100, CompletionTokens: 10}, written: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, info, session := senseNovaPartialBillingFixture(t)
			if !tc.senseNova {
				ResetSenseNovaAttempt(c)
			}
			if tc.written {
				c.Writer.WriteHeaderNow()
			}
			PostSenseNovaPartialConsumeQuota(c, info, tc.usage)
			assert.True(t, session.NeedsRefund(), "empty failures retain the controller's full-refund path")
			quota, err := model.GetUserQuota(911, false)
			require.NoError(t, err)
			assert.Equal(t, 99_500, quota)
			var count int64
			require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&count).Error)
			assert.Zero(t, count, "do not create a charged/consume record from the request's 9000-token estimate alone")
		})
	}
}
