package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateSelfBranchesNeverOverwriteAccountingFields(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	cases := []struct {
		name string
		body string
	}{
		{
			name: "profile",
			body: `{"display_name":"profile-after","quota":0,"used_quota":0,"request_count":0}`,
		},
		{
			name: "sidebar",
			body: `{"sidebar_modules":"{\"chat\":{\"enabled\":true}}","quota":0,"used_quota":0,"request_count":0}`,
		},
		{
			name: "language",
			body: `{"language":"zh","quota":0,"used_quota":0,"request_count":0}`,
		},
	}

	for index, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			user := model.User{
				Username:     "self-update-" + testCase.name,
				Password:     "password",
				DisplayName:  "before",
				Status:       common.UserStatusEnabled,
				Group:        "default",
				Quota:        1000,
				UsedQuota:    20,
				RequestCount: 3,
			}
			require.NoError(t, db.Create(&user).Error)
			t.Cleanup(func() { _ = db.Unscoped().Delete(&model.User{}, user.Id).Error })

			debit := 100 + index*50
			require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
				"quota":         gorm.Expr("quota - ?", debit),
				"used_quota":    gorm.Expr("used_quota + ?", debit),
				"request_count": gorm.Expr("request_count + ?", 1),
			}).Error)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set("id", user.Id)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/api/user/self", strings.NewReader(testCase.body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			UpdateSelf(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success, recorder.Body.String())

			var got model.User
			require.NoError(t, db.First(&got, user.Id).Error)
			assert.Equal(t, 1000-debit, got.Quota)
			assert.Equal(t, 20+debit, got.UsedQuota)
			assert.Equal(t, 4, got.RequestCount)
			assert.Equal(t, common.UserStatusEnabled, got.Status)
			assert.Equal(t, "default", got.Group)
			if testCase.name == "profile" {
				assert.Equal(t, "profile-after", got.DisplayName)
			}
		})
	}
}
