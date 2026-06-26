package perfmetrics

import (
	"testing"

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
