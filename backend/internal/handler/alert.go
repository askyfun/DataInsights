package handler

import (
	"data-insights/internal/domain/entity"
	"data-insights/internal/router"
	"data-insights/internal/service/alert"
)

// AlertHandler handles the metric alert HTTP surface (issue #155): rule CRUD
// plus trigger history. It mirrors DashboardFolderHandler's shape: thin binding
// layer over the alert service, entity types straight into the Out slots.
type AlertHandler struct {
	svc alert.Service
}

// NewAlertHandler creates a new AlertHandler
func NewAlertHandler(svc alert.Service) *AlertHandler {
	return &AlertHandler{svc: svc}
}

// alertPathIn is the In shape for routes that carry only the {id} path param.
// alert 的 id 是 UUIDv7 字符串（非自增整数），按原样交给 service 解析。
type alertPathIn struct{}

// alertListIn is the In shape of GET /api/alerts. No pagination in this phase,
// so there is nothing to bind — the type exists because the generic router
// needs one.
type alertListIn struct{}

// alertCreateIn is the JSON body of POST /api/alerts. It mirrors
// entity.AlertRuleCreateRequest one-to-one (guarded by contract_parity_test.go)
// with form:"-" on every field so the router's ShouldBindQuery pass cannot read
// query params into the body struct.
type alertCreateIn struct {
	Name      string               `json:"name" form:"-"`
	ChartID   int                  `json:"chart_id" form:"-"`
	Metric    string               `json:"metric" form:"-"`
	Operator  entity.AlertOperator `json:"operator" form:"-"`
	Threshold float64              `json:"threshold" form:"-"`
}

// alertUpdateIn is the JSON body of PUT /api/alerts/:id. Like
// entity.AlertRuleUpdateRequest every field is a pointer: absent (or null)
// means "keep the stored value".
type alertUpdateIn struct {
	Name      *string               `json:"name" form:"-"`
	ChartID   *int                  `json:"chart_id" form:"-"`
	Metric    *string               `json:"metric" form:"-"`
	Operator  *entity.AlertOperator `json:"operator" form:"-"`
	Threshold *float64              `json:"threshold" form:"-"`
	Enabled   *bool                 `json:"enabled" form:"-"`
}

// List handles GET /api/alerts.
func (h *AlertHandler) List(
	req router.Request[alertListIn], res *router.Response[[]entity.AlertRule],
) error {
	rules, err := h.svc.ListRules(req.Ctx.Request.Context())
	if err != nil {
		return err
	}
	if rules == nil {
		rules = []entity.AlertRule{}
	}
	res.Out = rules
	return nil
}

// Get handles GET /api/alerts/:id
func (h *AlertHandler) Get(
	req router.Request[alertPathIn], res *router.Response[*entity.AlertRule],
) error {
	result, err := h.svc.GetRule(req.Ctx.Request.Context(), req.Ctx.Param("id"))
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// Create handles POST /api/alerts. The id is server-generated.
func (h *AlertHandler) Create(
	req router.Request[alertCreateIn], res *router.Response[*entity.AlertRule],
) error {
	result, err := h.svc.CreateRule(req.Ctx.Request.Context(), entity.AlertRuleCreateRequest{
		Name:      req.In.Name,
		ChartID:   req.In.ChartID,
		Metric:    req.In.Metric,
		Operator:  req.In.Operator,
		Threshold: req.In.Threshold,
	})
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// Update handles PUT /api/alerts/:id (merge semantics: unprovided = preserved).
func (h *AlertHandler) Update(
	req router.Request[alertUpdateIn], res *router.Response[*entity.AlertRule],
) error {
	result, err := h.svc.UpdateRule(req.Ctx.Request.Context(), req.Ctx.Param("id"), entity.AlertRuleUpdateRequest{
		Name:      req.In.Name,
		ChartID:   req.In.ChartID,
		Metric:    req.In.Metric,
		Operator:  req.In.Operator,
		Threshold: req.In.Threshold,
		Enabled:   req.In.Enabled,
	})
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// Delete handles DELETE /api/alerts/:id (soft delete, idempotent).
func (h *AlertHandler) Delete(
	req router.Request[alertPathIn], res *router.Response[dashboardStatusOut],
) error {
	if err := h.svc.DeleteRule(req.Ctx.Request.Context(), req.Ctx.Param("id")); err != nil {
		return err
	}
	res.Out = dashboardStatusOut{Status: "ok"}
	return nil
}

// ListTriggers handles GET /api/alerts/:id/triggers (newest first, capped).
func (h *AlertHandler) ListTriggers(
	req router.Request[alertPathIn], res *router.Response[[]entity.AlertTrigger],
) error {
	triggers, err := h.svc.ListTriggers(req.Ctx.Request.Context(), req.Ctx.Param("id"))
	if err != nil {
		return err
	}
	if triggers == nil {
		triggers = []entity.AlertTrigger{}
	}
	res.Out = triggers
	return nil
}
