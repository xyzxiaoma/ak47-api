package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func senseNovaCapacityTestRequest() senseNovaBudgetRequest {
	return senseNovaBudgetRequest{ChannelID: 15, Fingerprint: "synthetic-fingerprint", Model: "deepseek-v4-pro", ID: "capacity-request", PromptTokens: 28935, OutputTokens: 32000, HasOutputLimit: true}
}

func senseNovaCapacityTestReserve(t *testing.T, request senseNovaBudgetRequest) *senseNovaBudgetReservation {
	t.Helper()
	policy := senseNovaTestBudgetPolicy()
	policy.TokensPerMinute = 0
	reservation, _, err := reserveSenseNovaBudget(context.Background(), request, policy)
	require.NoError(t, err)
	require.NotNil(t, reservation)
	return reservation
}

func TestSenseNovaCapacityShapeBoundaries(t *testing.T) {
	request := senseNovaCapacityTestRequest()
	shape := senseNovaCapacityShape(request)
	nearby := request
	nearby.PromptTokens, nearby.OutputTokens = 32768, 32768
	assert.Equal(t, shape, senseNovaCapacityShape(nearby))
	for _, field := range []string{"input", "output", "absent", "zero"} {
		other := request
		switch field {
		case "input":
			other.PromptTokens = 32769
		case "output":
			other.OutputTokens = 32769
		case "absent":
			other.HasOutputLimit = false
		case "zero":
			other.OutputTokens = 0
		}
		assert.NotEqual(t, shape, senseNovaCapacityShape(other), field)
	}
	request.PromptTokens, request.OutputTokens = 0, 1
	nearby = request
	nearby.PromptTokens, nearby.OutputTokens = 8192, 1024
	assert.Equal(t, senseNovaCapacityShape(request), senseNovaCapacityShape(nearby))
	request.OutputTokens = 0
	nearby = request
	nearby.HasOutputLimit = false
	assert.NotEqual(t, senseNovaCapacityShape(request), senseNovaCapacityShape(nearby))
	request.PromptTokens, request.OutputTokens = math.MaxInt64, math.MaxInt64
	assert.NotEmpty(t, senseNovaCapacityShape(request), "largest integer must terminate without overflow")
}

func TestSenseNovaCapacityStoreOutcomesIsolationAndExpiry(t *testing.T) {
	server, advance := setupSenseNovaBudgetRedis(t)
	ctx := context.Background()
	request := senseNovaCapacityTestRequest()
	owner := senseNovaCapacityTestReserve(t, request)
	requests := []senseNovaBudgetRequest{request}
	for _, field := range []string{"channel", "fingerprint", "model", "input", "output", "absent", "zero"} {
		other := request
		switch field {
		case "channel":
			other.ChannelID++
		case "fingerprint":
			other.Fingerprint += "other"
		case "model":
			other.Model += "other"
		case "input":
			other.PromptTokens = 5
		case "output":
			other.OutputTokens = 128
		case "absent":
			other.HasOutputLimit = false
		case "zero":
			other.OutputTokens = 0
		}
		requests = append(requests, other)
	}
	require.NoError(t, recordSenseNovaCapacity(ctx, owner, request, false, true, 0))
	observed, err := readSenseNovaCapacity(ctx, requests)
	require.NoError(t, err)
	require.Len(t, observed, len(requests))
	assert.Equal(t, senseNovaCapacityObservation{Failures: 1, RetryAfter: time.Minute}, observed[0])
	for _, observation := range observed[1:] {
		assert.Zero(t, observation)
	}
	// Non-TPM failures cannot create or erase capacity evidence.
	require.NoError(t, recordSenseNovaCapacity(ctx, owner, request, false, false, 600))
	again, err := readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	assert.Equal(t, observed[:1], again)
	// Successful tiny traffic only clears its own class.
	require.NoError(t, finishSenseNovaBudget(ctx, owner, -1, false))
	advance(time.Minute)
	owner = senseNovaCapacityTestReserve(t, requests[4])
	require.NoError(t, recordSenseNovaCapacity(ctx, owner, requests[4], true, false, 0))
	observed, err = readSenseNovaCapacity(ctx, requests)
	require.NoError(t, err)
	assert.Equal(t, int64(1), observed[0].Failures)
	assert.True(t, observed[4].Verified)
	require.NoError(t, finishSenseNovaBudget(ctx, owner, -1, false))
	advance(time.Minute)
	owner = senseNovaCapacityTestReserve(t, request)
	require.NoError(t, recordSenseNovaCapacity(ctx, owner, request, true, false, 0))
	observed, err = readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	assert.Equal(t, []senseNovaCapacityObservation{{Verified: true}}, observed)
	for _, key := range server.Keys() {
		assert.NotContains(t, key, request.Fingerprint)
		assert.NotContains(t, key, request.Model)
	}
	advance(14 * time.Minute)
	observed, err = readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	require.True(t, observed[0].Verified)
	advance(time.Minute)
	observed, err = readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	assert.Zero(t, observed[0], "reading near expiry must not extend evidence")
}

