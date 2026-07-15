package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
)

const (
	RegistrationGroupPolicyOptionKey = "RegistrationGroupPolicy"
	RegistrationSourcePassword       = "password"
	RegistrationSourceWeChat         = "wechat"
	defaultRegistrationGroup         = "default"
)

// RegistrationGroupPolicy controls the initial group assigned to newly
// registered users. Source keys are password, wechat, or oauth:<provider-slug>.
type RegistrationGroupPolicy struct {
	Enabled         bool              `json:"enabled"`
	DefaultGroup    string            `json:"default_group"`
	SourceOverrides map[string]string `json:"source_overrides"`
}

// OAuthRegistrationSource returns the stable source key used by OAuth routes.
// providerName is the registry name from /api/oauth/:provider, so custom OAuth
// providers naturally use their slug.
func OAuthRegistrationSource(providerName string) string {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	if providerName == "" {
		return "oauth"
	}
	return "oauth:" + providerName
}

// ResolveRegistrationGroup reads the current policy directly from the options
// table for each registration. Any missing, disabled, malformed, or unavailable
// configuration safely resolves to the built-in default group.
func ResolveRegistrationGroup(source string) string {
	policy, err := loadRegistrationGroupPolicy()
	if err != nil {
		common.SysError("failed to load registration group policy: " + err.Error())
		return defaultRegistrationGroup
	}
	if !policy.Enabled {
		return defaultRegistrationGroup
	}

	defaultGroup := strings.TrimSpace(policy.DefaultGroup)
	if !isValidRegistrationGroup(defaultGroup) {
		defaultGroup = defaultRegistrationGroup
	}
	if override, ok := policy.SourceOverrides[normalizeRegistrationSource(source)]; ok {
		override = strings.TrimSpace(override)
		if isValidRegistrationGroup(override) {
			return override
		}
		common.SysLog("registration group override is invalid, using policy default: " + override)
	}
	return defaultGroup
}

func loadRegistrationGroupPolicy() (RegistrationGroupPolicy, error) {
	policy := RegistrationGroupPolicy{}
	if DB == nil {
		return policy, errors.New("database is not initialized")
	}

	var option Option
	err := DB.Where(&Option{Key: RegistrationGroupPolicyOptionKey}).Take(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return policy, nil
	}
	if err != nil {
		return policy, err
	}
	if err := common.UnmarshalJsonStr(option.Value, &policy); err != nil {
		return RegistrationGroupPolicy{}, err
	}

	policy.DefaultGroup = strings.TrimSpace(policy.DefaultGroup)
	policy.SourceOverrides, err = normalizeRegistrationSourceOverrides(policy.SourceOverrides)
	if err != nil {
		return RegistrationGroupPolicy{}, err
	}
	return policy, nil
}

func normalizeRegistrationSource(source string) string {
	return strings.ToLower(strings.TrimSpace(source))
}

func normalizeRegistrationSourceOverrides(overrides map[string]string) (map[string]string, error) {
	if len(overrides) == 0 {
		return nil, nil
	}

	normalized := make(map[string]string, len(overrides))
	for rawSource, group := range overrides {
		source := normalizeRegistrationSource(rawSource)
		if source == "" {
			continue
		}
		if _, exists := normalized[source]; exists {
			return nil, fmt.Errorf("registration source overrides collide after normalization: %q", source)
		}
		normalized[source] = strings.TrimSpace(group)
	}
	return normalized, nil
}

func isValidRegistrationGroup(group string) bool {
	if group == defaultRegistrationGroup {
		return true
	}
	if group == "" {
		return false
	}
	if _, ok := ratio_setting.GetGroupRatioCopy()[group]; ok {
		return true
	}
	_, ok := ratio_setting.GetGroupGroupRatioCopy()[group]
	return ok
}
