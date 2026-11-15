package handler

import (
	"tablewave/internal/router"
	"tablewave/internal/service/setup"
)

// SetupHandler 暴露首启状态探测（R-101 / #226），匿名可访问。
type SetupHandler struct {
	svc *setup.Service
}

func NewSetupHandler(svc *setup.Service) *SetupHandler {
	return &SetupHandler{svc: svc}
}

// Status 返回 {needs_admin, has_sample, has_any_resource, version}。
func (h *SetupHandler) Status(req router.Request[struct{}], res *router.Response[*setup.Status]) error {
	st, err := h.svc.Status(req.Ctx.Request.Context())
	if err != nil {
		return err
	}
	res.Out = st
	return nil
}
