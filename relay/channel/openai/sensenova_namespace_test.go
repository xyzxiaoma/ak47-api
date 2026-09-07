package openai

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Codex 0.153.4 represents function identity as name + optional namespace:
// https://github.com/openai/codex/blob/rust-v0.153.4/codex-rs/protocol/src/models.rs#L972
func TestSenseNovaResponsesNamespaceToolsRoundTrip(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "stream"}[stream], func(t *testing.T) {
			c, recorder, info, adaptor := senseNovaResponsesFixture(t, stream)
			var request dto.OpenAIResponsesRequest
			require.NoError(t, common.Unmarshal([]byte(`{
    "model":"deepseek-v4-pro",
    "tools":[
     {"type":"function","name":"spawn_agent","parameters":{"type":"object"}},
     {"type":"namespace","name":"multi_agent_v1","description":"Manage agents","tools":[
      {"type":"function","name":"spawn_agent","description":"Spawn one agent","parameters":{"type":"object","properties":{"message":{"type":"string"}},"required":["message"],"additionalProperties":false}},
      {"type":"function","name":"wait_agent","description":"Wait for an agent","parameters":{"type":"object","properties":{"targets":{"type":"array","items":{"type":"string"}}}}}
     ]},
     {"type":"namespace","name":"other","description":"Another agent service","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object"}}]}
    ],
    "input":[
     {"type":"function_call","call_id":"old_call","namespace":"multi_agent_v1","name":"spawn_agent","arguments":"{\"message\":\"Read a file\"}"},
     {"type":"function_call_output","call_id":"old_call","output":"agent-1"}
    ]
   }`), &request))
			info.Request = &request
			originalTools := string(request.Tools)
			converted, err := adaptor.ConvertOpenAIResponsesRequest(c, info, request)
			require.NoError(t, err)
			chat, ok := converted.(*dto.GeneralOpenAIRequest)
			require.True(t, ok)
			require.Len(t, chat.Tools, 4)
			names := make(map[string]bool)
			for _, tool := range chat.Tools {
				assert.Equal(t, "function", tool.Type)
				assert.Regexp(t, `^[A-Za-z0-9_-]{1,64}$`, tool.Function.Name)
				assert.False(t, names[tool.Function.Name], "namespace collisions must not overwrite tools")
				names[tool.Function.Name] = true
			}
			assert.Equal(t, "spawn_agent", chat.Tools[0].Function.Name)
			assert.Contains(t, chat.Tools[1].Function.Description, "multi_agent_v1")
			assert.Contains(t, chat.Tools[1].Function.Description, "Spawn one agent")
			schema, err := common.Marshal(chat.Tools[1].Function.Parameters)
			require.NoError(t, err)
			assert.JSONEq(t, `{"type":"object","properties":{"message":{"type":"string"}},"required":["message"],"additionalProperties":false}`, string(schema))
			require.Len(t, chat.Messages, 2)
			calls := chat.Messages[0].ParseToolCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, chat.Tools[1].Function.Name, calls[0].Function.Name)
			assert.JSONEq(t, `{"message":"Read a file"}`, calls[0].Function.Arguments)
			assert.Equal(t, "old_call", calls[0].ID)
			assert.Equal(t, "old_call", chat.Messages[1].ToolCallId)
			assert.Equal(t, "agent-1", chat.Messages[1].Content)
			assert.Equal(t, originalTools, string(request.Tools))

			upstreamCalls := []map[string]any{
				{"index": 0, "id": "call_ns", "type": "function", "function": map[string]any{"name": chat.Tools[1].Function.Name, "arguments": `{"message":"Check the output"}`}},
				{"index": 1, "id": "call_other", "type": "function", "function": map[string]any{"name": chat.Tools[3].Function.Name, "arguments": `{}`}},
				{"index": 2, "id": "call_plain", "type": "function", "function": map[string]any{"name": chat.Tools[0].Function.Name, "arguments": `{}`}},
			}
			choice := map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "tool_calls": upstreamCalls}, "finish_reason": "tool_calls"}
			if stream {
				choice["delta"] = choice["message"]
				delete(choice, "message")
			}
			wire, err := common.Marshal(map[string]any{"id": "chat_ns", "model": "deepseek-v4-pro", "choices": []any{choice}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}})
			require.NoError(t, err)
			body := string(wire)
			if stream {
				body = "data: " + body + "\n\ndata: [DONE]\n\n"
			}
			_, apiErr := adaptor.DoResponse(c, &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, info)
			require.Nil(t, apiErr)
			var response map[string]any
			if stream {
				for _, line := range strings.Split(recorder.Body.String(), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var event map[string]any
					require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &event))
					if event["type"] == "response.completed" {
						response = event["response"].(map[string]any)
					}
					if event["type"] == "response.output_item.added" || event["type"] == "response.output_item.done" {
						item := event["item"].(map[string]any)
						if item["call_id"] == "call_ns" {
							assert.Equal(t, "multi_agent_v1", item["namespace"])
							assert.Equal(t, "spawn_agent", item["name"])
						}
					}
				}
			} else {
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			}
			require.NotNil(t, response)
			output, ok := response["output"].([]any)
			require.True(t, ok)
			require.Len(t, output, 3)
			first := output[0].(map[string]any)
			assert.Equal(t, "function_call", first["type"])
			assert.Equal(t, "multi_agent_v1", first["namespace"])
			assert.Equal(t, "spawn_agent", first["name"])
			assert.Equal(t, "call_ns", first["call_id"])
			assert.JSONEq(t, `{"message":"Check the output"}`, first["arguments"].(string))
			assert.Equal(t, "other", output[1].(map[string]any)["namespace"])
			assert.NotContains(t, output[2].(map[string]any), "namespace")

			// Replay the gateway's actual output, not a separately constructed history.
			replay := request
			replay.Input, err = common.Marshal(append(output, map[string]any{"type": "function_call_output", "call_id": "call_ns", "output": "agent-2"}))
			require.NoError(t, err)
			replayValue, err := adaptor.ConvertOpenAIResponsesRequest(c, info, replay)
			require.NoError(t, err)
			replayChat := replayValue.(*dto.GeneralOpenAIRequest)
			replayCalls := replayChat.Messages[0].ParseToolCalls()
			require.Len(t, replayCalls, 3)
			assert.Equal(t, chat.Tools[1].Function.Name, replayCalls[0].Function.Name)
			assert.Equal(t, chat.Tools[3].Function.Name, replayCalls[1].Function.Name)
			assert.Equal(t, "spawn_agent", replayCalls[2].Function.Name)
		})
	}
}

