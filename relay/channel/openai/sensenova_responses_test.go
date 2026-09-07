package openai

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func senseNovaResponsesFixture(t *testing.T, stream bool) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo, *Adaptor) {
	t.Helper()
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.SenseNovaKeyState{}))
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB; sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	base := "https://token.sensenova.cn"
	ch := &model.Channel{Type: constant.ChannelTypeOpenAI, BaseURL: &base, Key: "fake-protocol-key", Models: "deepseek-v4-pro", Status: 1, SenseNovaPool: true}
	require.NoError(t, db.Create(ch).Error)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "sensenova-protocol-test")
	_, _, selectionErr := service.SelectSenseNovaKey(c, ch, "deepseek-v4-pro")
	require.Nil(t, selectionErr)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: ch.Type, ChannelBaseUrl: base, UpstreamModelName: "deepseek-v4-pro", SupportStreamOptions: true},
		RelayMode:   relayconstant.RelayModeResponses, RelayFormat: types.RelayFormatOpenAIResponses,
		RequestURLPath: "/v1/responses", IsStream: stream, DisablePing: true,
	}
	adaptor := &Adaptor{}
	adaptor.Init(info)
	return c, recorder, info, adaptor
}

func TestSenseNovaResponsesRequestConversion(t *testing.T) {
	c, _, info, adaptor := senseNovaResponsesFixture(t, true)
	var req dto.OpenAIResponsesRequest
	require.NoError(t, common.Unmarshal([]byte(`{
 "model":"deepseek-v4-pro","stream":true,"store":false,"instructions":"Use tools.",
 "input":[{"role":"developer","content":[{"type":"input_text","text":"Use the workspace."}]},{"role":"user","content":"Read result.txt"},{"type":"function_call","call_id":"call_shell","name":"exec_command","arguments":"{\"cmd\":\"cat result.txt\"}"},{"type":"function_call_output","call_id":"call_shell","output":"OK"},{"type":"custom_tool_call","call_id":"call_patch","name":"apply_patch","input":"patch body"},{"type":"custom_tool_call_output","call_id":"call_patch","output":"patch applied"}],
 "tools":[{"type":"function","name":"exec_command","parameters":{"type":"object","properties":{"cmd":{"type":"string"}}}},{"type":"custom","name":"apply_patch","format":{"type":"text"}}],
 "parallel_tool_calls":false,"max_output_tokens":128,"reasoning":{"effort":"high"}
 }`), &req))
	converted, err := adaptor.ConvertOpenAIResponsesRequest(c, info, req)
	require.NoError(t, err)
	chat, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok, "SenseNova must receive Chat Completions, got %T", converted)
	require.Len(t, chat.Messages, 7)
	assert.Equal(t, "system", chat.Messages[0].Role)
	assert.Equal(t, "Use tools.", chat.Messages[0].Content)
	assert.Equal(t, "system", chat.Messages[1].Role)
	assert.Equal(t, "Use the workspace.", chat.Messages[1].Content)
	assert.Equal(t, "call_shell", chat.Messages[4].ToolCallId)
	assert.Equal(t, "OK", chat.Messages[4].Content)
	assert.Equal(t, "tool", chat.Messages[6].Role)
	assert.Equal(t, "call_patch", chat.Messages[6].ToolCallId)
	assert.Equal(t, "patch applied", chat.Messages[6].Content)
	require.Len(t, chat.Tools, 2)
	assert.Equal(t, "exec_command", chat.Tools[0].Function.Name)
	assert.Equal(t, "apply_patch", chat.Tools[1].Function.Name)
	require.NotNil(t, chat.MaxCompletionTokens)
	assert.Equal(t, uint(128), *chat.MaxCompletionTokens)
	require.NotNil(t, chat.ParallelTooCalls)
	assert.False(t, *chat.ParallelTooCalls)
	require.NotNil(t, chat.StreamOptions)
	assert.True(t, chat.StreamOptions.IncludeUsage)
	assert.Equal(t, "high", chat.ReasoningEffort)
	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://token.sensenova.cn/v1/chat/completions", url)
	assert.Equal(t, relayconstant.RelayModeResponses, info.RelayMode)
	assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.RelayFormat)
}

