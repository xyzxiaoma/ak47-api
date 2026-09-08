package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSenseNovaStreamFailureRetryAndHealth(t *testing.T) {
	for _, deliveredTool := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty retries another key", true: "tool output prevents replay"}[deliveredTool], func(t *testing.T) {
			ch := setupSenseNovaController(t)
			base := "https://token.sensenova.cn"
			ch.BaseURL = &base
			require.NoError(t, model.DB.Save(ch).Error)
			oldTimeout := constant.StreamingTimeout
			constant.StreamingTimeout = 30
			t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
			require.Nil(t, middleware.SetupContextForSelectedChannel(c, ch, "glm-5.2"))
			first := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: ch.Type, UpstreamModelName: "glm-5.2"}, OriginModelName: "glm-5.2", RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: types.RelayFormatClaude, IsStream: true, DisablePing: true, ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}}
			info.SetEstimatePromptTokens(28000)
			body := ""
			if deliveredTool {
				body = "data: " + `{"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_write","type":"function","function":{"name":"Write","arguments":"{}"}}]}}]}` + "\n\n"
			}
			usage, upstreamErr := openai.OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))})
			require.NotNil(t, upstreamErr)
			if deliveredTool {
				require.NotNil(t, usage)
			} else {
				assert.Nil(t, usage)
			}
			safe := service.RecordSenseNovaRelayFailure(c, upstreamErr)
			assert.Equal(t, !deliveredTool, shouldRetry(c, safe, 3))
			assert.Equal(t, deliveredTool, types.IsSkipRetryError(safe))
			states, err := model.ListSenseNovaStates(ch.Id)
			require.NoError(t, err)
			var failureFound bool
			for _, state := range states {
				if state.Fingerprint == model.SenseNovaFingerprint(first) && state.Scope == "glm-5.2" {
					failureFound = true
					assert.Equal(t, "upstream_unavailable", state.Reason)
					assert.Zero(t, state.LastSuccessAt)
				}
			}
			require.True(t, failureFound)
			if !deliveredTool {
				_, err := getChannel(c, info, &service.RetryParam{Ctx: c, ModelName: "glm-5.2", Retry: common.GetPointer(1)})
				require.Nil(t, err)
				assert.NotEqual(t, first, common.GetContextKeyString(c, constant.ContextKeyChannelKey))
			}
		})
	}
}
