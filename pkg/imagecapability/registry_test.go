package imagecapability

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveProviderImageCapabilities(t *testing.T) {
	tests := []struct {
		name              string
		channelType       int
		model             string
		provider          string
		sizeMode          SizeMode
		defaultResolution string
	}{
		{name: "gpt image", model: "gpt-image-2", provider: ProviderOpenAI, sizeMode: SizeModeDimensions},
		{name: "xai image", channelType: constant.ChannelTypeXai, model: "grok-imagine-image", provider: ProviderXAI, sizeMode: SizeModeAspectRatioResolution, defaultResolution: "1K"},
		{name: "imagen", channelType: constant.ChannelTypeGemini, model: "imagen-4.0-generate-001", provider: ProviderImagen, sizeMode: SizeModeAspectRatioResolution, defaultResolution: "1K"},
		{name: "gemini native alias", channelType: constant.ChannelTypeGemini, model: "gemini-3.1-flash-image-4k", provider: ProviderGemini, sizeMode: SizeModeAspectRatioResolution, defaultResolution: "4K"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capability, ok := Resolve(test.channelType, test.model)
			require.True(t, ok)
			assert.Equal(t, test.provider, capability.Provider)
			assert.Equal(t, test.sizeMode, capability.SizeMode)
			assert.Equal(t, test.defaultResolution, capability.DefaultResolution)
		})
	}
}

func TestResolveRejectsUnsupportedProviderModelPair(t *testing.T) {
	_, ok := Resolve(constant.ChannelTypeGemini, "gpt-image-2")
	assert.False(t, ok)

	_, ok = Resolve(constant.ChannelTypeXai, "gemini-3.1-flash-image")
	assert.False(t, ok)
}

func TestIntersectUsesConservativeSharedCapabilities(t *testing.T) {
	left, ok := Resolve(0, "gpt-image-2")
	require.True(t, ok)
	right := left
	right.Sizes = []string{"1024x1024"}
	right.DefaultSize = "1024x1024"
	right.SupportsEditing = false

	result := Intersect(left, right)
	assert.Equal(t, []string{"1024x1024"}, result.Sizes)
	assert.Equal(t, "1024x1024", result.DefaultSize)
	assert.False(t, result.SupportsEditing)
}

func TestNormalizeGeminiImageModelExtractsResolutionAlias(t *testing.T) {
	model, resolution := NormalizeGeminiImageModel("gemini-3.1-flash-image-2k")
	assert.Equal(t, "gemini-3.1-flash-image", model)
	assert.Equal(t, "2K", resolution)
}

func TestApplyModelAliasDefaultsPreservesPublicResolution(t *testing.T) {
	capability, ok := Resolve(constant.ChannelTypeGemini, "gemini-3.1-flash-image")
	require.True(t, ok)

	capability = ApplyModelAliasDefaults(capability, "public-gemini-image-4k")
	assert.Equal(t, "1K", capability.DefaultResolution)

	capability = ApplyModelAliasDefaults(capability, "gemini-3.1-flash-image-4k")
	assert.Equal(t, "4K", capability.DefaultResolution)
}
