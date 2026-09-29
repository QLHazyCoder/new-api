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
	ImageGenerationModels = []string{
		"dall-e-3",
		"dall-e-2",
		"prefix:dall-e", // Deprecated upstream models; retained for compatible routes.
		"gpt-image-",
		"qwen-image",
		"z-image",
		"wan2.7-image-pro",
		"wan2.7-image",
		"wan2.6-image",
		"wan2.6-t2i",
		"wan2.5-t2i-preview",
		"wan2.2-t2i-flash",
		"wan2.2-t2i-plus",
		"wanx2.1-t2i-turbo",
		"wanx2.1-t2i-plus",
		"wanx2.0-t2i-turbo",
		"prefix:imagen-",
		"flux-",
		"flux.1-",
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
	modelName = strings.ToLower(modelName)
	for _, m := range ImageGenerationModels {
		if prefix, ok := strings.CutPrefix(m, "prefix:"); ok {
			if strings.HasPrefix(modelName, prefix) {
				return true
			}
			continue
		}
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}

// ResolveChannelImageCapability resolves the public name through its channel
// mapping without changing the name that clients use in their requests.
func ResolveChannelImageCapability(channelType int, modelName string, modelMappings ...string) (imagecapability.Capability, bool) {
	if len(modelMappings) > 0 {
		mappedModel, _, err := ResolveModelMapping(modelName, modelMappings[0])
		if err != nil {
			return imagecapability.Capability{}, false
		}
		publicModel := modelName
		modelName = mappedModel
		capability, ok := imagecapability.Resolve(channelType, modelName)
		return imagecapability.ApplyModelAliasDefaults(capability, publicModel), ok
	}
	return imagecapability.Resolve(channelType, modelName)
}

// IsChannelImageGenerationModel keeps the legacy image model list for routes
// that have not yet been given configurable Playground capabilities.
func IsChannelImageGenerationModel(channelType int, modelName string, modelMappings ...string) bool {
	_, ok := ResolveChannelImageCapability(channelType, modelName, modelMappings...)
	if len(modelMappings) > 0 {
		mappedModel, _, err := ResolveModelMapping(modelName, modelMappings[0])
		if err != nil {
			return false
		}
		modelName = mappedModel
	}
	// A configured exclusion must not be undone by the legacy name list.
	return ok || !imagecapability.KnownModel(modelName) && IsImageGenerationModel(modelName)
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
