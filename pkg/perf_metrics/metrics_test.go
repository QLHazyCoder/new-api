package perfmetrics

import (
	"net/http"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestBuildQueryResultIncludesCounters(t *testing.T) {
	result := buildQueryResult("gpt-test", map[bucketKey]counters{
		{model: "gpt-test", group: "default", bucketTs: 1000}: {
			requestCount:   10,
			successCount:   7,
			totalLatencyMs: 2000,
			ttftSumMs:      800,
			ttftCount:      4,
			outputTokens:   100,
			generationMs:   1000,
		},
		{model: "gpt-test", group: "default", bucketTs: 1060}: {
			requestCount:   5,
			successCount:   5,
			totalLatencyMs: 1500,
			ttftSumMs:      600,
			ttftCount:      3,
			outputTokens:   75,
			generationMs:   1500,
		},
	})

	require.Len(t, result.Groups, 1)

	group := result.Groups[0]
	require.Equal(t, int64(15), group.RequestCount)
	require.Equal(t, int64(12), group.SuccessCount)
	require.Equal(t, 80.0, group.SuccessRate)
	require.Len(t, group.Series, 2)
	require.Equal(t, int64(10), group.Series[0].RequestCount)
	require.Equal(t, int64(7), group.Series[0].SuccessCount)
	require.Equal(t, int64(5), group.Series[1].RequestCount)
	require.Equal(t, int64(5), group.Series[1].SuccessCount)
}

func TestAtomicBucketFailedSampleDoesNotIncreaseSuccessCount(t *testing.T) {
	bucket := &atomicBucket{}
	bucket.add(Sample{Success: true})
	bucket.add(Sample{Success: false})

	snapshot := bucket.snapshot()
	require.Equal(t, int64(2), snapshot.requestCount)
	require.Equal(t, int64(1), snapshot.successCount)
}

func TestShouldRecordRelayFailureExcludesOnlyRawImageUpstreamBadRequest(t *testing.T) {
	tests := []struct {
		name       string
		format     types.RelayFormat
		playground bool
		rawStatus  int
		want       bool
	}{
		{
			name:      "regular image upstream 400",
			format:    types.RelayFormatOpenAIImage,
			rawStatus: http.StatusBadRequest,
			want:      false,
		},
		{
			name:       "playground image upstream 400",
			format:     types.RelayFormatOpenAIImage,
			playground: true,
			rawStatus:  http.StatusBadRequest,
			want:       false,
		},
		{
			name:      "image upstream 500",
			format:    types.RelayFormatOpenAIImage,
			rawStatus: http.StatusInternalServerError,
			want:      true,
		},
		{
			name:      "image upstream 429",
			format:    types.RelayFormatOpenAIImage,
			rawStatus: http.StatusTooManyRequests,
			want:      true,
		},
		{
			name:      "text upstream 400",
			format:    types.RelayFormatOpenAI,
			rawStatus: http.StatusBadRequest,
			want:      true,
		},
		{
			name:      "local return error 400",
			format:    types.RelayFormatOpenAIImage,
			rawStatus: 0,
			want:      true,
		},
		{
			name:      "parameter override 400",
			format:    types.RelayFormatOpenAIImage,
			rawStatus: 0,
			want:      true,
		},
		{
			name:   "response mapping to 400",
			format: types.RelayFormatOpenAIImage,
			// A mapped 400 retains the original non-400 upstream status.
			rawStatus: http.StatusInternalServerError,
			want:      true,
		},
		{
			name:   "response body conversion to 400",
			format: types.RelayFormatOpenAIImage,
			// A 400 returned by local response parsing is not an upstream 400.
			rawStatus: http.StatusOK,
			want:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				RelayFormat:                test.format,
				IsPlayground:               test.playground,
				LastUpstreamHTTPStatusCode: test.rawStatus,
			}
			require.Equal(t, test.want, ShouldRecordRelayFailure(info))
		})
	}
}

func TestShouldRecordRelayFailureUsesTheFinalImageAttemptStatus(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayFormat:                types.RelayFormatOpenAIImage,
		LastUpstreamHTTPStatusCode: http.StatusBadRequest,
	}

	// ImageHelper clears this request-scoped field at the start of a retry.
	require.False(t, ShouldRecordRelayFailure(info))
	info.LastUpstreamHTTPStatusCode = 0
	require.True(t, ShouldRecordRelayFailure(info))
}
