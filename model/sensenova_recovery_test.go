package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSenseNovaProbePreservesTrafficHistory(t *testing.T) {
	for _, scope := range []string{"", "glm-5.2"} {
		t.Run("scope="+scope, func(t *testing.T) {
			channel := setupSenseNovaTest(t)
			key := "test-account-a"
			snapshot, err := SenseNovaKeySnapshot(channel.Id, key, "glm-5.2")
			require.NoError(t, err)
			applied, err := RecordSenseNovaSuccess(snapshot, key, 1000)
			require.NoError(t, err)
			require.True(t, applied)
			applied, err = RecordSenseNovaFailure(snapshot, key, scope, "rate_limited", false, 180, 1001)
			require.NoError(t, err)
			require.True(t, applied)
			claim, err := ClaimSenseNovaProbe(channel.Id, key, scope, 1181, false)
			require.NoError(t, err)
			applied, err = FinishSenseNovaProbe(claim, true, "", false, 0, 1182)
			require.NoError(t, err)
			require.True(t, applied)
			states, err := ListSenseNovaStates(channel.Id)
			require.NoError(t, err)
			for _, state := range states {
				assert.Equal(t, int64(1000), state.LastSuccessAt, "a probe is not successful real traffic")
				if state.Scope == scope {
					assert.Equal(t, SenseNovaUntested, state.State, "capacity still needs a real request")
					assert.Equal(t, "rate_limited", state.Reason)
					assert.Equal(t, 1, state.Failures)
					assert.Equal(t, int64(1001), state.LastFailureAt)
					assert.Equal(t, int64(1181), state.LastProbeAt)
				}
			}
		})
	}
}

func pendingSenseNovaRecovery(t *testing.T, scope string) (*Channel, *SenseNovaSnapshot, string, int64) {
	t.Helper()
	channel := setupSenseNovaTest(t)
	key, now := "test-account-a", time.Now().Unix()
	snapshot, err := SenseNovaKeySnapshot(channel.Id, key, "glm-5.2")
	require.NoError(t, err)
	applied, err := RecordSenseNovaSuccess(snapshot, key, now-200)
	require.NoError(t, err)
	require.True(t, applied)
	applied, err = RecordSenseNovaFailure(snapshot, key, scope, "rate_limited", false, 0, now-61)
	require.NoError(t, err)
	require.True(t, applied)
	probe, err := ClaimSenseNovaProbe(channel.Id, key, scope, now-1, false)
	require.NoError(t, err)
	applied, err = FinishSenseNovaProbe(probe, true, "", false, 0, now)
	require.NoError(t, err)
	require.True(t, applied)
	snapshot, err = SenseNovaKeySnapshot(channel.Id, key, "glm-5.2")
	require.NoError(t, err)
	return channel, snapshot, key, now
}

func TestSenseNovaRecoveryRequiresOwnedSuccess(t *testing.T) {
	channel, snapshot, key, now := pendingSenseNovaRecovery(t, "glm-5.2")
	applied, err := RecordSenseNovaSuccess(snapshot, key, now+1)
	require.NoError(t, err)
	assert.False(t, applied, "an unclaimed selection must not prove recovery")
	states, err := ListSenseNovaStates(channel.Id)
	require.NoError(t, err)
	for _, state := range states {
		if state.Scope == "glm-5.2" {
			assert.Equal(t, SenseNovaUntested, state.State)
			assert.Equal(t, 1, state.Failures)
		}
	}
	claim, err := ClaimSenseNovaRecovery(snapshot, key, now+1)
	require.NoError(t, err)
	require.True(t, claim.RecoveryLease)
	applied, err = RecordSenseNovaSuccess(claim, key, now+2)
	require.NoError(t, err)
	require.True(t, applied)
	require.NoError(t, ReleaseSenseNovaRecovery(context.Background(), claim, key))
	states, err = ListSenseNovaStates(channel.Id)
	require.NoError(t, err)
	for _, state := range states {
		assert.Equal(t, SenseNovaUsable, state.State)
		assert.Empty(t, state.Reason)
		assert.Zero(t, state.Failures)
		assert.Zero(t, state.LeaseUntil)
		assert.Equal(t, now+2, state.LastSuccessAt)
	}
}

