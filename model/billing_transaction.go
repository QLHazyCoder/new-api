package model

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const billingTransactionAttempts = 5

// withBillingTransaction retries the complete database transaction after a
// transient deadlock or serialization failure. Retrying the whole closure is
// required so every row read and write observes a fresh snapshot and lock set.
func withBillingTransaction(ctx context.Context, fn func(*gorm.DB) error) error {
	if fn == nil {
		return errors.New("billing transaction callback is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var err error
	for attempt := 0; attempt < billingTransactionAttempts; attempt++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		err = DB.WithContext(ctx).Transaction(fn)
		if !isRetryableBillingTransactionError(err) || attempt == billingTransactionAttempts-1 {
			return err
		}
		// A lock can outlive the statement that reported it (especially on
		// SQLite's single-writer fallback), so use a bounded exponential pause
		// instead of immediately re-entering the same contention window.
		delay := time.Duration(50*(1<<attempt)+int(time.Now().UnixNano()%7)) * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

func isRetryableBillingTransactionError(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1205 || mysqlErr.Number == 1213
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40P01" || pgErr.Code == "40001"
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "deadlock found") || strings.Contains(message, "could not serialize access") || strings.Contains(message, "database is locked")
}

func lockBillingUser(tx *gorm.DB, userID int) error {
	if userID <= 0 {
		return errors.New("invalid billing user id")
	}
	var user User
	return lockForUpdate(tx).Select("id").Where("id = ?", userID).First(&user).Error
}

func lockBillingSubscription(tx *gorm.DB, subscriptionID int) error {
	if subscriptionID <= 0 {
		return errors.New("invalid billing subscription id")
	}
	var subscription UserSubscription
	return lockForUpdate(tx).Select("id").Where("id = ?", subscriptionID).First(&subscription).Error
}

func lockBillingToken(tx *gorm.DB, tokenID int) error {
	if tokenID <= 0 {
		return errors.New("invalid billing token id")
	}
	var token Token
	return lockForUpdate(tx).Select("id").Where("id = ?", tokenID).First(&token).Error
}
