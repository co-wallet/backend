package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	JWTSecret   string
	Port        string
	AppURL      string // e.g. https://mywallet.example.com — used to build invite links

	AdminUsername string
	AdminEmail    string
	AdminPassword string

	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/cowallet?sslmode=disable"),
		JWTSecret:     getEnv("JWT_SECRET", "change-me-in-production"),
		Port:          getEnv("PORT", "8080"),
		AppURL:        getEnv("APP_URL", "http://localhost:3000"),
		AdminUsername: getEnv("ADMIN_USERNAME", "admin"),
		AdminEmail:    getEnv("ADMIN_EMAIL", "admin@co-wallet.local"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		SMTPHost:      os.Getenv("SMTP_HOST"),
		SMTPPort:      getEnv("SMTP_PORT", "587"),
		SMTPUser:      os.Getenv("SMTP_USER"),
		SMTPPass:      os.Getenv("SMTP_PASS"),
		SMTPFrom:      getEnv("SMTP_FROM", "no-reply@co-wallet.local"),
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
