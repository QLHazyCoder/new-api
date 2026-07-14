package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRankingQuotaBucketsFollowNaturalDayOrigin(t *testing.T) {
	truncateTables(t)
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	dayOne := time.Date(2026, time.July, 13, 0, 0, 0, 0, location).Unix()
	dayTwo := time.Date(2026, time.July, 14, 0, 0, 0, 0, location).Unix()
	dayThree := time.Date(2026, time.July, 15, 0, 0, 0, 0, location).Unix()
	rows := []QuotaData{
		{ModelName: "model-a", CreatedAt: dayOne, TokenUsed: 10},
		{ModelName: "model-a", CreatedAt: dayOne + 23*3600, TokenUsed: 20},
		{ModelName: "model-a", CreatedAt: dayTwo, TokenUsed: 30},
		{ModelName: "model-b", CreatedAt: dayTwo + 3600, TokenUsed: 40},
		{ModelName: "excluded-at-end", CreatedAt: dayThree, TokenUsed: 999},
	}
	require.NoError(t, DB.Create(&rows).Error)

	buckets, err := GetRankingQuotaBuckets(dayOne, dayThree, 24*3600, dayOne)
	require.NoError(t, err)
	assert.ElementsMatch(t, []RankingQuotaBucket{
		{ModelName: "model-a", Bucket: dayOne, Tokens: 30},
		{ModelName: "model-a", Bucket: dayTwo, Tokens: 30},
		{ModelName: "model-b", Bucket: dayTwo, Tokens: 40},
	}, buckets)

	totals, err := GetRankingQuotaTotals(dayOne, dayTwo)
	require.NoError(t, err)
	assert.Equal(t, []RankingQuotaTotal{{ModelName: "model-a", TotalTokens: 30}}, totals)
}
