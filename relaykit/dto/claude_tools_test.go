package dto

import (
	"testing"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeRequestWireToolTokenCountMeta(t *testing.T) {
	var req ClaudeRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{
		"model": "claude-sonnet-4-5",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "Find local weather"}],
		"tools": [
			{
				"name": "web_search",
				"description": "Look up weather using a custom tool",
				"input_schema": {
					"type": "object",
					"properties": {
						"place": {
							"type": "object",
							"properties": {"city": {"type": "string", "description": "City to forecast"}},
							"required": ["city"]
						}
					},
					"required": ["place"]
				},
				"cache_control": {"type": "ephemeral"}
			},
			{
				"type": "web_search_20250305",
				"name": "web_search",
				"max_uses": 3,
				"allowed_domains": ["weather.example.com"],
				"user_location": {
					"type": "approximate",
					"timezone": "Asia/Shanghai",
					"country": "CN",
					"region": "Shanghai",
					"city": "Shanghai"
				}
			}
		]
	}`, &req))
	before, err := kitutil.Marshal(req)
	require.NoError(t, err)
	require.IsType(t, map[string]any{}, req.GetTools()[0])

	meta := req.GetTokenCountMeta()
	assert.Equal(t, 2, meta.ToolsCount)
	assert.Equal(t, 1, meta.MessagesCount)
	assert.Equal(t, 1024, meta.MaxTokens)
	assert.Contains(t, meta.CombineText, "web_search")
	assert.Contains(t, meta.CombineText, "Look up weather using a custom tool")
	assert.Contains(t, meta.CombineText, `{"properties":{"place":{"properties":{"city":{"description":"City to forecast","type":"string"}},"required":["city"],"type":"object"}},"required":["place"],"type":"object"}`)
	assert.Contains(t, meta.CombineText, `{"type":"approximate","timezone":"Asia/Shanghai","country":"CN","region":"Shanghai","city":"Shanghai"}`)

	normalTools, searchTools := ProcessTools(req.GetTools())
	assert.Len(t, normalTools, 1)
	assert.Len(t, searchTools, 1)
	after, err := kitutil.Marshal(req)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "accounting must preserve the full wire request, including extra tool fields")
}

func TestProcessToolsPreservesTypedTools(t *testing.T) {
	ordinary := Tool{Name: "forecast", Description: "Get a forecast", InputSchema: map[string]any{"type": "object"}}
	search := ClaudeWebSearchTool{
		Type: "web_search_20250305", Name: "web_search", MaxUses: 3,
		UserLocation: &ClaudeWebSearchUserLocation{Type: "approximate", City: "Shanghai"},
	}
	normalTools, searchTools := ProcessTools([]any{ordinary, &ordinary, search, &search})
	require.Len(t, normalTools, 2)
	require.Len(t, searchTools, 2)
	assert.Equal(t, ordinary, *normalTools[0])
	assert.Same(t, &ordinary, normalTools[1])
	assert.Equal(t, search, *searchTools[0])
	assert.Same(t, &search, searchTools[1])

	req := ClaudeRequest{Tools: []any{ordinary, &ordinary, search, &search}}
	meta := req.GetTokenCountMeta()
	assert.Equal(t, 4, meta.ToolsCount)
	assert.Contains(t, meta.CombineText, "Get a forecast")
	assert.Contains(t, meta.CombineText, `{"type":"object"}`)
	assert.Contains(t, meta.CombineText, `{"type":"approximate","city":"Shanghai"}`)
}

func TestProcessToolsSkipsNilAndUndecodableTools(t *testing.T) {
	cases := []struct {
		name string
		tool any
	}{
		{name: "nil"},
		{name: "nil ordinary pointer", tool: (*Tool)(nil)},
		{name: "nil search pointer", tool: (*ClaudeWebSearchTool)(nil)},
		{name: "nil map", tool: map[string]any(nil)},
		{name: "unsupported type", tool: "forecast"},
		{name: "invalid ordinary schema", tool: map[string]any{"name": "forecast", "input_schema": "invalid"}},
		{name: "invalid search location", tool: map[string]any{"type": "web_search_20250305", "name": "web_search", "user_location": "invalid"}},
		{name: "unserializable map", tool: map[string]any{"name": "forecast", "input_schema": make(chan int)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			normalTools, searchTools := ProcessTools([]any{tc.tool})
			assert.Empty(t, normalTools)
			assert.Empty(t, searchTools)
			req := ClaudeRequest{Tools: []any{tc.tool}}
			require.NotPanics(t, func() {
				meta := req.GetTokenCountMeta()
				assert.Zero(t, meta.ToolsCount)
				assert.Empty(t, meta.CombineText)
			})
		})
	}
}
