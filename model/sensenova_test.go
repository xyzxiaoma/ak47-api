package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSenseNovaTest(t *testing.T) *Channel {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Channel{}, &SenseNovaKeyState{}))
	c := &Channel{Key: "test-account-a\ntest-account-b", Status: common.ChannelStatusEnabled, SenseNovaPool: true,
		ChannelInfo: ChannelInfo{IsMultiKey: true}}
	require.NoError(t, DB.Create(c).Error)
	t.Cleanup(func() {
		DB.Where("channel_id = ?", c.Id).Delete(&SenseNovaKeyState{})
		DB.Delete(c)
	})
	return c
}

func TestSenseNovaCooldownRecoveryAndSharedScope(t *testing.T) {
	c := setupSenseNovaTest(t)
	key := "test-account-a"
	snapshot, err := SenseNovaKeySnapshot(c.Id, key, "glm-5.2")
	require.NoError(t, err)
	ok, err := RecordSenseNovaFailure(snapshot, key, "", "quota_exhausted", false, 0, 1000)
	require.NoError(t, err)
	require.True(t, ok)
	_, err = SenseNovaKeySnapshot(c.Id, key, "kimi-k3")
	require.ErrorIs(t, err, ErrSenseNovaUnavailable)
	_, err = SenseNovaKeySnapshot(c.Id, "test-account-b", "kimi-k3")
	require.NoError(t, err)
	_, err = ClaimSenseNovaProbe(c.Id, key, "", 1059, false)
	require.ErrorIs(t, err, ErrSenseNovaUnavailable)
	claim, err := ClaimSenseNovaProbe(c.Id, key, "", 1060, false)
	require.NoError(t, err)
	_, err = ClaimSenseNovaProbe(c.Id, key, "", 1061, false)
	require.ErrorIs(t, err, ErrSenseNovaUnavailable)
	ok, err = FinishSenseNovaProbe(claim, false, "quota_exhausted", false, 0, 1061)
	require.NoError(t, err)
	require.True(t, ok)
	states, err := ListDueSenseNovaProbes(1361, 10)
	require.NoError(t, err)
	require.NotEmpty(t, states)
	claim, err = ClaimSenseNovaProbe(c.Id, key, "", 1361, false)
	require.NoError(t, err)
	ok, err = FinishSenseNovaProbe(claim, false, "quota_exhausted", false, 0, 1362)
	require.NoError(t, err)
	require.True(t, ok)
	claim, err = ClaimSenseNovaProbe(c.Id, key, "", 2262, false)
	require.NoError(t, err)
	ok, err = FinishSenseNovaProbe(claim, true, "", false, 0, 2263)
	require.NoError(t, err)
	require.True(t, ok)
	_, err = SenseNovaKeySnapshot(c.Id, key, "kimi-k3")
	require.NoError(t, err)
	states, err = ListSenseNovaStates(c.Id)
	require.NoError(t, err)
	for _, state := range states {
		if state.Scope == "" && state.Fingerprint == SenseNovaFingerprint(key) {
			assert.Equal(t, SenseNovaUsable, state.State)
			assert.Equal(t, int64(2263), state.LastSuccessAt)
			assert.Zero(t, state.NextProbeAt)
		}
	}
}

