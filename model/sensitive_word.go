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
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	goahocorasick "github.com/anknown/ahocorasick"
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
	SensitiveWordMaxRules       = 10000
	sensitiveWordConfigCacheTTL = 5 * time.Second
	sensitiveWordMigrationKey   = "SensitiveWordRulesMigrationVersion"
	sensitiveWordMigrationValue = "1"
)

type SensitiveWordRule struct {
	ID   int64  `json:"id" gorm:"primaryKey"`
	Name string `json:"name" gorm:"type:varchar(64);not null;default:'';index"`
	// Word is retained for migration and compatibility with installations that
	// predate the rule-word child table. New writes use Words instead.
	Word      string                   `json:"-" gorm:"type:varchar(200);not null;index"`
	Scope     string                   `json:"scope" gorm:"type:varchar(16);not null;index"`
	Enabled   bool                     `json:"enabled" gorm:"not null;default:true;index"`
	CreatedBy int                      `json:"created_by" gorm:"index"`
	Version   int64                    `json:"version" gorm:"not null;default:1"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
	Groups    []SensitiveWordRuleGroup `json:"groups,omitempty" gorm:"foreignKey:RuleID;constraint:OnDelete:CASCADE"`
	Words     []SensitiveWordRuleWord  `json:"-" gorm:"foreignKey:RuleID;constraint:OnDelete:CASCADE"`
}

type SensitiveWordRuleWord struct {
	ID             int64     `json:"id" gorm:"primaryKey"`
	RuleID         int64     `json:"rule_id" gorm:"not null;index;uniqueIndex:idx_sensitive_rule_word"`
	Word           string    `json:"word" gorm:"type:text;not null"`
	NormalizedHash string    `json:"-" gorm:"type:char(64);not null;uniqueIndex:idx_sensitive_rule_word"`
	CreatedAt      time.Time `json:"created_at"`
}

type SensitiveWordRuleSummary struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Scope     string    `json:"scope"`
	Groups    []string  `json:"groups"`
	WordCount int       `json:"word_count"`
	Enabled   bool      `json:"enabled"`
	CreatedBy int       `json:"created_by"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SensitiveWordRuleDetail struct {
	SensitiveWordRuleSummary
	Words []string `json:"words"`
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
	MatchedRuleNames  string    `json:"matched_rule_names" gorm:"type:text"`
	MatchedWords      string    `json:"matched_words" gorm:"type:text"`
	MatchedSnippets   string    `json:"matched_snippets" gorm:"type:text"`
	MatchedScope      string    `json:"matched_scope" gorm:"type:varchar(16);index"`
	WhitelistBypassed bool      `json:"whitelist_bypassed" gorm:"index"`
	Blocked           bool      `json:"blocked" gorm:"index"`
	ViolationCount    int       `json:"violation_count"`
	AutoBanned        bool      `json:"auto_banned"`
	ObserveOnly       bool      `json:"observe_only"`
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
	CheckPrompt             bool   `json:"check_prompt"`
	Mode                    string `json:"mode"`
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
	MatchedRuleNames  []string
	MatchedScope      string
	AuditID           int64
	Message           string
	UserStatusBefore  int
	UserStatusAfter   int
	QuotaBefore       int
	QuotaAfter        int
	ObserveOnly       bool
}

type sensitiveRuntimeRule struct {
	ID      int64
	Name    string
	Scope   string
	Version int64
	Groups  map[string]struct{}
}

type sensitiveRuntimeSnapshot struct {
	machine           *goahocorasick.Machine
	words             map[string][]sensitiveRuntimeRule
	version           int64
	migrationComplete bool
	legacyWords       []string
}

var sensitiveRuntime struct {
	sync.RWMutex
	snapshot *sensitiveRuntimeSnapshot
}

var sensitiveConfigRuntime struct {
	sync.RWMutex
	config   SensitiveWordConfig
	loadedAt time.Time
	loaded   bool
}

const (
	sensitiveWordBlockMessage       = "你的请求因命中敏感词已被拦截，已记录 1 次；累计超过 5 次将立即封号，余额不退，如果有攻击破解别人网站等情节严重的情况将会直接报警。请勿使用当前分组进行违规对话；如有误判，请联系群主审核并清理你的记录。"
	legacySensitiveWordBlockMessage = "你的请求因命中敏感词已被拦截，已记录 1 次；累计超过 5 次将立即封号，但不会清理余额。请勿使用当前分组进行违规对话；如有误判，请联系群主审核并清理你的记录。"
)

func invalidateSensitiveWordRuntime() {
	sensitiveRuntime.Lock()
	sensitiveRuntime.snapshot = nil
	sensitiveRuntime.Unlock()

	// Rules and policy share the same relay path. Clear both local snapshots
	// together so a successful admin save takes effect on the next request.
	sensitiveConfigRuntime.Lock()
	sensitiveConfigRuntime.config = SensitiveWordConfig{}
	sensitiveConfigRuntime.loadedAt = time.Time{}
	sensitiveConfigRuntime.loaded = false
	sensitiveConfigRuntime.Unlock()
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

func sensitiveWordHash(word string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(word))))
	return hex.EncodeToString(hash[:])
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
		if group == "auto" {
			return nil, errors.New("局部敏感词规则不能绑定自动分组")
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

func defaultSensitiveWordConfig() SensitiveWordConfig {
	return SensitiveWordConfig{
		Enabled:                 setting.ShouldCheckPromptSensitive(),
		CheckPrompt:             true,
		Mode:                    "block",
		AuditEnabled:            true,
		BlockMessage:            sensitiveWordBlockMessage,
		BanThreshold:            SensitiveWordBanThreshold,
		FullPromptRetentionDays: 180,
		MaxPromptRunes:          SensitiveWordMaxPromptRunes,
	}
}

func normalizeSensitiveWordConfig(cfg SensitiveWordConfig) SensitiveWordConfig {
	if cfg.BanThreshold <= 0 {
		cfg.BanThreshold = SensitiveWordBanThreshold
	}
	if cfg.Mode != "block" && cfg.Mode != "observe" && cfg.Mode != "off" {
		cfg.Mode = "block"
	}
	if cfg.MaxPromptRunes <= 0 || cfg.MaxPromptRunes > SensitiveWordMaxPromptRunes {
		cfg.MaxPromptRunes = SensitiveWordMaxPromptRunes
	}
	if cfg.BlockMessage == "" || cfg.BlockMessage == legacySensitiveWordBlockMessage || strings.Contains(cfg.BlockMessage, "清空余额") || strings.Contains(cfg.BlockMessage, "清零余额") {
		cfg.BlockMessage = sensitiveWordBlockMessage
	}
	return cfg
}

func loadSensitiveWordConfig() SensitiveWordConfig {
	cfg := defaultSensitiveWordConfig()
	if DB != nil {
		var option Option
		if DB.Where(&Option{Key: "SensitiveWordConfig"}).First(&option).Error == nil {
			_ = json.Unmarshal([]byte(option.Value), &cfg)
		}
	}
	cfg = normalizeSensitiveWordConfig(cfg)
	// Reuse the rule snapshot for the displayed version instead of adding a
	// MAX(version) database query to every relay request.
	if snapshot, err := getSensitiveRuntimeSnapshot(); err == nil && snapshot != nil && snapshot.version > cfg.RuleVersion {
		cfg.RuleVersion = snapshot.version
	}
	if cfg.RuleVersion <= 0 {
		cfg.RuleVersion = 1
	}
	return cfg
}

func GetSensitiveWordConfig() SensitiveWordConfig {
	now := time.Now()
	sensitiveConfigRuntime.RLock()
	if sensitiveConfigRuntime.loaded && now.Sub(sensitiveConfigRuntime.loadedAt) < sensitiveWordConfigCacheTTL {
		cfg := sensitiveConfigRuntime.config
		sensitiveConfigRuntime.RUnlock()
		return cfg
	}
	sensitiveConfigRuntime.RUnlock()

	sensitiveConfigRuntime.Lock()
	defer sensitiveConfigRuntime.Unlock()
	if sensitiveConfigRuntime.loaded && now.Sub(sensitiveConfigRuntime.loadedAt) < sensitiveWordConfigCacheTTL {
		return sensitiveConfigRuntime.config
	}
	cfg := loadSensitiveWordConfig()
	sensitiveConfigRuntime.config = cfg
	sensitiveConfigRuntime.loadedAt = now
	sensitiveConfigRuntime.loaded = true
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
	if err := DB.Model(&User{}).Where("sensitive_word_whitelist = ?", true).Count(&stats.WhitelistUsers).Error; err != nil {
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
	if cfg.Mode != "block" && cfg.Mode != "observe" && cfg.Mode != "off" {
		return errors.New("敏感词处理模式无效")
	}
	cfg = normalizeSensitiveWordConfig(cfg)
	current := GetSensitiveWordConfig()
	if cfg.RuleVersion <= current.RuleVersion {
		cfg.RuleVersion = current.RuleVersion + 1
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := DB.Save(&Option{Key: "SensitiveWordConfig", Value: string(raw)}).Error; err != nil {
		return err
	}
	invalidateSensitiveWordRuntime()
	return nil
}

func normalizeSensitiveWords(words []string) ([]string, error) {
	result := make([]string, 0, len(words))
	seen := make(map[string]struct{}, len(words))
	for _, raw := range words {
		word, err := normalizeSensitiveWord(raw)
		if err != nil {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			return nil, err
		}
		normalized := strings.ToLower(word)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, word)
	}
	if len(result) == 0 {
		return nil, errors.New("敏感词规则至少包含一个有效词条")
	}
	if len(result) > SensitiveWordMaxRules {
		return nil, fmt.Errorf("单条规则最多包含 %d 个敏感词", SensitiveWordMaxRules)
	}
	return result, nil
}

func ruleWords(rule SensitiveWordRule) []string {
	words := make([]string, 0, len(rule.Words)+1)
	seen := make(map[string]struct{})
	for _, item := range rule.Words {
		if item.Word == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(item.Word))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		words = append(words, strings.TrimSpace(item.Word))
	}
	if len(words) == 0 && strings.TrimSpace(rule.Word) != "" {
		for _, item := range strings.Split(rule.Word, "\n") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			key := strings.ToLower(item)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			words = append(words, item)
		}
	}
	return words
}

