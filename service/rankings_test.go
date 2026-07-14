package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRankingTimeRangesUseConfiguredCalendarDays(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	previousLocation := time.Local
	time.Local = location
	t.Cleanup(func() { time.Local = previousLocation })

	now := time.Date(2026, time.July, 14, 10, 30, 15, 0, location)
	tests := []struct {
		period    string
		wantStart time.Time
	}{
		{period: "today", wantStart: time.Date(2026, time.July, 14, 0, 0, 0, 0, location)},
		{period: "week", wantStart: time.Date(2026, time.July, 8, 0, 0, 0, 0, location)},
		{period: "month", wantStart: time.Date(2026, time.June, 15, 0, 0, 0, 0, location)},
		{period: "year", wantStart: time.Date(2025, time.July, 15, 0, 0, 0, 0, location)},
	}

	for _, test := range tests {
		t.Run(test.period, func(t *testing.T) {
			config, err := rankingConfig(test.period)
			require.NoError(t, err)
			start, end := rankingTimeRange(config, now)
			assert.Equal(t, test.wantStart.Unix(), start)
			assert.Equal(t, now.Unix()+1, end)
		})
	}
}

func TestPreviousRankingRangeMatchesElapsedCalendarWindow(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	previousLocation := time.Local
	time.Local = location
	t.Cleanup(func() { time.Local = previousLocation })

	config, err := rankingConfig("week")
	require.NoError(t, err)
	currentStart := time.Date(2026, time.July, 8, 0, 0, 0, 0, location)
	currentEnd := time.Date(2026, time.July, 14, 10, 30, 16, 0, location)
	start, end := previousRankingTimeRange(config, currentStart.Unix(), currentEnd.Unix())

	assert.Equal(t, time.Date(2026, time.July, 1, 0, 0, 0, 0, location).Unix(), start)
	assert.Equal(t, time.Date(2026, time.July, 7, 10, 30, 16, 0, location).Unix(), end)
}
