package handler

import (
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"data-insights/internal/domain/entity"
	"data-insights/internal/export"
	"data-insights/internal/listpage"
	"data-insights/internal/query"
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

// chartListIn carries the list query params. Values are bound as strings so
// non-numeric input keeps the pre-migration fall-back-to-default behaviour
// instead of turning into a 400 bind error.
type chartListIn struct {
	Limit   string `form:"limit"`
	Offset  string `form:"offset"`
	Sort    string `form:"sort"`
	Order   string `form:"order"`
	Keyword string `form:"keyword"`
}

// List handles GET /api/charts
func (h *ChartHandler) List(req router.Request[chartListIn], res *router.Response[listpage.Result[entity.Chart]]) error {
	result, err := h.svc.List(req.Ctx.Request.Context(), listParams(req.In.Limit, req.In.Offset, req.In.Sort, req.In.Order, req.In.Keyword))
	if err != nil {
		return err
	}
	if result.Items == nil {
		result.Items = []entity.Chart{}
	}
	res.Out = result
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

const (
	// 默认导出行数上限（issue #120「行数上限可配（超限截断并提示）」的默认值）。
	defaultExportLimit = 10000
	maxExportLimit     = 100000
)

// chartExportIn 是 POST /api/charts/export 的请求体（issue #120）：
// 查询部分与 /charts/query **同一个** ChartSpecQueryRequest（v1/v2 超集），
// 保证导出与屏幕上的图表走同一条处理器管道；format/limit/name 只影响落盘形态。
type chartExportIn struct {
	Name   string        `json:"name"`
	Format string        `json:"format"`
	Limit  int           `json:"limit"`
	Query  *chartQueryIn `json:"query"`
	// ColumnNames 是列稳定 id → 列名的映射。v1 请求的 dims 发的是列 id
	// （0000i52x），而导出表头要用列名，后台拿不到前端字段表，故由调用方随请求带上；
	// 缺失时回退列 id（不猜、不查库）。
	ColumnNames map[string]string `json:"column_names"`
}

// Export handles POST /api/charts/export（手工注册：响应体是文件流，不是 JSON 信封）。
//
// 行数上限：table 走服务端分页（page=1、page_size=limit），其他图型的负载是聚合
// 结果（行数很小），仍在写盘前裁一次。截断与否通过 X-Export-Truncated 告知前端。
func (h *ChartHandler) Export(c *gin.Context) {
	var in chartExportIn
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, response.CodeBadRequest, "invalid request body: "+err.Error())
		return
	}
	if in.Query == nil {
		response.Error(c, response.CodeBadRequest, "query is required")
		return
	}
	format := strings.ToLower(strings.TrimSpace(in.Format))
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "xlsx" {
		response.Error(c, response.CodeBadRequest, "format must be csv or xlsx")
		return
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultExportLimit
	}
	if limit > maxExportLimit {
		limit = maxExportLimit
	}

	chartReq := in.Query.toRequest()
	// table 的「当前查询」通常是某一页；导出要的是整份结果，因此覆盖分页为
	// 「首页 + limit 行」。其他图型不使用 pagination，保持请求原样。
	if chartReq.ChartType == "table" {
		chartReq.Pagination = &entity.Pagination{Page: 1, PageSize: limit}
	}

	result, err := h.svc.Query(c.Request.Context(), &chartReq)
	if err != nil {
		respondServiceError(c, err)
		return
	}

	table := export.Tabularize(&chartReq, result.Data, in.ColumnNames)
	if table == nil {
		response.Error(c, response.CodeBadRequest, "chart data cannot be exported as a table")
		return
	}
	truncated := false
	if tr, ok := result.Data.(*query.TableResponse); ok && tr.Pagination.Total > limit {
		truncated = true
	}
	if len(table.Rows) > limit {
		table.Rows = table.Rows[:limit]
		truncated = true
	}

	name := export.SanitizeFilename(in.Name)
	if name == "" {
		name = "export"
	}
	ext := format
	disposition := fmt.Sprintf("attachment; filename=\"%s.%s\"; filename*=UTF-8''%s.%s",
		name, ext, url.PathEscape(name), ext)
	c.Header("Content-Disposition", disposition)
	c.Header("X-Export-Rows", strconv.Itoa(len(table.Rows)))
	c.Header("X-Export-Limit", strconv.Itoa(limit))
	if truncated {
		c.Header("X-Export-Truncated", "true")
	} else {
		c.Header("X-Export-Truncated", "false")
	}

	if format == "xlsx" {
		c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		if err := export.WriteXLSX(c.Writer, table, "Data"); err != nil {
			// 头已下发、body 已开始写，无法再改成 JSON 错误信封：记日志即可
			// （AGENTS.md 禁止空错误块）。
			slog.Error("chart export: write xlsx", "name", name, "err", err)
		}
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	if err := export.WriteCSV(c.Writer, table); err != nil {
		slog.Error("chart export: write csv", "name", name, "err", err)
	}
}
