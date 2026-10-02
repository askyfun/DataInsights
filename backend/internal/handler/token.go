package handler

import (
	"strconv"

	"data-insights/internal/domain/entity"
	"data-insights/internal/middleware"
	"data-insights/internal/response"
	"data-insights/internal/router"
	"data-insights/internal/service/auth"
)

// TokenHandler serves the PAT management endpoints (issue #184). EVERY route
// here is behind middleware.Bearer(allowPAT=false): these are user-management
// operations, so a long-lived machine token is refused — you must present a
// human session to create/list/revoke PATs (#184 acceptance 3).
type TokenHandler struct {
	svc auth.Service
}

func NewTokenHandler(svc auth.Service) *TokenHandler { return &TokenHandler{svc: svc} }

type patCreateIn struct {
	Name  string `json:"name" form:"-"`
	Never bool   `json:"never" form:"-"`
}

type tokenStatusOut struct {
	Status string `json:"status"`
}

// Create mints a PAT and returns its plaintext exactly once.
func (h *TokenHandler) Create(req router.Request[patCreateIn], res *router.Response[*entity.PATCreateResult]) error {
	p, ok := middleware.PrincipalFrom(req.Ctx)
	if !ok {
		return router.NewBusinessError(response.CodeUnauthorized, "unauthenticated")
	}
	out, err := h.svc.CreatePAT(req.Ctx.Request.Context(), p.UserID, req.In.Name, req.In.Never)
	if err != nil {
		return err
	}
	res.Out = out
	return nil
}

// List returns the caller's PATs (metadata only, never a secret).
func (h *TokenHandler) List(req router.Request[struct{}], res *router.Response[[]entity.TokenInfo]) error {
	p, ok := middleware.PrincipalFrom(req.Ctx)
	if !ok {
		return router.NewBusinessError(response.CodeUnauthorized, "unauthenticated")
	}
	out, err := h.svc.ListPATs(req.Ctx.Request.Context(), p.UserID)
	if err != nil {
		return err
	}
	res.Out = out
	return nil
}

// Revoke revokes one of the caller's PATs by id (idempotent, self-scoped).
func (h *TokenHandler) Revoke(req router.Request[struct{}], res *router.Response[*tokenStatusOut]) error {
	p, ok := middleware.PrincipalFrom(req.Ctx)
	if !ok {
		return router.NewBusinessError(response.CodeUnauthorized, "unauthenticated")
	}
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}
	if err := h.svc.RevokeByID(req.Ctx.Request.Context(), p.UserID, id); err != nil {
		return err
	}
	res.Out = &tokenStatusOut{Status: "ok"}
	return nil
}