func TestSenseNovaStaleResultsAndAdministratorPrecedence(t *testing.T) {
	c := setupSenseNovaTest(t)
	key := "test-account-a"
	snapshot, err := SenseNovaKeySnapshot(c.Id, key, "glm-5.2")
	require.NoError(t, err)
	ok, err := RecordSenseNovaFailure(snapshot, key, "", "rate_limited", false, 0, 1000)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = RecordSenseNovaSuccess(snapshot, key, 1001)
	require.NoError(t, err)
	assert.False(t, ok, "a stale in-flight success cannot undo a newer failure")
	claim, err := ClaimSenseNovaProbe(c.Id, key, "", 1060, false)
	require.NoError(t, err)
	c.ChannelInfo.MultiKeyStatusList = map[int]int{0: common.ChannelStatusManuallyDisabled}
	require.NoError(t, DB.Model(c).Update("channel_info", c.ChannelInfo).Error)
	ok, err = FinishSenseNovaProbe(claim, true, "", false, 0, 1061)
	require.NoError(t, err)
	assert.False(t, ok)
	require.NoError(t, InvalidateSenseNovaKeys(c.Id, []string{key}))
	c.ChannelInfo.MultiKeyStatusList = nil
	require.NoError(t, DB.Model(c).Update("channel_info", c.ChannelInfo).Error)
	ok, err = FinishSenseNovaProbe(claim, true, "", false, 0, 1062)
	require.NoError(t, err)
	assert.False(t, ok, "disable/enable cannot revive an old claim")
}

func TestSenseNovaModelIsolationAndKeyReplacement(t *testing.T) {
	c := setupSenseNovaTest(t)
	key := "test-account-a"
	snapshot, err := SenseNovaKeySnapshot(c.Id, key, "glm-5.2")
	require.NoError(t, err)
	ok, err := RecordSenseNovaFailure(snapshot, key, "glm-5.2", "model_unavailable", false, 0, 1000)
	require.NoError(t, err)
	require.True(t, ok)
	_, err = SenseNovaKeySnapshot(c.Id, key, "kimi-k3")
	require.NoError(t, err)
	_, err = SenseNovaKeySnapshot(c.Id, key, "glm-5.2")
	require.ErrorIs(t, err, ErrSenseNovaUnavailable)
	require.NoError(t, DB.Model(c).Update("key", "test-account-b\ntest-account-a").Error)
	_, err = SenseNovaKeySnapshot(c.Id, key, "glm-5.2")
	require.ErrorIs(t, err, ErrSenseNovaUnavailable, "cooldown follows the secret across reorder")
	claim, err := ClaimSenseNovaProbe(c.Id, key, "glm-5.2", 1060, false)
	require.NoError(t, err)
	require.NoError(t, InvalidateSenseNovaKeys(c.Id, []string{key}))
	require.NoError(t, DB.Model(c).Update("key", "test-account-b\ntest-account-c").Error)
	ok, err = FinishSenseNovaProbe(claim, true, "", false, 0, 1061)
	require.NoError(t, err)
	assert.False(t, ok)
	_, err = SenseNovaKeySnapshot(c.Id, "test-account-c", "glm-5.2")
	require.NoError(t, err)
	require.NoError(t, DB.Model(c).Update("key", "test-account-b\ntest-account-a").Error)
	ok, err = FinishSenseNovaProbe(claim, true, "", false, 0, 1062)
	require.NoError(t, err)
	assert.False(t, ok, "remove and re-add must not resurrect a stale probe")
}

func TestSenseNovaConcurrentSuccessCannotSuppressQuotaFailure(t *testing.T) {
	c := setupSenseNovaTest(t)
	key := "test-account-a"
	first, err := SenseNovaKeySnapshot(c.Id, key, "glm-5.2")
	require.NoError(t, err)
	second, err := SenseNovaKeySnapshot(c.Id, key, "glm-5.2")
	require.NoError(t, err)
	ok, err := RecordSenseNovaSuccess(first, key, 1000)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = RecordSenseNovaFailure(second, key, "", "quota_exhausted", false, 0, 1001)
	require.NoError(t, err)
	require.True(t, ok, "a concurrent healthy completion must not hide quota exhaustion")
	_, err = SenseNovaKeySnapshot(c.Id, key, "kimi-k3")
	require.ErrorIs(t, err, ErrSenseNovaUnavailable)
}

