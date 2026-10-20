package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/tokenkit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesUsageAccumulatorTerminalAccounting(t *testing.T) {
	operation_setting.SetToolPriceForTest("responses_priced_fn", 5)
	t.Cleanup(func() { operation_setting.DeleteToolPriceForTest("responses_priced_fn") })
	for _, tc := range []struct {
		eventType  string
		wantImages int
	}{
		{eventType: "response.completed", wantImages: 1},
		{eventType: "response.done", wantImages: 1},
		{eventType: "response.incomplete", wantImages: 1},
		{eventType: "response.failed", wantImages: 1},
		{eventType: "response.cancelled", wantImages: 1},
		{eventType: "response.canceled", wantImages: 1},
	} {
		t.Run(tc.eventType, func(t *testing.T) {
			info := &relaycommon.RelayInfo{OriginModelName: "gpt-5.1", StreamStatus: relaycommon.NewStreamStatus()}
			accumulator := NewResponsesUsageAccumulator(info)
			for _, item := range []dto.ResponsesOutput{
				{Type: dto.BuildInCallWebSearchCall},
				{Type: dto.BuildInCallFileSearchCall},
				{Type: dto.BuildInCallFunctionCall, Name: "responses_priced_fn"},
				{Type: dto.BuildInCallFunctionCall, Name: "responses_unpriced_fn"},
			} {
				accumulator.Observe(&dto.ResponsesStreamResponse{Type: dto.ResponsesOutputTypeItemDone, Item: &item}, nil)
			}
			image := dto.ResponsesOutput{ID: "image-1", Type: dto.ResponsesOutputTypeImageGenerationCall, Status: "completed", Result: "image-data"}
			accumulator.Observe(&dto.ResponsesStreamResponse{Type: dto.ResponsesOutputTypeItemDone, Item: &image}, nil)
			upstream := &dto.Usage{InputTokens: 20, OutputTokens: 5, TotalTokens: 25, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 4}}
			upstream.BillingUsage = dto.NewOpenAIResponsesBillingUsage(upstream)
			terminal := &dto.ResponsesStreamResponse{
				Type: tc.eventType,
				Response: &dto.OpenAIResponsesResponse{
					Usage: upstream, Output: []dto.ResponsesOutput{image},
				},
			}
			accumulator.Observe(terminal, nil)
			accumulator.Observe(terminal, nil)
			usage := accumulator.Finish()
			assert.Equal(t, tc.eventType == "response.failed", info.StreamStatus.ResponseFailed())
			assert.NotEmpty(t, info.StreamStatus.ResponseOutcome())

			assert.Equal(t, 20, usage.PromptTokens)
			assert.Equal(t, 5, usage.CompletionTokens)
			assert.Equal(t, 25, usage.TotalTokens)
			assert.Equal(t, 4, usage.PromptTokensDetails.CachedTokens)
			require.NotNil(t, usage.BillingUsage)
			assert.Equal(t, upstream.BillingUsage, usage.BillingUsage)
			assert.NotSame(t, upstream.BillingUsage, usage.BillingUsage)
			tools := info.ResponsesUsageInfo.BuiltInTools
			for _, name := range []string{dto.BuildInToolWebSearchPreview, dto.BuildInToolFileSearch, "responses_priced_fn"} {
				require.Contains(t, tools, name)
				assert.Equal(t, 1, tools[name].CallCount)
			}
			assert.NotContains(t, tools, "responses_unpriced_fn")
			require.Contains(t, tools, dto.BuildInToolImageGeneration)
			assert.Equal(t, tc.wantImages, tools[dto.BuildInToolImageGeneration].CallCount)
		})
	}
}

