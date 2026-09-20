package handler

import (
	"time"

	"data-insights/internal/domain/entity"
	"data-insights/internal/response"
	"data-insights/internal/router"
	"data-insights/internal/service/share"

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

// sharePathIn is the In shape for routes that need nothing bound: the List
// route carries no params and the token routes read :token from req.Ctx
// (see the datasource package doc, which is the project-wide migration
// reference).
type sharePathIn struct{}

// List handles GET /api/shares
func (h *ShareHandler) List(req router.Request[sharePathIn], res *router.Response[[]entity.Share]) error {
	shares, err := h.svc.List(req.Ctx.Request.Context())
	if err != nil {
		return err
	}
	if shares == nil {
		shares = []entity.Share{}
	}
	res.Out = shares
	return nil
}

// shareCreateIn is the JSON body of POST /api/shares. Every field carries
// form:"-" so the router's ShouldBindQuery pass cannot touch the body struct
// (see the datasource package doc).
type shareCreateIn struct {
	ChartID   int    `json:"chart_id" form:"-"`
	Password  string `json:"password" form:"-"`
	ExpiresAt string `json:"expires_at" form:"-"`
}

// Create handles POST /api/shares
func (h *ShareHandler) Create(req router.Request[shareCreateIn], res *router.Response[*entity.Share]) error {
	in := req.In

	var password *string
	if in.Password != "" {
		password = &in.Password
	}
	var expiresAt *string
	if in.ExpiresAt != "" {
		expiresAt = &in.ExpiresAt
	}

	result, err := h.svc.Create(req.Ctx.Request.Context(), in.ChartID, password, expiresAt)
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// Get handles GET /api/shares/:token
func (h *ShareHandler) Get(req router.Request[sharePathIn], res *router.Response[*entity.Share]) error {
	token := req.Ctx.Param("token")
	if token == "" {
		return router.NewBusinessError(response.CodeBadRequest, "token is required")
	}

	share, err := h.svc.GetByToken(req.Ctx.Request.Context(), token)
	if err != nil {
		return router.NewBusinessError(response.CodeNotFound, "share not found")
	}
	res.Out = share
	return nil
}

// shareVerifyIn is the JSON body of POST /api/shares/:token/verify.
type shareVerifyIn struct {
	Password string `json:"password" form:"-"`
}

// Verify handles POST /api/shares/:token/verify — validates the share password
// and returns the share (chart_id included) on success.
func (h *ShareHandler) Verify(req router.Request[shareVerifyIn], res *router.Response[*entity.Share]) error {
	token := req.Ctx.Param("token")
	if token == "" {
		return router.NewBusinessError(response.CodeBadRequest, "token is required")
	}

	if err := h.svc.ValidatePassword(req.Ctx.Request.Context(), token, req.In.Password); err != nil {
		return router.NewBusinessError(response.CodeBadRequest, err.Error())
	}

	share, err := h.svc.GetByToken(req.Ctx.Request.Context(), token)
	if err != nil {
		return router.NewBusinessError(response.CodeNotFound, "share not found")
	}
	res.Out = share
	return nil
}

// View handles GET /share/:token. It is deliberately NOT migrated to the
// generic router: its success response is a 302 redirect, which the
// JSON-envelope router cannot emit (registering it there would append a
// success envelope to the redirect body). It stays outside the 30 /api
// endpoints, in the same exemption class as the health route.
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