func TestSenseNovaChannelUpdatePreservesCooldownAndInvalidatesClaims(t *testing.T) {
	c := setupSenseNovaTest(t)
	key := "test-account-a"
	snapshot, err := SenseNovaKeySnapshot(c.Id, key, "glm-5.2")
	require.NoError(t, err)
	ok, err := RecordSenseNovaFailure(snapshot, key, "", "quota_exhausted", false, 0, 1000)
	require.NoError(t, err)
	require.True(t, ok)
	claim, err := ClaimSenseNovaProbe(c.Id, key, "", 1060, false)
	require.NoError(t, err)
	c.ChannelInfo.MultiKeyStatusList = map[int]int{0: common.ChannelStatusManuallyDisabled}
	require.NoError(t, c.Update())
	c.ChannelInfo.MultiKeyStatusList = map[int]int{}
	require.NoError(t, c.Update())
	ok, err = FinishSenseNovaProbe(claim, true, "", false, 0, 1061)
	require.NoError(t, err)
	assert.False(t, ok)
	_, err = SenseNovaKeySnapshot(c.Id, key, "kimi-k3")
	require.ErrorIs(t, err, ErrSenseNovaUnavailable, "manual enable does not claim exhausted quota recovered")
	c.SenseNovaPool = false
	require.NoError(t, c.Update())
	stored, err := GetChannelById(c.Id, true)
	require.NoError(t, err)
	assert.False(t, stored.SenseNovaPool, "explicit false survives GORM zero-value filtering")
}

func TestSenseNovaDueListSkipsDisabledKeysAndExpiredClaim(t *testing.T) {
	c := setupSenseNovaTest(t)
	for _, key := range c.GetKeys() {
		snapshot, err := SenseNovaKeySnapshot(c.Id, key, "glm-5.2")
		require.NoError(t, err)
		ok, err := RecordSenseNovaFailure(snapshot, key, "", "quota_exhausted", false, 0, 1000)
		require.NoError(t, err)
		require.True(t, ok)
	}
	c.ChannelInfo.MultiKeyStatusList = map[int]int{0: common.ChannelStatusManuallyDisabled}
	require.NoError(t, c.Update())
	due, err := ListDueSenseNovaProbes(1060, 1)
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, SenseNovaFingerprint("test-account-b"), due[0].Fingerprint)
	claim, err := ClaimSenseNovaProbe(c.Id, "test-account-b", "", 1060, false)
	require.NoError(t, err)
	ok, err := FinishSenseNovaProbe(claim, true, "", false, 0, 1120)
	require.NoError(t, err)
	assert.False(t, ok, "expired results cannot restore eligibility")
	next, err := ClaimSenseNovaProbe(c.Id, "test-account-b", "", 1120, false)
	require.NoError(t, err)
	assert.Greater(t, next.Snapshot.Versions[""], claim.Snapshot.Versions[""])
}

func TestSenseNovaModelProbeAuthenticationFailureInvalidatesAccount(t *testing.T) {
	c := setupSenseNovaTest(t)
	key := "test-account-a"
	snapshot, err := SenseNovaKeySnapshot(c.Id, key, "glm-5.2")
	require.NoError(t, err)
	ok, err := RecordSenseNovaFailure(snapshot, key, "glm-5.2", "model_unavailable", false, 0, 1000)
	require.NoError(t, err)
	require.True(t, ok)
	claim, err := ClaimSenseNovaProbe(c.Id, key, "glm-5.2", 1060, false)
	require.NoError(t, err)
	ok, err = FinishSenseNovaProbe(claim, false, "authentication_failed", true, 0, 1061)
	require.NoError(t, err)
	require.True(t, ok)
	_, err = SenseNovaKeySnapshot(c.Id, key, "kimi-k3")
	require.ErrorIs(t, err, ErrSenseNovaUnavailable)
	due, err := ListDueSenseNovaProbes(2000, 10)
	require.NoError(t, err)
	assert.Empty(t, due, "model cooldowns beneath an invalid account are not probed")
	_, err = ClaimSenseNovaProbe(c.Id, key, "", 2000, true)
	require.ErrorIs(t, err, ErrSenseNovaUnavailable, "invalid credentials require operator attention")
}