func TestSenseNovaCapacityStorePenaltyAndRedisTime(t *testing.T) {
	_, advance := setupSenseNovaBudgetRedis(t)
	ctx := context.Background()
	request := senseNovaCapacityTestRequest()
	for i, expected := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 5 * time.Minute, 5 * time.Minute, 20 * time.Minute, 24 * time.Hour} {
		request.ID = fmt.Sprint(i)
		owner := senseNovaCapacityTestReserve(t, request)
		retryAfter := int64(0)
		if i == 5 {
			retryAfter = 1200
		}
		if i == 6 {
			retryAfter = math.MaxInt64
		}
		require.NoError(t, recordSenseNovaCapacity(ctx, owner, request, false, true, retryAfter))
		require.NoError(t, finishSenseNovaBudget(ctx, owner, -1, false))
		observed, err := readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
		require.NoError(t, err)
		assert.Equal(t, int64(i+1), observed[0].Failures)
		assert.Equal(t, expected, observed[0].RetryAfter)
		advance(time.Minute)
		observed, err = readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
		require.NoError(t, err)
		assert.Equal(t, expected-time.Minute, observed[0].RetryAfter)
	}
	advance(16 * time.Minute)
	observed, err := readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	assert.Positive(t, observed[0].RetryAfter, "long Retry-After must outlive ordinary evidence TTL")
	advance(24 * time.Hour)
	observed, err = readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	assert.Zero(t, observed[0])
}

func TestSenseNovaCapacityStorePublicationIsIdempotent(t *testing.T) {
	_, advance := setupSenseNovaBudgetRedis(t)
	ctx := context.Background()
	request := senseNovaCapacityTestRequest()
	owner := senseNovaCapacityTestReserve(t, request)
	require.NoError(t, recordSenseNovaCapacity(ctx, owner, request, false, true, 0))
	advance(5 * time.Second)
	require.NoError(t, recordSenseNovaCapacity(ctx, owner, request, false, true, 1200))
	observed, err := readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	assert.Equal(t, []senseNovaCapacityObservation{{Failures: 1, RetryAfter: 55 * time.Second}}, observed,
		"retrying an uncertain write must not count twice or extend its penalty")
	require.NoError(t, recordSenseNovaCapacity(ctx, owner, request, true, false, 0))
	observed, err = readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	assert.Equal(t, []senseNovaCapacityObservation{{Failures: 1, RetryAfter: 55 * time.Second}}, observed,
		"the accepted outcome cannot be relabeled by the same owner")
	advance(15*time.Minute - 5*time.Second)
	observed, err = readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	assert.Zero(t, observed[0], "duplicate publication must not refresh evidence expiry")
}

func TestSenseNovaCapacityStorePreservesFutureRetryDeadline(t *testing.T) {
	_, advance := setupSenseNovaBudgetRedis(t)
	ctx := context.Background()
	request := senseNovaCapacityTestRequest()
	owner := senseNovaCapacityTestReserve(t, request)
	require.NoError(t, recordSenseNovaCapacity(ctx, owner, request, false, true, 1200))
	require.NoError(t, finishSenseNovaBudget(ctx, owner, -1, false))
	advance(time.Minute)
	request.ID = "second-dispatch"
	owner = senseNovaCapacityTestReserve(t, request)
	require.NoError(t, recordSenseNovaCapacity(ctx, owner, request, false, true, 0))
	observed, err := readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	assert.Equal(t, []senseNovaCapacityObservation{{Failures: 2, RetryAfter: 19 * time.Minute}}, observed,
		"a later rejection without a hint must retain the established future retry deadline")
	advance(16 * time.Minute)
	observed, err = readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	assert.Equal(t, 3*time.Minute, observed[0].RetryAfter, "retained deadline must also retain its evidence")
}

