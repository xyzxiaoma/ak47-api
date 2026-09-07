package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SenseNovaUntested = "untested"
	SenseNovaUsable   = "usable"
	SenseNovaCooling  = "cooling"
	SenseNovaInvalid  = "invalid"
)

var ErrSenseNovaUnavailable = errors.New("SenseNova key is unavailable")

var AllowedSenseNovaModels = []string{"deepseek-v4-pro", "deepseek-v4-flash", "glm-5.2", "kimi-k3"}

func IsSenseNovaModel(name string) bool {
	for _, model := range AllowedSenseNovaModels {
		if name == model {
			return true
		}
	}
	return false
}

// SenseNovaKeyState never contains a credential. Empty Scope denotes the shared
// account pool; model scopes restrict only the corresponding model.
type SenseNovaKeyState struct {
	ID            int64  `json:"-" gorm:"primaryKey"`
	ChannelID     int    `json:"channel_id" gorm:"uniqueIndex:sensenova_identity,priority:1"`
	Fingerprint   string `json:"fingerprint" gorm:"type:varchar(64);uniqueIndex:sensenova_identity,priority:2"`
	Scope         string `json:"scope" gorm:"type:varchar(64);uniqueIndex:sensenova_identity,priority:3"`
	State         string `json:"state" gorm:"type:varchar(16);index:sensenova_due,priority:1"`
	Reason        string `json:"reason" gorm:"type:varchar(64)"`
	LastSuccessAt int64  `json:"last_success_at"`
	LastFailureAt int64  `json:"last_failure_at"`
	LastProbeAt   int64  `json:"last_probe_at"`
	NextProbeAt   int64  `json:"next_probe_at" gorm:"index:sensenova_due,priority:2"`
	Failures      int    `json:"failures"`
	Version       int64  `json:"-"`
	LeaseUntil    int64  `json:"-"`
}

type SenseNovaSnapshot struct {
	ChannelID   int
	Fingerprint string
	Versions    map[string]int64
}

type SenseNovaProbeClaim struct {
	Snapshot   *SenseNovaSnapshot
	Scope      string
	Key        string   `json:"-"`
	Channel    *Channel `json:"-"`
	LeaseUntil int64
}

func SenseNovaFingerprint(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

// All state transactions lock the channel first, giving every SQL dialect the
// same lock order and serializing edits with claims and request outcomes.
func senseNovaLiveChannel(tx *gorm.DB, channelID int, key string) (*Channel, error) {
	var channel Channel
	if err := lockForUpdate(tx).First(&channel, channelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSenseNovaUnavailable
		}
		return nil, err
	}
	if !channel.SenseNovaPool || channel.Status != common.ChannelStatusEnabled {
		return nil, ErrSenseNovaUnavailable
	}
	for index, configured := range channel.GetKeys() {
		if configured != key {
			continue
		}
		status, set := channel.ChannelInfo.MultiKeyStatusList[index]
		if set && status != common.ChannelStatusEnabled {
			return nil, ErrSenseNovaUnavailable
		}
		return &channel, nil
	}
	return nil, ErrSenseNovaUnavailable
}

func senseNovaState(tx *gorm.DB, channelID int, fingerprint, scope string) (*SenseNovaKeyState, error) {
	state := SenseNovaKeyState{ChannelID: channelID, Fingerprint: fingerprint, Scope: scope, State: SenseNovaUntested, Version: 1}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return nil, err
	}
	state.ID = 0
	err := lockForUpdate(tx).Where("channel_id = ? AND fingerprint = ? AND scope = ?", channelID, fingerprint, scope).First(&state).Error
	return &state, err
}

func senseNovaReadSnapshot(tx *gorm.DB, channelID int, key, scope string) (*SenseNovaSnapshot, map[string]*SenseNovaKeyState, error) {
	snapshot := &SenseNovaSnapshot{ChannelID: channelID, Fingerprint: SenseNovaFingerprint(key), Versions: make(map[string]int64)}
	states := make(map[string]*SenseNovaKeyState)
	scopes := []string{""}
	if scope != "" {
		scopes = append(scopes, scope)
	}
	for _, item := range scopes {
		state, err := senseNovaState(tx, channelID, snapshot.Fingerprint, item)
		if err != nil {
			return nil, nil, err
		}
		states[item] = state
		snapshot.Versions[item] = state.Version
	}
	return snapshot, states, nil
}

