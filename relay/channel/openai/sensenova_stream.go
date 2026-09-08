package openai

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// SenseNovaChatStreamHandler delays terminal events until the upstream stream
// has completed. Once any text, reasoning or tool delta is delivered, failures
// are terminal to this attempt; partial tools must never be replayed.
func SenseNovaChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(errors.New("invalid SenseNova response"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	defer service.CloseResponseBodyGracefully(resp)
	// A heartbeat would commit HTTP 200 before an empty/error response is known.
	previousDisablePing := info.DisablePing
	info.DisablePing = true
	defer func() { info.DisablePing = previousDisablePing }()
	var streamErr *types.NewAPIError
	var prefix, usageData string
	var terminals []string
	var text strings.Builder
	var toolCount int
	var usage *dto.Usage
	var responseID, systemFingerprint string
	var created int64
	outputs := make(map[int]bool)
	finishes := make(map[int]string)
	tools := make(senseNovaStreamTools)
	seenTools := make(map[string]struct{})
	var toolNames []string
	hasDeliveredOutput := false
	invalid := func() *types.NewAPIError {
		return types.NewOpenAIError(errors.New("SenseNova stream ended without a valid completion"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var errorResponse dto.OpenAITextResponse
		if common.UnmarshalJsonStr(data, &errorResponse) == nil {
			if upstream := errorResponse.GetOpenAIError(); upstream != nil {
				streamErr = types.WithOpenAIError(*upstream, http.StatusBadGateway)
				sr.Stop(errors.New("SenseNova upstream stream failed"))
				return
			}
		}
		var chunk dto.ChatCompletionsStreamResponse
		if common.UnmarshalJsonStr(data, &chunk) != nil || (len(chunk.Choices) == 0 && chunk.Usage == nil) {
			streamErr = invalid()
			sr.Stop(streamErr)
			return
		}
		if service.ValidUsage(chunk.Usage) {
			usage = chunk.Usage
			usageData = data
		}
		if chunk.Id != "" {
			responseID = chunk.Id
		}
		if chunk.Created != 0 {
			created = chunk.Created
		}
		if chunk.GetSystemFingerprint() != "" {
			systemFingerprint = chunk.GetSystemFingerprint()
		}
		semantic, terminal := false, false
		forward := chunk
		forward.Choices = nil
		ending := chunk
		ending.Choices = nil
		for _, choice := range chunk.Choices {
			if _, finished := finishes[choice.Index]; finished {
				streamErr = invalid()
				sr.Stop(streamErr)
				return
			}
			if choice.Index < 0 || (info.RelayFormat == types.RelayFormatClaude && choice.Index != 0) || !tools.observe(choice.Index, choice.Delta.ToolCalls) {
				streamErr = invalid()
				sr.Stop(streamErr)
				return
			}
			hasOutput := choice.Delta.GetContentString() != "" || choice.Delta.GetReasoningContent() != "" || len(choice.Delta.ToolCalls) > 0
			outputs[choice.Index] = outputs[choice.Index] || hasOutput
			semantic = semantic || hasOutput
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finishes[choice.Index] = *choice.FinishReason
				terminal = true
				endChoice := choice
				endChoice.Delta = dto.ChatCompletionsStreamResponseChoiceDelta{}
				ending.Choices = append(ending.Choices, endChoice)
				if !hasOutput {
					continue
				}
				choice.FinishReason = nil
			}
			forward.Choices = append(forward.Choices, choice)
		}
		if terminal {
			// Buffer only terminal markers. Co-located deltas keep their order,
			// including other choices that are still producing output.
			ending.Usage = nil
			encoded, err := common.Marshal(ending)
			if err != nil {
				streamErr = invalid()
				sr.Stop(streamErr)
				return
			}
			terminals = append(terminals, string(encoded))
			encoded, err = common.Marshal(forward)
			if err != nil {
				streamErr = invalid()
				sr.Stop(streamErr)
				return
			}
			data = string(encoded)
		}
		if len(forward.Choices) == 0 {
			return
		}
		if prefix == "" && !hasDeliveredOutput && info.RelayFormat == types.RelayFormatClaude {
			// The converter's first event establishes the message. Keep parallel
			// tools in the subsequent content event so no call can be dropped.
			initial := chunk
			initial.Usage = nil
			initial.Choices = []dto.ChatCompletionsStreamResponseChoice{{Index: 0, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Role: "assistant"}}}
			encoded, err := common.Marshal(initial)
			if err != nil {
				streamErr = invalid()
				sr.Stop(streamErr)
				return
			}
			prefix = string(encoded)
		}
		if !hasDeliveredOutput && !semantic {
			prefix = data
			return
		}
		if prefix != "" {
			if err := HandleStreamFormat(c, info, prefix, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent); err != nil {
				streamErr = invalid()
				sr.Stop(streamErr)
				return
			}
			prefix = ""
		}
		if err := sendSenseNovaStreamContent(c, info, data); err != nil {
			streamErr = invalid()
			sr.Stop(streamErr)
			return
		}
		hasDeliveredOutput = hasDeliveredOutput || semantic
		_ = ProcessStreamResponse(forward, &text, &toolCount)
		collectStreamFunctionCallNames(data, seenTools, &toolNames)
	})
	if streamErr == nil {
		if len(outputs) == 0 || len(finishes) != len(outputs) || info.StreamStatus.EndReason != relaycommon.StreamEndReasonDone || info.StreamStatus.HasErrors() || c.Request.Context().Err() != nil {
			streamErr = invalid()
		} else {
			for index, hasOutput := range outputs {
				if !validSenseNovaCompletion(finishes[index], hasOutput) || !tools.validFinish(index, finishes[index]) {
					streamErr = invalid()
					break
				}
			}
		}
	}
	if streamErr != nil {
		if !c.Writer.Written() {
			// Restore ordinary JSON error headers and permit a fresh streaming attempt.
			for _, name := range []string{"Content-Type", "Cache-Control", "Connection", "Transfer-Encoding", "X-Accel-Buffering"} {
				c.Writer.Header().Del(name)
			}
			delete(c.Keys, "event_stream_headers_set")
			return nil, streamErr
		}
		types.ErrOptionWithSkipRetry()(streamErr)
		if c.Request.Context().Err() == nil {
			if info.RelayFormat == types.RelayFormatClaude {
				helper.ClaudeChunkData(c, dto.ClaudeResponse{Type: "error"}, `{"type":"error","error":{"type":"api_error","message":"SenseNova stream failed"}}`)
			} else {
				_ = helper.StringData(c, `{"error":{"type":"upstream_error","code":"upstream_stream_failed","message":"SenseNova stream failed"}}`)
			}
		}
		if !hasDeliveredOutput {
			return nil, streamErr
		}
	} else {
		if prefix != "" {
			_ = HandleStreamFormat(c, info, prefix, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
		}
		for _, data := range terminals {
			var chunk dto.ChatCompletionsStreamResponse
			_ = common.UnmarshalJsonStr(data, &chunk)
			_ = ProcessStreamResponse(chunk, &text, &toolCount)
			collectStreamFunctionCallNames(data, seenTools, &toolNames)
		}
	}
	measured := usage != nil
	if !measured {
		usage = service.ResponseText2Usage(c, text.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		usage.CompletionTokens += toolCount * 7
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	applyUsagePostProcessing(info, usage, []byte(usageData))
	for _, name := range toolNames {
		info.CountBillableToolCall(dto.BuildInCallFunctionCall, name)
	}
	if streamErr != nil {
		return usage, streamErr
	}
	if info.RelayFormat == types.RelayFormatClaude {
		info.ClaudeConvertInfo.Usage = usage
	}
	for _, data := range terminals {
		if info.RelayFormat == types.RelayFormatClaude {
			var terminal dto.ChatCompletionsStreamResponse
			_ = common.UnmarshalJsonStr(data, &terminal)
			terminal.Usage = usage
			encoded, err := common.Marshal(terminal)
			if err != nil {
				return usage, invalid()
			}
			data = string(encoded)
		}
		if err := HandleStreamFormat(c, info, data, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent); err != nil {
			return usage, invalid()
		}
	}
	if info.RelayFormat == types.RelayFormatOpenAI {
		if info.ShouldIncludeUsage {
			finalUsage := helper.GenerateFinalUsageResponse(responseID, created, info.UpstreamModelName, *usage)
			finalUsage.SetSystemFingerprint(systemFingerprint)
			_ = helper.ObjectData(c, finalUsage)
		}
		helper.Done(c)
	}
	return usage, nil
}

// The Claude converter consumes one semantic kind per delta. Split combined
// provider deltas so reasoning, answer text and tools are all delivered.
func sendSenseNovaStreamContent(c *gin.Context, info *relaycommon.RelayInfo, data string) error {
	if info.RelayFormat != types.RelayFormatClaude {
		return HandleStreamFormat(c, info, data, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
	}
	var chunk dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
		return err
	}
	if len(chunk.Choices) != 1 {
		return HandleStreamFormat(c, info, data, false, false)
	}
	choice := chunk.Choices[0]
	var deltas []dto.ChatCompletionsStreamResponseChoiceDelta
	if thinking := choice.Delta.GetReasoningContent(); thinking != "" {
		deltas = append(deltas, dto.ChatCompletionsStreamResponseChoiceDelta{ReasoningContent: &thinking})
	}
	if content := choice.Delta.GetContentString(); content != "" {
		deltas = append(deltas, dto.ChatCompletionsStreamResponseChoiceDelta{Content: &content})
	}
	if len(choice.Delta.ToolCalls) > 0 {
		deltas = append(deltas, dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: choice.Delta.ToolCalls})
	}
	if len(deltas) < 2 {
		return HandleStreamFormat(c, info, data, false, false)
	}
	for _, delta := range deltas {
		choice.Delta = delta
		chunk.Choices = []dto.ChatCompletionsStreamResponseChoice{choice}
		encoded, err := common.Marshal(chunk)
		if err != nil {
			return err
		}
		if err = HandleStreamFormat(c, info, string(encoded), false, false); err != nil {
			return err
		}
	}
	return nil
}
