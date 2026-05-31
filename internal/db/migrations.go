package db

import (
	"database/sql"
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

const WalletMigrationsTable = "wallet_migrations"

// EnsureSchema applies SQL migrations and then enforces model tables.
func EnsureSchema(cfg config.Config, database DB, logger *slog.Logger) error {
	if err := ensureWalletTypeEnum(database); err != nil {
		return fmt.Errorf("ensure wallet_type enum: %w", err)
	}

	if err := ensureWalletsTable(database); err != nil {
		return fmt.Errorf("ensure wallets table: %w", err)
	}

	if err := ApplySQLMigrations(cfg, database, logger); err != nil {
		return fmt.Errorf("sql migrations: %w", err)
	}

	if err := database.Gorm.AutoMigrate(&models.Wallet{}, &models.WalletTopupCredit{}); err != nil {
		return fmt.Errorf("gorm auto-migrate failed: %w", err)
	}

	return nil
}

func migrationDriverConfig() *migratepg.Config {
	return &migratepg.Config{
		MigrationsTable: WalletMigrationsTable,
	}
}

// NewMigrate builds a golang-migrate instance that tracks versions in wallet_migrations.
func NewMigrate(cfg config.Config, sqlDB *sql.DB) (*migrate.Migrate, error) {
	driver, err := migratepg.WithInstance(sqlDB, migrationDriverConfig())
	if err != nil {
		return nil, fmt.Errorf("create migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(cfg.MigrationsPath, "postgres", driver)
	if err != nil {
		return nil, fmt.Errorf("create migration instance: %w", err)
	}

	return m, nil
}

// ApplySQLMigrations runs pending SQL migrations with wallet-specific tracking and recovery.
func ApplySQLMigrations(cfg config.Config, database DB, logger *slog.Logger) error {
	m, err := NewMigrate(cfg, database.SQL)
	if err != nil {
		return err
	}
	defer closeMigrate(m, logger)

	version, dirty, err := m.Version()
	switch {
	case err == nil:
		logger.Info("migration_status",
			slog.Uint64("version", uint64(version)),
			slog.Bool("dirty", dirty),
			slog.String("table", WalletMigrationsTable),
		)
	case errors.Is(err, migrate.ErrNilVersion):
		logger.Info("migration_status",
			slog.String("state", "empty"),
			slog.String("table", WalletMigrationsTable),
		)
		if baseline, baselineErr := detectBaselineVersion(database); baselineErr != nil {
			return fmt.Errorf("detect migration baseline: %w", baselineErr)
		} else if baseline > 0 {
			logger.Info("migration_baseline_detected", slog.Uint64("version", uint64(baseline)))
			if forceErr := m.Force(int(baseline)); forceErr != nil {
				return fmt.Errorf("force baseline migration version %d: %w", baseline, forceErr)
			}
			version = baseline
			dirty = false
		}
	default:
		return fmt.Errorf("read migration version: %w", err)
	}

	if dirty {
		if err := clearDirtyMigration(m, int(version), logger); err != nil {
			return err
		}
	}

	if err := runMigrationsUp(m, logger); err != nil {
		return err
	}

	if newVersion, _, verErr := m.Version(); verErr == nil {
		logger.Info("migrations_ready", slog.Uint64("version", uint64(newVersion)))
	} else if errors.Is(verErr, migrate.ErrNilVersion) {
		logger.Info("migrations_ready", slog.String("state", "empty"))
	}

	return nil
}

// RunMigrationsDown rolls back one migration step.
func RunMigrationsDown(cfg config.Config, database DB, logger *slog.Logger) error {
	m, err := NewMigrate(cfg, database.SQL)
	if err != nil {
		return err
	}
	defer closeMigrate(m, logger)

	version, dirty, verErr := m.Version()
	switch {
	case verErr == nil:
		logger.Info("migration_down_start",
			slog.Uint64("version", uint64(version)),
			slog.Bool("dirty", dirty),
		)
	case errors.Is(verErr, migrate.ErrNilVersion):
		logger.Info("migration_down_noop", slog.String("reason", "no migrations applied"))
		return nil
	default:
		return fmt.Errorf("read migration version: %w", verErr)
	}

	if dirty {
		if err := clearDirtyMigration(m, int(version), logger); err != nil {
			return err
		}
	}

	if err := m.Steps(-1); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("migration_down_noop", slog.String("reason", "already at first migration"))
			return nil
		}
		return fmt.Errorf("rollback migration: %w", err)
	}

	return nil
}

func runMigrationsUp(m *migrate.Migrate, logger *slog.Logger) error {
	return runMigrationsUpWithRetry(m, logger, 0)
}

func runMigrationsUpWithRetry(m *migrate.Migrate, logger *slog.Logger, attempts int) error {
	if attempts > 2 {
		return fmt.Errorf("apply migrations: exceeded dirty recovery attempts")
	}

	err := m.Up()
	if err == nil {
		logger.Info("migrations_applied")
		return nil
	}
	if errors.Is(err, migrate.ErrNoChange) {
		logger.Info("migrations_up_to_date")
		return nil
	}

	var dirtyErr *migrate.ErrDirty
	if errors.As(err, &dirtyErr) {
		if clearErr := clearDirtyMigration(m, int(dirtyErr.Version), logger); clearErr != nil {
			return clearErr
		}
		return runMigrationsUpWithRetry(m, logger, attempts+1)
	}

	if version, ok := parseDirtyVersion(err.Error()); ok {
		if clearErr := clearDirtyMigration(m, version, logger); clearErr != nil {
			return clearErr
		}
		return runMigrationsUpWithRetry(m, logger, attempts+1)
	}

	return fmt.Errorf("apply migrations: %w", err)
}

func clearDirtyMigration(m *migrate.Migrate, version int, logger *slog.Logger) error {
	logger.Warn("migration_clearing_dirty", slog.Int("version", version))
	if err := m.Force(version); err != nil {
		return fmt.Errorf("force dirty migration version %d: %w", version, err)
	}
	return nil
}

func closeMigrate(m *migrate.Migrate, logger *slog.Logger) {
	sourceErr, dbErr := m.Close()
	if sourceErr != nil {
		logger.Warn("migration_close_source_failed", slog.Any("error", sourceErr))
	}
	if dbErr != nil {
		logger.Warn("migration_close_database_failed", slog.Any("error", dbErr))
	}
}

func detectBaselineVersion(database DB) (uint, error) {
	checks := []struct {
		version uint
		query   string
	}{
		{
			version: 10,
			query: `SELECT EXISTS (
				SELECT 1
				FROM wallets
				WHERE wallet_type = 'system'::wallet_type
			)`,
		},
		{
			version: 8,
			query: `SELECT EXISTS (
				SELECT 1
				FROM information_schema.columns
				WHERE table_schema = 'public'
				  AND table_name = 'withdrawals'
				  AND column_name = 'id'
				  AND udt_name = 'uuid'
			)`,
		},
		{
			version: 5,
			query: `SELECT EXISTS (
				SELECT 1
				FROM information_schema.tables
				WHERE table_schema = 'public'
				  AND table_name = 'system_configs'
			)`,
		},
		{
			version: 4,
			query: `SELECT EXISTS (
				SELECT 1
				FROM information_schema.tables
				WHERE table_schema = 'public'
				  AND table_name = 'withdrawals'
			)`,
		},
		{
			version: 3,
			query: `SELECT EXISTS (
				SELECT 1
				FROM pg_indexes
				WHERE schemaname = 'public'
				  AND indexname = 'wallets_user_id_wallet_type_unique'
			)`,
		},
		{
			version: 2,
			query: `SELECT EXISTS (
				SELECT 1
				FROM information_schema.tables
				WHERE table_schema = 'public'
				  AND table_name = 'wallet_topup_credits'
			)`,
		},
		{
			version: 1,
			query: `SELECT EXISTS (
				SELECT 1
				FROM information_schema.tables
				WHERE table_schema = 'public'
				  AND table_name = 'wallets'
			)`,
		},
	}

	for _, check := range checks {
		var exists bool
		if err := database.SQL.QueryRow(check.query).Scan(&exists); err != nil {
			return 0, fmt.Errorf("baseline check version %d: %w", check.version, err)
		}
		if exists {
			return check.version, nil
		}
	}

	return 0, nil
}

func parseDirtyVersion(message string) (int, bool) {
	re := regexp.MustCompile(`(?i)dirty database version (\d+)`)
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
    CREATE TYPE wallet_type AS ENUM ('passenger', 'driver', 'owner', 'system');
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_enum e
    JOIN pg_type t ON e.enumtypid = t.oid
    WHERE t.typname = 'wallet_type' AND e.enumlabel = 'system'
  ) THEN
    ALTER TYPE wallet_type ADD VALUE 'system';
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
