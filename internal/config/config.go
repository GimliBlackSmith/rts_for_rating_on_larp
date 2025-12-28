package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL    string
	TelegramToken  string
	WebhookURL     string
	WebhookPath    string
	WebhookCert    string
	ServerAddr     string
	MigrateOnStart bool
	ConfigCacheTTL time.Duration
	AdminToken     string
}

func Load() Config {
	return Config{
		DatabaseURL:    getEnv("DATABASE_URL", ""),
		TelegramToken:  getEnv("TELEGRAM_TOKEN", ""),
		WebhookURL:     getEnv("WEBHOOK_URL", ""),
		WebhookPath:    getEnv("WEBHOOK_PATH", "/webhook"),
		WebhookCert:    getEnv("WEBHOOK_CERT", ""),
		ServerAddr:     getEnv("SERVER_ADDR", ":8080"),
		MigrateOnStart: getEnvBool("MIGRATE_ON_START", true),
		ConfigCacheTTL: getEnvDuration("CONFIG_CACHE_TTL", 30*time.Second),
		AdminToken:     getEnv("ADMIN_TOKEN", ""),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
