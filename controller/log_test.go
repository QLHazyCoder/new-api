package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateLogRetentionDaysOption(t *testing.T) {
	for _, value := range []string{"0", "30", " 30 ", "3650"} {
		require.NoError(t, validateLogRetentionDaysOption(value))
	}

	for _, value := range []string{"-1", "3651", "1.5", "abc"} {
		require.Error(t, validateLogRetentionDaysOption(value))
	}
}
