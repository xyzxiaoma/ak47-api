package service

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func senseNovaLatencyFixture(t *testing.T) (*gin.Context, func(time.Duration)) {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	start := time.Unix(1000, 0)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, start)
	InitSenseNovaLatency(c)
	s := senseNovaLatencyState(c)
	now := start
	s.now = func() time.Time { return now }
	c.Set(senseNovaAttemptContext, &senseNovaAttempt{key: "secret-api-key"})
	return c, func(offset time.Duration) { now = start.Add(offset) }
}

func TestSenseNovaLatencyWaitRetryAndClockOrigins(t *testing.T) {
	c, at := senseNovaLatencyFixture(t)
	// Initial health wait exists before an attempt; selection resets preserve it.
	ResetSenseNovaAttempt(c)
	RecordSenseNovaAdmissionWait(c, SenseNovaWaitHealth, 10*time.Millisecond)
	InitSenseNovaLatency(c)
	c.Set(senseNovaAttemptContext, &senseNovaAttempt{key: "first-secret"})
	at(20 * time.Millisecond)
	headers := MarkSenseNovaLatencyDispatch(c)
	oldObserver := SenseNovaStreamLatencyObserver(c)
	at(25 * time.Millisecond)
	headers()
	at(30 * time.Millisecond)
	FinishSenseNovaLatencyAttempt(c, nil, true)
	RecordSenseNovaAdmissionWait(c, SenseNovaWaitCapacity, 40*time.Millisecond)
	ResetSenseNovaAttempt(c)
	RecordSenseNovaAdmissionWait(c, SenseNovaWaitHealth, 10*time.Millisecond)
	c.Set(senseNovaAttemptContext, &senseNovaAttempt{key: "second-secret"})
	at(85 * time.Millisecond)
	MarkSenseNovaLatencyDispatch(c)()
	at(90 * time.Millisecond)
	oldObserver(`{"choices":[{"delta":{"content":"late old attempt"}}]}`)
	observer := SenseNovaStreamLatencyObserver(c)
	observer(`{"choices":[{"delta":{"reasoning_content":"private reasoning"}}]}`)
	at(110 * time.Millisecond)
	observer(`{"choices":[{"delta":{"content":"private answer"}}]}`)
	at(120 * time.Millisecond)
	FinishSenseNovaLatencyAttempt(c, nil, false)
	snapshot := SenseNovaLatencyLogInfo(c)
	assert.False(t, snapshot.Complete)
	assert.EqualValues(t, 120, snapshot.TotalMS)
	assert.EqualValues(t, 60, snapshot.TotalWaitMS)
	assert.EqualValues(t, 20, snapshot.HealthWaitMS)
	assert.EqualValues(t, 40, snapshot.CapacityWaitMS)
	assert.EqualValues(t, 50, snapshot.RetryWaitMS)
	require.Len(t, snapshot.Attempts, 2)
	first, second := snapshot.Attempts[0], snapshot.Attempts[1]
	assert.EqualValues(t, 20, first.DispatchMS)
	assert.Equal(t, common.GetPointer(int64(5)), first.HeadersMS)
	assert.Equal(t, common.GetPointer(int64(30)), first.FinishMS)
	assert.Equal(t, common.GetPointer(int64(10)), first.DurationMS)
	assert.Nil(t, first.FirstSemanticMS)
	assert.Nil(t, first.FirstAnswerMS)
	assert.Equal(t, "error", first.Outcome)
	assert.EqualValues(t, 85, second.DispatchMS)
	assert.Equal(t, common.GetPointer(int64(0)), second.HeadersMS, "observed zero must differ from absent")
	assert.Equal(t, common.GetPointer(int64(5)), second.FirstSemanticMS)
	assert.Equal(t, common.GetPointer(int64(25)), second.FirstAnswerMS)
	assert.Equal(t, common.GetPointer(int64(120)), second.FinishMS)
	assert.Equal(t, "success", second.Outcome)
	at(130 * time.Millisecond)
	CompleteSenseNovaLatency(c, false)
	final := SenseNovaLatencyLogInfo(c)
	assert.True(t, final.Complete)
	assert.EqualValues(t, 130, final.TotalMS)
	assert.EqualValues(t, 120, snapshot.TotalMS, "previous snapshots must remain immutable")
	at(150 * time.Millisecond)
	CompleteSenseNovaLatency(c, true)
	assert.Equal(t, final, SenseNovaLatencyLogInfo(c), "completion is idempotent")
	data, err := common.Marshal(final)
	require.NoError(t, err)
	for _, secret := range []string{"first-secret", "second-secret", "private", "late old attempt"} {
		assert.NotContains(t, string(data), secret)
	}
}

