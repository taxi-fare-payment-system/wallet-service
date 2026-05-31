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
	// Do not call m.Close(): the postgres driver closes the shared *sql.DB passed via WithInstance.

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
	default:
		return fmt.Errorf("read migration version: %w", err)
	}

	if dirty {
		if err := clearDirtyMigration(m, int(version), logger); err != nil {
			return err
		}
	}

	if err := reconcileMigrationVersion(m, database, logger); err != nil {
		return err
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
	// Do not call m.Close(): the postgres driver closes the shared *sql.DB passed via WithInstance.

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

func reconcileMigrationVersion(m *migrate.Migrate, database DB, logger *slog.Logger) error {
	recorded, _, err := m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read migration version for reconcile: %w", err)
	}

	actual, err := detectActualSchemaVersion(database)
	if err != nil {
		return fmt.Errorf("detect actual schema version: %w", err)
	}

	if actual >= recorded {
		return nil
	}

	logger.Warn("migration_version_ahead_of_schema",
		slog.Uint64("recorded", uint64(recorded)),
		slog.Uint64("actual", uint64(actual)),
	)
	if err := m.Force(int(actual)); err != nil {
		return fmt.Errorf("force migration version from %d to %d: %w", recorded, actual, err)
	}

	return nil
}

func detectActualSchemaVersion(database DB) (uint, error) {
	wallets, err := tableExists(database, "wallets")
	if err != nil {
		return 0, err
	}
	if !wallets {
		return 0, nil
	}

	topupCredits, err := tableExists(database, "wallet_topup_credits")
	if err != nil {
		return 0, err
	}
	if !topupCredits {
		return 1, nil
	}

	userTypeIndex, err := indexExists(database, "wallets_user_id_wallet_type_unique")
	if err != nil {
		return 0, err
	}
	if !userTypeIndex {
		return 2, nil
	}

	withdrawals, err := tableExists(database, "withdrawals")
	if err != nil {
		return 0, err
	}
	if !withdrawals {
		return 3, nil
	}

	systemConfigs, err := tableExists(database, "system_configs")
	if err != nil {
		return 0, err
	}
	if !systemConfigs {
		return 4, nil
	}

	dailyLimit, err := configKeyExists(database, "daily_withdrawal_limit")
	if err != nil {
		return 0, err
	}
	if !dailyLimit {
		return 4, nil
	}

	withdrawalsUUID, err := columnIsUUID(database, "withdrawals", "id")
	if err != nil {
		return 0, err
	}
	if !withdrawalsUUID {
		return 5, nil
	}

	farePlatformFee, err := configKeyExists(database, "fare_platform_fee")
	if err != nil {
		return 0, err
	}
	systemWallet, err := systemWalletExists(database)
	if err != nil {
		return 0, err
	}
	if !farePlatformFee || !systemWallet {
		return 8, nil
	}

	return 10, nil
}

func tableExists(database DB, name string) (bool, error) {
	var exists bool
	err := database.SQL.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)`, name).Scan(&exists)
	return exists, err
}

func indexExists(database DB, name string) (bool, error) {
	var exists bool
	err := database.SQL.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = 'public' AND indexname = $1
		)`, name).Scan(&exists)
	return exists, err
}

func columnIsUUID(database DB, table, column string) (bool, error) {
	var exists bool
	err := database.SQL.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = $1
			  AND column_name = $2
			  AND udt_name = 'uuid'
		)`, table, column).Scan(&exists)
	return exists, err
}

func configKeyExists(database DB, key string) (bool, error) {
	exists, err := tableExists(database, "system_configs")
	if err != nil || !exists {
		return false, err
	}

	var found bool
	err = database.SQL.QueryRow(`SELECT EXISTS (SELECT 1 FROM system_configs WHERE key = $1)`, key).Scan(&found)
	return found, err
}

func systemWalletExists(database DB) (bool, error) {
	var exists bool
	err := database.SQL.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM wallets
			WHERE user_id = '__system__' AND wallet_type = 'system'::wallet_type
		)`).Scan(&exists)
	return exists, err
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
