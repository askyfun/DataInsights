package handler

import (
	"time"

	"dataray/internal/domain/entity"
	"dataray/internal/response"
	"dataray/internal/service/share"

	"github.com/gin-gonic/gin"
)

// ShareHandler handles share HTTP requests
type ShareHandler struct {
	svc share.Service
}

// NewShareHandler creates a new ShareHandler
func NewShareHandler(svc share.Service) *ShareHandler {
	return &ShareHandler{svc: svc}
}

// List handles GET /api/shares
func (h *ShareHandler) List(c *gin.Context) {
	shares, err := h.svc.List(c.Request.Context())
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if shares == nil {
		shares = []entity.Share{}
	}
	response.Success(c, shares)
}

// Create handles POST /api/shares
func (h *ShareHandler) Create(c *gin.Context) {
	var req struct {
		ChartID   int    `json:"chart_id"`
		Password  string `json:"password"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var password *string
	if req.Password != "" {
		password = &req.Password
	}
	var expiresAt *string
	if req.ExpiresAt != "" {
		expiresAt = &req.ExpiresAt
	}

	result, err := h.svc.Create(c.Request.Context(), req.ChartID, password, expiresAt)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

// Get handles GET /api/shares/:token
func (h *ShareHandler) Get(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		response.BadRequest(c, "token is required")
		return
	}

	share, err := h.svc.GetByToken(c.Request.Context(), token)
	if err != nil {
		response.NotFound(c, "share not found")
		return
	}
	response.Success(c, share)
}

// Verify handles POST /api/shares/:token/verify — validates the share password
// and returns the share (chart_id included) on success.
func (h *ShareHandler) Verify(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		response.BadRequest(c, "token is required")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := h.svc.ValidatePassword(c.Request.Context(), token, req.Password); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	share, err := h.svc.GetByToken(c.Request.Context(), token)
	if err != nil {
		response.NotFound(c, "share not found")
		return
	}
	response.Success(c, share)
}

// View handles GET /share/:token
func (h *ShareHandler) View(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		response.BadRequest(c, "token is required")
		return
	}

	share, err := h.svc.GetByToken(c.Request.Context(), token)
	if err != nil {
		response.NotFound(c, "share not found")
		return
	}

	if share.ExpiresAt != nil {
		expiresAt, err := time.Parse(time.RFC3339, *share.ExpiresAt)
		if err == nil && expiresAt.Before(time.Now()) {
			response.BusinessError(c, "share link has expired")
			return
		}
	}

	c.Redirect(302, "/#/share/"+token)
}
