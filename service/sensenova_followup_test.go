package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSenseNovaFollowupRequiresLiveVerifiedCompletion(t *testing.T) {
	for _, outcome := range []string{"failed", "unsent", "stale", "no-session", "expired-grant"} {
		t.Run(outcome, func(t *testing.T) {
			server, advance := setupSenseNovaBudgetRedis(t)
			ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
			policy.TokensPerMinute, policy.Followups, policy.FollowupInterval = 0, 2, 5*time.Second
			req := senseNovaTestBudgetRequest("initial", 10)
			req.Conversation = "hashed-session"
			if outcome == "no-session" {
				req.Conversation = ""
			}
			first, _, err := reserveSenseNovaBudget(ctx, req, policy)
			require.NoError(t, err)
			require.NotNil(t, first)
			if outcome == "stale" {
				advance(policy.Lease + time.Second)
			}
			published, err := finishSenseNovaBudgetOutcome(ctx, first, -1, outcome == "unsent", outcome != "failed")
			require.NoError(t, err)
			assert.Equal(t, outcome == "no-session" || outcome == "expired-grant", published)
			if outcome == "expired-grant" {
				advance(time.Minute)
			}
			assert.False(t, server.Exists(first.keys[6]), "only unexpired verified conversation grants permit followups")
			if outcome == "failed" || outcome == "stale" {
				assert.EqualValues(t, 1, common.RDB.ZCard(ctx, first.keys[7]).Val(), "dispatched failures consume request demand")
			}
		})
	}
}

func TestSenseNovaFollowupRequestRateSurvivesReplenishedGrant(t *testing.T) {
	_, advance := setupSenseNovaBudgetRedis(t)
	ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
	policy.TokensPerMinute, policy.Followups, policy.FollowupInterval = 0, 2, 5*time.Second
	req := senseNovaTestBudgetRequest("initial", 0)
	req.Conversation = "hashed-session"
	var last *senseNovaBudgetReservation
	for i := 0; i < 3; i++ {
		req.ID = fmt.Sprint(i)
		r, _, err := reserveSenseNovaBudget(ctx, req, policy)
		require.NoError(t, err)
		require.NotNil(t, r)
		require.NoError(t, finishSenseNovaBudgetVerified(ctx, r, -1, false, true))
		last = r
		advance(5 * time.Second)
	}
	// Independent rolling demand remains binding even if allowance state is
	// refreshed by an operator or another process during a rolling upgrade.
	require.NoError(t, common.RDB.HSet(ctx, last.keys[6], "remaining", 2).Err())
	req.ID = "fourth"
	r, wait, err := reserveSenseNovaBudget(ctx, req, policy)
	require.NoError(t, err)
	assert.Nil(t, r)
	assert.Equal(t, 45*time.Second, wait)
}

func TestSenseNovaUnsentFollowupRestoresOriginalPaceDeadline(t *testing.T) {
	_, advance := setupSenseNovaBudgetRedis(t)
	ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
	policy.TokensPerMinute, policy.Followups, policy.FollowupInterval = 0, 2, 5*time.Second
	req := senseNovaTestBudgetRequest("initial", 10)
	req.Conversation = "hashed-session"
	first, _, err := reserveSenseNovaBudget(ctx, req, policy)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NoError(t, finishSenseNovaBudgetVerified(ctx, first, -1, false, true))
	advance(5 * time.Second)
	req.ID = "unsent-followup"
	unsent, _, err := reserveSenseNovaBudget(ctx, req, policy)
	require.NoError(t, err)
	require.NotNil(t, unsent)
	advance(2 * time.Second)
	require.NoError(t, finishSenseNovaBudgetVerified(ctx, unsent, -1, true, false))
	for _, field := range []string{unsent.id, unsent.id + ":pace", unsent.id + ":pace-expiry"} {
		assert.False(t, common.RDB.HExists(ctx, unsent.keys[9], field).Val(), "unused followup metadata must be removed after pace restoration")
	}
	req.ID, req.Conversation = "other-conversation", "another-session"
	wait, _, err := readSenseNovaBudgetWait(ctx, req, policy)
	require.NoError(t, err)
	assert.Equal(t, 53*time.Second, wait, "unsent work cannot extend the last dispatched start's pacing deadline")
	advance(53 * time.Second)
	next, _, err := reserveSenseNovaBudget(ctx, req, policy)
	require.NoError(t, err)
	require.NotNil(t, next, "ordinary requests regain eligibility at the original deadline")
}

func TestSenseNovaUnsentReservationsBoundMetadata(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprint("expired-lease-", expired), func(t *testing.T) {
			server, advance := setupSenseNovaBudgetRedis(t)
			ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
			policy.TokensPerMinute, policy.Interval = 0, 300*time.Second
			for i := 0; i < 20; i++ {
				req := senseNovaTestBudgetRequest(fmt.Sprint("unsent-", i), 10)
				r, _, err := reserveSenseNovaBudget(ctx, req, policy)
				require.NoError(t, err)
				require.NotNil(t, r)
				if expired {
					advance(policy.Lease + time.Second)
				}
				require.NoError(t, finishSenseNovaBudget(ctx, r, -1, true))
				assert.False(t, server.Exists(r.keys[9]), "removed unused ledger rows must not leave undiscoverable metadata")
			}
		})
	}
}