func TestSenseNovaLatencySemanticEventsAndAbsence(t *testing.T) {
	for _, tc := range []struct {
		name, data       string
		semantic, answer bool
	}{
		{"heartbeat", `: ping`, false, false},
		{"empty", ``, false, false},
		{"role", `{"choices":[{"delta":{"role":"assistant","content":""}}]}`, false, false},
		{"usage", `{"choices":[],"usage":{"total_tokens":22}}`, false, false},
		{"finish", `{"choices":[{"delta":{},"finish_reason":"stop"}]}`, false, false},
		{"tool name", `{"choices":[{"delta":{"tool_calls":[{"id":"private-id","function":{"name":"private-name","arguments":""}}]}}]}`, false, false},
		{"tool arguments", `{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"{"}}]}}]}`, true, false},
		{"reasoning", `{"choices":[{"delta":{"reasoning_content":"private"}}]}`, true, false},
		{"reasoning alias", `{"choices":[{"delta":{"reasoning":"private"}}]}`, true, false},
		{"answer", `{"choices":[{"delta":{"content":"private"}}]}`, true, true},
		{"error", `{"error":{"message":"private"},"choices":[{"delta":{"content":"ignored"}}]}`, false, false},
		{"invalid", `{"choices":`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, at := senseNovaLatencyFixture(t)
			MarkSenseNovaLatencyDispatch(c)
			at(5 * time.Millisecond)
			SenseNovaStreamLatencyObserver(c)(tc.data)
			entry := SenseNovaLatencyLogInfo(c).Attempts[0]
			assert.Equal(t, tc.semantic, entry.FirstSemanticMS != nil)
			assert.Equal(t, tc.answer, entry.FirstAnswerMS != nil)
			data, err := common.Marshal(entry)
			require.NoError(t, err)
			assert.NotContains(t, string(data), "private")
			assert.NotContains(t, string(data), "headers_ms")
			assert.NotContains(t, string(data), "finish_ms")
			if !tc.semantic {
				assert.NotContains(t, string(data), "first_semantic_ms")
			}
			if !tc.answer {
				assert.NotContains(t, string(data), "first_answer_text_ms")
			}
		})
	}
}

func TestSenseNovaLatencyCancellationAndNoDispatch(t *testing.T) {
	c, at := senseNovaLatencyFixture(t)
	ResetSenseNovaAttempt(c)
	ctx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	cancel()
	at(8 * time.Millisecond)
	RecordSenseNovaAdmissionWait(c, SenseNovaWaitCapacity, 8*time.Millisecond)
	RecordSenseNovaAdmissionWait(c, SenseNovaWaitCause(90), time.Second)
	RecordSenseNovaAdmissionWait(c, SenseNovaWaitHealth, -time.Second)
	CompleteSenseNovaLatency(c, true)
	entry := SenseNovaLatencyLogInfo(c)
	assert.Equal(t, "canceled", entry.Outcome)
	assert.Empty(t, entry.Attempts)
	assert.EqualValues(t, 8, entry.TotalWaitMS)
	assert.Zero(t, entry.RetryWaitMS)
	other, _ := gin.CreateTestContext(httptest.NewRecorder())
	RecordSenseNovaAdmissionWait(other, SenseNovaWaitCapacity, time.Second)
	assert.Nil(t, SenseNovaLatencyLogInfo(other), "ordinary channels must have no diagnostics")
}

func TestSenseNovaLatencySubMillisecondWaitAggregation(t *testing.T) {
	c, _ := senseNovaLatencyFixture(t)
	MarkSenseNovaLatencyDispatch(c)
	FinishSenseNovaLatencyAttempt(c, nil, true)
	RecordSenseNovaAdmissionWait(c, SenseNovaWaitHealth, 600*time.Microsecond)
	RecordSenseNovaAdmissionWait(c, SenseNovaWaitCapacity, 600*time.Microsecond)
	entry := SenseNovaLatencyLogInfo(c)
	assert.EqualValues(t, 1, entry.TotalWaitMS, "truncate after summing actual sleeps")
	assert.EqualValues(t, 1, entry.RetryWaitMS)
	assert.LessOrEqual(t, entry.RetryWaitMS, entry.TotalWaitMS)
}

func TestSenseNovaLatencyCompletionAndAdminSnapshot(t *testing.T) {
	c, at := senseNovaLatencyFixture(t)
	MarkSenseNovaLatencyDispatch(c)
	at(15 * time.Millisecond)
	ObserveSenseNovaCompletionLatency(c, &dto.OpenAITextResponse{Choices: []dto.OpenAITextResponseChoice{{Message: dto.Message{Content: "secret answer"}}}})
	FinishSenseNovaLatencyAttempt(c, nil, false)
	admin := map[string]interface{}{"existing": true}
	AppendSenseNovaLatencyAdminInfo(c, admin)
	entry, ok := admin["sensenova_latency"].(*SenseNovaLatencyLog)
	require.True(t, ok)
	assert.Equal(t, common.GetPointer(int64(15)), entry.Attempts[0].FirstSemanticMS)
	assert.Equal(t, common.GetPointer(int64(15)), entry.Attempts[0].FirstAnswerMS)
	assert.Equal(t, true, admin["existing"])
	data, err := common.Marshal(admin)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "secret")
}

func TestSenseNovaLatencyConcurrentObserversAndBound(t *testing.T) {
	c, _ := senseNovaLatencyFixture(t)
	senseNovaLatencyState(c).now = time.Now
	MarkSenseNovaLatencyDispatch(c)
	observer := SenseNovaStreamLatencyObserver(c)
	var wg sync.WaitGroup
	for _, work := range []func(){
		func() { observer(`{"choices":[{"delta":{"content":"secret"}}]}`) },
		func() { observer(`{"choices":[{"delta":{"reasoning_content":"secret"}}]}`) },
		func() { SenseNovaLatencyLogInfo(c) },
		func() { FinishSenseNovaLatencyAttempt(c, nil, false) },
	} {
		wg.Add(1)
		go func(work func()) { defer wg.Done(); work() }(work)
	}
	wg.Wait()
	for range SenseNovaMaxAttempts {
		MarkSenseNovaLatencyDispatch(c)
	}
	assert.Len(t, SenseNovaLatencyLogInfo(c).Attempts, SenseNovaMaxAttempts)
}
