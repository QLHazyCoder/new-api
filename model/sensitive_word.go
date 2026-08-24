package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SensitiveWordScopeGlobal    = "global"
	SensitiveWordScopeGroup     = "group"
	SensitiveWordLogAction      = "sensitive_word_block"
	SensitiveWordBanThreshold   = 5
	SensitiveWordMaxRunes       = 200
	SensitiveWordMaxPromptRunes = 65536
)

type SensitiveWordRule struct {
	ID        int64                    `json:"id" gorm:"primaryKey"`
	Word      string                   `json:"word" gorm:"type:varchar(200);not null;index"`
	Scope     string                   `json:"scope" gorm:"type:varchar(16);not null;index"`
	Enabled   bool                     `json:"enabled" gorm:"not null;default:true;index"`
	CreatedBy int                      `json:"created_by" gorm:"index"`
	Version   int64                    `json:"version" gorm:"not null;default:1"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
	Groups    []SensitiveWordRuleGroup `json:"groups,omitempty" gorm:"foreignKey:RuleID;constraint:OnDelete:CASCADE"`
}

type SensitiveWordRuleGroup struct {
	RuleID    int64  `json:"rule_id" gorm:"primaryKey"`
	GroupName string `json:"group_name" gorm:"type:varchar(64);primaryKey;index"`
}

type SensitiveWordWhitelist struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	UserID    int       `json:"user_id" gorm:"uniqueIndex;not null"`
	Enabled   bool      `json:"enabled" gorm:"not null;default:true;index"`
	Remark    string    `json:"remark" gorm:"type:varchar(255)"`
	CreatedBy int       `json:"created_by" gorm:"index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	User      *User     `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

type SensitiveWordAuditEvent struct {
	ID                int64     `json:"id" gorm:"primaryKey"`
	RequestID         string    `json:"request_id" gorm:"type:varchar(128);index"`
	UserID            int       `json:"user_id" gorm:"index"`
	UsernameSnapshot  string    `json:"username_snapshot"`
	TokenID           int       `json:"token_id" gorm:"index"`
	TokenNameSnapshot string    `json:"token_name_snapshot"`
	GroupName         string    `json:"group_name" gorm:"index"`
	ModelName         string    `json:"model_name" gorm:"index"`
	Endpoint          string    `json:"endpoint"`
	Protocol          string    `json:"protocol"`
	PromptHash        string    `json:"prompt_hash" gorm:"type:char(64);index"`
	RedactedPreview   string    `json:"redacted_preview" gorm:"type:text"`
	FullPrompt        string    `json:"full_prompt" gorm:"type:text"`
	MatchedRuleIDs    string    `json:"matched_rule_ids" gorm:"type:text"`
	MatchedWords      string    `json:"matched_words" gorm:"type:text"`
	MatchedSnippets   string    `json:"matched_snippets" gorm:"type:text"`
	MatchedScope      string    `json:"matched_scope" gorm:"type:varchar(16);index"`
	WhitelistBypassed bool      `json:"whitelist_bypassed" gorm:"index"`
	Blocked           bool      `json:"blocked" gorm:"index"`
	ViolationCount    int       `json:"violation_count"`
	AutoBanned        bool      `json:"auto_banned"`
	UserStatusBefore  int       `json:"user_status_before"`
	UserStatusAfter   int       `json:"user_status_after"`
	QuotaBefore       int       `json:"quota_before"`
	QuotaAfter        int       `json:"quota_after"`
	RuleVersion       int64     `json:"rule_version"`
	CreatedAt         time.Time `json:"created_at" gorm:"index"`
}

type SensitiveWordStats struct {
	TotalRules     int64 `json:"total_rules"`
	GlobalRules    int64 `json:"global_rules"`
	GroupRules     int64 `json:"group_rules"`
	WhitelistUsers int64 `json:"whitelist_users"`
	TodayHits      int64 `json:"today_hits"`
	TodayBlocks    int64 `json:"today_blocks"`
	TodayWhitelist int64 `json:"today_whitelist"`
	TodayAutoBans  int64 `json:"today_auto_bans"`
}

type SensitiveWordConfig struct {
	Enabled                 bool   `json:"enabled"`
	AuditEnabled            bool   `json:"audit_enabled"`
	BlockMessage            string `json:"block_message"`
	BanThreshold            int    `json:"ban_threshold"`
	FullPromptRetentionDays int    `json:"full_prompt_retention_days"`
	MaxPromptRunes          int    `json:"max_prompt_runes"`
	RuleVersion             int64  `json:"rule_version"`
}

type SensitiveCheckInput struct {
	RequestID string
	UserID    int
	Username  string
	TokenID   int
	TokenName string
	GroupName string
	ModelName string
	Endpoint  string
	Protocol  string
	Prompt    string
}

type SensitiveCheckResult struct {
	Matched           bool
	Blocked           bool
	WhitelistBypassed bool
	AutoBanned        bool
	ViolationCount    int
	MatchedWords      []string
	MatchedRuleIDs    []int64
	MatchedScope      string
	AuditID           int64
	Message           string
	UserStatusBefore  int
	UserStatusAfter   int
	QuotaBefore       int
	QuotaAfter        int
}

func normalizeSensitiveWord(word string) (string, error) {
	word = strings.TrimSpace(word)
	if word == "" {
		return "", errors.New("敏感词不能为空")
	}
	if len([]rune(word)) > SensitiveWordMaxRunes {
		return "", fmt.Errorf("敏感词长度不能超过 %d", SensitiveWordMaxRunes)
	}
	return word, nil
}

func validSensitiveGroups(groups []string) ([]string, error) {
	available := ratio_setting.GetGroupRatioCopy()
	seen := map[string]bool{}
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" || seen[group] {
			continue
		}
		if _, ok := available[group]; !ok {
			return nil, fmt.Errorf("分组 %s 不在分组定价中", group)
		}
		seen[group] = true
		result = append(result, group)
	}
	sort.Strings(result)
	return result, nil
}

