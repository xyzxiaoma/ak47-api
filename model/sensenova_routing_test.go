package model

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type senseNovaRoutingSQLRecorder struct {
	logger.Interface
	statements []string
}

func (recorder *senseNovaRoutingSQLRecorder) Trace(_ context.Context, _ time.Time, query func() (string, int64), _ error) {
	sql, _ := query()
	recorder.statements = append(recorder.statements, sql)
}

func TestSenseNovaRoutingBatchReadsWithoutCreatingOrWritingHealth(t *testing.T) {
	channel := setupSenseNovaTest(t)
	keys := make([]string, 200)
	for i := range keys {
		keys[i] = fmt.Sprintf("synthetic-routing-key-%d", i)
	}
	channel.Key = strings.Join(keys, "\n")
	existing := SenseNovaKeyState{ChannelID: channel.Id, Fingerprint: SenseNovaFingerprint(keys[0]), Scope: "deepseek-v4-pro", State: SenseNovaUsable, Version: 3, LastSuccessAt: 900}
	require.NoError(t, DB.Create(&existing).Error)
	recorder := &senseNovaRoutingSQLRecorder{Interface: logger.Default}
	previousDB := DB
	DB = DB.Session(&gorm.Session{Logger: recorder})
	t.Cleanup(func() { DB = previousDB })
	hints, err := SenseNovaRoutingCandidates(channel, "deepseek-v4-pro", 1000)
	require.NoError(t, err)
	require.Len(t, hints, 200)
	for _, key := range keys {
		assert.Equal(t, SenseNovaRoutingEligibility{Available: true}, hints[SenseNovaFingerprint(key)])
	}
	require.Len(t, recorder.statements, 1, "a full pool must use a single read, with no per-key transaction or writes")
	assert.True(t, strings.HasPrefix(strings.ToUpper(recorder.statements[0]), "SELECT "))
	assert.NotContains(t, strings.ToUpper(recorder.statements[0]), "FOR UPDATE")
	states, err := ListSenseNovaStates(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, []SenseNovaKeyState{existing}, states, "unseen candidates must not materialize health rows or mutate existing ones")
}

func TestSenseNovaRoutingEligibilityMatchesAuthoritativeSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name     string
		states   []SenseNovaKeyState
		expected SenseNovaRoutingEligibility
	}{
		{name: "unseen", expected: SenseNovaRoutingEligibility{Available: true}},
		{name: "usable", states: []SenseNovaKeyState{{Scope: "", State: SenseNovaUsable}, {Scope: "deepseek-v4-pro", State: SenseNovaUsable}}, expected: SenseNovaRoutingEligibility{Available: true}},
		{name: "due global rate", states: []SenseNovaKeyState{{Scope: "", State: SenseNovaCooling, Reason: "rate_limited", NextProbeAt: 1000}}, expected: SenseNovaRoutingEligibility{Available: true, NeedsRecovery: true}},
		{name: "due model rate", states: []SenseNovaKeyState{{Scope: "deepseek-v4-pro", State: SenseNovaCooling, Reason: "rate_limited", NextProbeAt: 999}}, expected: SenseNovaRoutingEligibility{Available: true, NeedsRecovery: true}},
		{name: "future rate", states: []SenseNovaKeyState{{Scope: "deepseek-v4-pro", State: SenseNovaCooling, Reason: "rate_limited", NextProbeAt: 1001}}},
		{name: "non rate cooling remains unavailable", states: []SenseNovaKeyState{{Scope: "", State: SenseNovaCooling, Reason: "quota_exhausted", NextProbeAt: 900}}},
		{name: "pending recovery", states: []SenseNovaKeyState{{Scope: "deepseek-v4-pro", State: SenseNovaUntested, Reason: "rate_limited"}}, expected: SenseNovaRoutingEligibility{Available: true, NeedsRecovery: true}},
		{name: "pending active lease", states: []SenseNovaKeyState{{Scope: "", State: SenseNovaUntested, Reason: "rate_limited", LeaseUntil: 1001}}},
		{name: "pending expired lease", states: []SenseNovaKeyState{{Scope: "", State: SenseNovaUntested, Reason: "rate_limited", LeaseUntil: 1000}}, expected: SenseNovaRoutingEligibility{Available: true, NeedsRecovery: true}},
		{name: "due rate active lease", states: []SenseNovaKeyState{{Scope: "deepseek-v4-pro", State: SenseNovaCooling, Reason: "rate_limited", NextProbeAt: 999, LeaseUntil: 1001}}},
		{name: "global invalid beats model recovery", states: []SenseNovaKeyState{{Scope: "", State: SenseNovaInvalid}, {Scope: "deepseek-v4-pro", State: SenseNovaCooling, Reason: "rate_limited", NextProbeAt: 900}}},
		{name: "model invalid beats global recovery", states: []SenseNovaKeyState{{Scope: "", State: SenseNovaCooling, Reason: "rate_limited", NextProbeAt: 900}, {Scope: "deepseek-v4-pro", State: SenseNovaInvalid}}},
		{name: "unaffected model", states: []SenseNovaKeyState{{Scope: "glm-5.2", State: SenseNovaInvalid}}, expected: SenseNovaRoutingEligibility{Available: true}},
		{name: "ordinary probe lease remains a hint", states: []SenseNovaKeyState{{Scope: "", State: SenseNovaUsable, LeaseUntil: 1001}}, expected: SenseNovaRoutingEligibility{Available: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := setupSenseNovaTest(t)
			key := channel.GetKeys()[0]
			for _, state := range tc.states {
				state.ChannelID, state.Fingerprint, state.Version = channel.Id, SenseNovaFingerprint(key), 1
				require.NoError(t, DB.Create(&state).Error)
			}
			hints, err := SenseNovaRoutingCandidates(channel, "deepseek-v4-pro", 1000)
			require.NoError(t, err)
			actual, exists := hints[SenseNovaFingerprint(key)]
			require.True(t, exists)
			assert.Equal(t, tc.expected, actual)
			snapshot, err := SenseNovaRequestSnapshot(channel.Id, key, "deepseek-v4-pro", 1000)
			if !tc.expected.Available {
				assert.ErrorIs(t, err, ErrSenseNovaUnavailable)
			} else {
				require.NoError(t, err)
				assert.Equal(t, snapshot.NeedsRecovery, actual.NeedsRecovery)
			}
		})
	}
}

