package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const senseNovaAttemptContext = "sensenova_attempt"
const senseNovaExcludedContext = "sensenova_excluded"
const senseNovaRetryContext = "sensenova_retry"
const senseNovaSelectionContext = "sensenova_selection"
const senseNovaAttemptCountContext = "sensenova_attempt_count"

const SenseNovaMaxAttempts = 4

type senseNovaRequestContextKey struct{}
type senseNovaAttemptRequestContextKey struct{}

type senseNovaSelection struct {
	channelID int
	model     string
}

func IsSenseNovaRequest(ctx context.Context) bool {
	value, _ := ctx.Value(senseNovaRequestContextKey{}).(bool)
	return value
}

var senseNovaOffsets sync.Map

type senseNovaAttempt struct {
	snapshot   *model.SenseNovaSnapshot
	key        string
	model      string
	number     int
	failed     bool
	limitKind  string
	retryAfter int64
	recovery   *senseNovaRecoveryRenewal
}

// ValidateSenseNovaPool restricts the opt-in policy to its actual provider and
// model identities; overrides could otherwise change the credential or scope.
func ValidateSenseNovaPool(channel *model.Channel) error {
	if !channel.SenseNovaPool {
		return nil
	}
	u, err := url.Parse(channel.GetBaseURL())
	if err != nil || u.Scheme != "https" || u.Host != "token.sensenova.cn" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("SenseNova pool requires https://token.sensenova.cn as its base URL")
	}
	if channel.Type != constant.ChannelTypeOpenAI {
		return errors.New("SenseNova pool requires an OpenAI channel")
	}
	if len(channel.GetModels()) == 0 {
		return errors.New("SenseNova pool requires at least one supported model")
	}
	for _, name := range channel.GetModels() {
		if !model.IsSenseNovaModel(name) {
			return fmt.Errorf("unsupported SenseNova pool model: %s", name)
		}
	}
	for _, value := range []string{channel.GetModelMapping(), channel.GetStatusCodeMapping(), stringValue(channel.HeaderOverride), stringValue(channel.ParamOverride)} {
		value = strings.TrimSpace(value)
		if value != "" && value != "{}" {
			return errors.New("SenseNova pool does not support model, status, header or parameter overrides")
		}
	}
	return nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func ResetSenseNovaAttempt(c *gin.Context) {
	c.Set(senseNovaAttemptContext, (*senseNovaAttempt)(nil))
	c.Set(senseNovaRetryContext, false)
	c.Set(senseNovaSelectionContext, (*senseNovaSelection)(nil))
	if c.Request != nil {
		ctx := context.WithValue(c.Request.Context(), senseNovaAttemptRequestContextKey{}, (*senseNovaAttempt)(nil))
		c.Request = c.Request.WithContext(context.WithValue(ctx, senseNovaRequestContextKey{}, false))
	}
}
func IsSenseNovaAttempt(c *gin.Context) bool { return getSenseNovaAttempt(c) != nil }
func SenseNovaAttemptChannelID(c *gin.Context) int {
	if a := getSenseNovaAttempt(c); a != nil {
		return a.snapshot.ChannelID
	}
	return 0
}
func getSenseNovaAttempt(c *gin.Context) *senseNovaAttempt {
	v, _ := c.Get(senseNovaAttemptContext)
	a, _ := v.(*senseNovaAttempt)
	return a
}

func SelectSenseNovaKey(c *gin.Context, channel *model.Channel, name string) (string, int, *types.NewAPIError) {
	InitSenseNovaLatency(c)
	state, configErr := senseNovaAdmissionState(c, name)
	if configErr != nil {
		return "", 0, senseNovaAdmissionError("Invalid SenseNova admission configuration", http.StatusServiceUnavailable)
	}
	defer releaseSenseNovaAdmissionQueue(state)
	for {
		key, index, selectedErr := selectSenseNovaKeyOnce(c, channel, name)
		if selectedErr == nil || state == nil {
			return key, index, selectedErr
		}
		fresh, err := model.GetChannelById(channel.Id, true)
		if err != nil || fresh.Status != common.ChannelStatusEnabled || !fresh.SenseNovaPool || ValidateSenseNovaPool(fresh) != nil || !containsSenseNovaModel(fresh, name) {
			return "", 0, selectedErr
		}
		delay, cooling := senseNovaHealthWait(c, fresh, name)
		if !cooling {
			return "", 0, selectedErr
		}
		if waitErr := waitSenseNovaAdmission(c, state, channel.Id, delay, SenseNovaWaitHealth); waitErr != nil {
			return "", 0, waitErr
		}
	}
}

