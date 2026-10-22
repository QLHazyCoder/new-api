package ratio_setting

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

var defaultGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

var groupRatioMap = types.NewRWMap[string, float64]()

var defaultGroupGroupRatio = map[string]map[string]float64{
	"vip": {
		"edit_this": 0.9,
	},
}

var groupGroupRatioMap = types.NewRWMap[string, map[string]float64]()

var groupDescriptionsMap = types.NewRWMap[string, string]()

var groupTypesMap = types.NewRWMap[string, GroupTypeInfo]()

var groupTypeMap = types.NewRWMap[string, string]()

var defaultGroupSpecialUsableGroup = map[string]map[string]string{}

type GroupTypeInfo struct {
	Name  string `json:"name"`
	Icon  string `json:"icon,omitempty"`
	Order int    `json:"order"`
}

type GroupRatioSetting struct {
	GroupRatio              *types.RWMap[string, float64]            `json:"group_ratio"`
	GroupGroupRatio         *types.RWMap[string, map[string]float64] `json:"group_group_ratio"`
	GroupDescriptions       *types.RWMap[string, string]             `json:"group_descriptions"`
	GroupSpecialUsableGroup *types.RWMap[string, map[string]string]  `json:"group_special_usable_group"`
	GroupTypes              *types.RWMap[string, GroupTypeInfo]      `json:"group_types"`
	GroupTypeMap            *types.RWMap[string, string]             `json:"group_type_map"`
}

var groupRatioSetting GroupRatioSetting

func init() {
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)

	groupRatioMap.AddAll(defaultGroupRatio)
	groupGroupRatioMap.AddAll(defaultGroupGroupRatio)

	groupRatioSetting = GroupRatioSetting{
		GroupSpecialUsableGroup: groupSpecialUsableGroup,
		GroupRatio:              groupRatioMap,
		GroupGroupRatio:         groupGroupRatioMap,
		GroupDescriptions:       groupDescriptionsMap,
		GroupTypes:              groupTypesMap,
		GroupTypeMap:            groupTypeMap,
	}

	config.GlobalConfig.Register("group_ratio_setting", &groupRatioSetting)
}

func GetGroupRatioSetting() *GroupRatioSetting {
	if groupRatioSetting.GroupSpecialUsableGroup == nil {
		groupRatioSetting.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
		groupRatioSetting.GroupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)
	}
	if groupRatioSetting.GroupDescriptions == nil {
		groupRatioSetting.GroupDescriptions = types.NewRWMap[string, string]()
	}
	if groupRatioSetting.GroupTypes == nil {
		groupRatioSetting.GroupTypes = types.NewRWMap[string, GroupTypeInfo]()
	}
	if groupRatioSetting.GroupTypeMap == nil {
		groupRatioSetting.GroupTypeMap = types.NewRWMap[string, string]()
	}
	return &groupRatioSetting
}

func GetGroupRatioCopy() map[string]float64 {
	return groupRatioMap.ReadAll()
}

func GetGroupDescription(name string) (string, bool) {
	return GetGroupRatioSetting().GroupDescriptions.Get(name)
}

func GetGroupDescriptionsCopy() map[string]string {
	return GetGroupRatioSetting().GroupDescriptions.ReadAll()
}

func GetGroupTypesCopy() map[string]GroupTypeInfo {
	return GetGroupRatioSetting().GroupTypes.ReadAll()
}

func GetGroupTypeMapCopy() map[string]string {
	return GetGroupRatioSetting().GroupTypeMap.ReadAll()
}

func GroupTypes2JSONString() string {
	return GetGroupRatioSetting().GroupTypes.MarshalJSONString()
}

func UpdateGroupTypesByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(GetGroupRatioSetting().GroupTypes, jsonStr)
}

func GroupTypeMap2JSONString() string {
	return GetGroupRatioSetting().GroupTypeMap.MarshalJSONString()
}

func UpdateGroupTypeMapByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(GetGroupRatioSetting().GroupTypeMap, jsonStr)
}

func CheckGroupTypes(jsonStr string) error {
	groupTypes := make(map[string]GroupTypeInfo)
	if err := common.Unmarshal([]byte(jsonStr), &groupTypes); err != nil {
		return err
	}
	names := make(map[string]struct{}, len(groupTypes))
	for typeID, typeInfo := range groupTypes {
		if strings.TrimSpace(typeID) == "" {
			return errors.New("group type id must not be empty")
		}
		name := strings.TrimSpace(typeInfo.Name)
		if name == "" {
			return errors.New("group type name must not be empty: " + typeID)
		}
		nameKey := strings.ToLower(name)
		if _, ok := names[nameKey]; ok {
			return errors.New("group type name must be unique: " + name)
		}
		names[nameKey] = struct{}{}
	}
	return nil
}

func CheckGroupTypeMap(jsonStr string) error {
	groupTypeMap := make(map[string]string)
	if err := common.Unmarshal([]byte(jsonStr), &groupTypeMap); err != nil {
		return err
	}
	for groupName, typeID := range groupTypeMap {
		if strings.TrimSpace(groupName) == "" {
			return errors.New("group name must not be empty")
		}
		if strings.TrimSpace(typeID) == "" {
			return errors.New("group type id must not be empty: " + groupName)
		}
	}
	return nil
}

func ContainsGroupRatio(name string) bool {
	_, ok := groupRatioMap.Get(name)
	return ok
}

func GroupRatio2JSONString() string {
	return groupRatioMap.MarshalJSONString()
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupRatioMap, jsonStr)
}

func GetGroupRatio(name string) float64 {
	ratio, ok := groupRatioMap.Get(name)
	if !ok {
		common.SysLog(common.LogText("group ratio not found: %s", name))
		return 1
	}
	return ratio
}

func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	gp, ok := groupGroupRatioMap.Get(userGroup)
	if !ok {
		return -1, false
	}
	ratio, ok := gp[usingGroup]
	if !ok {
		return -1, false
	}
	return ratio, true
}

func GroupGroupRatio2JSONString() string {
	return groupGroupRatioMap.MarshalJSONString()
}

func GetGroupGroupRatioCopy() map[string]map[string]float64 {
	return groupGroupRatioMap.ReadAll()
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupGroupRatioMap, jsonStr)
}

func CheckGroupRatio(jsonStr string) error {
	checkGroupRatio := make(map[string]float64)
	err := json.Unmarshal([]byte(jsonStr), &checkGroupRatio)
	if err != nil {
		return err
	}
	for name, ratio := range checkGroupRatio {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + name)
		}
	}
	return nil
}
