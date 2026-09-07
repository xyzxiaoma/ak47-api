package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSenseNovaRecoveryAdmissionOwnsAndReleasesLease(t *testing.T) {
	for _, enabled := range []string{"true", "false"} {
		t.Run("admission="+enabled, func(t *testing.T) {
			channel, _ := senseNovaAdmissionFixture(t)
			t.Setenv("SENSENOVA_ADMISSION_ENABLED", enabled)
			channel.Key = "fake-account-a"
			channel.ChannelInfo.MultiKeySize = 1
			require.NoError(t, model.DB.Save(channel).Error)
			now := time.Now().Unix()
			require.NoError(t, model.DB.Create(&model.SenseNovaKeyState{ChannelID: channel.Id,
				Fingerprint: model.SenseNovaFingerprint(channel.Key), Scope: "deepseek-v4-pro",
				State: model.SenseNovaUntested, Reason: "rate_limited", Failures: 1,
				LastSuccessAt: now - 200, LastFailureAt: now - 61, Version: 1}).Error)
			first := senseNovaAdmissionContext(t, channel)
			second := senseNovaAdmissionContext(t, channel)
			parent, cancel := context.WithCancel(first.Request.Context())
			defer cancel()
			first.Request = first.Request.WithContext(parent)
			require.Nil(t, AdmitSenseNovaAttempt(first))
			t.Cleanup(func() { FinishSenseNovaAdmission(first) })
			assert.True(t, getSenseNovaAttempt(first).snapshot.RecoveryLease)
			assert.NotNil(t, AdmitSenseNovaAttempt(second), "a competing request must not also verify recovery")
			FinishSenseNovaAdmission(second)
			cancel()
			FinishSenseNovaAdmission(first)
			assert.ErrorIs(t, first.Request.Context().Err(), context.Canceled)
			states, err := model.ListSenseNovaStates(channel.Id)
			require.NoError(t, err)
			for _, state := range states {
				assert.Zero(t, state.LeaseUntil)
				if state.Scope == "deepseek-v4-pro" {
					assert.Equal(t, model.SenseNovaUntested, state.State)
					assert.Equal(t, 1, state.Failures, "cancellation is not proof of recovery")
					assert.Equal(t, now-200, state.LastSuccessAt)
				}
			}
			third := senseNovaAdmissionContext(t, channel)
			require.Nil(t, AdmitSenseNovaAttempt(third), "an unsent canceled owner must not block the next request")
			t.Cleanup(func() { FinishSenseNovaAdmission(third) })
			RecordSenseNovaRelaySuccess(third)
			states, err = model.ListSenseNovaStates(channel.Id)
			require.NoError(t, err)
			for _, state := range states {
				assert.Equal(t, model.SenseNovaUsable, state.State)
				assert.Zero(t, state.Failures)
				assert.Zero(t, state.LeaseUntil)
				assert.GreaterOrEqual(t, state.LastSuccessAt, now)
			}
		})
	}
}

func TestSenseNovaRecoveryLeaseLossCancelsUpstream(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	key, now := channel.GetKeys()[0], time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.SenseNovaKeyState{ChannelID: channel.Id,
		Fingerprint: model.SenseNovaFingerprint(key), Scope: "deepseek-v4-pro",
		State: model.SenseNovaUntested, Reason: "rate_limited", Failures: 1, Version: 1}).Error)
	snapshot, err := model.SenseNovaKeySnapshot(channel.Id, key, "deepseek-v4-pro")
	require.NoError(t, err)
	claim, err := model.ClaimSenseNovaRecovery(snapshot, key, now)
	require.NoError(t, err)
	require.NoError(t, model.InvalidateSenseNovaKeys(channel.Id, []string{key}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		runSenseNovaRecoveryRenewal(ctx, cancel, claim, key, time.Millisecond)
	}()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Error("loss of recovery ownership must cancel the upstream context")
		cancel()
	}
	<-done
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}

