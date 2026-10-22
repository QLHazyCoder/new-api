package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

const CanvasMaxDocumentBytes = 10 << 20

var (
	ErrCanvasConflict = errors.New("Canvas content changed in another window")
	ErrCanvasInvalid  = errors.New("Invalid canvas content")
	canvasIDPattern   = regexp.MustCompile("^[a-zA-Z0-9_:-]{1,64}$")
)

// CanvasJSON deliberately exceeds MySQL TEXT's 64KiB ceiling.
type CanvasJSON string

func (CanvasJSON) GormDataType() string { return "string" }
func (CanvasJSON) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	if db.Dialector.Name() == "mysql" {
		return "longtext"
	}
	return "text"
}

type CanvasProject struct {
	ID            int64      `json:"-" gorm:"primaryKey"`
	UserID        int        `json:"-" gorm:"uniqueIndex:idx_canvas_project_owner;index:idx_canvas_project_recent,priority:1"`
	ProjectID     string     `json:"project_id" gorm:"type:varchar(64);uniqueIndex:idx_canvas_project_owner"`
	Title         string     `json:"title" gorm:"type:varchar(200)"`
	SchemaVersion int        `json:"schema_version"`
	Revision      int64      `json:"revision"`
	Document      CanvasJSON `json:"-" `
	CreatedAt     int64      `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt     int64      `json:"updated_at" gorm:"autoUpdateTime:milli;index:idx_canvas_project_recent,priority:2"`
	DeletedAt     int64      `json:"deleted_at"`
}

type CanvasAsset struct {
	ID          int64  `json:"-" gorm:"primaryKey"`
	UserID      int    `json:"-" gorm:"uniqueIndex:idx_canvas_asset_owner;index:idx_canvas_asset_recent,priority:1"`
	AssetID     string `json:"asset_id" gorm:"type:varchar(64);uniqueIndex:idx_canvas_asset_owner"`
	Kind        string `json:"kind" gorm:"type:varchar(16)"`
	Title       string `json:"title" gorm:"type:varchar(200)"`
	SourceRunID string `json:"source_run_id,omitempty" gorm:"type:varchar(64);index"`
	StorageKey  string `json:"-" gorm:"type:varchar(512)"`
	MIME        string `json:"mime" gorm:"type:varchar(128)"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256" gorm:"type:varchar(64)"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt   int64  `json:"updated_at" gorm:"autoUpdateTime:milli;index:idx_canvas_asset_recent,priority:2"`
	DeletedAt   int64  `json:"deleted_at"`
}

type CanvasProjectAssetRef struct {
	ID        int64  `gorm:"primaryKey"`
	UserID    int    `gorm:"uniqueIndex:idx_canvas_ref_owner,priority:1"`
	Scope     string `gorm:"type:varchar(16);uniqueIndex:idx_canvas_ref_owner,priority:4"`
	ProjectID string `gorm:"type:varchar(64);uniqueIndex:idx_canvas_ref_owner,priority:2"`
	AssetID   string `gorm:"type:varchar(64);uniqueIndex:idx_canvas_ref_owner,priority:3;index"`
}

type CanvasGenerationRun struct {
	ID              int64  `json:"-" gorm:"primaryKey"`
	UserID          int    `json:"-" gorm:"uniqueIndex:idx_canvas_run_request;index:idx_canvas_run_recent,priority:1"`
	RunID           string `json:"run_id" gorm:"type:varchar(64);uniqueIndex"`
	ClientRequestID string `json:"client_request_id" gorm:"type:varchar(64);uniqueIndex:idx_canvas_run_request"`
	RequestHash     string `json:"-" gorm:"type:varchar(64)"`
	Operation       string `json:"operation" gorm:"type:varchar(64)"`
	Model           string `json:"model" gorm:"type:varchar(256)"`
	GroupName       string `json:"group" gorm:"type:varchar(128)"`
	Status          string `json:"status" gorm:"type:varchar(32);index"`
	TaskID          string `json:"task_id,omitempty" gorm:"type:varchar(256)"`
	ResultPath      string `json:"-" gorm:"type:varchar(512)"`
	ResultMIME      string `json:"result_mime" gorm:"type:varchar(128)"`
	HTTPStatus      int    `json:"http_status"`
	ArchiveStatus   string `json:"archive_status" gorm:"type:varchar(32)"`
	CreatedAt       int64  `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt       int64  `json:"updated_at" gorm:"autoUpdateTime:milli;index:idx_canvas_run_recent,priority:2"`
}

// Small application namespaces are versioned independently, so saving a
// preference cannot overwrite projects or another tab's generation history.
type CanvasUserSetting struct {
	ID        int64      `json:"-" gorm:"primaryKey"`
	UserID    int        `json:"-" gorm:"uniqueIndex:idx_canvas_setting_owner"`
	Namespace string     `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_canvas_setting_owner"`
	EntryKey  string     `json:"key" gorm:"type:varchar(192);uniqueIndex:idx_canvas_setting_owner"`
	Value     CanvasJSON `json:"value"`
	Revision  int64      `json:"revision"`
	UpdatedAt int64      `json:"updated_at" gorm:"autoUpdateTime:milli"`
}

