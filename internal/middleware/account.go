package middleware

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:generate mockgen -source=account.go -destination=mocks/mock_member_checker.go -package=mocks

type memberChecker interface {
	IsMember(ctx context.Context, accountID, userID string) (bool, error)
}

// AccountMember ensures the current user is a member (or owner) of the account in the URL.
func AccountMember(checker memberChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			accountID := chi.URLParam(r, "accountID")
			userID := UserIDFromCtx(r.Context())

			ok, err := checker.IsMember(r.Context(), accountID, userID)
			if err != nil || !ok {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
