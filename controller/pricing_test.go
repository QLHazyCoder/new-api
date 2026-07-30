package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterPricingByUsableGroupsRedactsHiddenGroups(t *testing.T) {
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalSpecialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
		specialGroups.Clear()
		specialGroups.AddAll(originalSpecialGroups)
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"public":"Public group"}`))
	specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	specialGroups.Clear()

	pricing := []model.Pricing{
		{ModelName: "shared-model", EnableGroup: []string{"public", "member", "private"}},
		{ModelName: "private-model", EnableGroup: []string{"private"}},
		{ModelName: "all-model", EnableGroup: []string{"all", "private"}},
	}

	usableGroups := service.GetUserUsableGroups("member")
	require.Len(t, usableGroups, 2)
	assert.Contains(t, usableGroups, "public")
	assert.Contains(t, usableGroups, "member")
	filtered := filterPricingByUsableGroups(pricing, usableGroups)

	require.Len(t, filtered, 2)
	assert.Equal(t, "shared-model", filtered[0].ModelName)
	assert.Equal(t, []string{"public", "member"}, filtered[0].EnableGroup)
	assert.Equal(t, "all-model", filtered[1].ModelName)
	assert.Equal(t, []string{"all"}, filtered[1].EnableGroup)
	assert.Equal(t, []string{"public", "member", "private"}, pricing[0].EnableGroup)
	assert.Equal(t, []string{"all", "private"}, pricing[2].EnableGroup)
}

func TestFilterPricingByUsableGroupsRejectsEmptyVisibility(t *testing.T) {
	pricing := []model.Pricing{
		{ModelName: "all-model", EnableGroup: []string{"all"}},
	}

	filtered := filterPricingByUsableGroups(pricing, map[string]string{})

	assert.Empty(t, filtered)
}
