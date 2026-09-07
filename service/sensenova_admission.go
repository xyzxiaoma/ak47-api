package service

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const senseNovaAdmissionContextKey = "sensenova_admission"

type senseNovaAdmission struct {
	config        *senseNovaAdmissionConfig
	id            string
	deadline      time.Time
	channelID     int
	queued        bool
	reservation   *senseNovaBudgetReservation
	dispatched    bool
	successful    bool
	actualTokens  int64
	parentContext context.Context
	stopRenew     context.CancelFunc
	cancelRequest context.CancelFunc
	renewDone     chan struct{}
}

func senseNovaAdmissionState(c *gin.Context, name string) (*senseNovaAdmission, error) {
	if value, exists := c.Get(senseNovaAdmissionContextKey); exists {
		state, _ := value.(*senseNovaAdmission)
		return state, nil
	}
	cfg, err := senseNovaAdmissionConfigFor(name)
	if err != nil || cfg == nil {
		return nil, err
	}
	started := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if started.IsZero() {
		started = time.Now()
	}
	state := &senseNovaAdmission{config: cfg, id: uuid.NewString(), deadline: started.Add(cfg.wait), actualTokens: -1}
	c.Set(senseNovaAdmissionContextKey, state)
	return state, nil
}

func currentSenseNovaAdmission(c *gin.Context) *senseNovaAdmission {
	value, _ := c.Get(senseNovaAdmissionContextKey)
	state, _ := value.(*senseNovaAdmission)
	return state
}

func senseNovaAdmissionError(message string, status int) *types.NewAPIError {
	return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeGetChannelFailed, status, types.ErrOptionWithSkipRetry())
}

// waitSenseNovaAdmission holds a shared queue lease only while no key can
// accept this request. The single deadline is reused across selection/retries.
func waitSenseNovaAdmission(c *gin.Context, state *senseNovaAdmission, channelID int, delay time.Duration, cause SenseNovaWaitCause) *types.NewAPIError {
	if c.Request == nil || c.Request.Context().Err() != nil || c.Writer.Written() {
		return senseNovaAdmissionError("SenseNova admission canceled", http.StatusServiceUnavailable)
	}
	remaining := time.Until(state.deadline)
	if delay > 0 {
		seconds := int64(math.Ceil(delay.Seconds()))
		if seconds > 86400 {
			seconds = 86400
		}
		c.Set("sensenova_admission_retry_after", seconds)
	}
	if remaining <= 0 {
		return senseNovaAdmissionError("SenseNova capacity wait timed out", http.StatusServiceUnavailable)
	}
	if !common.RedisEnabled || common.RDB == nil {
		return senseNovaAdmissionError("SenseNova admission store unavailable", http.StatusServiceUnavailable)
	}
	if !state.queued {
		acquired, err := acquireSenseNovaQueue(c.Request.Context(), channelID, state.id, state.config.queueLimit, remaining+5*time.Second)
		if err != nil {
			return senseNovaAdmissionError("SenseNova admission store unavailable", http.StatusServiceUnavailable)
		}
		if !acquired {
			return senseNovaAdmissionError("SenseNova capacity queue is full", http.StatusTooManyRequests)
		}
		state.channelID = channelID
		state.queued = true
	}
	if delay <= 0 || delay > 2*time.Second {
		delay = 2 * time.Second
	}
	if delay > remaining {
		delay = remaining
	}
	started := time.Now()
	defer func() { RecordSenseNovaAdmissionWait(c, cause, time.Since(started)) }()
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-c.Request.Context().Done():
		return senseNovaAdmissionError("SenseNova admission canceled", http.StatusServiceUnavailable)
	case <-timer.C:
		return nil
	}
}

