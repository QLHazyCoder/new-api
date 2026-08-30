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

func GetSensitiveWordConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": model.GetSensitiveWordConfig()})
}
func UpdateSensitiveWordConfig(c *gin.Context) {
	var cfg model.SensitiveWordConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		common.ApiError(c, err)
		return
	}
	previous := model.GetSensitiveWordConfig()
	if err := model.SaveSensitiveWordConfig(cfg); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "sensitive_word.config_update", map[string]interface{}{
		"enabled":               cfg.Enabled,
		"check_prompt":          cfg.CheckPrompt,
		"mode":                  cfg.Mode,
		"audit_enabled":         cfg.AuditEnabled,
		"ban_threshold":         cfg.BanThreshold,
		"retention_days":        cfg.FullPromptRetentionDays,
		"max_prompt_runes":      cfg.MaxPromptRunes,
		"block_message_changed": previous.BlockMessage != cfg.BlockMessage,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": model.GetSensitiveWordConfig()})
}
func GetSensitiveWordGroups(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": model.ListSensitiveWordGroups()})
}
func GetSensitiveWordStats(c *gin.Context) {
	stats, err := model.GetSensitiveWordStats()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
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
	Name    string   `json:"name"`
	Word    string   `json:"word"` // legacy single-word payload
	Words   []string `json:"words"`
	Scope   string   `json:"scope"`
	Groups  []string `json:"groups"`
	Enabled *bool    `json:"enabled"`
}

func sensitiveRuleWords(req sensitiveRuleRequest) []string {
	if len(req.Words) > 0 {
		return req.Words
	}
	return strings.Split(req.Word, "\n")
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
	rule, err := model.UpsertSensitiveWordRule(0, name, sensitiveRuleWords(req), req.Scope, req.Groups, c.GetInt("id"), req.Enabled)
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
	rule, err := model.UpsertSensitiveWordRule(id, req.Name, sensitiveRuleWords(req), req.Scope, req.Groups, c.GetInt("id"), req.Enabled)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "sensitive_word_rule.update", map[string]interface{}{"rule_id": id, "name": rule.Name, "scope": rule.Scope, "enabled": rule.Enabled})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rule})
}

func SetSensitiveWordRuleStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiError(c, fmt.Errorf("规则 ID 无效"))
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.SetSensitiveWordRuleEnabled(id, req.Enabled); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "sensitive_word_rule.status", map[string]interface{}{"rule_id": id, "enabled": req.Enabled})
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

type sensitiveWhitelistRequest struct {
	UserID  int    `json:"user_id"`
	Enabled bool   `json:"enabled"`
	Remark  string `json:"remark"`
}

func GetSensitiveWordWhitelist(c *gin.Context) {
	items, err := model.ListSensitiveWordWhitelist()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}
func CreateSensitiveWordWhitelist(c *gin.Context) {
	var req sensitiveWhitelistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	before := model.IsSensitiveWordWhitelisted(req.UserID)
	item, err := model.UpsertSensitiveWordWhitelist(req.UserID, c.GetInt("id"), req.Enabled, req.Remark)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, req.UserID, "sensitive_word.whitelist_update", map[string]interface{}{"target_user_id": req.UserID, "before": before, "after": req.Enabled})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": item})
}
func DeleteSensitiveWordWhitelist(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || id <= 0 {
		common.ApiError(c, fmt.Errorf("用户 ID 无效"))
		return
	}
	before := model.IsSensitiveWordWhitelisted(id)
	if err := model.DeleteSensitiveWordWhitelist(id); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, id, "sensitive_word.whitelist_update", map[string]interface{}{"target_user_id": id, "before": before, "after": false})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func GetSensitiveWordAudits(c *gin.Context) {
	page := 1
	size := 20
	if v, err := strconv.Atoi(c.Query("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(c.Query("page_size")); err == nil && v > 0 && v <= 100 {
		size = v
	}
	var events []model.SensitiveWordAuditEvent
	var total int64
	tx := model.DB.Model(&model.SensitiveWordAuditEvent{})
	if uid, err := strconv.Atoi(c.Query("user_id")); err == nil && uid > 0 {
		tx = tx.Where("user_id = ?", uid)
	}
	if group := c.Query("group"); group != "" {
		tx = tx.Where("group_name = ?", group)
	}
	if keyword := c.Query("keyword"); keyword != "" {
		tx = tx.Where("matched_words LIKE ?", "%"+keyword+"%")
	}
	if requestID := c.Query("request_id"); requestID != "" {
		tx = tx.Where("request_id = ?", requestID)
	}
	if blocked := c.Query("blocked"); blocked == "true" || blocked == "false" {
		tx = tx.Where("blocked = ?", blocked == "true")
	}
	if err := tx.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := tx.Order("created_at desc, id desc").Limit(size).Offset((page - 1) * size).Find(&events).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	for i := range events {
		events[i].FullPrompt = ""
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": events, "total": total, "page": page, "page_size": size}})
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
func ClearSensitiveWordViolations(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, fmt.Errorf("用户 ID 无效"))
		return
	}
	if err := model.ClearSensitiveWordViolations(id); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, id, "sensitive_word_clear_violations", map[string]interface{}{"user_id": id})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func sensitiveWordEnableResetAuditParams(result *model.SensitiveWordEnableResetResult, source string) map[string]interface{} {
	return map[string]interface{}{
		"target_user_id":         result.UserID,
		"status_before":          result.StatusBefore,
		"status_after":           result.StatusAfter,
		"before":                 result.ViolationCountBefore,
		"after":                  result.ViolationCountAfter,
		"violation_count_before": result.ViolationCountBefore,
		"violation_count_after":  result.ViolationCountAfter,
		"auth_version_before":    result.AuthVersionBefore,
		"auth_version_after":     result.AuthVersionAfter,
		"quota_before":           result.QuotaBefore,
		"quota_after":            result.QuotaAfter,
		"used_quota_before":      result.UsedQuotaBefore,
		"used_quota_after":       result.UsedQuotaAfter,
		"status_changed":         result.StatusChanged,
		"sessions_revoked":       result.SessionsRevoked,
		"balance_changed":        false,
		"source":                 source,
	}
}

func UnbanSensitiveWordUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, fmt.Errorf("用户 ID 无效"))
		return
	}
	result, err := model.EnableUserAndResetSensitiveWordViolations(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, id, "sensitive_word.enable_reset", sensitiveWordEnableResetAuditParams(result, "sensitive_word_unban"))
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Keep the JSON shape stable for clients that want to import rules in bulk.
func DecodeSensitiveWordImport(raw string) ([]string, error) {
	var words []string
	err := json.Unmarshal([]byte(raw), &words)
	return words, err
}
