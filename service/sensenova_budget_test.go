package service

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSenseNovaBudgetRedis(t *testing.T) (*miniredis.Miniredis, func(time.Duration)) {
	t.Helper()
	server := miniredis.RunT(t)
	previous := common.RDB
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	common.RDB = client
	t.Cleanup(func() { common.RDB = previous; _ = client.Close() })
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	server.SetTime(now)
	// FastForward expires keys; SetTime independently advances Redis TIME.
	return server, func(d time.Duration) { now = now.Add(d); server.FastForward(d); server.SetTime(now) }
}

func senseNovaTestBudgetPolicy() senseNovaBudgetPolicy {
	return senseNovaBudgetPolicy{TokensPerMinute: 100, OutputAllowance: 20, Window: time.Minute, Interval: time.Minute, Lease: 10 * time.Second}
}

func senseNovaTestBudgetRequest(id string, tokens int64) senseNovaBudgetRequest {
	return senseNovaBudgetRequest{ChannelID: 7, Fingerprint: "fingerprint", Model: "deepseek-v4-pro", ID: id, PromptTokens: tokens, HasOutputLimit: true}
}

func TestSenseNovaBudgetConcurrentAdmission(t *testing.T) {
	setupSenseNovaBudgetRedis(t)
	ctx := context.Background()
	policy := senseNovaTestBudgetPolicy()
	var wg sync.WaitGroup
	results := make(chan *senseNovaBudgetReservation, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest(fmt.Sprint(i), 60), policy)
			assert.NoError(t, err)
			if r != nil {
				results <- r
			}
		}(i)
	}
	wg.Wait()
	close(results)
	require.Len(t, results, 1)
	for r := range results {
		require.NoError(t, finishSenseNovaBudget(ctx, r, -1, false))
	}
	r, wait, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("overdraw", 41), policy)
	require.NoError(t, err)
	assert.Nil(t, r)
	assert.Equal(t, time.Minute, wait)
	r, wait, err = reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("remaining", 40), policy)
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Zero(t, wait)
}

func TestSenseNovaBudgetInflightIdempotenceAndScope(t *testing.T) {
	setupSenseNovaBudgetRedis(t)
	ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
	request := senseNovaTestBudgetRequest("first", 10)
	r, _, err := reserveSenseNovaBudget(ctx, request, policy)
	require.NoError(t, err)
	require.NotNil(t, r)
	again, _, err := reserveSenseNovaBudget(ctx, request, policy)
	require.NoError(t, err)
	require.Equal(t, r, again)
	blocked, wait, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("second", 10), policy)
	require.NoError(t, err)
	assert.Nil(t, blocked)
	assert.Equal(t, policy.Lease, wait)
	for _, field := range []string{"channel", "fingerprint", "model"} {
		other := request
		switch field {
		case "channel":
			other.ChannelID++
		case "fingerprint":
			other.Fingerprint += "2"
		case "model":
			other.Model += "-other"
		}
		admitted, _, err := reserveSenseNovaBudget(ctx, other, policy)
		require.NoError(t, err)
		assert.NotNil(t, admitted)
	}
	require.NoError(t, finishSenseNovaBudget(ctx, r, 10, false))
	next, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("full-remainder", 90), policy)
	require.NoError(t, err)
	assert.NotNil(t, next, "idempotent reserve must not double debit")
}

func TestSenseNovaBudgetReconciliation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		actual    int64
		unused    bool
		remaining int64
	}{
		{"unused", -1, true, 100}, {"uncertain", -1, false, 40}, {"lower", 20, false, 80},
		{"larger", 90, false, 10}, {"zero", 0, false, 100}, {"over-limit", 120, false, 0}, {"overflow-usage", math.MaxInt64, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, advance := setupSenseNovaBudgetRedis(t)
			ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
			r, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("first", 60), policy)
			require.NoError(t, err)
			require.NotNil(t, r)
			require.NoError(t, finishSenseNovaBudget(ctx, r, tc.actual, tc.unused))
			// Duplicate completion must not debit or credit other reservations.
			require.NoError(t, finishSenseNovaBudget(ctx, r, tc.actual, tc.unused))
			if tc.remaining < 100 {
				blocked, wait, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("too-much", tc.remaining+1), policy)
				require.NoError(t, err)
				assert.Nil(t, blocked)
				assert.Equal(t, time.Minute, wait)
			}
			if tc.remaining > 0 {
				next, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("next", tc.remaining), policy)
				require.NoError(t, err)
				require.NotNil(t, next)
				require.NoError(t, finishSenseNovaBudget(ctx, next, -1, false))
			}
			advance(time.Minute)
			next, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("new-window", 100), policy)
			require.NoError(t, err)
			assert.NotNil(t, next)
		})
	}
}