func releaseSenseNovaAdmissionQueue(state *senseNovaAdmission) {
	if state == nil || !state.queued {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := releaseSenseNovaQueue(ctx, state.channelID, state.id); err != nil {
		common.SysError("SenseNova admission queue cleanup failed")
	}
	state.queued = false
}

// senseNovaHealthWait never revives a key: only the existing recovery
// scheduler or a valid concurrent success can make a cooling key eligible.
func senseNovaHealthWait(c *gin.Context, channel *model.Channel, name string) (time.Duration, bool) {
	states, err := model.ListSenseNovaStates(channel.Id)
	if err != nil {
		return 0, false
	}
	excludedValue, _ := c.Get(senseNovaExcludedContext)
	excluded, _ := excludedValue.(map[string]bool)
	delay := time.Duration(0)
	found := false
	now := time.Now().Unix()
	for index, key := range channel.GetKeys() {
		if status, ok := channel.ChannelInfo.MultiKeyStatusList[index]; ok && status != common.ChannelStatusEnabled {
			continue
		}
		fingerprint := model.SenseNovaFingerprint(key)
		if excluded[strconv.Itoa(channel.Id)+":"+fingerprint] {
			continue
		}
		cooling, invalid := false, false
		keyDelay := time.Second
		for _, state := range states {
			if state.Fingerprint != fingerprint || (state.Scope != "" && state.Scope != name) {
				continue
			}
			if state.State == model.SenseNovaInvalid {
				invalid = true
			}
			if state.State == model.SenseNovaCooling {
				cooling = true
				if candidate := time.Duration(state.NextProbeAt-now) * time.Second; candidate > keyDelay {
					keyDelay = candidate
				}
			}
		}
		if cooling && !invalid && (!found || keyDelay < delay) {
			delay = keyDelay
			found = true
		}
	}
	return delay, found
}

// AdmitSenseNovaAttempt can change the chosen key before dispatch, but never
// the authorized channel, model, pricing group or upstream attempt number.
func AdmitSenseNovaAttempt(c *gin.Context) *types.NewAPIError {
	attempt := getSenseNovaAttempt(c)
	if attempt == nil {
		return nil
	}
	state, err := senseNovaAdmissionState(c, attempt.model)
	if err != nil {
		return senseNovaAdmissionError("Invalid SenseNova admission configuration", http.StatusServiceUnavailable)
	}
	if state == nil {
		return nil
	}
	if state.reservation != nil {
		return nil
	}
	if !common.RedisEnabled || common.RDB == nil {
		return senseNovaAdmissionError("SenseNova admission store unavailable", http.StatusServiceUnavailable)
	}
	value, _ := c.Get("sensenova_request_metrics")
	metrics, _ := value.(map[string]interface{})
	estimated, ok := metrics["estimated_prompt_tokens"].(int)
	if !ok || estimated < 0 {
		return senseNovaAdmissionError("SenseNova token estimate unavailable", http.StatusServiceUnavailable)
	}
	request := senseNovaBudgetRequest{ChannelID: attempt.snapshot.ChannelID, Model: attempt.model, ID: state.id + "-" + strconv.Itoa(attempt.number), PromptTokens: int64(estimated)}
	for _, field := range []string{"requested_max_tokens", "requested_max_completion_tokens", "requested_max_tokens_to_sample", "requested_max_output_tokens"} {
		if ceiling, ok := metrics[field].(uint); ok {
			if uint64(ceiling) > math.MaxInt64 {
				return senseNovaAdmissionError("SenseNova output budget is too large", http.StatusTooManyRequests)
			}
			if !request.HasOutputLimit || int64(ceiling) > request.OutputTokens {
				request.OutputTokens = int64(ceiling)
			}
			request.HasOutputLimit = true
		}
	}
	for {
		if c.Request == nil || c.Request.Context().Err() != nil || c.Writer.Written() {
			return senseNovaAdmissionError("SenseNova admission canceled", http.StatusServiceUnavailable)
		}
		channel, err := model.GetChannelById(request.ChannelID, true)
		if err != nil || channel.Status != common.ChannelStatusEnabled || !channel.SenseNovaPool || ValidateSenseNovaPool(channel) != nil || !containsSenseNovaModel(channel, request.Model) {
			return senseNovaAdmissionError("SenseNova pool is unavailable", http.StatusServiceUnavailable)
		}
		keys := channel.GetKeys()
		start := 0
		for i, key := range keys {
			if key == attempt.key {
				start = i
				break
			}
		}
		excludedValue, _ := c.Get(senseNovaExcludedContext)
		excluded, _ := excludedValue.(map[string]bool)
		delay := time.Duration(0)
		candidates, oversized := 0, 0
		for offset := range keys {
			index := (start + offset) % len(keys)
			key := keys[index]
			if status, ok := channel.ChannelInfo.MultiKeyStatusList[index]; ok && status != common.ChannelStatusEnabled {
				continue
			}
			fingerprint := model.SenseNovaFingerprint(key)
			if excluded[strconv.Itoa(channel.Id)+":"+fingerprint] {
				continue
			}
			snapshot, snapshotErr := model.SenseNovaKeySnapshot(channel.Id, key, request.Model)
			if errors.Is(snapshotErr, model.ErrSenseNovaUnavailable) {
				continue
			}
			if snapshotErr != nil {
				return senseNovaAdmissionError("SenseNova health store unavailable", http.StatusServiceUnavailable)
			}
			candidates++
			request.Fingerprint = fingerprint
			reservation, wait, reserveErr := reserveSenseNovaBudget(c.Request.Context(), request, state.config.policy(fingerprint))
			if errors.Is(reserveErr, errSenseNovaBudgetTooLarge) {
				oversized++
				continue
			}
			if reserveErr != nil {
				return senseNovaAdmissionError("SenseNova admission store unavailable", http.StatusServiceUnavailable)
			}
			if reservation == nil {
				if wait > 0 && (delay == 0 || wait < delay) {
					delay = wait
				}
				continue
			}
			// A queued administrative edit must not race an admission into a stale key.
			snapshot, snapshotErr = model.SenseNovaKeySnapshot(channel.Id, key, request.Model)
			if snapshotErr != nil {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_ = finishSenseNovaBudget(cleanupCtx, reservation, -1, true)
				cancel()
				if !errors.Is(snapshotErr, model.ErrSenseNovaUnavailable) {
					return senseNovaAdmissionError("SenseNova health store unavailable", http.StatusServiceUnavailable)
				}
				continue
			}
			state.channelID = channel.Id
			state.reservation = reservation
			state.dispatched = false
			state.successful = false
			state.actualTokens = -1
			common.SetContextKey(c, constant.ContextKeyLocalCountTokens, false)
			attempt.snapshot = snapshot
			attempt.key = key
			common.SetContextKey(c, constant.ContextKeyChannelKey, key)
			common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, index)
			common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, channel.ChannelInfo.IsMultiKey)
			c.Set("sensenova_admission_retry_after", int64(0))
			releaseSenseNovaAdmissionQueue(state)
			startSenseNovaBudgetRenewal(c, state)
			return nil
		}
		healthDelay, cooling := senseNovaHealthWait(c, channel, request.Model)
		if candidates > 0 && candidates == oversized && !cooling {
			return senseNovaAdmissionError("SenseNova request exceeds configured TPM budget", http.StatusTooManyRequests)
		}
		cause := SenseNovaWaitCapacity
		if cooling && (delay == 0 || healthDelay < delay) {
			delay = healthDelay
			cause = SenseNovaWaitHealth
		}
		if delay == 0 {
			return senseNovaAdmissionError("SenseNova pool has no eligible capacity", http.StatusServiceUnavailable)
		}
		if waitErr := waitSenseNovaAdmission(c, state, channel.Id, delay, cause); waitErr != nil {
			return waitErr
		}
	}
}

