package service

import (
	"slices"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/imagecapability"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
)

func GetUserImageModelGroups(userGroup string) ([]dto.UserImageGroupOption, error) {
	usableGroups := GetUserUsableGroups(userGroup)
	autoGroups := GetUserAutoGroup(userGroup)
	queryGroupSet := make(map[string]bool)
	for group := range usableGroups {
		if group != "auto" {
			queryGroupSet[group] = true
		}
	}
	for _, group := range autoGroups {
		queryGroupSet[group] = true
	}
	queryGroups := sortedGroupNames(queryGroupSet)

	abilities, err := model.GetEnabledAbilitiesWithChannelsByGroups(queryGroups)
	if err != nil {
		return nil, err
	}

	groupNames := make([]string, 0, len(usableGroups))
	for group := range usableGroups {
		groupNames = append(groupNames, group)
	}
	sort.Strings(groupNames)

	options := make([]dto.UserImageGroupOption, 0, len(groupNames))
	for _, group := range groupNames {
		includedGroups := map[string]bool{group: true}
		ratio := any(GetUserGroupRatio(userGroup, group))
		if group == "auto" {
			includedGroups = make(map[string]bool, len(autoGroups))
			for _, autoGroup := range autoGroups {
				includedGroups[autoGroup] = true
			}
			ratio = "自动"
		}

		models := buildImageModelOptions(abilities, includedGroups)
		if len(models) == 0 {
			continue
		}
		options = append(options, dto.UserImageGroupOption{
			Label:  group,
			Value:  group,
			Ratio:  ratio,
			Desc:   usableGroups[group],
			Models: models,
		})
	}
	return options, nil
}

func buildImageModelOptions(abilities []model.AbilityWithChannel, includedGroups map[string]bool) []dto.UserImageModelOption {
	type aggregate struct {
		capability  imagecapability.Capability
		initialized bool
	}

	models := make(map[string]aggregate)
	generation := jsplugin.DefaultRegistry.Generation()
	for _, ability := range abilities {
		if !includedGroups[ability.Group] {
			continue
		}
		if !slices.Contains(model.ResolveAbilityEndpointTypes(ability, generation, nil), constant.EndpointTypeImageGeneration) {
			continue
		}
		capability, ok := common.ResolveChannelImageCapability(ability.ChannelType, ability.Model, ability.ChannelModelMapping)
		if !ok {
			continue
		}

		current := models[ability.Model]
		if current.initialized {
			current.capability = imagecapability.Intersect(current.capability, capability)
		} else {
			current.capability = capability
			current.initialized = true
		}
		models[ability.Model] = current
	}

	modelNames := make([]string, 0, len(models))
	for modelName := range models {
		modelNames = append(modelNames, modelName)
	}
	sort.Strings(modelNames)

	options := make([]dto.UserImageModelOption, 0, len(modelNames))
	for _, modelName := range modelNames {
		capability := models[modelName].capability
		options = append(options, dto.UserImageModelOption{
			Label:        modelName,
			Value:        modelName,
			Capabilities: dto.ImageCapabilitiesFrom(capability),
		})
	}
	return options
}

func sortedGroupNames(groups map[string]bool) []string {
	names := make([]string, 0, len(groups))
	for group := range groups {
		names = append(names, group)
	}
	sort.Strings(names)
	return names
}