func TestResponsesUsageAccumulatorInterruptedTextFallback(t *testing.T) {
	for _, withUsage := range []bool{false, true} {
		name := "disconnect without terminal usage"
		if withUsage {
			name = "failed response preserves native billing usage"
		}
		t.Run(name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4o"}}
			info.SetEstimatePromptTokens(100)
			accumulator := NewResponsesUsageAccumulator(info)
			accumulator.Observe(&dto.ResponsesStreamResponse{Type: "response.output_text.delta", Delta: "hello"}, nil)
			if withUsage {
				upstream := &dto.Usage{InputTokens: 20, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 4}}
				upstream.BillingUsage = dto.NewOpenAIResponsesBillingUsage(upstream)
				accumulator.Observe(&dto.ResponsesStreamResponse{Type: "response.failed", Response: &dto.OpenAIResponsesResponse{Usage: upstream}}, nil)
			}
			usage := accumulator.Finish()
			wantCompletion := 1
			if withUsage {
				wantCompletion = 0
				assert.Zero(t, usage.CompletionTokens)
				assert.Equal(t, 20, usage.PromptTokens)
				assert.Equal(t, 20, usage.TotalTokens)
				require.NotNil(t, usage.BillingUsage)
				assert.Equal(t, dto.BillingUsageSourceOAIResponses, usage.BillingUsage.Source)
				assert.False(t, usage.BillingUsage.Estimated)
				canonical, ok := usage.BillingUsage.CanonicalUsage()
				require.True(t, ok)
				assert.Equal(t, 4, canonical.PromptTokensDetails.CachedTokens)
				assert.Zero(t, canonical.CompletionTokens)
			} else {
				assert.Equal(t, 1, usage.CompletionTokens)
				assert.Equal(t, 100, usage.PromptTokens)
				assert.Equal(t, 101, usage.TotalTokens)
			}
			accumulator.Observe(&dto.ResponsesStreamResponse{Type: "response.output_text.delta", Delta: " late output"}, nil)
			assert.Same(t, usage, accumulator.Finish())
			assert.Equal(t, wantCompletion, usage.CompletionTokens)
			assert.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
		})
	}
}

func TestResponsesUsageAccumulatorExplicitFailureDoesNotEstimate(t *testing.T) {
	for _, tc := range []struct {
		name           string
		event          string
		deltaType      string
		delta          string
		wantPrompt     int
		wantCompletion int
		wantCache      int
	}{
		{name: "failed without usage", event: `{"type":"response.failed","response":{"status":"failed"}}`, delta: " "},
		{name: "failed after text", event: `{"type":"response.failed","response":{"status":"failed"}}`, delta: "hello"},
		{name: "failed with null usage", event: `{"type":"response.failed","response":{"usage":null}}`, delta: "hello"},
		{name: "failed with zero usage", event: `{"type":"response.failed","response":{"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`, delta: "hello"},
		{name: "failed with input only", event: `{"type":"response.failed","response":{"usage":{"input_tokens":20,"output_tokens":0,"input_tokens_details":{"cached_tokens":4}}}}`, delta: "hello", wantPrompt: 20, wantCache: 4},
		{name: "failed with output only", event: `{"type":"response.failed","response":{"usage":{"input_tokens":0,"output_tokens":5}}}`, delta: "hello", wantCompletion: 5},
		{name: "failed with complete usage", event: `{"type":"response.failed","response":{"usage":{"input_tokens":20,"output_tokens":5,"input_tokens_details":{"cached_tokens":4}}}}`, delta: "hello", wantPrompt: 20, wantCompletion: 5, wantCache: 4},
		{name: "flat error", event: `{"type":"error","code":"server_error"}`, delta: "hello"},
		{name: "response error", event: `{"type":"response.error","code":"server_error"}`, delta: "hello"},
		{name: "response error with usage", event: `{"type":"response.error","response":{"usage":{"input_tokens":20,"output_tokens":5,"input_tokens_details":{"cached_tokens":4}}}}`, delta: "hello", wantPrompt: 20, wantCompletion: 5, wantCache: 4},
		{name: "flat error with response usage", event: `{"type":"error","response":{"usage":{"input_tokens":20,"output_tokens":5}}}`, delta: "hello", wantPrompt: 20, wantCompletion: 5},
		{name: "done with failed status", event: `{"type":"response.done","response":{"status":"failed"}}`, delta: "hello"},
		{name: "failed status with usage", event: `{"type":"response.in_progress","response":{"status":"failed","usage":{"input_tokens":20,"output_tokens":5}}}`, delta: "hello", wantPrompt: 20, wantCompletion: 5},
		{name: "failed after reasoning", event: `{"type":"response.failed"}`, deltaType: "response.reasoning_text.delta", delta: "Inspect the request"},
		{name: "failed after function arguments", event: `{"type":"response.failed"}`, deltaType: "response.function_call_arguments.delta", delta: `{"command":"ls"}`},
		{name: "failed after refusal", event: `{"type":"response.failed"}`, deltaType: "response.refusal.delta", delta: "I cannot help with that request"},
		{name: "failed with terminal output only", event: `{"type":"response.failed","response":{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}]}}`},
	} {
		for _, withStatus := range []bool{false, true} {
			statusName := "without stream status"
			if withStatus {
				statusName = "with stream status"
			}
			t.Run(tc.name+"/"+statusName, func(t *testing.T) {
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4o"}}
				info.SetEstimatePromptTokens(80505)
				if withStatus {
					info.StreamStatus = relaycommon.NewStreamStatus()
				}
				accumulator := NewResponsesUsageAccumulator(info)
				accumulator.Observe(&dto.ResponsesStreamResponse{Type: "response.created"}, nil)
				if tc.delta != "" {
					deltaType := tc.deltaType
					if deltaType == "" {
						deltaType = "response.output_text.delta"
					}
					accumulator.Observe(&dto.ResponsesStreamResponse{Type: deltaType, Delta: tc.delta}, nil)
				}
				var event dto.ResponsesStreamResponse
				require.NoError(t, common.UnmarshalJsonStr(tc.event, &event))
				accumulator.Observe(&event, common.StringToByteSlice(tc.event))
				usage := accumulator.Finish()
				require.NotNil(t, usage)
				assert.Equal(t, tc.wantPrompt, usage.PromptTokens)
				assert.Equal(t, tc.wantCompletion, usage.CompletionTokens)
				assert.Equal(t, tc.wantPrompt+tc.wantCompletion, usage.TotalTokens)
				assert.Equal(t, tc.wantCache, usage.PromptTokensDetails.CachedTokens)
				if withStatus {
					assert.True(t, info.StreamStatus.ResponseFailed())
				}
				accumulator.Observe(&dto.ResponsesStreamResponse{Type: "response.output_text.delta", Delta: " late output"}, nil)
				assert.Same(t, usage, accumulator.Finish())
				assert.Equal(t, tc.wantPrompt+tc.wantCompletion, usage.TotalTokens)
			})
		}
	}
}