func buildSensitiveWordRuleSummaryWithCount(rule SensitiveWordRule, wordCount int) SensitiveWordRuleSummary {
	groups := make([]string, 0, len(rule.Groups))
	for _, item := range rule.Groups {
		groups = append(groups, item.GroupName)
	}
	sort.Strings(groups)
	return SensitiveWordRuleSummary{ID: rule.ID, Name: rule.Name, Scope: rule.Scope, Groups: groups, WordCount: wordCount, Enabled: rule.Enabled, CreatedBy: rule.CreatedBy, Version: rule.Version, CreatedAt: rule.CreatedAt, UpdatedAt: rule.UpdatedAt}
}

func buildSensitiveWordRuleSummary(rule SensitiveWordRule) SensitiveWordRuleSummary {
	return buildSensitiveWordRuleSummaryWithCount(rule, len(ruleWords(rule)))
}

func ListSensitiveWordRules() ([]SensitiveWordRuleSummary, error) {
	var rules []SensitiveWordRule
	if err := DB.Preload("Groups").Order("id desc").Find(&rules).Error; err != nil {
		return nil, err
	}
	ruleIDs := make([]int64, 0, len(rules))
	for _, rule := range rules {
		ruleIDs = append(ruleIDs, rule.ID)
	}
	wordCounts := make(map[int64]int, len(ruleIDs))
	if len(ruleIDs) > 0 {
		var counts []struct {
			RuleID    int64 `gorm:"column:rule_id"`
			WordCount int   `gorm:"column:word_count"`
		}
		if err := DB.Model(&SensitiveWordRuleWord{}).
			Select("rule_id, COUNT(*) AS word_count").
			Where("rule_id IN ?", ruleIDs).
			Group("rule_id").
			Scan(&counts).Error; err != nil {
			return nil, err
		}
		for _, item := range counts {
			wordCounts[item.RuleID] = item.WordCount
		}
	}
	result := make([]SensitiveWordRuleSummary, 0, len(rules))
	for _, rule := range rules {
		wordCount := wordCounts[rule.ID]
		if wordCount == 0 && strings.TrimSpace(rule.Word) != "" {
			wordCount = len(ruleWords(rule))
		}
		result = append(result, buildSensitiveWordRuleSummaryWithCount(rule, wordCount))
	}
	return result, nil
}

