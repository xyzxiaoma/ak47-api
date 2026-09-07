package controller

import "github.com/QuantumNous/new-api/model"

// SenseNovaKeyHealth is intentionally a separate response type: persistence
// leases, versions and credential material must not cross the console boundary.
type SenseNovaKeyHealth struct {
	State         string                 `json:"state"`
	Reason        string                 `json:"reason"`
	LastSuccessAt int64                  `json:"last_success_at"`
	LastFailureAt int64                  `json:"last_failure_at"`
	LastProbeAt   int64                  `json:"last_probe_at"`
	NextProbeAt   int64                  `json:"next_probe_at"`
	ModelStates   []SenseNovaModelHealth `json:"model_states,omitempty"`
}

type SenseNovaModelHealth struct {
	Model       string `json:"model"`
	State       string `json:"state"`
	Reason      string `json:"reason"`
	NextProbeAt int64  `json:"next_probe_at"`
}

func projectSenseNovaHealth(states []model.SenseNovaKeyState) map[string]*SenseNovaKeyHealth {
	result := make(map[string]*SenseNovaKeyHealth)
	for _, state := range states {
		health := result[state.Fingerprint]
		if health == nil {
			health = &SenseNovaKeyHealth{State: model.SenseNovaUntested}
			result[state.Fingerprint] = health
		}
		if state.Scope == "" {
			health.State = state.State
			health.Reason = state.Reason
			health.LastSuccessAt = state.LastSuccessAt
			health.LastFailureAt = state.LastFailureAt
			health.LastProbeAt = state.LastProbeAt
			health.NextProbeAt = state.NextProbeAt
			continue
		}
		if state.State == model.SenseNovaCooling || state.State == model.SenseNovaInvalid ||
			(state.State == model.SenseNovaUntested && state.Reason == "rate_limited") {
			health.ModelStates = append(health.ModelStates, SenseNovaModelHealth{
				Model: state.Scope, State: state.State, Reason: state.Reason, NextProbeAt: state.NextProbeAt,
			})
		}
	}
	return result
}
