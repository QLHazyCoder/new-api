package imagecapability

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
		{name: "gemini native alias", channelType: constant.ChannelTypeGemini, model: "gemini-3.1-flash-image-4K", provider: ProviderGemini, sizeMode: SizeModeAspectRatioResolution, defaultResolution: "4K"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capability, ok := Resolve(test.channelType, test.model)
			require.True(t, ok)
			assert.Equal(t, test.provider, capability.Provider)
			assert.Equal(t, test.sizeMode, capability.SizeMode)
			assert.Equal(t, test.defaultResolution, capability.DefaultResolution)
			if test.name == "gemini native alias" {
				assert.Empty(t, capability.Resolutions)
			}
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

func TestConfiguredResolutionSuffixesAreCaseInsensitive(t *testing.T) {
	capability, ok := Resolve(constant.ChannelTypeGemini, "gemini-3.1-flash-image-2K")
	require.True(t, ok)
	assert.Equal(t, "2K", capability.DefaultResolution)
	assert.Empty(t, capability.Resolutions)
	assert.Equal(t, "2K", GeminiImageResolution("gemini-3.1-flash-image-2K"))

	capability, ok = Resolve(constant.ChannelTypeGemini, "gemini-3.1-flash-image-1k")
	require.True(t, ok)
	assert.Equal(t, "1K", capability.DefaultResolution)
	assert.Empty(t, capability.Resolutions)
	assert.Equal(t, "1K", GeminiImageResolution("gemini-3.1-flash-image-1k"))
}

func TestApplyModelAliasDefaultsPreservesPublicResolution(t *testing.T) {
	capability, ok := Resolve(constant.ChannelTypeGemini, "gemini-3.1-flash-image")
	require.True(t, ok)

	capability = ApplyModelAliasDefaults(capability, "public-gemini-image-4k")
	assert.Equal(t, "1K", capability.DefaultResolution)

	capability = ApplyModelAliasDefaults(capability, "gemini-3.1-flash-image-4k")
	assert.Equal(t, "4K", capability.DefaultResolution)
	assert.Empty(t, capability.Resolutions)
}

func TestRegistryCreatesAndReloadsExternalConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image-capabilities.json")
	registry := newCapabilityRegistry(path, defaultRules, defaultConfigJSON)

	capability, ok := registry.resolve(1, "gpt-image-2")
	require.True(t, ok)
	assert.Contains(t, capability.Sizes, "3840x2160")
	_, err := os.Stat(path)
	require.NoError(t, err)

	const replacement = `{
  "version": 1,
  "rules": [{
    "name": "custom-image",
    "match": {"exact_models": ["custom-image"]},
    "capabilities": {
      "provider": "other",
      "size_mode": "dimensions",
      "sizes": ["4096x4096"],
      "default_size": "4096x4096",
      "max_images": 1
    }
  }]
}`
	require.NoError(t, os.WriteFile(path, []byte(replacement), 0o640))
	registry.forceNextReloadForTest()

	capability, ok = registry.resolve(1, "custom-image")
	require.True(t, ok)
	assert.Equal(t, []string{"4096x4096"}, capability.Sizes)
	_, ok = registry.resolve(1, "gpt-image-2")
	assert.False(t, ok)
}

func TestRegistryRetainsLastValidExternalConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image-capabilities.json")
	const valid = `{
  "version": 1,
  "rules": [{
    "name": "custom-image",
    "match": {"exact_models": ["custom-image"]},
    "capabilities": {
      "provider": "other",
      "size_mode": "dimensions",
      "sizes": ["4096x4096"],
      "default_size": "4096x4096",
      "max_images": 1
    }
  }]
}`
	require.NoError(t, os.WriteFile(path, []byte(valid), 0o640))
	registry := newCapabilityRegistry(path, defaultRules, defaultConfigJSON)

	_, ok := registry.resolve(1, "custom-image")
	require.True(t, ok)
	require.NoError(t, os.WriteFile(path, []byte(`{"version": 1, "rules": []}`), 0o640))
	registry.forceNextReloadForTest()

	capability, ok := registry.resolve(1, "custom-image")
	require.True(t, ok)
	assert.Equal(t, []string{"4096x4096"}, capability.Sizes)
}

func (r *capabilityRegistry) forceNextReloadForTest() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastChecked = time.Time{}
}
