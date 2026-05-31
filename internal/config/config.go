package config

import (
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL           string
	Env                   string
	MigrationsPath        string
	Port                  string
	LogLevel              string
	AuthServiceBaseURL    string
	PaymentServiceBaseURL string
	HTTPClientTimeout     time.Duration
	TripServiceBaseURL    string
	RabbitMQURL           string
	AnalyticsExchange     string
	NotificationExchange  string

	DBMaxOpenConns int
	DBMaxIdleConns int
	DBConnMaxIdle  time.Duration
	DBConnMaxLife  time.Duration


}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		DatabaseURL:           mustGetenv("DATABASE_URL"),
		Env:                   getenvDefault("ENV", "local"),
		MigrationsPath:        getenvDefault("MIGRATIONS_PATH", "file://migrations"),
		Port:                  getenvDefault("PORT", "8088"),
		LogLevel:              getenvDefault("LOG_LEVEL", "info"),
		AuthServiceBaseURL:    mustGetenvAny([]string{"SERVICE_AUTH_URL", "AUTH_SERVICE_BASE_URL"}),
		PaymentServiceBaseURL: mustGetenvAny([]string{"SERVICE_PAYMENT_URL", "PAYMENT_SERVICE_BASE_URL"}),
		HTTPClientTimeout:     getenvDurationDefault("HTTP_CLIENT_TIMEOUT", 10*time.Second),
		TripServiceBaseURL:    getenvAnyDefault([]string{"SERVICE_TRIP_URL", "TRIP_SERVICE_BASE_URL"}, ""),
		RabbitMQURL:           getenvDefault("RABBITMQ_URL", ""),
		AnalyticsExchange: getenvAnyDefault(
			[]string{"ANALYTICS_EXCHANGE", "RABBITMQ_EXCHANGE_ANALYTICS"},
			"analytics_exchange",
		),
		NotificationExchange: getenvAnyDefault(
			[]string{"NOTIFICATION_EXCHANGE", "RABBITMQ_EXCHANGE_NOTIFICATION"},
			"notification_exchange",
		),
		DBMaxOpenConns:        getenvIntDefault("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:        getenvIntDefault("DB_MAX_IDLE_CONNS", 25),
		DBConnMaxIdle:         getenvDurationDefault("DB_CONN_MAX_IDLE", 5*time.Minute),
		DBConnMaxLife:         getenvDurationDefault("DB_CONN_MAX_LIFE", 30*time.Minute),

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

func mustGetenvAny(keys []string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	panic("missing required env vars: " + strings.Join(keys, ", "))
}

func getenvDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func getenvAnyDefault(keys []string, def string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return def
}