func GetSensitiveWordRuleDetail(id int64) (*SensitiveWordRuleDetail, error) {
	var rule SensitiveWordRule
	if err := DB.Preload("Groups").Preload("Words").First(&rule, id).Error; err != nil {
		return nil, err
	}
	words := ruleWords(rule)
	return &SensitiveWordRuleDetail{SensitiveWordRuleSummary: buildSensitiveWordRuleSummary(rule), Words: words}, nil
}

func UpsertSensitiveWordRule(id int64, name string, words []string, scope string, groups []string, actor int, enabled *bool) (*SensitiveWordRuleDetail, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("规则名称不能为空")
	}
	if len([]rune(name)) > 64 {
		return nil, errors.New("规则名称不能超过 64 个字符")
	}
	if len(words) == 0 {
		return nil, errors.New("敏感词规则至少包含一个有效词条")
	}
	words, err := normalizeSensitiveWords(words)
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
		if enabled != nil {
			rule.Enabled = *enabled
		}
		rule.Name, rule.Scope, rule.UpdatedAt = name, scope, time.Now()
		// Keep the legacy column populated for old readers while the child table
		// becomes the canonical source for new code.
		rule.Word = words[0]
		if id > 0 {
			rule.Version++
		}
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
		if err := tx.Where("rule_id = ?", rule.ID).Delete(&SensitiveWordRuleWord{}).Error; err != nil {
			return err
		}
		for _, word := range words {
			if err := tx.Create(&SensitiveWordRuleWord{RuleID: rule.ID, Word: word, NormalizedHash: sensitiveWordHash(word), CreatedAt: time.Now()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	invalidateSensitiveWordRuntime()
	return GetSensitiveWordRuleDetail(rule.ID)
}

func SetSensitiveWordRuleEnabled(id int64, enabled bool) error {
	result := DB.Model(&SensitiveWordRule{}).Where("id = ?", id).Updates(map[string]interface{}{
		"enabled": enabled,
		"version": gorm.Expr("version + ?", 1),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	invalidateSensitiveWordRuntime()
	return nil
}

func DeleteSensitiveWordRule(id int64) error {
	if id <= 0 {
		return errors.New("规则 ID 无效")
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("rule_id = ?", id).Delete(&SensitiveWordRuleWord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("rule_id = ?", id).Delete(&SensitiveWordRuleGroup{}).Error; err != nil {
			return err
		}
		result := tx.Delete(&SensitiveWordRule{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	}); err != nil {
		return err
	}
	invalidateSensitiveWordRuntime()
	return nil
}

func ListSensitiveWordGroups() []string {
	groups := ratio_setting.GetGroupRatioCopy()
	result := make([]string, 0, len(groups))
	for group := range groups {
		if group == "auto" {
			continue
		}
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
	if err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Select("id").First(&user, userID).Error; err != nil {
			return err
		}
		if err := tx.Model(&User{}).Where("id = ?", userID).Update("sensitive_word_whitelist", enabled).Error; err != nil {
			return err
		}
		err := tx.Where("user_id = ?", userID).First(&item).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			item = SensitiveWordWhitelist{UserID: userID, CreatedBy: actor}
		} else if err != nil {
			return err
		}
		item.Enabled, item.Remark, item.UpdatedAt = enabled, strings.TrimSpace(remark), time.Now()
		return tx.Save(&item).Error
	}); err != nil {
		return nil, err
	}
	return &item, nil
}

func IsSensitiveWordWhitelisted(userID int) bool {
	var user User
	// The User column is canonical. The legacy table is migrated at startup and
	// retained only so existing admin integrations keep working. Falling back to
	// it here would make a user-drawer switch-off silently ineffective.
	return DB.Select("sensitive_word_whitelist").First(&user, userID).Error == nil && user.SensitiveWordWhitelist
}

func ListSensitiveWordWhitelist() ([]SensitiveWordWhitelist, error) {
	var users []User
	if err := DB.Select("id").Where("sensitive_word_whitelist = ?", true).Order("id desc").Find(&users).Error; err != nil {
		return nil, err
	}
	var legacyItems []SensitiveWordWhitelist
	if err := DB.Find(&legacyItems).Error; err != nil {
		return nil, err
	}
	legacyByUserID := make(map[int]SensitiveWordWhitelist, len(legacyItems))
	for _, item := range legacyItems {
		legacyByUserID[item.UserID] = item
	}
	items := make([]SensitiveWordWhitelist, 0, len(users))
	for index := range users {
		user := users[index]
		item, ok := legacyByUserID[user.Id]
		if !ok {
			item = SensitiveWordWhitelist{UserID: user.Id, Enabled: true}
		}
		item.Enabled = true
		items = append(items, item)
	}
	return items, nil
}
func DeleteSensitiveWordWhitelist(userID int) error {
	result := DB.Model(&User{}).Where("id = ?", userID).Update("sensitive_word_whitelist", false)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
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

// normalizeSensitivePrompt intentionally preserves line boundaries and JSON
// punctuation while removing transport-only variation. The same snapshot is
// used for matching, hashing and the administrator-only evidence record.
func normalizeSensitivePrompt(value string) string {
	value = strings.ReplaceAll(value, "\x00", "")
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimSpace(value)
}

func buildSensitiveRuntimeSnapshot() (*sensitiveRuntimeSnapshot, error) {
	var rules []SensitiveWordRule
	if err := DB.Preload("Groups").Preload("Words").Where("enabled = ?", true).Order("id asc").Find(&rules).Error; err != nil {
		return nil, err
	}
	words := make([]string, 0)
	metadata := make(map[string][]sensitiveRuntimeRule)
	seenWords := make(map[string]struct{})
	var version int64 = 1
	for _, rule := range rules {
		if rule.Version > version {
			version = rule.Version
		}
		groups := make(map[string]struct{}, len(rule.Groups))
		for _, group := range rule.Groups {
			groups[group.GroupName] = struct{}{}
		}
		for _, word := range ruleWords(rule) {
			normalized := strings.ToLower(word)
			if _, ok := seenWords[normalized]; !ok {
				seenWords[normalized] = struct{}{}
				words = append(words, normalized)
			}
			metadata[normalized] = append(metadata[normalized], sensitiveRuntimeRule{ID: rule.ID, Name: rule.Name, Scope: rule.Scope, Version: rule.Version, Groups: groups})
		}
	}
	var migrationOption Option
	migrationComplete := DB.Where(&Option{Key: sensitiveWordMigrationKey, Value: sensitiveWordMigrationValue}).First(&migrationOption).Error == nil
	legacyWords := make([]string, 0)
	if !migrationComplete {
		legacySource := append([]string(nil), setting.SensitiveWords...)
		var legacyOption Option
		if DB.Where(&Option{Key: "SensitiveWords"}).First(&legacyOption).Error == nil {
			legacySource = strings.Split(legacyOption.Value, "\n")
		}
		seenLegacyWords := make(map[string]struct{}, len(legacySource))
		for _, word := range legacySource {
			word = strings.TrimSpace(word)
			if word == "" {
				continue
			}
			key := strings.ToLower(word)
			if _, ok := seenLegacyWords[key]; ok {
				continue
			}
			seenLegacyWords[key] = struct{}{}
			legacyWords = append(legacyWords, word)
		}
	}
	if len(words) == 0 {
		return &sensitiveRuntimeSnapshot{version: version, words: metadata, migrationComplete: migrationComplete, legacyWords: legacyWords}, nil
	}
	keywords := make([][]rune, 0, len(words))
	for _, word := range words {
		keywords = append(keywords, []rune(word))
	}
	machine := new(goahocorasick.Machine)
	if err := machine.Build(keywords); err != nil {
		return nil, err
	}
	return &sensitiveRuntimeSnapshot{machine: machine, words: metadata, version: version, migrationComplete: migrationComplete, legacyWords: legacyWords}, nil
}

func getSensitiveRuntimeSnapshot() (*sensitiveRuntimeSnapshot, error) {
	sensitiveRuntime.RLock()
	snapshot := sensitiveRuntime.snapshot
	sensitiveRuntime.RUnlock()
	if snapshot != nil {
		return snapshot, nil
	}
	sensitiveRuntime.Lock()
	defer sensitiveRuntime.Unlock()
	if sensitiveRuntime.snapshot != nil {
		return sensitiveRuntime.snapshot, nil
	}
	built, err := buildSensitiveRuntimeSnapshot()
	if err != nil {
		return nil, err
	}
	sensitiveRuntime.snapshot = built
	return built, nil
}

func legacySensitiveWordMatches(prompt string) ([]string, error) {
	snapshot, err := getSensitiveRuntimeSnapshot()
	if err != nil {
		return nil, err
	}
	if snapshot == nil || snapshot.migrationComplete {
		return nil, nil
	}
	matched := make([]string, 0)
	lowerPrompt := strings.ToLower(prompt)
	for _, word := range snapshot.legacyWords {
		if strings.Contains(lowerPrompt, strings.ToLower(word)) && !containsSensitiveWord(matched, word) {
			matched = append(matched, word)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return strings.ToLower(matched[i]) < strings.ToLower(matched[j]) })
	return matched, nil
}

func isSensitiveWordMigrationComplete() bool {
	snapshot, err := getSensitiveRuntimeSnapshot()
	return err == nil && snapshot != nil && snapshot.migrationComplete
}

func matchSensitiveRules(prompt, group string) ([]SensitiveWordRule, []string, error) {
	snapshot, err := getSensitiveRuntimeSnapshot()
	if err != nil {
		return nil, nil, err
	}
	if snapshot == nil || snapshot.machine == nil {
		return nil, nil, nil
	}
	hits := make([]SensitiveWordRule, 0)
	words := make([]string, 0)
	seen := make(map[string]struct{})
	for _, term := range snapshot.machine.MultiPatternSearch([]rune(strings.ToLower(prompt)), false) {
		word := string(term.Word)
		for _, metadata := range snapshot.words[word] {
			if metadata.Scope == SensitiveWordScopeGroup {
				if _, ok := metadata.Groups[group]; !ok {
					continue
				}
			}
			key := fmt.Sprintf("%d:%s", metadata.ID, word)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			hits = append(hits, SensitiveWordRule{ID: metadata.ID, Name: metadata.Name, Word: word, Scope: metadata.Scope, Version: metadata.Version})
			if !containsSensitiveWord(words, word) {
				words = append(words, word)
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].ID != hits[j].ID {
			return hits[i].ID < hits[j].ID
		}
		return strings.ToLower(hits[i].Word) < strings.ToLower(hits[j].Word)
	})
	sort.Slice(words, func(i, j int) bool {
		return strings.ToLower(words[i]) < strings.ToLower(words[j])
	})
	return hits, words, nil
}

func containsSensitiveWord(words []string, candidate string) bool {
	for _, word := range words {
		if strings.EqualFold(word, candidate) {
			return true
		}
	}
	return false
}

func normalizeSensitiveGroups(groups []string, fallback string) []string {
	seen := make(map[string]struct{}, len(groups)+1)
	result := make([]string, 0, len(groups)+1)
	for _, group := range append(groups, fallback) {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		result = append(result, group)
	}
	return result
}

// matchSensitiveRulesForGroups evaluates each candidate exactly once. This is
// used for an auto group before billing so changing to another candidate during
// a retry cannot bypass a group-scoped policy.
func matchSensitiveRulesForGroups(prompt string, groups []string, fallbackGroup string) ([]SensitiveWordRule, []string, string, error) {
	candidates := normalizeSensitiveGroups(groups, fallbackGroup)
	if len(candidates) == 0 {
		candidates = []string{""}
	}
	rules := make([]SensitiveWordRule, 0)
	words := make([]string, 0)
	seenRules := make(map[string]struct{})
	matchedGroup := ""
	for _, group := range candidates {
		hits, hitWords, err := matchSensitiveRules(prompt, group)
		if err != nil {
			return nil, nil, "", err
		}
		for _, hit := range hits {
			key := fmt.Sprintf("%d:%s", hit.ID, strings.ToLower(hit.Word))
			if _, ok := seenRules[key]; ok {
				continue
			}
			seenRules[key] = struct{}{}
			rules = append(rules, hit)
			if hit.Scope == SensitiveWordScopeGroup && matchedGroup == "" {
				matchedGroup = group
			}
		}
		for _, word := range hitWords {
			if !containsSensitiveWord(words, word) {
				words = append(words, word)
			}
		}
	}
	if matchedGroup == "" && len(candidates) > 0 {
		matchedGroup = candidates[0]
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].ID != rules[j].ID {
			return rules[i].ID < rules[j].ID
		}
		return strings.ToLower(rules[i].Word) < strings.ToLower(rules[j].Word)
	})
	sort.Slice(words, func(i, j int) bool {
		return strings.ToLower(words[i]) < strings.ToLower(words[j])
	})
	return rules, words, matchedGroup, nil
}

func sensitiveMatchSnippets(prompt string, words []string) []string {
	promptRunes := []rune(prompt)
	normalizedPrompt := []rune(strings.ToLower(prompt))
	seen := make(map[string]struct{}, len(words))
	result := make([]string, 0, len(words))
	for _, word := range words {
		normalizedWord := []rune(strings.ToLower(strings.TrimSpace(word)))
		if len(normalizedWord) == 0 || len(normalizedWord) > len(normalizedPrompt) {
			continue
		}
		for start := 0; start+len(normalizedWord) <= len(normalizedPrompt); start++ {
			matched := true
			for offset := range normalizedWord {
				if normalizedPrompt[start+offset] != normalizedWord[offset] {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
			from := max(0, start-24)
			to := min(len(promptRunes), start+len(normalizedWord)+24)
			snippet := redactedSensitivePreview(string(promptRunes[from:to]))
			if _, ok := seen[snippet]; !ok && snippet != "" {
				seen[snippet] = struct{}{}
				result = append(result, snippet)
			}
			break
		}
	}
	return result
}

func CheckSensitiveRequest(input SensitiveCheckInput) (*SensitiveCheckResult, error) {
	return CheckSensitiveRequestForGroups(input, []string{input.GroupName})
}

// CheckSensitiveRequestForGroups records one policy decision for the request.
// Auto routing supplies its ordered candidate groups so a later cross-group
// retry cannot bypass a local rule after pre-consume has started.
func CheckSensitiveRequestForGroups(input SensitiveCheckInput, candidateGroups []string) (*SensitiveCheckResult, error) {
	cfg := GetSensitiveWordConfig()
	input.Prompt = normalizeSensitivePrompt(input.Prompt)
	if !cfg.Enabled || !cfg.CheckPrompt || cfg.Mode == "off" || input.Prompt == "" {
		return &SensitiveCheckResult{}, nil
	}

	rules, words, matchedGroup, err := matchSensitiveRulesForGroups(input.Prompt, candidateGroups, input.GroupName)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		// Only deployments that have not yet completed the one-time Option import
		// use the legacy option snapshot. Once the migration marker exists, rules
		// are controlled exclusively by the new table so deleting a rule takes
		// effect immediately instead of silently falling back to old data.
		legacyWords, legacyErr := legacySensitiveWordMatches(input.Prompt)
		if legacyErr != nil {
			return nil, legacyErr
		}
		if len(legacyWords) > 0 {
			words = legacyWords
			rules = []SensitiveWordRule{{ID: 0, Name: "旧配置导入", Word: legacyWords[0], Scope: SensitiveWordScopeGlobal, Version: 1}}
		}
	}
	if len(rules) == 0 {
		return &SensitiveCheckResult{}, nil
	}
	if matchedGroup != "" {
		input.GroupName = matchedGroup
	}

	ids := make([]int64, 0, len(rules))
	names := make([]string, 0, len(rules))
	seenIDs := make(map[int64]struct{}, len(rules))
	scopes := map[string]bool{}
	ruleVersion := int64(1)
	for _, rule := range rules {
		if _, ok := seenIDs[rule.ID]; !ok {
			seenIDs[rule.ID] = struct{}{}
			ids = append(ids, rule.ID)
			if rule.Name != "" {
				names = append(names, rule.Name)
			}
		}
		scopes[rule.Scope] = true
		if rule.Version > ruleVersion {
			ruleVersion = rule.Version
		}
	}
	scope := SensitiveWordScopeGlobal
	if !scopes[SensitiveWordScopeGlobal] {
		scope = SensitiveWordScopeGroup
	} else if scopes[SensitiveWordScopeGroup] {
		scope = "global+group"
	}
	hash := sha256.Sum256([]byte(input.Prompt))
	observeOnly := cfg.Mode == "observe"
	result := &SensitiveCheckResult{
		Matched:          true,
		ObserveOnly:      observeOnly,
		MatchedWords:     words,
		MatchedRuleIDs:   ids,
		MatchedRuleNames: names,
		MatchedScope:     scope,
		Message:          cfg.BlockMessage,
	}

	var event SensitiveWordAuditEvent
	err = DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, input.UserID).Error; err != nil {
			return err
		}
		result.UserStatusBefore, result.QuotaBefore = user.Status, user.Quota
		result.UserStatusAfter, result.QuotaAfter = user.Status, user.Quota

		// User.SensitiveWordWhitelist is the canonical policy state. See
		// MigrateSensitiveWordData for the one-way import of legacy rows.
		whitelisted := user.SensitiveWordWhitelist
		result.WhitelistBypassed = whitelisted
		result.Blocked = !whitelisted && !observeOnly

		if result.Blocked {
			result.ViolationCount = user.SensitiveWordViolationCount + 1
			updates := map[string]interface{}{"sensitive_word_violation_count": result.ViolationCount}
			result.AutoBanned = result.ViolationCount >= cfg.BanThreshold && user.Role != common.RoleRootUser && user.Status != common.UserStatusDisabled
			if result.AutoBanned {
				nextVersion, err := IncrementUserAuthVersionWithTx(tx, input.UserID)
				if err != nil {
					return err
				}
				updates["status"] = common.UserStatusDisabled
				updates["auth_version"] = nextVersion
				result.UserStatusAfter = common.UserStatusDisabled
			}
			if err := tx.Model(&User{}).Where("id = ?", input.UserID).Updates(updates).Error; err != nil {
				return err
			}
		} else {
			result.ViolationCount = user.SensitiveWordViolationCount
		}

		preview, fullPrompt := "", ""
		snippets := make([]string, 0)
		if cfg.AuditEnabled {
			preview = redactedSensitivePreview(input.Prompt)
			fullPrompt = truncateSensitivePrompt(input.Prompt, cfg.MaxPromptRunes)
			snippets = sensitiveMatchSnippets(input.Prompt, words)
		}
		idsRaw, _ := json.Marshal(ids)
		namesRaw, _ := json.Marshal(names)
		wordsRaw, _ := json.Marshal(words)
		snippetsRaw, _ := json.Marshal(snippets)
		event = SensitiveWordAuditEvent{
			RequestID: input.RequestID, UserID: input.UserID, UsernameSnapshot: input.Username,
			TokenID: input.TokenID, TokenNameSnapshot: input.TokenName, GroupName: input.GroupName,
			ModelName: input.ModelName, Endpoint: input.Endpoint, Protocol: input.Protocol,
			PromptHash: hex.EncodeToString(hash[:]), RedactedPreview: preview, FullPrompt: fullPrompt,
			MatchedRuleIDs: string(idsRaw), MatchedRuleNames: string(namesRaw), MatchedWords: string(wordsRaw), MatchedSnippets: string(snippetsRaw),
			MatchedScope: scope, WhitelistBypassed: whitelisted, Blocked: result.Blocked, ObserveOnly: observeOnly,
			ViolationCount: result.ViolationCount, AutoBanned: result.AutoBanned,
			UserStatusBefore: result.UserStatusBefore, UserStatusAfter: result.UserStatusAfter,
			QuotaBefore: result.QuotaBefore, QuotaAfter: result.QuotaAfter, RuleVersion: ruleVersion, CreatedAt: time.Now(),
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		// A failed transaction rolls back the counter and audit event. Do not let
		// the relay return the normal "recorded" blocking response in that case.
		result.Blocked = false
		result.AutoBanned = false
		result.ViolationCount = 0
		return result, err
	}

	result.AuditID = event.ID
	if result.AutoBanned {
		_ = invalidateUserCache(input.UserID)
		if _, err := RevokeAllUserSessions(input.UserID, "sensitive_word_threshold"); err != nil {
			common.SysLog("failed to revoke sensitive-word user sessions: " + err.Error())
		}
	}

	action := "blocked"
	if result.WhitelistBypassed {
		action = "whitelist_bypass"
	} else if observeOnly {
		action = "observe"
	}
	other := map[string]interface{}{
		"action":   SensitiveWordLogAction,
		"audit_id": event.ID,
		"keyword_filter": map[string]interface{}{
			"action":             action,
			"request_id":         input.RequestID,
			"rule_ids":           ids,
			"rule_names":         names,
			"matched_words":      words,
			"scope":              scope,
			"group":              input.GroupName,
			"model":              input.ModelName,
			"endpoint":           input.Endpoint,
			"protocol":           input.Protocol,
			"violation_count":    result.ViolationCount,
			"whitelist_bypassed": result.WhitelistBypassed,
			"blocked":            result.Blocked,
			"auto_banned":        result.AutoBanned,
			"observe_only":       observeOnly,
			"user_status_before": result.UserStatusBefore,
			"user_status_after":  result.UserStatusAfter,
			"balance_changed":    false,
			"prompt_hash":        event.PromptHash,
			"rule_version":       ruleVersion,
		},
	}
	log := &Log{
		UserId: input.UserID, Username: input.Username, CreatedAt: common.GetTimestamp(),
		Type: LogTypeSensitiveWordBlock, Content: "关键词拦截记录", TokenId: input.TokenID,
		TokenName: input.TokenName, ModelName: input.ModelName, Group: input.GroupName,
		RequestId: input.RequestID, Other: common.MapToJsonStr(other),
	}
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
	result := DB.Model(&User{}).Where("id = ?", userID).Update("sensitive_word_violation_count", 0)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// normalizeLegacySensitiveWords preserves the old matcher as a fallback when
// a historical option contains a value that the new editor intentionally
// rejects (for example, a word longer than the new limit). A migration must
// never make an already-enforced legacy word silently stop matching.
func normalizeLegacySensitiveWords(words []string) ([]string, bool) {
	result := make([]string, 0, min(len(words), SensitiveWordMaxRules))
	seen := make(map[string]struct{}, len(words))
	complete := true
	invalidCount := 0
	for _, raw := range words {
		word, err := normalizeSensitiveWord(raw)
		if err != nil {
			if strings.TrimSpace(raw) != "" {
				complete = false
				invalidCount++
			}
			continue
		}
		key := strings.ToLower(word)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if len(result) >= SensitiveWordMaxRules {
			complete = false
			invalidCount++
			continue
		}
		result = append(result, word)
	}
	if invalidCount > 0 {
		common.SysLog(fmt.Sprintf("sensitive-word migration retained %d legacy entries outside the new rule limits", invalidCount))
	}
	return result, complete
}

// MigrateSensitiveWordData imports legacy word-level rules and the old global
// option into the rule-word model. It is intentionally idempotent so both the
// normal and fast migration paths can call it safely.
func MigrateSensitiveWordData() error {
	if DB == nil {
		return nil
	}
	migrationComplete := true
	var rules []SensitiveWordRule
	if err := DB.Find(&rules).Error; err != nil {
		return err
	}
	for _, rule := range rules {
		if strings.TrimSpace(rule.Name) == "" {
			if err := DB.Model(&SensitiveWordRule{}).Where("id = ?", rule.ID).Update("name", fmt.Sprintf("迁移规则 #%d", rule.ID)).Error; err != nil {
				return err
			}
		}
		var count int64
		if err := DB.Model(&SensitiveWordRuleWord{}).Where("rule_id = ?", rule.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 || strings.TrimSpace(rule.Word) == "" {
			continue
		}
		words, complete := normalizeLegacySensitiveWords(strings.Split(rule.Word, "\n"))
		if !complete {
			// Keep the old Word column as the source for this rule. ruleWords
			// intentionally falls back to it when no child rows exist.
			migrationComplete = false
			continue
		}
		if len(words) == 0 {
			continue
		}
		if err := DB.Transaction(func(tx *gorm.DB) error {
			for _, word := range words {
				if err := tx.Create(&SensitiveWordRuleWord{RuleID: rule.ID, Word: word, NormalizedHash: sensitiveWordHash(word), CreatedAt: time.Now()}).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}

	var globalCount int64
	if err := DB.Model(&SensitiveWordRule{}).Where("scope = ?", SensitiveWordScopeGlobal).Count(&globalCount).Error; err != nil {
		return err
	}
	legacyWords := append([]string(nil), setting.SensitiveWords...)
	var legacyOption Option
	optionErr := DB.Where(&Option{Key: "SensitiveWords"}).First(&legacyOption).Error
	if optionErr == nil {
		legacyWords = strings.Split(legacyOption.Value, "\n")
	} else if !errors.Is(optionErr, gorm.ErrRecordNotFound) {
		return optionErr
	}
	legacyWords, legacyComplete := normalizeLegacySensitiveWords(legacyWords)
	if !legacyComplete {
		migrationComplete = false
	}
	if globalCount == 0 && len(legacyWords) > 0 {
		if _, err := UpsertSensitiveWordRule(0, "旧配置导入", legacyWords, SensitiveWordScopeGlobal, nil, 0, nil); err != nil {
			return err
		}
	}

	var whitelist []SensitiveWordWhitelist
	if err := DB.Where("enabled = ?", true).Find(&whitelist).Error; err != nil {
		return err
	}
	for _, item := range whitelist {
		if err := DB.Model(&User{}).Where("id = ?", item.UserID).Update("sensitive_word_whitelist", true).Error; err != nil {
			return err
		}
	}
	if migrationComplete {
		if err := DB.Save(&Option{Key: sensitiveWordMigrationKey, Value: sensitiveWordMigrationValue}).Error; err != nil {
			return err
		}
	} else if err := DB.Where(&Option{Key: sensitiveWordMigrationKey}).Delete(&Option{}).Error; err != nil {
		return err
	}
	invalidateSensitiveWordRuntime()
	return nil
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
