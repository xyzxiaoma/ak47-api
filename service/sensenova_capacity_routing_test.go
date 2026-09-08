package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func senseNovaLargeAdmissionContext(t *testing.T, channel *model.Channel) *gin.Context {
	t.Helper()
	c := senseNovaAdmissionContext(t, channel)
	c.Set("sensenova_request_metrics", map[string]interface{}{"estimated_prompt_tokens": 28935, "requested_max_tokens": uint(32000)})
	return c
}

func seedSenseNovaCapacityRouting(t *testing.T, channel *model.Channel, key string, success bool) {
	t.Helper()
	request := senseNovaBudgetRequest{ChannelID: channel.Id, Fingerprint: model.SenseNovaFingerprint(key), Model: "deepseek-v4-pro", ID: fmt.Sprint(t.Name(), time.Now().UnixNano()), PromptTokens: 28935, OutputTokens: 32000, HasOutputLimit: true}
	policy := senseNovaTestBudgetPolicy()
	policy.TokensPerMinute = 1_000_000
	reservation, _, err := reserveSenseNovaBudget(context.Background(), request, policy)
	require.NoError(t, err)
	require.NotNil(t, reservation)
	require.NoError(t, recordSenseNovaCapacity(context.Background(), reservation, request, success, !success, 0))
	require.NoError(t, finishSenseNovaBudget(context.Background(), reservation, 30000, false))
}

func seedSenseNovaDueRecoveries(t *testing.T, channel *model.Channel) {
	t.Helper()
	for _, key := range channel.GetKeys() {
		snapshot, err := model.SenseNovaKeySnapshot(channel.Id, key, "deepseek-v4-pro")
		require.NoError(t, err)
		applied, err := model.RecordSenseNovaFailure(snapshot, key, "deepseek-v4-pro", "rate_limited", false, 0, time.Now().Unix()-61)
		require.NoError(t, err)
		require.True(t, applied)
	}
}

