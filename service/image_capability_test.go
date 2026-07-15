package service

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildImageModelOptionsFiltersGroupsAndUsesModelMapping(t *testing.T) {
	abilities := []model.AbilityWithChannel{
		{
			Ability:             model.Ability{Group: "image", Model: "public-image"},
			ChannelType:         constant.ChannelTypeOpenAI,
			ChannelModelMapping: `{"public-image":"gpt-image-2"}`,
		},
		{
			Ability:     model.Ability{Group: "image", Model: "text-only"},
			ChannelType: constant.ChannelTypeOpenAI,
		},
		{
			Ability:     model.Ability{Group: "other", Model: "grok-imagine-image"},
			ChannelType: constant.ChannelTypeXai,
		},
	}

	options := buildImageModelOptions(abilities, map[string]bool{"image": true})
	require.Len(t, options, 1)
	assert.Equal(t, "public-image", options[0].Value)
	assert.Equal(t, "openai", options[0].Capabilities.Provider)
	assert.Equal(t, "dimensions", options[0].Capabilities.SizeMode)
}

func TestBuildImageModelOptionsIntersectsCandidateChannelCapabilities(t *testing.T) {
	abilities := []model.AbilityWithChannel{
		{
			Ability:             model.Ability{Group: "auto-a", Model: "public-image"},
			ChannelType:         constant.ChannelTypeOpenAI,
			ChannelModelMapping: `{"public-image":"gpt-image-2"}`,
		},
		{
			Ability:             model.Ability{Group: "auto-b", Model: "public-image"},
			ChannelType:         constant.ChannelTypeOpenAI,
			ChannelModelMapping: `{"public-image":"dall-e-3"}`,
		},
	}

	options := buildImageModelOptions(abilities, map[string]bool{"auto-a": true, "auto-b": true})
	require.Len(t, options, 1)
	assert.Equal(t, []string{"1024x1024", "1024x1792", "1792x1024"}, options[0].Capabilities.Sizes)
	assert.Empty(t, options[0].Capabilities.OutputFormats)
	assert.False(t, options[0].Capabilities.SupportsEditing)
}

func TestBuildImageModelOptionsSkipsInvalidMappingAndUnsupportedPair(t *testing.T) {
	abilities := []model.AbilityWithChannel{
		{
			Ability:             model.Ability{Group: "image", Model: "cycle"},
			ChannelType:         constant.ChannelTypeOpenAI,
			ChannelModelMapping: `{"cycle":"other","other":"cycle"}`,
		},
		{
			Ability:     model.Ability{Group: "image", Model: "gpt-image-2"},
			ChannelType: constant.ChannelTypeGemini,
		},
	}

	options := buildImageModelOptions(abilities, map[string]bool{"image": true})
	assert.Empty(t, options)
}

func TestBuildImageModelOptionsPreservesGeminiPublicResolutionAlias(t *testing.T) {
	abilities := []model.AbilityWithChannel{
		{
			Ability:             model.Ability{Group: "gemini", Model: "gemini-3.1-flash-image-4k"},
			ChannelType:         constant.ChannelTypeGemini,
			ChannelModelMapping: `{"gemini-3.1-flash-image-4k":"gemini-3.1-flash-image"}`,
		},
	}

	options := buildImageModelOptions(abilities, map[string]bool{"gemini": true})
	require.Len(t, options, 1)
	assert.Equal(t, "4K", options[0].Capabilities.DefaultResolution)
}
