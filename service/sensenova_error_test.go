package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestSenseNovaObservedLimitTuples(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		code                        any
		providerType, message, kind string
	}{
		{"mixed limit", "429001", "rate_limit_error", "inference exceeds tpm/rpm limit", "rate_limit"},
		{"numeric mixed limit", float64(429001), "rate_limit_error", "inference exceeds tpm/rpm limit", "rate_limit"},
		{"code alone", "429001", "", "", "rate_limit"},
		{"unknown text is not TPM", "429001", "rate_limit_error", "fake-secret tpm user prompt", "rate_limit"},
		{"observed request exhaustion", float64(8), "quota_exceeded_error", "rpm exhausted", "rpm"},
		{"normalized request exhaustion", "8", " Quota_Exceeded_Error ", "  RPM\t exhausted\n", "rpm"},
		{"different code", "9", "quota_exceeded_error", "rpm exhausted", "unknown"},
		{"missing type", "8", "", "rpm exhausted", "unknown"},
		{"different type", "8", "rate_limit_error", "rpm exhausted", "unknown"},
		{"message suffix", "8", "quota_exceeded_error", "rpm exhausted fake-secret", "unknown"},
		{"quoted message", "8", "quota_exceeded_error", "request mentioned rpm exhausted", "unknown"},
		{"known named TPM", "ModelAccountTpmRateLimitExceeded", "", "", "tpm"},
		{"known exact TPM tuple", "429001", "invalid_request_error", "inference tpm exhausted", "tpm"},
		{"TPM text alone is ambiguous", "429001", "rate_limit_error", "inference tpm exhausted", "rate_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, status := range []int{http.StatusTooManyRequests, http.StatusBadGateway} {
				upstream := types.WithOpenAIError(types.OpenAIError{Code: tc.code, Type: tc.providerType, Message: tc.message}, status)
				assert.Equal(t, tc.kind, senseNovaLimitKind(upstream))
				failure := ClassifySenseNovaFailure(upstream)
				if tc.kind != "unknown" || status == http.StatusTooManyRequests {
					assert.Equal(t, SenseNovaFailure{State: "cooling", Reason: "rate_limited"}, failure,
						"embedded limits must remain model-rate failures, never account-credit failures")
				} else {
					assert.Equal(t, SenseNovaFailure{State: "cooling", Reason: "upstream_unavailable"}, failure)
				}
			}
		})
	}
}

func TestSenseNovaAmbiguousLimitDoesNotPublishTPMEvidence(t *testing.T) {
	setupSenseNovaBudgetRedis(t)
	ctx := context.Background()
	request := senseNovaCapacityTestRequest()
	owner := senseNovaCapacityTestReserve(t, request)
	upstream := types.WithOpenAIError(types.OpenAIError{Code: "429001", Type: "rate_limit_error", Message: "inference exceeds tpm/rpm limit"}, http.StatusBadGateway)
	require.NoError(t, recordSenseNovaCapacity(ctx, owner, request, false, senseNovaLimitKind(upstream) == "tpm", 0))
	observations, err := readSenseNovaCapacity(ctx, []senseNovaBudgetRequest{request})
	require.NoError(t, err)
	assert.Equal(t, []senseNovaCapacityObservation{{}}, observations, "ambiguous limits cannot establish a request-size TPM penalty")
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
