package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanvasCatalogUsesGroupScopedRoutingAndExplicitSpeechMetadata(t *testing.T) {
	truncate(t)
	model.InitColumnNamesForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.Ability{}, &model.Model{}))
	t.Cleanup(func() {
		require.NoError(t, model.DB.Where("channel_id IN ?", []int{901, 902, 903}).Delete(&model.Ability{}).Error)
		require.NoError(t, model.DB.Where("model_name IN ?", []string{"public-model", "speech-model"}).Delete(&model.Model{}).Error)
	})
	for _, channel := range []model.Channel{
		{Id: 901, Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled, Name: "text"},
		{Id: 902, Type: constant.ChannelTypeSora, Status: common.ChannelStatusEnabled, Name: "video"},
		{Id: 903, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "audio"},
	} {
		require.NoError(t, model.DB.Create(&channel).Error)
	}
	for _, ability := range []model.Ability{
		{Group: "default", Model: "public-model", ChannelId: 901, Enabled: true},
		{Group: "private-not-usable", Model: "public-model", ChannelId: 902, Enabled: true},
		{Group: "default", Model: "speech-model", ChannelId: 903, Enabled: true},
	} {
		require.NoError(t, model.DB.Create(&ability).Error)
	}
	require.NoError(t, model.DB.Create(&model.Model{ModelName: "speech-model", Endpoints: `{"speech":{"path":"/v1/audio/speech","method":"POST"}}`}).Error)
	options, err := CanvasModelCatalog("default")
	require.NoError(t, err)
	assert.Contains(t, options, CanvasModelOption{Name: "public-model", Group: "default", Capability: "text"})
	assert.Contains(t, options, CanvasModelOption{Name: "speech-model", Group: "default", Capability: "audio"})
	assert.NotContains(t, options, CanvasModelOption{Name: "public-model", Group: "default", Capability: "video"}, "same model on a hidden group's video channel cannot leak capabilities")
	for _, option := range options {
		assert.NotEqual(t, "private-not-usable", option.Group)
	}
}