func TestSenseNovaRecoveryLeaseSerializesAndKeepsBackoff(t *testing.T) {
	for _, scope := range []string{"", "glm-5.2"} {
		t.Run("scope="+scope, func(t *testing.T) {
			channel, snapshot, key, now := pendingSenseNovaRecovery(t, scope)
			claim, err := ClaimSenseNovaRecovery(snapshot, key, now)
			require.NoError(t, err)
			_, err = ClaimSenseNovaRecovery(snapshot, key, now)
			require.ErrorIs(t, err, ErrSenseNovaUnavailable, "only one competing selector may own recovery")
			_, err = SenseNovaKeySnapshot(channel.Id, key, "glm-5.2")
			require.ErrorIs(t, err, ErrSenseNovaUnavailable)
			_, err = ClaimSenseNovaProbe(channel.Id, key, scope, now+60, true)
			require.ErrorIs(t, err, ErrSenseNovaUnavailable, "probe must not race a real recovery request")
			renewed, err := RenewSenseNovaRecovery(context.Background(), claim, key, now+90)
			require.NoError(t, err)
			require.True(t, renewed)
			applied, err := RecordSenseNovaFailure(claim, key, scope, "rate_limited", false, 400, now+130)
			require.NoError(t, err)
			require.True(t, applied, "renewed owner remains valid after original expiry")
			require.NoError(t, ReleaseSenseNovaRecovery(context.Background(), claim, key))
			states, err := ListSenseNovaStates(channel.Id)
			require.NoError(t, err)
			for _, state := range states {
				assert.Zero(t, state.LeaseUntil, "all still-owned scopes must release after failure")
				assert.Equal(t, now-200, state.LastSuccessAt)
				if state.Scope == scope {
					assert.Equal(t, SenseNovaCooling, state.State)
					assert.Equal(t, 2, state.Failures)
					assert.Equal(t, int64(400), state.NextProbeAt-state.LastFailureAt, "preserve a longer provider Retry-After")
				}
			}
			renewed, err = RenewSenseNovaRecovery(context.Background(), claim, key, now+131)
			require.NoError(t, err)
			assert.False(t, renewed)
		})
	}
}

func TestSenseNovaRecoveryReleaseDoesNotAffectNewOwner(t *testing.T) {
	channel, snapshot, key, now := pendingSenseNovaRecovery(t, "glm-5.2")
	first, err := ClaimSenseNovaRecovery(snapshot, key, now)
	require.NoError(t, err)
	require.NoError(t, ReleaseSenseNovaRecovery(context.Background(), first, key))
	snapshot, err = SenseNovaKeySnapshot(channel.Id, key, "glm-5.2")
	require.NoError(t, err)
	second, err := ClaimSenseNovaRecovery(snapshot, key, now)
	require.NoError(t, err)
	require.NoError(t, ReleaseSenseNovaRecovery(context.Background(), first, key))
	renewed, err := RenewSenseNovaRecovery(context.Background(), second, key, now+1)
	require.NoError(t, err)
	assert.True(t, renewed)
	applied, err := RecordSenseNovaSuccess(first, key, now+2)
	require.NoError(t, err)
	assert.False(t, applied)
	require.NoError(t, ReleaseSenseNovaRecovery(context.Background(), second, key))
}

func TestSenseNovaRecoveryExpiredOwnerCannotRenewOrPublish(t *testing.T) {
	_, snapshot, key, now := pendingSenseNovaRecovery(t, "glm-5.2")
	claim, err := ClaimSenseNovaRecovery(snapshot, key, now)
	require.NoError(t, err)
	expired := now + SenseNovaRecoveryLeaseSeconds
	renewed, err := RenewSenseNovaRecovery(context.Background(), claim, key, expired)
	require.NoError(t, err)
	assert.False(t, renewed)
	applied, err := RecordSenseNovaSuccess(claim, key, expired)
	require.NoError(t, err)
	assert.False(t, applied)
	applied, err = RecordSenseNovaFailure(claim, key, "glm-5.2", "rate_limited", false, 0, expired)
	require.NoError(t, err)
	assert.False(t, applied)
	require.NoError(t, ReleaseSenseNovaRecovery(context.Background(), claim, key))
}

func TestSenseNovaProbeFailureCannotErasePendingRateHistory(t *testing.T) {
	channel, _, key, now := pendingSenseNovaRecovery(t, "glm-5.2")
	probe, err := ClaimSenseNovaProbe(channel.Id, key, "glm-5.2", now+60, true)
	require.NoError(t, err)
	applied, err := FinishSenseNovaProbe(probe, false, "upstream_unavailable", false, 0, now+61)
	require.NoError(t, err)
	require.True(t, applied)
	probe, err = ClaimSenseNovaProbe(channel.Id, key, "glm-5.2", now+361, false)
	require.NoError(t, err)
	applied, err = FinishSenseNovaProbe(probe, true, "", false, 0, now+362)
	require.NoError(t, err)
	require.True(t, applied)
	states, err := ListSenseNovaStates(channel.Id)
	require.NoError(t, err)
	for _, state := range states {
		if state.Scope == "glm-5.2" {
			assert.Equal(t, SenseNovaUntested, state.State)
			assert.Equal(t, "rate_limited", state.Reason)
			assert.Equal(t, 2, state.Failures)
		}
	}
}

