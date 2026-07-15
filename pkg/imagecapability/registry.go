package imagecapability

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
)

const defaultMaxImages = 4

func Resolve(channelType int, modelName string) (Capability, bool) {
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	if normalized == "" {
		return Capability{}, false
	}

	resolution := GeminiImageResolution(normalized)
	if channelType == constant.ChannelTypeGemini {
		if strings.HasPrefix(normalized, "imagen-") {
			return imagenCapability(), true
		}
		if isNativeGeminiImageModel(normalized) {
			return nativeGeminiCapability(normalized, resolution), true
		}
		return Capability{}, false
	}

	if channelType == constant.ChannelTypeXai {
		if strings.HasPrefix(normalized, "grok-imagine-image") {
			return xaiCapability(), true
		}
		return Capability{}, false
	}

	switch {
	case normalized == "gpt-image-2":
		return gptImage2Capability(), true
	case strings.HasPrefix(normalized, "gpt-image-") || normalized == "chatgpt-image-latest":
		return openAIImageCapability(), true
	case normalized == "dall-e" || normalized == "dall-e-2":
		return dallE2Capability(), true
	case normalized == "dall-e-3":
		return dallE3Capability(), true
	case strings.HasPrefix(normalized, "grok-imagine-image"):
		return xaiCapability(), true
	case strings.HasPrefix(normalized, "imagen-"):
		return imagenCapability(), true
	case isNativeGeminiImageModel(normalized):
		return nativeGeminiCapability(normalized, resolution), true
	case strings.HasPrefix(normalized, "flux-") || strings.HasPrefix(normalized, "flux.1-"):
		return genericImageCapability(), true
	default:
		return Capability{}, false
	}
}

func GeminiImageResolution(modelName string) string {
	lower := strings.ToLower(strings.TrimSpace(modelName))
	if !isNativeGeminiImageModel(lower) {
		return ""
	}
	for _, suffix := range []string{"-1k", "-2k", "-4k"} {
		if strings.HasSuffix(lower, suffix) {
			return strings.ToUpper(strings.TrimPrefix(suffix, "-"))
		}
	}
	return ""
}

func ApplyModelAliasDefaults(capability Capability, modelName string) Capability {
	resolution := GeminiImageResolution(modelName)
	if resolution != "" {
		capability.DefaultResolution = resolution
		capability.Resolutions = nil
	}
	return capability
}

func Intersect(left Capability, right Capability) Capability {
	provider := left.Provider
	if provider != right.Provider {
		provider = ProviderMixed
	}

	sizeMode := left.SizeMode
	if sizeMode != right.SizeMode {
		sizeMode = SizeModeNone
	}

	result := Capability{
		Provider:                  provider,
		SizeMode:                  sizeMode,
		Sizes:                     intersectValues(left.Sizes, right.Sizes),
		AspectRatios:              intersectValues(left.AspectRatios, right.AspectRatios),
		Resolutions:               intersectValues(left.Resolutions, right.Resolutions),
		Qualities:                 intersectValues(left.Qualities, right.Qualities),
		OutputFormats:             intersectValues(left.OutputFormats, right.OutputFormats),
		SupportsEditing:           left.SupportsEditing && right.SupportsEditing,
		SupportsModeration:        left.SupportsModeration && right.SupportsModeration,
		SupportsOutputCompression: left.SupportsOutputCompression && right.SupportsOutputCompression,
		MaxImages:                 minimumPositive(left.MaxImages, right.MaxImages),
	}
	if sizeMode == SizeModeNone {
		result.Sizes = nil
		result.AspectRatios = nil
		result.Resolutions = nil
	}
	result.DefaultSize = intersectedDefault(left.DefaultSize, right.DefaultSize, result.Sizes)
	result.DefaultAspectRatio = intersectedDefault(left.DefaultAspectRatio, right.DefaultAspectRatio, result.AspectRatios)
	result.DefaultResolution = intersectedDefault(left.DefaultResolution, right.DefaultResolution, result.Resolutions)
	if result.DefaultResolution == "" && sizeMode == SizeModeAspectRatioResolution && len(result.Resolutions) == 0 &&
		left.DefaultResolution != "" && strings.EqualFold(left.DefaultResolution, right.DefaultResolution) {
		result.DefaultResolution = left.DefaultResolution
	}
	result.DefaultQuality = intersectedDefault(left.DefaultQuality, right.DefaultQuality, result.Qualities)
	result.DefaultOutputFormat = intersectedDefault(left.DefaultOutputFormat, right.DefaultOutputFormat, result.OutputFormats)
	return result
}