func SenseNovaKeySnapshot(channelID int, key, model string) (*SenseNovaSnapshot, error) {
	if !IsSenseNovaModel(model) {
		return nil, ErrSenseNovaUnavailable
	}
	var snapshot *SenseNovaSnapshot
	err := DB.Transaction(func(tx *gorm.DB) error {
		if _, err := senseNovaLiveChannel(tx, channelID, key); err != nil {
			return err
		}
		var states map[string]*SenseNovaKeyState
		var err error
		snapshot, states, err = senseNovaReadSnapshot(tx, channelID, key, model)
		if err != nil {
			return err
		}
		for _, state := range states {
			if state.State != SenseNovaUntested && state.State != SenseNovaUsable {
				return ErrSenseNovaUnavailable
			}
		}
		return nil
	})
	return snapshot, err
}

// A result must match both the account and model versions selected by its
// request. This prevents old successes from erasing more recent cooldowns.
func senseNovaMatchSnapshot(tx *gorm.DB, snapshot *SenseNovaSnapshot, key string) (map[string]*SenseNovaKeyState, error) {
	if snapshot == nil || snapshot.Fingerprint != SenseNovaFingerprint(key) {
		return nil, ErrSenseNovaUnavailable
	}
	if _, exists := snapshot.Versions[""]; !exists {
		return nil, ErrSenseNovaUnavailable
	}
	if _, err := senseNovaLiveChannel(tx, snapshot.ChannelID, key); err != nil {
		return nil, err
	}
	states := make(map[string]*SenseNovaKeyState)
	for scope, version := range snapshot.Versions {
		state, err := senseNovaState(tx, snapshot.ChannelID, snapshot.Fingerprint, scope)
		if err != nil {
			return nil, err
		}
		if state.Version != version {
			return nil, ErrSenseNovaUnavailable
		}
		states[scope] = state
	}
	return states, nil
}

func senseNovaApplyFailure(tx *gorm.DB, state *SenseNovaKeyState, reason string, invalid bool, retryAfter, now int64) error {
	state.State = SenseNovaCooling
	// Reasons are controlled codes, never upstream messages or credentials.
	switch reason {
	case "quota_exhausted", "rate_limited", "model_unavailable", "authentication_failed", "upstream_unavailable", "probe_failed":
		state.Reason = reason
	default:
		state.Reason = "upstream_unavailable"
	}
	state.Failures++
	delay := int64(60)
	if state.Failures == 2 {
		delay = 300
	}
	if state.Failures >= 3 {
		delay = 900
	}
	if retryAfter > delay {
		if retryAfter > 86400 {
			retryAfter = 86400
		}
		delay = retryAfter
	}
	state.NextProbeAt = now + delay
	if invalid {
		state.State = SenseNovaInvalid
		state.NextProbeAt = 0
	}
	state.LastFailureAt = now
	state.LeaseUntil = 0
	state.Version++
	return tx.Save(state).Error
}

func RecordSenseNovaFailure(snapshot *SenseNovaSnapshot, key, scope, reason string, invalid bool, retryAfter, now int64) (bool, error) {
	applied := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		states, err := senseNovaMatchSnapshot(tx, snapshot, key)
		if err != nil {
			return err
		}
		state, ok := states[scope]
		if !ok {
			return ErrSenseNovaUnavailable
		}
		if err := senseNovaApplyFailure(tx, state, reason, invalid, retryAfter, now); err != nil {
			return err
		}
		applied = true
		return nil
	})
	if errors.Is(err, ErrSenseNovaUnavailable) {
		err = nil
	}
	return applied, err
}

func RecordSenseNovaSuccess(snapshot *SenseNovaSnapshot, key string, now int64) (bool, error) {
	applied := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		states, err := senseNovaMatchSnapshot(tx, snapshot, key)
		if err != nil {
			return err
		}
		for _, state := range states {
			if state.State == SenseNovaCooling || state.State == SenseNovaInvalid {
				return ErrSenseNovaUnavailable
			}
			state.State, state.Reason = SenseNovaUsable, ""
			if now > state.LastSuccessAt {
				state.LastSuccessAt = now
			}
			// Successful traffic in another model does not own the account's
			// in-flight probe lease. Releasing it here permits duplicate probes.
			state.NextProbeAt, state.Failures = 0, 0
			if err := tx.Save(state).Error; err != nil {
				return err
			}
		}
		applied = true
		return nil
	})
	if errors.Is(err, ErrSenseNovaUnavailable) {
		err = nil
	}
	return applied, err
}

