package ratio_setting

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"

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

const AutoGroupTypeID = "__auto__"

var groupTypeOptionsMutex sync.RWMutex

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
	setting := GetGroupRatioSetting()
	groupTypeOptionsMutex.RLock()
	defer groupTypeOptionsMutex.RUnlock()
	return setting.GroupTypes.ReadAll()
}

func GetGroupTypeMapCopy() map[string]string {
	setting := GetGroupRatioSetting()
	groupTypeOptionsMutex.RLock()
	defer groupTypeOptionsMutex.RUnlock()
	return setting.GroupTypeMap.ReadAll()
}

func GetGroupTypeOptionsCopy() (map[string]GroupTypeInfo, map[string]string) {
	setting := GetGroupRatioSetting()
	groupTypeOptionsMutex.RLock()
	defer groupTypeOptionsMutex.RUnlock()
	return setting.GroupTypes.ReadAll(), setting.GroupTypeMap.ReadAll()
}

func GroupTypes2JSONString() string {
	setting := GetGroupRatioSetting()
	groupTypeOptionsMutex.RLock()
	defer groupTypeOptionsMutex.RUnlock()
	return setting.GroupTypes.MarshalJSONString()
}

func UpdateGroupTypesByJSONString(jsonStr string) error {
	if err := CheckGroupTypes(jsonStr); err != nil {
		return err
	}
	groupTypes := make(map[string]GroupTypeInfo)
	if err := common.Unmarshal([]byte(jsonStr), &groupTypes); err != nil {
		return err
	}
	setting := GetGroupRatioSetting()
	groupTypeOptionsMutex.Lock()
	defer groupTypeOptionsMutex.Unlock()
	setting.GroupTypes.Clear()
	setting.GroupTypes.AddAll(groupTypes)
	return nil
}

func GroupTypeMap2JSONString() string {
	setting := GetGroupRatioSetting()
	groupTypeOptionsMutex.RLock()
	defer groupTypeOptionsMutex.RUnlock()
	return setting.GroupTypeMap.MarshalJSONString()
}

func GroupTypeOptions2JSONString() (string, string) {
	setting := GetGroupRatioSetting()
	groupTypeOptionsMutex.RLock()
	defer groupTypeOptionsMutex.RUnlock()
	return setting.GroupTypes.MarshalJSONString(), setting.GroupTypeMap.MarshalJSONString()
}

func UpdateGroupTypeMapByJSONString(jsonStr string) error {
	if err := CheckGroupTypeMap(jsonStr); err != nil {
		return err
	}
	groupTypeMap := make(map[string]string)
	if err := common.Unmarshal([]byte(jsonStr), &groupTypeMap); err != nil {
		return err
	}
	setting := GetGroupRatioSetting()
	groupTypeOptionsMutex.Lock()
	defer groupTypeOptionsMutex.Unlock()
	setting.GroupTypeMap.Clear()
	setting.GroupTypeMap.AddAll(groupTypeMap)
	return nil
}

// UpdateGroupTypeOptionsByJSONString replaces the related maps as one runtime snapshot.
func UpdateGroupTypeOptionsByJSONString(groupTypesJSON, groupTypeMapJSON string) error {
	if err := CheckGroupTypeAssignments(groupTypesJSON, groupTypeMapJSON); err != nil {
		return err
	}
	groupTypes := make(map[string]GroupTypeInfo)
	if err := common.Unmarshal([]byte(groupTypesJSON), &groupTypes); err != nil {
		return err
	}
	groupTypeMap := make(map[string]string)
	if err := common.Unmarshal([]byte(groupTypeMapJSON), &groupTypeMap); err != nil {
		return err
	}
	setting := GetGroupRatioSetting()
	groupTypeOptionsMutex.Lock()
	defer groupTypeOptionsMutex.Unlock()
	setting.GroupTypes.Clear()
	setting.GroupTypes.AddAll(groupTypes)
	setting.GroupTypeMap.Clear()
	setting.GroupTypeMap.AddAll(groupTypeMap)
	return nil
}

func CheckGroupTypes(jsonStr string) error {
	groupTypes := make(map[string]GroupTypeInfo)
	if err := common.Unmarshal([]byte(jsonStr), &groupTypes); err != nil {
		return err
	}
	if groupTypes == nil {
		return common.NewMessage("Group types must be a JSON object")
	}
	names := make(map[string]struct{}, len(groupTypes))
	for typeID, typeInfo := range groupTypes {
		if strings.TrimSpace(typeID) == "" || typeID != strings.TrimSpace(typeID) {
			return common.NewMessage("Group type ID must be non-empty and trimmed")
		}
		if typeID == AutoGroupTypeID {
			return common.NewMessage("Group type ID is reserved for automatic routing")
		}
		name := strings.TrimSpace(typeInfo.Name)
		if name == "" {
			return common.NewMessage("Group type name must not be empty: {{type}}", map[string]any{"type": typeID})
		}
		nameKey := strings.ToLower(name)
		if _, ok := names[nameKey]; ok {
			return common.NewMessage("Group type name must be unique: {{name}}", map[string]any{"name": name})
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
	if groupTypeMap == nil {
		return common.NewMessage("Group type assignments must be a JSON object")
	}
	for groupName, typeID := range groupTypeMap {
		if strings.TrimSpace(groupName) == "" || groupName != strings.TrimSpace(groupName) {
			return common.NewMessage("Group name must be non-empty and trimmed")
		}
		if groupName == "auto" {
			return common.NewMessage("Automatic routing cannot be assigned a group type")
		}
		if strings.TrimSpace(typeID) == "" || typeID != strings.TrimSpace(typeID) {
			return common.NewMessage("Group type ID must be non-empty and trimmed: {{group}}", map[string]any{"group": groupName})
		}
	}
	return nil
}

func CheckGroupTypeAssignments(groupTypesJSON, groupTypeMapJSON string) error {
	if err := CheckGroupTypes(groupTypesJSON); err != nil {
		return err
	}
	if err := CheckGroupTypeMap(groupTypeMapJSON); err != nil {
		return err
	}
	groupTypes := make(map[string]GroupTypeInfo)
	if err := common.Unmarshal([]byte(groupTypesJSON), &groupTypes); err != nil {
		return err
	}
	assignments := make(map[string]string)
	if err := common.Unmarshal([]byte(groupTypeMapJSON), &assignments); err != nil {
		return err
	}
	for groupName, typeID := range assignments {
		if _, ok := groupTypes[typeID]; !ok {
			return common.NewMessage("Group type {{type}} does not exist (group {{group}})", map[string]any{"type": typeID, "group": groupName})
		}
	}
	return nil
}

func CheckGroupTypeOptions(groupTypesJSON, groupTypeMapJSON string, groupRatios map[string]float64) error {
	if err := CheckGroupTypeAssignments(groupTypesJSON, groupTypeMapJSON); err != nil {
		return err
	}
	assignments := make(map[string]string)
	if err := common.Unmarshal([]byte(groupTypeMapJSON), &assignments); err != nil {
		return err
	}
	for groupName := range assignments {
		if _, ok := groupRatios[groupName]; !ok {
			return common.NewMessage("Group {{group}} does not exist in group ratios", map[string]any{"group": groupName})
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
