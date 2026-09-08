package openai

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSenseNovaLatencyChatAndClaudeStreamError(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
		for _, hasAnswer := range []bool{false, true} {
			name := string(format) + "/error-only"
			if hasAnswer {
				name = string(format) + "/answer-then-error"
			}
			t.Run(name, func(t *testing.T) {
				c, _, info, adaptor := senseNovaResponsesFixture(t, true)
				info.RelayMode = relayconstant.RelayModeChatCompletions
				info.RelayFormat = format
				info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}
				body := "data: " + `{"error":{"code":429001,"message":"private provider error"}}` + "\n\ndata: [DONE]\n\n"
				if hasAnswer {
					body = "data: " + `{"choices":[{"index":0,"delta":{"role":"assistant","content":"private answer"}}]}` + "\n\n" + body
				}
				service.MarkSenseNovaLatencyDispatch(c)()
				resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
				_, apiErr := adaptor.DoResponse(c, resp, info)
				require.NotNil(t, apiErr)
				entry := service.SenseNovaLatencyLogInfo(c)
				require.NotNil(t, entry)
				require.Len(t, entry.Attempts, 1)
				assert.Equal(t, "stream_error", entry.Attempts[0].Outcome)
				assert.Equal(t, hasAnswer, entry.Attempts[0].FirstSemanticMS != nil)
				assert.Equal(t, hasAnswer, entry.Attempts[0].FirstAnswerMS != nil)
				service.CompleteSenseNovaLatency(c, false)
				final := service.SenseNovaLatencyLogInfo(c)
				assert.Equal(t, "stream_error", final.Outcome)
				data, err := common.Marshal(final)
				require.NoError(t, err)
				assert.NotContains(t, string(data), "private")
			})
		}
	}
}
