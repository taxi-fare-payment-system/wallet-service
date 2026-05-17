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

	"wallet_service/internal/auth"
	"wallet_service/internal/config"
	"wallet_service/internal/db"
	"wallet_service/internal/handlers"
	"wallet_service/internal/messaging"
	"wallet_service/internal/payment"
	"wallet_service/internal/repository"
	"wallet_service/internal/server"
	"wallet_service/internal/services"
	"wallet_service/internal/trip"
	"wallet_service/internal/rabbitmq"
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

	if err := db.EnsureSchema(cfg, database, logger); err != nil {
		logger.Error("db_schema_init_failed", slog.Any("error", err))
		os.Exit(1)
	}

	walletRepo := repository.NewWalletRepository(database.Gorm)


	httpClient := &http.Client{Timeout: cfg.HTTPClientTimeout}
	authClient, err := auth.NewClient(cfg.AuthServiceBaseURL, httpClient)
	if err != nil {
		logger.Error("auth_client_init_failed", slog.Any("error", err))
		os.Exit(1)
	}
	paymentClient, err := payment.NewClient(cfg.PaymentServiceBaseURL, httpClient)
	if err != nil {
		logger.Error("payment_client_init_failed", slog.Any("error", err))
		os.Exit(1)
	}

	bus, err := messaging.NewPublisher(cfg.RabbitMQURL, cfg.AnalyticsExchange, cfg.NotificationExchange)
	if err != nil {
		logger.Error("rabbitmq_publisher_init_failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() { _ = bus.Close() }()

	walletService := &services.WalletService{WalletRepo: walletRepo, Bus: bus}
	walletHandlers := &handlers.WalletHandlers{WalletRepo: walletRepo, Bus: bus}
	topupHandlers := &handlers.TopupHandlers{
		WalletRepo:    walletRepo,
		WalletService: walletService,
		AuthClient:    authClient,
		PaymentClient: paymentClient,
		Bus:           bus,
	}
	var tripClient *trip.Client
	if cfg.TripServiceBaseURL != "" {
		tc, err := trip.NewClient(cfg.TripServiceBaseURL, httpClient)
		if err != nil {
			logger.Error("trip_client_init_failed", slog.Any("error", err))
			os.Exit(1)
		}
		tripClient = tc
	}

	payFareHandlers := &handlers.PayFareHandlers{
		WalletRepo:    walletRepo,
		WalletService: walletService,
		PaymentClient: paymentClient,
		TripClient:    tripClient,
		Bus:           bus,
	}
	transactionsHandlers := &handlers.TransactionsHandlers{
		Logger:        logger,
		PaymentClient: paymentClient,
		WalletRepo:    walletRepo,
	}
	configRepo := repository.NewConfigRepository(database.Gorm)
	adminHandlers := &handlers.AdminHandlers{
		WalletRepo: walletRepo,
		ConfigRepo: configRepo,
		AuthClient: authClient,
		Bus:        bus,
	}

	withdrawalRepo := repository.NewWithdrawalRepository(database.Gorm)
	withdrawDeleteHandlers := &handlers.WithdrawDeleteHandlers{
		WalletRepo:     walletRepo,
		WithdrawalRepo: withdrawalRepo,
		ConfigRepo:     configRepo,
		PaymentClient:  paymentClient,
		Bus:            bus,
	}
	assistantHandlers := &handlers.AssistantHandlers{PaymentClient: paymentClient}
	transferHandlers := &handlers.TransferHandlers{
		WalletRepo:    walletRepo,
		WalletService: walletService,
		PaymentClient: paymentClient,
		AuthClient:    authClient,
	}

	router := server.NewRouter(
		logger,
		database.SQL,
		walletHandlers,
		topupHandlers,
		payFareHandlers,
		transactionsHandlers,
		adminHandlers,
		withdrawDeleteHandlers,
		assistantHandlers,
		transferHandlers,
	)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
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