func ValidCanvasID(id string) bool { return canvasIDPattern.MatchString(id) }

func CanvasMetadataEndpoints(names []string) (map[string]map[string]common.EndpointInfo, error) {
	var records []Model
	if err := DB.Find(&records).Error; err != nil {
		return nil, err
	}
	resolved := resolveModelMetadata(records, names)
	result := make(map[string]map[string]common.EndpointInfo, len(resolved))
	for name, metadata := range resolved {
		result[name] = catalogMetadataEndpoints(metadata.Endpoints)
	}
	return result, nil
}

func ListUnreferencedCanvasAssets(cutoff int64) ([]CanvasAsset, error) {
	var assets []CanvasAsset
	refs := DB.Model(&CanvasProjectAssetRef{}).Select("1").Where("canvas_project_asset_refs.user_id = canvas_assets.user_id AND canvas_project_asset_refs.asset_id = canvas_assets.asset_id")
	err := DB.Where("deleted_at = 0 AND created_at < ?", cutoff).Where("NOT EXISTS (?)", refs).Order("updated_at").Limit(100).Find(&assets).Error
	return assets, err
}

func DeleteCanvasAsset(userID int, assetID string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var asset CanvasAsset
		if err := lockForUpdate(tx).Where("user_id = ? AND asset_id = ? AND deleted_at = 0", userID, assetID).First(&asset).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&CanvasProjectAssetRef{}).Where("user_id = ? AND asset_id = ?", userID, assetID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrCanvasConflict
		}
		return tx.Model(&asset).Update("deleted_at", time.Now().UnixMilli()).Error
	})
}

