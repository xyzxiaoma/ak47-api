package controller

import "github.com/QuantumNous/new-api/relaykit/dto"

// senseNovaRequestMetrics retains only already-validated numeric request sizes.
// It never serializes the request or reads messages, tools, headers or secrets.
func senseNovaRequestMetrics(request dto.Request, estimatedPromptTokens int) map[string]interface{} {
	metrics := map[string]interface{}{"estimated_prompt_tokens": estimatedPromptTokens}
	switch request := request.(type) {
	case *dto.GeneralOpenAIRequest:
		if request != nil {
			if request.MaxTokens != nil {
				metrics["requested_max_tokens"] = *request.MaxTokens
			}
			if request.MaxCompletionTokens != nil {
				metrics["requested_max_completion_tokens"] = *request.MaxCompletionTokens
			}
		}
	case *dto.ClaudeRequest:
		if request != nil {
			if request.MaxTokens != nil {
				metrics["requested_max_tokens"] = *request.MaxTokens
			}
			if request.MaxTokensToSample != nil {
				metrics["requested_max_tokens_to_sample"] = *request.MaxTokensToSample
			}
		}
	case *dto.OpenAIResponsesRequest:
		if request != nil && request.MaxOutputTokens != nil {
			metrics["requested_max_output_tokens"] = *request.MaxOutputTokens
		}
	case *dto.GeminiChatRequest:
		if request != nil && request.GenerationConfig.MaxOutputTokens != nil {
			metrics["requested_max_output_tokens"] = *request.GenerationConfig.MaxOutputTokens
		}
	}
	return metrics
}
