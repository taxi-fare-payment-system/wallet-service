package db

import (
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"

	"wallet_service/internal/config"
	"wallet_service/internal/models"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// EnsureSchema applies SQL migrations and then enforces model tables.
func EnsureSchema(cfg config.Config, database DB, logger *slog.Logger) error {
	if err := ensureWalletTypeEnum(database); err != nil {
		return fmt.Errorf("ensure wallet_type enum: %w", err)
	}

	if err := ensureWalletsTable(database); err != nil {
		return fmt.Errorf("ensure wallets table: %w", err)
	}

	if err := runSQLMigrations(cfg, database); err != nil {
		logger.Warn("sql_migration_failed_falling_back_to_gorm", slog.Any("error", err))
	}

	if err := database.Gorm.AutoMigrate(&models.Wallet{}, &models.WalletTopupCredit{}); err != nil {
		return fmt.Errorf("gorm auto-migrate failed: %w", err)
	}

	return nil
}

func runSQLMigrations(cfg config.Config, database DB) error {
	driver, err := migratepg.WithInstance(database.SQL, &migratepg.Config{})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(cfg.MigrationsPath, "postgres", driver)
	if err != nil {
		return fmt.Errorf("create migration instance: %w", err)
	}

	if err := m.Up(); err == nil || errors.Is(err, migrate.ErrNoChange) {
		return nil
	} else {
		var dirty *migrate.ErrDirty
		if errors.As(err, &dirty) {
			return forceAndRetry(m, int(dirty.Version))
		}
		if version, ok := parseDirtyVersion(err.Error()); ok {
			return forceAndRetry(m, version)
		}
		return fmt.Errorf("apply migrations: %w", err)
	}
}

func forceAndRetry(m *migrate.Migrate, version int) error {
	if forceErr := m.Force(version); forceErr != nil {
		return fmt.Errorf("force dirty migration version %d: %w", version, forceErr)
	}
	if retryErr := m.Up(); retryErr != nil && !errors.Is(retryErr, migrate.ErrNoChange) {
		return fmt.Errorf("retry after dirty migration version %d: %w", version, retryErr)
	}
	return nil
}

func parseDirtyVersion(message string) (int, bool) {
	re := regexp.MustCompile(`Dirty database version (\d+)`)
	match := re.FindStringSubmatch(message)
	if len(match) != 2 {
		return 0, false
	}
	version, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	return version, true
}

func ensureWalletTypeEnum(database DB) error {
	return database.Gorm.Exec(`
DO $$
BEGIN
  CREATE EXTENSION IF NOT EXISTS pgcrypto;
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'wallet_type') THEN
    CREATE TYPE wallet_type AS ENUM ('passenger', 'driver', 'owner');
  END IF;
END $$;
`).Error
}

func ensureWalletsTable(database DB) error {
	return database.Gorm.Exec(`
CREATE TABLE IF NOT EXISTS wallets (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      TEXT NOT NULL,
  wallet_type  wallet_type NOT NULL,
  sub_city_id  BIGINT NULL,
  freezed      BOOLEAN NOT NULL DEFAULT FALSE,
  balance      NUMERIC(12,2) NOT NULL DEFAULT 0,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`).Error
}
