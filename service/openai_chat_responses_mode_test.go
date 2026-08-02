package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionsResponsesUpgradeRemainsDisabled(t *testing.T) {
	policy := model_setting.ChatCompletionsToResponsesPolicy{
		Enabled:       true,
		AllChannels:   true,
		ModelPatterns: []string{"^gpt-5.*$"},
	}

	require.False(t, ShouldChatCompletionsUseResponsesPolicy(policy, 1, 1, "gpt-5"))
	require.False(t, ShouldChatCompletionsUseResponsesGlobal(1, 1, "gpt-5"))
}
