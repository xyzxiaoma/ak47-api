package openai

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSenseNovaChatStreamCompletionIntegrity(t *testing.T) {
	event := func(s string) string { return "data: " + s + "\n\n" }
	role := event(`{"id":"chat-1","choices":[{"index":0,"delta":{"role":"assistant"}}]}`)
	text := event(`{"id":"chat-1","choices":[{"index":0,"delta":{"content":"hello"}}]}`)
	tool := event(`{"id":"chat-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Write","arguments":"{}"}}]}}]}`)
	finish := event(`{"id":"chat-1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
	toolFinish := event(`{"id":"chat-1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
	reasoning := event(`{"id":"chat-1","choices":[{"index":0,"delta":{"reasoning_content":"thinking"}}]}`)
	usageEvent := event(`{"id":"chat-1","choices":[],"usage":{"prompt_tokens":28000,"completion_tokens":12,"total_tokens":28012}}`)
	emptyTool := event(`{"choices":[{"index":0,"delta":{"tool_calls":[{}]}}]}`)
	brokenTool := event(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_bad","function":{"name":"Write","arguments":"{"}}]}}]}`)
	parallelTools := event(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Write","arguments":"{}"}},{"index":1,"id":"call_2","type":"function","function":{"name":"Read","arguments":"{}"}}]}}]}`)
	mixedText := event(`{"choices":[{"index":0,"delta":{"reasoning_content":"thinking","content":"hello"}}]}`)
	mixedTool := event(`{"choices":[{"index":0,"delta":{"content":"hello","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Write","arguments":"{}"}}]}}]}`)
	failure := event(`{"error":{"code":429001,"type":"rate_limit_error","message":"private provider data"}}`)
	done := event("[DONE]")
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
		for _, tc := range []struct {
			name, body     string
			valid, written bool
		}{
			{"empty", "", false, false}, {"done-only", done, false, false},
			{"role-only", role + done, false, false}, {"malformed", event("{"), false, false},
			{"error-before-output", role + failure + done, false, false},
			{"truncated-text", role + text, false, true},
			{"truncated-after-finish", role + text + finish, false, true},
			{"malformed-done", role + text + finish + event("[DONE]garbage"), false, true},
			{"usage-only", usageEvent + done, false, false},
			{"empty-tool", emptyTool + toolFinish + done, false, false},
			{"text-tool-finish", text + toolFinish + done, false, true},
			{"invalid-tool-json", brokenTool + toolFinish + done, false, true},
			{"error-after-tool", role + tool + failure + done, false, true},
			{"error-after-finish", role + text + finish + failure + done, false, true},
			{"valid-text", role + text + finish + done, true, true},
			{"valid-tool", role + tool + toolFinish + done, true, true},
			{"valid-thinking", role + reasoning + text + finish + usageEvent + done, true, true},
			{"valid-parallel-no-role", parallelTools + toolFinish + done, true, true},
			{"valid-mixed-thinking-text", mixedText + finish + done, true, true},
			{"valid-mixed-text-tool", mixedTool + toolFinish + done, true, true},
		} {
			t.Run(string(format)+"/"+tc.name, func(t *testing.T) {
				c, recorder, info, adaptor := senseNovaResponsesFixture(t, true)
				info.RelayMode = relayconstant.RelayModeChatCompletions
				info.RelayFormat = format
				info.SetEstimatePromptTokens(28000)
				info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}
				resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}
				usage, apiErr := adaptor.DoResponse(c, resp, info)
				if tc.valid {
					require.Nil(t, apiErr)
					require.NotNil(t, usage)
				} else {
					require.NotNil(t, apiErr)
					if !tc.written {
						assert.Nil(t, usage)
					}
				}
				assert.Equal(t, tc.written, c.Writer.Written())
				assert.NotContains(t, recorder.Body.String(), "private provider data")
				if tc.name == "valid-parallel-no-role" {
					assert.Contains(t, recorder.Body.String(), "call_2")
				}
				if strings.HasPrefix(tc.name, "valid-mixed") {
					assert.Contains(t, recorder.Body.String(), "hello")
				}
				if tc.name == "valid-mixed-thinking-text" {
					assert.Contains(t, recorder.Body.String(), "thinking")
				}
				if !tc.valid {
					assert.NotContains(t, recorder.Body.String(), "[DONE]")
					assert.NotContains(t, recorder.Body.String(), "message_stop")
					if tc.written {
						assert.Contains(t, recorder.Body.String(), "error")
					} else {
						assert.Empty(t, recorder.Body.String())
					}
				} else if format == types.RelayFormatClaude {
					assert.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"message_stop"`), recorder.Body.String())
				}
			})
		}
	}
}

func TestSenseNovaChatStreamMixedChoiceOrderingAndUsageIdentity(t *testing.T) {
	c, recorder, info, adaptor := senseNovaResponsesFixture(t, true)
	info.RelayMode = relayconstant.RelayModeChatCompletions
	info.RelayFormat = types.RelayFormatOpenAI
	info.ShouldIncludeUsage = true
	body := ""
	for _, data := range []string{
		`{"id":"chat-multi","created":123,"system_fingerprint":"fp-original","choices":[{"index":0,"delta":{"content":"zero"}}]}`,
		`{"id":"chat-multi","choices":[{"index":0,"delta":{},"finish_reason":"stop"},{"index":1,"delta":{"content":"A"}}]}`,
		`{"id":"chat-multi","choices":[{"index":1,"delta":{"content":"B"}}]}`,
		`{"id":"chat-multi","choices":[{"index":1,"delta":{},"finish_reason":"stop"}]}`,
		`[DONE]`,
	} {
		body += "data: " + data + "\n\n"
	}
	_, apiErr := adaptor.DoResponse(c, &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, info)
	require.Nil(t, apiErr)
	var secondChoice string
	var finalUsage *dto.ChatCompletionsStreamResponse
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") || strings.Contains(line, "[DONE]") {
			continue
		}
		var chunk dto.ChatCompletionsStreamResponse
		require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &chunk))
		for _, choice := range chunk.Choices {
			if choice.Index == 1 {
				secondChoice += choice.Delta.GetContentString()
			}
		}
		if chunk.Usage != nil {
			finalUsage = &chunk
		}
	}
	assert.Equal(t, "AB", secondChoice)
	require.NotNil(t, finalUsage)
	assert.Equal(t, "chat-multi", finalUsage.Id)
	assert.Equal(t, int64(123), finalUsage.Created)
	assert.Equal(t, "fp-original", finalUsage.GetSystemFingerprint())
}

func TestSenseNovaNonStreamCompletionIntegrity(t *testing.T) {
	for _, body := range []string{`{}`, `{"choices":[]}`, `{"choices":[{"message":{"role":"assistant","content":""},"finish_reason":"stop"}]}`, `{"error":{"code":429001}}`} {
		t.Run(body, func(t *testing.T) {
			c, recorder, info, adaptor := senseNovaResponsesFixture(t, false)
			info.RelayMode = relayconstant.RelayModeChatCompletions
			info.RelayFormat = types.RelayFormatOpenAI
			info.SetEstimatePromptTokens(28000)
			_, apiErr := adaptor.DoResponse(c, &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, info)
			require.NotNil(t, apiErr)
			assert.False(t, c.Writer.Written())
			assert.Empty(t, recorder.Body.String())
		})
	}
}
