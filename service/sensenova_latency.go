package service

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

const senseNovaLatencyContext = "sensenova_latency"

type SenseNovaWaitCause uint8

const (
	SenseNovaWaitHealth SenseNovaWaitCause = iota + 1
	SenseNovaWaitCapacity
)

// Durations use Go's monotonic clock. Dispatch/finish are offsets from request
// start; header/semantic/answer timings are offsets from that attempt's dispatch.
// Waits are disjoint actual sleeps. RetryWaitMS is a SUBSET of TotalWaitMS.
// TotalMS on a log snapshot includes everything so far (including attempts and
// waits); Complete distinguishes the final request snapshot from consume/error
// snapshots taken before billing/cleanup finishes. Never add these overlapping
// measurements together. Non-stream output is observed after decoding the body.
type SenseNovaLatencyLog struct {
	TotalMS        int64                        `json:"total_ms"`
	Complete       bool                         `json:"complete"`
	Outcome        string                       `json:"outcome,omitempty"`
	TotalWaitMS    int64                        `json:"total_wait_ms"`
	HealthWaitMS   int64                        `json:"health_wait_ms"`
	CapacityWaitMS int64                        `json:"capacity_wait_ms"`
	RetryWaitMS    int64                        `json:"retry_wait_ms"`
	Attempts       []SenseNovaAttemptLatencyLog `json:"attempts,omitempty"`
}

type SenseNovaAttemptLatencyLog struct {
	Attempt         int    `json:"attempt"`
	KeyID           string `json:"key_id"`
	DispatchMS      int64  `json:"dispatch_ms"`
	HeadersMS       *int64 `json:"headers_ms,omitempty"`
	FirstSemanticMS *int64 `json:"first_semantic_ms,omitempty"`
	FirstAnswerMS   *int64 `json:"first_answer_text_ms,omitempty"`
	FinishMS        *int64 `json:"finish_ms,omitempty"`
	DurationMS      *int64 `json:"duration_ms,omitempty"`
	Outcome         string `json:"outcome,omitempty"`
}

type senseNovaLatencyAttempt struct {
	upstreamError                               bool
	keyID                                       string
	dispatch, headers, semantic, answer, finish time.Time
	outcome                                     string
}

type senseNovaLatency struct {
	mu                                  sync.Mutex
	start, finish                       time.Time
	healthWait, capacityWait, retryWait time.Duration
	attempts                            []*senseNovaLatencyAttempt
	outcome                             string
	now                                 func() time.Time
}

// InitSenseNovaLatency is called by opted-in selection before any health sleep.
// It is idempotent and survives ResetSenseNovaAttempt and key reselection.
func InitSenseNovaLatency(c *gin.Context) {
	if c == nil || senseNovaLatencyState(c) != nil {
		return
	}
	start := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if start.IsZero() {
		start = time.Now()
	}
	c.Set(senseNovaLatencyContext, &senseNovaLatency{start: start, now: time.Now})
}

func senseNovaLatencyState(c *gin.Context) *senseNovaLatency {
	if c == nil {
		return nil
	}
	v, _ := c.Get(senseNovaLatencyContext)
	s, _ := v.(*senseNovaLatency)
	return s
}

// RecordSenseNovaAdmissionWait records one actual sleep, including interrupted
// sleeps. Callers must not also record the enclosing admission duration.
func RecordSenseNovaAdmissionWait(c *gin.Context, cause SenseNovaWaitCause, elapsed time.Duration) {
	if elapsed <= 0 || (cause != SenseNovaWaitHealth && cause != SenseNovaWaitCapacity) {
		return
	}
	s := senseNovaLatencyState(c)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.finish.IsZero() {
		return
	}
	if cause == SenseNovaWaitHealth {
		s.healthWait += elapsed
	} else {
		s.capacityWait += elapsed
	}
	if len(s.attempts) > 0 && !s.attempts[len(s.attempts)-1].finish.IsZero() {
		s.retryWait += elapsed
	}
}

