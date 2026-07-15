package gemini

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	channelconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/imagecapability"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if len(request.Contents) > 0 {
		for i, content := range request.Contents {
			if i == 0 {
				if request.Contents[0].Role == "" {
					request.Contents[0].Role = "user"
				}
			}
			for _, part := range content.Parts {
				if part.FileData != nil {
					if part.FileData.MimeType == "" && strings.Contains(part.FileData.FileUri, "www.youtube.com") {
						part.FileData.MimeType = "video/webm"
					}
				}
			}
		}
	}
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	result, err := relayconvert.ConvertRequest(c, info, types.RelayFormatGemini, req)
	if err != nil {
		return nil, err
	}
	geminiRequest, ok := result.Value.(*dto.GeminiChatRequest)
	if !ok {
		return nil, fmt.Errorf("expected Gemini generateContent request, got %T", result.Value)
	}
	return geminiRequest, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info.RelayMode == constant.RelayModeImagesEdits {
		return nil, errors.New("Gemini image editing is not supported by the images edits endpoint")
	}

	aliasResolution := imagecapability.GeminiImageResolution(info.UpstreamModelName)
	if aliasResolution == "" {
		aliasResolution = imagecapability.GeminiImageResolution(info.OriginModelName)
	}
	capability, ok := imagecapability.Resolve(channelconstant.ChannelTypeGemini, info.UpstreamModelName)
	if !ok {
		return nil, fmt.Errorf("model %s does not support Gemini image generation", info.UpstreamModelName)
	}

	if capability.Provider == imagecapability.ProviderImagen {
		return convertImagenRequest(request, capability)
	}
	return convertNativeGeminiImageRequest(request, capability, aliasResolution)
}

func convertImagenRequest(request dto.ImageRequest, capability imagecapability.Capability) (dto.GeminiImageRequest, error) {
	aspectRatio, err := requestedAspectRatio(request, capability)
	if err != nil {
		return dto.GeminiImageRequest{}, err
	}
	resolution, err := requestedImageResolution(request, capability, true)
	if err != nil {
		return dto.GeminiImageRequest{}, err
	}

	return dto.GeminiImageRequest{
		Instances: []dto.GeminiImageInstance{
			{
				Prompt: request.Prompt,
			},
		},
		Parameters: dto.GeminiImageParameters{
			SampleCount:      int(lo.FromPtrOr(request.N, uint(1))),
			AspectRatio:      aspectRatio,
			PersonGeneration: "allow_adult",
			ImageSize:        resolution,
		},
	}, nil
}

func convertNativeGeminiImageRequest(request dto.ImageRequest, capability imagecapability.Capability, aliasResolution string) (*dto.GeminiChatRequest, error) {
	if lo.FromPtrOr(request.N, uint(1)) > 1 {
		return nil, errors.New("Gemini native image generation supports n=1 per request")
	}
	aspectRatio, err := requestedAspectRatio(request, capability)
	if err != nil {
		return nil, err
	}
	if aliasResolution != "" && request.Resolution != "" && !strings.EqualFold(request.Resolution, aliasResolution) {
		return nil, fmt.Errorf("resolution %s conflicts with model resolution %s", request.Resolution, aliasResolution)
	}
	if aliasResolution != "" {
		request.Resolution = aliasResolution
	}
	resolution, err := requestedImageResolution(request, capability, false)
	if err != nil {
		return nil, err
	}
	imageConfig, err := common.Marshal(map[string]string{
		"aspectRatio": aspectRatio,
		"imageSize":   resolution,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Gemini image config: %w", err)
	}

	return &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{
				Role: "user",
				Parts: []dto.GeminiPart{
					{Text: request.Prompt},
				},
			},
		},
		GenerationConfig: dto.GeminiChatGenerationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
			ImageConfig:        imageConfig,
		},
	}, nil
}

func requestedAspectRatio(request dto.ImageRequest, capability imagecapability.Capability) (string, error) {
	aspectRatio := strings.TrimSpace(request.AspectRatio)
	if aspectRatio == "" {
		size := strings.TrimSpace(request.Size)
		if strings.Contains(size, ":") {
			aspectRatio = size
		} else {
			switch size {
			case "256x256", "512x512", "1024x1024":
				aspectRatio = "1:1"
			case "1536x1024":
				aspectRatio = "3:2"
			case "1024x1536":
				aspectRatio = "2:3"
			case "1024x1792":
				aspectRatio = "9:16"
			case "1792x1024":
				aspectRatio = "16:9"
			}
		}
	}
	if aspectRatio == "" {
		aspectRatio = capability.DefaultAspectRatio
	}
	if !containsImageOption(capability.AspectRatios, aspectRatio) {
		return "", fmt.Errorf("aspect_ratio %s is not supported by this image model", aspectRatio)
	}
	return aspectRatio, nil
}

func requestedImageResolution(request dto.ImageRequest, capability imagecapability.Capability, legacyQuality bool) (string, error) {
	resolution := strings.ToUpper(strings.TrimSpace(request.Resolution))
	if resolution == "" && legacyQuality {
		switch strings.ToLower(strings.TrimSpace(request.Quality)) {
		case "hd", "high", "2k":
			resolution = "2K"
		case "standard", "medium", "low", "auto", "1k":
			resolution = "1K"
		}
	}
	if resolution == "" {
		resolution = capability.DefaultResolution
	}
	if len(capability.Resolutions) == 0 {
		if capability.DefaultResolution == "" || !strings.EqualFold(capability.DefaultResolution, resolution) {
			return "", fmt.Errorf("resolution %s is not supported by this image model", resolution)
		}
		return capability.DefaultResolution, nil
	}
	if !containsImageOption(capability.Resolutions, resolution) {
		return "", fmt.Errorf("resolution %s is not supported by this image model", resolution)
	}
	return resolution, nil
}

