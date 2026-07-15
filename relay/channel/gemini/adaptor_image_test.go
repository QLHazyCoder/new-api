package gemini

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertImageRequestBuildsNativeGeminiRequest(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3.1-flash-image-2k",
		},
	}
	request := dto.ImageRequest{
		Prompt:      "a glass sculpture",
		AspectRatio: "16:9",
		Resolution:  "4K",
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(nil, info, request)
	require.NoError(t, err)
	require.IsType(t, &dto.GeminiChatRequest{}, converted)
	geminiRequest := converted.(*dto.GeminiChatRequest)
	assert.Equal(t, "gemini-3.1-flash-image", info.UpstreamModelName)
	require.Len(t, geminiRequest.Contents, 1)
	assert.Equal(t, "a glass sculpture", geminiRequest.Contents[0].Parts[0].Text)
	assert.Equal(t, []string{"TEXT", "IMAGE"}, geminiRequest.GenerationConfig.ResponseModalities)

	var imageConfig map[string]string
	require.NoError(t, common.Unmarshal(geminiRequest.GenerationConfig.ImageConfig, &imageConfig))
	assert.Equal(t, "16:9", imageConfig["aspectRatio"])
	assert.Equal(t, "4K", imageConfig["imageSize"])
}

func TestConvertImageRequestUsesGeminiAliasResolution(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3.1-flash-image-2k",
		},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(nil, info, dto.ImageRequest{Prompt: "test"})
	require.NoError(t, err)
	geminiRequest := converted.(*dto.GeminiChatRequest)
	var imageConfig map[string]string
	require.NoError(t, common.Unmarshal(geminiRequest.GenerationConfig.ImageConfig, &imageConfig))
	assert.Equal(t, "2K", imageConfig["imageSize"])
}

func TestConvertImageRequestPreservesOriginAliasResolutionAfterMapping(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "gemini-3.1-flash-image-4k",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3.1-flash-image",
		},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(nil, info, dto.ImageRequest{Prompt: "test"})
	require.NoError(t, err)
	geminiRequest := converted.(*dto.GeminiChatRequest)
	var imageConfig map[string]string
	require.NoError(t, common.Unmarshal(geminiRequest.GenerationConfig.ImageConfig, &imageConfig))
	assert.Equal(t, "4K", imageConfig["imageSize"])
}

func TestConvertImageRequestBuildsImagenRequestWithExplicitResolution(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "imagen-4.0-generate-001",
		},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(nil, info, dto.ImageRequest{
		Prompt:      "a mountain lake",
		AspectRatio: "4:3",
		Resolution:  "2K",
	})
	require.NoError(t, err)
	request := converted.(dto.GeminiImageRequest)
	assert.Equal(t, "4:3", request.Parameters.AspectRatio)
	assert.Equal(t, "2K", request.Parameters.ImageSize)
}

func TestConvertImageRequestRejectsNativeGeminiBatch(t *testing.T) {
	count := uint(2)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3.1-flash-image",
		},
	}

	_, err := (&Adaptor{}).ConvertImageRequest(nil, info, dto.ImageRequest{Prompt: "test", N: &count})
	require.EqualError(t, err, "Gemini native image generation supports n=1 per request")
}
