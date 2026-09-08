package service

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type senseNovaModelBudget struct {
	Default int64            `json:"default"`
	Keys    map[string]int64 `json:"keys"`
}

type senseNovaAdmissionConfig struct {
	budget           senseNovaModelBudget
	outputAllowance  int64
	interval         time.Duration
	wait             time.Duration
	queueLimit       int
	affinity         bool
	followups        int
	followupInterval time.Duration
}

// These are operator admission policies, not asserted provider quota values.
func senseNovaAdmissionConfigFor(name string) (*senseNovaAdmissionConfig, error) {
	enabled := true
	if raw := strings.TrimSpace(os.Getenv("SENSENOVA_ADMISSION_ENABLED")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, errors.New("invalid SenseNova admission configuration")
		}
		enabled = parsed
	}
	if !enabled {
		return nil, nil
	}
	names := os.Getenv("SENSENOVA_ADMISSION_MODELS")
	if strings.TrimSpace(names) == "" {
		names = "deepseek-v4-pro"
	}
	applies := false
	for _, candidate := range strings.Split(names, ",") {
		candidate = strings.TrimSpace(candidate)
		if !model.IsSenseNovaModel(candidate) {
			return nil, errors.New("invalid SenseNova admission model configuration")
		}
		applies = applies || candidate == name
	}
	if !applies {
		return nil, nil
	}
	cfg := &senseNovaAdmissionConfig{outputAllowance: 4096, interval: 60 * time.Second, wait: 75 * time.Second, queueLimit: 32, followupInterval: 5 * time.Second}
	if raw := strings.TrimSpace(os.Getenv("SENSENOVA_CONVERSATION_AFFINITY_ENABLED")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, errors.New("invalid SenseNova affinity configuration")
		}
		cfg.affinity = value
	}
	if raw := strings.TrimSpace(os.Getenv("SENSENOVA_CANARY_FOLLOWUPS")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 2 {
			return nil, errors.New("invalid SenseNova followup configuration")
		}
		cfg.followups = value
	}
	if cfg.followups > 0 && !cfg.affinity {
		return nil, errors.New("SenseNova followups require conversation affinity")
	}
	limits := map[string]senseNovaModelBudget{}
	if raw := strings.TrimSpace(os.Getenv("SENSENOVA_TPM_LIMITS")); raw != "" {
		if common.Unmarshal([]byte(raw), &limits) != nil {
			return nil, errors.New("invalid SenseNova TPM configuration")
		}
		for name, budget := range limits {
			if !model.IsSenseNovaModel(name) || budget.Default < 0 || budget.Default > 1_000_000_000_000 {
				return nil, errors.New("invalid SenseNova TPM configuration")
			}
			for fingerprint, limit := range budget.Keys {
				if len(fingerprint) != 64 || strings.Trim(fingerprint, "0123456789abcdef") != "" || limit < 0 || limit > 1_000_000_000_000 {
					return nil, errors.New("invalid SenseNova TPM key configuration")
				}
			}
		}
	}
	cfg.budget = limits[name]
	for _, setting := range []struct {
		name     string
		target   *int64
		min, max int64
	}{
		{"SENSENOVA_OUTPUT_TOKEN_ALLOWANCE", &cfg.outputAllowance, 0, 1_000_000},
	} {
		if raw := strings.TrimSpace(os.Getenv(setting.name)); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < setting.min || value > setting.max {
				return nil, errors.New("invalid SenseNova admission configuration")
			}
			*setting.target = value
		}
	}
	for _, setting := range []struct {
		name     string
		target   *time.Duration
		min, max int64
	}{
		{"SENSENOVA_UNKNOWN_TPM_INTERVAL_SECONDS", &cfg.interval, 60, 300},
		{"SENSENOVA_ADMISSION_WAIT_SECONDS", &cfg.wait, 1, 75},
		{"SENSENOVA_CANARY_FOLLOWUP_INTERVAL_SECONDS", &cfg.followupInterval, 5, 60},
	} {
		if raw := strings.TrimSpace(os.Getenv(setting.name)); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < setting.min || value > setting.max {
				return nil, errors.New("invalid SenseNova admission configuration")
			}
			*setting.target = time.Duration(value) * time.Second
		}
	}
	if raw := strings.TrimSpace(os.Getenv("SENSENOVA_ADMISSION_QUEUE_LIMIT")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 32 {
			return nil, errors.New("invalid SenseNova queue configuration")
		}
		cfg.queueLimit = value
	}
	return cfg, nil
}

func (cfg *senseNovaAdmissionConfig) policy(fingerprint string) senseNovaBudgetPolicy {
	limit := cfg.budget.Default
	if specific, ok := cfg.budget.Keys[fingerprint]; ok {
		limit = specific
	}
	return senseNovaBudgetPolicy{TokensPerMinute: limit, OutputAllowance: cfg.outputAllowance, Interval: cfg.interval, Window: time.Minute, Lease: 120 * time.Second,
		Followups: cfg.followups, FollowupInterval: cfg.followupInterval}
}
