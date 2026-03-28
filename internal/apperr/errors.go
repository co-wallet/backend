package apperr

import "errors"

// Sentinel errors — use errors.Is / errors.As to check.
var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrValidation = errors.New("validation error")
)