// SanitizeCanvasValue rejects credential/executable config and removes
// transient object URLs. It returns only references that can be owner-checked.
func SanitizeCanvasValue(value any) (any, []string, error) {
	wrapped := false
	if text, ok := value.(string); ok {
		var decoded any
		if common.UnmarshalJsonStr(text, &decoded) == nil {
			value = decoded
			wrapped = true
		}
	}
	refs := map[string]bool{}
	var walk func(any, int) (any, error)
	walk = func(item any, depth int) (any, error) {
		if depth > 128 {
			return nil, ErrCanvasInvalid
		}
		switch typed := item.(type) {
		case map[string]any:
			out := make(map[string]any, len(typed))
			for key, child := range typed {
				normalized := strings.ToLower(strings.ReplaceAll(key, "_", ""))
				switch normalized {
				case "apikey", "authorization", "accesstoken", "refreshtoken", "cookie", "password", "baseurl", "script", "plugincode":
					if child != nil && child != "" {
						return nil, fmt.Errorf("%w: credentials and scripts are not allowed", ErrCanvasInvalid)
					}
					continue
				}
				if text, ok := child.(string); ok {
					if strings.HasPrefix(text, "blob:") {
						out[key] = ""
						continue
					}
					if strings.HasPrefix(text, "data:") && normalized != "text" && normalized != "content" && normalized != "prompt" {
						if storageKey, ok := typed["storageKey"].(string); ok && storageKey != "" {
							out[key] = ""
							continue
						}
						return nil, fmt.Errorf("%w: media must be uploaded", ErrCanvasInvalid)
					}
					if strings.Contains(normalized, "storagekey") && text != "" {
						refs[text] = true
					}
				}
				if normalized == "references" {
					if values, ok := child.([]any); ok {
						for _, value := range values {
							text, ok := value.(string)
							if !ok {
								continue
							}
							kind, _, _ := strings.Cut(text, ":")
							if slices.Contains([]string{"image", "video", "audio", "file"}, kind) {
								refs[text] = true
							}
						}
					}
				}
				cleaned, err := walk(child, depth+1)
				if err != nil {
					return nil, err
				}
				out[key] = cleaned
			}
			return out, nil
		case []any:
			out := make([]any, len(typed))
			for i := range typed {
				cleaned, err := walk(typed[i], depth+1)
				if err != nil {
					return nil, err
				}
				out[i] = cleaned
			}
			return out, nil
		default:
			return item, nil
		}
	}
	cleaned, err := walk(value, 0)
	if wrapped && err == nil {
		raw, encodeErr := common.Marshal(cleaned)
		cleaned = string(raw)
		err = encodeErr
	}
	keys := make([]string, 0, len(refs))
	for key := range refs {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return cleaned, keys, err
}

func SaveCanvasProject(userID int, projectID string, revision int64, document map[string]any) (*CanvasProject, error) {
	if userID <= 0 || !ValidCanvasID(projectID) {
		return nil, ErrCanvasInvalid
	}
	title, _ := document["title"].(string)
	if strings.TrimSpace(title) == "" || utf8.RuneCountInString(title) > 200 {
		return nil, ErrCanvasInvalid
	}
	document["id"] = projectID
	cleaned, refs, err := SanitizeCanvasValue(document)
	if err != nil {
		return nil, err
	}
	raw, err := common.Marshal(cleaned)
	if err != nil || len(raw) > CanvasMaxDocumentBytes {
		return nil, ErrCanvasInvalid
	}
	if err := ValidateCanvasDocument(raw); err != nil {
		return nil, err
	}
	record := &CanvasProject{UserID: userID, ProjectID: projectID, Title: title, Document: CanvasJSON(raw), SchemaVersion: 1, Revision: revision + 1}
	err = DB.Transaction(func(tx *gorm.DB) error {
		// Check every asset in the same transaction that replaces references.
		for _, assetID := range refs {
			var asset CanvasAsset
			if err := lockForUpdate(tx).Where("user_id = ? AND asset_id = ? AND deleted_at = 0", userID, assetID).First(&asset).Error; err != nil {
				return ErrCanvasInvalid
			}
		}
		if revision == 0 {
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(record)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrCanvasConflict
			}
		} else {
			result := tx.Model(&CanvasProject{}).Where("user_id = ? AND project_id = ? AND revision = ? AND deleted_at = 0", userID, projectID, revision).
				Updates(map[string]any{"title": title, "document": CanvasJSON(raw), "revision": revision + 1, "updated_at": time.Now().UnixMilli()})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrCanvasConflict
			}
		}
		if err := tx.Where("user_id = ? AND project_id = ? AND scope = ?", userID, projectID, "project").Delete(&CanvasProjectAssetRef{}).Error; err != nil {
			return err
		}
		for _, assetID := range refs {
			if err := tx.Create(&CanvasProjectAssetRef{UserID: userID, ProjectID: projectID, AssetID: assetID, Scope: "project"}).Error; err != nil {
				return err
			}
		}
		return tx.Where("user_id = ? AND project_id = ?", userID, projectID).First(record).Error
	})
	return record, err
}