// MarkSenseNovaLatencyDispatch counts actual client.Do invocations, not key
// selections or budget reservations. Its returned callback is attempt-bound.
func MarkSenseNovaLatencyDispatch(c *gin.Context) func() {
	s := senseNovaLatencyState(c)
	a := getSenseNovaAttempt(c)
	if s == nil || a == nil {
		return func() {}
	}
	s.mu.Lock()
	if len(s.attempts) >= SenseNovaMaxAttempts || !s.finish.IsZero() {
		s.mu.Unlock()
		return func() {}
	}
	attempt := &senseNovaLatencyAttempt{keyID: model.SenseNovaFingerprint(a.key), dispatch: s.now()}
	s.attempts = append(s.attempts, attempt)
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if attempt.headers.IsZero() && attempt.finish.IsZero() {
			attempt.headers = s.now()
		}
	}
}

// SenseNovaStreamLatencyObserver captures the exact attempt before scanner
// goroutines start. It retains no content and never buffers or changes SSE.
func SenseNovaStreamLatencyObserver(c *gin.Context) func(string) {
	s := senseNovaLatencyState(c)
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if len(s.attempts) == 0 {
		s.mu.Unlock()
		return nil
	}
	a := s.attempts[len(s.attempts)-1]
	s.mu.Unlock()
	return func(data string) {
		s.mu.Lock()
		done := !a.finish.IsZero()
		s.mu.Unlock()
		if done {
			return
		}
		// Decode only semantic fields. Error objects, usage, heartbeat, role,
		// tool IDs/names and finish-only chunks cannot become first content.
		var chunk struct {
			Error   any `json:"error"`
			Choices []struct {
				Delta dto.ChatCompletionsStreamResponseChoiceDelta `json:"delta"`
			} `json:"choices"`
		}
		if common.UnmarshalJsonStr(data, &chunk) != nil {
			return
		}
		// Continue recognizing errors after first output: legacy Chat/Claude
		// handlers can return nil for an error envelope followed by [DONE].
		if chunk.Error != nil {
			s.observeUpstreamError(a)
			return
		}
		semantic, answer := false, false
		for _, choice := range chunk.Choices {
			delta := choice.Delta
			answer = answer || delta.GetContentString() != ""
			semantic = semantic || answer || delta.GetReasoningContent() != ""
			for _, tool := range delta.ToolCalls {
				semantic = semantic || tool.Function.Arguments != ""
			}
		}
		s.observeContent(a, semantic, answer)
	}
}

func (s *senseNovaLatency) observeUpstreamError(a *senseNovaLatencyAttempt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.finish.IsZero() {
		a.upstreamError = true
	}
}

