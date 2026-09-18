package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenRequestAcceptsNumberAndDecimalStringQuota(t *testing.T) {
	for _, payload := range []string{
		`{"name":"large-number","remain_quota":50000000000}`,
		`{"name":"large-string","remain_quota":"50000000000"}`,
	} {
		var request tokenRequest
		require.NoError(t, json.Unmarshal([]byte(payload), &request))
		require.Equal(t, int64(50000000000), request.RemainQuota)
	}
}