func selectSenseNovaKeyOnce(c *gin.Context, channel *model.Channel, name string) (string, int, *types.NewAPIError) {
	ResetSenseNovaAttempt(c)
	unavailable := func() (string, int, *types.NewAPIError) {
		return "", 0, types.NewErrorWithStatusCode(errors.New("SenseNova pool temporarily has no eligible key"), types.ErrorCodeGetChannelFailed, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	fresh, err := model.GetChannelById(channel.Id, true)
	if err != nil || !fresh.SenseNovaPool || ValidateSenseNovaPool(fresh) != nil || !model.IsSenseNovaModel(name) {
		return unavailable()
	}
	c.Set(senseNovaSelectionContext, &senseNovaSelection{channelID: fresh.Id, model: name})
	if c.Request != nil {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), senseNovaRequestContextKey{}, true))
	}
	keys := fresh.GetKeys()
	if len(keys) == 0 {
		return unavailable()
	}
	value, _ := senseNovaOffsets.LoadOrStore(channel.Id, &atomic.Uint64{})
	start := int(value.(*atomic.Uint64).Add(1)-1) % len(keys)
	excluded, _ := c.Get(senseNovaExcludedContext)
	set, _ := excluded.(map[string]bool)
	for i := 0; i < len(keys); i++ {
		index := (start + i) % len(keys)
		key := keys[index]
		if set[fmt.Sprintf("%d:%s", channel.Id, model.SenseNovaFingerprint(key))] {
			continue
		}
		snapshot, snapshotErr := model.SenseNovaRequestSnapshot(channel.Id, key, name, time.Now().Unix())
		if errors.Is(snapshotErr, model.ErrSenseNovaUnavailable) {
			continue
		}
		if snapshotErr != nil {
			return unavailable()
		}
		number := c.GetInt(senseNovaAttemptCountContext) + 1
		c.Set(senseNovaAttemptCountContext, number)
		attempt := &senseNovaAttempt{snapshot: snapshot, key: key, model: name, number: number}
		c.Set(senseNovaAttemptContext, attempt)
		if c.Request != nil {
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), senseNovaAttemptRequestContextKey{}, attempt))
		}
		return key, index, nil
	}
	return unavailable()
}

func RecordSenseNovaRelaySuccess(c *gin.Context) {
	stopSenseNovaRecoveryRenewal(c)
	defer finishSenseNovaAdmissionAttempt(c)
	if state := currentSenseNovaAdmission(c); state != nil {
		state.successful = true
	}
	if a := getSenseNovaAttempt(c); a != nil {
		a.failed = false
		a.retryAfter = 0
		if _, err := model.RecordSenseNovaSuccess(a.snapshot, a.key, time.Now().Unix()); err != nil {
			common.SysError("SenseNova success state persistence failed")
		}
	}
}

// Record failures before selecting a retry. The request-local exclusion remains
// effective even if the database cannot persist the outcome.
func RecordSenseNovaRelayFailure(c *gin.Context, upstream *types.NewAPIError) *types.NewAPIError {
	stopSenseNovaRecoveryRenewal(c)
	defer finishSenseNovaAdmissionAttempt(c)
	a := getSenseNovaAttempt(c)
	if a == nil || upstream == nil {
		return upstream
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		c.Set(senseNovaRetryContext, false)
		return types.NewErrorWithStatusCode(errors.New("SenseNova request canceled"), types.ErrorCodeBadResponseStatusCode, upstream.StatusCode, types.ErrOptionWithSkipRetry())
	}
	failure := ClassifySenseNovaFailure(upstream)
	// Gateway validation/billing errors are not rejected provider attempts.
	a.failed = failure.State != "" || upstream.GetErrorType() != types.ErrorTypeNewAPIError
	if a.failed {
		a.limitKind = senseNovaLimitKind(upstream)
	}
	c.Set(senseNovaRetryContext, failure.State != "")
	if failure.State != "" {
		value, _ := c.Get(senseNovaExcludedContext)
		set, _ := value.(map[string]bool)
		if set == nil {
			set = make(map[string]bool)
		}
		set[fmt.Sprintf("%d:%s", a.snapshot.ChannelID, a.snapshot.Fingerprint)] = true
		c.Set(senseNovaExcludedContext, set)
		scope := a.model
		if failure.AccountWide {
			scope = ""
		}
		if _, err := model.RecordSenseNovaFailure(a.snapshot, a.key, scope, failure.Reason, failure.State == model.SenseNovaInvalid, a.retryAfter, time.Now().Unix()); err != nil {
			common.SysError("SenseNova failure state persistence failed")
		}
	}
	// Provider errors may echo arbitrary credentials in message/code/metadata.
	// Keep local validation errors intact, but expose only safe provider codes.
	if failure.State != "" || upstream.GetErrorType() != types.ErrorTypeNewAPIError {
		reason := failure.Reason
		if reason == "" {
			reason = "upstream_request_rejected"
		}
		return types.NewErrorWithStatusCode(errors.New("SenseNova: "+reason), types.ErrorCodeBadResponseStatusCode, upstream.StatusCode)
	}
	return upstream
}

