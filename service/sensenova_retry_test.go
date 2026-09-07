package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSenseNovaRetryAfterParsing(t *testing.T) {
	now := time.Date(2026, 9, 7, 6, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name, header string
		want         int64
	}{
		{"seconds", "120", 120},
		{"whitespace", " 120 ", 120},
		{"zero", "0", 0},
		{"negative", "-1", 0},
		{"signed", "+1", 0},
		{"fraction", "1.5", 0},
		{"secret", "fake-secret", 0},
		{"empty", "", 0},
		{"large", "999999", 86400},
		{"overflow", strings.Repeat("9", 100), 86400},
		{"invalid large", strings.Repeat("9", 100) + "secret", 0},
		{"future date", now.Add(120 * time.Second).Format(http.TimeFormat), 120},
		{"past date", now.Add(-time.Second).Format(http.TimeFormat), 0},
		{"present date", now.Format(http.TimeFormat), 0},
		{"distant date", now.Add(7 * 24 * time.Hour).Format(http.TimeFormat), 86400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, senseNovaRetryAfter(tt.header, now))
		})
	}
	assert.Equal(t, int64(1), senseNovaRetryAfter(now.Add(time.Second).Format(http.TimeFormat), now.Add(time.Millisecond)))
}

func TestSenseNovaRelayRetryAfterIsAttemptLocal(t *testing.T) {
	channel, c := senseNovaServiceFixture(t)
	first, _, selectedErr := SelectSenseNovaKey(c, channel, "glm-5.2")
	require.Nil(t, selectedErr)
	assert.Nil(t, SenseNovaAttemptLogInfo(c))
	client := &http.Client{Transport: senseNovaRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"180"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"ModelAccountTpmRateLimitExceeded","message":"fake-secret user prompt"}}`))}, nil
	})}
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, "https://token.sensenova.cn/v1/chat/completions", nil)
	require.NoError(t, err)
	response, err := client.Do(request)
	require.NoError(t, err)
	upstream := RelayErrorHandler(c.Request.Context(), response, true)
	safe := RecordSenseNovaRelayFailure(c, upstream)
	assert.Equal(t, "SenseNova: rate_limited", safe.Error())
	assert.Equal(t, map[string]interface{}{
		"key_id": model.SenseNovaFingerprint(first), "attempt": 1,
		"max_attempts": 4, "limit_kind": "tpm", "retry_after_seconds": int64(180),
	}, SenseNovaAttemptLogInfo(c))
	assert.Empty(t, c.Writer.Header().Get("Retry-After"))
	states, err := model.ListSenseNovaStates(channel.Id)
	require.NoError(t, err)
	var cooled bool
	for _, state := range states {
		if state.Fingerprint == model.SenseNovaFingerprint(first) && state.Scope == "glm-5.2" {
			cooled = true
			assert.Equal(t, "rate_limited", state.Reason)
			assert.Equal(t, int64(180), state.NextProbeAt-state.LastFailureAt)
		}
		if state.Scope == "" {
			assert.NotEqual(t, model.SenseNovaCooling, state.State)
		}
	}
	require.True(t, cooled)
	_, err = model.SenseNovaKeySnapshot(channel.Id, first, "kimi-k3")
	require.NoError(t, err, "TPM must not cool other models on the same account")

	second, _, selectedErr := SelectSenseNovaKey(c, channel, "glm-5.2")
	require.Nil(t, selectedErr)
	assert.NotEqual(t, first, second)
	assert.Nil(t, SenseNovaAttemptLogInfo(c))
	RecordSenseNovaRelaySuccess(c)
	SetSenseNovaRetryAfterHeader(c)
	assert.Empty(t, c.Writer.Header().Get("Retry-After"), "successful fallback must not inherit a hint")

	// A later rejected attempt without a header gets fallback backoff, not A's.
	response = &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"busy"}}`))}
	RecordSenseNovaRelayFailure(c, RelayErrorHandler(c.Request.Context(), response, false))
	info := SenseNovaAttemptLogInfo(c)
	assert.Equal(t, 2, info["attempt"])
	assert.Equal(t, "unknown", info["limit_kind"])
	assert.NotContains(t, info, "retry_after_seconds")
	states, err = model.ListSenseNovaStates(channel.Id)
	require.NoError(t, err)
	for _, state := range states {
		if state.Fingerprint == model.SenseNovaFingerprint(second) && state.Scope == "glm-5.2" {
			assert.Equal(t, int64(60), state.NextProbeAt-state.LastFailureAt)
		}
	}
}

