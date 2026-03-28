package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/co-wallet/backend/internal/config"
	"github.com/co-wallet/backend/internal/db"
	"github.com/co-wallet/backend/internal/handler"
	"github.com/co-wallet/backend/internal/middleware"
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

	// Repositories
	userRepo := repository.NewUserRepository(pool)
	accountRepo := repository.NewAccountRepository(pool)

	// Services
	authSvc := service.NewAuthService(userRepo, cfg.JWTSecret)

	// Seed admin on first launch
	if err = service.SeedAdmin(ctx, userRepo, cfg.AdminUsername, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Fatalf("seed admin: %v", err)
	}

	// Handlers
	authHandler := handler.NewAuthHandler(authSvc, userRepo)
	accountHandler := handler.NewAccountHandler(accountRepo, userRepo)

	// Router
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", handler.Health)

		// Public auth routes
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
			r.Post("/refresh", authHandler.Refresh)
		})

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(authSvc))

			r.Get("/users/me", authHandler.Me)
			r.Patch("/users/me", authHandler.UpdateMe)

			// Accounts
			r.Get("/accounts", accountHandler.List)
			r.Post("/accounts", accountHandler.Create)
			r.Route("/accounts/{accountID}", func(r chi.Router) {
				r.Use(middleware.AccountMember(accountRepo))
				r.Get("/", accountHandler.Get)
				r.Patch("/", accountHandler.Update)
				r.Delete("/", accountHandler.Delete)
				r.Get("/members", accountHandler.ListMembers)
				r.Post("/members", accountHandler.AddMember)
				r.Patch("/members/{userID}", accountHandler.UpdateMember)
				r.Delete("/members/{userID}", accountHandler.RemoveMember)
			})
		})
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
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