func TestResponsesUsageAccumulatorFailedTerminalRetainsReportedSnapshot(t *testing.T) {
	for _, finalUsage := range []*dto.Usage{nil, {}} {
		name := "missing final usage"
		if finalUsage != nil {
			name = "zero final usage"
		}
		t.Run(name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{StreamStatus: relaycommon.NewStreamStatus()}
			info.SetEstimatePromptTokens(80505)
			accumulator := NewResponsesUsageAccumulator(info)
			reported := &dto.Usage{InputTokens: 20, OutputTokens: 5, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 4}}
			reported.BillingUsage = dto.NewOpenAIResponsesBillingUsage(reported)
			accumulator.Observe(&dto.ResponsesStreamResponse{Type: "response.error", Response: &dto.OpenAIResponsesResponse{Usage: reported}}, nil)
			accumulator.Observe(&dto.ResponsesStreamResponse{Type: "response.failed", Response: &dto.OpenAIResponsesResponse{Usage: finalUsage}}, nil)
			usage := accumulator.Finish()
			assert.Equal(t, 20, usage.PromptTokens)
			assert.Equal(t, 5, usage.CompletionTokens)
			assert.Equal(t, 25, usage.TotalTokens)
			assert.Equal(t, 4, usage.PromptTokensDetails.CachedTokens)
			require.NotNil(t, usage.BillingUsage)
			assert.Equal(t, reported.BillingUsage, usage.BillingUsage)
			assert.False(t, usage.BillingUsage.Estimated)
		})
	}
}

