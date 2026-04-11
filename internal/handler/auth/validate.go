package auth

import (
	"fmt"
	"strings"

	"github.com/co-wallet/backend/internal/apperr"
)

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (r *loginReq) normalize() {
	r.Email = strings.ToLower(strings.TrimSpace(r.Email))
}

type refreshReq struct {
	RefreshToken string `json:"refreshToken"`
}

func (r *refreshReq) validate() error {
	if strings.TrimSpace(r.RefreshToken) == "" {
		return fmt.Errorf("%w: refreshToken is required", apperr.ErrValidation)
	}
	return nil
}

type updateMeReq struct {
	DefaultCurrency string `json:"defaultCurrency"`
}