func TestSenseNovaSafeAttemptLimitCategories(t *testing.T) {
	for _, tt := range []struct{ code, want string }{
		{"ModelAccountTpmRateLimitExceeded", "tpm"},
		{"ModelAccountRpmRateLimitExceeded", "rpm"},
		{"overloaded_error", "capacity"},
		{"insufficient_quota", "quota"},
		{"invalid_api_key", "authentication"},
		{"quota_exceeded_error", "unknown"},
		{"modelaccounttpmratelimitexceeded", "unknown"},
		{"fake-secret-ModelAccountTpmRateLimitExceeded", "unknown"},
		{"concurrency maybe fake-secret", "unknown"},
	} {
		t.Run(tt.code, func(t *testing.T) {
			channel, c := senseNovaServiceFixture(t)
			key, _, selectedErr := SelectSenseNovaKey(c, channel, "glm-5.2")
			require.Nil(t, selectedErr)
			safe := RecordSenseNovaRelayFailure(c, types.WithOpenAIError(types.OpenAIError{Code: tt.code, Message: "fake-secret " + key}, 429))
			info := SenseNovaAttemptLogInfo(c)
			require.NotNil(t, info)
			assert.Equal(t, tt.want, info["limit_kind"])
			encoded, err := common.Marshal(info)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "fake-secret")
			assert.NotContains(t, string(encoded), key)
			assert.NotContains(t, safe.Error(), "fake-secret")
			assert.NotContains(t, safe.Error(), key)
		})
	}
}

func TestSenseNovaCodeOnlyHTTPRejectionsStaySafe(t *testing.T) {
	for _, tt := range []struct{ name, body, kind string }{
		{"code only", `{"error":{"code":"ModelAccountTpmRateLimitExceeded"}}`, "tpm"},
		{"unknown code only", `{"error":{"code":"fake-secret"}}`, "unknown"},
		{"arbitrary metadata", `{"error":{"code":{"secret":"fake-secret"},"message":"fake-secret prompt","param":"fake-secret","type":"fake-secret"},"metadata":"fake-secret"}`, "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			channel, c := senseNovaServiceFixture(t)
			_, _, selectedErr := SelectSenseNovaKey(c, channel, "glm-5.2")
			require.Nil(t, selectedErr)
			response := &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"fake-secret"}}, Body: io.NopCloser(strings.NewReader(tt.body))}
			safe := RecordSenseNovaRelayFailure(c, RelayErrorHandler(c.Request.Context(), response, true))
			assert.True(t, ShouldRetrySenseNova(c, safe))
			info := SenseNovaAttemptLogInfo(c)
			require.NotNil(t, info)
			assert.Equal(t, tt.kind, info["limit_kind"])
			assert.NotContains(t, info, "retry_after_seconds")
			encoded, err := common.Marshal(safe.ToOpenAIError())
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "fake-secret")
			encoded, err = common.Marshal(info)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "fake-secret")
		})
	}
}