func TestSenseNovaRecoveryRenewalStopsWhenDatabaseIsBlocked(t *testing.T) {
	for _, cancelRenewal := range []bool{true, false} {
		name := "database deadline"
		if cancelRenewal {
			name = "normal cleanup cancellation"
		}
		t.Run(name, func(t *testing.T) {
			channel, _ := senseNovaAdmissionFixture(t)
			key, now := channel.GetKeys()[0], time.Now().Unix()
			require.NoError(t, model.DB.Create(&model.SenseNovaKeyState{ChannelID: channel.Id,
				Fingerprint: model.SenseNovaFingerprint(key), Scope: "deepseek-v4-pro",
				State: model.SenseNovaUntested, Reason: "rate_limited", Failures: 1, Version: 1}).Error)
			snapshot, err := model.SenseNovaKeySnapshot(channel.Id, key, "deepseek-v4-pro")
			require.NoError(t, err)
			claim, err := model.ClaimSenseNovaRecovery(snapshot, key, now)
			require.NoError(t, err)
			sqlDB, err := model.DB.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			connection, err := sqlDB.Conn(context.Background())
			require.NoError(t, err)
			ctx, stop := context.WithCancel(context.Background())
			upstream, cancelUpstream := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				defer close(done)
				runSenseNovaRecoveryRenewal(ctx, cancelUpstream, claim, key, time.Millisecond)
			}()
			defer func() {
				stop()
				cancelUpstream()
				_ = connection.Close()
				<-done
			}()
			require.Eventually(t, func() bool { return sqlDB.Stats().WaitCount > 0 }, time.Second, time.Millisecond,
				"renewal must be waiting for the occupied database connection")
			if cancelRenewal {
				stop()
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("blocked database renewal must terminate on cancellation or its own deadline")
			}
			if cancelRenewal {
				assert.NoError(t, upstream.Err(), "stopping renewal for normal cleanup must not cancel a retry")
			} else {
				assert.ErrorIs(t, upstream.Err(), context.Canceled, "database renewal failure must cancel upstream work")
			}
		})
	}
}

func TestSenseNovaRecoveryCleanupBoundsBlockedDatabase(t *testing.T) {
	channel, c := senseNovaServiceFixture(t)
	t.Setenv("SENSENOVA_ADMISSION_ENABLED", "false")
	channel.Key = "fake-account-a"
	channel.ChannelInfo.MultiKeySize = 1
	require.NoError(t, model.DB.Save(channel).Error)
	require.NoError(t, model.DB.Create(&model.SenseNovaKeyState{ChannelID: channel.Id,
		Fingerprint: model.SenseNovaFingerprint(channel.Key), Scope: "glm-5.2",
		State: model.SenseNovaUntested, Reason: "rate_limited", Failures: 1, Version: 1}).Error)
	_, _, selectionErr := SelectSenseNovaKey(c, channel, "glm-5.2")
	require.Nil(t, selectionErr)
	require.Nil(t, AdmitSenseNovaAttempt(c))
	claim := getSenseNovaAttempt(c).snapshot
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	connection, err := sqlDB.Conn(context.Background())
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		FinishSenseNovaAdmission(c)
	}()
	defer func() {
		_ = connection.Close()
		<-done
	}()
	require.Eventually(t, func() bool { return sqlDB.Stats().WaitCount > 0 }, time.Second, time.Millisecond)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup must return when its database deadline expires")
	}
	require.NoError(t, connection.Close())
	states, err := model.ListSenseNovaStates(channel.Id)
	require.NoError(t, err)
	for _, state := range states {
		assert.Equal(t, claim.Versions[state.Scope], state.Version, "timed-out cleanup must not change ownership")
		assert.Positive(t, state.LeaseUntil, "unreleased lease remains bounded by its original expiry")
	}
	FinishSenseNovaAdmission(c)
}

func TestSenseNovaRecoveryDueTrafficDoesNotWaitForProbeScan(t *testing.T) {
	channel, c := senseNovaServiceFixture(t)
	t.Setenv("SENSENOVA_ADMISSION_ENABLED", "false")
	channel.Key = "fake-account-a"
	channel.ChannelInfo.MultiKeySize = 1
	require.NoError(t, model.DB.Save(channel).Error)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.SenseNovaKeyState{ChannelID: channel.Id,
		Fingerprint: model.SenseNovaFingerprint(channel.Key), Scope: "glm-5.2",
		State: model.SenseNovaCooling, Reason: "rate_limited", Failures: 2,
		NextProbeAt: now - 1, LastFailureAt: now - 301, Version: 1}).Error)
	_, _, selectionErr := SelectSenseNovaKey(c, channel, "glm-5.2")
	require.Nil(t, selectionErr, "expired rate cooldown should permit one real verifier without a tiny probe")
	require.Nil(t, AdmitSenseNovaAttempt(c))
	t.Cleanup(func() { FinishSenseNovaAdmission(c) })
	assert.True(t, getSenseNovaAttempt(c).snapshot.RecoveryLease)
	states, err := model.ListSenseNovaStates(channel.Id)
	require.NoError(t, err)
	for _, state := range states {
		if state.Scope == "glm-5.2" {
			assert.Zero(t, state.LastProbeAt, "no synthetic probe was needed")
			assert.Equal(t, 2, state.Failures)
			assert.Equal(t, model.SenseNovaUntested, state.State)
		}
	}
}