func TestSenseNovaRoutingCurrentInventoryAndAdminStatus(t *testing.T) {
	channel := setupSenseNovaTest(t)
	first, second := channel.GetKeys()[0], channel.GetKeys()[1]
	state := SenseNovaKeyState{ChannelID: channel.Id, Fingerprint: SenseNovaFingerprint(first), Scope: "", State: SenseNovaInvalid, Version: 1}
	require.NoError(t, DB.Create(&state).Error)
	// Another channel's restriction cannot leak to this channel's same key.
	other := setupSenseNovaTest(t)
	state.ID, state.ChannelID, state.Fingerprint = 0, other.Id, SenseNovaFingerprint(second)
	require.NoError(t, DB.Create(&state).Error)
	channel.Key = second + "\n" + first
	channel.ChannelInfo.MultiKeyStatusList = map[int]int{0: common.ChannelStatusManuallyDisabled}
	hints, err := SenseNovaRoutingCandidates(channel, "deepseek-v4-pro", 1000)
	require.NoError(t, err)
	assert.Equal(t, map[string]SenseNovaRoutingEligibility{SenseNovaFingerprint(first): {}, SenseNovaFingerprint(second): {}}, hints)
	channel.ChannelInfo.MultiKeyStatusList = nil
	hints, err = SenseNovaRoutingCandidates(channel, "deepseek-v4-pro", 1000)
	require.NoError(t, err)
	assert.Equal(t, SenseNovaRoutingEligibility{Available: true}, hints[SenseNovaFingerprint(second)])
	assert.Zero(t, hints[SenseNovaFingerprint(first)], "health follows fingerprints after reordering")
	channel.Key = second + "\nnew-synthetic-routing-key"
	hints, err = SenseNovaRoutingCandidates(channel, "deepseek-v4-pro", 1000)
	require.NoError(t, err)
	assert.NotContains(t, hints, SenseNovaFingerprint(first), "removed tombstones are not candidates")
	assert.Equal(t, SenseNovaRoutingEligibility{Available: true}, hints[SenseNovaFingerprint("new-synthetic-routing-key")])
}

func TestSenseNovaRoutingRejectsDisabledPoolOrUnknownModel(t *testing.T) {
	channel := setupSenseNovaTest(t)
	for _, name := range []string{"", "DeepSeek-v4-pro", "deepseek-v4-pro-alias"} {
		_, err := SenseNovaRoutingCandidates(channel, name, 1000)
		assert.ErrorIs(t, err, ErrSenseNovaUnavailable)
	}
	channel.Status = common.ChannelStatusManuallyDisabled
	_, err := SenseNovaRoutingCandidates(channel, "deepseek-v4-pro", 1000)
	assert.ErrorIs(t, err, ErrSenseNovaUnavailable)
	channel.Status, channel.SenseNovaPool = common.ChannelStatusEnabled, false
	_, err = SenseNovaRoutingCandidates(channel, "deepseek-v4-pro", 1000)
	assert.ErrorIs(t, err, ErrSenseNovaUnavailable)
	_, err = SenseNovaRoutingCandidates(nil, "deepseek-v4-pro", 1000)
	assert.ErrorIs(t, err, ErrSenseNovaUnavailable)
}
