package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Kept for configurations that still set BATCH_UPDATE_ENABLED. Financial
// counters are always written synchronously; no in-memory batch is started.
func InitBatchUpdater() {
	common.SysLog("BATCH_UPDATE_ENABLED is deprecated: quota and usage updates are synchronous; monitor database write latency and connection pool")
}

func RecordExist(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func shouldUpdateRedis(fromDB bool, err error) bool {
	return common.RedisEnabled && fromDB && err == nil
}
