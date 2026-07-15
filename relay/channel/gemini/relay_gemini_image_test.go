package gemini

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiNativeImageHandlerNormalizesInlineImagesAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/pg/images/generations", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3.1-flash-image",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3.1-flash-image",
		},
	}
	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Parts: []dto.GeminiPart{
						{Text: "generated"},
						{InlineData: &dto.GeminiInlineData{MimeType: "image/png", Data: "aW1hZ2U="}},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 20,
			TotalTokenCount:      30,
		},
	}
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiNativeImageHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	assert.Equal(t, 10, usage.PromptTokens)
	assert.Equal(t, 20, usage.CompletionTokens)

	var imageResponse dto.ImageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &imageResponse))
	require.Len(t, imageResponse.Data, 1)
	assert.Equal(t, "aW1hZ2U=", imageResponse.Data[0].B64Json)
	assert.Equal(t, "image/png", imageResponse.Data[0].MimeType)
}

func TestGeminiNativeImageHandlerRejectsResponseWithoutImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/pg/images/generations", nil)
	blockReason := "SAFETY"
	payload := dto.GeminiChatResponse{
		PromptFeedback: &dto.GeminiChatPromptFeedback{BlockReason: &blockReason},
	}
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-image"}}

	usage, newAPIError := GeminiNativeImageHandler(c, info, resp)
	assert.Nil(t, usage)
	require.NotNil(t, newAPIError)
	assert.Contains(t, newAPIError.Error(), "no images generated")
}

func TestGeminiNativeImageHandlerEstimatesUsageWithoutMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/pg/images/generations", nil)
	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Parts: []dto.GeminiPart{
						{InlineData: &dto.GeminiInlineData{MimeType: "image/png", Data: "aW1hZ2U="}},
					},
				},
			},
		},
	}
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3.1-flash-image",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3.1-flash-image",
		},
	}

	usage, newAPIError := GeminiNativeImageHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	assert.Equal(t, 1400, usage.CompletionTokens)
	assert.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
}
