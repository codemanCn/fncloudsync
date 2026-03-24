package config

import (
	"errors"
	"os"
)

type Config struct {
	Addr      string
	DBPath    string
	SecretKey string
}

func Load() (Config, error) {
	cfg := Config{
		Addr:      envOrDefault("APP_ADDR", ":8080"),
		DBPath:    envOrDefault("APP_DB_PATH", "fn-cloudsync.db"),
		SecretKey: os.Getenv("APP_SECRET_KEY"),
	}

	if cfg.SecretKey == "" {
		return Config{}, errors.New("APP_SECRET_KEY is required")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
