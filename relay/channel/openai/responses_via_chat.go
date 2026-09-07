package openai

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func OaiChatToResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var chatResp dto.OpenAITextResponse
	if err := common.Unmarshal(body, &chatResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	senseNova := info.ChannelType == constant.ChannelTypeOpenAI && service.IsSenseNovaRequest(c.Request.Context())
	if oaiError := chatResp.GetOpenAIError(); oaiError != nil && (oaiError.Type != "" || senseNova) {
		status := resp.StatusCode
		if senseNova && status == http.StatusOK {
			status = http.StatusBadGateway
		}
		return nil, types.WithOpenAIError(*oaiError, status)
	}
	if senseNova {
		if len(chatResp.Choices) != 1 {
			return nil, types.NewOpenAIError(errors.New("invalid SenseNova Chat completion"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		}
		choice := chatResp.Choices[0]
		hasOutput := choice.Message.StringContent() != "" || choice.Message.GetReasoningContent() != "" || len(choice.Message.ParseToolCalls()) > 0
		if !validSenseNovaCompletion(choice.FinishReason, hasOutput) {
			return nil, types.NewOpenAIError(errors.New("invalid SenseNova Chat completion"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		}
	}

	service.ObserveSenseNovaCompletionLatency(c, &chatResp)
	if responseID := helper.GetResponseID(c); responseID != "" {
		chatResp.Id = responseID
	}
	convertResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAIResponses, &chatResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	responsesResp, ok := convertResult.Value.(*dto.OpenAIResponsesResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI responses response, got %T", convertResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if senseNova {
		custom := newSenseNovaCustomToolResponse(info)
		for i := range responsesResp.Output {
			if err := custom.restoreOutput(&responsesResp.Output[i], false); err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			}
		}
	}
	usage := convertResult.Usage
	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		responsesResp.Usage = relayconvert.UsageFromChatUsage(usage)
	}

	responseBody, err := common.Marshal(responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiChatToResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	responseID := helper.GetResponseID(c)
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, relayconvert.ResponseStreamOptions{
		ID:    responseID,
		Model: info.UpstreamModelName,
	})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	streamErr := (*types.NewAPIError)(nil)
	senseNova := info.ChannelType == constant.ChannelTypeOpenAI && service.IsSenseNovaRequest(c.Request.Context())
	finishReason := ""
	hasOutput := false
	var custom *senseNovaCustomToolResponse
	if senseNova {
		custom = newSenseNovaCustomToolResponse(info)
	}

	sendEvent := func(event relayconvert.ChatToResponsesStreamEvent) bool {
		events := []relayconvert.ChatToResponsesStreamEvent{event}
		if custom != nil {
			var err error
			events, err = custom.restoreEvent(event)
			if err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
				return false
			}
		}
		for _, output := range events {
			data, err := common.Marshal(output.Payload)
			if err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
				return false
			}
			helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: output.Type}, string(data))
		}
		return true
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}

		var errorResp dto.OpenAITextResponse
		if err := common.UnmarshalJsonStr(data, &errorResp); err == nil {
			if oaiError := errorResp.GetOpenAIError(); oaiError != nil && (oaiError.Type != "" || senseNova) {
				status := resp.StatusCode
				if senseNova && status == http.StatusOK {
					status = http.StatusBadGateway
				}
				streamErr = types.WithOpenAIError(*oaiError, status)
				if senseNova {
					// StreamStatus is logged by the scanner. Keep upstream content out.
					sr.Stop(errors.New("SenseNova upstream stream failed"))
				} else {
					sr.Stop(streamErr)
				}
				return
			}
		}

		var chunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			if senseNova {
				streamErr = types.NewOpenAIError(errors.New("invalid SenseNova stream data"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
				sr.Stop(streamErr)
				return
			}
			logger.LogError(c, "failed to unmarshal chat stream response: "+err.Error())
			sr.Error(err)
			return
		}
		if senseNova {
			if len(chunk.Choices) > 1 || (len(chunk.Choices) == 0 && chunk.Usage == nil) {
				streamErr = types.NewOpenAIError(errors.New("empty SenseNova stream data"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
				sr.Stop(streamErr)
				return
			}
			if len(chunk.Choices) == 1 {
				choice := chunk.Choices[0]
				hasOutput = hasOutput || choice.Delta.GetContentString() != "" || choice.Delta.GetReasoningContent() != "" || len(choice.Delta.ToolCalls) > 0
				if chunk.IsFinished() {
					finishReason = *choice.FinishReason
				}
			}
		}

		results, err := relayconvert.ConvertStreamResponseChunk(c, info, state, &chunk)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		for _, result := range results {
			event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
			if !ok {
				streamErr = types.NewOpenAIError(fmt.Errorf("expected OAI responses stream event, got %T", result.Value), types.ErrorCodeBadResponse, http.StatusInternalServerError)
				sr.Stop(streamErr)
				return
			}
			if !sendEvent(event) {
				sr.Stop(streamErr)
				return
			}
		}
	})

	if senseNova && streamErr == nil && (!validSenseNovaCompletion(finishReason, hasOutput) || c.Request.Context().Err() != nil || !info.StreamStatus.IsNormalEnd() || info.StreamStatus.HasErrors()) {
		streamErr = types.NewOpenAIError(errors.New("SenseNova stream ended before a complete response"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	if streamErr != nil {
		if senseNova && c.Request.Context().Err() == nil {
			failed := dto.ResponsesStreamResponse{Type: "response.failed", Response: &dto.OpenAIResponsesResponse{ID: responseID, Object: "response", Model: info.UpstreamModelName, Status: []byte(`"failed"`), Error: types.OpenAIError{Type: "upstream_error", Code: "upstream_stream_failed", Message: "SenseNova stream failed"}}}
			data, _ := common.Marshal(failed)
			helper.ResponseChunkData(c, failed, string(data))
		}
		return nil, streamErr
	}

	usage := state.Usage()
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, state.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		state.SetUsage(usage)
	}

	finalResults, err := relayconvert.FinalizeStreamResponse(c, info, state)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	for _, result := range finalResults {
		event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
		if !ok {
			return nil, types.NewOpenAIError(fmt.Errorf("expected OAI responses stream event, got %T", result.Value), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		if !sendEvent(event) {
			return nil, streamErr
		}
	}

	return usage, nil
}

func validSenseNovaCompletion(finishReason string, hasOutput bool) bool {
	switch finishReason {
	case "stop", "tool_calls", "function_call":
		return hasOutput
	case "length", "content_filter":
		return true
	default:
		return false
	}
}

// senseNovaCustomToolResponse restores freeform tools wrapped as functions for
// SenseNova. Arguments are buffered until valid JSON is complete so escape
// sequences split across upstream deltas cannot corrupt the freeform input.
type senseNovaCustomToolResponse struct {
	tools     map[string]senseNovaToolIdentity
	arguments map[string]*strings.Builder
}

type senseNovaToolIdentity struct {
	name, namespace string
	custom          bool
}

type senseNovaToolDefinition struct {
	Type  string                    `json:"type"`
	Name  string                    `json:"name"`
	Tools []senseNovaToolDefinition `json:"tools"`
}

func newSenseNovaCustomToolResponse(info *relaycommon.RelayInfo) *senseNovaCustomToolResponse {
	state := &senseNovaCustomToolResponse{tools: make(map[string]senseNovaToolIdentity), arguments: make(map[string]*strings.Builder)}
	request, ok := info.Request.(*dto.OpenAIResponsesRequest)
	if !ok || request == nil {
		return state
	}
	var tools []senseNovaToolDefinition
	if common.Unmarshal(request.Tools, &tools) != nil {
		return state
	}
	for _, tool := range tools {
		if tool.Type == "custom" {
			state.tools[tool.Name] = senseNovaToolIdentity{name: tool.Name, custom: true}
		} else if tool.Type == "namespace" {
			for _, child := range tool.Tools {
				state.tools[senseNovaNamespaceToolName(tool.Name, child.Name)] = senseNovaToolIdentity{name: child.Name, namespace: tool.Name, custom: child.Type == "custom"}
			}
		}
	}
	return state
}

func (s *senseNovaCustomToolResponse) restoreOutput(item *dto.ResponsesOutput, started bool) error {
	if item == nil {
		return nil
	}
	if item.Type == "reasoning" {
		summary := make([]dto.ResponsesReasoningSummaryPart, 0, len(item.Content))
		for _, part := range item.Content {
			summary = append(summary, dto.ResponsesReasoningSummaryPart{Type: "summary_text", Text: part.Text})
		}
		item.Summary = &summary
		item.Content = nil
		return nil
	}
	tool, ok := s.tools[item.Name]
	if item.Type != "function_call" || !ok {
		return nil
	}
	item.Name = tool.name
	if tool.namespace != "" {
		item.Namespace = &tool.namespace
	}
	if !tool.custom {
		return nil
	}
	input := ""
	if !started {
		var payload struct {
			Input *string `json:"input"`
		}
		if common.UnmarshalJsonStr(item.ArgumentsString(), &payload) != nil || payload.Input == nil {
			return errors.New("invalid SenseNova custom tool arguments")
		}
		input = *payload.Input
	}
	item.Type = "custom_tool_call"
	item.Input = &input
	item.Arguments = nil
	return nil
}

func (s *senseNovaCustomToolResponse) restoreEvent(event relayconvert.ChatToResponsesStreamEvent) ([]relayconvert.ChatToResponsesStreamEvent, error) {
	payload := &event.Payload
	if payload.Item != nil {
		started := event.Type == "response.output_item.added"
		if started && payload.Item.Type == "function_call" && s.tools[payload.Item.Name].custom {
			s.arguments[payload.Item.ID] = &strings.Builder{}
		}
		if err := s.restoreOutput(payload.Item, started); err != nil {
			return nil, err
		}
	}
	if arguments := s.arguments[payload.ItemID]; arguments != nil {
		switch event.Type {
		case "response.function_call_arguments.delta":
			arguments.WriteString(payload.Delta)
			return nil, nil
		case "response.function_call_arguments.done":
			var value struct {
				Input *string `json:"input"`
			}
			if common.UnmarshalJsonStr(arguments.String(), &value) != nil || value.Input == nil {
				return nil, errors.New("invalid SenseNova custom tool arguments")
			}
			delta := event
			delta.Type = "response.custom_tool_call_input.delta"
			delta.Payload.Type = delta.Type
			delta.Payload.Delta = *value.Input
			event.Type = "response.custom_tool_call_input.done"
			payload.Type = event.Type
			payload.Input = value.Input
			return []relayconvert.ChatToResponsesStreamEvent{delta, event}, nil
		}
	}
	if payload.Response != nil {
		for i := range payload.Response.Output {
			if err := s.restoreOutput(&payload.Response.Output[i], false); err != nil {
				return nil, err
			}
		}
	}
	return []relayconvert.ChatToResponsesStreamEvent{event}, nil
}
