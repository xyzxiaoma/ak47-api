package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func senseNovaAdmissionFixture(t *testing.T) (*model.Channel, *miniredis.Miniredis) {
	t.Helper()
	channel, _ := senseNovaServiceFixture(t)
	channel.Models = "deepseek-v4-pro,glm-5.2"
	require.NoError(t, model.DB.Save(channel).Error)
	previousRedis, previousEnabled := common.RDB, common.RedisEnabled
	server := miniredis.RunT(t)
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() { _ = common.RDB.Close(); common.RDB = previousRedis; common.RedisEnabled = previousEnabled })
	t.Setenv("SENSENOVA_ADMISSION_ENABLED", "true")
	t.Setenv("SENSENOVA_ADMISSION_MODELS", "deepseek-v4-pro")
	t.Setenv("SENSENOVA_TPM_LIMITS", "")
	t.Setenv("SENSENOVA_ADMISSION_WAIT_SECONDS", "1")
	return channel, server
}

func senseNovaAdmissionContext(t *testing.T, channel *model.Channel) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "authorized-group")
	_, _, err := SelectSenseNovaKey(c, channel, "deepseek-v4-pro")
	require.Nil(t, err)
	c.Set("sensenova_request_metrics", map[string]interface{}{"estimated_prompt_tokens": 9000, "requested_max_output_tokens": uint(128)})
	return c
}

func TestSenseNovaAdmissionSelectsBudgetEligibleKeyWithoutCountingRetry(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	first := senseNovaAdmissionContext(t, channel)
	require.Nil(t, AdmitSenseNovaAttempt(first))
	firstKey := getSenseNovaAttempt(first).key
	MarkSenseNovaBudgetDispatched(first)
	ObserveSenseNovaUsage(first, &dto.Usage{PromptTokens: 9000, CompletionTokens: 2, TotalTokens: 9002})
	RecordSenseNovaRelaySuccess(first)
	FinishSenseNovaAdmission(first)

	senseNovaOffsets.Delete(channel.Id)
	next := senseNovaAdmissionContext(t, channel)
	require.Equal(t, firstKey, getSenseNovaAttempt(next).key)
	require.Nil(t, AdmitSenseNovaAttempt(next))
	assert.NotEqual(t, firstKey, getSenseNovaAttempt(next).key)
	assert.Equal(t, 1, getSenseNovaAttempt(next).number)
	assert.Equal(t, "authorized-group", common.GetContextKeyString(next, constant.ContextKeyUsingGroup))
	assert.Equal(t, getSenseNovaAttempt(next).key, common.GetContextKeyString(next, constant.ContextKeyChannelKey))
	FinishSenseNovaAdmission(next)
}

func TestSenseNovaAdmissionReleasesUnsentButKeepsUncertainDebit(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	channel.Key = "fake-account-a"
	channel.ChannelInfo.MultiKeySize = 1
	require.NoError(t, model.DB.Save(channel).Error)
	first := senseNovaAdmissionContext(t, channel)
	require.Nil(t, AdmitSenseNovaAttempt(first))
	FinishSenseNovaAdmission(first)
	second := senseNovaAdmissionContext(t, channel)
	require.Nil(t, AdmitSenseNovaAttempt(second), "pre-dispatch failure must not consume a slot")
	MarkSenseNovaBudgetDispatched(second)
	FinishSenseNovaAdmission(second)
	third := senseNovaAdmissionContext(t, channel)
	ctx, cancel := context.WithCancel(third.Request.Context())
	third.Request = third.Request.WithContext(ctx)
	cancel()
	err := AdmitSenseNovaAttempt(third)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	FinishSenseNovaAdmission(third)
}

func TestSenseNovaAdmissionRejectsOversizedKnownBudgetWithoutCooling(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	t.Setenv("SENSENOVA_TPM_LIMITS", `{"deepseek-v4-pro":{"default":8000}}`)
	c := senseNovaAdmissionContext(t, channel)
	err := AdmitSenseNovaAttempt(c)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusTooManyRequests, err.StatusCode)
	_, stateErr := model.SenseNovaKeySnapshot(channel.Id, getSenseNovaAttempt(c).key, "deepseek-v4-pro")
	assert.NoError(t, stateErr)
	assert.False(t, ShouldRetrySenseNova(c, err))
	FinishSenseNovaAdmission(c)
}

func TestSenseNovaAdmissionRedisFailureFailsClosedAndDisabledPreservesLegacy(t *testing.T) {
	channel, server := senseNovaAdmissionFixture(t)
	c := senseNovaAdmissionContext(t, channel)
	server.Close()
	err := AdmitSenseNovaAttempt(c)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.NotContains(t, err.Error(), "fake-account")
	t.Setenv("SENSENOVA_ADMISSION_ENABLED", "false")
	c = senseNovaAdmissionContext(t, channel)
	require.Nil(t, AdmitSenseNovaAttempt(c))
	FinishSenseNovaAdmission(c)
}

