package handler

import (
	"github.com/gin-gonic/gin"

	"data-insights/internal/middleware"
)

// principalOwnerID reads the authenticated principal off the request context
// and returns the user id to stamp as owner_id (R-95, issue #220). Anonymous
// requests return 0 so the service keeps the SystemOwnerID placeholder
// semantics (#171). Create handlers only — Update never rewrites ownership.
func principalOwnerID(c *gin.Context) int {
	if p, ok := middleware.PrincipalFrom(c); ok {
		return p.UserID
	}
	return 0
}
