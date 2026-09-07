package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
)

func TestSenseNovaFailureClassification(t *testing.T) {
	for _, tt := range []struct {
		name, code, message string
		status              int
		state, reason       string
		account             bool
	}{
		{"quota", "insufficient_quota", "credits exhausted", 429, "cooling", "quota_exhausted", true},
		{"ambiguous quota code", "quota_exceeded_error", "limit exceeded", 429, "cooling", "rate_limited", false},
		{"explicit credit exhaustion", "quota_exceeded_error", "积分不足", 429, "cooling", "quota_exhausted", true},
		{"generic rate limit", "rate_limit_error", "too many requests", 429, "cooling", "rate_limited", false},
		{"embedded TPM", "429001", "", 502, "cooling", "rate_limited", false},
		{"embedded named TPM", "ModelAccountTpmRateLimitExceeded", "", 502, "cooling", "rate_limited", false},
		{"embedded RPM", "ModelAccountRpmRateLimitExceeded", "", 502, "cooling", "rate_limited", false},
		{"embedded unknown", "429002", "", 502, "cooling", "upstream_unavailable", false},
		{"auth", "invalid_api_key", "invalid key", 401, "invalid", "authentication_failed", true},
		{"model permission", "permission_denied", "model forbidden", 403, "cooling", "model_unavailable", false},
		{"missing model", "model_not_found", "not found", 404, "cooling", "model_unavailable", false},
		{"capacity", "overloaded_error", "busy", 503, "cooling", "upstream_unavailable", false},
		{"client request", "invalid_request_error", "bad parameter", 400, "", "", false},
		{"context length", "context_length_exceeded", "too long", 400, "", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := types.WithOpenAIError(types.OpenAIError{Code: tt.code, Message: tt.message}, tt.status)
			got := ClassifySenseNovaFailure(err)
			assert.Equal(t, tt.state, got.State)
			assert.Equal(t, tt.reason, got.Reason)
			assert.Equal(t, tt.account, got.AccountWide)
		})
	}
}

func TestSenseNovaDoesNotClassifyGatewayQuotaAsUpstreamExhaustion(t *testing.T) {
	err := types.NewErrorWithStatusCode(errors.New("insufficient quota"), types.ErrorCodeInsufficientUserQuota, 429)
	assert.Empty(t, ClassifySenseNovaFailure(err).State)
	assert.Empty(t, ClassifySenseNovaFailure(nil).State)
}

func TestSenseNovaTransportFailureIsModelScoped(t *testing.T) {
	err := types.NewError(errors.New("connection reset"), types.ErrorCodeDoRequestFailed)
	got := ClassifySenseNovaFailure(err)
	assert.Equal(t, "cooling", got.State)
	assert.Equal(t, "upstream_unavailable", got.Reason)
	assert.False(t, got.AccountWide)
}
