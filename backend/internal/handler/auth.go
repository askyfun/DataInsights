package handler

import (
	"data-insights/internal/domain/entity"
	"data-insights/internal/middleware"
	"data-insights/internal/response"
	"data-insights/internal/router"
	"data-insights/internal/service/auth"
)

// AuthHandler serves the R-82 account endpoints (issue #183): bootstrap
// register, login, self-revocation, and "who am I".
type AuthHandler struct {
	svc auth.Service
}

func NewAuthHandler(svc auth.Service) *AuthHandler { return &AuthHandler{svc: svc} }

type authCredentialsIn struct {
	Username string `json:"username" form:"-"`
	Password string `json:"password" form:"-"`
}

type authRegisterIn struct {
	Username       string `json:"username" form:"-"`
	Password       string `json:"password" form:"-"`
	BootstrapToken string `json:"bootstrap_token" form:"-"`
}

type bootstrapStatusOut struct {
	NeedsBootstrap bool   `json:"needs_bootstrap"`
	EnforceMode    string `json:"enforce_mode"`
}

type recoverAdminIn struct {
	ResetToken  string `json:"reset_token" form:"-"`
	NewPassword string `json:"new_password" form:"-"`
}

type authStatusOut struct {
	Status string `json:"status"`
}

// Register creates the first administrator and returns a session token. It
// requires the one-time bootstrap token; once any admin row exists it refuses
// with 20200.
func (h *AuthHandler) Register(req router.Request[authRegisterIn], res *router.Response[*entity.AuthResult]) error {
	out, err := h.svc.Register(req.Ctx.Request.Context(), req.In.Username, req.In.Password,
		req.In.BootstrapToken, req.Ctx.ClientIP(), req.Ctx.Request.UserAgent())
	if err != nil {
		return err
	}
	res.Out = out
	return nil
}

// BootstrapStatus lets the UI decide between first-run setup and login.
func (h *AuthHandler) BootstrapStatus(req router.Request[struct{}], res *router.Response[*bootstrapStatusOut]) error {
	needs, mode, err := h.svc.BootstrapStatus(req.Ctx.Request.Context())
	if err != nil {
		return err
	}
	res.Out = &bootstrapStatusOut{NeedsBootstrap: needs, EnforceMode: mode}
	return nil
}

// RecoverAdmin resets the first active administrator's password with the
// single-use ADMIN_RESET_TOKEN.
func (h *AuthHandler) RecoverAdmin(req router.Request[recoverAdminIn], res *router.Response[*authStatusOut]) error {
	if err := h.svc.RecoverAdmin(req.Ctx.Request.Context(), req.In.ResetToken, req.In.NewPassword); err != nil {
		return err
	}
	res.Out = &authStatusOut{Status: "ok"}
	return nil
}

// Login exchanges credentials for a 24h session token.
func (h *AuthHandler) Login(req router.Request[authCredentialsIn], res *router.Response[*entity.AuthResult]) error {
	out, err := h.svc.Login(req.Ctx.Request.Context(), req.In.Username, req.In.Password,
		req.Ctx.ClientIP(), req.Ctx.Request.UserAgent())
	if err != nil {
		return err
	}
	res.Out = out
	return nil
}

// Logout revokes the bearer token used to make the call (immediate, no cache).
func (h *AuthHandler) Logout(req router.Request[struct{}], res *router.Response[*authStatusOut]) error {
	if _, ok := middleware.PrincipalFrom(req.Ctx); !ok {
		return router.NewBusinessError(response.CodeUnauthorized, "unauthenticated")
	}
	if err := h.svc.Revoke(req.Ctx.Request.Context(), middleware.RawTokenFrom(req.Ctx)); err != nil {
		return err
	}
	res.Out = &authStatusOut{Status: "ok"}
	return nil
}

// Me returns the authenticated user.
func (h *AuthHandler) Me(req router.Request[struct{}], res *router.Response[*entity.User]) error {
	p, ok := middleware.PrincipalFrom(req.Ctx)
	if !ok {
		return router.NewBusinessError(response.CodeUnauthorized, "unauthenticated")
	}
	out, err := h.svc.CurrentUser(req.Ctx.Request.Context(), p.UserID)
	if err != nil {
		return err
	}
	res.Out = out
	return nil
}