func TestSenseNovaResponsesRejectUnsupportedBeforeDispatch(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		compact     bool
	}{
		{"compact", ``, true}, {"previous", `,"previous_response_id":"resp_old"`, false},
		{"conversation", `,"conversation":"conv_old"`, false}, {"prompt", `,"prompt":{"id":"pmpt_old"}`, false},
		{"context management", `,"context_management":[{"type":"compaction"}]`, false},
		{"compaction item", `,"input":[{"type":"compaction","encrypted_content":"opaque"}]`, false},
		{"item reference", `,"input":[{"type":"item_reference","id":"msg_old"}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, info, adaptor := senseNovaResponsesFixture(t, false)
			if tc.compact {
				info.RelayMode = relayconstant.RelayModeResponsesCompact
			}
			var req dto.OpenAIResponsesRequest
			require.NoError(t, common.Unmarshal([]byte(`{"model":"deepseek-v4-pro"`+tc.field+`}`), &req))
			_, err := adaptor.ConvertOpenAIResponsesRequest(c, info, req)
			require.Error(t, err)
			var apiErr *types.NewAPIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.True(t, types.IsSkipRetryError(apiErr))
			assert.Empty(t, service.ClassifySenseNovaFailure(apiErr).State)
		})
	}
}

func TestSenseNovaResponsesNativeOpenAIUnchanged(t *testing.T) {
	c, _, info, adaptor := senseNovaResponsesFixture(t, false)
	service.ResetSenseNovaAttempt(c)
	req := dto.OpenAIResponsesRequest{Model: "gpt-test", PreviousResponseID: "resp_native"}
	converted, err := adaptor.ConvertOpenAIResponsesRequest(c, info, req)
	require.NoError(t, err)
	assert.IsType(t, req, converted)
	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://token.sensenova.cn/v1/responses", url)
}

func TestSenseNovaResponsesResponseConversion(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "stream"}[stream], func(t *testing.T) {
			c, recorder, info, adaptor := senseNovaResponsesFixture(t, stream)
			_, err := adaptor.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{Model: "deepseek-v4-pro"})
			require.NoError(t, err)
			body := `{"id":"chat_test","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call_shell","type":"function","function":{"name":"exec_command","arguments":"{\"cmd\":\"cat result.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":80}}}`
			contentType := "application/json"
			if stream {
				contentType = "text/event-stream"
				body = "data: " + `{"id":"chat_test","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_shell","type":"function","function":{"name":"exec_command","arguments":"{\"cmd\":"}}]}}]}` + "\n\ndata: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"cat result.txt\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: " + `{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":80}}}` + "\n\ndata: [DONE]\n\n"
			}
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body))}
			usage, apiErr := adaptor.DoResponse(c, resp, info)
			require.Nil(t, apiErr)
			require.IsType(t, &dto.Usage{}, usage)
			assert.Equal(t, 120, usage.(*dto.Usage).TotalTokens)
			output := recorder.Body.String()
			assert.Contains(t, output, `"type":"function_call"`)
			assert.Contains(t, output, `"call_id":"call_shell"`)
			assert.Contains(t, output, `"name":"exec_command"`)
			assert.Contains(t, output, `"arguments":"{\"cmd\":\"cat result.txt\"}"`)
			assert.Contains(t, output, `"input_tokens":100`)
			assert.Contains(t, output, `"cached_tokens":80`)
			if stream {
				assert.Equal(t, 1, strings.Count(output, "event: response.completed\n"))
			} else {
				assert.Contains(t, output, `"object":"response"`)
			}
		})
	}
}

