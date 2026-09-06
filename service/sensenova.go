package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

type senseNovaRequestContextKey struct{}

func IsSenseNovaRequest(ctx context.Context) bool {
	value, _ := ctx.Value(senseNovaRequestContextKey{}).(bool)
	return value
}

var senseNovaOffsets sync.Map

type senseNovaAttempt struct {
	snapshot *model.SenseNovaSnapshot
	key      string
	model    string
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
	unavailable := func() (string, int, *types.NewAPIError) {
		return "", 0, types.NewErrorWithStatusCode(errors.New("SenseNova pool temporarily has no eligible key"), types.ErrorCodeGetChannelFailed, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	fresh, err := model.GetChannelById(channel.Id, true)
	if err != nil || !fresh.SenseNovaPool || ValidateSenseNovaPool(fresh) != nil || !model.IsSenseNovaModel(name) {
		return unavailable()
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
		snapshot, snapshotErr := model.SenseNovaKeySnapshot(channel.Id, key, name)
		if errors.Is(snapshotErr, model.ErrSenseNovaUnavailable) {
			continue
		}
		if snapshotErr != nil {
			return unavailable()
		}
		c.Set(senseNovaAttemptContext, &senseNovaAttempt{snapshot: snapshot, key: key, model: name})
		if c.Request != nil {
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), senseNovaRequestContextKey{}, true))
		}
		return key, index, nil
	}
	return unavailable()
}

func RecordSenseNovaRelaySuccess(c *gin.Context) {
	if a := getSenseNovaAttempt(c); a != nil {
		if _, err := model.RecordSenseNovaSuccess(a.snapshot, a.key, time.Now().Unix()); err != nil {
			common.SysError("SenseNova success state persistence failed")
		}
	}
}

// Record failures before selecting a retry. The request-local exclusion remains
// effective even if the database cannot persist the outcome.
func RecordSenseNovaRelayFailure(c *gin.Context, upstream *types.NewAPIError) *types.NewAPIError {
	a := getSenseNovaAttempt(c)
	if a == nil || upstream == nil {
		return upstream
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		c.Set(senseNovaRetryContext, false)
		return types.NewErrorWithStatusCode(errors.New("SenseNova request canceled"), types.ErrorCodeBadResponseStatusCode, upstream.StatusCode, types.ErrOptionWithSkipRetry())
	}
	failure := ClassifySenseNovaFailure(upstream)
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
		if _, err := model.RecordSenseNovaFailure(a.snapshot, a.key, scope, failure.Reason, failure.State == model.SenseNovaInvalid, 0, time.Now().Unix()); err != nil {
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
