package categoryhandler

import "github.com/co-wallet/backend/internal/service"

// CategoryResponse mirrors service.CategoryNode for the HTTP layer.
// Using service.CategoryNode directly keeps the tree shape intact.
type CategoryResponse = service.CategoryNode
