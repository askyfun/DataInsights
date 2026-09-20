package handler

import (
	"strconv"

	"data-insights/internal/domain/entity"
	"data-insights/internal/response"
	"data-insights/internal/router"
	"data-insights/internal/service/chart"
)

// ChartHandler handles chart HTTP requests
type ChartHandler struct {
	svc chart.Service
}

// NewChartHandler creates a new ChartHandler
func NewChartHandler(svc chart.Service) *ChartHandler {
	return &ChartHandler{svc: svc}
}

// chartPathIn is the In shape for routes that carry only path params
// (id): nothing to bind, the id is read from req.Ctx (see the datasource
// package doc, which is the project-wide migration reference).
type chartPathIn struct{}

// chartStatusOut is the {"status":"ok"} payload for Delete (previously
// gin.H{"status": "ok"}).
type chartStatusOut struct {
	Status string `json:"status"`
}

// chartListIn carries the pagination query params for List. Values are
// bound as strings so non-numeric input keeps the pre-migration
// fall-back-to-default behaviour instead of turning into a 400 bind error.
type chartListIn struct {
	Limit  string `form:"limit"`
	Offset string `form:"offset"`
}

// pagination is the List binding post-processing step: it reproduces the
// old getPaginationParams defaults and clamps exactly (limit=100 when
// missing/garbage/out of range, offset>=0).
func (in chartListIn) pagination() (limit, offset int) {
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

// List handles GET /api/charts
func (h *ChartHandler) List(req router.Request[chartListIn], res *router.Response[[]entity.Chart]) error {
	limit, offset := req.In.pagination()

	charts, err := h.svc.List(req.Ctx.Request.Context(), limit, offset)
	if err != nil {
		return err
	}
	if charts == nil {
		charts = []entity.Chart{}
	}
	res.Out = charts
	return nil
}

// Get handles GET /api/charts/:id
func (h *ChartHandler) Get(req router.Request[chartPathIn], res *router.Response[*entity.Chart]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	chart, err := h.svc.GetByID(req.Ctx.Request.Context(), id)
	if err != nil {
		return router.NewBusinessError(response.CodeNotFound, err.Error())
	}
	res.Out = chart
	return nil
}

// chartCreateIn is the JSON body of POST /api/charts. It mirrors entity.Chart
// (the type the pre-migration handler bound directly): the shared domain
// entity cannot carry form:"-" tags, and without them the router's
// ShouldBindQuery pass would fall back to Go field names and let a stray
// query param leak into the body struct. Every field is passed through to
// the service exactly as the old handler did; the name change only surfaces
// in the json bind-error text (accepted unavoidable diff #1).
type chartCreateIn struct {
	ID        int    `json:"id" form:"-"`
	Name      string `json:"name" form:"-"`
	DatasetID int    `json:"dataset_id" form:"-"`
	ChartType string `json:"chart_type" form:"-"`
	Config    string `json:"config" form:"-"`
	CreatedAt string `json:"created_at" form:"-"`
	UpdatedAt string `json:"updated_at" form:"-"`
}

// Create handles POST /api/charts
func (h *ChartHandler) Create(req router.Request[chartCreateIn], res *router.Response[*entity.Chart]) error {
	in := req.In

	chart := &entity.Chart{
		ID:        in.ID,
		Name:      in.Name,
		DatasetID: in.DatasetID,
		ChartType: in.ChartType,
		Config:    in.Config,
		CreatedAt: in.CreatedAt,
		UpdatedAt: in.UpdatedAt,
	}
	// The empty-config default is handler business logic and stays here.
	if chart.Config == "" {
		chart.Config = "{}"
	}

	result, err := h.svc.Create(req.Ctx.Request.Context(), chart)
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// chartUpdateIn is the JSON body of PUT /api/charts/:id (mirroring the old
// anonymous struct); the id arrives via the path, not the In struct.
type chartUpdateIn struct {
	Name      string `json:"name" form:"-"`
	DatasetID int    `json:"dataset_id" form:"-"`
	ChartType string `json:"chart_type" form:"-"`
	Config    string `json:"config" form:"-"`
}

// Update handles PUT /api/charts/:id
func (h *ChartHandler) Update(req router.Request[chartUpdateIn], res *router.Response[*entity.Chart]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	in := req.In

	chart := &entity.Chart{
		ID:        id,
		Name:      in.Name,
		DatasetID: in.DatasetID,
		ChartType: in.ChartType,
		Config:    in.Config,
	}

	result, err := h.svc.Update(req.Ctx.Request.Context(), chart)
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// Delete handles DELETE /api/charts/:id
func (h *ChartHandler) Delete(req router.Request[chartPathIn], res *router.Response[chartStatusOut]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	if err := h.svc.Delete(req.Ctx.Request.Context(), id); err != nil {
		return err
	}
	res.Out = chartStatusOut{Status: "ok"}
	return nil
}

// GetData handles GET /api/charts/:id/data. The Out stays `any` because the
// envelope data is result.Data verbatim (the pre-migration handler passed
// result.Data straight into the success envelope); the service's
// SelectSQL/CountSQL are deliberately not projected into this response.
func (h *ChartHandler) GetData(req router.Request[chartPathIn], res *router.Response[any]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	result, err := h.svc.GetData(req.Ctx.Request.Context(), id)
	if err != nil {
		return err
	}
	res.Out = result.Data
	return nil
}

// chartQueryIn mirrors entity.ChartQueryRequest (the JSON body of
// POST /api/charts/query) with form:"-" on every field so the router's
// ShouldBindQuery pass cannot read query params into the body struct (see
// chartCreateIn and the dataset Query precedent).
// v1/v2 协议超集（裁定2）：spec_version 缺失或 !=2 时消费平铺 dims/metrics；
// spec_version=2 时消费 dimension_groups/metric_groups（槽位语义 + binding_id）。
type chartQueryIn struct {
	DatasetID  int                   `json:"dataset_id" form:"-"`
	ChartType  string                `json:"chart_type" form:"-"`
	Dims       []string              `json:"dims" form:"-"`
	Metrics    []entity.MetricConfig `json:"metrics" form:"-"`
	Filters    []entity.Filter       `json:"filters" form:"-"`
	Pagination *entity.Pagination    `json:"pagination" form:"-"`
	Sort       *entity.SortConfig    `json:"sort" form:"-"`

	SpecVersion     *int                      `json:"spec_version" form:"-"`
	DimensionGroups []entity.DimensionGroupIn `json:"dimension_groups" form:"-"`
	MetricGroups    []entity.MetricGroupIn    `json:"metric_groups" form:"-"`

	QueryOptions map[string]any `json:"query_options,omitempty" form:"-"`
}

func (in chartQueryIn) toRequest() entity.ChartQueryRequest {
	return entity.ChartQueryRequest{
		DatasetID:  in.DatasetID,
		ChartType:  in.ChartType,
		Dims:       in.Dims,
		Metrics:    in.Metrics,
		Filters:    in.Filters,
		Pagination: in.Pagination,
		Sort:       in.Sort,

		SpecVersion:     in.SpecVersion,
		DimensionGroups: in.DimensionGroups,
		MetricGroups:    in.MetricGroups,

		QueryOptions: in.QueryOptions,
	}
}

// Query handles POST /api/charts/query. Like the pre-migration handler the
// envelope data is the whole ChartDataResult (data + select_sql).
func (h *ChartHandler) Query(req router.Request[chartQueryIn], res *router.Response[entity.ChartDataResult]) error {
	chartReq := req.In.toRequest()

	result, err := h.svc.Query(req.Ctx.Request.Context(), &chartReq)
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}
