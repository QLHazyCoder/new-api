package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// walletQuotaSchemaSpec lists every persisted balance field that participates
// in the wallet domain. Request charges and historical request logs are
// intentionally absent: those remain in the int32 charge domain.
type walletQuotaSchemaSpec struct {
	table   string
	model   interface{}
	columns []string
}

var walletQuotaSchemaSpecs = []walletQuotaSchemaSpec{
	{table: "users", model: &User{}, columns: []string{"quota", "used_quota", "aff_quota", "aff_history"}},
	{table: "tokens", model: &Token{}, columns: []string{"remain_quota", "used_quota"}},
	{table: "channels", model: &Channel{}, columns: []string{"used_quota"}},
	{table: "redemptions", model: &Redemption{}, columns: []string{"quota"}},
	{table: "checkins", model: &Checkin{}, columns: []string{"quota_awarded"}},
	{table: "quota_data", model: &QuotaData{}, columns: []string{"quota"}},
	{table: "affiliate_reward_events", model: &AffiliateRewardEvent{}, columns: []string{"base_quota", "reward_rate_bps", "reward_quota", "aff_quota_delta", "user_quota_delta"}},
	{table: "sensitive_word_audit_events", model: &SensitiveWordAuditEvent{}, columns: []string{"quota_before", "quota_after"}},
}

func walletQuotaColumnTypes(spec walletQuotaSchemaSpec) (map[string]string, error) {
	columns, err := DB.Migrator().ColumnTypes(spec.model)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(columns))
	for _, column := range columns {
		result[strings.ToLower(column.Name())] = strings.ToLower(column.DatabaseTypeName())
	}
	return result, nil
}

func isSignedBigIntType(databaseType string) bool {
	databaseType = strings.ToLower(strings.TrimSpace(databaseType))
	if strings.Contains(databaseType, "unsigned") {
		return false
	}
	if DB != nil && DB.Dialector != nil && DB.Dialector.Name() == string(common.DatabaseTypeSQLite) {
		// SQLite uses dynamic affinity. INTEGER and BIGINT both store the full
		// signed 64-bit range, so accept either spelling here.
		return strings.Contains(databaseType, "int")
	}
	return databaseType == "bigint" || strings.HasPrefix(databaseType, "bigint(")
}

func validateWalletQuotaSchema() error {
	if DB == nil {
		return nil
	}
	for _, spec := range walletQuotaSchemaSpecs {
		if !DB.Migrator().HasTable(spec.model) {
			continue
		}
		columnTypes, err := walletQuotaColumnTypes(spec)
		if err != nil {
			return fmt.Errorf("inspect wallet quota schema for %s: %w", spec.table, err)
		}
		for _, column := range spec.columns {
			databaseType, ok := columnTypes[strings.ToLower(column)]
			if !ok {
				return fmt.Errorf("wallet quota column %s.%s is missing", spec.table, column)
			}
			if !isSignedBigIntType(databaseType) {
				return fmt.Errorf("wallet quota column %s.%s must be signed BIGINT, got %s", spec.table, column, databaseType)
			}
		}
	}
	return nil
}

// inspectWalletQuotaRangeBeforeMigration is deliberately read-only. It gives
// operators an audit signal for installations that already contain values
// outside the old int32 range without rewriting or scaling any historical data.
func inspectWalletQuotaRangeBeforeMigration() error {
	if DB == nil {
		return nil
	}
	for _, spec := range walletQuotaSchemaSpecs {
		if !DB.Migrator().HasTable(spec.model) {
			continue
		}
		columnTypes, err := walletQuotaColumnTypes(spec)
		if err != nil {
			return fmt.Errorf("inspect existing wallet quota columns for %s: %w", spec.table, err)
		}
		for _, column := range spec.columns {
			if _, ok := columnTypes[strings.ToLower(column)]; !ok {
				// AutoMigrate will add a missing column. Do not make the
				// pre-migration audit fail on a partially migrated legacy schema.
				continue
			}
			var count int64
			query := DB.Table(spec.table).
				Where(column+" > ? OR "+column+" < ?", common.MaxChargeQuota, common.MinChargeQuota).
				Count(&count)
			if query.Error != nil {
				return fmt.Errorf("inspect existing wallet quota values for %s.%s: %w", spec.table, column, query.Error)
			}
			if count > 0 {
				common.SysLog(fmt.Sprintf("wallet quota migration found %d existing %s.%s values outside int32 charge range; preserving them", count, spec.table, column))
			}
		}
	}
	return nil
}
