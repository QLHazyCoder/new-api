package common

import (
	"strings"

	"github.com/QuantumNous/new-api/pkg/imagecapability"
)

var (
	// OpenAIResponseOnlyModels is a list of models that are only available for OpenAI responses.
	OpenAIResponseOnlyModels = []string{
		"o3-pro",
		"o3-deep-research",
		"o4-mini-deep-research",
	}
	OpenAITextModels = []string{
		"gpt-",
		"o1",
		"o3",
		"o4",
		"chatgpt",
	}
)

func IsOpenAIResponseOnlyModel(modelName string) bool {
	for _, m := range OpenAIResponseOnlyModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}

func IsImageGenerationModel(modelName string) bool {
	_, ok := imagecapability.Resolve(0, modelName)
	return ok
}

func IsChannelImageGenerationModel(channelType int, modelName string, modelMappings ...string) bool {
	if len(modelMappings) > 0 {
		mappedModel, _, err := ResolveModelMapping(modelName, modelMappings[0])
		if err != nil {
			return false
		}
		modelName = mappedModel
	}
	_, ok := imagecapability.Resolve(channelType, modelName)
	return ok
}

func IsOpenAITextModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	for _, m := range OpenAITextModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}
