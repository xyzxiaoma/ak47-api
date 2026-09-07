package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
)

var senseNovaProbeSlots = make(chan struct{}, 2)
var senseNovaRecoveryOnce sync.Once

func StartSenseNovaRecovery() {
	senseNovaRecoveryOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for now := range ticker.C {
				if !common.IsMasterNode {
					continue
				}
				states, err := model.ListDueSenseNovaProbes(now.Unix(), 32)
				if err != nil {
					common.SysError("SenseNova recovery scan failed")
					continue
				}
				for _, state := range states {
					channel, err := model.GetChannelById(state.ChannelID, true)
					if err != nil {
						continue
					}
					for _, key := range channel.GetKeys() {
						if model.SenseNovaFingerprint(key) == state.Fingerprint {
							_ = launchSenseNovaProbe(channel, key, state.Scope, false)
							break
						}
					}
				}
			}
		}()
	})
}

func QueueSenseNovaProbe(channel *model.Channel, key string) error {
	if err := ValidateSenseNovaPool(channel); err != nil {
		return err
	}
	scope := senseNovaProbeModel(channel)
	states, err := model.ListSenseNovaStates(channel.Id)
	if err != nil {
		return err
	}
	fingerprint := model.SenseNovaFingerprint(key)
	globalCooling := false
	for _, state := range states {
		if state.Fingerprint != fingerprint {
			continue
		}
		if state.Scope == "" && state.State == model.SenseNovaCooling {
			globalCooling = true
		}
		if state.Scope != "" && state.State == model.SenseNovaCooling {
			scope = state.Scope
		}
	}
	if globalCooling {
		scope = ""
	}
	return launchSenseNovaProbe(channel, key, scope, true)
}

func senseNovaProbeModel(channel *model.Channel) string {
	for _, name := range channel.GetModels() {
		if name == "deepseek-v4-flash" {
			return name
		}
	}
	return channel.GetModels()[0]
}

func launchSenseNovaProbe(channel *model.Channel, key, scope string, force bool) error {
	if ValidateSenseNovaPool(channel) != nil {
		return model.ErrSenseNovaUnavailable
	}
	select {
	case senseNovaProbeSlots <- struct{}{}:
	default:
		return errors.New("SenseNova probes are busy; retry shortly")
	}
	claim, err := model.ClaimSenseNovaProbe(channel.Id, key, scope, time.Now().Unix(), force)
	if err != nil {
		<-senseNovaProbeSlots
		return errors.New("SenseNova key is disabled, invalid or already being checked; retry later")
	}
	go func() {
		defer func() { <-senseNovaProbeSlots }()
		settings := claim.Channel.GetSetting()
		client, err := GetHttpClientWithProxySettings(settings.Proxy, settings)
		if err != nil {
			_, _ = model.FinishSenseNovaProbe(claim, false, "probe_failed", false, 0, time.Now().Unix())
			return
		}
		copyClient := *client
		copyClient.Timeout = 20 * time.Second
		copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		success, failure, retryAfter := executeSenseNovaProbe(ctx, &copyClient, claim)
		if _, err := model.FinishSenseNovaProbe(claim, success, failure.Reason, failure.State == model.SenseNovaInvalid, retryAfter, time.Now().Unix()); err != nil {
			common.SysError("SenseNova probe result persistence failed")
		}
	}()
	return nil
}

// executeSenseNovaProbe is isolated from scheduling and billing for deterministic
// fake-transport tests. No response bodies or credentials are logged or stored.
func executeSenseNovaProbe(ctx context.Context, client *http.Client, claim *model.SenseNovaProbeClaim) (bool, SenseNovaFailure, int64) {
	failed := SenseNovaFailure{State: model.SenseNovaCooling, Reason: "probe_failed"}
	if claim == nil || claim.Channel == nil || ValidateSenseNovaPool(claim.Channel) != nil {
		return false, failed, 0
	}
	name := claim.Scope
	if name == "" {
		name = senseNovaProbeModel(claim.Channel)
	}
	requestPayload := map[string]interface{}{
		"model": name, "messages": []map[string]string{{"role": "user", "content": "Reply OK"}},
		"stream": false, "max_tokens": 8, "thinking": map[string]string{"type": "disabled"},
	}
	// SenseNova GLM rejects disabled thinking unless reasoning effort is also none.
	if name == "glm-5.2" {
		requestPayload["reasoning_effort"] = "none"
	}
	payload, err := common.Marshal(requestPayload)
	if err != nil {
		return false, failed, 0
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(claim.Channel.GetBaseURL(), "/")+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return false, failed, 0
	}
	req.Header.Set("Authorization", "Bearer "+claim.Key)
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return false, failed, 0
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024+1))
	if err != nil || len(body) > 64*1024 {
		return false, failed, 0
	}
	var result struct {
		Error *struct {
			Message string      `json:"message"`
			Code    interface{} `json:"code"`
			Type    string      `json:"type"`
		} `json:"error"`
		Choices []struct {
			Message *struct {
				Role    string      `json:"role"`
				Content interface{} `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if common.Unmarshal(body, &result) != nil {
		classified := ClassifySenseNovaFailure(types.WithOpenAIError(types.OpenAIError{}, response.StatusCode))
		if classified.State != "" {
			failed = classified
		}
		return false, failed, senseNovaRetryAfter(response.Header.Get("Retry-After"), time.Now())
	}
	if response.StatusCode == http.StatusOK && result.Error == nil && len(result.Choices) > 0 && result.Choices[0].Message != nil && result.Choices[0].Message.Role == "assistant" {
		_, textContent := result.Choices[0].Message.Content.(string)
		finish := result.Choices[0].FinishReason
		if textContent && (finish == "stop" || finish == "length") {
			return true, SenseNovaFailure{}, 0
		}
	}
	if response.StatusCode == http.StatusOK && result.Error == nil {
		return false, failed, 0
	}
	status := response.StatusCode
	if status == http.StatusOK {
		status = http.StatusBadGateway
	}
	message, code := "", "bad_response_status_code"
	if result.Error != nil {
		message = result.Error.Message
		if s, ok := result.Error.Code.(string); ok {
			code = s
		}
	}
	classified := ClassifySenseNovaFailure(types.WithOpenAIError(types.OpenAIError{Message: message, Code: code}, status))
	if classified.State != "" {
		failed = classified
	}
	return false, failed, senseNovaRetryAfter(response.Header.Get("Retry-After"), time.Now())
}

func senseNovaRetryAfter(value string, now time.Time) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	// Delta-seconds is unsigned decimal, not a signed integer. Accumulate
	// with saturation so even excessively large digit strings remain bounded.
	seconds := int64(0)
	digits := true
	for _, char := range value {
		if char < '0' || char > '9' {
			digits = false
			break
		}
		seconds = seconds*10 + int64(char-'0')
		if seconds > 86400 {
			seconds = 86400
		}
	}
	if digits {
		return seconds
	}
	date, err := http.ParseTime(value)
	if err != nil || !date.After(now) {
		return 0
	}
	delay := date.Sub(now)
	if delay >= 24*time.Hour {
		return 86400
	}
	// Round up: a future HTTP date must not permit retrying before that date.
	return int64((delay + time.Second - 1) / time.Second)
}
