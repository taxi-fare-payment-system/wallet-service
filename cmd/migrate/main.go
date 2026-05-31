package main

import (
	"fmt"
	"log/slog"
	"os"

	"wallet_service/internal/config"
	"wallet_service/internal/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	database, err := db.Connect(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer database.SQL.Close()

	cmd := "up"
	if len(os.Args) >= 2 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "up":
		if err := db.EnsureSchema(cfg, database, logger); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "down":
		if err := db.RunMigrationsDown(cfg, database, logger); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/migrate [up|down]")
		os.Exit(2)
	}
}
