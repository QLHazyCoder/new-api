package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// MigrateLegacyPublicVideoEndpoints only changes catalog metadata. Plugin
// protocol names and billing options are deliberately outside its scope.
func MigrateLegacyPublicVideoEndpoints() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		pluginModels := tx.Table("abilities").
			Select("abilities.model").
			Joins("join channels on abilities.channel_id = channels.id").
			Where("abilities.enabled = ? AND channels.status = ? AND channels.type = ?", true, common.ChannelStatusEnabled, constant.ChannelTypeTaskPlugin)
		var records []Model
		if err := tx.Where("endpoints LIKE ? OR (name_rule = ? AND endpoints LIKE ? AND model_name IN (?))", "%openai_video%", NameRuleExact, `%"openai"%`, pluginModels).Find(&records).Error; err != nil {
			return err
		}
		for _, record := range records {
			pluginOnly, err := hasOnlyPluginChannels(tx, record)
			if err != nil {
				return err
			}
			normalized, changed, conflict, err := migratePublicVideoEndpoint(record.Endpoints, pluginOnly)
			if err != nil {
				common.SysLog(fmt.Sprintf("skip malformed model %d endpoint metadata during video endpoint migration: %v", record.Id, err))
				continue
			}
			if !changed {
				continue
			}
			if conflict {
				common.SysLog(fmt.Sprintf("model %d has both video endpoint keys; retaining the canonical openai-video definition", record.Id))
			}
			if err := tx.Model(&Model{}).Where("id = ?", record.Id).UpdateColumn("endpoints", normalized).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func hasOnlyPluginChannels(tx *gorm.DB, record Model) (bool, error) {
	if record.NameRule != NameRuleExact || !strings.Contains(record.Endpoints, `"openai"`) {
		return false, nil
	}
	var channelTypes []int
	err := tx.Table("abilities").
		Select("channels.type").
		Joins("join channels on abilities.channel_id = channels.id").
		Where("abilities.model = ? AND abilities.enabled = ? AND channels.status = ?", record.ModelName, true, common.ChannelStatusEnabled).
		Scan(&channelTypes).Error
	if err != nil || len(channelTypes) == 0 {
		return false, err
	}
	for _, channelType := range channelTypes {
		if channelType != constant.ChannelTypeTaskPlugin {
			return false, nil
		}
	}
	return true, nil
}

func migratePublicVideoEndpoint(raw string, pluginOnly bool) (normalized string, changed, conflict bool, err error) {
	if !strings.Contains(raw, "openai_video") && !pluginOnly {
		return raw, false, false, nil
	}
	var value any
	if err = common.UnmarshalJsonStr(raw, &value); err != nil {
		return raw, false, false, err
	}
	switch endpoints := value.(type) {
	case map[string]any:
		if legacy, exists := endpoints["openai_video"]; exists {
			_, conflict = endpoints["openai-video"]
			if !conflict {
				endpoints["openai-video"] = legacy
			}
			delete(endpoints, "openai_video")
			changed = true
		}
		if pluginOnly && standardChatMetadataEndpoint(endpoints["openai"]) {
			delete(endpoints, "openai")
			changed = true
		}
	case []any:
		seenCanonical := false
		for _, endpoint := range endpoints {
			seenCanonical = seenCanonical || endpoint == "openai-video"
		}
		migrated := make([]any, 0, len(endpoints))
		for _, endpoint := range endpoints {
			if pluginOnly && endpoint == "openai" {
				changed = true
				continue
			}
			if endpoint == "openai_video" {
				changed = true
				conflict = seenCanonical
				if seenCanonical {
					continue
				}
				endpoint = "openai-video"
				seenCanonical = true
			}
			migrated = append(migrated, endpoint)
		}
		if !changed {
			return raw, false, false, nil
		}
		value = migrated
	default:
		return raw, false, false, nil
	}
	if !changed {
		return raw, false, false, nil
	}
	data, err := common.Marshal(value)
	if err != nil {
		return raw, false, false, err
	}
	return string(data), true, conflict, nil
}

func standardChatMetadataEndpoint(endpoint any) bool {
	switch value := endpoint.(type) {
	case string:
		return value == "/v1/chat/completions"
	case map[string]any:
		return value["path"] == "/v1/chat/completions"
	}
	return false
}
