package taghandler

import (
	"github.com/co-wallet/backend/internal/httputil"
	"github.com/co-wallet/backend/internal/service"
)

type Handler struct {
	service *service.TagService
}

func New(svc *service.TagService) *Handler {
	return &Handler{service: svc}
}

var (
	jsonResponse       = httputil.JSONResponse
	jsonError          = httputil.JSONError
	handleServiceError = httputil.HandleServiceError
)
