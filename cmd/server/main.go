package main

import (
	"context"
	"log"
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

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect to db: %v", err)
	}
	defer pool.Close()
	log.Println("connected to database")

	migrationsDir := "migrations"
	if dir := os.Getenv("MIGRATIONS_DIR"); dir != "" {
		migrationsDir = dir
	}
	if err = db.RunMigrations(cfg.DatabaseURL, migrationsDir); err != nil {
		log.Fatalf("run migrations: %v", err)
	}
	log.Println("migrations applied")

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
		log.Fatalf("seed admin: %v", err)
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
		log.Printf("server listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	log.Println("server stopped")
}