func ShouldRetrySenseNova(c *gin.Context, upstream *types.NewAPIError) bool {
	return IsSenseNovaAttempt(c) && upstream != nil && c.GetBool(senseNovaRetryContext)
}

// SenseNovaAttemptLogInfo contains only controlled, admin-only diagnostics.
// Call before selecting another key, which clears the previous attempt.
func SenseNovaAttemptLogInfo(c *gin.Context) map[string]interface{} {
	a := getSenseNovaAttempt(c)
	if a == nil || !a.failed {
		return nil
	}
	info := map[string]interface{}{
		"key_id":       a.snapshot.Fingerprint,
		"attempt":      a.number,
		"max_attempts": SenseNovaMaxAttempts,
		"limit_kind":   a.limitKind,
	}
	if a.retryAfter > 0 {
		info["retry_after_seconds"] = a.retryAfter
	}
	return info
}

// SetSenseNovaRetryAfterHeader is only for the final failed response. Recovery
// times are lower-bound hints, not promises that future requests fit a limit.
func SetSenseNovaRetryAfterHeader(c *gin.Context) {
	if c.Writer.Written() || (c.Request != nil && c.Request.Context().Err() != nil) {
		return
	}
	if hint := c.GetInt64("sensenova_admission_retry_after"); hint > 0 {
		c.Header("Retry-After", strconv.FormatInt(hint, 10))
		return
	}
	value, _ := c.Get(senseNovaSelectionContext)
	selection, _ := value.(*senseNovaSelection)
	if selection == nil {
		return
	}
	a := getSenseNovaAttempt(c)
	if a != nil && !a.failed {
		return
	}
	seconds := int64(0)
	if a != nil {
		seconds = a.retryAfter
	}
	channel, err := model.GetChannelById(selection.channelID, true)
	if err != nil || !channel.SenseNovaPool || channel.Status != common.ChannelStatusEnabled {
		return
	}
	states, err := model.ListSenseNovaStates(channel.Id)
	if err == nil {
		// Ignore removed/manual-disabled keys and other models. For each live
		// identity, account and requested-model restrictions both apply.
		live := make(map[string]int64)
		for index, key := range channel.GetKeys() {
			if status, set := channel.ChannelInfo.MultiKeyStatusList[index]; !set || status == common.ChannelStatusEnabled {
				live[model.SenseNovaFingerprint(key)] = 0
			}
		}
		for _, state := range states {
			if (state.Scope == "" || state.Scope == selection.model) && state.State == model.SenseNovaInvalid {
				delete(live, state.Fingerprint)
			}
		}
		now := time.Now().Unix()
		for _, state := range states {
			if current, exists := live[state.Fingerprint]; exists && (state.Scope == "" || state.Scope == selection.model) && state.State == model.SenseNovaCooling {
				if delay := state.NextProbeAt - now; delay > current {
					live[state.Fingerprint] = delay
				}
			}
		}
		// Retain the current key's known restriction even if its CAS write
		// lost a race. It must not extend another independent key's delay.
		if a != nil {
			if current, exists := live[a.snapshot.Fingerprint]; exists && a.retryAfter > current {
				live[a.snapshot.Fingerprint] = a.retryAfter
			}
		}
		seconds = 0
		first := true
		for _, delay := range live {
			if first || delay < seconds {
				seconds = delay
				first = false
			}
		}
	}
	if seconds > 86400 {
		seconds = 86400
	}
	if seconds > 0 {
		c.Header("Retry-After", strconv.FormatInt(seconds, 10))
	}
}
