package xai

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertImageRequestUsesExplicitAspectRatioAndResolution(t *testing.T) {
	request := dto.ImageRequest{
		Model:       "grok-imagine-image",
		Prompt:      "a city at night",
		AspectRatio: "16:9",
		Resolution:  "2K",
	}

	converted := ConvertImageRequest(request)
	assert.Equal(t, "16:9", converted.AspectRatio)
	assert.Equal(t, "2k", converted.Resolution)
}

func TestConvertImageRequestKeepsLegacySizeMapping(t *testing.T) {
	converted := ConvertImageRequest(dto.ImageRequest{Size: "1024x1536"})
	assert.Equal(t, "2:3", converted.AspectRatio)
	assert.Empty(t, converted.Resolution)
}

func TestAdaptorRejectsImageEditing(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}
	_, err := (&Adaptor{}).ConvertImageRequest(nil, info, dto.ImageRequest{})
	require.EqualError(t, err, "xAI image editing is not supported by the images edits endpoint")
}
