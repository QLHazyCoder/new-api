package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openRequiredOptionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	return db
}

func TestSeedRequiredOptionsCreatesMissingValueAndPreservesExistingValue(t *testing.T) {
	db := openRequiredOptionTestDB(t)

	require.NoError(t, seedRequiredOptions(db))
	var option Option
	require.NoError(t, db.Where(&Option{Key: playgroundImageConcurrencyKey}).First(&option).Error)
	require.Equal(t, "0", option.Value)

	require.NoError(t, db.Model(&Option{}).
		Where(&Option{Key: playgroundImageConcurrencyKey}).
		Update("value", "7").Error)
	require.NoError(t, seedRequiredOptions(db))
	require.NoError(t, db.Where(&Option{Key: playgroundImageConcurrencyKey}).First(&option).Error)
	require.Equal(t, "7", option.Value)
}

func TestValidateRequiredOptionsRejectsMissingAndInvalidValues(t *testing.T) {
	db := openRequiredOptionTestDB(t)

	err := validateRequiredOptions(db)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	require.NoError(t, db.Create(&Option{Key: playgroundImageConcurrencyKey, Value: "invalid"}).Error)
	require.ErrorContains(t, validateRequiredOptions(db), "must be a non-negative integer")

	require.NoError(t, db.Model(&Option{}).
		Where(&Option{Key: playgroundImageConcurrencyKey}).
		Update("value", "4").Error)
	require.NoError(t, validateRequiredOptions(db))
}