func ClaimSenseNovaProbe(channelID int, key, scope string, now int64, force bool) (*SenseNovaProbeClaim, error) {
	if scope != "" && !IsSenseNovaModel(scope) {
		return nil, ErrSenseNovaUnavailable
	}
	var claim *SenseNovaProbeClaim
	err := DB.Transaction(func(tx *gorm.DB) error {
		channel, err := senseNovaLiveChannel(tx, channelID, key)
		if err != nil {
			return err
		}
		snapshot, states, err := senseNovaReadSnapshot(tx, channelID, key, scope)
		if err != nil {
			return err
		}
		global, target := states[""], states[scope]
		if global.State == SenseNovaInvalid || global.LeaseUntil > now || target.State == SenseNovaInvalid || target.LeaseUntil > now {
			return ErrSenseNovaUnavailable
		}
		if scope != "" && global.State == SenseNovaCooling {
			return ErrSenseNovaUnavailable
		}
		// A manual probe is not permission to bypass a provider cooldown.
		// Otherwise a tiny successful probe could reopen this key too early.
		if target.State == SenseNovaCooling && target.NextProbeAt > now {
			return ErrSenseNovaUnavailable
		}
		if force {
			if target.LastProbeAt > 0 && now-target.LastProbeAt < 60 {
				return ErrSenseNovaUnavailable
			}
		} else if target.State != SenseNovaCooling || target.NextProbeAt > now {
			return ErrSenseNovaUnavailable
		}
		lease := now + 60
		for item, state := range states {
			state.LeaseUntil = lease
			state.Version++
			if item == scope {
				state.LastProbeAt = now
			}
			if err := tx.Save(state).Error; err != nil {
				return err
			}
			snapshot.Versions[item] = state.Version
		}
		claim = &SenseNovaProbeClaim{Snapshot: snapshot, Scope: scope, Key: key, Channel: channel, LeaseUntil: lease}
		return nil
	})
	return claim, err
}

func FinishSenseNovaProbe(claim *SenseNovaProbeClaim, success bool, reason string, invalid bool, retryAfter, now int64) (bool, error) {
	if claim == nil {
		return false, nil
	}
	applied := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		states, err := senseNovaMatchSnapshot(tx, claim.Snapshot, claim.Key)
		if err != nil {
			return err
		}
		target, exists := states[claim.Scope]
		if !exists || target.LeaseUntil != claim.LeaseUntil || now >= claim.LeaseUntil {
			return ErrSenseNovaUnavailable
		}
		resultScope := claim.Scope
		if !success && (invalid || reason == "quota_exhausted") {
			resultScope, target = "", states[""]
		}
		if !success && resultScope == "" && reason == "model_unavailable" {
			// A model probe cannot prove that the shared account pool recovered.
			reason = "probe_failed"
		}
		for scope, state := range states {
			if scope == resultScope {
				continue
			}
			if success && scope == "" && (state.State == SenseNovaUntested || state.State == SenseNovaUsable) {
				state.State, state.Reason = SenseNovaUsable, ""
				if now > state.LastSuccessAt {
					state.LastSuccessAt = now
				}
			}
			state.LeaseUntil = 0
			state.Version++
			if err := tx.Save(state).Error; err != nil {
				return err
			}
		}
		if success {
			target.State, target.Reason = SenseNovaUsable, ""
			target.LastSuccessAt, target.NextProbeAt, target.LeaseUntil, target.Failures = now, 0, 0, 0
			target.Version++
			err = tx.Save(target).Error
		} else {
			err = senseNovaApplyFailure(tx, target, reason, invalid, retryAfter, now)
		}
		if err != nil {
			return err
		}
		applied = true
		return nil
	})
	if errors.Is(err, ErrSenseNovaUnavailable) {
		err = nil
	}
	return applied, err
}

func ListSenseNovaStates(channelID int) ([]SenseNovaKeyState, error) {
	states := make([]SenseNovaKeyState, 0)
	err := DB.Where("channel_id = ?", channelID).Order("id ASC").Find(&states).Error
	return states, err
}

func ListDueSenseNovaProbes(now int64, limit int) ([]SenseNovaKeyState, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	states := make([]SenseNovaKeyState, 0)
	channels := make(map[int]*Channel)
	accountStates := make(map[int]map[string]SenseNovaKeyState)
	for offset := 0; ; offset += 100 {
		var candidates []SenseNovaKeyState
		activeChannels := DB.Model(&Channel{}).Select("id").Where("sensenova_pool = ? AND status = ?", true, common.ChannelStatusEnabled)
		err := DB.Where("state = ? AND next_probe_at <= ? AND lease_until <= ?", SenseNovaCooling, now, now).
			Where("channel_id IN (?)", activeChannels).Order("next_probe_at ASC, id ASC").Offset(offset).Limit(100).Find(&candidates).Error
		if err != nil {
			return nil, err
		}
		for _, candidate := range candidates {
			channel, exists := channels[candidate.ChannelID]
			if !exists {
				channel, err = GetChannelById(candidate.ChannelID, true)
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				if err != nil {
					return nil, err
				}
				channels[candidate.ChannelID] = channel
				var globals []SenseNovaKeyState
				if err := DB.Where("channel_id = ? AND scope = ?", channel.Id, "").Find(&globals).Error; err != nil {
					return nil, err
				}
				accountStates[channel.Id] = make(map[string]SenseNovaKeyState)
				for _, global := range globals {
					accountStates[channel.Id][global.Fingerprint] = global
				}
			}
			global := accountStates[channel.Id][candidate.Fingerprint]
			if global.State == SenseNovaInvalid || global.LeaseUntil > now || (candidate.Scope != "" && global.State == SenseNovaCooling) {
				continue
			}
			for key, status := range senseNovaManualKeyStates(channel) {
				if status == common.ChannelStatusEnabled && SenseNovaFingerprint(key) == candidate.Fingerprint {
					states = append(states, candidate)
					break
				}
			}
			if len(states) == limit {
				return states, nil
			}
		}
		if len(candidates) < 100 {
			return states, nil
		}
	}
}

