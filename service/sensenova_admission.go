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
	config                *senseNovaAdmissionConfig
	id                    string
	deadline              time.Time
	channelID             int
	queued                bool
	reservation           *senseNovaBudgetReservation
	dispatched            bool
	successful            bool
	actualTokens          int64
	parentContext         context.Context
	stopRenew             context.CancelFunc
	cancelRequest         context.CancelFunc
	renewDone             chan struct{}
	capacityRequest       senseNovaBudgetRequest
	capacityRecoveryOwner string
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

// senseNovaHealthWait never revives a key. It waits for cooldown expiry,
// recovery ownership, or the health scheduler; only real traffic confirms rate
// recovery, and a tiny probe cannot erase that requirement.
func senseNovaHealthWait(c *gin.Context, channel *model.Channel, name string) (time.Duration, bool) {
	delay, found, _ := senseNovaHealthCapacityWait(c, channel, name, nil, nil)
	return delay, found
}

// Available candidates already have size/budget wait hints. Do not replace
// their capacity penalties with the one-second health recovery polling hint.
func senseNovaHealthCapacityWait(c *gin.Context, channel *model.Channel, name string, available map[string]bool, request *senseNovaBudgetRequest) (time.Duration, bool, bool) {
	pending, err := senseNovaPendingHealthCandidates(c, channel, name)
	if err != nil {
		return 0, false, false
	}
	candidates := make([]senseNovaPendingHealthCandidate, 0, len(pending))
	requests := make([]senseNovaBudgetRequest, 0, len(pending))
	for _, candidate := range pending {
		if available[candidate.fingerprint] {
			continue
		}
		candidates = append(candidates, candidate)
		if request != nil {
			candidateRequest := *request
			candidateRequest.Fingerprint = candidate.fingerprint
			requests = append(requests, candidateRequest)
		}
	}
	if request != nil && len(requests) > 0 {
		observations, readErr := readSenseNovaCapacity(c.Request.Context(), requests)
		if readErr != nil {
			return 0, false, false
		}
		for i, observation := range observations {
			candidates[i].wait = max(candidates[i].wait, observation.RetryAfter)
			candidates[i].minimum = max(candidates[i].minimum, observation.RetryAfter)
		}
		if state := currentSenseNovaAdmission(c); state != nil {
			viable := candidates[:0]
			for i, candidate := range candidates {
				// A health owner may finish early, but cannot erase fixed
				// pacing. Inspect the budget without acquiring an unsent lease.
				wait, minimum, budgetErr := readSenseNovaBudgetWait(c.Request.Context(), requests[i], state.config.policy(candidate.fingerprint))
				if errors.Is(budgetErr, errSenseNovaBudgetTooLarge) {
					continue
				}
				if budgetErr != nil {
					return 0, false, false
				}
				candidate.wait = max(candidate.wait, wait)
				candidate.minimum = max(candidate.minimum, minimum)
				viable = append(viable, candidate)
			}
			candidates = viable
		}
	}
	delay := time.Duration(0)
	mayReleaseEarly := false
	remaining := time.Duration(0)
	if state := currentSenseNovaAdmission(c); state != nil {
		remaining = time.Until(state.deadline)
	}
	for _, candidate := range candidates {
		if delay == 0 || candidate.wait < delay {
			delay = candidate.wait
		}
		if candidate.minimum < candidate.wait && candidate.minimum < remaining {
			// A live verifier may finish early only after this key's fixed
			// health AND request-size cooldowns permit another admission.
			mayReleaseEarly = true
		}
	}
	return delay, len(candidates) > 0, mayReleaseEarly
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
		if attempt.snapshot.NeedsRecovery && attempt.recovery == nil {
			if c.Request == nil || c.Request.Context().Err() != nil || c.Writer.Written() {
				return senseNovaAdmissionError("SenseNova recovery canceled", http.StatusServiceUnavailable)
			}
			snapshot, claimErr := model.ClaimSenseNovaRecovery(attempt.snapshot, attempt.key, time.Now().Unix())
			if claimErr != nil {
				return senseNovaAdmissionError("SenseNova recovery is unavailable", http.StatusServiceUnavailable)
			}
			attempt.snapshot = snapshot
			startSenseNovaRecoveryRenewal(c, attempt)
		}
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
	if state.config.affinity {
		_, request.Conversation = senseNovaConversationIdentity(c, request.ChannelID, request.Model)
	}
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
		ordered, orderErr := senseNovaCapacityCandidates(c, channel, request, attempt.key)
		if orderErr != nil {
			return senseNovaAdmissionError("SenseNova capacity store unavailable", http.StatusServiceUnavailable)
		}
		delay := time.Duration(0)
		mayReleaseEarly, ordinaryWaiting := false, false
		oversized := 0
		available := make(map[string]bool, len(ordered))
		for _, candidate := range ordered {
			available[candidate.request.Fingerprint] = true
		}
		for _, candidate := range ordered {
			key, fingerprint := candidate.key, candidate.request.Fingerprint
			if candidate.observation.RetryAfter > 0 {
				if delay == 0 || candidate.observation.RetryAfter < delay {
					delay = candidate.observation.RetryAfter
				}
				continue
			}
			recovering := candidate.rank == 2
			if recovering && ordinaryWaiting {
				continue
			}
			policy := state.config.policy(fingerprint)
			reservation, wait, fixedMinimum, reserveErr := reserveSenseNovaBudgetWithWait(c.Request.Context(), candidate.request, policy)
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
				// Only a wait whose fixed part fits this request can justify
				// polling or postponing a recovery candidate.
				if fixedMinimum <= time.Until(state.deadline) {
					mayReleaseEarly = mayReleaseEarly || wait > fixedMinimum
					ordinaryWaiting = ordinaryWaiting || !recovering
				}
				continue
			}
			recoveryOwner := ""
			if recovering {
				recoveryOwner = reservation.owner
				claimed, leaseWait, claimErr := claimSenseNovaCapacityRecovery(c.Request.Context(), channel.Id, request.Model, recoveryOwner, policy.Lease)
				if claimErr != nil || !claimed {
					cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					_ = finishSenseNovaBudget(cleanupCtx, reservation, -1, true)
					cancel()
					if claimErr != nil {
						return senseNovaAdmissionError("SenseNova recovery store unavailable", http.StatusServiceUnavailable)
					}
					if leaseWait > 0 && (delay == 0 || leaseWait < delay) {
						delay = leaseWait
					}
					mayReleaseEarly = true
					continue
				}
			}
			// A queued administrative edit must not race an admission into a stale key.
			snapshot, snapshotErr := model.SenseNovaRequestSnapshot(channel.Id, key, request.Model, time.Now().Unix())
			if snapshotErr == nil && snapshot.NeedsRecovery {
				if !recovering {
					// Health changed after ranking. Rerank before taking a global
					// recovery slot, instead of bypassing the recovery bound.
					snapshotErr = model.ErrSenseNovaUnavailable
				} else {
					snapshot, snapshotErr = model.ClaimSenseNovaRecovery(snapshot, key, time.Now().Unix())
				}
			}
			if snapshotErr != nil {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				if recoveryOwner != "" {
					_ = releaseSenseNovaCapacityRecovery(cleanupCtx, channel.Id, request.Model, recoveryOwner)
				}
				_ = finishSenseNovaBudget(cleanupCtx, reservation, -1, true)
				cancel()
				if !errors.Is(snapshotErr, model.ErrSenseNovaUnavailable) {
					return senseNovaAdmissionError("SenseNova health store unavailable", http.StatusServiceUnavailable)
				}
				delete(available, fingerprint)
				continue
			}
			state.channelID = channel.Id
			state.reservation = reservation
			state.dispatched = false
			state.successful = false
			state.actualTokens = -1
			state.capacityRequest = candidate.request
			state.capacityRecoveryOwner = recoveryOwner
			common.SetContextKey(c, constant.ContextKeyLocalCountTokens, false)
			attempt.snapshot = snapshot
			attempt.key = key
			common.SetContextKey(c, constant.ContextKeyChannelKey, key)
			common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, candidate.index)
			common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, channel.ChannelInfo.IsMultiKey)
			c.Set("sensenova_admission_retry_after", int64(0))
			releaseSenseNovaAdmissionQueue(state)
			startSenseNovaBudgetRenewal(c, state)
			startSenseNovaRecoveryRenewal(c, attempt)
			return nil
		}
		healthDelay, cooling, healthMayReleaseEarly := senseNovaHealthCapacityWait(c, channel, request.Model, available, &request)
		if len(ordered) > 0 && len(ordered) == oversized && !cooling {
			return senseNovaAdmissionError("SenseNova request exceeds configured TPM budget", http.StatusTooManyRequests)
		}
		cause := SenseNovaWaitCapacity
		if cooling && (delay == 0 || healthDelay < delay) {
			delay = healthDelay
			cause = SenseNovaWaitHealth
		}
		mayReleaseEarly = mayReleaseEarly || healthMayReleaseEarly
		if delay == 0 {
			return senseNovaAdmissionError("SenseNova pool has no eligible capacity", http.StatusServiceUnavailable)
		}
		if !mayReleaseEarly && delay > time.Until(state.deadline) {
			c.Set("sensenova_admission_retry_after", min(int64(math.Ceil(delay.Seconds())), int64(86400)))
			return senseNovaAdmissionError("SenseNova capacity cannot recover within this request's wait budget", http.StatusServiceUnavailable)
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
	recoveryOwner, capacityRequest := state.capacityRecoveryOwner, state.capacityRequest
	go func() {
		defer close(done)
		runSenseNovaBudgetRenewal(renewalContext, cancelRequest, reservation, recoveryOwner, capacityRequest, 30*time.Second)
	}()
}