func gptImage2Capability() Capability {
	return Capability{
		Provider:                  ProviderOpenAI,
		SizeMode:                  SizeModeDimensions,
		Sizes:                     []string{"auto", "1024x1024", "1024x1536", "1536x1024", "1024x1792", "1792x1024", "2048x2048", "2560x1440", "1440x2560", "3840x2160", "2160x3840"},
		Qualities:                 []string{"auto", "low", "medium", "high"},
		OutputFormats:             []string{"png", "jpeg", "webp"},
		DefaultSize:               "1024x1024",
		DefaultQuality:            "auto",
		DefaultOutputFormat:       "png",
		SupportsEditing:           true,
		SupportsModeration:        true,
		SupportsOutputCompression: true,
		MaxImages:                 defaultMaxImages,
	}
}

func openAIImageCapability() Capability {
	return Capability{
		Provider:                  ProviderOpenAI,
		SizeMode:                  SizeModeDimensions,
		Sizes:                     []string{"auto", "1024x1024", "1024x1536", "1536x1024"},
		Qualities:                 []string{"auto", "low", "medium", "high"},
		OutputFormats:             []string{"png", "jpeg", "webp"},
		DefaultSize:               "1024x1024",
		DefaultQuality:            "auto",
		DefaultOutputFormat:       "png",
		SupportsEditing:           true,
		SupportsModeration:        true,
		SupportsOutputCompression: true,
		MaxImages:                 defaultMaxImages,
	}
}

func dallE2Capability() Capability {
	return Capability{
		Provider:    ProviderOpenAI,
		SizeMode:    SizeModeDimensions,
		Sizes:       []string{"256x256", "512x512", "1024x1024"},
		DefaultSize: "1024x1024",
		MaxImages:   defaultMaxImages,
	}
}

func dallE3Capability() Capability {
	return Capability{
		Provider:       ProviderOpenAI,
		SizeMode:       SizeModeDimensions,
		Sizes:          []string{"1024x1024", "1024x1792", "1792x1024"},
		Qualities:      []string{"standard", "hd"},
		DefaultSize:    "1024x1024",
		DefaultQuality: "standard",
		MaxImages:      defaultMaxImages,
	}
}

func xaiCapability() Capability {
	return Capability{
		Provider:           ProviderXAI,
		SizeMode:           SizeModeAspectRatioResolution,
		AspectRatios:       []string{"auto", "1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3", "2:1", "1:2", "19.5:9", "9:19.5", "20:9", "9:20"},
		Resolutions:        []string{"1K", "2K"},
		DefaultAspectRatio: "auto",
		DefaultResolution:  "1K",
		MaxImages:          defaultMaxImages,
	}
}

func imagenCapability() Capability {
	return Capability{
		Provider:           ProviderImagen,
		SizeMode:           SizeModeAspectRatioResolution,
		AspectRatios:       []string{"1:1", "3:4", "4:3", "9:16", "16:9"},
		Resolutions:        []string{"1K", "2K"},
		DefaultAspectRatio: "1:1",
		DefaultResolution:  "1K",
		MaxImages:          defaultMaxImages,
	}
}

func nativeGeminiCapability(modelName string, resolution string) Capability {
	resolutions := []string{"1K"}
	if strings.HasPrefix(modelName, "gemini-3") {
		resolutions = []string{"1K", "2K", "4K"}
	}
	resolutionLocked := resolution != ""
	if !resolutionLocked {
		resolution = resolutions[0]
	}
	if resolutionLocked {
		resolutions = nil
	}
	return Capability{
		Provider:           ProviderGemini,
		SizeMode:           SizeModeAspectRatioResolution,
		AspectRatios:       []string{"1:1", "1:4", "1:8", "2:3", "3:2", "3:4", "4:1", "4:3", "4:5", "5:4", "8:1", "9:16", "16:9", "21:9"},
		Resolutions:        resolutions,
		DefaultAspectRatio: "1:1",
		DefaultResolution:  resolution,
		MaxImages:          defaultMaxImages,
	}
}

func genericImageCapability() Capability {
	return Capability{
		Provider:    ProviderOther,
		SizeMode:    SizeModeDimensions,
		Sizes:       []string{"1024x1024", "1024x1536", "1536x1024"},
		DefaultSize: "1024x1024",
		MaxImages:   defaultMaxImages,
	}
}

func isNativeGeminiImageModel(modelName string) bool {
	return strings.HasPrefix(modelName, "gemini-") && strings.Contains(modelName, "image")
}

func intersectValues(left []string, right []string) []string {
	values := make([]string, 0)
	for _, value := range left {
		if containsValue(right, value) {
			values = append(values, value)
		}
	}
	return values
}

func containsValue(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func intersectedDefault(left string, right string, values []string) string {
	if left != "" && strings.EqualFold(left, right) && containsValue(values, left) {
		return left
	}
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

func minimumPositive(left int, right int) int {
	if left <= 0 {
		return right
	}
	if right <= 0 || left < right {
		return left
	}
	return right
}
