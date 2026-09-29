package model

import (
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// ResolveAbilityEndpointTypes describes routes available to this specific
// channel/model pair. Plugin protocols are resolved from the host's actual
// model-scoped routing index, not from the plugin's model list alone.
func ResolveAbilityEndpointTypes(ability AbilityWithChannel, generation *jsplugin.RoutingGeneration, advancedCustom *dto.AdvancedCustomConfig) []constant.EndpointType {
	if ability.ChannelType == constant.ChannelTypeAdvancedCustom && advancedCustom != nil {
		return advancedCustom.SupportedEndpointTypesForModel(ability.Model)
	}

	upstreamModel, _, err := common.ResolveModelMapping(ability.Model, ability.ChannelModelMapping)
	if err != nil {
		return nil
	}
	pluginEndpoints, pluginBound := resolvePluginEndpointTypes(ability, upstreamModel, generation)
	if pluginBound || ability.ChannelType == constant.ChannelTypeTaskPlugin {
		return pluginEndpoints
	}
	return common.GetEndpointTypesByChannelType(ability.ChannelType, ability.Model, ability.ChannelModelMapping)
}

func getPricingEndpointTypesForAbility(ability AbilityWithChannel, advancedCustomConfigs map[int]*dto.AdvancedCustomConfig) []constant.EndpointType {
	return ResolveAbilityEndpointTypes(ability, jsplugin.DefaultRegistry.Generation(), advancedCustomConfigs[ability.ChannelId])
}

func resolvePluginEndpointTypes(ability AbilityWithChannel, upstreamModel string, generation *jsplugin.RoutingGeneration) ([]constant.EndpointType, bool) {
	if generation == nil {
		return nil, false
	}
	declared, ok := generation.CanonicalModel(ability.Model)
	if !ok {
		declared, ok = generation.CanonicalModel(upstreamModel)
	}
	if !ok {
		return nil, false
	}
	settings := dto.ChannelSettings{}
	if ability.ChannelSetting != "" && common.UnmarshalJsonStr(ability.ChannelSetting, &settings) != nil {
		return nil, false
	}
	bindings := settings.TaskPluginBindings()
	eligible := make(map[string]bool)
	for _, plugin := range generation.PluginsByModel(declared) {
		switch ability.ChannelType {
		case constant.ChannelTypeTaskPlugin:
			eligible[plugin.Meta.Key] = settings.TaskPluginKey == plugin.Meta.Key
		case constant.ChannelTypeNewAPI:
			eligible[plugin.Meta.Key] = slices.Contains(bindings, plugin.Meta.Key) && plugin.Meta.SupportsUpstream(jsplugin.UpstreamKindNewAPI)
		default:
			eligible[plugin.Meta.Key] = slices.Contains(plugin.Meta.ChannelTypes, ability.ChannelType)
		}
	}
	endpoints := make([]constant.EndpointType, 0, 3)
	bound := false
	for _, allowed := range eligible {
		bound = bound || allowed
	}
	for _, protocol := range []struct {
		name     string
		path     string
		typeName constant.EndpointType
	}{
		{"openai_responses", "/v1/responses", constant.EndpointTypeOpenAIResponse},
		{"openai_video", "/v1/videos", constant.EndpointTypeOpenAIVideo},
		{jsplugin.ProtocolOpenAIImage, "/v1/images/generations", constant.EndpointTypeImageGeneration},
	} {
		for _, candidate := range generation.LookupEndpointCandidates("POST", protocol.path, declared) {
			if candidate.Plugin != nil && candidate.Protocol == protocol.name && eligible[candidate.Plugin.Meta.Key] {
				endpoints = append(endpoints, protocol.typeName)
				break
			}
		}
	}
	return endpoints, bound
}

// Public model metadata can name custom endpoints, but it cannot turn a
// plugin-only channel into an unrelated built-in chat or image route.
func pluginOnlyCatalogModel(abilities []AbilityWithChannel) bool {
	if len(abilities) == 0 {
		return false
	}
	for _, ability := range abilities {
		if ability.ChannelType != constant.ChannelTypeTaskPlugin {
			return false
		}
	}
	return true
}
