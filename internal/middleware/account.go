package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/co-wallet/backend/internal/repository"
)

// AccountMember ensures the current user is a member (or owner) of the account in the URL.
func AccountMember(accounts *repository.AccountRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			accountID := chi.URLParam(r, "accountID")
			userID := UserIDFromCtx(r.Context())

			ok, err := accounts.IsMember(r.Context(), accountID, userID)
			if err != nil || !ok {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
