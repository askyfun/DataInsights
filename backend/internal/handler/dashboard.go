package handler

import (
	"strconv"

	"data-insights/internal/domain/entity"
	"data-insights/internal/response"
	"data-insights/internal/router"
	"data-insights/internal/service/dashboard"

	"github.com/google/uuid"
)

// DashboardHandler handles dashboard HTTP requests: the 12-column grid
// container (CRUD + soft delete) and the reverse "which dashboards embed this
// chart" lookup. It follows the generic-router migration pattern documented in
// the datasource package doc.
type DashboardHandler struct {
	svc dashboard.Service
}

// NewDashboardHandler creates a new DashboardHandler
func NewDashboardHandler(svc dashboard.Service) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

// dashboardPathIn is the In shape for routes that carry only the {id} path
// param: nothing to bind, the id is read from req.Ctx.
type dashboardPathIn struct{}

// dashboardStatusOut is the {"status":"ok"} payload for Delete (previously
// gin.H{"status": "ok"} in the sibling handlers).
type dashboardStatusOut struct {
	Status string `json:"status"`
}

// dashboardListIn carries the pagination query params for List. Values bind as
// strings so non-numeric input keeps the fall-back-to-default behaviour instead
// of turning into a 400 bind error (same contract as chartListIn/datasourceListIn).
type dashboardListIn struct {
	Limit  string `form:"limit"`
	Offset string `form:"offset"`
}

