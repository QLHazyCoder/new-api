package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type registrationGroupTestProvider struct {
	existingUser *model.User
}

func (p *registrationGroupTestProvider) GetName() string { return "GitHub" }
func (p *registrationGroupTestProvider) IsEnabled() bool { return true }
func (p *registrationGroupTestProvider) ExchangeToken(context.Context, string, *gin.Context) (*oauth.OAuthToken, error) {
	return nil, nil
}
func (p *registrationGroupTestProvider) GetUserInfo(context.Context, *oauth.OAuthToken) (*oauth.OAuthUser, error) {
	return nil, nil
}
func (p *registrationGroupTestProvider) IsUserIDTaken(string) bool {
	return p.existingUser != nil
}
func (p *registrationGroupTestProvider) FillUserByProviderID(user *model.User, _ string) error {
	*user = *p.existingUser
	return nil
}
func (p *registrationGroupTestProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.GitHubId = providerUserID
}
func (p *registrationGroupTestProvider) GetProviderPrefix() string { return "github_" }

func setupRegistrationGroupControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalRegisterEnabled := common.RegisterEnabled
	originalPasswordRegisterEnabled := common.PasswordRegisterEnabled
	originalEmailVerificationEnabled := common.EmailVerificationEnabled
	originalWeChatAuthEnabled := common.WeChatAuthEnabled
	originalWeChatServerAddress := common.WeChatServerAddress
	originalWeChatServerToken := common.WeChatServerToken
	originalQuotaForNewUser := common.QuotaForNewUser
	originalQuotaForInviter := common.QuotaForInviter
	originalQuotaForInvitee := common.QuotaForInvitee
	originalGenerateDefaultToken := constant.GenerateDefaultToken
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	originalGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	common.WeChatAuthEnabled = true
	common.QuotaForNewUser = 0
	common.QuotaForInviter = 0
	common.QuotaForInvitee = 0
	constant.GenerateDefaultToken = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}, &model.Log{}))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"friend":1,"vip":2}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		common.RegisterEnabled = originalRegisterEnabled
		common.PasswordRegisterEnabled = originalPasswordRegisterEnabled
		common.EmailVerificationEnabled = originalEmailVerificationEnabled
		common.WeChatAuthEnabled = originalWeChatAuthEnabled
		common.WeChatServerAddress = originalWeChatServerAddress
		common.WeChatServerToken = originalWeChatServerToken
		common.QuotaForNewUser = originalQuotaForNewUser
		common.QuotaForInviter = originalQuotaForInviter
		common.QuotaForInvitee = originalQuotaForInvitee
		constant.GenerateDefaultToken = originalGenerateDefaultToken
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func storeRegistrationGroupPolicy(t *testing.T, db *gorm.DB, policy model.RegistrationGroupPolicy) {
	t.Helper()
	data, err := common.Marshal(policy)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Option{
		Key:   model.RegistrationGroupPolicyOptionKey,
		Value: string(data),
	}).Error)
}

func TestPasswordRegistrationAppliesRegistrationGroupPolicy(t *testing.T) {
	db := setupRegistrationGroupControllerTestDB(t)
	storeRegistrationGroupPolicy(t, db, model.RegistrationGroupPolicy{
		Enabled:      true,
		DefaultGroup: "friend",
	})

	router := gin.New()
	router.POST("/api/user/register", Register)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(`{"username":"policy_user","password":"password123"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var created model.User
	require.NoError(t, db.Where("username = ?", "policy_user").First(&created).Error)
	require.Equal(t, "friend", created.Group)
}

func TestWeChatRegistrationAppliesSourceOverride(t *testing.T) {
	db := setupRegistrationGroupControllerTestDB(t)
	storeRegistrationGroupPolicy(t, db, model.RegistrationGroupPolicy{
		Enabled:      true,
		DefaultGroup: "friend",
		SourceOverrides: map[string]string{
			model.RegistrationSourceWeChat: "vip",
		},
	})

	wechatServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"message":"","data":"wechat-policy-user"}`))
	}))
	defer wechatServer.Close()
	common.WeChatServerAddress = wechatServer.URL
	common.WeChatServerToken = "test-token"

	router := gin.New()
	router.Use(sessions.Sessions("registration-group-test", cookie.NewStore([]byte("test-secret"))))
	router.GET("/api/oauth/wechat", WeChatAuth)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/oauth/wechat?code=test-code", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var created model.User
	require.NoError(t, db.Where("wechat_id = ?", "wechat-policy-user").First(&created).Error)
	require.Equal(t, "vip", created.Group)
}

