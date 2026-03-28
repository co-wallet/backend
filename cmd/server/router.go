package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/co-wallet/backend/internal/handler"
	accounthandler "github.com/co-wallet/backend/internal/handler/account"
	categoryhandler "github.com/co-wallet/backend/internal/handler/category"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
)

func newRouter(
	authSvc *service.AuthService,
	accountSvc *service.AccountService,
	categorySvc *service.CategoryService,
	userRepo *repository.UserRepository,
	accountRepo *repository.AccountRepository,
) http.Handler {
	authHandler := handler.NewAuthHandler(authSvc, userRepo)
	accountHandler := accounthandler.New(accountSvc)
	categoryHandler := categoryhandler.New(categorySvc)

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
		})
	})

	return r
}
