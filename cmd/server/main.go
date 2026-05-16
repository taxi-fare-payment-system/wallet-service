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

	walletRepo := repository.NewWalletRepository(database.Gorm)

	var publisher *rabbitmq.Publisher
	if cfg.RabbitMQURL != "" {
		p, err := rabbitmq.NewPublisher(cfg.RabbitMQURL, cfg.NotificationExchange)
		if err != nil {
			logger.Error("rabbitmq_publisher_init_failed", slog.Any("error", err))
		} else {
			publisher = p
			logger.Info("rabbitmq_publisher_initialized")
			defer publisher.Close()
		}
	}

	walletHandlers := &handlers.WalletHandlers{WalletRepo: walletRepo}
	walletService := &services.WalletService{WalletRepo: walletRepo, Pub: publisher}

	httpClient := &http.Client{Timeout: cfg.HTTPClientTimeout}
	paymentClient, err := payment.NewClient(cfg.PaymentServiceBaseURL, httpClient)
	if err != nil {
		logger.Error("payment_client_init_failed", slog.Any("error", err))
		os.Exit(1)
	}
	topupHandlers := &handlers.TopupHandlers{
		WalletRepo:    walletRepo,
		WalletService: walletService,
		PaymentClient: paymentClient,
	}
	var tripClient *trip.Client
	if cfg.TripServiceBaseURL != "" {
		tc, err := trip.NewClient(cfg.TripServiceBaseURL, cfg.TripValidatePath, httpClient)
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
	}
	transactionsHandlers := &handlers.TransactionsHandlers{PaymentClient: paymentClient}

	var authClient *auth.Client
	if cfg.AuthServiceBaseURL != "" {
		ac, err := auth.NewClient(cfg.AuthServiceBaseURL, cfg.AuthVerifyAdminPath, httpClient)
		if err != nil {
			logger.Error("auth_client_init_failed", slog.Any("error", err))
			os.Exit(1)
		}
		authClient = ac
	}
	configRepo := repository.NewConfigRepository(database.Gorm)
	adminHandlers := &handlers.AdminHandlers{
		WalletRepo: walletRepo, 
		ConfigRepo: configRepo,
		AuthClient: authClient,
	}
	
	withdrawalRepo := repository.NewWithdrawalRepository(database.Gorm)
	withdrawDeleteHandlers := &handlers.WithdrawDeleteHandlers{
		WalletRepo:     walletRepo,
		WithdrawalRepo: withdrawalRepo,
		ConfigRepo:     configRepo,
	}
	transferHandlers := &handlers.TransferHandlers{
		WalletRepo:    walletRepo,
		WalletService: walletService,
		PaymentClient: paymentClient,
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