func TestResponsesUsageAccumulatorErrorBeforeTerminalKeepsCompletedTools(t *testing.T) {
	operation_setting.SetToolPriceForTest("failed_response_priced_fn", 5)
	t.Cleanup(func() { operation_setting.DeleteToolPriceForTest("failed_response_priced_fn") })
	info := &relaycommon.RelayInfo{StreamStatus: relaycommon.NewStreamStatus()}
	info.SetEstimatePromptTokens(80505)
	accumulator := NewResponsesUsageAccumulator(info)
	accumulator.Observe(&dto.ResponsesStreamResponse{
		Type: dto.ResponsesOutputTypeItemDone,
		Item: &dto.ResponsesOutput{Type: dto.BuildInCallFunctionCall, Name: "failed_response_priced_fn"},
	}, nil)
	accumulator.Observe(&dto.ResponsesStreamResponse{Type: "error", Code: "server_error"}, nil)
	terminal := &dto.ResponsesStreamResponse{
		Type: "response.failed",
		Response: &dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{
			{ID: "complete-image", Type: dto.ResponsesOutputTypeImageGenerationCall, Status: "completed", Result: "final-image"},
			{ID: "partial-image", Type: dto.ResponsesOutputTypeImageGenerationCall, Status: "partial", Result: "partial-image"},
		}},
	}
	accumulator.Observe(terminal, nil)
	accumulator.Observe(terminal, nil)
	usage := accumulator.Finish()
	assert.Zero(t, usage.TotalTokens)
	require.NotNil(t, info.ResponsesUsageInfo)
	tools := info.ResponsesUsageInfo.BuiltInTools
	require.Contains(t, tools, "failed_response_priced_fn")
	require.Contains(t, tools, dto.BuildInToolImageGeneration)
	assert.Equal(t, 1, tools["failed_response_priced_fn"].CallCount)
	assert.Equal(t, 1, tools[dto.BuildInToolImageGeneration].CallCount)
	assert.Same(t, usage, accumulator.Finish())
	assert.Equal(t, 1, tools[dto.BuildInToolImageGeneration].CallCount)
}

func TestObserveResponsesOutcomeRecordsProtocolFacts(t *testing.T) {
	for _, tc := range []struct {
		name        string
		event       string
		wantOutcome relaycommon.ResponseOutcome
		wantCode    string
		wantType    string
		wantIncompl string
	}{
		{"flat sse error", `{"type":"error","code":"context_length_exceeded","message":"too long"}`, relaycommon.ResponseOutcomeFailed, "context_length_exceeded", "", ""},
		{"done with failed status", `{"type":"response.done","response":{"status":"failed","error":{"code":"invalid_api_key","type":"invalid_request_error","message":"bad key"}}}`, relaycommon.ResponseOutcomeFailed, "invalid_api_key", "invalid_request_error", ""},
		{"incomplete keeps reason", `{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`, relaycommon.ResponseOutcomeIncomplete, "", "", "max_output_tokens"},
		{"in progress is not terminal", `{"type":"response.created","response":{"status":"in_progress"}}`, relaycommon.ResponseOutcomeUnknown, "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var event dto.ResponsesStreamResponse
			require.NoError(t, common.UnmarshalJsonStr(tc.event, &event))
			info := &relaycommon.RelayInfo{StreamStatus: relaycommon.NewStreamStatus()}
			ObserveResponsesOutcome(info, &event)
			outcome := info.StreamStatus.OutcomeSnapshot()
			assert.Equal(t, tc.wantOutcome, outcome.Response)
			assert.Equal(t, tc.wantCode, outcome.ErrorCode)
			assert.Equal(t, tc.wantType, outcome.ErrorType)
			assert.Equal(t, tc.wantIncompl, outcome.IncompleteReason)
		})
	}
}

func TestResponsesUsageAccumulatorDisconnectBillsCompletedImage(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: "gpt-5.1", StreamStatus: relaycommon.NewStreamStatus()}
	accumulator := NewResponsesUsageAccumulator(info)
	accumulator.Observe(&dto.ResponsesStreamResponse{
		Type: dto.ResponsesOutputTypeItemDone,
		Item: &dto.ResponsesOutput{ID: "complete-image", Type: dto.ResponsesOutputTypeImageGenerationCall, Status: "completed", Result: "final-image-data"},
	}, nil)
	accumulator.Observe(&dto.ResponsesStreamResponse{
		Type: dto.ResponsesOutputTypeItemDone,
		Item: &dto.ResponsesOutput{ID: "partial-image", Type: dto.ResponsesOutputTypeImageGenerationCall, Status: "partial", Result: "partial-image-data"},
	}, nil)
	usage := accumulator.Finish()
	assert.Zero(t, usage.TotalTokens)
	require.Contains(t, info.ResponsesUsageInfo.BuiltInTools, dto.BuildInToolImageGeneration)
	assert.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolImageGeneration].CallCount)
	accumulator.Finish()
	assert.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolImageGeneration].CallCount)
}