func TestSenseNovaAdmissionHealthProbeCannotRestoreSpentCapacity(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	channel.Key = "fake-account-a"
	channel.ChannelInfo.MultiKeySize = 1
	require.NoError(t, model.DB.Save(channel).Error)
	first := senseNovaAdmissionContext(t, channel)
	require.Nil(t, AdmitSenseNovaAttempt(first))
	MarkSenseNovaBudgetDispatched(first)
	RecordSenseNovaRelaySuccess(first)
	FinishSenseNovaAdmission(first)
	second := senseNovaAdmissionContext(t, channel)
	RecordSenseNovaRelaySuccess(second) // health-only success, no budget reservation
	ctx, cancel := context.WithTimeout(second.Request.Context(), 40*time.Millisecond)
	defer cancel()
	second.Request = second.Request.WithContext(ctx)
	err := AdmitSenseNovaAttempt(second)
	require.NotNil(t, err, "health state must not clear admission pacing")
	FinishSenseNovaAdmission(second)
}

func TestSenseNovaAdmissionWaitsForCoolingKeyAndHonorsCancellation(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	first := senseNovaAdmissionContext(t, channel)
	RecordSenseNovaRelayFailure(first, types.WithOpenAIError(types.OpenAIError{Code: "429001"}, 429))
	second := senseNovaAdmissionContext(t, channel)
	RecordSenseNovaRelayFailure(second, types.WithOpenAIError(types.OpenAIError{Code: "429001"}, 429))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	started := time.Now()
	_, _, err := SelectSenseNovaKey(c, channel, "deepseek-v4-pro")
	require.NotNil(t, err)
	assert.GreaterOrEqual(t, time.Since(started), 30*time.Millisecond)
	assert.Less(t, time.Since(started), time.Second)
	FinishSenseNovaAdmission(c)
}

func TestSenseNovaAdmissionCleanupPreservesLeaseLossCancellation(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	c := senseNovaAdmissionContext(t, channel)
	require.Nil(t, AdmitSenseNovaAttempt(c))
	MarkSenseNovaBudgetDispatched(c)
	state := currentSenseNovaAdmission(c)
	require.NoError(t, state.parentContext.Err())
	state.cancelRequest() // The renewal worker cancels this child on lease loss.
	failure := types.WithOpenAIError(types.OpenAIError{Code: "429001"}, 429)
	RecordSenseNovaRelayFailure(c, failure)
	assert.ErrorIs(t, c.Request.Context().Err(), context.Canceled)
	assert.False(t, ShouldRetrySenseNova(c, failure), "lease loss must terminate this request")
	FinishSenseNovaAdmission(c)
}

func TestSenseNovaAdmissionConfigSafetyBounds(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"SENSENOVA_UNKNOWN_TPM_INTERVAL_SECONDS", "59"},
		{"SENSENOVA_ADMISSION_QUEUE_LIMIT", "33"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SENSENOVA_ADMISSION_ENABLED", "true")
			t.Setenv("SENSENOVA_ADMISSION_MODELS", "deepseek-v4-pro")
			t.Setenv(tc.name, tc.value)
			_, err := senseNovaAdmissionConfigFor("deepseek-v4-pro")
			assert.Error(t, err, "operator overrides must preserve the approved safety bounds")
		})
	}
}

func TestSenseNovaAdmissionOnlyMeasuredUsageCanRefundBudget(t *testing.T) {
	for _, localEstimate := range []bool{false, true} {
		t.Run(map[bool]string{false: "measured", true: "estimated"}[localEstimate], func(t *testing.T) {
			channel, _ := senseNovaAdmissionFixture(t)
			t.Setenv("SENSENOVA_TPM_LIMITS", `{"deepseek-v4-pro":{"default":100}}`)
			c := senseNovaAdmissionContext(t, channel)
			c.Set("sensenova_request_metrics", map[string]interface{}{"estimated_prompt_tokens": 60, "requested_max_output_tokens": uint(20)})
			require.Nil(t, AdmitSenseNovaAttempt(c))
			MarkSenseNovaBudgetDispatched(c)
			common.SetContextKey(c, constant.ContextKeyLocalCountTokens, localEstimate)
			ObserveSenseNovaUsage(c, &dto.Usage{PromptTokens: 60, CompletionTokens: 1, TotalTokens: 61})
			RecordSenseNovaRelaySuccess(c)
			defer FinishSenseNovaAdmission(c)
			attempt := getSenseNovaAttempt(c)
			request := senseNovaBudgetRequest{ChannelID: channel.Id, Fingerprint: model.SenseNovaFingerprint(attempt.key), Model: attempt.model, ID: "check-remainder", PromptTokens: 21, HasOutputLimit: true}
			state := currentSenseNovaAdmission(c)
			next, _, err := reserveSenseNovaBudget(context.Background(), request, state.config.policy(request.Fingerprint))
			require.NoError(t, err)
			if localEstimate {
				assert.Nil(t, next, "locally estimated output must retain the 80-token reservation")
			} else {
				assert.NotNil(t, next, "measured 61-token usage must release the unused allowance")
			}
		})
	}
}
