package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/co-wallet/backend/internal/handler"
	accounthandler "github.com/co-wallet/backend/internal/handler/account"
	adminhandler "github.com/co-wallet/backend/internal/handler/admin"
	analyticshandler "github.com/co-wallet/backend/internal/handler/analytics"
	categoryhandler "github.com/co-wallet/backend/internal/handler/category"
	currencyhandler "github.com/co-wallet/backend/internal/handler/currency"
	taghandler "github.com/co-wallet/backend/internal/handler/tag"
	transactionhandler "github.com/co-wallet/backend/internal/handler/transaction"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
)

func newRouter(
	authSvc *service.AuthService,
	accountSvc *service.AccountService,
	categorySvc *service.CategoryService,
	transactionSvc *service.TransactionService,
	tagSvc *service.TagService,
	analyticsSvc *service.AnalyticsService,
	currencySvc *service.CurrencyService,
	adminSvc *service.AdminService,
	userRepo *repository.UserRepository,
	accountRepo *repository.AccountRepository,
) http.Handler {
	authHandler := handler.NewAuthHandler(authSvc, userRepo)
	accountHandler := accounthandler.New(accountSvc)
	categoryHandler := categoryhandler.New(categorySvc)
	transactionHandler := transactionhandler.New(transactionSvc)
	tagHandler := taghandler.New(tagSvc)
	analyticsHandler := analyticshandler.New(analyticsSvc)
	currencyHandler := currencyhandler.New(currencySvc)
	adminHandler := adminhandler.New(adminSvc)

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", handler.Health)

		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
			r.Post("/refresh", authHandler.Refresh)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(authSvc))

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

			r.Get("/currencies", currencyHandler.List)

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
			})
		})
	})

	return r
}