func containsSenseNovaModel(channel *model.Channel, name string) bool {
	for _, candidate := range strings.Split(channel.Models, ",") {
		if strings.TrimSpace(candidate) == name {
			return true
		}
	}
	return false
}

func startSenseNovaBudgetRenewal(c *gin.Context, state *senseNovaAdmission) {
	state.parentContext = c.Request.Context()
	requestContext, cancelRequest := context.WithCancel(state.parentContext)
	renewalContext, stop := context.WithCancel(state.parentContext)
	state.cancelRequest = cancelRequest
	state.stopRenew = stop
	state.renewDone = make(chan struct{})
	c.Request = c.Request.WithContext(requestContext)
	reservation, done := state.reservation, state.renewDone
	go func() {
		defer close(done)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-renewalContext.Done():
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(renewalContext, 2*time.Second)
				err := renewSenseNovaBudget(ctx, reservation)
				cancel()
				if err != nil {
					common.SysError("SenseNova admission lease renewal failed")
					cancelRequest()
					return
				}
			}
		}
	}()
}

func MarkSenseNovaBudgetDispatched(c *gin.Context) {
	if state := currentSenseNovaAdmission(c); state != nil && state.reservation != nil {
		state.dispatched = true
	}
}

func ObserveSenseNovaUsage(c *gin.Context, usage *dto.Usage) {
	state := currentSenseNovaAdmission(c)
	if state == nil || state.reservation == nil || usage == nil || common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens) {
		return
	}
	// Locally reconstructed usage omits unobserved generation (for example
	// reasoning and tool arguments). Only measured usage may refund capacity.
	prompt, completion := int64(usage.PromptTokens), int64(usage.CompletionTokens)
	if prompt < 0 || completion < 0 || prompt > math.MaxInt64-completion {
		return
	}
	actual := prompt + completion
	if int64(usage.TotalTokens) > actual {
		actual = int64(usage.TotalTokens)
	}
	if actual > 0 {
		state.actualTokens = actual
	}
}

func finishSenseNovaAdmissionAttempt(c *gin.Context) {
	state := currentSenseNovaAdmission(c)
	if state == nil || state.reservation == nil {
		return
	}
	if state.stopRenew != nil {
		state.stopRenew()
		<-state.renewDone
		requestCanceled := c.Request.Context().Err() != nil
		state.cancelRequest()
		// Preserve cancellation from lease loss or renewal failure so cleanup
		// cannot turn a terminated request into a fresh retry on another key.
		if !requestCanceled {
			c.Request = c.Request.WithContext(state.parentContext)
		}
		state.stopRenew = nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	actualTokens := int64(-1)
	if state.successful {
		actualTokens = state.actualTokens
	}
	if err := finishSenseNovaBudget(ctx, state.reservation, actualTokens, !state.dispatched); err != nil {
		common.SysError("SenseNova admission completion failed")
	}
	state.reservation = nil
}

// FinishSenseNovaAdmission also handles validation or billing failures that
// occur after a reservation but before the transport marks actual dispatch.
func FinishSenseNovaAdmission(c *gin.Context) {
	finishSenseNovaAdmissionAttempt(c)
	releaseSenseNovaAdmissionQueue(currentSenseNovaAdmission(c))
}