func containsImageOption(options []string, value string) bool {
	for _, option := range options {
		if strings.EqualFold(option, value) {
			return true
		}
	}
	return false
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {

}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	requestModelName := info.UpstreamModelName
	isImageRelay := info.RelayMode == constant.RelayModeImagesGenerations || info.RelayMode == constant.RelayModeImagesEdits
	if !isImageRelay && model_setting.GetGeminiSettings().ThinkingAdapterEnabled &&
		!model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) {
		// 新增逻辑：处理 -thinking-<budget> 格式
		if strings.Contains(requestModelName, "-thinking-") {
			parts := strings.Split(requestModelName, "-thinking-")
			requestModelName = parts[0]
		} else if strings.HasSuffix(requestModelName, "-thinking") { // 旧的适配
			requestModelName = strings.TrimSuffix(requestModelName, "-thinking")
		} else if strings.HasSuffix(requestModelName, "-nothinking") {
			requestModelName = strings.TrimSuffix(requestModelName, "-nothinking")
		} else if baseModel, level, ok := reasoning.TrimEffortSuffix(requestModelName); ok && level != "" {
			requestModelName = baseModel
		}
	}

	version := model_setting.GetGeminiVersionSetting(requestModelName)

	if strings.HasPrefix(requestModelName, "imagen") {
		return fmt.Sprintf("%s/%s/models/%s:predict", info.ChannelBaseUrl, version, requestModelName), nil
	}

	if strings.HasPrefix(requestModelName, "text-embedding") ||
		strings.HasPrefix(requestModelName, "embedding") ||
		strings.HasPrefix(requestModelName, "gemini-embedding") {
		action := "embedContent"
		if info.IsGeminiBatchEmbedding {
			action = "batchEmbedContents"
		}
		return fmt.Sprintf("%s/%s/models/%s:%s", info.ChannelBaseUrl, version, requestModelName, action), nil
	}

	action := "generateContent"
	if info.IsStream {
		action = "streamGenerateContent?alt=sse"
		if info.RelayMode == constant.RelayModeGemini {
			info.DisablePing = true
		}
	}
	return fmt.Sprintf("%s/%s/models/%s:%s", info.ChannelBaseUrl, version, requestModelName, action), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-goog-api-key", info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	result, err := relayconvert.ConvertRequest(c, info, types.RelayFormatGemini, request)
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	if request.Input == nil {
		return nil, errors.New("input is required")
	}

	inputs := request.ParseInput()
	if len(inputs) == 0 {
		return nil, errors.New("input is empty")
	}
	// We always build a batch-style payload with `requests`, so ensure we call the
	// batch endpoint upstream to avoid payload/endpoint mismatches.
	info.IsGeminiBatchEmbedding = true
	// process all inputs
	geminiRequests := make([]map[string]interface{}, 0, len(inputs))
	for _, input := range inputs {
		geminiRequest := map[string]interface{}{
			"model": fmt.Sprintf("models/%s", info.UpstreamModelName),
			"content": dto.GeminiChatContent{
				Parts: []dto.GeminiPart{
					{
						Text: input,
					},
				},
			},
		}

		// set specific parameters for different models
		// https://ai.google.dev/api/embeddings?hl=zh-cn#method:-models.embedcontent
		switch info.UpstreamModelName {
		case "text-embedding-004", "gemini-embedding-exp-03-07", "gemini-embedding-001":
			// Only newer models introduced after 2024 support OutputDimensionality
			dimensions := lo.FromPtrOr(request.Dimensions, 0)
			if dimensions > 0 {
				geminiRequest["outputDimensionality"] = dimensions
			}
		}
		geminiRequests = append(geminiRequests, geminiRequest)
	}

	return map[string]interface{}{
		"requests": geminiRequests,
	}, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	result, err := relayconvert.ConvertRequest(c, info, types.RelayFormatGemini, &request)
	if err != nil {
		return nil, err
	}
	geminiRequest, ok := result.Value.(*dto.GeminiChatRequest)
	if !ok {
		return nil, fmt.Errorf("expected Gemini generateContent request, got %T", result.Value)
	}
	return geminiRequest, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == constant.RelayModeResponses {
		if info.IsStream {
			return GeminiResponsesStreamHandler(c, info, resp)
		}
		return GeminiResponsesHandler(c, info, resp)
	}

	if info.RelayMode == constant.RelayModeGemini {
		if strings.Contains(info.RequestURLPath, ":embedContent") ||
			strings.Contains(info.RequestURLPath, ":batchEmbedContents") {
			return NativeGeminiEmbeddingHandler(c, resp, info)
		}
		if info.IsStream {
			return GeminiTextGenerationStreamHandler(c, info, resp)
		} else {
			return GeminiTextGenerationHandler(c, info, resp)
		}
	}

	if info.RelayMode == constant.RelayModeImagesGenerations || info.RelayMode == constant.RelayModeImagesEdits {
		if strings.HasPrefix(info.UpstreamModelName, "imagen") {
			return GeminiImageHandler(c, info, resp)
		}
		return GeminiNativeImageHandler(c, info, resp)
	}

	// check if the model is an embedding model
	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		return GeminiEmbeddingHandler(c, info, resp)
	}

	if info.IsStream {
		return GeminiChatStreamHandler(c, info, resp)
	} else {
		return GeminiChatHandler(c, info, resp)
	}

}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