func TestSenseNovaSuccessfulTrafficPreservesOtherModelProbeLease(t *testing.T) {
	c := setupSenseNovaTest(t)
	key := "test-account-a"
	snapshot, err := SenseNovaKeySnapshot(c.Id, key, "glm-5.2")
	require.NoError(t, err)
	ok, err := RecordSenseNovaFailure(snapshot, key, "glm-5.2", "model_unavailable", false, 0, 1000)
	require.NoError(t, err)
	require.True(t, ok)
	claim, err := ClaimSenseNovaProbe(c.Id, key, "glm-5.2", 1060, false)
	require.NoError(t, err)
	snapshot, err = SenseNovaKeySnapshot(c.Id, key, "kimi-k3")
	require.NoError(t, err)
	ok, err = RecordSenseNovaSuccess(snapshot, key, 1061)
	require.NoError(t, err)
	require.True(t, ok)
	_, err = ClaimSenseNovaProbe(c.Id, key, "kimi-k3", 1062, true)
	require.ErrorIs(t, err, ErrSenseNovaUnavailable, "one account cannot have overlapping model probes")
	ok, err = FinishSenseNovaProbe(claim, true, "", false, 0, 1063)
	require.NoError(t, err)
	require.True(t, ok, "unrelated successful traffic must not invalidate the owned probe")
}

func TestSenseNovaInitialModelProbeMarksAccountUsable(t *testing.T) {
	c := setupSenseNovaTest(t)
	key := "test-account-a"
	claim, err := ClaimSenseNovaProbe(c.Id, key, "glm-5.2", 1000, true)
	require.NoError(t, err)
	ok, err := FinishSenseNovaProbe(claim, true, "", false, 0, 1001)
	require.NoError(t, err)
	require.True(t, ok)
	states, err := ListSenseNovaStates(c.Id)
	require.NoError(t, err)
	require.Len(t, states, 2)
	for _, state := range states {
		assert.Equal(t, SenseNovaUsable, state.State, "both the model and account have successful health evidence")
		assert.Equal(t, int64(1001), state.LastSuccessAt)
		assert.Zero(t, state.LeaseUntil)
	}
	_, err = SenseNovaKeySnapshot(c.Id, key, "kimi-k3")
	require.NoError(t, err)
}

func TestSenseNovaManualProbeHonorsCoolingDeadline(t *testing.T) {
	for _, scope := range []string{"", "glm-5.2"} {
		t.Run("scope="+scope, func(t *testing.T) {
			channel := setupSenseNovaTest(t)
			key := "test-account-a"
			snapshot, err := SenseNovaKeySnapshot(channel.Id, key, "glm-5.2")
			require.NoError(t, err)
			applied, err := RecordSenseNovaFailure(snapshot, key, scope, "rate_limited", false, 180, 1000)
			require.NoError(t, err)
			require.True(t, applied)
			_, err = ClaimSenseNovaProbe(channel.Id, key, scope, 1179, true)
			require.ErrorIs(t, err, ErrSenseNovaUnavailable, "manual testing must honor the same Retry-After as automatic recovery")
			_, err = SenseNovaKeySnapshot(channel.Id, key, "glm-5.2")
			require.ErrorIs(t, err, ErrSenseNovaUnavailable)
			claim, err := ClaimSenseNovaProbe(channel.Id, key, scope, 1180, true)
			require.NoError(t, err)
			applied, err = FinishSenseNovaProbe(claim, true, "", false, 0, 1181)
			require.NoError(t, err)
			require.True(t, applied)
			_, err = SenseNovaKeySnapshot(channel.Id, key, "glm-5.2")
			assert.NoError(t, err, "a due successful manual probe still restores routing")
		})
	}
}
