package model

import "github.com/QuantumNous/new-api/common"

// SenseNovaRoutingEligibility is a read-only scheduling hint, not ownership or
// permission to dispatch. Admission must recheck the selected key with
// SenseNovaRequestSnapshot and ClaimSenseNovaRecovery after budget reservation.
type SenseNovaRoutingEligibility struct {
	Available     bool
	NeedsRecovery bool
}

// SenseNovaRoutingCandidates returns fingerprint-keyed hints for the current
// inventory in one read. Missing rows represent untested keys; evaluating a
// large pool must not create health rows or take a transaction for each key.
func SenseNovaRoutingCandidates(channel *Channel, name string, now int64) (map[string]SenseNovaRoutingEligibility, error) {
	if channel == nil || channel.Id <= 0 || !channel.SenseNovaPool || channel.Status != common.ChannelStatusEnabled || !IsSenseNovaModel(name) {
		return nil, ErrSenseNovaUnavailable
	}
	hints := make(map[string]SenseNovaRoutingEligibility)
	fingerprints := make([]string, 0)
	for key, status := range senseNovaManualKeyStates(channel) {
		fingerprint := SenseNovaFingerprint(key)
		hints[fingerprint] = SenseNovaRoutingEligibility{Available: status == common.ChannelStatusEnabled}
		if status == common.ChannelStatusEnabled {
			fingerprints = append(fingerprints, fingerprint)
		}
	}
	if len(fingerprints) == 0 {
		return hints, nil
	}
	var states []SenseNovaKeyState
	if err := DB.Select("fingerprint", "scope", "state", "reason", "next_probe_at", "lease_until").
		Where("channel_id = ? AND fingerprint IN ? AND scope IN ?", channel.Id, fingerprints, []string{"", name}).Find(&states).Error; err != nil {
		return nil, err
	}
	for _, state := range states {
		hint := hints[state.Fingerprint]
		if !hint.Available {
			continue
		}
		due := senseNovaRecoveryDue(&state, now)
		if state.State != SenseNovaUntested && state.State != SenseNovaUsable && !due {
			hints[state.Fingerprint] = SenseNovaRoutingEligibility{}
			continue
		}
		if senseNovaNeedsRecovery(&state) || due {
			if state.LeaseUntil > now {
				hints[state.Fingerprint] = SenseNovaRoutingEligibility{}
				continue
			}
			hint.NeedsRecovery = true
		}
		hints[state.Fingerprint] = hint
	}
	return hints, nil
}
