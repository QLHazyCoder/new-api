package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveModelMappingFollowsMappingChain(t *testing.T) {
	model, mapped, err := ResolveModelMapping("public-image", `{"public-image":"image-alias","image-alias":"gpt-image-2"}`)
	require.NoError(t, err)
	assert.True(t, mapped)
	assert.Equal(t, "gpt-image-2", model)
}

func TestResolveModelMappingRejectsCycle(t *testing.T) {
	_, _, err := ResolveModelMapping("model-a", `{"model-a":"model-b","model-b":"model-a"}`)
	require.EqualError(t, err, "model_mapping_contains_cycle")
}

func TestResolveModelMappingTreatsSelfMappingAsNoop(t *testing.T) {
	model, mapped, err := ResolveModelMapping("gpt-image-2", `{"gpt-image-2":"gpt-image-2"}`)
	require.NoError(t, err)
	assert.False(t, mapped)
	assert.Equal(t, "gpt-image-2", model)
}

func TestImageCapabilityUsesMappedUpstreamModel(t *testing.T) {
	mapping := `{"public-gemini-image":"gemini-3.1-flash-image"}`
	assert.True(t, IsChannelImageGenerationModel(constant.ChannelTypeGemini, "public-gemini-image", mapping))
	assert.False(t, IsChannelImageGenerationModel(constant.ChannelTypeGemini, "public-gemini-image"))
}
