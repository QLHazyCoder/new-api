package router

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accessTokenExemptRoutes are /api routes that never reach dashboard
// authentication, so a personal access token has no rule to satisfy there.
var accessTokenExemptRoutes = []string{
	// Anonymous pages and setup.
	"GET /api/setup",
	"POST /api/setup",
	"GET /api/status",
	"GET /api/uptime/status",
	"GET /api/notice",
	"GET /api/user-agreement",
	"GET /api/privacy-policy",
	"GET /api/about",
	"GET /api/home_page_content",
	"GET /api/ratio_config",
	"GET /api/user/groups",
	// Signed image content validates its short-lived signature in the handler.
	"GET /api/playground/image-tasks/:id/content",

	// Anonymous account entry points.
	"GET /api/verification",
	"GET /api/reset_password",
	"POST /api/user/reset",
	"POST /api/user/register",
	"GET /api/user/login/encryption-key",
	"POST /api/user/login",
	"POST /api/user/login/2fa",
	"POST /api/user/login/verify",
	"POST /api/user/login/passkey/begin",
	"POST /api/user/login/passkey/finish",
	"POST /api/user/passkey/login/begin",
	"POST /api/user/passkey/login/finish",
	"GET /api/oauth/wechat",
	"GET /api/oauth/telegram/login",
	"GET /api/oauth/telegram/bind/:flow_token",

	// Browser session cookie endpoints.
	"POST /api/user/auth/refresh",
	"POST /api/user/auth/logout",

	// Payment callbacks.
	"POST /api/stripe/webhook",
	"POST /api/creem/webhook",
	"POST /api/waffo/webhook",
	"POST /api/waffo-pancake/webhook/:env",
	"GET /api/user/epay/notify",
	"POST /api/user/epay/notify",
	"GET /api/subscription/epay/notify",
	"POST /api/subscription/epay/notify",
	"GET /api/subscription/epay/return",
	"POST /api/subscription/epay/return",

	// API key authentication (TokenAuthReadOnly).
	"GET /api/usage/token/",
	"GET /api/log/token",
}

// accessTokenHelperRoutes are declared through handlePermissionRoute outside
// channelPermissionRoutes.
var accessTokenHelperRoutes = map[string]authz.Permission{
	"GET /api/task_plugin_options":          authz.TaskPluginBind,
	"GET /api/audit":                        authz.AuditRead,
	"GET /api/log/sensitive-word-audit/:id": authz.AuditRead,
}

func newAccessTokenScopeTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// A permission route that is also in the static table panics here.
	require.NotPanics(t, func() {
		SetApiRouter(engine)
		SetDashboardRouter(engine)
		SetRelayRouter(engine)
		SetTaskPluginProtocolRouter(engine)
		SetVideoRouter(engine)
		SetTaskRouter(engine)
	})
	return engine
}

func TestAccessTokenRouteRulesCoverEveryDashboardRoute(t *testing.T) {
	engine := newAccessTokenScopeTestEngine(t)
	exempt := map[string]bool{}
	for _, key := range accessTokenExemptRoutes {
		exempt[key] = true
	}
	registered := map[string]bool{}
	panel := map[string]bool{}
	for _, route := range engine.Routes() {
		if !strings.HasPrefix(route.Path, "/api/") && !strings.HasPrefix(route.Path, "/pg/") {
			continue
		}
		key := route.Method + " " + route.Path
		registered[key] = true
		if !exempt[key] {
			panel[key] = true
		}
	}
	for _, key := range accessTokenExemptRoutes {
		assert.True(t, registered[key], "exempt route %s is no longer registered", key)
	}

	declared := map[string]int{}
	for _, key := range middleware.AccessTokenRouteRuleKeys() {
		declared[key]++
	}
	for key := range panel {
		assert.Equal(t, 1, declared[key], "panel route %s must be declared exactly once", key)
	}
	for key := range declared {
		assert.True(t, panel[key], "declared route %s is not a registered panel route", key)
	}

	permissionRoutes := map[string]authz.Permission{}
	for _, route := range channelPermissionRoutes {
		permissionRoutes[route.method+" "+joinPaths("/api/channel", route.path)] = route.permission
	}
	for key, permission := range accessTokenHelperRoutes {
		permissionRoutes[key] = permission
	}
	for key, permission := range permissionRoutes {
		rule, ok := middleware.AccessTokenRouteRule(key)
		require.True(t, ok, key)
		assert.Equal(t, "scope", rule.Kind(), key)
		assert.Equal(t, service.AccessTokenScopeOf(permission), rule.Scope(), key)
	}
}

