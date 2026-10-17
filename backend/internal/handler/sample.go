package handler

import (
	"data-insights/internal/router"
	"data-insights/internal/service/sample"
)

// SampleHandler exposes the built-in sample dataset (R-79 / issue #174).
type SampleHandler struct {
	svc sample.Service
}

func NewSampleHandler(svc sample.Service) *SampleHandler {
	return &SampleHandler{svc: svc}
}

type samplePathIn struct{}

type sampleStatusOut struct {
	Status string `json:"status"`
}

// Status handles GET /api/sample-dataset/status.
func (h *SampleHandler) Status(req router.Request[samplePathIn], res *router.Response[*sample.Status]) error {
	status, err := h.svc.Status(req.Ctx.Request.Context())
	if err != nil {
		return err
	}
	res.Out = status
	return nil
}

// Load handles POST /api/sample-dataset/load. Idempotent: loading twice
// returns the already-loaded status instead of duplicating rows.
func (h *SampleHandler) Load(req router.Request[samplePathIn], res *router.Response[*sample.Status]) error {
	status, err := h.svc.Load(req.Ctx.Request.Context())
	if err != nil {
		return err
	}
	res.Out = status
	return nil
}

// Dismiss handles POST /api/sample-dataset/dismiss (persisted "ignore").
func (h *SampleHandler) Dismiss(req router.Request[samplePathIn], res *router.Response[*sample.Status]) error {
	status, err := h.svc.Dismiss(req.Ctx.Request.Context())
	if err != nil {
		return err
	}
	res.Out = status
	return nil
}

// Remove handles DELETE /api/sample-dataset (one-click removal).
func (h *SampleHandler) Remove(req router.Request[samplePathIn], res *router.Response[sampleStatusOut]) error {
	if err := h.svc.Remove(req.Ctx.Request.Context()); err != nil {
		return err
	}
	res.Out = sampleStatusOut{Status: "ok"}
	return nil
}