func (s *senseNovaLatency) observeContent(a *senseNovaLatencyAttempt, semantic, answer bool) {
	if !semantic && !answer {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !a.finish.IsZero() {
		return
	}
	now := s.now()
	if semantic && a.semantic.IsZero() {
		a.semantic = now
	}
	if answer && a.answer.IsZero() {
		a.answer = now
	}
}

func ObserveSenseNovaCompletionLatency(c *gin.Context, response *dto.OpenAITextResponse) {
	s := senseNovaLatencyState(c)
	if s == nil || response == nil {
		return
	}
	s.mu.Lock()
	if len(s.attempts) == 0 {
		s.mu.Unlock()
		return
	}
	a := s.attempts[len(s.attempts)-1]
	s.mu.Unlock()
	if response.Error != nil {
		s.observeUpstreamError(a)
		return
	}
	semantic, answer := false, false
	for _, choice := range response.Choices {
		answer = answer || choice.Message.StringContent() != ""
		semantic = semantic || answer || choice.Message.GetReasoningContent() != ""
		for _, tool := range choice.Message.ParseToolCalls() {
			semantic = semantic || tool.Function.Arguments != ""
		}
	}
	s.observeContent(a, semantic, answer)
}

// FinishSenseNovaLatencyAttempt runs before consume logging (at DoResponse
// return) and again at the controller for transport/status/validation failures.
// First finish wins, so retries and settlement cannot move the upstream clock.
func FinishSenseNovaLatencyAttempt(c *gin.Context, info *relaycommon.RelayInfo, failed bool) {
	s := senseNovaLatencyState(c)
	if s == nil {
		return
	}
	outcome := "success"
	if failed {
		outcome = "error"
	}
	if info != nil && info.IsStream && info.StreamStatus != nil && (!info.StreamStatus.IsNormalEnd() || info.StreamStatus.HasErrors()) {
		outcome = "stream_error"
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		outcome = "canceled"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.attempts) == 0 {
		return
	}
	a := s.attempts[len(s.attempts)-1]
	if !a.finish.IsZero() {
		return
	}
	if a.upstreamError && outcome != "canceled" {
		outcome = "error"
		if info != nil && info.IsStream {
			outcome = "stream_error"
		}
	}
	a.finish, a.outcome = s.now(), outcome
}

func SenseNovaLatencyLogInfo(c *gin.Context) *SenseNovaLatencyLog {
	s := senseNovaLatencyState(c)
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	end := s.finish
	if end.IsZero() {
		end = s.now()
	}
	health, capacity := s.healthWait.Milliseconds(), s.capacityWait.Milliseconds()
	// Round the aggregate after summing actual durations. Independently
	// truncated buckets can differ from their total by one millisecond.
	result := &SenseNovaLatencyLog{TotalMS: end.Sub(s.start).Milliseconds(), Complete: !s.finish.IsZero(), Outcome: s.outcome,
		HealthWaitMS: health, CapacityWaitMS: capacity, TotalWaitMS: (s.healthWait + s.capacityWait).Milliseconds(), RetryWaitMS: s.retryWait.Milliseconds()}
	for i, a := range s.attempts {
		result.Attempts = append(result.Attempts, SenseNovaAttemptLatencyLog{Attempt: i + 1, KeyID: a.keyID,
			DispatchMS: a.dispatch.Sub(s.start).Milliseconds(), HeadersMS: senseNovaElapsedMS(a.dispatch, a.headers),
			FirstSemanticMS: senseNovaElapsedMS(a.dispatch, a.semantic), FirstAnswerMS: senseNovaElapsedMS(a.dispatch, a.answer),
			FinishMS: senseNovaElapsedMS(s.start, a.finish), DurationMS: senseNovaElapsedMS(a.dispatch, a.finish), Outcome: a.outcome})
	}
	return result
}

func senseNovaElapsedMS(start, end time.Time) *int64 {
	if end.IsZero() {
		return nil
	}
	value := end.Sub(start).Milliseconds()
	return &value
}

// CompleteSenseNovaLatency emits one final administrator diagnostic, including
// requests rejected before dispatch or consume logging. Only allowlisted
// timings/outcomes and fingerprints are serialized; no upstream/client data.
func CompleteSenseNovaLatency(c *gin.Context, failed bool) {
	s := senseNovaLatencyState(c)
	if s == nil {
		return
	}
	FinishSenseNovaLatencyAttempt(c, nil, failed)
	s.mu.Lock()
	if !s.finish.IsZero() {
		s.mu.Unlock()
		return
	}
	s.finish, s.outcome = s.now(), "success"
	if failed {
		s.outcome = "error"
	}
	if len(s.attempts) > 0 {
		lastOutcome := s.attempts[len(s.attempts)-1].outcome
		if lastOutcome != "" && lastOutcome != "success" {
			s.outcome = lastOutcome
		}
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		s.outcome = "canceled"
	}
	s.mu.Unlock()
	data, err := common.Marshal(map[string]any{"admin_info": map[string]any{"sensenova_latency": SenseNovaLatencyLogInfo(c)}})
	if err == nil {
		logger.LogInfo(c, "SenseNova request latency: "+string(data))
	}
}