func TestSenseNovaCapacityStoreRejectsLostOrWrongOwner(t *testing.T) {
	_, advance := setupSenseNovaBudgetRedis(t)
	ctx := context.Background()
	request := senseNovaCapacityTestRequest()
	old := senseNovaCapacityTestReserve(t, request)
	other := request
	other.Fingerprint += "other"
	require.Error(t, recordSenseNovaCapacity(ctx, old, other, true, false, 0))
	advance(time.Minute)
	current := senseNovaCapacityTestReserve(t, request)
	require.NoError(t, recordSenseNovaCapacity(ctx, current, request, false, true, 0))
	require.ErrorIs(t, recordSenseNovaCapacity(ctx, old, request, true, false, 0), errSenseNovaBudgetLeaseLost)
	observed, err := readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	assert.Equal(t, int64(1), observed[0].Failures)
	require.NoError(t, finishSenseNovaBudget(ctx, current, -1, false))
	require.ErrorIs(t, recordSenseNovaCapacity(ctx, current, request, true, false, 0), errSenseNovaBudgetLeaseLost)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	require.Error(t, recordSenseNovaCapacity(canceled, current, request, true, false, 0))
	require.Error(t, recordSenseNovaCapacity(ctx, nil, request, true, false, 0))
}

func TestSenseNovaCapacityRecoveryExclusiveScopeAndOwnership(t *testing.T) {
	server, advance := setupSenseNovaBudgetRedis(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	winners := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			owner := fmt.Sprint(i)
			ok, wait, err := claimSenseNovaCapacityRecovery(ctx, 15, "deepseek-v4-pro", owner, 10*time.Second)
			assert.NoError(t, err)
			if ok {
				winners <- owner
			} else {
				assert.Equal(t, 10*time.Second, wait)
			}
		}(i)
	}
	wg.Wait()
	close(winners)
	require.Len(t, winners, 1)
	owner := <-winners
	ok, _, err := claimSenseNovaCapacityRecovery(ctx, 15, "deepseek-v4-pro", owner, time.Second)
	require.NoError(t, err)
	require.True(t, ok)
	advance(8 * time.Second)
	ok, err = renewSenseNovaCapacityRecovery(ctx, 15, "deepseek-v4-pro", owner, 10*time.Second)
	require.NoError(t, err)
	require.True(t, ok)
	advance(3 * time.Second)
	ok, _, err = claimSenseNovaCapacityRecovery(ctx, 15, "deepseek-v4-pro", "replacement", 10*time.Second)
	require.NoError(t, err)
	require.False(t, ok)
	for _, scope := range []struct {
		channel int
		name    string
	}{{16, "deepseek-v4-pro"}, {15, "deepseek-v4-flash"}} {
		ok, _, err = claimSenseNovaCapacityRecovery(ctx, scope.channel, scope.name, "separate", time.Second)
		require.NoError(t, err)
		assert.True(t, ok)
	}
	advance(7 * time.Second)
	ok, _, err = claimSenseNovaCapacityRecovery(ctx, 15, "deepseek-v4-pro", "replacement", 10*time.Second)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = renewSenseNovaCapacityRecovery(ctx, 15, "deepseek-v4-pro", owner, time.Hour)
	require.NoError(t, err)
	assert.False(t, ok)
	require.NoError(t, releaseSenseNovaCapacityRecovery(ctx, 15, "deepseek-v4-pro", owner))
	ok, _, err = claimSenseNovaCapacityRecovery(ctx, 15, "deepseek-v4-pro", "third", time.Second)
	require.NoError(t, err)
	assert.False(t, ok, "stale release must not free replacement")
	for _, key := range server.Keys() {
		assert.NotContains(t, key, "deepseek")
		value, err := server.Get(key)
		require.NoError(t, err)
		assert.False(t, strings.Contains(value, "replacement"), "store owner as a fingerprint")
	}
	require.NoError(t, releaseSenseNovaCapacityRecovery(ctx, 15, "deepseek-v4-pro", "replacement"))
	ok, _, err = claimSenseNovaCapacityRecovery(ctx, 15, "deepseek-v4-pro", "third", time.Second)
	require.NoError(t, err)
	assert.True(t, ok)
	advance(time.Second)
	assert.Empty(t, server.Keys())
}