func TestOAuthRegistrationAppliesProviderOverride(t *testing.T) {
	db := setupRegistrationGroupControllerTestDB(t)
	storeRegistrationGroupPolicy(t, db, model.RegistrationGroupPolicy{
		Enabled:      true,
		DefaultGroup: "friend",
		SourceOverrides: map[string]string{
			"oauth:github": "vip",
		},
	})

	provider := &registrationGroupTestProvider{}
	oauthUser := &oauth.OAuthUser{ProviderUserID: "github-policy-user", Username: "oauth_policy_user"}

	router := gin.New()
	router.Use(sessions.Sessions("registration-group-test", cookie.NewStore([]byte("test-secret"))))
	router.GET("/oauth-contract", func(c *gin.Context) {
		user, err := findOrCreateOAuthUser(c, "github", provider, oauthUser, sessions.Default(c))
		require.NoError(t, err)
		require.Equal(t, "vip", user.Group)
		c.Status(http.StatusNoContent)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/oauth-contract", nil))

	require.Equal(t, http.StatusNoContent, recorder.Code)
	var created model.User
	require.NoError(t, db.Where("github_id = ?", "github-policy-user").First(&created).Error)
	require.Equal(t, "vip", created.Group)
}

func TestExistingOAuthLoginKeepsCurrentGroup(t *testing.T) {
	db := setupRegistrationGroupControllerTestDB(t)
	storeRegistrationGroupPolicy(t, db, model.RegistrationGroupPolicy{
		Enabled:      true,
		DefaultGroup: "friend",
	})

	existing := &model.User{
		Username:    "existing_oauth_user",
		DisplayName: "Existing OAuth User",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "legacy-group",
		GitHubId:    "existing-provider-id",
	}
	require.NoError(t, existing.Insert(0))
	provider := &registrationGroupTestProvider{existingUser: existing}

	router := gin.New()
	router.Use(sessions.Sessions("registration-group-test", cookie.NewStore([]byte("test-secret"))))
	router.GET("/oauth-existing-contract", func(c *gin.Context) {
		user, err := findOrCreateOAuthUser(c, "github", provider, &oauth.OAuthUser{ProviderUserID: existing.GitHubId}, sessions.Default(c))
		require.NoError(t, err)
		require.Equal(t, "legacy-group", user.Group)
		c.Status(http.StatusNoContent)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/oauth-existing-contract", nil))

	require.Equal(t, http.StatusNoContent, recorder.Code)
	var persisted model.User
	require.NoError(t, db.First(&persisted, existing.Id).Error)
	require.Equal(t, "legacy-group", persisted.Group)
}

func TestDirectUserCreationKeepsExplicitGroup(t *testing.T) {
	db := setupRegistrationGroupControllerTestDB(t)
	storeRegistrationGroupPolicy(t, db, model.RegistrationGroupPolicy{
		Enabled:      true,
		DefaultGroup: "friend",
	})

	user := &model.User{
		Username:    "admin_created_user",
		DisplayName: "Admin Created User",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "admin-selected-group",
	}
	require.NoError(t, user.Insert(0))

	var persisted model.User
	require.NoError(t, db.First(&persisted, user.Id).Error)
	require.Equal(t, "admin-selected-group", persisted.Group)
}
