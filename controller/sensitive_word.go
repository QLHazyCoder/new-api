package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetSensitiveWordPolicy(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": model.GetSensitiveWordPolicy()})
}

func UpdateSensitiveWordPolicy(c *gin.Context) {
	var policy model.SensitiveWordPolicy
	if err := c.ShouldBindJSON(&policy); err != nil {
		common.ApiError(c, err)
		return
	}
	previous := model.GetSensitiveWordPolicy()
	if err := model.SaveSensitiveWordPolicy(policy, c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "sensitive_word.policy_update", map[string]interface{}{
		"enabled":               previous.Enabled != policy.Enabled,
		"check_prompt":          policy.CheckPrompt,
		"retain_full_prompt":    policy.RetainFullPrompt,
		"ban_threshold":         policy.BanThreshold,
		"retention_days":        policy.FullPromptRetentionDays,
		"max_prompt_runes":      policy.MaxPromptRunes,
		"block_message_changed": previous.BlockMessage != policy.BlockMessage,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": model.GetSensitiveWordPolicy()})
}
func GetSensitiveWordGroups(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": model.ListSensitiveWordGroups()})
}
func GetSensitiveWordRules(c *gin.Context) {
	rules, err := model.ListSensitiveWordRules()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rules})
}

func GetSensitiveWordRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiError(c, fmt.Errorf("规则 ID 无效"))
		return
	}
	rule, err := model.GetSensitiveWordRuleDetail(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rule})
}

type sensitiveRuleRequest struct {
	Name   string   `json:"name"`
	Words  []string `json:"words"`
	Scope  string   `json:"scope"`
	Groups []string `json:"groups"`
	Mode   string   `json:"mode"`
}

func sensitiveRuleWords(req sensitiveRuleRequest) []string {
	return req.Words
}

func CreateSensitiveWordRule(c *gin.Context) {
	var req sensitiveRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "未命名规则"
	}
	mode := req.Mode
	if mode == "" {
		mode = model.SensitiveWordModeObserve
	}
	rule, err := model.UpsertSensitiveWordRuleWithMode(0, name, sensitiveRuleWords(req), req.Scope, req.Groups, c.GetInt("id"), mode)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "sensitive_word_rule.create", map[string]interface{}{"rule_id": rule.ID, "name": rule.Name, "scope": rule.Scope})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rule})
}
func UpdateSensitiveWordRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiError(c, fmt.Errorf("规则 ID 无效"))
		return
	}
	var req sensitiveRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	mode := req.Mode
	if mode == "" {
		mode = model.SensitiveWordModeObserve
	}
	rule, err := model.UpsertSensitiveWordRuleWithMode(id, req.Name, sensitiveRuleWords(req), req.Scope, req.Groups, c.GetInt("id"), mode)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "sensitive_word_rule.update", map[string]interface{}{"rule_id": id, "name": rule.Name, "scope": rule.Scope, "mode": rule.Mode})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rule})
}

func SetSensitiveWordRuleMode(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiError(c, fmt.Errorf("规则 ID 无效"))
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.SetSensitiveWordRuleMode(id, req.Mode); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "sensitive_word_rule.mode", map[string]interface{}{"rule_id": id, "mode": req.Mode})
	c.JSON(http.StatusOK, gin.H{"success": true})
}
func DeleteSensitiveWordRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiError(c, fmt.Errorf("规则 ID 无效"))
		return
	}
	if err := model.DeleteSensitiveWordRule(id); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "sensitive_word_rule.delete", map[string]interface{}{"rule_id": id})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func GetSensitiveWordAudit(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiError(c, fmt.Errorf("审计 ID 无效"))
		return
	}
	var event model.SensitiveWordAuditEvent
	if err := model.DB.First(&event, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": event})
}

// Keep the JSON shape stable for clients that want to import rules in bulk.
func DecodeSensitiveWordImport(raw string) ([]string, error) {
	var words []string
	err := json.Unmarshal([]byte(raw), &words)
	return words, err
}
