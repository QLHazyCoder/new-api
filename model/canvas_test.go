package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanvasDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			dsn := os.Getenv("TEST_CANVAS_" + strings.ToUpper(dialect) + "_DSN")
			if dialect != "sqlite" && dsn == "" {
				t.Skip("database DSN not configured")
			}
			db := openSensitiveWordMatrixDB(t, dialect, dsn)
			oldDB := DB
			oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
			DB = db
			databaseType := common.DatabaseTypeSQLite
			if dialect == "mysql" {
				databaseType = common.DatabaseTypeMySQL
			}
			if dialect == "postgres" {
				databaseType = common.DatabaseTypePostgreSQL
			}
			common.SetDatabaseTypes(databaseType, oldLog)
			initCol()
			t.Cleanup(func() { DB = oldDB; common.SetDatabaseTypes(oldMain, oldLog); initCol() })

			// Represents the released schema before the additive canvas upgrade.
			require.NoError(t, db.AutoMigrate(&User{}))
			legacy := User{Username: "canvas-retained", Password: "unused", Group: "default"}
			require.NoError(t, db.Create(&legacy).Error)
			for range 2 {
				require.NoError(t, db.AutoMigrate(&CanvasProject{}, &CanvasAsset{}, &CanvasProjectAssetRef{}, &CanvasGenerationRun{}, &CanvasUserSetting{}))
			}
			var retained User
			require.NoError(t, db.First(&retained, legacy.Id).Error)
			assert.Equal(t, "canvas-retained", retained.Username)
			asset := CanvasAsset{UserID: legacy.Id, AssetID: "image:owned", MIME: "image/png"}
			require.NoError(t, db.Create(&asset).Error)
			doc := map[string]any{"title": "无限画布 测试", "nodes": []any{map[string]any{"id": "node", "type": "text", "position": map[string]any{"x": 0, "y": 0}, "width": 200, "height": 100, "text": strings.Repeat("中文", 40000), "storageKey": "image:owned", "url": "blob:transient"}}, "connections": []any{}, "chatSessions": []any{}, "backgroundMode": "lines", "viewport": map[string]any{"x": 0, "y": 0, "k": 1}}
			project, err := SaveCanvasProject(legacy.Id, "project-1", 0, doc)
			require.NoError(t, err)
			assert.EqualValues(t, 1, project.Revision)
			assert.Greater(t, len(project.Document), 65535, "MySQL must not truncate a large document")
			assert.NotContains(t, string(project.Document), "blob:transient")
			_, err = SaveCanvasProject(legacy.Id, "project-1", 0, doc)
			assert.ErrorIs(t, err, ErrCanvasConflict)
			project, err = SaveCanvasProject(legacy.Id, "project-1", 1, doc)
			require.NoError(t, err)
			assert.EqualValues(t, 2, project.Revision)
			_, err = SaveCanvasProject(legacy.Id, "project-1", 1, doc)
			assert.ErrorIs(t, err, ErrCanvasConflict)
			_, err = SaveCanvasProject(legacy.Id+1, "project-1", 0, doc)
			assert.ErrorIs(t, err, ErrCanvasInvalid, "another user cannot reference an owner's asset")
			require.NoError(t, db.Model(&CanvasProject{}).Where("id = ?", project.ID).Update("deleted_at", 1).Error)
			_, err = SaveCanvasProject(legacy.Id, "project-1", 2, doc)
			assert.ErrorIs(t, err, ErrCanvasConflict, "autosave cannot resurrect a deleted project")
			var refs int64
			require.NoError(t, db.Model(&CanvasProjectAssetRef{}).Where("user_id = ?", legacy.Id).Count(&refs).Error)
			assert.EqualValues(t, 1, refs)
			assert.ErrorIs(t, DeleteCanvasAsset(legacy.Id, "image:owned"), ErrCanvasConflict)
			orphan := CanvasAsset{UserID: legacy.Id, AssetID: "image:orphan", CreatedAt: 1}
			require.NoError(t, db.Create(&orphan).Error)
			unused, err := ListUnreferencedCanvasAssets(2)
			require.NoError(t, err)
			require.Len(t, unused, 1)
			assert.Equal(t, "image:orphan", unused[0].AssetID)
			require.NoError(t, PurgeCanvasProject(*project, 2))
			assert.NoError(t, DeleteCanvasAsset(legacy.Id, "image:owned"))

			setting := CanvasUserSetting{UserID: legacy.Id, Namespace: "app_state", EntryKey: "preferences", Value: `{"theme":"dark"}`}
			require.NoError(t, SaveCanvasSetting(&setting, 0))
			setting.ID = 0
			assert.ErrorIs(t, SaveCanvasSetting(&setting, 0), ErrCanvasConflict)
			require.NoError(t, SaveCanvasSetting(&setting, 1))
			assert.ErrorIs(t, SaveCanvasSetting(&setting, 1), ErrCanvasConflict)
			var count int64
			require.NoError(t, db.Model(&User{}).Count(&count).Error)
			assert.EqualValues(t, 1, count)
			require.NoError(t, db.AutoMigrate(&CanvasProject{}, &CanvasUserSetting{}))
			require.NoError(t, db.First(&retained, legacy.Id).Error)
			assert.Equal(t, "canvas-retained", retained.Username)
			var version string
			if dialect == "sqlite" {
				require.NoError(t, db.Raw("select sqlite_version()").Scan(&version).Error)
			} else {
				require.NoError(t, db.Raw("select version()").Scan(&version).Error)
			}
			t.Log("database version:", version)
		})
	}
}

func TestCanvasRejectsCredentialsAndScripts(t *testing.T) {
	for _, key := range []string{"apiKey", "access_token", "Authorization", "baseUrl", "script", "password"} {
		_, _, err := SanitizeCanvasValue(map[string]any{"nodes": []any{map[string]any{key: "secret"}}})
		assert.ErrorIs(t, err, ErrCanvasInvalid, key)
	}
	_, _, err := SanitizeCanvasValue(map[string]any{"imageUrl": "data:image/png;base64,c2VjcmV0"})
	assert.ErrorIs(t, err, ErrCanvasInvalid)
	prompt := `{"apiKey":"an example to explain, not a credential"}`
	cleaned, _, err := SanitizeCanvasValue(map[string]any{"text": prompt})
	require.NoError(t, err)
	assert.Equal(t, prompt, cleaned.(map[string]any)["text"], "text and prompts are preserved verbatim")
	_, refs, err := SanitizeCanvasValue(map[string]any{"references": []any{"image:retained", "audio:retained"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"audio:retained", "image:retained"}, refs)
	assert.ErrorIs(t, ValidateCanvasDocument([]byte(`{"nodes":[]}`)), ErrCanvasInvalid)
	assert.ErrorIs(t, ValidateCanvasDocument([]byte(`{"nodes":[],"connections":[{"id":"dangling","fromNodeId":"absent","toNodeId":"absent"}],"chatSessions":[],"backgroundMode":"lines","viewport":{"x":0,"y":0,"k":1}}`)), ErrCanvasInvalid)
}
