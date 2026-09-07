package oairesponses

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexCustomToolOutputReplay(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{Model: "deepseek-v4-pro", Input: []byte(`[
 {"type":"custom_tool_call","call_id":"patch_1","name":"apply_patch","input":"patch body"},
 {"type":"custom_tool_call_output","call_id":"patch_1","output":"patch applied"}
 ]`)}
	converted, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 2)
	calls := converted.Messages[0].ParseToolCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "patch_1", calls[0].ID)
	assert.Equal(t, "apply_patch", calls[0].Function.Name)
	assert.Equal(t, "patch body", calls[0].Function.Arguments)
	assert.Equal(t, "tool", converted.Messages[1].Role)
	assert.Equal(t, "patch_1", converted.Messages[1].ToolCallId)
	assert.Equal(t, "patch applied", converted.Messages[1].Content)
}
