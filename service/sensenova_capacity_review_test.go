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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSenseNovaCapacityReviewInitialSelectionRejectsLongCooldown(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	for _, key := range channel.GetKeys() {
		snapshot, err := model.SenseNovaKeySnapshot(channel.Id, key, "deepseek-v4-pro")
		require.NoError(t, err)
		_, err = model.RecordSenseNovaFailure(snapshot, key, "deepseek-v4-pro", "rate_limited", false, 300, time.Now().Unix())
		require.NoError(t, err)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	defer FinishSenseNovaAdmission(c)
	_, _, err := SelectSenseNovaKey(c, channel, "deepseek-v4-pro")
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.NoError(t, ctx.Err(), "an impossible fixed cooldown should fail before waiting for client cancellation")
	assert.GreaterOrEqual(t, c.GetInt64("sensenova_admission_retry_after"), int64(299))
}

func TestSenseNovaCapacityReviewFinishedPacingRejectsImpossibleWait(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	t.Setenv("SENSENOVA_UNKNOWN_TPM_INTERVAL_SECONDS", "300")
	for range channel.GetKeys() {
		c := senseNovaLargeAdmissionContext(t, channel)
		require.Nil(t, AdmitSenseNovaAttempt(c))
		MarkSenseNovaBudgetDispatched(c)
		RecordSenseNovaRelaySuccess(c)
		FinishSenseNovaAdmission(c)
	}
	c := senseNovaLargeAdmissionContext(t, channel)
	defer FinishSenseNovaAdmission(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 100*time.Millisecond)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	err := AdmitSenseNovaAttempt(c)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.NoError(t, ctx.Err(), "completed owners cannot free a fixed pacing timer early")
}

func TestSenseNovaCapacityReviewCombinesHealthAndShapeCooldown(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	t.Setenv("SENSENOVA_ADMISSION_WAIT_SECONDS", "75")
	for _, key := range channel.GetKeys() {
		seedSenseNovaCapacityRouting(t, channel, key, false)
		seedSenseNovaCapacityRouting(t, channel, key, false)
		snapshot, err := model.SenseNovaKeySnapshot(channel.Id, key, "deepseek-v4-pro")
		require.NoError(t, err)
		_, err = model.RecordSenseNovaFailure(snapshot, key, "deepseek-v4-pro", "rate_limited", false, 60, time.Now().Unix())
		require.NoError(t, err)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	defer FinishSenseNovaAdmission(c)
	_, _, selectionErr := SelectSenseNovaKey(c, channel, "deepseek-v4-pro")
	require.Nil(t, selectionErr, "defer a potentially recoverable health wait until request metrics exist")
	assert.Nil(t, currentSenseNovaAdmission(c).reservation, "initial nomination cannot reserve or authorize dispatch")
	c.Set("sensenova_request_metrics", map[string]interface{}{"estimated_prompt_tokens": 28935, "requested_max_tokens": uint(32000)})
	err := AdmitSenseNovaAttempt(c)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.NoError(t, ctx.Err(), "the combined 120-second penalty cannot fit the 75-second wait budget")
	assert.GreaterOrEqual(t, c.GetInt64("sensenova_admission_retry_after"), int64(119))
}

func TestSenseNovaCapacityReviewPendingNominationPreservesHealthAndAdminGates(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	t.Setenv("SENSENOVA_ADMISSION_WAIT_SECONDS", "75")
	for i, key := range channel.GetKeys() {
		snapshot, err := model.SenseNovaKeySnapshot(channel.Id, key, "deepseek-v4-pro")
		require.NoError(t, err)
		reason, scope := "rate_limited", "deepseek-v4-pro"
		if i == 0 {
			reason, scope = "authentication_failed", ""
		}
		_, err = model.RecordSenseNovaFailure(snapshot, key, scope, reason, i == 0, 60, time.Now().Unix())
		require.NoError(t, err)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	defer FinishSenseNovaAdmission(c)
	key, index, err := SelectSenseNovaKey(c, channel, "deepseek-v4-pro")
	require.Nil(t, err)
	assert.Equal(t, channel.GetKeys()[1], key, "invalid identities cannot be nominated")
	assert.Equal(t, 1, index)
	_, claimErr := model.ClaimSenseNovaRecovery(getSenseNovaAttempt(c).snapshot, key, time.Now().Unix())
	assert.ErrorIs(t, claimErr, model.ErrSenseNovaUnavailable, "a nomination is not a health generation or recovery owner")
	channel.ChannelInfo.MultiKeyStatusList = map[int]int{1: common.ChannelStatusManuallyDisabled}
	require.NoError(t, model.DB.Save(channel).Error)
	c.Set("sensenova_request_metrics", map[string]interface{}{"estimated_prompt_tokens": 28935, "requested_max_tokens": uint(32000)})
	require.NotNil(t, AdmitSenseNovaAttempt(c), "a subsequent administrative disable must prevent admission")
	assert.Nil(t, currentSenseNovaAdmission(c).reservation)
	_, _, err = SelectSenseNovaKey(c, channel, "deepseek-v4-pro")
	require.NotNil(t, err, "disabled and invalid keys cannot be nominated")
}

func TestSenseNovaCapacityReviewDisabledAdmissionKeepsCoolingSelectionUnavailable(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	t.Setenv("SENSENOVA_ADMISSION_ENABLED", "false")
	for _, key := range channel.GetKeys() {
		snapshot, err := model.SenseNovaKeySnapshot(channel.Id, key, "deepseek-v4-pro")
		require.NoError(t, err)
		_, err = model.RecordSenseNovaFailure(snapshot, key, "deepseek-v4-pro", "rate_limited", false, 60, time.Now().Unix())
		require.NoError(t, err)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	_, _, err := SelectSenseNovaKey(c, channel, "deepseek-v4-pro")
	require.NotNil(t, err)
	assert.Nil(t, getSenseNovaAttempt(c))
}
