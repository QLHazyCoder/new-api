package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAmountDiscountPolicyControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := model.DB
	originalDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalOptionMap := common.OptionMap
	originalDiscounts := operation_setting.GetAmountDiscountPolicy().Discounts
	originalGroups := operation_setting.GetAmountDiscountPolicy().EligibleGroups

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, originalLogDatabaseType)
	model.InitColumnNamesForTest()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}))
	model.DB = db
	common.OptionMap = map[string]string{}

	setting := operation_setting.GetPaymentSetting()
	setting.AmountDiscount = map[int]float64{200: 0.9}
	setting.AmountDiscountEligibleGroups = []string{"legacy"}
	require.NoError(t, operation_setting.RefreshAmountDiscountPolicy())

	users := []model.User{
		{Username: "enabled", Password: "password", AffCode: "enabled-code", Group: "default", Status: common.UserStatusEnabled},
		{Username: "disabled", Password: "password", AffCode: "disabled-code", Group: "friend", Status: common.UserStatusDisabled},
		{Username: "deleted", Password: "password", AffCode: "deleted-code", Group: "deleted", Status: common.UserStatusEnabled},
	}
	for index := range users {
		require.NoError(t, db.Create(&users[index]).Error)
	}
	require.NoError(t, db.Delete(&users[2]).Error)

	t.Cleanup(func() {
		model.DB = originalDB
		common.OptionMap = originalOptionMap
		common.SetDatabaseTypes(originalDatabaseType, originalLogDatabaseType)
		model.InitColumnNamesForTest()
		setting.AmountDiscount = originalDiscounts
		setting.AmountDiscountEligibleGroups = originalGroups
		require.NoError(t, operation_setting.RefreshAmountDiscountPolicy())
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestGetAmountDiscountGroupsReturnsOccupiedNamesWithoutCounts(t *testing.T) {
	setupAmountDiscountPolicyControllerTest(t)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	GetAmountDiscountGroups(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Groups []string `json:"groups"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, []string{"default", "friend"}, response.Data.Groups)
}

func TestUpdateAmountDiscountPolicyPersistsPolicyAndAllowsSelectedStaleGroup(t *testing.T) {
	db := setupAmountDiscountPolicyControllerTest(t)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/option/payment/amount-discount-policy",
		strings.NewReader(`{"amount_discount":{"500":0.98,"200":0.99},"eligible_groups":[" friend ","legacy","friend"]}`),
	)

	UpdateAmountDiscountPolicy(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success, recorder.Body.String())
	policy := operation_setting.GetAmountDiscountPolicy()
	assert.Equal(t, map[int]float64{200: 0.99, 500: 0.98}, policy.Discounts)
	assert.Equal(t, []string{"friend", "legacy"}, policy.EligibleGroups)

	var savedOptions []model.Option
	require.NoError(t, db.Order("key ASC").Find(&savedOptions).Error)
	require.Len(t, savedOptions, 2)
	assert.Equal(t, operation_setting.AmountDiscountOptionKey, savedOptions[0].Key)
	assert.Equal(t, operation_setting.AmountDiscountEligibleGroupsOptionKey, savedOptions[1].Key)
}

func TestUpdateAmountDiscountPolicyRejectsUnoccupiedNewGroup(t *testing.T) {
	setupAmountDiscountPolicyControllerTest(t)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/option/payment/amount-discount-policy",
		strings.NewReader(`{"amount_discount":{"200":0.99},"eligible_groups":["missing"]}`),
	)

	UpdateAmountDiscountPolicy(context)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "has no users")
	assert.Equal(t, []string{"legacy"}, operation_setting.GetAmountDiscountPolicy().EligibleGroups)
}

func TestUpdateOptionRejectsPartialAmountDiscountPolicyWrite(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/option/",
		strings.NewReader(`{"key":"payment_setting.amount_discount","value":"{\"200\":0.99}"}`),
	)

	UpdateOption(context)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "专用接口")
}