func TestSenseNovaUnsentMetadataCleanupPreservesReplacement(t *testing.T) {
	setupSenseNovaBudgetRedis(t)
	ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
	policy.TokensPerMinute = 0
	req := senseNovaTestBudgetRequest("reused-request-id", 10)
	old, _, err := reserveSenseNovaBudget(ctx, req, policy)
	require.NoError(t, err)
	require.NotNil(t, old)
	require.NoError(t, finishSenseNovaBudget(ctx, old, -1, true))
	replacement, _, err := reserveSenseNovaBudget(ctx, req, policy)
	require.NoError(t, err)
	require.NotNil(t, replacement)
	require.NoError(t, finishSenseNovaBudget(ctx, old, -1, true))
	assert.True(t, common.RDB.HExists(ctx, replacement.keys[9], replacement.id).Val(), "stale cleanup cannot delete replacement metadata")
	require.NoError(t, renewSenseNovaBudget(ctx, replacement))
}

func TestSenseNovaFollowupBoundedSuccessAndFailure(t *testing.T) {
	_, advance := setupSenseNovaBudgetRedis(t)
	ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
	policy.TokensPerMinute, policy.Followups, policy.FollowupInterval = 0, 2, 5*time.Second
	req := senseNovaTestBudgetRequest("first", 10)
	req.Conversation = "hashed-session"
	first, _, err := reserveSenseNovaBudget(ctx, req, policy)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NoError(t, finishSenseNovaBudgetVerified(ctx, first, -1, false, true))
	advance(5 * time.Second)
	// The allowance belongs to the original authenticated conversation.
	other := req
	other.ID, other.Conversation = "other", "another-session"
	denied, _, err := reserveSenseNovaBudget(ctx, other, policy)
	require.NoError(t, err)
	require.Nil(t, denied)
	for i := 0; i < 2; i++ {
		req.ID = fmt.Sprint("followup", i)
		next, _, err := reserveSenseNovaBudget(ctx, req, policy)
		require.NoError(t, err)
		require.NotNil(t, next)
		require.NoError(t, finishSenseNovaBudgetVerified(ctx, next, -1, false, true))
		advance(5 * time.Second)
	}
	req.ID = "fourth"
	denied, wait, err := reserveSenseNovaBudget(ctx, req, policy)
	require.NoError(t, err)
	assert.Nil(t, denied)
	assert.Positive(t, wait)
	advance(time.Minute)
	req.ID = "new-window"
	first, _, err = reserveSenseNovaBudget(ctx, req, policy)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NoError(t, finishSenseNovaBudgetVerified(ctx, first, -1, false, true))
	advance(5 * time.Second)
	req.ID = "failed-followup"
	failed, _, err := reserveSenseNovaBudget(ctx, req, policy)
	require.NoError(t, err)
	require.NotNil(t, failed)
	require.NoError(t, finishSenseNovaBudgetVerified(ctx, failed, -1, false, false))
	advance(5 * time.Second)
	req.ID = "after-failure"
	denied, _, err = reserveSenseNovaBudget(ctx, req, policy)
	require.NoError(t, err)
	assert.Nil(t, denied, "failure withdraws the remaining allowance")
}

func TestSenseNovaFollowupConcurrentOwnershipAndUnsentRefund(t *testing.T) {
	_, advance := setupSenseNovaBudgetRedis(t)
	ctx, policy := context.Background(), senseNovaTestBudgetPolicy()
	policy.TokensPerMinute, policy.Followups, policy.FollowupInterval = 0, 2, 5*time.Second
	req := senseNovaTestBudgetRequest("initial", 10)
	req.Conversation = "hashed-session"
	first, _, err := reserveSenseNovaBudget(ctx, req, policy)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NoError(t, finishSenseNovaBudgetVerified(ctx, first, -1, false, true))
	advance(5 * time.Second)
	var wg sync.WaitGroup
	admitted := make(chan *senseNovaBudgetReservation, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := req
			r.ID = fmt.Sprint(i)
			next, _, err := reserveSenseNovaBudget(ctx, r, policy)
			assert.NoError(t, err)
			if next != nil {
				admitted <- next
			}
		}(i)
	}
	wg.Wait()
	close(admitted)
	require.Len(t, admitted, 1)
	for next := range admitted {
		require.NoError(t, finishSenseNovaBudgetVerified(ctx, next, -1, true, false))
	}
	req.ID = "after-unsent"
	next, _, err := reserveSenseNovaBudget(ctx, req, policy)
	require.NoError(t, err)
	require.NotNil(t, next, "unused reservation restores allowance and local request slot")
	require.NoError(t, finishSenseNovaBudgetVerified(ctx, next, -1, false, false))
	// A duplicate successful finish must not resurrect the failed allowance.
	require.NoError(t, finishSenseNovaBudgetVerified(ctx, next, -1, false, true))
	advance(5 * time.Second)
	req.ID = "duplicate"
	next, _, err = reserveSenseNovaBudget(ctx, req, policy)
	require.NoError(t, err)
	assert.Nil(t, next)
}