func GetSensitiveWordConfig() SensitiveWordConfig {
	cfg := SensitiveWordConfig{Enabled: setting.ShouldCheckPromptSensitive(), AuditEnabled: true, BlockMessage: "你的请求因命中敏感词已被拦截，已记录 1 次；累计超过 5 次将立即封号并清空余额。请勿使用当前分组进行违规对话；如有误判，请联系群主审核并清理你的记录。", BanThreshold: SensitiveWordBanThreshold, FullPromptRetentionDays: 180, MaxPromptRunes: SensitiveWordMaxPromptRunes}
	var option Option
	if DB.Where("`key` = ?", "SensitiveWordConfig").First(&option).Error == nil {
		_ = json.Unmarshal([]byte(option.Value), &cfg)
	}
	if cfg.BanThreshold <= 0 {
		cfg.BanThreshold = SensitiveWordBanThreshold
	}
	if cfg.MaxPromptRunes <= 0 || cfg.MaxPromptRunes > SensitiveWordMaxPromptRunes {
		cfg.MaxPromptRunes = SensitiveWordMaxPromptRunes
	}
	if cfg.BlockMessage == "" {
		cfg.BlockMessage = "你的请求因命中敏感词已被拦截，已记录 1 次；累计超过 5 次将立即封号并清空余额。请勿使用当前分组进行违规对话；如有误判，请联系群主审核并清理你的记录。"
	}
	return cfg
}

func GetSensitiveWordStats() (SensitiveWordStats, error) {
	var stats SensitiveWordStats
	if err := DB.Model(&SensitiveWordRule{}).Count(&stats.TotalRules).Error; err != nil {
		return stats, err
	}
	if err := DB.Model(&SensitiveWordRule{}).Where("scope = ?", SensitiveWordScopeGlobal).Count(&stats.GlobalRules).Error; err != nil {
		return stats, err
	}
	if err := DB.Model(&SensitiveWordRule{}).Where("scope = ?", SensitiveWordScopeGroup).Count(&stats.GroupRules).Error; err != nil {
		return stats, err
	}
	if err := DB.Model(&SensitiveWordWhitelist{}).Where("enabled = ?", true).Count(&stats.WhitelistUsers).Error; err != nil {
		return stats, err
	}
	start := time.Now().Truncate(24 * time.Hour)
	base := DB.Model(&SensitiveWordAuditEvent{}).Where("created_at >= ?", start)
	if err := base.Count(&stats.TodayHits).Error; err != nil {
		return stats, err
	}
	if err := base.Where("blocked = ?", true).Count(&stats.TodayBlocks).Error; err != nil {
		return stats, err
	}
	if err := base.Where("whitelist_bypassed = ?", true).Count(&stats.TodayWhitelist).Error; err != nil {
		return stats, err
	}
	if err := base.Where("auto_banned = ?", true).Count(&stats.TodayAutoBans).Error; err != nil {
		return stats, err
	}
	return stats, nil
}

