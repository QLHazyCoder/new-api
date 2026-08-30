package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSensitiveWordPromptExtractionAcrossRelayProtocols(t *testing.T) {
	tests := []struct {
		name   string
		marker string
		text   func() string
	}{
		{
			name:   "OpenAI Chat",
			marker: "openai-chat-sensitive-marker",
			text: func() string {
				return (&GeneralOpenAIRequest{Messages: []Message{{Role: "user", Content: "openai-chat-sensitive-marker"}}}).GetTokenCountMeta().CombineText
			},
		},
		{
			name:   "OpenAI Responses",
			marker: "openai-responses-sensitive-marker",
			text: func() string {
				return (&OpenAIResponsesRequest{Input: json.RawMessage(`[{"role":"user","content":[{"type":"input_text","text":"openai-responses-sensitive-marker"}]}]`)}).GetTokenCountMeta().CombineText
			},
		},
		{
			name:   "Claude Messages",
			marker: "claude-sensitive-marker",
			text: func() string {
				return (&ClaudeRequest{System: "claude-system", Messages: []ClaudeMessage{{Role: "user", Content: "claude-sensitive-marker"}}}).GetTokenCountMeta().CombineText
			},
		},
		{
			name:   "Gemini GenerateContent",
			marker: "gemini-sensitive-marker",
			text: func() string {
				return (&GeminiChatRequest{Contents: []GeminiChatContent{{Role: "user", Parts: []GeminiPart{{Text: "gemini-sensitive-marker"}}}}}).GetTokenCountMeta().CombineText
			},
		},
		{
			name:   "OpenAI Image",
			marker: "image-sensitive-marker",
			text: func() string {
				return (&ImageRequest{Prompt: "image-sensitive-marker"}).GetTokenCountMeta().CombineText
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Contains(t, test.text(), test.marker)
		})
	}
}
