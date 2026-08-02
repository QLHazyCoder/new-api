package service

import "github.com/QuantumNous/new-api/setting/model_setting"

func ShouldChatCompletionsUseResponsesPolicy(policy model_setting.ChatCompletionsToResponsesPolicy, channelID int, channelType int, model string) bool {
	_ = policy
	_ = channelID
	_ = channelType
	_ = model

	// Public endpoints preserve the requested protocol and are never silently
	// upgraded from Chat Completions to Responses.
	return false
}

func ShouldChatCompletionsUseResponsesGlobal(channelID int, channelType int, model string) bool {
	_ = channelID
	_ = channelType
	_ = model
	return false
}
