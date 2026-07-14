package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRegistrationGroupPolicyTest(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := DB
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	originalGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	DB = db

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"friend":1,"vip":2}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"special":{"default":1}}`))

	t.Cleanup(func() {
		DB = originalDB
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func TestResolveRegistrationGroup(t *testing.T) {
	db := setupRegistrationGroupPolicyTest(t)

	tests := []struct {
		name   string
		policy *RegistrationGroupPolicy
		raw    string
		source string
		want   string
	}{
		{
			name:   "missing policy uses default",
			source: RegistrationSourcePassword,
			want:   defaultRegistrationGroup,
		},
		{
			name: "disabled policy uses default",
			policy: &RegistrationGroupPolicy{
				Enabled:      false,
				DefaultGroup: "friend",
			},
			source: RegistrationSourcePassword,
			want:   defaultRegistrationGroup,
		},
		{
			name: "enabled policy uses configured default group",
			policy: &RegistrationGroupPolicy{
				Enabled:      true,
				DefaultGroup: "friend",
			},
			source: RegistrationSourcePassword,
			want:   "friend",
		},
		{
			name: "source override wins",
			policy: &RegistrationGroupPolicy{
				Enabled:      true,
				DefaultGroup: "friend",
				SourceOverrides: map[string]string{
					"OAuth:GitHub": "vip",
				},
			},
			source: " oauth:github ",
			want:   "vip",
		},
		{
			name: "custom oauth slug uses override",
			policy: &RegistrationGroupPolicy{
				Enabled:      true,
				DefaultGroup: "friend",
				SourceOverrides: map[string]string{
					"oauth:company-sso": "vip",
				},
			},
			source: OAuthRegistrationSource("company-sso"),
			want:   "vip",
		},
		{
			name: "group group ratio top-level key is valid",
			policy: &RegistrationGroupPolicy{
				Enabled:      true,
				DefaultGroup: "special",
			},
			source: RegistrationSourceWeChat,
			want:   "special",
		},
		{
			name: "invalid override safely uses configured default",
			policy: &RegistrationGroupPolicy{
				Enabled:      true,
				DefaultGroup: "friend",
				SourceOverrides: map[string]string{
					RegistrationSourcePassword: "removed-group",
				},
			},
			source: RegistrationSourcePassword,
			want:   "friend",
		},
		{
			name: "valid override survives invalid configured default",
			policy: &RegistrationGroupPolicy{
				Enabled:      true,
				DefaultGroup: "removed-group",
				SourceOverrides: map[string]string{
					RegistrationSourcePassword: "vip",
				},
			},
			source: RegistrationSourcePassword,
			want:   "vip",
		},
		{
			name: "invalid configured default safely uses default",
			policy: &RegistrationGroupPolicy{
				Enabled:      true,
				DefaultGroup: "removed-group",
			},
			source: RegistrationSourcePassword,
			want:   defaultRegistrationGroup,
		},
		{
			name:   "malformed json safely uses default",
			raw:    `{"enabled":`,
			source: RegistrationSourcePassword,
			want:   defaultRegistrationGroup,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, db.Where(&Option{Key: RegistrationGroupPolicyOptionKey}).Delete(&Option{}).Error)
			if tt.policy != nil || tt.raw != "" {
				raw := tt.raw
				if tt.policy != nil {
					data, err := common.Marshal(tt.policy)
					require.NoError(t, err)
					raw = string(data)
				}
				require.NoError(t, db.Create(&Option{Key: RegistrationGroupPolicyOptionKey, Value: raw}).Error)
			}

			require.Equal(t, tt.want, ResolveRegistrationGroup(tt.source))
		})
	}
}

func TestResolveRegistrationGroupDatabaseUnavailable(t *testing.T) {
	originalDB := DB
	DB = nil
	t.Cleanup(func() { DB = originalDB })

	require.Equal(t, defaultRegistrationGroup, ResolveRegistrationGroup(RegistrationSourcePassword))
}

func TestResolveRegistrationGroupReadsLatestPolicyFromDatabase(t *testing.T) {
	db := setupRegistrationGroupPolicyTest(t)

	storePolicy := func(defaultGroup string) {
		data, err := common.Marshal(RegistrationGroupPolicy{
			Enabled:      true,
			DefaultGroup: defaultGroup,
		})
		require.NoError(t, err)
		require.NoError(t, db.Save(&Option{
			Key:   RegistrationGroupPolicyOptionKey,
			Value: string(data),
		}).Error)
	}

	storePolicy("friend")
	require.Equal(t, "friend", ResolveRegistrationGroup(RegistrationSourcePassword))

	storePolicy("vip")
	require.Equal(t, "vip", ResolveRegistrationGroup(RegistrationSourcePassword))
}

func TestOAuthRegistrationSource(t *testing.T) {
	require.Equal(t, "oauth:github", OAuthRegistrationSource(" GitHub "))
	require.Equal(t, "oauth:company-sso", OAuthRegistrationSource("company-sso"))
	require.Equal(t, "oauth", OAuthRegistrationSource("  "))
}

func TestDefaultRegistrationGroupIsAlwaysValid(t *testing.T) {
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	originalGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
	})

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	require.True(t, isValidRegistrationGroup(defaultRegistrationGroup))
}
