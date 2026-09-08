package service

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/types"
)

// SenseNovaFailure contains safe classification codes, never raw upstream text.
// A generic 429 does not tell us whether the account's credit pool is exhausted.
type SenseNovaFailure struct {
	State       string
	Reason      string
	AccountWide bool
}

// Named codes and observed provider tuples are matched exactly. Message/type
// normalization only ignores case and whitespace; substrings never establish
// TPM or RPM. Keep diagnostics separate from health/account scoping.
func senseNovaLimitKind(err *types.NewAPIError) string {
	if err == nil || err.GetErrorType() == types.ErrorTypeNewAPIError {
		return "unknown"
	}
	providerType, providerMessage := "", ""
	if provider, ok := err.RelayError.(types.OpenAIError); ok {
		providerType = strings.ToLower(strings.TrimSpace(provider.Type))
		providerMessage = strings.ToLower(strings.Join(strings.Fields(provider.Message), " "))
	}
	switch string(err.GetErrorCode()) {
	case "ModelAccountTpmRateLimitExceeded":
		return "tpm"
	case "429001":
		if providerType == "invalid_request_error" && providerMessage == "inference tpm exhausted" {
			return "tpm"
		}
		// The same code also means "inference exceeds tpm/rpm limit".
		// A bare code or mixed message cannot prove token exhaustion.
		return "rate_limit"
	case "8":
		if providerType == "quota_exceeded_error" && providerMessage == "rpm exhausted" {
			return "rpm"
		}
		return "unknown"
	case "ModelAccountRpmRateLimitExceeded":
		return "rpm"
	case "overloaded_error":
		return "capacity"
	case "insufficient_quota", "insufficient_balance":
		return "quota"
	case "invalid_api_key", "authentication_error":
		return "authentication"
	default:
		return "unknown"
	}
}

func ClassifySenseNovaFailure(err *types.NewAPIError) SenseNovaFailure {
	if err == nil {
		return SenseNovaFailure{}
	}
	if err.GetErrorType() == types.ErrorTypeNewAPIError {
		if err.GetErrorCode() == types.ErrorCodeDoRequestFailed {
			return SenseNovaFailure{State: "cooling", Reason: "upstream_unavailable"}
		}
		// Gateway validation, billing and database failures are not key failures.
		if err.GetErrorCode() != types.ErrorCodeBadResponseStatusCode {
			return SenseNovaFailure{}
		}
	}
	code := strings.ToLower(string(err.GetErrorCode()))
	if err.StatusCode == http.StatusUnauthorized || code == "invalid_api_key" || code == "authentication_error" {
		return SenseNovaFailure{State: "invalid", Reason: "authentication_failed", AccountWide: true}
	}
	if code == "insufficient_quota" || code == "insufficient_balance" {
		return SenseNovaFailure{State: "cooling", Reason: "quota_exhausted", AccountWide: true}
	}
	// Embedded errors in HTTP-200 streams are surfaced as bad gateway. Exact
	// provider limit codes still identify model capacity, independent of status.
	if kind := senseNovaLimitKind(err); kind == "tpm" || kind == "rpm" || kind == "rate_limit" {
		return SenseNovaFailure{State: "cooling", Reason: "rate_limited"}
	}
	if err.StatusCode == http.StatusPaymentRequired || err.StatusCode == http.StatusTooManyRequests || err.StatusCode == http.StatusForbidden {
		message := strings.ToLower(err.Error())
		if code == "insufficient_quota" || code == "insufficient_balance" ||
			strings.Contains(message, "credits exhausted") || strings.Contains(message, "insufficient credits") ||
			strings.Contains(message, "积分不足") || strings.Contains(message, "额度已用尽") {
			return SenseNovaFailure{State: "cooling", Reason: "quota_exhausted", AccountWide: true}
		}
	}
	switch err.StatusCode {
	case http.StatusTooManyRequests:
		return SenseNovaFailure{State: "cooling", Reason: "rate_limited"}
	case http.StatusForbidden, http.StatusNotFound:
		return SenseNovaFailure{State: "cooling", Reason: "model_unavailable"}
	case http.StatusPaymentRequired:
		return SenseNovaFailure{State: "cooling", Reason: "quota_exhausted", AccountWide: true}
	}
	if err.StatusCode >= 500 && err.StatusCode <= 599 {
		return SenseNovaFailure{State: "cooling", Reason: "upstream_unavailable"}
	}
	return SenseNovaFailure{}
}
