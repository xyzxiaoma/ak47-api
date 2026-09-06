package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func senseNovaServiceFixture(t *testing.T) (*model.Channel, *gin.Context) {
	t.Helper()
	original := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.SenseNovaKeyState{}))
	model.DB = db
	t.Cleanup(func() { model.DB = original; sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	base := "https://token.sensenova.cn"
	channel := &model.Channel{Type: 1, BaseURL: &base, Key: "fake-account-a\nfake-account-b", Models: "glm-5.2,kimi-k3", Status: 1, SenseNovaPool: true, ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2}}
	require.NoError(t, db.Create(channel).Error)
	senseNovaOffsets.Delete(channel.Id)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return channel, c
}

func TestSenseNovaRelayFailoverAndAllCooling(t *testing.T) {
	channel, c := senseNovaServiceFixture(t)
	first, _, err := SelectSenseNovaKey(c, channel, "glm-5.2")
	require.Nil(t, err)
	quota := types.WithOpenAIError(types.OpenAIError{Message: "credits exhausted " + first, Code: "insufficient_quota"}, 429)
	safe := RecordSenseNovaRelayFailure(c, quota)
	assert.NotContains(t, safe.Error(), first)
	ResetSenseNovaAttempt(c)
	second, _, err := SelectSenseNovaKey(c, channel, "kimi-k3")
	require.Nil(t, err)
	assert.NotEqual(t, first, second)
	RecordSenseNovaRelaySuccess(c)
	RecordSenseNovaRelayFailure(c, quota)
	_, _, err = SelectSenseNovaKey(c, channel, "glm-5.2")
	require.NotNil(t, err)
	assert.Equal(t, 503, err.StatusCode)
	due, scanErr := model.ListDueSenseNovaProbes(time.Now().Unix()+61, 10)
	require.NoError(t, scanErr)
	assert.Len(t, due, 2)
}

func TestSenseNovaRequestExclusionSurvivesPersistenceFailure(t *testing.T) {
	channel, c := senseNovaServiceFixture(t)
	first, _, err := SelectSenseNovaKey(c, channel, "glm-5.2")
	require.Nil(t, err)
	// Drop only health storage: recording fails, but the channel still exists.
	require.NoError(t, model.DB.Migrator().DropTable(&model.SenseNovaKeyState{}))
	RecordSenseNovaRelayFailure(c, types.WithOpenAIError(types.OpenAIError{Message: "busy"}, 429))
	require.NoError(t, model.DB.AutoMigrate(&model.SenseNovaKeyState{}))
	senseNovaOffsets.Delete(channel.Id)
	second, _, err := SelectSenseNovaKey(c, channel, "glm-5.2")
	require.Nil(t, err)
	assert.NotEqual(t, first, second)
}

type senseNovaRoundTrip func(*http.Request) (*http.Response, error)

func (f senseNovaRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSenseNovaProbeRequiresValidSuccessAndExactCredential(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		success    bool
		reason     string
	}{
		{"success", `{"choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}]}`, 200, true, ""},
		{"embedded error", `{"error":{"message":"invalid key","code":"invalid_api_key"}}`, 200, false, "authentication_failed"},
		{"malformed", `<html>login</html>`, 200, false, "probe_failed"},
		{"non-json auth", `unauthorized`, 401, false, "authentication_failed"},
		{"empty choices", `{"choices":[]}`, 200, false, "probe_failed"},
		{"empty message", `{"choices":[{"message":{}}]}`, 200, false, "probe_failed"},
		{"invalid content", `{"choices":[{"message":{"role":"assistant","content":{}},"finish_reason":"stop"}]}`, 200, false, "probe_failed"},
		{"invalid finish", `{"choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"garbage"}]}`, 200, false, "probe_failed"},
		{"quota", `{"error":{"message":"credits exhausted","code":"insufficient_quota"}}`, 429, false, "quota_exhausted"},
		{"rate", `{"error":{"message":"busy"}}`, 429, false, "rate_limited"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base := "https://token.sensenova.cn"
			claim := &model.SenseNovaProbeClaim{Key: "only-this-fixture-key", Scope: "kimi-k3", Channel: &model.Channel{Type: 1, BaseURL: &base, Models: "glm-5.2,kimi-k3", SenseNovaPool: true}}
			client := &http.Client{Transport: senseNovaRoundTrip(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "Bearer "+claim.Key, r.Header.Get("Authorization"))
				assert.Equal(t, base+"/v1/chat/completions", r.URL.String())
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				var payload map[string]interface{}
				require.NoError(t, common.Unmarshal(body, &payload))
				assert.Equal(t, "kimi-k3", payload["model"])
				assert.Equal(t, float64(8), payload["max_tokens"])
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body)), Header: http.Header{"Retry-After": []string{"120"}}}, nil
			})}
			success, failure, _ := executeSenseNovaProbe(context.Background(), client, claim)
			assert.Equal(t, tt.success, success)
			assert.Equal(t, tt.reason, failure.Reason)
		})
	}
}

func TestSenseNovaValidationAndRetryAfter(t *testing.T) {
	channel, _ := senseNovaServiceFixture(t)
	require.NoError(t, ValidateSenseNovaPool(channel))
	for _, base := range []string{"https://evil.example", "https://token.sensenova.cn@evil.example", "https://token.sensenova.cn/v1", "http://token.sensenova.cn", "https://token.sensenova.cn?key=x"} {
		channel.BaseURL = &base
		assert.Error(t, ValidateSenseNovaPool(channel), fmt.Sprint(base))
	}
	now := time.Now().UTC().Truncate(time.Second)
	assert.Equal(t, int64(120), senseNovaRetryAfter(now.Add(120*time.Second).Format(http.TimeFormat), now))
	assert.Equal(t, int64(86400), senseNovaRetryAfter("999999", now))
	assert.Zero(t, senseNovaRetryAfter("-1", now))
}

func TestSenseNovaRawFailureBodyIsNotLogged(t *testing.T) {
	channel, c := senseNovaServiceFixture(t)
	_, _, selectedErr := SelectSenseNovaKey(c, channel, "glm-5.2")
	require.Nil(t, selectedErr)
	var output bytes.Buffer
	common.LogWriterMu.Lock()
	previous := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &output
	common.LogWriterMu.Unlock()
	t.Cleanup(func() { common.LogWriterMu.Lock(); gin.DefaultErrorWriter = previous; common.LogWriterMu.Unlock() })
	for _, body := range []string{"unauthorized fake-secret-must-not-be-logged", `{"credential":"fake-secret-must-not-be-logged"}`} {
		response := &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(body))}
		upstream := RelayErrorHandler(c.Request.Context(), response, true)
		require.NotNil(t, upstream)
		assert.NotContains(t, output.String(), "fake-secret-must-not-be-logged")
		assert.NotContains(t, upstream.Error(), "fake-secret-must-not-be-logged")
	}
}

func TestSenseNovaClientCancellationDoesNotCoolKey(t *testing.T) {
	channel, c := senseNovaServiceFixture(t)
	key, _, selectedErr := SelectSenseNovaKey(c, channel, "glm-5.2")
	require.Nil(t, selectedErr)
	ctx, cancel := context.WithCancel(c.Request.Context())
	cancel()
	c.Request = c.Request.WithContext(ctx)
	err := RecordSenseNovaRelayFailure(c, types.NewError(errors.New("context canceled"), types.ErrorCodeDoRequestFailed))
	assert.False(t, ShouldRetrySenseNova(c, err))
	_, snapshotErr := model.SenseNovaKeySnapshot(channel.Id, key, "glm-5.2")
	assert.NoError(t, snapshotErr)
}