func TestApplyResponsesUsageCopiesTokenDetails(t *testing.T) {
	dst := &dto.Usage{}
	src := &dto.Usage{
		InputTokens:  11,
		OutputTokens: 7,
		TotalTokens:  18,
		InputTokensDetails: &dto.InputTokenDetails{
			CachedTokens:         3,
			CachedCreationTokens: 2,
			TextTokens:           6,
			AudioTokens:          4,
			ImageTokens:          5,
		},
		OutputTokensDetails: &dto.OutputTokenDetails{
			TextTokens:      1,
			AudioTokens:     2,
			ImageTokens:     3,
			ReasoningTokens: 4,
		},
		PromptCacheHitTokens: 3,
		UsageSemantic:        "openai",
		UsageSource:          "upstream",
	}

	ApplyResponsesUsage(dst, src)

	assert.Equal(t, 11, dst.PromptTokens)
	assert.Equal(t, 7, dst.CompletionTokens)
	assert.Equal(t, 18, dst.TotalTokens)
	require.NotNil(t, dst.InputTokensDetails)
	assert.Equal(t, *src.InputTokensDetails, dst.PromptTokensDetails)
	assert.Equal(t, src.InputTokensDetails, dst.InputTokensDetails)
	assert.NotSame(t, src.InputTokensDetails, dst.InputTokensDetails)
	assert.Equal(t, *src.OutputTokensDetails, dst.CompletionTokenDetails)
	require.NotNil(t, dst.OutputTokensDetails)
	assert.Equal(t, *src.OutputTokensDetails, *dst.OutputTokensDetails)
	assert.NotSame(t, src.OutputTokensDetails, dst.OutputTokensDetails)
	assert.Equal(t, 3, dst.PromptCacheHitTokens)
	assert.Equal(t, "openai", dst.UsageSemantic)
	assert.Equal(t, "upstream", dst.UsageSource)
}

func TestApplyResponsesUsageFallsBackToCompletionTokenDetails(t *testing.T) {
	dst := &dto.Usage{}
	src := &dto.Usage{
		CompletionTokenDetails: dto.OutputTokenDetails{
			ReasoningTokens: 9,
		},
	}

	ApplyResponsesUsage(dst, src)

	assert.Equal(t, 9, dst.CompletionTokenDetails.ReasoningTokens)
	require.NotNil(t, dst.OutputTokensDetails)
	assert.Equal(t, 9, dst.OutputTokensDetails.ReasoningTokens)
}

func TestApplyResponsesUsagePreservesBillingSnapshotAcrossPartialUpdates(t *testing.T) {
	dst := &dto.Usage{}
	src := &dto.Usage{
		InputTokens:  20,
		OutputTokens: 10,
		TotalTokens:  30,
		InputTokensDetails: &dto.InputTokenDetails{
			CachedTokens: 3,
			AudioTokens:  2,
		},
		CompletionTokenDetails: dto.OutputTokenDetails{AudioTokens: 4},
		UsageSemantic:          "openai",
		UsageSource:            "upstream",
	}
	src.BillingUsage = dto.NewOpenAIResponsesBillingUsage(src)
	ApplyResponsesUsage(dst, src)
	ApplyResponsesUsage(dst, &dto.Usage{
		OutputTokens:        12,
		InputTokensDetails:  &dto.InputTokenDetails{},
		OutputTokensDetails: &dto.OutputTokenDetails{ReasoningTokens: 5},
	})

	assert.Equal(t, 20, dst.PromptTokens)
	assert.Equal(t, 12, dst.CompletionTokens)
	assert.Equal(t, 32, dst.TotalTokens)
	assert.Equal(t, 3, dst.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 2, dst.PromptTokensDetails.AudioTokens)
	assert.Equal(t, 4, dst.CompletionTokenDetails.AudioTokens)
	assert.Equal(t, 5, dst.CompletionTokenDetails.ReasoningTokens)
	require.NotNil(t, dst.OutputTokensDetails)
	assert.Equal(t, dst.CompletionTokenDetails, *dst.OutputTokensDetails)
	assert.Equal(t, "openai", dst.UsageSemantic)
	assert.Equal(t, "upstream", dst.UsageSource)
	require.NotNil(t, dst.BillingUsage)
	assert.Equal(t, src.BillingUsage, dst.BillingUsage)
	assert.NotSame(t, src.BillingUsage, dst.BillingUsage)
}

