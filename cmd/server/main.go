package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/co-wallet/backend/internal/config"
	"github.com/co-wallet/backend/internal/db"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
)

const defaultJWTSecret = "change-me-in-production"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	if cfg.JWTSecret == defaultJWTSecret {
		slog.Warn("JWT_SECRET is using the default placeholder — set a strong secret in production")
	}

	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect to db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("connected to database")

	migrationsDir := "migrations"
	if dir := os.Getenv("MIGRATIONS_DIR"); dir != "" {
		migrationsDir = dir
	}
	if err = db.RunMigrations(cfg.DatabaseURL, migrationsDir); err != nil {
		slog.Error("run migrations", "err", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")

	userRepo := repository.NewUserRepository(pool)
	accountRepo := repository.NewAccountRepository(pool)
	categoryRepo := repository.NewCategoryRepository(pool)
	transactionRepo := repository.NewTransactionRepository(pool)
	tagRepo := repository.NewTagRepository(pool)
	analyticsRepo := repository.NewAnalyticsRepository(pool)
	currencyRepo := repository.NewCurrencyRepository(pool)
	adminRepo := repository.NewAdminRepository(pool)
	inviteRepo := repository.NewInviteRepository(pool)

	userSvc := service.NewUserService(userRepo)
	authSvc := service.NewAuthService(userRepo, cfg.JWTSecret)
	accountSvc := service.NewAccountService(pool, accountRepo, userRepo)
	categorySvc := service.NewCategoryService(categoryRepo)
	tagSvc := service.NewTagService(tagRepo)
	transactionSvc := service.NewTransactionService(transactionRepo, accountRepo, tagRepo)
	analyticsSvc := service.NewAnalyticsService(analyticsRepo)
	currencySvc := service.NewCurrencyService(currencyRepo)
	adminSvc := service.NewAdminService(adminRepo, currencySvc)
	inviteSvc := service.NewInviteService(pool, inviteRepo, userRepo, authSvc, service.SMTPConfig{
		Host: cfg.SMTPHost,
		Port: cfg.SMTPPort,
		User: cfg.SMTPUser,
		Pass: cfg.SMTPPass,
		From: cfg.SMTPFrom,
	}, cfg.AppURL)

	if err = service.SeedAdmin(ctx, userRepo, cfg.AdminUsername, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		slog.Error("seed admin", "err", err)
		os.Exit(1)
	}

	currencySvc.StartRateFetcher(ctx)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      newRouter(authSvc, userSvc, accountSvc, categorySvc, transactionSvc, tagSvc, analyticsSvc, currencySvc, adminSvc, inviteSvc, accountRepo),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "err", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}
