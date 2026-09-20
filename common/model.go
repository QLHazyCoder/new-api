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

// IsChannelImageGenerationModel resolves a channel mapping before checking the
// shared image-capability registry. This keeps image endpoint selection in
// sync with the capability system rather than relying on model-name heuristics.
func IsChannelImageGenerationModel(channelType int, modelName string, modelMappings ...string) bool {
	if len(modelMappings) > 0 {
		mappedModel, _, err := ResolveModelMapping(modelName, modelMappings[0])
		if err != nil {
			return false
		}
		modelName = mappedModel
	}
	_, ok := imagecapability.Resolve(channelType, modelName)
	// The capability registry is authoritative when it has a channel-specific
	// rule. Keep the upstream legacy catalogue as a fallback for compatible
	// image models that do not yet need Playground capability metadata.
	return ok || IsImageGenerationModel(modelName)
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