func TestResponsesUsageAccumulatorMissingUsageEstimation(t *testing.T) {
	const model = "gpt-4o"
	const summary = "Inspect the repository before editing."
	const arguments = `{"command":["bash","-lc","ls"]}`
	inProgress := &dto.OpenAIResponsesResponse{Status: []byte(`"in_progress"`)}
	for _, tc := range []struct {
		name           string
		events         []dto.ResponsesStreamResponse
		wantPrompt     int
		wantCompletion int
	}{
		{
			name: "tool call stream cut before terminal usage",
			events: []dto.ResponsesStreamResponse{
				{Type: "response.created", Response: inProgress},
				{Type: "response.reasoning_summary_text.delta", Delta: summary},
				{Type: "response.function_call_arguments.delta", Delta: arguments},
				{Type: dto.ResponsesOutputTypeItemDone, Item: &dto.ResponsesOutput{Type: dto.BuildInCallFunctionCall, Name: "shell"}},
			},
			wantPrompt:     100,
			wantCompletion: tokenkit.Count(model, summary+arguments),
		},
		{
			name: "created only then disconnect bills the prompt",
			events: []dto.ResponsesStreamResponse{
				{Type: "response.created", Response: inProgress},
			},
			wantPrompt: 100,
		},
		{
			name: "incomplete without usage bills the prompt",
			events: []dto.ResponsesStreamResponse{
				{Type: "response.created", Response: inProgress},
				{Type: "response.incomplete", Response: &dto.OpenAIResponsesResponse{Status: []byte(`"incomplete"`)}},
			},
			wantPrompt: 100,
		},
		{
			name: "completed without usage estimates from terminal output",
			events: []dto.ResponsesStreamResponse{
				{Type: "response.completed", Response: &dto.OpenAIResponsesResponse{
					Status: []byte(`"completed"`),
					Output: []dto.ResponsesOutput{{
						Type:    "message",
						Role:    "assistant",
						Content: []dto.ResponsesOutputContent{{Type: "output_text", Text: "final answer"}},
					}},
				}},
			},
			wantPrompt:     100,
			wantCompletion: tokenkit.Count(model, "final answer"),
		},
		{
			name: "explicit failure without usage bills nothing",
			events: []dto.ResponsesStreamResponse{
				{Type: "response.created", Response: inProgress},
				{Type: "response.failed", Response: &dto.OpenAIResponsesResponse{Status: []byte(`"failed"`)}},
			},
		},
		{
			name: "flat error event bills nothing",
			events: []dto.ResponsesStreamResponse{
				{Type: "response.created", Response: inProgress},
				{Type: "error", Code: "server_error"},
			},
		},
		{
			name: "no upstream events bills nothing",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				ChannelMeta:  &relaycommon.ChannelMeta{UpstreamModelName: model},
				StreamStatus: relaycommon.NewStreamStatus(),
			}
			info.SetEstimatePromptTokens(100)
			accumulator := NewResponsesUsageAccumulator(info)
			for i := range tc.events {
				accumulator.Observe(&tc.events[i], nil)
			}
			usage := accumulator.Finish()
			assert.Equal(t, tc.wantPrompt, usage.PromptTokens)
			assert.Equal(t, tc.wantCompletion, usage.CompletionTokens)
			assert.Equal(t, tc.wantPrompt+tc.wantCompletion, usage.TotalTokens)
		})
	}
}