func TestSenseNovaCapacityRoutingPrefersOrdinaryKeyOverThreeDueRecoveries(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	channel.Key = "fake-account-a\nfake-account-b\nfake-account-c\nfake-account-d"
	channel.ChannelInfo.MultiKeySize = 4
	require.NoError(t, model.DB.Save(channel).Error)
	for _, key := range channel.GetKeys()[:3] {
		snapshot, err := model.SenseNovaKeySnapshot(channel.Id, key, "deepseek-v4-pro")
		require.NoError(t, err)
		applied, err := model.RecordSenseNovaFailure(snapshot, key, "deepseek-v4-pro", "rate_limited", false, 0, time.Now().Unix()-61)
		require.NoError(t, err)
		require.True(t, applied)
	}

	c := senseNovaAdmissionContext(t, channel)
	c.Set("sensenova_request_metrics", map[string]interface{}{"estimated_prompt_tokens": 28935, "requested_max_tokens": uint(32000)})
	defer FinishSenseNovaAdmission(c)
	require.Equal(t, "fake-account-a", getSenseNovaAttempt(c).key, "initial selection still rotates before request metrics")
	require.Nil(t, AdmitSenseNovaAttempt(c))
	assert.Equal(t, "fake-account-d", getSenseNovaAttempt(c).key, "do not use the customer's large request to test three known-failing keys first")
	assert.Equal(t, 1, getSenseNovaAttempt(c).number, "admission reselection is not another upstream attempt")
	assert.Equal(t, "authorized-group", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
}

func TestSenseNovaCapacityRoutingDoesNotBurnDeadlineOnLongCooldown(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	c := senseNovaAdmissionContext(t, channel)
	for _, key := range channel.GetKeys() {
		snapshot, err := model.SenseNovaKeySnapshot(channel.Id, key, "deepseek-v4-pro")
		require.NoError(t, err)
		_, err = model.RecordSenseNovaFailure(snapshot, key, "deepseek-v4-pro", "rate_limited", false, 300, time.Now().Unix())
		require.NoError(t, err)
	}
	// The request was selected before a concurrent failure published a cooldown.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 600*time.Millisecond)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	started := time.Now()
	err := AdmitSenseNovaAttempt(c)
	defer FinishSenseNovaAdmission(c)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.Less(t, time.Since(started), 300*time.Millisecond, "a known five-minute wait cannot fit the one-second admission deadline")
	assert.NotContains(t, err.Error(), "fake-account")
}

func TestSenseNovaCapacityRoutingPrefersVerifiedSizeOverFreshRotation(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	seedSenseNovaCapacityRouting(t, channel, "fake-account-b", true)
	c := senseNovaLargeAdmissionContext(t, channel)
	defer FinishSenseNovaAdmission(c)
	require.Equal(t, "fake-account-a", getSenseNovaAttempt(c).key)
	require.Nil(t, AdmitSenseNovaAttempt(c))
	assert.Equal(t, "fake-account-b", getSenseNovaAttempt(c).key)
	assert.Empty(t, currentSenseNovaAdmission(c).capacityRecoveryOwner)
}

func TestSenseNovaCapacityRoutingAddedFreshKeyServesWhenVerifiedCapacityBusy(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	channel.Key = "fake-account-a"
	channel.ChannelInfo.MultiKeySize = 1
	require.NoError(t, model.DB.Save(channel).Error)
	first := senseNovaLargeAdmissionContext(t, channel)
	require.Nil(t, AdmitSenseNovaAttempt(first))
	MarkSenseNovaBudgetDispatched(first)
	ObserveSenseNovaUsage(first, &dto.Usage{PromptTokens: 27354, CompletionTokens: 2403, TotalTokens: 29757})
	RecordSenseNovaRelaySuccess(first)
	FinishSenseNovaAdmission(first)

	channel.Key += "\nfake-new-account"
	channel.ChannelInfo.MultiKeySize = 2
	require.NoError(t, model.DB.Save(channel).Error)
	senseNovaOffsets.Delete(channel.Id)
	next := senseNovaLargeAdmissionContext(t, channel)
	defer FinishSenseNovaAdmission(next)
	require.Nil(t, AdmitSenseNovaAttempt(next))
	assert.Equal(t, "fake-new-account", getSenseNovaAttempt(next).key, "new independent capacity must participate while the successful key is paced")
	assert.Equal(t, 1, getSenseNovaAttempt(next).number)
}

func TestSenseNovaCapacityRoutingRotatesAllFreshKeysBeyondAttemptBound(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	channel.Key = "fake-a\nfake-b\nfake-c\nfake-d\nfake-e\nfake-f"
	channel.ChannelInfo.MultiKeySize = 6
	require.NoError(t, model.DB.Save(channel).Error)
	seen := make(map[string]int)
	for range 12 {
		c := senseNovaLargeAdmissionContext(t, channel)
		require.Nil(t, AdmitSenseNovaAttempt(c))
		seen[getSenseNovaAttempt(c).key]++
		FinishSenseNovaAdmission(c) // no dispatch; leave all capacities equal
	}
	require.Len(t, seen, 6)
	for _, count := range seen {
		assert.Equal(t, 2, count)
	}
}

func TestSenseNovaCapacityRoutingPenaltyIsScopedToRequestSize(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	seedSenseNovaCapacityRouting(t, channel, "fake-account-a", false)
	large := senseNovaLargeAdmissionContext(t, channel)
	require.Nil(t, AdmitSenseNovaAttempt(large))
	assert.Equal(t, "fake-account-b", getSenseNovaAttempt(large).key)
	FinishSenseNovaAdmission(large)

	senseNovaOffsets.Delete(channel.Id)
	small := senseNovaAdmissionContext(t, channel) // 9000 input / 128 output
	defer FinishSenseNovaAdmission(small)
	require.Nil(t, AdmitSenseNovaAttempt(small))
	assert.Equal(t, "fake-account-a", getSenseNovaAttempt(small).key, "shape penalty alone must not disable smaller workloads")
	MarkSenseNovaBudgetDispatched(small)
	RecordSenseNovaRelaySuccess(small)
	request := senseNovaBudgetRequest{ChannelID: channel.Id, Fingerprint: model.SenseNovaFingerprint("fake-account-a"), Model: "deepseek-v4-pro", PromptTokens: 28935, OutputTokens: 32000, HasOutputLimit: true}
	observations, err := readSenseNovaCapacity(context.Background(), []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	assert.EqualValues(t, 1, observations[0].Failures, "small success cannot erase large-request rejection")
}

func TestSenseNovaCapacityRoutingPreservesFourthRecoverySuccess(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	channel.Key = "fake-a\nfake-b\nfake-c\nfake-d"
	channel.ChannelInfo.MultiKeySize = 4
	require.NoError(t, model.DB.Save(channel).Error)
	seedSenseNovaDueRecoveries(t, channel)
	c := senseNovaLargeAdmissionContext(t, channel)
	defer FinishSenseNovaAdmission(c)
	seen := make(map[string]bool)
	for attempt := 0; attempt < SenseNovaMaxAttempts; attempt++ {
		require.Nil(t, AdmitSenseNovaAttempt(c))
		require.NotEmpty(t, currentSenseNovaAdmission(c).capacityRecoveryOwner)
		key := getSenseNovaAttempt(c).key
		require.False(t, seen[key], "each upstream attempt must use a distinct key")
		seen[key] = true
		MarkSenseNovaBudgetDispatched(c)
		if attempt == SenseNovaMaxAttempts-1 {
			RecordSenseNovaRelaySuccess(c)
			break
		}
		upstream := RecordSenseNovaRelayFailure(c, types.WithOpenAIError(types.OpenAIError{Code: "429001"}, 429))
		require.True(t, ShouldRetrySenseNova(c, upstream))
		_, _, selectionErr := SelectSenseNovaKey(c, channel, "deepseek-v4-pro")
		require.Nil(t, selectionErr)
	}
	assert.Len(t, seen, 4)
}

func TestSenseNovaCapacityRoutingUnsentRecoveryReleasesOwnership(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	seedSenseNovaDueRecoveries(t, channel)
	c := senseNovaLargeAdmissionContext(t, channel)
	defer FinishSenseNovaAdmission(c)
	require.Nil(t, AdmitSenseNovaAttempt(c))
	state := currentSenseNovaAdmission(c)
	require.NotEmpty(t, state.capacityRecoveryOwner)
	FinishSenseNovaAdmission(c)
	assert.Empty(t, state.capacityRecoveryOwner)
	require.Nil(t, AdmitSenseNovaAttempt(c), "unsent reservation and recovery lease must be released")
	MarkSenseNovaBudgetDispatched(c)
	assert.True(t, state.dispatched)
}

func TestSenseNovaCapacityRoutingPoolRecoveryContentionCancelsAndReleasesUnsentBudget(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	seedSenseNovaDueRecoveries(t, channel)
	first := senseNovaLargeAdmissionContext(t, channel)
	defer FinishSenseNovaAdmission(first)
	require.Nil(t, AdmitSenseNovaAttempt(first))
	second := senseNovaLargeAdmissionContext(t, channel)
	defer FinishSenseNovaAdmission(second)
	ctx, cancel := context.WithTimeout(second.Request.Context(), 40*time.Millisecond)
	defer cancel()
	second.Request = second.Request.WithContext(ctx)
	started := time.Now()
	err := AdmitSenseNovaAttempt(second)
	require.NotNil(t, err)
	assert.ErrorIs(t, second.Request.Context().Err(), context.DeadlineExceeded)
	assert.Less(t, time.Since(started), time.Second)
	assert.Nil(t, currentSenseNovaAdmission(second).reservation)

	request := senseNovaBudgetRequest{ChannelID: channel.Id, Fingerprint: model.SenseNovaFingerprint("fake-account-b"), Model: "deepseek-v4-pro", ID: "verify-unsent-cleanup", PromptTokens: 28935}
	reservation, _, reserveErr := reserveSenseNovaBudget(context.Background(), request, currentSenseNovaAdmission(second).config.policy(request.Fingerprint))
	require.NoError(t, reserveErr)
	require.NotNil(t, reservation, "pool contention must release key B's unsent pacing and lease")
	require.NoError(t, finishSenseNovaBudget(context.Background(), reservation, -1, true))
}

func TestSenseNovaCapacityRoutingCanceledDispatchCannotPublishSuccess(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	c := senseNovaLargeAdmissionContext(t, channel)
	defer FinishSenseNovaAdmission(c)
	require.Nil(t, AdmitSenseNovaAttempt(c))
	request := currentSenseNovaAdmission(c).capacityRequest
	MarkSenseNovaBudgetDispatched(c)
	ctx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	cancel()
	RecordSenseNovaRelaySuccess(c)
	observations, err := readSenseNovaCapacity(context.Background(), []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	assert.False(t, observations[0].Verified)
	assert.Zero(t, observations[0].Failures)
}
