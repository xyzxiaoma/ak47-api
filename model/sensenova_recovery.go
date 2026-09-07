package model

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

const SenseNovaRecoveryLeaseSeconds int64 = 120

func senseNovaNeedsRecovery(state *SenseNovaKeyState) bool {
	return state.State == SenseNovaUntested && state.Reason == "rate_limited"
}

func senseNovaRecoveryDue(state *SenseNovaKeyState, now int64) bool {
	return state.State == SenseNovaCooling && state.Reason == "rate_limited" && state.NextProbeAt <= now
}

// ClaimSenseNovaRecovery is called only after capacity admission. Selection is
// not ownership: one actual request must verify that a tiny probe's success
// also applies to a customer-sized payload. The account lock serializes claims
// across models, workers and probe scheduling without a new database column.
func ClaimSenseNovaRecovery(snapshot *SenseNovaSnapshot, key string, now int64) (*SenseNovaSnapshot, error) {
	var claim *SenseNovaSnapshot
	err := DB.Transaction(func(tx *gorm.DB) error {
		states, err := senseNovaMatchSnapshot(tx, snapshot, key)
		if err != nil {
			return err
		}
		pending := false
		for _, state := range states {
			due := senseNovaRecoveryDue(state, now)
			if state.State != SenseNovaUntested && state.State != SenseNovaUsable && !due {
				return ErrSenseNovaUnavailable
			}
			pending = pending || senseNovaNeedsRecovery(state) || due
		}
		if !pending {
			claim = snapshot
			return nil
		}
		claim = &SenseNovaSnapshot{ChannelID: snapshot.ChannelID, Fingerprint: snapshot.Fingerprint,
			Versions: make(map[string]int64), NeedsRecovery: true, RecoveryLease: true}
		for scope, state := range states {
			if state.LeaseUntil > now {
				return ErrSenseNovaUnavailable
			}
			if senseNovaRecoveryDue(state, now) {
				state.State, state.NextProbeAt = SenseNovaUntested, 0
			}
			state.LeaseUntil = now + SenseNovaRecoveryLeaseSeconds
			state.Version++
			if err := tx.Save(state).Error; err != nil {
				return err
			}
			claim.Versions[scope] = state.Version
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claim, nil
}

// RenewSenseNovaRecovery never revives an expired or superseded owner.
func RenewSenseNovaRecovery(ctx context.Context, snapshot *SenseNovaSnapshot, key string, now int64) (bool, error) {
	if snapshot == nil || !snapshot.RecoveryLease {
		return false, nil
	}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		states, err := senseNovaMatchSnapshot(tx, snapshot, key)
		if err != nil {
			return err
		}
		for _, state := range states {
			if state.LeaseUntil <= now {
				return ErrSenseNovaUnavailable
			}
			state.LeaseUntil = now + SenseNovaRecoveryLeaseSeconds
			if err := tx.Save(state).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, ErrSenseNovaUnavailable) {
		return false, nil
	}
	return err == nil, err
}

// ReleaseSenseNovaRecovery releases each still-owned generation independently:
// a failed request may have already advanced just its model or account row.
func ReleaseSenseNovaRecovery(ctx context.Context, snapshot *SenseNovaSnapshot, key string) error {
	if snapshot == nil || !snapshot.RecoveryLease {
		return nil
	}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if snapshot.Fingerprint != SenseNovaFingerprint(key) {
			return ErrSenseNovaUnavailable
		}
		if _, err := senseNovaLiveChannel(tx, snapshot.ChannelID, key); err != nil {
			return err
		}
		for scope, version := range snapshot.Versions {
			if err := tx.Model(&SenseNovaKeyState{}).
				Where("channel_id = ? AND fingerprint = ? AND scope = ? AND version = ? AND lease_until > ?", snapshot.ChannelID, snapshot.Fingerprint, scope, version, 0).
				Updates(map[string]interface{}{"lease_until": 0, "version": gorm.Expr("version + ?", 1)}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, ErrSenseNovaUnavailable) {
		return nil
	}
	return err
}
