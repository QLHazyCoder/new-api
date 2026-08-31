package middleware

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func abortWithOpenAiMessage(c *gin.Context, statusCode int, message string, code ...types.ErrorCode) {
	codeStr := ""
	if len(code) > 0 {
		codeStr = string(code[0])
	}
	isSensitiveWord := len(code) > 0 && code[0] == types.ErrorCodeSensitiveWordsDetected
	userId := c.GetInt("id")
	displayMessage := message
	if !isSensitiveWord {
		displayMessage = common.MessageWithRequestId(message, c.GetString(common.RequestIdKey))
	}
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": displayMessage,
			"type":    "new_api_error",
			"code":    codeStr,
		},
	})
	c.Abort()
	if isSensitiveWord {
		logger.LogInfo(c, fmt.Sprintf("user %d | sensitive word auto-ban response", userId))
		return
	}
	logger.LogError(c.Request.Context(), fmt.Sprintf("user %d | %s", userId, message))
}

func abortWithMidjourneyMessage(c *gin.Context, statusCode int, code int, description string) {
	c.JSON(statusCode, gin.H{
		"description": description,
		"type":        "new_api_error",
		"code":        code,
	})
	c.Abort()
	logger.LogError(c.Request.Context(), description)
}
