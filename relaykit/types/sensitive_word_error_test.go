package types

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSensitiveWordBlockErrorIsNonRetryableAndProtocolStable(t *testing.T) {
	const message = "你的请求因命中敏感词已被拦截，已记录 1 次；累计超过 5 次将立即封号，余额不退，如果有攻击破解别人网站等情节严重的情况将会直接报警。请勿使用当前分组进行违规对话；如有误判，请联系群主审核并清理你的记录。"

	err := NewOpenAIError(
		errors.New(message),
		ErrorCodeSensitiveWordsDetected,
		http.StatusForbidden,
		ErrOptionWithSkipRetry(),
		ErrOptionWithNoRecordErrorLog(),
	)

	require.Equal(t, http.StatusForbidden, err.StatusCode)
	require.Equal(t, ErrorCodeSensitiveWordsDetected, err.GetErrorCode())
	require.True(t, IsSkipRetryError(err))
	require.False(t, IsRecordErrorLog(err))

	openAI := err.ToOpenAIError()
	require.Equal(t, message, openAI.Message)
	require.Equal(t, string(ErrorCodeSensitiveWordsDetected), openAI.Type)
	require.Equal(t, ErrorCodeSensitiveWordsDetected, openAI.Code)

	claude := err.ToClaudeError()
	require.Equal(t, message, claude.Message)
	require.Equal(t, string(ErrorCodeSensitiveWordsDetected), claude.Type)
}
