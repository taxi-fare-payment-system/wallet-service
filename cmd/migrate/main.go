package main

import (
	"fmt"
	"os"

	"wallet_service/internal/config"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	gormDB, err := gorm.Open(gormpg.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	driver, err := migratepg.WithInstance(sqlDB, &migratepg.Config{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	m, err := migrate.NewWithDatabaseInstance(cfg.MigrationsPath, "postgres", driver)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cmd := "up"
	if len(os.Args) >= 2 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/migrate [up|down]")
		os.Exit(2)
	}
}
