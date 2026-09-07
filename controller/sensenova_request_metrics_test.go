package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSenseNovaRequestMetricsOnlyRetainNumbersAndPreserveAbsence(t *testing.T) {
	for _, tt := range []struct {
		name    string
		request dto.Request
		want    map[string]interface{}
	}{
		{"absent", &dto.GeneralOpenAIRequest{Prompt: "private prompt"}, map[string]interface{}{"estimated_prompt_tokens": 33}},
		{"explicit zero", &dto.GeneralOpenAIRequest{MaxTokens: common.GetPointer(uint(0))}, map[string]interface{}{"estimated_prompt_tokens": 33, "requested_max_tokens": uint(0)}},
		{"openai ceilings", &dto.GeneralOpenAIRequest{MaxTokens: common.GetPointer(uint(128)), MaxCompletionTokens: common.GetPointer(uint(256)), Prompt: "private prompt"}, map[string]interface{}{"estimated_prompt_tokens": 33, "requested_max_tokens": uint(128), "requested_max_completion_tokens": uint(256)}},
		{"claude", &dto.ClaudeRequest{MaxTokens: common.GetPointer(uint(128)), System: "private system"}, map[string]interface{}{"estimated_prompt_tokens": 33, "requested_max_tokens": uint(128)}},
		{"claude legacy", &dto.ClaudeRequest{MaxTokensToSample: common.GetPointer(uint(96))}, map[string]interface{}{"estimated_prompt_tokens": 33, "requested_max_tokens_to_sample": uint(96)}},
		{"responses", &dto.OpenAIResponsesRequest{MaxOutputTokens: common.GetPointer(uint(128))}, map[string]interface{}{"estimated_prompt_tokens": 33, "requested_max_output_tokens": uint(128)}},
		{"gemini", &dto.GeminiChatRequest{GenerationConfig: dto.GeminiChatGenerationConfig{MaxOutputTokens: common.GetPointer(uint(128))}}, map[string]interface{}{"estimated_prompt_tokens": 33, "requested_max_output_tokens": uint(128)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, senseNovaRequestMetrics(tt.request, 33))
		})
	}
}

func TestSenseNovaErrorLogKeepsSafeDiagnosticsAdminOnly(t *testing.T) {
	channel := setupSenseNovaController(t)
	base := "https://token.sensenova.cn"
	channel.BaseURL = &base
	require.NoError(t, model.DB.Save(channel).Error)
	previous := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = previous })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("id", 1)
	c.Set("username", "fixture-user")
	c.Set("token_id", 7)
	c.Set(common.RequestIdKey, "fixture-request")
	c.Set("sensenova_request_metrics", senseNovaRequestMetrics(&dto.GeneralOpenAIRequest{
		MaxCompletionTokens: common.GetPointer(uint(4096)), Prompt: "private prompt must not be logged",
	}, 1234))
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "glm-5.2"))
	service.MarkSenseNovaLatencyDispatch(c)()
	key := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	body, err := common.Marshal(map[string]interface{}{"error": map[string]interface{}{
		"code": "ModelAccountTpmRateLimitExceeded", "message": "private upstream message " + key,
	}})
	require.NoError(t, err)
	upstream := service.RelayErrorHandler(c.Request.Context(), &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{"120"}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}, false)
	safe := service.RecordSenseNovaRelayFailure(c, upstream)
	service.FinishSenseNovaLatencyAttempt(c, nil, true)
	processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, true, key, false), safe)
	var entry model.Log
	require.NoError(t, model.LOG_DB.First(&entry).Error)
	var other struct {
		AdminInfo struct {
			SenseNova map[string]interface{}       `json:"sensenova"`
			Latency   *service.SenseNovaLatencyLog `json:"sensenova_latency"`
		} `json:"admin_info"`
	}
	require.NoError(t, common.UnmarshalJsonStr(entry.Other, &other))
	require.NotNil(t, other.AdminInfo.Latency)
	require.Len(t, other.AdminInfo.Latency.Attempts, 1)
	assert.NotNil(t, other.AdminInfo.Latency.Attempts[0].HeadersMS)
	assert.NotNil(t, other.AdminInfo.Latency.Attempts[0].FinishMS)
	assert.Nil(t, other.AdminInfo.Latency.Attempts[0].FirstSemanticMS)
	assert.Equal(t, "error", other.AdminInfo.Latency.Attempts[0].Outcome)
	details := other.AdminInfo.SenseNova
	require.NotNil(t, details)
	assert.Equal(t, "tpm", details["limit_kind"])
	assert.Equal(t, float64(1), details["attempt"])
	assert.Equal(t, float64(4), details["max_attempts"])
	assert.Equal(t, float64(120), details["retry_after_seconds"])
	assert.Equal(t, float64(1234), details["estimated_prompt_tokens"])
	assert.Equal(t, float64(4096), details["requested_max_completion_tokens"])
	assert.Equal(t, "fixture-request", entry.RequestId)
	for _, secret := range []string{key, "private prompt", "private upstream message"} {
		assert.NotContains(t, entry.Content+entry.Other, secret)
	}
	publicLogs, err := model.GetLogByTokenId(7)
	require.NoError(t, err)
	require.Len(t, publicLogs, 1)
	assert.NotContains(t, publicLogs[0].Other, "admin_info")
	assert.NotContains(t, publicLogs[0].Other, "key_id")
	assert.NotContains(t, publicLogs[0].Other, "sensenova_latency")
}