func TestSenseNovaRequestRecoveryHonorsDeadlineAndFailureClass(t *testing.T) {
	for _, reason := range []string{"rate_limited", "quota_exhausted", "upstream_unavailable", "model_unavailable"} {
		t.Run(reason, func(t *testing.T) {
			channel := setupSenseNovaTest(t)
			key := "test-account-a"
			snapshot, err := SenseNovaKeySnapshot(channel.Id, key, "glm-5.2")
			require.NoError(t, err)
			applied, err := RecordSenseNovaFailure(snapshot, key, "glm-5.2", reason, false, 180, 1000)
			require.NoError(t, err)
			require.True(t, applied)
			_, err = SenseNovaRequestSnapshot(channel.Id, key, "glm-5.2", 1179)
			require.ErrorIs(t, err, ErrSenseNovaUnavailable)
			snapshot, err = SenseNovaRequestSnapshot(channel.Id, key, "glm-5.2", 1180)
			if reason != "rate_limited" {
				require.ErrorIs(t, err, ErrSenseNovaUnavailable, "other failure classes still require health recovery")
				return
			}
			require.NoError(t, err)
			require.True(t, snapshot.NeedsRecovery)
			states, err := ListSenseNovaStates(channel.Id)
			require.NoError(t, err)
			for _, state := range states {
				if state.Scope == "glm-5.2" {
					assert.Equal(t, SenseNovaCooling, state.State, "selection must not claim or restore the key")
					assert.Zero(t, state.LeaseUntil)
				}
			}
			_, err = ClaimSenseNovaRecovery(snapshot, key, 1179)
			require.ErrorIs(t, err, ErrSenseNovaUnavailable, "dispatch must recheck the deadline")
			claim, err := ClaimSenseNovaRecovery(snapshot, key, 1180)
			require.NoError(t, err)
			require.True(t, claim.RecoveryLease)
			_, err = ClaimSenseNovaProbe(channel.Id, key, "glm-5.2", 1180, false)
			require.ErrorIs(t, err, ErrSenseNovaUnavailable)
			require.NoError(t, ReleaseSenseNovaRecovery(context.Background(), claim, key))
		})
	}
}

func TestSenseNovaRequestRecoveryReclaimsExpiredLease(t *testing.T) {
	channel, snapshot, key, now := pendingSenseNovaRecovery(t, "")
	old, err := ClaimSenseNovaRecovery(snapshot, key, now)
	require.NoError(t, err)
	snapshot, err = SenseNovaRequestSnapshot(channel.Id, key, "glm-5.2", now+SenseNovaRecoveryLeaseSeconds)
	require.NoError(t, err)
	fresh, err := ClaimSenseNovaRecovery(snapshot, key, now+SenseNovaRecoveryLeaseSeconds)
	require.NoError(t, err)
	require.NoError(t, ReleaseSenseNovaRecovery(context.Background(), old, key))
	renewed, err := RenewSenseNovaRecovery(context.Background(), fresh, key, now+SenseNovaRecoveryLeaseSeconds+1)
	require.NoError(t, err)
	assert.True(t, renewed, "crashed owner's cleanup must not release the replacement")
	require.NoError(t, ReleaseSenseNovaRecovery(context.Background(), fresh, key))
}

func TestSenseNovaRateRecoveryDoesNotInheritHealthFailureBackoff(t *testing.T) {
	channel := setupSenseNovaTest(t)
	key := "test-account-a"
	snapshot, err := SenseNovaKeySnapshot(channel.Id, key, "glm-5.2")
	require.NoError(t, err)
	applied, err := RecordSenseNovaFailure(snapshot, key, "glm-5.2", "rate_limited", false, 0, 1000)
	require.NoError(t, err)
	require.True(t, applied)
	for attempt := 2; attempt <= 3; attempt++ {
		now := int64(1000 + (attempt-1)*60)
		snapshot, err = SenseNovaRequestSnapshot(channel.Id, key, "glm-5.2", now)
		require.NoError(t, err)
		claim, claimErr := ClaimSenseNovaRecovery(snapshot, key, now)
		require.NoError(t, claimErr)
		applied, err = RecordSenseNovaFailure(claim, key, "glm-5.2", "rate_limited", false, 0, now)
		require.NoError(t, err)
		require.True(t, applied)
		require.NoError(t, ReleaseSenseNovaRecovery(context.Background(), claim, key))
		states, stateErr := ListSenseNovaStates(channel.Id)
		require.NoError(t, stateErr)
		for _, state := range states {
			if state.Scope == "glm-5.2" {
				assert.Equal(t, attempt, state.Failures, "keep the real failure history")
				require.Equal(t, now+60, state.NextProbeAt, "temporary rate limiting must not become a five/fifteen minute health outage")
			}
		}
	}
	snapshot, err = SenseNovaRequestSnapshot(channel.Id, key, "glm-5.2", 1180)
	require.NoError(t, err)
	claim, err := ClaimSenseNovaRecovery(snapshot, key, 1180)
	require.NoError(t, err)
	applied, err = RecordSenseNovaFailure(claim, key, "glm-5.2", "rate_limited", false, 450, 1180)
	require.NoError(t, err)
	require.True(t, applied)
	require.NoError(t, ReleaseSenseNovaRecovery(context.Background(), claim, key))
	states, err := ListSenseNovaStates(channel.Id)
	require.NoError(t, err)
	for _, state := range states {
		if state.Scope == "glm-5.2" {
			assert.Equal(t, int64(1630), state.NextProbeAt, "a longer provider Retry-After remains authoritative")
			assert.Equal(t, 4, state.Failures)
		}
	}
}
