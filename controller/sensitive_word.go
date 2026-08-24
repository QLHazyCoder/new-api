package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

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
	if err := model.SaveSensitiveWordConfig(cfg); err != nil {
		common.ApiError(c, err)
		return
	}
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

type sensitiveRuleRequest struct {
	Word    string   `json:"word"`
	Scope   string   `json:"scope"`
	Groups  []string `json:"groups"`
	Enabled *bool    `json:"enabled"`
}

func CreateSensitiveWordRule(c *gin.Context) {
	var req sensitiveRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	rule, err := model.UpsertSensitiveWordRule(0, req.Word, req.Scope, req.Groups, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rule})
}
func UpdateSensitiveWordRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req sensitiveRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	rule, err := model.UpsertSensitiveWordRule(id, req.Word, req.Scope, req.Groups, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Enabled != nil {
		model.DB.Model(&model.SensitiveWordRule{}).Where("id = ?", id).Update("enabled", *req.Enabled)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rule})
}
func DeleteSensitiveWordRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := model.DeleteSensitiveWordRule(id); err != nil {
		common.ApiError(c, err)
		return
	}
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
	item, err := model.UpsertSensitiveWordWhitelist(req.UserID, c.GetInt("id"), req.Enabled, req.Remark)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": item})
}
func DeleteSensitiveWordWhitelist(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("user_id"))
	if err := model.DeleteSensitiveWordWhitelist(id); err != nil {
		common.ApiError(c, err)
		return
	}
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
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var event model.SensitiveWordAuditEvent
	if err := model.DB.First(&event, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": event})
}
func ClearSensitiveWordViolations(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := model.ClearSensitiveWordViolations(id); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordOperationAuditLog(c.GetInt("id"), "clear sensitive word violations", c.ClientIP(), "sensitive_word_clear_violations", map[string]interface{}{"user_id": id}, nil, nil)
	c.JSON(http.StatusOK, gin.H{"success": true})
}
func UnbanSensitiveWordUser(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := model.UnbanSensitiveWordUser(id); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordOperationAuditLog(c.GetInt("id"), "unban sensitive word user", c.ClientIP(), "sensitive_word_unban", map[string]interface{}{"user_id": id}, nil, nil)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Keep the JSON shape stable for clients that want to import rules in bulk.
func DecodeSensitiveWordImport(raw string) ([]string, error) {
	var words []string
	err := json.Unmarshal([]byte(raw), &words)
	return words, err
}