// InvalidateSenseNovaKeys preserves tombstones, so a removed then re-added
// secret cannot match an old request/probe generation.
func InvalidateSenseNovaKeys(channelID int, keys []string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).First(&channel, channelID).Error; err != nil {
			return err
		}
		return invalidateSenseNovaKeys(tx, channelID, keys, false)
	})
}

func invalidateSenseNovaKeys(tx *gorm.DB, channelID int, keys []string, reset bool) error {
	for _, key := range keys {
		changes := map[string]interface{}{
			"lease_until": 0, "version": gorm.Expr("version + ?", 1),
		}
		if reset {
			changes["state"], changes["reason"], changes["failures"], changes["next_probe_at"] = SenseNovaUntested, "", 0, 0
		}
		if err := tx.Model(&SenseNovaKeyState{}).Where("channel_id = ? AND fingerprint = ?", channelID, SenseNovaFingerprint(key)).Updates(changes).Error; err != nil {
			return err
		}
	}
	return nil
}

// updateSenseNovaPoolIfNeeded commits configuration and generation changes
// under the same channel lock used by claims/results. Legacy updates retain
// their existing persistence path.
func (channel *Channel) updateSenseNovaPoolIfNeeded() (bool, error) {
	var current Channel
	if err := DB.Select("id", "sensenova_pool").First(&current, channel.Id).Error; err != nil {
		return false, err
	}
	if !channel.SenseNovaPool && !current.SenseNovaPool {
		return false, nil
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&current, channel.Id).Error; err != nil {
			return err
		}
		next := *channel
		if next.Key == "" {
			next.Key = current.Key
		}
		next.Keys = nil
		if next.Status == 0 {
			next.Status = current.Status
		}
		before, after := senseNovaManualKeyStates(&current), senseNovaManualKeyStates(&next)
		changed := make([]string, 0)
		reset := make([]string, 0)
		resetAll := current.SenseNovaPool != next.SenseNovaPool
		for key, status := range before {
			newStatus, exists := after[key]
			if resetAll || !exists {
				reset = append(reset, key)
			} else if newStatus != status || current.Status != next.Status {
				changed = append(changed, key)
			}
		}
		for key := range after {
			if _, exists := before[key]; !exists {
				reset = append(reset, key)
			}
		}
		if err := invalidateSenseNovaKeys(tx, channel.Id, changed, false); err != nil {
			return err
		}
		if err := invalidateSenseNovaKeys(tx, channel.Id, reset, true); err != nil {
			return err
		}
		if err := tx.Model(channel).Updates(channel).Error; err != nil {
			return err
		}
		// GORM struct Updates omits false; explicit opt-out must be persisted.
		return tx.Model(channel).Update("sensenova_pool", channel.SenseNovaPool).Error
	})
	return true, err
}

func senseNovaManualKeyStates(channel *Channel) map[string]int {
	states := make(map[string]int)
	for index, key := range channel.GetKeys() {
		status, set := channel.ChannelInfo.MultiKeyStatusList[index]
		if !set {
			status = common.ChannelStatusEnabled
		}
		states[key] = status
	}
	return states
}

func updateSenseNovaChannelStatusByTag(tag string, status int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var channels []Channel
		if err := lockForUpdate(tx).Where("tag = ? AND sensenova_pool = ?", tag, true).Order("id ASC").Find(&channels).Error; err != nil {
			return err
		}
		for _, channel := range channels {
			if channel.Status == status {
				continue
			}
			if err := invalidateSenseNovaKeys(tx, channel.Id, channel.GetKeys(), false); err != nil {
				return err
			}
		}
		return tx.Model(&Channel{}).Where("tag = ?", tag).Update("status", status).Error
	})
}