func runSenseNovaBudgetRenewal(renewalContext context.Context, cancelRequest context.CancelFunc, reservation *senseNovaBudgetReservation, recoveryOwner string, capacityRequest senseNovaBudgetRequest, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-renewalContext.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(renewalContext, 2*time.Second)
			err := renewSenseNovaBudget(ctx, reservation)
			if err == nil && recoveryOwner != "" {
				var owned bool
				owned, err = renewSenseNovaCapacityRecovery(ctx, capacityRequest.ChannelID, capacityRequest.Model, recoveryOwner, reservation.lease)
				if err == nil && !owned {
					err = errSenseNovaBudgetLeaseLost
				}
			}
			cancel()
			// Stopping the worker interrupts Redis too; only a failure while
			// renewal is still live should cancel dispatch or response reads.
			if renewalContext.Err() != nil {
				return
			}
			if err != nil {
				common.SysError("SenseNova admission lease renewal failed")
				cancelRequest()
				return
			}
		}
	}
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
	stopSenseNovaRecoveryRenewal(c)
	if attempt := getSenseNovaAttempt(c); attempt != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := model.ReleaseSenseNovaRecovery(cleanupCtx, attempt.snapshot, attempt.key)
		cancel()
		if err != nil {
			common.SysError("SenseNova recovery lease release failed")
		}
	}
	state := currentSenseNovaAdmission(c)
	if state == nil || state.reservation == nil {
		return
	}
	requestCanceled := c.Request == nil || c.Request.Context().Err() != nil
	if state.stopRenew != nil {
		state.stopRenew()
		<-state.renewDone
		requestCanceled = requestCanceled || c.Request.Context().Err() != nil
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
	if state.dispatched && !requestCanceled {
		attempt := getSenseNovaAttempt(c)
		tpm, retryAfter := false, int64(0)
		if attempt != nil {
			tpm, retryAfter = attempt.limitKind == "tpm", attempt.retryAfter
		}
		if err := recordSenseNovaCapacity(ctx, state.reservation, state.capacityRequest, state.successful, tpm, retryAfter); err != nil {
			common.SysError("SenseNova capacity observation failed")
		}
	}
	if state.capacityRecoveryOwner != "" {
		if err := releaseSenseNovaCapacityRecovery(ctx, state.capacityRequest.ChannelID, state.capacityRequest.Model, state.capacityRecoveryOwner); err != nil {
			common.SysError("SenseNova capacity recovery cleanup failed")
		}
		state.capacityRecoveryOwner = ""
	}
	actualTokens := int64(-1)
	if state.successful {
		actualTokens = state.actualTokens
	}
	verified := state.dispatched && state.successful && !requestCanceled
	published, finishErr := finishSenseNovaBudgetOutcome(ctx, state.reservation, actualTokens, !state.dispatched, verified)
	if finishErr != nil {
		common.SysError("SenseNova admission completion failed")
	}
	if published && state.config.affinity {
		scope, session := senseNovaConversationIdentity(c, state.capacityRequest.ChannelID, state.capacityRequest.Model)
		if _, err := senseNovaConversationPreference(ctx, scope, session, state.capacityRequest.Fingerprint); err != nil {
			common.SysError("SenseNova conversation preference update failed")
		}
	}
	state.reservation = nil
}

// FinishSenseNovaAdmission also handles validation or billing failures that
// occur after a reservation but before the transport marks actual dispatch.
func FinishSenseNovaAdmission(c *gin.Context) {
	finishSenseNovaAdmissionAttempt(c)
	releaseSenseNovaAdmissionQueue(currentSenseNovaAdmission(c))
}