func TestSenseNovaBudgetUnknownPacing(t *testing.T) {
	for _, unused := range []bool{false, true} {
		t.Run(fmt.Sprint(unused), func(t *testing.T) {
			_, advance := setupSenseNovaBudgetRedis(t)
			ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
			policy.TokensPerMinute = 0
			r, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("first", math.MaxInt64), policy)
			require.NoError(t, err, "unknown TPM must not invent a token quota")
			require.NotNil(t, r)
			require.NoError(t, finishSenseNovaBudget(ctx, r, -1, unused))
			next, wait, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("next", 1), policy)
			require.NoError(t, err)
			if unused {
				assert.NotNil(t, next)
				return
			}
			assert.Nil(t, next)
			assert.Equal(t, time.Minute, wait)
			advance(59 * time.Second)
			next, wait, err = reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("next", 1), policy)
			require.NoError(t, err)
			assert.Nil(t, next)
			assert.Equal(t, time.Second, wait)
			advance(time.Second)
			next, _, err = reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("next", 1), policy)
			require.NoError(t, err)
			assert.NotNil(t, next)
		})
	}
}

func TestSenseNovaBudgetRollingWindowAndStaleFinish(t *testing.T) {
	_, advance := setupSenseNovaBudgetRedis(t)
	ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
	old, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("old", 60), policy)
	require.NoError(t, err)
	advance(30 * time.Second)
	next, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("next", 40), policy)
	require.NoError(t, err)
	require.NotNil(t, next, "expired in-flight lease must recover")
	require.NoError(t, finishSenseNovaBudget(ctx, old, 20, false))
	blocked, wait, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("blocked", 1), policy)
	require.NoError(t, err)
	assert.Nil(t, blocked, "stale completion cannot free newer lease")
	assert.Equal(t, policy.Lease, wait)
	require.NoError(t, finishSenseNovaBudget(ctx, next, -1, false))
	advance(30 * time.Second)
	require.NoError(t, finishSenseNovaBudget(ctx, old, 0, true))
	r, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("remaining", 60), policy)
	require.NoError(t, err)
	require.NotNil(t, r)
	require.NoError(t, finishSenseNovaBudget(ctx, r, -1, false))
	r, wait, err = reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("over", 1), policy)
	require.NoError(t, err)
	assert.Nil(t, r)
	assert.Equal(t, 30*time.Second, wait, "history expires per admission, not a fixed bucket")
}

func TestSenseNovaBudgetWaitUntilEnoughCapacityExpires(t *testing.T) {
	_, advance := setupSenseNovaBudgetRedis(t)
	ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
	for i, tokens := range []int64{20, 60, 20} {
		if i > 0 {
			advance(10 * time.Second)
		}
		r, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest(fmt.Sprint(i), tokens), policy)
		require.NoError(t, err)
		require.NotNil(t, r)
		require.NoError(t, finishSenseNovaBudget(ctx, r, tokens, false))
	}
	request := senseNovaTestBudgetRequest("needs-eighty", 80)
	r, wait, err := reserveSenseNovaBudget(ctx, request, policy)
	require.NoError(t, err)
	require.Nil(t, r)
	require.Equal(t, 50*time.Second, wait, "the first 20-token expiry alone cannot admit an 80-token request")
	advance(49 * time.Second)
	r, wait, err = reserveSenseNovaBudget(ctx, request, policy)
	require.NoError(t, err)
	require.Nil(t, r)
	require.Equal(t, time.Second, wait)
	advance(time.Second)
	r, wait, err = reserveSenseNovaBudget(ctx, request, policy)
	require.NoError(t, err)
	require.NotNil(t, r)
	require.Zero(t, wait)
}

func TestSenseNovaBudgetLeaseRenewalAndTTL(t *testing.T) {
	server, advance := setupSenseNovaBudgetRedis(t)
	ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
	r, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("long", 100), policy)
	require.NoError(t, err)
	require.NotNil(t, r)
	advance(9 * time.Second)
	require.NoError(t, renewSenseNovaBudget(ctx, r))
	advance(2 * time.Second)
	blocked, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("blocked", 0), policy)
	require.NoError(t, err)
	assert.Nil(t, blocked)
	advance(50 * time.Second)
	next, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("new", 100), policy)
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Error(t, renewSenseNovaBudget(ctx, r), "old owner cannot renew newer lease")
	require.NoError(t, finishSenseNovaBudget(ctx, r, 0, true))
	blocked, _, err = reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("blocked-again", 0), policy)
	require.NoError(t, err)
	assert.Nil(t, blocked)
	require.NoError(t, finishSenseNovaBudget(ctx, next, -1, false))
	for _, key := range server.Keys() {
		assert.Positive(t, server.TTL(key), "key %s must expire", key)
	}
	advance(2 * time.Minute)
	assert.Empty(t, server.Keys())
}

