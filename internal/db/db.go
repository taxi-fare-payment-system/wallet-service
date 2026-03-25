package db

import (
	"database/sql"

	"wallet_service/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	Gorm *gorm.DB
	SQL  *sql.DB
}

func Connect(cfg config.Config) (DB, error) {
	gormDB, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return DB{}, err
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return DB{}, err
	}

	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetConnMaxIdleTime(cfg.DBConnMaxIdle)
	sqlDB.SetConnMaxLifetime(cfg.DBConnMaxLife)

	return DB{Gorm: gormDB, SQL: sqlDB}, nil
}