// pagination is the List binding post-processing step: limit=100 when
// missing/garbage/out of range, offset>=0.
func (in dashboardListIn) pagination() (limit, offset int) {
	limit, _ = strconv.Atoi(orDefault(in.Limit, "100"))
	offset, _ = strconv.Atoi(orDefault(in.Offset, "0"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return
}

// parseDashboardID validates the {id} path param.
//
// bi_dashboard.id is a UUID column, so a non-UUID value would otherwise come
// back from PostgreSQL as a cast error (500); it is rejected here as 20100,
// same as every other unparseable :id in this package.
func parseDashboardID(raw string) (string, error) {
	if _, err := uuid.Parse(raw); err != nil {
		return "", router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}
	return raw, nil
}

// List handles GET /api/dashboards
func (h *DashboardHandler) List(req router.Request[dashboardListIn], res *router.Response[[]entity.Dashboard]) error {
	limit, offset := req.In.pagination()

	dashboards, err := h.svc.List(req.Ctx.Request.Context(), limit, offset)
	if err != nil {
		return err
	}
	if dashboards == nil {
		dashboards = []entity.Dashboard{}
	}
	res.Out = dashboards
	return nil
}

// Get handles GET /api/dashboards/:id
func (h *DashboardHandler) Get(req router.Request[dashboardPathIn], res *router.Response[*entity.Dashboard]) error {
	id, err := parseDashboardID(req.Ctx.Param("id"))
	if err != nil {
		return err
	}

	result, err := h.svc.Get(req.Ctx.Request.Context(), id)
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// dashboardCreateIn is the JSON body of POST /api/dashboards. It mirrors
// entity.DashboardCreateRequest one-to-one (guarded by
// contract_parity_test.go) with form:"-" on every field so the router's
// ShouldBindQuery pass cannot read query params into the body struct (see the
// datasource package doc).
type dashboardCreateIn struct {
	Name        string  `json:"name" form:"-"`
	Description *string `json:"description" form:"-"`
	LayoutJSON  string  `json:"layout_json" form:"-"`
	Status      string  `json:"status" form:"-"`
}

// Create handles POST /api/dashboards. The id is server-generated and the
// defaults (layout_json / status) are filled in by the service.
func (h *DashboardHandler) Create(req router.Request[dashboardCreateIn], res *router.Response[*entity.Dashboard]) error {
	in := req.In

	result, err := h.svc.Create(req.Ctx.Request.Context(), entity.DashboardCreateRequest{
		Name:        in.Name,
		Description: in.Description,
		LayoutJSON:  in.LayoutJSON,
		Status:      in.Status,
	})
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// dashboardUpdateIn is the JSON body of PUT /api/dashboards/:id; the id arrives
// via the path, not the In struct. Like entity.DashboardUpdateRequest every
// field is a pointer: absent (or null) means "keep the stored value", which is
// the repository-wide convention for PUT (see AGENTS.md).
type dashboardUpdateIn struct {
	Name        *string `json:"name" form:"-"`
	Description *string `json:"description" form:"-"`
	LayoutJSON  *string `json:"layout_json" form:"-"`
	Status      *string `json:"status" form:"-"`
}

// Update handles PUT /api/dashboards/:id
func (h *DashboardHandler) Update(req router.Request[dashboardUpdateIn], res *router.Response[*entity.Dashboard]) error {
	id, err := parseDashboardID(req.Ctx.Param("id"))
	if err != nil {
		return err
	}

	in := req.In

	result, err := h.svc.Update(req.Ctx.Request.Context(), id, entity.DashboardUpdateRequest{
		Name:        in.Name,
		Description: in.Description,
		LayoutJSON:  in.LayoutJSON,
		Status:      in.Status,
	})
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// Delete handles DELETE /api/dashboards/:id (soft delete).
func (h *DashboardHandler) Delete(req router.Request[dashboardPathIn], res *router.Response[dashboardStatusOut]) error {
	id, err := parseDashboardID(req.Ctx.Param("id"))
	if err != nil {
		return err
	}

	if err := h.svc.Delete(req.Ctx.Request.Context(), id); err != nil {
		return err
	}
	res.Out = dashboardStatusOut{Status: "ok"}
	return nil
}

// ListChartReferences handles GET /api/charts/:id/references. It lives on the
// charts group because the caller is the chart delete dialog, but it is served
// by this handler: the lookup scans bi_dashboard.layout_json.
func (h *DashboardHandler) ListChartReferences(req router.Request[chartPathIn], res *router.Response[entity.ChartReference]) error {
	chartID, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	dashboards, err := h.svc.CountChartReferences(req.Ctx.Request.Context(), chartID)
	if err != nil {
		return err
	}

	// Project to the identity pair the delete-time hint needs; the layout
	// document itself is deliberately not echoed back here.
	refs := entity.ChartReference{
		Count:      len(dashboards),
		Dashboards: make([]entity.ChartReferenceItem, 0, len(dashboards)),
	}
	for _, d := range dashboards {
		refs.Dashboards = append(refs.Dashboards, entity.ChartReferenceItem{ID: d.ID, Name: d.Name})
	}
	res.Out = refs
	return nil
}

// dashboardQueryIn is the In shape of POST /api/dashboards/:id/query. It mirrors
// entity.DashboardQueryRequest one-to-one (guarded by contract_parity_test.go)
// with form:"-" on every field so the router's ShouldBindQuery pass cannot read
// query params into the body struct (see the datasource package doc).
//
// The nested filter entries reuse the entity type directly, exactly like
// datasetQueryIn does with entity.Filter: they are pure data with no binding
// tags of their own to worry about.
type dashboardQueryIn struct {
	Filters []entity.DashboardQueryFilter `json:"filters" form:"-"`
}

// Query handles POST /api/dashboards/:id/query: the dashboard's batch fetch.
// The per-block merge of dashboard filters into the charts' own conditions is a
// single-point server-side algorithm (PRD §8.3) — the handler only forwards the
// filter values and the path id.
func (h *DashboardHandler) Query(
	req router.Request[dashboardQueryIn], res *router.Response[*entity.DashboardQueryResult],
) error {
	id, err := parseDashboardID(req.Ctx.Param("id"))
	if err != nil {
		return err
	}

	result, err := h.svc.Query(req.Ctx.Request.Context(), id, entity.DashboardQueryRequest{
		Filters: req.In.Filters,
	})
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}
