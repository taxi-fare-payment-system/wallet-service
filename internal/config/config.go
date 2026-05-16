package config

import (
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL           string
	Env                   string
	MigrationsPath        string
	Port                  string
	LogLevel              string
	PaymentServiceBaseURL string
	HTTPClientTimeout     time.Duration
	TripServiceBaseURL    string
	TripValidatePath      string
	AuthServiceBaseURL    string
	AuthVerifyAdminPath   string

	DBMaxOpenConns int
	DBMaxIdleConns int
	DBConnMaxIdle  time.Duration
	DBConnMaxLife  time.Duration

	RabbitMQURL          string
	NotificationExchange string
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		DatabaseURL:           mustGetenv("DATABASE_URL"),
		Env:                   getenvDefault("ENV", "local"),
		MigrationsPath:        getenvDefault("MIGRATIONS_PATH", "file://migrations"),
		Port:                  getenvDefault("PORT", "8081"),
		LogLevel:              getenvDefault("LOG_LEVEL", "info"),
		PaymentServiceBaseURL: mustGetenv("PAYMENT_SERVICE_BASE_URL"),
		HTTPClientTimeout:     getenvDurationDefault("HTTP_CLIENT_TIMEOUT", 10*time.Second),
		TripServiceBaseURL:    getenvDefault("TRIP_SERVICE_BASE_URL", ""),
		TripValidatePath:      getenvDefault("TRIP_VALIDATE_PATH", "/validate-trip-membership"),
		AuthServiceBaseURL:    getenvDefault("AUTH_SERVICE_BASE_URL", ""),
		AuthVerifyAdminPath:   getenvDefault("AUTH_VERIFY_ADMIN_PATH", "/verify-admin"),
		DBMaxOpenConns:        getenvIntDefault("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:        getenvIntDefault("DB_MAX_IDLE_CONNS", 25),
		DBConnMaxIdle:         getenvDurationDefault("DB_CONN_MAX_IDLE", 5*time.Minute),
		DBConnMaxLife:         getenvDurationDefault("DB_CONN_MAX_LIFE", 30*time.Minute),
		RabbitMQURL:          os.Getenv("RABBITMQ_URL"),
		NotificationExchange: getenvDefault("NOTIFICATION_EXCHANGE", "notification_exchange"),
	}

	return cfg, nil
}

func mustGetenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("missing required env var: " + key)
	}
	return v
}

func getenvDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}
