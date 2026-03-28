package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	JWTSecret   string
	Port        string

	AdminUsername string
	AdminEmail    string
	AdminPassword string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/cowallet?sslmode=disable"),
		JWTSecret:     getEnv("JWT_SECRET", "change-me-in-production"),
		Port:          getEnv("PORT", "8080"),
		AdminUsername: getEnv("ADMIN_USERNAME", "admin"),
		AdminEmail:    getEnv("ADMIN_EMAIL", "admin@co-wallet.local"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
