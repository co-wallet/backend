package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"golang.org/x/time/rate"

	"github.com/co-wallet/backend/internal/handler"
	accounthandler "github.com/co-wallet/backend/internal/handler/account"
	adminhandler "github.com/co-wallet/backend/internal/handler/admin"
	analyticshandler "github.com/co-wallet/backend/internal/handler/analytics"
	authhandler "github.com/co-wallet/backend/internal/handler/auth"
	categoryhandler "github.com/co-wallet/backend/internal/handler/category"
	currencyhandler "github.com/co-wallet/backend/internal/handler/currency"
	invitehandler "github.com/co-wallet/backend/internal/handler/invite"
	taghandler "github.com/co-wallet/backend/internal/handler/tag"
	transactionhandler "github.com/co-wallet/backend/internal/handler/transaction"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
)

func newRouter(
	authSvc *service.AuthService,
	userSvc *service.UserService,
	accountSvc *service.AccountService,
	categorySvc *service.CategoryService,
	transactionSvc *service.TransactionService,
	tagSvc *service.TagService,
	analyticsSvc *service.AnalyticsService,
	currencySvc *service.CurrencyService,
	adminSvc *service.AdminService,
	inviteSvc *service.InviteService,
	accountRepo *repository.AccountRepository,
) http.Handler {
	authHandler := authhandler.New(authSvc, userSvc)
	accountHandler := accounthandler.New(accountSvc, userSvc)
	categoryHandler := categoryhandler.New(categorySvc)
	transactionHandler := transactionhandler.New(transactionSvc)
	tagHandler := taghandler.New(tagSvc)
	analyticsHandler := analyticshandler.New(analyticsSvc, userSvc)
	currencyHandler := currencyhandler.New(currencySvc)
	adminHandler := adminhandler.New(adminSvc)
	inviteHandler := invitehandler.New(inviteSvc)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// 5 req/s with burst of 10 per IP, 5-minute idle eviction — applied to auth endpoints.
	authLimiter := middleware.RateLimit(rate.Limit(5), 10, 5*time.Minute)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", handler.Health)

		r.Route("/auth", func(r chi.Router) {
			r.Use(authLimiter)
			// /register removed — accounts are created via invite only
			r.Post("/login", authHandler.Login)
			r.Post("/refresh", authHandler.Refresh)
		})

		// Public endpoints (no auth required)
		r.Get("/invites/{token}", inviteHandler.Validate)
		r.With(authLimiter).Post("/invites/{token}/accept", inviteHandler.Accept)
		r.Get("/currencies", currencyHandler.List)

		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(authSvc))

			r.Get("/users", authHandler.ListUsers)
			r.Get("/users/me", authHandler.Me)
			r.Patch("/users/me", authHandler.UpdateMe)

			r.Get("/categories", categoryHandler.List)
			r.Post("/categories", categoryHandler.Create)
			r.Route("/categories/{categoryID}", func(r chi.Router) {
				r.Patch("/", categoryHandler.Update)
				r.Delete("/", categoryHandler.Delete)
			})

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

			r.Get("/transactions", transactionHandler.List)
			r.Post("/transactions", transactionHandler.Create)
			r.Route("/transactions/{transactionID}", func(r chi.Router) {
				r.Get("/", transactionHandler.Get)
				r.Patch("/", transactionHandler.Update)
				r.Delete("/", transactionHandler.Delete)
			})

			r.Get("/tags", tagHandler.List)
			r.Route("/tags/{tagID}", func(r chi.Router) {
				r.Patch("/", tagHandler.Rename)
				r.Delete("/", tagHandler.Delete)
			})

			r.Route("/analytics", func(r chi.Router) {
				r.Get("/summary", analyticsHandler.Summary)
				r.Get("/by-category", analyticsHandler.ByCategory)
				r.Get("/by-tag", analyticsHandler.ByTag)
			})

			// Admin routes
			r.Route("/admin", func(r chi.Router) {
				r.Use(middleware.Admin)

				r.Get("/users", adminHandler.ListUsers)
				r.Route("/users/{userID}", func(r chi.Router) {
					r.Get("/", adminHandler.GetUser)
					r.Patch("/", adminHandler.UpdateUser)
				})

				r.Get("/currencies", adminHandler.ListCurrencies)
				r.Post("/currencies", adminHandler.CreateCurrency)
				r.Patch("/currencies/{code}", adminHandler.UpdateCurrency)
				r.Post("/currencies/rates/refresh", adminHandler.RefreshRates)

				r.Get("/invites", inviteHandler.List)
				r.Post("/invites", inviteHandler.Create)
			})
		})
	})

	return r
}