func SaveSensitiveWordConfig(cfg SensitiveWordConfig) error {
	if cfg.BanThreshold <= 0 || cfg.BanThreshold > 1000 {
		return errors.New("封禁阈值必须在 1 到 1000 之间")
	}
	if cfg.MaxPromptRunes <= 0 || cfg.MaxPromptRunes > SensitiveWordMaxPromptRunes {
		return errors.New("提示词长度限制无效")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return DB.Save(&Option{Key: "SensitiveWordConfig", Value: string(raw)}).Error
}

func ListSensitiveWordRules() ([]SensitiveWordRule, error) {
	var rules []SensitiveWordRule
	err := DB.Preload("Groups").Order("id desc").Find(&rules).Error
	return rules, err
}

func UpsertSensitiveWordRule(id int64, word, scope string, groups []string, actor int) (*SensitiveWordRule, error) {
	word, err := normalizeSensitiveWord(word)
	if err != nil {
		return nil, err
	}
	if scope != SensitiveWordScopeGlobal && scope != SensitiveWordScopeGroup {
		return nil, errors.New("规则范围无效")
	}
	groups, err = validSensitiveGroups(groups)
	if err != nil {
		return nil, err
	}
	if scope == SensitiveWordScopeGlobal {
		groups = nil
	} else if len(groups) == 0 {
		return nil, errors.New("局部规则至少选择一个分组")
	}
	var rule SensitiveWordRule
	err = DB.Transaction(func(tx *gorm.DB) error {
		if id > 0 {
			if err := tx.First(&rule, id).Error; err != nil {
				return err
			}
		} else {
			rule = SensitiveWordRule{CreatedBy: actor, Enabled: true, Version: 1}
		}
		rule.Word, rule.Scope, rule.UpdatedAt = word, scope, time.Now()
		rule.Version++
		if err := tx.Save(&rule).Error; err != nil {
			return err
		}
		if err := tx.Where("rule_id = ?", rule.ID).Delete(&SensitiveWordRuleGroup{}).Error; err != nil {
			return err
		}
		for _, group := range groups {
			if err := tx.Create(&SensitiveWordRuleGroup{RuleID: rule.ID, GroupName: group}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func DeleteSensitiveWordRule(id int64) error { return DB.Delete(&SensitiveWordRule{}, id).Error }

func ListSensitiveWordGroups() []string {
	groups := ratio_setting.GetGroupRatioCopy()
	result := make([]string, 0, len(groups))
	for group := range groups {
		result = append(result, group)
	}
	sort.Strings(result)
	return result
}

func UpsertSensitiveWordWhitelist(userID, actor int, enabled bool, remark string) (*SensitiveWordWhitelist, error) {
	if userID <= 0 {
		return nil, errors.New("用户 ID 无效")
	}
	var item SensitiveWordWhitelist
	err := DB.Where("user_id = ?", userID).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item = SensitiveWordWhitelist{UserID: userID, CreatedBy: actor}
	} else if err != nil {
		return nil, err
	}
	item.Enabled, item.Remark, item.UpdatedAt = enabled, strings.TrimSpace(remark), time.Now()
	if err := DB.Save(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func IsSensitiveWordWhitelisted(userID int) bool {
	var n int64
	DB.Model(&SensitiveWordWhitelist{}).Where("user_id = ? AND enabled = ?", userID, true).Count(&n)
	return n > 0
}

func ListSensitiveWordWhitelist() ([]SensitiveWordWhitelist, error) {
	var items []SensitiveWordWhitelist
	err := DB.Order("id desc").Find(&items).Error
	return items, err
}
func DeleteSensitiveWordWhitelist(userID int) error {
	return DB.Where("user_id = ?", userID).Delete(&SensitiveWordWhitelist{}).Error
}

func redactedSensitivePreview(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = regexp.MustCompile(`(?i)(sk-[a-z0-9_-]{12,}|bearer\s+[a-z0-9._-]{12,}|https?://[^\s]+)`).ReplaceAllString(text, "[已脱敏]")
	runes := []rune(text)
	if len(runes) > 96 {
		runes = runes[:96]
	}
	return string(runes)
}

func matchSensitiveRules(prompt, group string) ([]SensitiveWordRule, []string) {
	var rules []SensitiveWordRule
	DB.Preload("Groups").Where("enabled = ?", true).Order("id asc").Find(&rules)
	hits := make([]SensitiveWordRule, 0)
	words := make([]string, 0)
	seen := map[string]bool{}
	lower := strings.ToLower(prompt)
	for _, rule := range rules {
		if rule.Scope == SensitiveWordScopeGroup {
			ok := false
			for _, g := range rule.Groups {
				if g.GroupName == group {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		if strings.Contains(lower, strings.ToLower(rule.Word)) {
			hits = append(hits, rule)
			if !seen[rule.Word] {
				words = append(words, rule.Word)
				seen[rule.Word] = true
			}
		}
	}
	return hits, words
}

func CheckSensitiveRequest(input SensitiveCheckInput) (*SensitiveCheckResult, error) {
	cfg := GetSensitiveWordConfig()
	if !cfg.Enabled || strings.TrimSpace(input.Prompt) == "" {
		return &SensitiveCheckResult{}, nil
	}
	rules, words := matchSensitiveRules(input.Prompt, input.GroupName)
	if len(rules) == 0 {
		// Keep the legacy option active during migration.
		legacyWords := make([]string, 0)
		for _, word := range setting.SensitiveWords {
			if strings.Contains(strings.ToLower(input.Prompt), strings.ToLower(word)) {
				legacyWords = append(legacyWords, word)
			}
		}
		if len(legacyWords) > 0 {
			words = legacyWords
			rules = []SensitiveWordRule{{ID: 0, Word: legacyWords[0], Scope: SensitiveWordScopeGlobal, Version: 1}}
		}
	}
	if len(rules) == 0 {
		return &SensitiveCheckResult{}, nil
	}
	ids := make([]int64, 0, len(rules))
	scopes := map[string]bool{}
	for _, r := range rules {
		ids = append(ids, r.ID)
		scopes[r.Scope] = true
	}
	scope := SensitiveWordScopeGlobal
	if !scopes[SensitiveWordScopeGlobal] {
		scope = SensitiveWordScopeGroup
	} else if scopes[SensitiveWordScopeGroup] {
		scope = "global+group"
	}
	hash := sha256.Sum256([]byte(input.Prompt))
	ruleVersion := int64(1)
	for _, r := range rules {
		if r.Version > ruleVersion {
			ruleVersion = r.Version
		}
	}
	whitelist := IsSensitiveWordWhitelisted(input.UserID)
	result := &SensitiveCheckResult{Matched: true, WhitelistBypassed: whitelist, Blocked: !whitelist, MatchedWords: words, MatchedRuleIDs: ids, MatchedScope: scope, Message: cfg.BlockMessage}
	var user User
	if err := DB.First(&user, input.UserID).Error; err != nil {
		return nil, err
	}
	result.UserStatusBefore, result.QuotaBefore = user.Status, user.Quota
	if !whitelist {
		err := DB.Transaction(func(tx *gorm.DB) error {
			var locked User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, input.UserID).Error; err != nil {
				return err
			}
			var count int64
			if err := tx.Model(&SensitiveWordAuditEvent{}).Where("user_id = ? AND blocked = ?", input.UserID, true).Count(&count).Error; err != nil {
				return err
			}
			result.ViolationCount = int(count) + 1
			result.AutoBanned = result.ViolationCount >= cfg.BanThreshold && locked.Role != common.RoleRootUser && locked.Status != common.UserStatusDisabled
			if result.AutoBanned {
				next, err := IncrementUserAuthVersionWithTx(tx, input.UserID)
				if err != nil {
					return err
				}
				result.UserStatusAfter = common.UserStatusDisabled
				result.QuotaAfter = 0
				if err := tx.Model(&User{}).Where("id = ?", input.UserID).Updates(map[string]any{"status": common.UserStatusDisabled, "quota": 0, "auth_version": next}).Error; err != nil {
					return err
				}
			} else {
				result.UserStatusAfter, result.QuotaAfter = locked.Status, locked.Quota
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		_ = invalidateUserCache(input.UserID)
	} else {
		result.UserStatusAfter, result.QuotaAfter = user.Status, user.Quota
	}
	event := &SensitiveWordAuditEvent{RequestID: input.RequestID, UserID: input.UserID, UsernameSnapshot: input.Username, TokenID: input.TokenID, TokenNameSnapshot: input.TokenName, GroupName: input.GroupName, ModelName: input.ModelName, Endpoint: input.Endpoint, Protocol: input.Protocol, PromptHash: hex.EncodeToString(hash[:]), RedactedPreview: redactedSensitivePreview(input.Prompt), FullPrompt: truncateSensitivePrompt(input.Prompt, cfg.MaxPromptRunes), MatchedScope: scope, WhitelistBypassed: whitelist, Blocked: result.Blocked, ViolationCount: result.ViolationCount, AutoBanned: result.AutoBanned, UserStatusBefore: result.UserStatusBefore, UserStatusAfter: result.UserStatusAfter, QuotaBefore: result.QuotaBefore, QuotaAfter: result.QuotaAfter, RuleVersion: ruleVersion, CreatedAt: time.Now()}
	idsRaw, _ := json.Marshal(ids)
	wordsRaw, _ := json.Marshal(words)
	event.MatchedRuleIDs, event.MatchedWords = string(idsRaw), string(wordsRaw)
	event.MatchedSnippets = string(wordsRaw)
	if err := DB.Create(event).Error; err != nil {
		return nil, err
	}
	result.AuditID = event.ID
	other := map[string]interface{}{"action": SensitiveWordLogAction, "audit_id": event.ID, "matched_words": words, "scope": scope, "whitelist_bypassed": whitelist, "violation_count": result.ViolationCount, "blocked": result.Blocked, "auto_banned": result.AutoBanned}
	log := &Log{UserId: input.UserID, Username: input.Username, CreatedAt: common.GetTimestamp(), Type: LogTypeSensitiveWordBlock, Content: "sensitive word audit: " + strings.Join(words, ", "), TokenId: input.TokenID, ModelName: input.ModelName, Group: input.GroupName, RequestId: input.RequestID, Other: common.MapToJsonStr(other)}
	if err := createLog(log); err != nil {
		common.SysLog("failed to record sensitive word log: " + err.Error())
	}
	return result, nil
}

func CleanupSensitiveWordAudits() error {
	cfg := GetSensitiveWordConfig()
	if cfg.FullPromptRetentionDays <= 0 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -cfg.FullPromptRetentionDays)
	return DB.Model(&SensitiveWordAuditEvent{}).Where("created_at < ?", cutoff).Updates(map[string]any{"full_prompt": "", "redacted_preview": ""}).Error
}

func truncateSensitivePrompt(value string, max int) string {
	value = strings.ReplaceAll(value, "\x00", "")
	runes := []rune(strings.TrimSpace(value))
	if max <= 0 {
		max = SensitiveWordMaxPromptRunes
	}
	if len(runes) > max {
		return string(runes[:max]) + "…"
	}
	return string(runes)
}

func ClearSensitiveWordViolations(userID int) error {
	return DB.Where("user_id = ? AND blocked = ?", userID, true).Delete(&SensitiveWordAuditEvent{}).Error
}
func UnbanSensitiveWordUser(userID int) error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		next, err := IncrementUserAuthVersionWithTx(tx, userID)
		if err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", userID).Updates(map[string]any{"status": common.UserStatusEnabled, "auth_version": next}).Error
	})
	if err == nil {
		_ = invalidateUserCache(userID)
	}
	return err
}
