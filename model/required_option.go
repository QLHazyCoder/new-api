package model

import (
	"errors"
	"fmt"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	playgroundImageConcurrencyKey        = "PlaygroundImageMaxConcurrency"
	playgroundImageDefaultMaxConcurrency = 0
)

func requiredOptionDefaults() []Option {
	return []Option{
		{
			Key:   playgroundImageConcurrencyKey,
			Value: strconv.Itoa(playgroundImageDefaultMaxConcurrency),
		},
	}
}

func seedRequiredOptions(db *gorm.DB) error {
	if db == nil {
		return errors.New("database is not initialized")
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(requiredOptionDefaults()).Error; err != nil {
		return fmt.Errorf("seed required options: %w", err)
	}
	return nil
}

func validateRequiredOptions(db *gorm.DB) error {
	if db == nil {
		return errors.New("database is not initialized")
	}
	var option Option
	if err := db.Where(&Option{Key: playgroundImageConcurrencyKey}).First(&option).Error; err != nil {
		return fmt.Errorf("required option %s: %w", playgroundImageConcurrencyKey, err)
	}
	if _, _, err := normalizePlaygroundImageMaxConcurrency(option.Value); err != nil {
		return fmt.Errorf("required option %s: %w", playgroundImageConcurrencyKey, err)
	}
	return nil
}
