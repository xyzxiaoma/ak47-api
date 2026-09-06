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