func TestSenseNovaResponsesTerminalFailure(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		stream     bool
	}{
		{"embedded code", `{"error":{"code":429001,"message":"synthetic upstream failure"}}`, false},
		{"empty choices", `{"choices":[]}`, false},
		{"empty completion", `{"choices":[{"message":{},"finish_reason":"stop"}],"usage":{"total_tokens":1}}`, false},
		{"invalid finish", `{"choices":[{"message":{"content":"partial"},"finish_reason":"unknown"}],"usage":{"total_tokens":1}}`, false},
		{"truncated", "data: " + `{"choices":[{"index":0,"delta":{"content":"partial"}}]}` + "\n\n", true},
		{"malformed", "data: invalid\n\ndata: [DONE]\n\n", true},
		{"stream error", "data: " + `{"error":{"code":429001,"message":"synthetic upstream failure"}}` + "\n\n", true},
		{"empty stream finish", "data: " + `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"total_tokens":1}}` + "\n\ndata: [DONE]\n\n", true},
		{"invalid stream finish", "data: " + `{"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"unknown"}],"usage":{"total_tokens":1}}` + "\n\ndata: [DONE]\n\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, recorder, info, adaptor := senseNovaResponsesFixture(t, tc.stream)
			_, err := adaptor.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{Model: "deepseek-v4-pro"})
			require.NoError(t, err)
			resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(tc.body))}
			_, apiErr := adaptor.DoResponse(c, resp, info)
			require.NotNil(t, apiErr)
			assert.NotContains(t, recorder.Body.String(), "response.completed")
			assert.NotContains(t, recorder.Body.String(), "synthetic upstream failure")
			if tc.stream {
				assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.failed\n"))
			}
		})
	}
}

func TestSenseNovaResponsesCustomToolRoundTrip(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "stream"}[stream], func(t *testing.T) {
			c, recorder, info, adaptor := senseNovaResponsesFixture(t, stream)
			var req dto.OpenAIResponsesRequest
			require.NoError(t, common.Unmarshal([]byte(`{"model":"deepseek-v4-pro","tools":[{"type":"custom","name":"apply_patch","description":"Apply a patch","format":{"type":"text"}}],"tool_choice":{"type":"custom","name":"apply_patch"},"input":[{"type":"custom_tool_call","call_id":"old_patch","name":"apply_patch","input":"old patch"},{"type":"custom_tool_call_output","call_id":"old_patch","output":"ok"}]}`), &req))
			info.Request = &req
			converted, err := adaptor.ConvertOpenAIResponsesRequest(c, info, req)
			require.NoError(t, err)
			chat, ok := converted.(*dto.GeneralOpenAIRequest)
			require.True(t, ok)
			require.Len(t, chat.Tools, 1)
			assert.Equal(t, "function", chat.Tools[0].Type)
			assert.Equal(t, "apply_patch", chat.Tools[0].Function.Name)
			assert.Empty(t, chat.Tools[0].Custom)
			choice, err := common.Marshal(chat.ToolChoice)
			require.NoError(t, err)
			assert.JSONEq(t, `{"type":"function","function":{"name":"apply_patch"}}`, string(choice))
			calls := chat.Messages[0].ParseToolCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, "function", calls[0].Type)
			assert.JSONEq(t, `{"input":"old patch"}`, calls[0].Function.Arguments)
			assert.Equal(t, "old_patch", chat.Messages[1].ToolCallId)
			assert.Equal(t, "ok", chat.Messages[1].Content)
			assert.Contains(t, string(req.Tools), `"type":"custom"`, "original request must remain usable for replay")
			body := `{"id":"chat_patch","model":"deepseek-v4-pro","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_patch","type":"function","function":{"name":"apply_patch","arguments":"{\"input\":\"patch\\nbody\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`
			if stream {
				body = "data: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_patch","type":"function","function":{"name":"apply_patch","arguments":"{\"input\":\"patch\\"}}]}}]}` + "\n\ndata: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"nbody\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: " + `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}` + "\n\ndata: [DONE]\n\n"
			}
			resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
			_, apiErr := adaptor.DoResponse(c, resp, info)
			require.Nil(t, apiErr)
			output := recorder.Body.String()
			assert.Contains(t, output, `"type":"custom_tool_call"`)
			assert.Contains(t, output, `"call_id":"call_patch"`)
			assert.Contains(t, output, `"name":"apply_patch"`)
			assert.Contains(t, output, `"input":"patch\nbody"`)
			assert.NotContains(t, output, `"type":"function_call"`)
			if stream {
				assert.Contains(t, output, "event: response.custom_tool_call_input.delta\n")
				assert.Contains(t, output, `"delta":"patch\nbody"`)
				assert.Contains(t, output, "event: response.custom_tool_call_input.done\n")
				assert.NotContains(t, output, "response.function_call_arguments")
			}
		})
	}
}
