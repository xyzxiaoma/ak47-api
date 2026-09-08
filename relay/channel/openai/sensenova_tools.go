package openai

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// Incremental tool state validates the completed call, while allowing partial
// argument deltas to reach clients without declaring an executable completion.
type senseNovaStreamTools map[int]map[int]*dto.ToolCallResponse

func (tools senseNovaStreamTools) observe(choice int, deltas []dto.ToolCallResponse) bool {
	for position, delta := range deltas {
		index := position
		if delta.Index != nil {
			index = *delta.Index
		}
		if index < 0 || (delta.ID == "" && delta.Function.Name == "" && delta.Function.Arguments == "") {
			return false
		}
		if delta.Type != nil && delta.Type != "" && delta.Type != "function" {
			return false
		}
		if tools[choice] == nil {
			tools[choice] = make(map[int]*dto.ToolCallResponse)
		}
		state := tools[choice][index]
		if state == nil {
			state = &dto.ToolCallResponse{}
			tools[choice][index] = state
		}
		if delta.ID != "" {
			if state.ID != "" && state.ID != delta.ID {
				return false
			}
			state.ID = delta.ID
		}
		state.Function.Name += delta.Function.Name
		state.Function.Arguments += delta.Function.Arguments
	}
	return true
}

func (tools senseNovaStreamTools) validFinish(choice int, finish string) bool {
	calls := tools[choice]
	if finish == "length" || finish == "content_filter" {
		return true
	}
	if finish != "tool_calls" {
		return len(calls) == 0 && finish != "function_call"
	}
	if len(calls) == 0 {
		return false
	}
	ids := make(map[string]bool)
	for _, call := range calls {
		if call.ID == "" || call.Function.Name == "" || ids[call.ID] {
			return false
		}
		ids[call.ID] = true
		var arguments map[string]any
		if common.UnmarshalJsonStr(call.Function.Arguments, &arguments) != nil || arguments == nil {
			return false
		}
	}
	return true
}