// ValidateCanvasDocument protects the renderer contract at the persistence
// boundary. Partial or imported documents must not corrupt a reloadable project.
func ValidateCanvasDocument(raw []byte) error {
	var doc struct {
		Nodes *[]struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Position *struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			} `json:"position"`
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"nodes"`
		Connections *[]struct {
			ID   string `json:"id"`
			From string `json:"fromNodeId"`
			To   string `json:"toNodeId"`
		} `json:"connections"`
		ChatSessions *[]struct {
			ID       string            `json:"id"`
			Messages *[]map[string]any `json:"messages"`
		} `json:"chatSessions"`
		Viewport *struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
			K float64 `json:"k"`
		} `json:"viewport"`
		Background string `json:"backgroundMode"`
	}
	if common.Unmarshal(raw, &doc) != nil || doc.Nodes == nil || doc.Connections == nil || doc.ChatSessions == nil || doc.Viewport == nil {
		return ErrCanvasInvalid
	}
	if !slices.Contains([]string{"lines", "dots", "blank"}, doc.Background) || doc.Viewport.K <= 0 || math.IsNaN(doc.Viewport.K) || math.IsInf(doc.Viewport.K, 0) {
		return ErrCanvasInvalid
	}
	nodes := map[string]bool{}
	for _, node := range *doc.Nodes {
		if !ValidCanvasID(node.ID) || nodes[node.ID] || node.Position == nil || node.Width <= 0 || node.Height <= 0 || !slices.Contains([]string{"image", "text", "config", "video", "audio", "group"}, node.Type) {
			return ErrCanvasInvalid
		}
		nodes[node.ID] = true
	}
	connections := map[string]bool{}
	for _, connection := range *doc.Connections {
		if !ValidCanvasID(connection.ID) || connections[connection.ID] || !nodes[connection.From] || !nodes[connection.To] {
			return ErrCanvasInvalid
		}
		connections[connection.ID] = true
	}
	sessions := map[string]bool{}
	for _, session := range *doc.ChatSessions {
		if !ValidCanvasID(session.ID) || sessions[session.ID] || session.Messages == nil {
			return ErrCanvasInvalid
		}
		sessions[session.ID] = true
	}
	return nil
}

func SaveCanvasSetting(setting *CanvasUserSetting, revision int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var value any
		if common.UnmarshalJsonStr(string(setting.Value), &value) != nil {
			return ErrCanvasInvalid
		}
		_, refs, err := SanitizeCanvasValue(value)
		if err != nil {
			return err
		}
		for _, assetID := range refs {
			var asset CanvasAsset
			if err := lockForUpdate(tx).Where("user_id = ? AND asset_id = ? AND deleted_at = 0", setting.UserID, assetID).First(&asset).Error; err != nil {
				return ErrCanvasInvalid
			}
		}
		setting.Revision = revision + 1
		if revision == 0 {
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(setting)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrCanvasConflict
			}
		} else {
			result := tx.Model(&CanvasUserSetting{}).Where("user_id = ? AND namespace = ? AND entry_key = ? AND revision = ?", setting.UserID, setting.Namespace, setting.EntryKey, revision).
				Updates(map[string]any{"value": setting.Value, "revision": revision + 1, "updated_at": time.Now().UnixMilli()})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrCanvasConflict
			}
		}
		refID := fmt.Sprintf("%x", sha256.Sum256([]byte(setting.Namespace+":"+setting.EntryKey)))
		if err := tx.Where("user_id = ? AND project_id = ? AND scope = ?", setting.UserID, refID, "state").Delete(&CanvasProjectAssetRef{}).Error; err != nil {
			return err
		}
		for _, assetID := range refs {
			if err := tx.Create(&CanvasProjectAssetRef{UserID: setting.UserID, ProjectID: refID, AssetID: assetID, Scope: "state"}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func DeleteCanvasSetting(userID int, namespace, key string, revision int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("user_id = ? AND namespace = ? AND entry_key = ? AND revision = ?", userID, namespace, key, revision).Delete(&CanvasUserSetting{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCanvasConflict
		}
		refID := fmt.Sprintf("%x", sha256.Sum256([]byte(namespace+":"+key)))
		return tx.Where("user_id = ? AND project_id = ? AND scope = ?", userID, refID, "state").Delete(&CanvasProjectAssetRef{}).Error
	})
}

func PurgeCanvasProject(project CanvasProject, cutoff int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var current CanvasProject
		err := lockForUpdate(tx).Where("id = ? AND deleted_at > 0 AND deleted_at < ?", project.ID, cutoff).First(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND project_id = ? AND scope = ?", current.UserID, current.ProjectID, "project").Delete(&CanvasProjectAssetRef{}).Error; err != nil {
			return err
		}
		return tx.Delete(&current).Error
	})
}