func TestSenseNovaResponsesNamespaceRejectsUnsupportedChildren(t *testing.T) {
	for _, children := range []string{`[{"type":"web_search"}]`, `[{"type":"namespace","name":"nested","tools":[]}]`} {
		c, _, info, adaptor := senseNovaResponsesFixture(t, false)
		request := dto.OpenAIResponsesRequest{Model: "deepseek-v4-pro", Tools: []byte(`[{"type":"namespace","name":"multi_agent_v1","tools":` + children + `}]`)}
		_, err := adaptor.ConvertOpenAIResponsesRequest(c, info, request)
		require.Error(t, err, "unsupported namespace children must not disappear")
	}
}

// Reasoning.summary is required by Codex 0.153.4; summary_text is not a
// valid reasoning.content variant in that tagged protocol definition.
func TestSenseNovaResponsesReasoningSummaryShape(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "stream"}[stream], func(t *testing.T) {
			c, recorder, info, adaptor := senseNovaResponsesFixture(t, stream)
			request := dto.OpenAIResponsesRequest{Model: "deepseek-v4-pro"}
			info.Request = &request
			_, err := adaptor.ConvertOpenAIResponsesRequest(c, info, request)
			require.NoError(t, err)
			body := `{"id":"chat_reasoning","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"Check the file.","content":"Done."},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
			if stream {
				body = "data: " + strings.Replace(body, `"message":`, `"delta":`, 1) + "\n\ndata: [DONE]\n\n"
			}
			_, apiErr := adaptor.DoResponse(c, &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, info)
			require.Nil(t, apiErr)
			var outputs []map[string]any
			if stream {
				for _, line := range strings.Split(recorder.Body.String(), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var event map[string]any
					require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &event))
					if item, ok := event["item"].(map[string]any); ok && item["type"] == "reasoning" {
						outputs = append(outputs, item)
					}
					if event["type"] == "response.completed" {
						for _, item := range event["response"].(map[string]any)["output"].([]any) {
							output := item.(map[string]any)
							if output["type"] == "reasoning" {
								outputs = append(outputs, output)
							}
						}
					}
				}
			} else {
				var response map[string]any
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
				for _, item := range response["output"].([]any) {
					output := item.(map[string]any)
					if output["type"] == "reasoning" {
						outputs = append(outputs, output)
					}
				}
			}
			require.NotEmpty(t, outputs)
			for _, output := range outputs {
				summary, ok := output["summary"].([]any)
				require.True(t, ok, "reasoning item must contain a summary array even when initially empty")
				assert.Nil(t, output["content"], "summary_text does not belong in reasoning.content")
				if output["status"] == "completed" {
					require.Len(t, summary, 1)
					assert.Equal(t, map[string]any{"type": "summary_text", "text": "Check the file."}, summary[0])
				}
			}
		})
	}
}

func TestSenseNovaResponsesForcedNamespaceToolChoice(t *testing.T) {
	for _, toolType := range []string{"function", "custom"} {
		t.Run(toolType, func(t *testing.T) {
			c, _, info, adaptor := senseNovaResponsesFixture(t, false)
			request := dto.OpenAIResponsesRequest{Model: "deepseek-v4-pro"}
			var err error
			request.Tools, err = common.Marshal([]any{
				map[string]any{"type": "function", "name": "run", "parameters": map[string]any{"type": "object"}},
				map[string]any{"type": "namespace", "name": "worker", "tools": []any{map[string]any{"type": toolType, "name": "run", "parameters": map[string]any{"type": "object"}}}},
			})
			require.NoError(t, err)
			request.ToolChoice, err = common.Marshal(map[string]string{"type": toolType, "name": "run", "namespace": "worker"})
			require.NoError(t, err)
			originalChoice := string(request.ToolChoice)
			converted, err := adaptor.ConvertOpenAIResponsesRequest(c, info, request)
			require.NoError(t, err)
			chat := converted.(*dto.GeneralOpenAIRequest)
			require.Len(t, chat.Tools, 2)
			assert.NotEqual(t, "run", chat.Tools[1].Function.Name)
			assert.Equal(t, map[string]any{"type": "function", "function": map[string]any{"name": chat.Tools[1].Function.Name}}, chat.ToolChoice)
			assert.Equal(t, originalChoice, string(request.ToolChoice))

			// An unadvertised namespace must not fall back to the same-named root tool.
			request.ToolChoice, err = common.Marshal(map[string]string{"type": toolType, "name": "run", "namespace": "missing"})
			require.NoError(t, err)
			_, err = adaptor.ConvertOpenAIResponsesRequest(c, info, request)
			require.Error(t, err)
		})
	}
}