func TestSenseNovaColdPoolHintUsesLiveModelRestrictions(t *testing.T) {
	channel, c := senseNovaServiceFixture(t)
	now := time.Now().Unix()
	keys := channel.GetKeys()
	for i, key := range keys {
		snapshot, err := model.SenseNovaKeySnapshot(channel.Id, key, "glm-5.2")
		require.NoError(t, err)
		applied, err := model.RecordSenseNovaFailure(snapshot, key, "glm-5.2", "rate_limited", false, int64(120+i*60), now)
		require.NoError(t, err)
		require.True(t, applied)
	}
	// Other-model and removed-key restrictions must not extend this hint.
	require.NoError(t, model.DB.Create(&model.SenseNovaKeyState{ChannelID: channel.Id, Fingerprint: model.SenseNovaFingerprint(keys[0]), Scope: "kimi-k3", State: model.SenseNovaCooling, NextProbeAt: now + 900}).Error)
	require.NoError(t, model.DB.Create(&model.SenseNovaKeyState{ChannelID: channel.Id, Fingerprint: model.SenseNovaFingerprint("removed-key"), Scope: "", State: model.SenseNovaCooling, NextProbeAt: now + 900}).Error)
	_, _, selectedErr := SelectSenseNovaKey(c, channel, "glm-5.2")
	require.NotNil(t, selectedErr)
	assert.Equal(t, http.StatusServiceUnavailable, selectedErr.StatusCode)
	assert.Nil(t, SenseNovaAttemptLogInfo(c), "no invented failed-attempt log for cold selection")
	SetSenseNovaRetryAfterHeader(c)
	seconds, err := strconv.ParseInt(c.Writer.Header().Get("Retry-After"), 10, 64)
	require.NoError(t, err)
	assert.InDelta(t, 120, seconds, 2, "independent keys recover at the earliest per-key deadline")

	// Both restrictions apply to A: max(account=240, model=120). B's
	// model=180 now supplies the earliest possible pool recovery.
	require.NoError(t, model.DB.Model(&model.SenseNovaKeyState{}).
		Where("channel_id = ? AND fingerprint = ? AND scope = ?", channel.Id, model.SenseNovaFingerprint(keys[0]), "").
		Updates(map[string]interface{}{"state": model.SenseNovaCooling, "next_probe_at": now + 240}).Error)
	SetSenseNovaRetryAfterHeader(c)
	seconds, err = strconv.ParseInt(c.Writer.Header().Get("Retry-After"), 10, 64)
	require.NoError(t, err)
	assert.InDelta(t, 180, seconds, 2)

	// A manually disabled identity cannot contribute a retry hint.
	channel.ChannelInfo.MultiKeyStatusList = map[int]int{1: common.ChannelStatusManuallyDisabled}
	require.NoError(t, model.DB.Model(channel).Update("channel_info", channel.ChannelInfo).Error)
	c.Writer.Header().Del("Retry-After")
	SetSenseNovaRetryAfterHeader(c)
	seconds, err = strconv.ParseInt(c.Writer.Header().Get("Retry-After"), 10, 64)
	require.NoError(t, err)
	assert.InDelta(t, 240, seconds, 2)
}

func TestSenseNovaFinalHintDoesNotExtendIndependentKeyCooldown(t *testing.T) {
	channel, c := senseNovaServiceFixture(t)
	for _, delay := range []string{"120", "600"} {
		_, _, selectedErr := SelectSenseNovaKey(c, channel, "glm-5.2")
		require.Nil(t, selectedErr)
		response := &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {delay}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"busy"}}`))}
		RecordSenseNovaRelayFailure(c, RelayErrorHandler(c.Request.Context(), response, false))
	}
	SetSenseNovaRetryAfterHeader(c)
	seconds, err := strconv.ParseInt(c.Writer.Header().Get("Retry-After"), 10, 64)
	require.NoError(t, err)
	assert.InDelta(t, 120, seconds, 2, "last rejected key must not override the earlier independent key")
}

func TestSenseNovaFinalHintOmitsCooldownWhenAnotherKeyIsHealthy(t *testing.T) {
	channel, c := senseNovaServiceFixture(t)
	_, _, selectedErr := SelectSenseNovaKey(c, channel, "glm-5.2")
	require.Nil(t, selectedErr)
	response := &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"600"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"busy"}}`))}
	RecordSenseNovaRelayFailure(c, RelayErrorHandler(c.Request.Context(), response, false))
	SetSenseNovaRetryAfterHeader(c)
	assert.Empty(t, c.Writer.Header().Get("Retry-After"), "an eligible independent key has no known waiting period")
}

func TestSenseNovaFinalHintCancellationWrittenAndNonPoolGuards(t *testing.T) {
	for _, mode := range []string{"canceled", "written", "non-pool"} {
		t.Run(mode, func(t *testing.T) {
			channel, c := senseNovaServiceFixture(t)
			_, _, selectedErr := SelectSenseNovaKey(c, channel, "glm-5.2")
			require.Nil(t, selectedErr)
			RecordSenseNovaRelayFailure(c, types.WithOpenAIError(types.OpenAIError{Message: "busy"}, 429))
			switch mode {
			case "canceled":
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			case "written":
				c.String(200, "already emitted")
			case "non-pool":
				c, _ = gin.CreateTestContext(httptest.NewRecorder())
			}
			SetSenseNovaRetryAfterHeader(c)
			assert.Empty(t, c.Writer.Header().Get("Retry-After"))
		})
	}
}
