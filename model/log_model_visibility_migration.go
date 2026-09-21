/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package model

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// LogModelVisibilityMigrationReport describes one dry run or apply pass over
// legacy usage logs whose model diagnostics were stored at the top level.
type LogModelVisibilityMigrationReport struct {
	Scanned    int64
	Candidates int64
	Migrated   int64
	Remaining  int64
}

type logModelVisibilityUpdate struct {
	id       int
	original string
	other    string
}

// InitLogDBForMaintenance opens only the configured log database. Unlike the
// normal service startup path, it does not run primary-database migrations or
// schema checks.
func InitLogDBForMaintenance() error {
	envName := "LOG_SQL_DSN"
	isLogDatabase := true
	if os.Getenv(envName) == "" {
		envName = "SQL_DSN"
		isLogDatabase = false
	}
	db, dbType, err := chooseDB(envName, isLogDatabase)
	if err != nil {
		return err
	}
	LOG_DB = db
	common.SetLogDatabaseType(dbType)
	return nil
}

// CloseLogDBForMaintenance closes the database opened by
// InitLogDBForMaintenance.
func CloseLogDBForMaintenance() error {
	if LOG_DB == nil {
		return nil
	}
	return closeDB(LOG_DB)
}

// MigrateLogModelVisibility moves legacy public model diagnostics into
// admin_info. It is intentionally an operator-invoked maintenance action, not
// part of application startup. Without apply it only reports affected rows.
func MigrateLogModelVisibility(apply bool, batchSize int) (LogModelVisibilityMigrationReport, error) {
	return migrateLogModelVisibility(LOG_DB, apply, batchSize)
}

func migrateLogModelVisibility(db *gorm.DB, apply bool, batchSize int) (LogModelVisibilityMigrationReport, error) {
	if db == nil {
		return LogModelVisibilityMigrationReport{}, errors.New("log database is not initialized")
	}
	if batchSize <= 0 {
		return LogModelVisibilityMigrationReport{}, errors.New("batch size must be positive")
	}
	if db.Dialector != nil && db.Dialector.Name() == string(common.DatabaseTypeClickHouse) {
		return LogModelVisibilityMigrationReport{}, errors.New("ClickHouse log visibility migration is not supported; migrate logs with the source SQL database before enabling ClickHouse")
	}

	var report LogModelVisibilityMigrationReport
	var lastID int
	hasCursor := false
	for {
		query := db.Order("id ASC").Limit(batchSize)
		if hasCursor {
			query = query.Where("id > ?", lastID)
		}

		var logs []Log
		if err := query.Find(&logs).Error; err != nil {
			return report, fmt.Errorf("read log batch: %w", err)
		}
		if len(logs) == 0 {
			break
		}

		report.Scanned += int64(len(logs))
		updates := make([]logModelVisibilityUpdate, 0, len(logs))
		for i := range logs {
			migrated, changed, err := migrateLogOtherModelDiagnostics(logs[i].Other)
			if err != nil {
				return report, fmt.Errorf("migrate log %d: %w", logs[i].Id, err)
			}
			if !changed {
				continue
			}
			report.Candidates++
			updates = append(updates, logModelVisibilityUpdate{
				id:       logs[i].Id,
				original: logs[i].Other,
				other:    migrated,
			})
		}

		if apply && len(updates) > 0 {
			if err := db.Transaction(func(tx *gorm.DB) error {
				for i := range updates {
					result := tx.Model(&Log{}).
						Where("id = ? AND other = ?", updates[i].id, updates[i].original).
						Update("other", updates[i].other)
					if result.Error != nil {
						return result.Error
					}
					if result.RowsAffected != 1 {
						return fmt.Errorf("log %d changed during migration", updates[i].id)
					}
				}
				return nil
			}); err != nil {
				return report, fmt.Errorf("apply log batch ending at %d: %w", logs[len(logs)-1].Id, err)
			}
			report.Migrated += int64(len(updates))
		}

		lastID = logs[len(logs)-1].Id
		hasCursor = true
	}

	remaining, err := countLegacyModelDiagnosticLogs(db, batchSize)
	if err != nil {
		return report, err
	}
	report.Remaining = remaining
	return report, nil
}

func countLegacyModelDiagnosticLogs(db *gorm.DB, batchSize int) (int64, error) {
	var count int64
	var lastID int
	hasCursor := false
	for {
		query := db.Order("id ASC").Limit(batchSize)
		if hasCursor {
			query = query.Where("id > ?", lastID)
		}

		var logs []Log
		if err := query.Find(&logs).Error; err != nil {
			return 0, fmt.Errorf("verify log batch: %w", err)
		}
		if len(logs) == 0 {
			return count, nil
		}
		for i := range logs {
			_, changed, err := migrateLogOtherModelDiagnostics(logs[i].Other)
			if err != nil {
				return 0, fmt.Errorf("verify log %d: %w", logs[i].Id, err)
			}
			if changed {
				count++
			}
		}
		lastID = logs[len(logs)-1].Id
		hasCursor = true
	}
}

func migrateLogOtherModelDiagnostics(other string) (string, bool, error) {
	if strings.TrimSpace(other) == "" {
		return other, false, nil
	}

	var values map[string]common.RawMessage
	if err := common.UnmarshalJsonStr(other, &values); err != nil {
		return "", false, fmt.Errorf("decode other JSON: %w", err)
	}

	hasLegacyDiagnostic := false
	for _, key := range modelDiagnosticLogOtherKeys {
		if _, exists := values[key]; exists {
			hasLegacyDiagnostic = true
			break
		}
	}
	if !hasLegacyDiagnostic {
		return other, false, nil
	}

	adminInfo := make(map[string]common.RawMessage)
	if rawAdminInfo, exists := values[logOtherAdminInfoKey]; exists && string(rawAdminInfo) != "null" {
		if err := common.Unmarshal(rawAdminInfo, &adminInfo); err != nil {
			return "", false, fmt.Errorf("decode admin_info: %w", err)
		}
		if adminInfo == nil {
			adminInfo = make(map[string]common.RawMessage)
		}
	}

	for _, key := range modelDiagnosticLogOtherKeys {
		value, exists := values[key]
		if !exists {
			continue
		}
		if _, scoped := adminInfo[key]; !scoped {
			adminInfo[key] = value
		}
		delete(values, key)
	}

	encodedAdminInfo, err := common.Marshal(adminInfo)
	if err != nil {
		return "", false, fmt.Errorf("encode admin_info: %w", err)
	}
	values[logOtherAdminInfoKey] = encodedAdminInfo
	encoded, err := common.Marshal(values)
	if err != nil {
		return "", false, fmt.Errorf("encode other JSON: %w", err)
	}
	return string(encoded), true, nil
}
