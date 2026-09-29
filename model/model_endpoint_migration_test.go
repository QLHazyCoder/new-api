package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateLegacyPublicVideoEndpoints(t *testing.T) {
	resetPricingEndpointTestTables(t)
	records := []Model{
		{ModelName: "legacy-map", Endpoints: `{"openai_video":{"path":"/v1/videos","method":"POST"},"custom":"/v1/custom"}`, Status: 1},
		{ModelName: "legacy-array", Endpoints: `["openai_video","image-generation"]`, Status: 1},
		{ModelName: "conflict", Endpoints: `{"openai_video":"/v1/old","openai-video":"/v1/videos"}`, Status: 1},
		{ModelName: "other", Endpoints: `{"custom":"/v1/openai_video/custom"}`, Status: 1},
		{ModelName: "plugin-only", Endpoints: `{"openai":"/v1/chat/completions","openai_video":"/v1/videos"}`, Status: 1},
		{ModelName: "mixed-channel", Endpoints: `{"openai":"/v1/chat/completions","openai_video":"/v1/videos"}`, Status: 1},
		{ModelName: "already-canonical", Endpoints: `{"openai":"/v1/chat/completions","openai-video":"/v1/videos"}`, Status: 1},
		{ModelName: "plugin-chat-only", Endpoints: `{"openai":"/v1/chat/completions"}`, Status: 1},
		{ModelName: "other-chat-only", Endpoints: `{"openai":"/v1/chat/completions"}`, Status: 1},
	}
	for i := range records {
		require.NoError(t, DB.Create(&records[i]).Error)
	}
	require.NoError(t, DB.Create(&Channel{Id: 720, Type: constant.ChannelTypeTaskPlugin, Key: "key-720", Status: common.ChannelStatusEnabled, Name: "plugin"}).Error)
	require.NoError(t, DB.Create(&Channel{Id: 721, Type: constant.ChannelTypeOpenAI, Key: "key-721", Status: common.ChannelStatusEnabled, Name: "chat"}).Error)
	for _, ability := range []Ability{
		{Group: "default", Model: "plugin-only", ChannelId: 720, Enabled: true},
		{Group: "default", Model: "mixed-channel", ChannelId: 720, Enabled: true},
		{Group: "default", Model: "mixed-channel", ChannelId: 721, Enabled: true},
		{Group: "default", Model: "already-canonical", ChannelId: 720, Enabled: true},
		{Group: "default", Model: "plugin-chat-only", ChannelId: 720, Enabled: true},
		{Group: "default", Model: "other-chat-only", ChannelId: 721, Enabled: true},
	} {
		require.NoError(t, DB.Create(&ability).Error)
	}
	for range 2 {
		require.NoError(t, MigrateLegacyPublicVideoEndpoints())
	}
	want := []string{
		`{"custom":"/v1/custom","openai-video":{"method":"POST","path":"/v1/videos"}}`,
		`["openai-video","image-generation"]`,
		`{"openai-video":"/v1/videos"}`,
		`{"custom":"/v1/openai_video/custom"}`,
		`{"openai-video":"/v1/videos"}`,
		`{"openai":"/v1/chat/completions","openai-video":"/v1/videos"}`,
		`{"openai-video":"/v1/videos"}`,
		`{}`,
		`{"openai":"/v1/chat/completions"}`,
	}
	for i, record := range records {
		var loaded Model
		require.NoError(t, DB.First(&loaded, record.Id).Error)
		assert.JSONEq(t, want[i], loaded.Endpoints)
	}
}

func TestValidateModelEndpointsRejectsInternalPluginProtocolOnly(t *testing.T) {
	for _, input := range []string{`{"openai_video":"/v1/videos"}`, `["openai_video"]`} {
		assert.ErrorContains(t, ValidateModelEndpoints(input), "openai-video")
	}
	for _, input := range []string{`{"openai-video":"/v1/videos"}`, `["openai-video"]`, `{"custom-path":"/v1/custom"}`} {
		assert.NoError(t, ValidateModelEndpoints(input))
	}
}
