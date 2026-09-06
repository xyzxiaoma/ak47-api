package controller

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSenseNovaRetryDoesNotReplayWrittenOrCancelledResponses(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{Message: "busy"}, 503)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Writer.WriteHeaderNow()
	assert.False(t, shouldRetry(c, err, 3))
	c, _ = gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(ctx)
	assert.False(t, shouldRetry(c, err, 3))
	assert.False(t, shouldRetry(c, err, 0))
}

func TestSenseNovaControllerFailoverWithLegacyRetriesDisabled(t *testing.T) {
	channel := setupSenseNovaController(t)
	base := "https://token.sensenova.cn"
	channel.BaseURL = &base
	require.NoError(t, model.DB.Model(channel).Update("base_url", base).Error)
	previous := common.RetryTimes
	common.RetryTimes = 0
	t.Cleanup(func() { common.RetryTimes = previous })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "paid-fixture-group")
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "glm-5.2"))
	assert.Equal(t, 2, relayRetryBudget(c))
	first := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	info := &relaycommon.RelayInfo{OriginModelName: "glm-5.2"}
	_, initialErr := getChannel(c, info, &service.RetryParam{Ctx: c})
	require.Nil(t, initialErr)
	assert.Equal(t, first, common.GetContextKeyString(c, constant.ContextKeyChannelKey), "first relay attempt reuses distribution selection")
	safe := service.RecordSenseNovaRelayFailure(c, types.WithOpenAIError(types.OpenAIError{Code: "insufficient_quota", Message: first}, 400))
	assert.NotContains(t, safe.Error(), first)
	require.True(t, shouldRetry(c, safe, 2), "retry decision survives sanitization of a code-only HTTP400 quota error")
	selected, err := getChannel(c, info, &service.RetryParam{Ctx: c, ModelName: "glm-5.2", TokenGroup: "paid-fixture-group", Retry: common.GetPointer(1)})
	require.Nil(t, err)
	assert.Equal(t, channel.Id, selected.Id)
	assert.NotEqual(t, first, common.GetContextKeyString(c, constant.ContextKeyChannelKey))
	assert.Equal(t, "paid-fixture-group", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	service.RecordSenseNovaRelaySuccess(c)
	_, snapshotErr := model.SenseNovaKeySnapshot(channel.Id, first, "kimi-k3")
	assert.ErrorIs(t, snapshotErr, model.ErrSenseNovaUnavailable)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now().Add(-91*time.Second))
	safe = service.RecordSenseNovaRelayFailure(c, types.WithOpenAIError(types.OpenAIError{Message: "busy"}, 429))
	assert.False(t, shouldRetry(c, safe, 2))
}

func TestSenseNovaImportPreservesManualIdentity(t *testing.T) {
	origin := &model.Channel{Key: "fixture-a\nfixture-b", ChannelInfo: model.ChannelInfo{MultiKeyStatusList: map[int]int{0: 2}}}
	next := &model.Channel{Key: "fixture-b\n fixture-a \nfixture-b\nfixture-c"}
	require.NoError(t, prepareSenseNovaKeys(next, origin))
	assert.Equal(t, "fixture-b\nfixture-a\nfixture-c", next.Key)
	assert.Equal(t, map[int]int{1: 2}, next.ChannelInfo.MultiKeyStatusList)
	assert.Equal(t, 3, next.ChannelInfo.MultiKeySize)
	assert.True(t, next.ChannelInfo.IsMultiKey)
}