func TestAccessTokenCatalogCoversEveryPermission(t *testing.T) {
	catalog := service.AccessTokenCatalog(1, common.RoleRootUser)
	scopes := map[string]bool{}
	resources := map[string]int{}
	for _, group := range catalog {
		for _, resource := range group.Resources {
			resources[resource.Resource]++
			for _, action := range resource.Actions {
				scopes[resource.Resource+":"+action.Action] = true
			}
		}
	}
	for _, permission := range authz.AllPermissions() {
		assert.True(t, scopes[service.AccessTokenScopeOf(permission)], "root catalog lacks %s", service.AccessTokenScopeOf(permission))
	}
	for resource, count := range resources {
		assert.Equal(t, 1, count, "resource %s appears in more than one catalog entry", resource)
	}
}

func TestAccessTokenRulesPreserveCustomFeatures(t *testing.T) {
	newAccessTokenScopeTestEngine(t)
	for _, test := range []struct {
		key, kind, scope string
	}{
		{"GET /api/user/image-models", "scope", "profile:read"},
		{"GET /api/option/payment/amount-discount-groups", "scope", "option:read"},
		{"PUT /api/option/payment/amount-discount-policy", "scope", "option:write"},
		{"GET /api/sensitive-words/policy", "scope", "sensitive_word:read"},
		{"GET /api/sensitive-words/groups", "scope", "sensitive_word:read"},
		{"GET /api/sensitive-words/rules", "scope", "sensitive_word:read"},
		{"GET /api/sensitive-words/rules/:id", "scope", "sensitive_word:read"},
		{"PUT /api/sensitive-words/policy", "scope", "sensitive_word:write"},
		{"POST /api/sensitive-words/rules", "scope", "sensitive_word:write"},
		{"PUT /api/sensitive-words/rules/:id", "scope", "sensitive_word:write"},
		{"DELETE /api/sensitive-words/rules/:id", "scope", "sensitive_word:write"},
		{"PATCH /api/sensitive-words/rules/:id/mode", "scope", "sensitive_word:write"},
		{"GET /api/log/sensitive-word-audit/:id", "scope", "audit:read"},
		{"GET /api/playground/image-batches/:id", "scope", "playground:read"},
		{"GET /api/playground/image-tasks", "scope", "playground:read"},
		{"POST /api/playground/image-batches/generations", "scope", "playground:write"},
		{"POST /api/playground/image-batches/edits", "scope", "playground:write"},
		{"POST /api/playground/image-tasks/:id/retry", "scope", "playground:write"},
		{"DELETE /api/playground/image-tasks/:id", "scope", "playground:write"},
		{"POST /pg/chat/completions", "session", ""},
		{"POST /pg/images/generations", "session", ""},
		{"POST /pg/images/edits", "session", ""},
	} {
		t.Run(test.key, func(t *testing.T) {
			rule, ok := middleware.AccessTokenRouteRule(test.key)
			require.True(t, ok)
			assert.Equal(t, test.kind, rule.Kind())
			assert.Equal(t, test.scope, rule.Scope())
		})
	}
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		scopes, err := service.NormalizeAccessTokenScopes(1, role, []string{"playground:write", "playground:read"})
		require.NoError(t, err)
		assert.Equal(t, []string{"playground:read", "playground:write"}, scopes)
		_, err = service.NormalizeAccessTokenScopes(1, role, []string{"sensitive_word:read", "sensitive_word:write"})
		if role == common.RoleCommonUser {
			assert.ErrorIs(t, err, service.ErrAccessTokenScopeForbidden)
		} else {
			require.NoError(t, err)
		}
	}
}

func TestAccessTokenVerificationScopesAreAllListed(t *testing.T) {
	files, err := filepath.Glob("../service/*.go")
	require.NoError(t, err)
	fileSet := token.NewFileSet()
	var scopes []string
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fileSet, file, nil, 0)
		require.NoError(t, err)
		for _, declaration := range parsed.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, spec := range general.Specs {
				value := spec.(*ast.ValueSpec)
				for i, name := range value.Names {
					if !strings.HasPrefix(name.Name, "VerificationScope") || i >= len(value.Values) {
						continue
					}
					literal, ok := value.Values[i].(*ast.BasicLit)
					require.True(t, ok, name.Name)
					scope, err := strconv.Unquote(literal.Value)
					require.NoError(t, err)
					scopes = append(scopes, scope)
				}
			}
		}
	}
	require.NotEmpty(t, scopes)
	for _, scope := range scopes {
		_, listed := service.AccessTokenVerificationScope(scope)
		assert.True(t, listed, "verification scope %s has no access token rule", scope)
	}
}