func TestSenseNovaCapacityStoreUnavailableIsSanitized(t *testing.T) {
	setupSenseNovaBudgetRedis(t)
	request := senseNovaCapacityTestRequest()
	owner := senseNovaCapacityTestReserve(t, request)
	client := common.RDB
	for _, mode := range []string{"missing", "closed"} {
		if mode == "missing" {
			common.RDB = nil
		} else {
			common.RDB = client
			require.NoError(t, client.Close())
		}
		_, err := readSenseNovaCapacity(context.Background(), []senseNovaBudgetRequest{request})
		assert.ErrorIs(t, err, errSenseNovaBudgetUnavailable)
		assert.ErrorIs(t, recordSenseNovaCapacity(context.Background(), owner, request, true, false, 0), errSenseNovaBudgetUnavailable)
		_, _, err = claimSenseNovaCapacityRecovery(context.Background(), 15, request.Model, "owner", time.Minute)
		assert.ErrorIs(t, err, errSenseNovaBudgetUnavailable)
		_, err = renewSenseNovaCapacityRecovery(context.Background(), 15, request.Model, "owner", time.Minute)
		assert.ErrorIs(t, err, errSenseNovaBudgetUnavailable)
		assert.ErrorIs(t, releaseSenseNovaCapacityRecovery(context.Background(), 15, request.Model, "owner"), errSenseNovaBudgetUnavailable)
	}
}

func TestSenseNovaCapacityStoreIgnoresLegacyEvidenceWithoutReleasingOwners(t *testing.T) {
	for _, verified := range []bool{false, true} {
		t.Run(fmt.Sprint(verified), func(t *testing.T) {
			server, _ := setupSenseNovaBudgetRedis(t)
			ctx := context.Background()
			request := senseNovaCapacityTestRequest()
			owner := senseNovaCapacityTestReserve(t, request)
			claimed, _, err := claimSenseNovaCapacityRecovery(ctx, request.ChannelID, request.Model, owner.owner, time.Minute)
			require.NoError(t, err)
			require.True(t, claimed)
			now, err := common.RDB.Time(ctx).Result()
			require.NoError(t, err)
			legacy := senseNovaBudgetBaseKey(request) + "capacity:i32768:o32768"
			verifiedValue, failures := 0, 3
			if verified {
				verifiedValue, failures = 1, 0
			}
			require.NoError(t, common.RDB.HSet(ctx, legacy, "verified", verifiedValue, "failures", failures, "retry", now.Add(4*time.Minute).UnixMilli(), "expires", now.Add(15*time.Minute).UnixMilli()).Err())
			require.NoError(t, common.RDB.Expire(ctx, legacy, 15*time.Minute).Err())
			observations, err := readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
			require.NoError(t, err)
			assert.Equal(t, []senseNovaCapacityObservation{{}}, observations, "older false TPM/success evidence cannot affect current selection")
			require.NoError(t, recordSenseNovaCapacity(ctx, owner, request, false, true, 0))
			observations, err = readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
			require.NoError(t, err)
			assert.Equal(t, []senseNovaCapacityObservation{{Failures: 1, RetryAfter: time.Minute}}, observations, "new explicit evidence starts a fresh history")
			assert.Equal(t, 15*time.Minute, server.TTL(legacy), "legacy evidence expires naturally, without touching live keys")
			require.NoError(t, renewSenseNovaBudget(ctx, owner))
			blocked, _, err := reserveSenseNovaBudget(ctx, senseNovaBudgetRequest{ChannelID: request.ChannelID, Fingerprint: request.Fingerprint, Model: request.Model, ID: "another-owner", HasOutputLimit: true}, senseNovaBudgetPolicy{Window: time.Minute, Interval: time.Minute, Lease: time.Minute})
			require.NoError(t, err)
			assert.Nil(t, blocked, "evidence migration must not release active budget ownership or pacing")
			claimed, _, err = claimSenseNovaCapacityRecovery(ctx, request.ChannelID, request.Model, "another-owner", time.Minute)
			require.NoError(t, err)
			assert.False(t, claimed, "evidence migration must not release the pool recovery owner")
		})
	}
}