func TestSenseNovaBudgetExpiredReservationCannotFinishReusedID(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprint(unknown), func(t *testing.T) {
			_, advance := setupSenseNovaBudgetRedis(t)
			ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
			if unknown {
				policy.TokensPerMinute = 0
			}
			request := senseNovaTestBudgetRequest("reused", 100)
			old, _, err := reserveSenseNovaBudget(ctx, request, policy)
			require.NoError(t, err)
			require.NotNil(t, old)
			advance(time.Minute)
			current, _, err := reserveSenseNovaBudget(ctx, request, policy)
			require.NoError(t, err)
			require.NotNil(t, current)
			require.NoError(t, finishSenseNovaBudget(ctx, old, 0, true))
			assert.Error(t, renewSenseNovaBudget(ctx, old))
			require.NoError(t, renewSenseNovaBudget(ctx, current))
			require.NoError(t, finishSenseNovaBudget(ctx, current, -1, false))
			blocked, wait, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("next", 1), policy)
			require.NoError(t, err)
			assert.Nil(t, blocked, "old unused completion must not refund current usage or pacing")
			assert.Equal(t, time.Minute, wait)
		})
	}
}

func TestSenseNovaBudgetCompletedWorkCannotBecomeUnused(t *testing.T) {
	setupSenseNovaBudgetRedis(t)
	ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
	r, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("first", 100), policy)
	require.NoError(t, err)
	require.NotNil(t, r)
	require.NoError(t, finishSenseNovaBudget(ctx, r, -1, false))
	require.NoError(t, finishSenseNovaBudget(ctx, r, 0, true))
	blocked, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("next", 1), policy)
	require.NoError(t, err)
	assert.Nil(t, blocked, "duplicate cleanup cannot refund dispatched work")
}

func TestSenseNovaBudgetLongStreamCompletion(t *testing.T) {
	for _, actual := range []int64{80, -1} {
		t.Run(fmt.Sprint(actual), func(t *testing.T) {
			_, advance := setupSenseNovaBudgetRedis(t)
			ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
			policy.Lease = 120 * time.Second
			r, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("long", 60), policy)
			require.NoError(t, err)
			require.NotNil(t, r)
			advance(50 * time.Second)
			require.NoError(t, renewSenseNovaBudget(ctx, r))
			advance(80 * time.Second)
			require.NoError(t, finishSenseNovaBudget(ctx, r, actual, false))
			// Even after history expired, successful usage (or the conservative
			// estimate for uncertain work) must occupy a fresh rolling window.
			debit := actual
			if debit < 0 {
				debit = 60
			}
			blocked, wait, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("over", 101-debit), policy)
			require.NoError(t, err)
			require.Nil(t, blocked)
			assert.Equal(t, time.Minute, wait)
			require.NoError(t, finishSenseNovaBudget(ctx, r, 0, true))
			next, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("remaining", 100-debit), policy)
			require.NoError(t, err)
			require.NotNil(t, next)
			require.NoError(t, finishSenseNovaBudget(ctx, next, -1, false))
			advance(time.Minute)
			full, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("full", 100), policy)
			require.NoError(t, err)
			assert.NotNil(t, full)
		})
	}
}

func TestSenseNovaBudgetExactLargeIntegers(t *testing.T) {
	setupSenseNovaBudgetRedis(t)
	ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
	policy.TokensPerMinute = 1<<53 - 1
	r, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("first", policy.TokensPerMinute-1), policy)
	require.NoError(t, err)
	require.NotNil(t, r)
	require.NoError(t, finishSenseNovaBudget(ctx, r, -1, false))
	next, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("one-left", 1), policy)
	require.NoError(t, err)
	require.NotNil(t, next)
	require.NoError(t, finishSenseNovaBudget(ctx, next, -1, false))
	blocked, _, err := reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("over", 1), policy)
	require.NoError(t, err)
	assert.Nil(t, blocked)
}

