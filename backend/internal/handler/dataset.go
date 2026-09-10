package handler

import (
	"strconv"

	"dataray/internal/domain/entity"
	"dataray/internal/response"
	"dataray/internal/router"
	"dataray/internal/service/dataset"
)

// DatasetHandler handles dataset HTTP requests
type DatasetHandler struct {
	svc dataset.Service
}

// NewDatasetHandler creates a new DatasetHandler
func NewDatasetHandler(svc dataset.Service) *DatasetHandler {
	return &DatasetHandler{svc: svc}
}

// datasetPathIn is the In shape for routes that carry only the path id:
// nothing to bind, the id is read from req.Ctx (see the datasource package
// doc, which is the project-wide migration reference).
type datasetPathIn struct{}

// datasetStatusOut is the {"status":"ok"} payload for Delete (previously
// gin.H{"status": "ok"}).
type datasetStatusOut struct {
	Status string `json:"status"`
}

// datasetListIn carries the pagination query params for List. Values are
// bound as strings so non-numeric input keeps the pre-migration
// fall-back-to-default behaviour instead of turning into a 400 bind error.
type datasetListIn struct {
	Limit  string `form:"limit"`
	Offset string `form:"offset"`
}

// pagination is the List binding post-processing step: it reproduces the
// old getPaginationParams defaults and clamps exactly (limit=100 when
// missing/garbage/out of range, offset>=0).
func (in datasetListIn) pagination() (limit, offset int) {
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

// List handles GET /api/datasets
func (h *DatasetHandler) List(req router.Request[datasetListIn], res *router.Response[[]entity.Dataset]) error {
	limit, offset := req.In.pagination()

	datasets, err := h.svc.List(req.Ctx.Request.Context(), limit, offset)
	if err != nil {
		return err
	}
	if datasets == nil {
		datasets = []entity.Dataset{}
	}
	res.Out = datasets
	return nil
}

// Get handles GET /api/datasets/:id
func (h *DatasetHandler) Get(req router.Request[datasetPathIn], res *router.Response[*entity.Dataset]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	ds, err := h.svc.GetByID(req.Ctx.Request.Context(), id)
	if err != nil {
		return router.NewBusinessError(response.CodeNotFound, err.Error())
	}
	res.Out = ds
	return nil
}

// datasetCreateIn is the JSON body of POST /api/datasets. Every field
// carries form:"-" so the router's ShouldBindQuery pass cannot touch the
// body struct (see the datasource package doc).
type datasetCreateIn struct {
	Name         string `json:"name" form:"-"`
	DatasourceID int    `json:"datasource_id" form:"-"`
	TableName    string `json:"table_name" form:"-"`
	QuerySQL     string `json:"query_sql" form:"-"`
	QueryType    string `json:"query_type" form:"-"`
	Mode         string `json:"mode" form:"-"`
	Description  string `json:"description" form:"-"`
	Tags         string `json:"tags" form:"-"`
	Columns      string `json:"columns" form:"-"`
	ShardEnabled bool   `json:"shard_enabled" form:"-"`
	ShardKeys    string `json:"shard_keys" form:"-"`
}

// Create handles POST /api/datasets
func (h *DatasetHandler) Create(req router.Request[datasetCreateIn], res *router.Response[*entity.Dataset]) error {
	in := req.In

	ds := &entity.Dataset{
		Name:         in.Name,
		DatasourceID: in.DatasourceID,
		QueryType:    in.QueryType,
		Mode:         in.Mode,
		Tags:         in.Tags,
		Columns:      in.Columns,
		ShardEnabled: in.ShardEnabled,
		ShardKeys:    in.ShardKeys,
	}
	if in.TableName != "" {
		ds.TableName = &in.TableName
	}
	if in.QuerySQL != "" {
		ds.QuerySQL = &in.QuerySQL
	}
	if in.Description != "" {
		ds.Description = &in.Description
	}
	if ds.QueryType == "" {
		ds.QueryType = "table"
	}
	if ds.Mode == "" {
		ds.Mode = "direct"
	}
	if ds.Tags == "" {
		ds.Tags = "[]"
	}
	if ds.QualityRules == "" {
		ds.QualityRules = "[]"
	}
	if ds.Columns == "" {
		ds.Columns = "[]"
	}

	result, err := h.svc.Create(req.Ctx.Request.Context(), ds)
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// Delete handles DELETE /api/datasets/:id
func (h *DatasetHandler) Delete(req router.Request[datasetPathIn], res *router.Response[datasetStatusOut]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	if err := h.svc.Delete(req.Ctx.Request.Context(), id); err != nil {
		return err
	}
	res.Out = datasetStatusOut{Status: "ok"}
	return nil
}

// GetColumns handles GET /api/datasets/:id/columns. The Out keeps the
// entity.DatasetColumn projection (key order follows the struct tags).
func (h *DatasetHandler) GetColumns(req router.Request[datasetPathIn], res *router.Response[[]entity.DatasetColumn]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	columns, err := h.svc.GetColumns(req.Ctx.Request.Context(), id)
	if err != nil {
		return err
	}
	res.Out = columns
	return nil
}

// UpdateColumns handles POST /api/datasets/:id/columns. The In is the bare
// []entity.DatasetColumn slice (not a wrapper object, and deliberately not a
// named alias: the name would leak into the json bind-error text, breaking
// the pre-migration message). ShouldBindQuery on a non-struct In is a no-op,
// so query params cannot reach the body.
func (h *DatasetHandler) UpdateColumns(req router.Request[[]entity.DatasetColumn], res *router.Response[*entity.Dataset]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	result, err := h.svc.UpdateColumns(req.Ctx.Request.Context(), id, req.In)
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// Preview handles GET /api/datasets/:id/preview. Unlike the datasource
// Preview this route is a bodyless GET, so it has no path+body bind shape.
func (h *DatasetHandler) Preview(req router.Request[datasetPathIn], res *router.Response[*entity.PreviewResult]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	result, err := h.svc.Preview(req.Ctx.Request.Context(), id)
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// datasetQueryIn mirrors entity.QueryConfig (the JSON body of
// POST /api/datasets/:id/query) with form:"-" on every field so the
// router's ShouldBindQuery pass cannot read query params into the body
// struct. The entity itself cannot carry the tags (shared domain type),
// so the handler keeps a local mirror and converts.
type datasetQueryIn struct {
	DimensionGroups []entity.FieldGroup `json:"dimension_groups" form:"-"`
	MetricGroups    []entity.FieldGroup `json:"metric_groups" form:"-"`
	Filters         []entity.Filter     `json:"filters" form:"-"`
	Sort            *entity.SortConfig  `json:"sort" form:"-"`
	Limit           int                 `json:"limit" form:"-"`
}

func (in datasetQueryIn) toQueryConfig() entity.QueryConfig {
	return entity.QueryConfig{
		DimensionGroups: in.DimensionGroups,
		MetricGroups:    in.MetricGroups,
		Filters:         in.Filters,
		Sort:            in.Sort,
		Limit:           in.Limit,
	}
}

// Query handles POST /api/datasets/:id/query
func (h *DatasetHandler) Query(req router.Request[datasetQueryIn], res *router.Response[[]map[string]any]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	result, err := h.svc.Query(req.Ctx.Request.Context(), id, req.In.toQueryConfig())
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}
