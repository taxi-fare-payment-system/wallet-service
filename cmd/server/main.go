package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wallet_service/internal/config"
	"wallet_service/internal/db"
	"wallet_service/internal/handlers"
	"wallet_service/internal/httpx"
	"wallet_service/internal/repository"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.LogLevel),
	}))

	database, err := db.Connect(cfg)
	if err != nil {
		logger.Error("db_connect_failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer database.SQL.Close()

	mux := http.NewServeMux()
	walletRepo := repository.NewWalletRepository(database.Gorm)
	walletHandlers := &handlers.WalletHandlers{WalletRepo: walletRepo}

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := database.SQL.PingContext(ctx); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"not_ready"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Wallet APIs (Milestone 2)
	mux.HandleFunc("GET /{id}", walletHandlers.GetWallet)
	mux.HandleFunc("GET /users/{userId}", walletHandlers.GetWalletByUser)
	mux.HandleFunc("POST /", walletHandlers.CreateWallet)

	handler := httpx.RequestIDMiddleware(httpx.AccessLogMiddleware(logger)(mux))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("server_listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server_failed", slog.Any("error", err))
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("server_shutting_down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server_shutdown_failed", slog.Any("error", err))
		_ = srv.Close()
	}
}

func parseLogLevel(v string) slog.Level {
	switch v {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