func TestSenseNovaBudgetOutputEstimateAndFailures(t *testing.T) {
	setupSenseNovaBudgetRedis(t)
	ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
	request := senseNovaTestBudgetRequest("allowance", 81)
	request.HasOutputLimit = false
	r, _, err := reserveSenseNovaBudget(ctx, request, policy)
	assert.ErrorIs(t, err, errSenseNovaBudgetTooLarge)
	assert.Nil(t, r)
	request.HasOutputLimit = true
	request.PromptTokens, request.OutputTokens = math.MaxInt64, 1
	r, _, err = reserveSenseNovaBudget(ctx, request, policy)
	assert.ErrorIs(t, err, errSenseNovaBudgetTooLarge)
	assert.Nil(t, r)
	request.PromptTokens, request.OutputTokens = 100, 0
	r, _, err = reserveSenseNovaBudget(ctx, request, policy)
	require.NoError(t, err)
	require.NotNil(t, r, "explicit zero must override absent-output allowance")
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, _, err = reserveSenseNovaBudget(canceled, senseNovaTestBudgetRequest("cancel", 1), policy)
	assert.Error(t, err)
	client := common.RDB
	common.RDB = nil
	_, _, err = reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("nil", 1), policy)
	assert.Error(t, err)
	assert.Error(t, renewSenseNovaBudget(ctx, r))
	assert.Error(t, finishSenseNovaBudget(ctx, r, -1, false))
	common.RDB = client
	require.NoError(t, client.Close())
	_, _, err = reserveSenseNovaBudget(ctx, senseNovaTestBudgetRequest("closed", 1), policy)
	assert.Error(t, err)
}

func TestSenseNovaQueueBoundedConcurrentAndExpiry(t *testing.T) {
	server, advance := setupSenseNovaBudgetRedis(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	admitted := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprint(i)
			ok, err := acquireSenseNovaQueue(ctx, 7, id, 2, time.Minute)
			assert.NoError(t, err)
			if ok {
				admitted <- id
			}
		}(i)
	}
	wg.Wait()
	require.Len(t, admitted, 2)
	id := <-admitted
	ok, err := acquireSenseNovaQueue(ctx, 7, id, 2, time.Minute)
	require.NoError(t, err)
	assert.True(t, ok, "same ID does not consume another slot")
	require.NoError(t, releaseSenseNovaQueue(ctx, 7, "not-owner"))
	ok, err = acquireSenseNovaQueue(ctx, 7, "blocked", 2, time.Minute)
	require.NoError(t, err)
	assert.False(t, ok)
	require.NoError(t, releaseSenseNovaQueue(ctx, 7, id))
	require.NoError(t, releaseSenseNovaQueue(ctx, 7, id))
	ok, err = acquireSenseNovaQueue(ctx, 7, "replacement", 2, time.Minute)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = acquireSenseNovaQueue(ctx, 8, "other-channel", 1, 2*time.Minute)
	require.NoError(t, err)
	assert.True(t, ok)
	advance(time.Minute)
	ok, err = acquireSenseNovaQueue(ctx, 7, "recovered", 2, time.Minute)
	require.NoError(t, err)
	assert.True(t, ok)
	for _, key := range server.Keys() {
		assert.Positive(t, server.TTL(key))
	}
	advance(2 * time.Minute)
	assert.Empty(t, server.Keys())
	ok, err = acquireSenseNovaQueue(ctx, 7, "disabled", 0, time.Minute)
	assert.False(t, ok)
	assert.Error(t, err)
	common.RDB = nil
	_, err = acquireSenseNovaQueue(ctx, 7, "failure", 1, time.Minute)
	assert.Error(t, err)
	assert.Error(t, releaseSenseNovaQueue(ctx, 7, "failure"))
}

func TestSenseNovaQueueMixedLeaseDurations(t *testing.T) {
	_, advance := setupSenseNovaBudgetRedis(t)
	ctx := context.Background()
	ok, err := acquireSenseNovaQueue(ctx, 7, "long", 2, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = acquireSenseNovaQueue(ctx, 7, "short", 2, time.Second)
	require.NoError(t, err)
	require.True(t, ok)
	// An idempotent retry with a shorter TTL must not shorten the first lease.
	ok, err = acquireSenseNovaQueue(ctx, 7, "long", 2, time.Second)
	require.NoError(t, err)
	require.True(t, ok)
	advance(2 * time.Second)
	ok, err = acquireSenseNovaQueue(ctx, 7, "replacement", 2, time.Minute)
	require.NoError(t, err)
	assert.True(t, ok, "dead waiter must be pruned while another waiter survives")
	ok, err = acquireSenseNovaQueue(ctx, 7, "blocked", 2, time.Minute)
	require.NoError(t, err)
	assert.False(t, ok, "short lease must not shorten the shared Redis key TTL")
}
