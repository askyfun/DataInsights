// Package handler contains the DataRay HTTP handlers.
//
// Batch 2 migration pattern — converting a handler to the generic router
// (dataray/internal/router). The datasource handlers in this file are the
// project-wide reference; copy this pattern for the dataset/chart/share
// domains. Hard rule: zero observable behavior change. The exact response
// bodies are pinned byte-for-byte by datasource_test.go (envelope
// {code,msg,trace,data}, HTTP always 200); after migrating an endpoint,
// re-wire only the registration lines in the test router helper — every
// assertion must keep passing untouched.
//
// # Handler signature
//
//	func (h *DatasourceHandler) Get(req router.Request[datasourcePathIn],
//	    res *router.Response[*entity.Datasource]) error
//
// Registration in cmd/routes.go: router.RegisterGetRoute / RegisterPostRoute
// / RegisterPutRoute / RegisterDeleteRoute(ds, path, h.Method) — In/Out are
// inferred from the method value; never call response.* from inside an
// migrated handler, the router wraps success and errors.
//
// # In struct rules
//
//   - Query params: fields with `form` tags. Declare them as string and
//     parse in a post-bind method that mirrors the old DefaultQuery+strconv
//     clamping (see datasourceListIn.pagination). Binding int directly is a
//     behavior change: a garbage ?limit=abc would answer 20100 where the old
//     handler silently used the default 100.
//   - Path params (:id, :table): NOT part of In. Read them via
//     req.Ctx.Param("id") inside the handler and parse there; an unparseable
//     id keeps returning BusinessError(CodeBadRequest, "invalid id").
//   - JSON body: the In struct itself. Give every body field `form:"-"`,
//     because gin's ShouldBindQuery (which the router always runs first)
//     falls back to the Go field name as key when no form tag exists —
//     without form:"-" a stray ?Name=evil would leak into the body struct
//     that the old handler never read from.
//   - Body binding gate: the router binds JSON for POST/PUT/PATCH only
//     (unconditionally, so an empty body yields the same 20100/"EOF" as the
//     old unconditional ShouldBindJSON); GET/DELETE never bind a body.
//
// # Out rules
//
// res.Out carries the payload only; the router wraps it in the unified
// success envelope. Response's normalizer turns nil slices/maps into
// [] / {} and nil root data into {}, so a typed nil slice Out already
// serializes as [] —
// keep any explicit nil guards only where the old code had them. Preserve
// map vs struct projection choices: GetColumns deliberately returns
// []map[string]any so key order stays alphabetical byte-for-byte; switching
// it to a struct would reorder the JSON keys.
//
// # Error mapping table
//
//	old response.BadRequest(c, msg)      → return router.NewBusinessError(response.CodeBadRequest, msg)
//	old response.NotFound(c, msg)       → return router.NewBusinessError(response.CodeNotFound, msg)
//	old response.InternalError(c, msg)  → return err (bare; the router's fallback calls
//	                                      response.InternalError(c, err.Error()) — identical body incl. trace)
//	other old response.Xxx(c, msg)      → NewBusinessError with that same code constant
//
// Do not "improve" existing semantics: every GetByID error currently maps
// to 404, TestConnection failures currently map to 400 — keep both.
//
// Known unavoidable diff (accepted): JSON type-mismatch bind errors embed
// the Go struct name, so "…Go struct field .port…" now reads
// "…Go struct field datasourceCreateIn.port…". Every code/msg/data shape on
// the normal contract paths is unchanged.
package handler

import (
	"strconv"

	"dataray/internal/domain/entity"
	"dataray/internal/response"
	"dataray/internal/router"
	"dataray/internal/service/datasource"
)

// DatasourceHandler handles datasource HTTP requests
type DatasourceHandler struct {
	svc datasource.Service
}

// NewDatasourceHandler creates a new DatasourceHandler
func NewDatasourceHandler(svc datasource.Service) *DatasourceHandler {
	return &DatasourceHandler{svc: svc}
}

// datasourcePathIn is the In shape for routes that carry only path params
// (id / table): nothing to bind, params are read from req.Ctx.
type datasourcePathIn struct{}

// datasourceStatusOut is the shared {"status":"ok"} payload for Delete and
// TestConnection (previously gin.H{"status": "ok"}).
type datasourceStatusOut struct {
	Status string `json:"status"`
}

// orDefault mirrors gin's DefaultQuery for a value already bound via a form
// tag: the empty string means "param absent" and falls back to the default.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// datasourceListIn carries the pagination query params for List. Values are
// bound as strings so non-numeric input keeps the pre-migration
// fall-back-to-default behaviour instead of turning into a 400 bind error.
type datasourceListIn struct {
	Limit  string `form:"limit"`
	Offset string `form:"offset"`
}

// pagination is the List binding post-processing step: it reproduces the
// old getPaginationParams defaults and clamps exactly (limit=100 when
// missing/garbage/out of range, offset>=0).
func (in datasourceListIn) pagination() (limit, offset int) {
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

// List handles GET /api/datasources
func (h *DatasourceHandler) List(req router.Request[datasourceListIn], res *router.Response[[]entity.Datasource]) error {
	limit, offset := req.In.pagination()

	datasources, err := h.svc.List(req.Ctx.Request.Context(), limit, offset)
	if err != nil {
		return err
	}
	if datasources == nil {
		datasources = []entity.Datasource{}
	}
	res.Out = datasources
	return nil
}

// Get handles GET /api/datasources/:id
func (h *DatasourceHandler) Get(req router.Request[datasourcePathIn], res *router.Response[*entity.Datasource]) error {
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

// datasourceCreateIn is the JSON body of POST /api/datasources. Every field
// carries form:"-" so the router's ShouldBindQuery pass cannot touch the
// body struct (see the package doc).
type datasourceCreateIn struct {
	Name         string `json:"name" form:"-"`
	Type         string `json:"type" form:"-"`
	Host         string `json:"host" form:"-"`
	Port         int    `json:"port" form:"-"`
	DatabaseName string `json:"database_name" form:"-"`
	Username     string `json:"username" form:"-"`
	Password     string `json:"password" form:"-"`
}

// Create handles POST /api/datasources
func (h *DatasourceHandler) Create(req router.Request[datasourceCreateIn], res *router.Response[*entity.Datasource]) error {
	in := req.In

	if in.Type == "" {
		in.Type = "postgresql"
	}

	ds := &entity.Datasource{
		Name:         in.Name,
		Type:         in.Type,
		Host:         in.Host,
		Port:         in.Port,
		DatabaseName: in.DatabaseName,
		Username:     in.Username,
		Password:     in.Password,
	}

	result, err := h.svc.Create(req.Ctx.Request.Context(), ds)
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// datasourceUpdateIn is the JSON body of PUT /api/datasources/:id; the id
// arrives via the path, not the In struct.
type datasourceUpdateIn struct {
	Name         string `json:"name" form:"-"`
	Type         string `json:"type" form:"-"`
	Host         string `json:"host" form:"-"`
	Port         int    `json:"port" form:"-"`
	DatabaseName string `json:"database_name" form:"-"`
	Username     string `json:"username" form:"-"`
	Password     string `json:"password" form:"-"`
}

// Update handles PUT /api/datasources/:id
func (h *DatasourceHandler) Update(req router.Request[datasourceUpdateIn], res *router.Response[*entity.Datasource]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	in := req.In

	ds := &entity.Datasource{
		ID:           id,
		Name:         in.Name,
		Type:         in.Type,
		Host:         in.Host,
		Port:         in.Port,
		DatabaseName: in.DatabaseName,
		Username:     in.Username,
		Password:     in.Password,
	}

	result, err := h.svc.Update(req.Ctx.Request.Context(), ds)
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// Delete handles DELETE /api/datasources/:id
func (h *DatasourceHandler) Delete(req router.Request[datasourcePathIn], res *router.Response[datasourceStatusOut]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	if err := h.svc.Delete(req.Ctx.Request.Context(), id); err != nil {
		return err
	}
	res.Out = datasourceStatusOut{Status: "ok"}
	return nil
}

// datasourceTestConnectionIn is the JSON body of POST /api/datasources/test.
type datasourceTestConnectionIn struct {
	Type         string `json:"type" form:"-"`
	Host         string `json:"host" form:"-"`
	Port         int    `json:"port" form:"-"`
	DatabaseName string `json:"database_name" form:"-"`
	Username     string `json:"username" form:"-"`
	Password     string `json:"password" form:"-"`
}

// TestConnection handles POST /api/datasources/test
func (h *DatasourceHandler) TestConnection(req router.Request[datasourceTestConnectionIn], res *router.Response[datasourceStatusOut]) error {
	in := req.In

	if in.Type == "" {
		in.Type = "postgresql"
	}

	config := entity.DatasourceConnectionConfig{
		Host:         in.Host,
		Port:         in.Port,
		DatabaseName: in.DatabaseName,
		Username:     in.Username,
		Password:     in.Password,
	}

	// Connection failures keep their pre-migration 20100 mapping, they are
	// not internal errors.
	if err := h.svc.TestConnection(req.Ctx.Request.Context(), config, in.Type); err != nil {
		return router.NewBusinessError(response.CodeBadRequest, err.Error())
	}
	res.Out = datasourceStatusOut{Status: "ok"}
	return nil
}

// GetTables handles GET /api/datasources/:id/tables
func (h *DatasourceHandler) GetTables(req router.Request[datasourcePathIn], res *router.Response[[]entity.TableInfo]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	tables, err := h.svc.GetTables(req.Ctx.Request.Context(), id)
	if err != nil {
		return err
	}
	res.Out = tables
	return nil
}

// GetColumns handles GET /api/datasources/:id/tables/:table/columns
func (h *DatasourceHandler) GetColumns(req router.Request[datasourcePathIn], res *router.Response[[]map[string]any]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	tableName := req.Ctx.Param("table")
	if tableName == "" {
		return router.NewBusinessError(response.CodeBadRequest, "table name is required")
	}

	columns, err := h.svc.GetColumns(req.Ctx.Request.Context(), id, tableName)
	if err != nil {
		return err
	}

	// The map projection is deliberate: it keeps the JSON key order
	// (comment, data_type, name) byte-identical to the pre-migration
	// response; a struct Out would emit name, data_type, comment instead.
	result := make([]map[string]any, len(columns))
	for i, col := range columns {
		result[i] = map[string]any{
			"name":      col.Name,
			"data_type": col.DataType,
			"comment":   col.Comment,
		}
	}
	res.Out = result
	return nil
}

// datasourcePreviewIn is the JSON body of POST /api/datasources/:id/preview.
type datasourcePreviewIn struct {
	TableName string `json:"table_name" form:"-"`
	QuerySQL  string `json:"query_sql" form:"-"`
	QueryType string `json:"query_type" form:"-"`
}

// Preview handles POST /api/datasources/:id/preview
func (h *DatasourceHandler) Preview(req router.Request[datasourcePreviewIn], res *router.Response[*entity.PreviewResult]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	in := req.In

	result, err := h.svc.Preview(req.Ctx.Request.Context(), id, in.TableName, in.QuerySQL, in.QueryType)
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// datasourceFieldDistributionIn is the JSON body of
// POST /api/datasources/:id/field-distribution.
type datasourceFieldDistributionIn struct {
	TableName string `json:"table_name" form:"-"`
	QuerySQL  string `json:"query_sql" form:"-"`
	QueryType string `json:"query_type" form:"-"`
	FieldName string `json:"field_name" form:"-"`
	Limit     int    `json:"limit" form:"-"`
}

// GetFieldDistribution handles POST /api/datasources/:id/field-distribution
func (h *DatasourceHandler) GetFieldDistribution(req router.Request[datasourceFieldDistributionIn], res *router.Response[*entity.FieldDistribution]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	in := req.In

	if in.FieldName == "" {
		return router.NewBusinessError(response.CodeBadRequest, "field_name is required")
	}

	result, err := h.svc.GetFieldDistribution(req.Ctx.Request.Context(), id, in.TableName, in.QuerySQL, in.QueryType, in.FieldName, in.Limit)
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// datasourceTableDataIn carries the query params of
// GET /api/datasources/:id/tables/:table/data. Like datasourceListIn the
// values bind as strings and are parsed post-bind so malformed input keeps
// the old fall-through-to-default behaviour.
type datasourceTableDataIn struct {
	Page      string `form:"page"`
	PageSize  string `form:"page_size"`
	SortField string `form:"sort_field"`
	SortOrder string `form:"sort_order"`
}

// query mirrors the old DefaultQuery reads: page defaults to 1, page_size
// to 20, sort_order to ASC; unparseable numbers fall back to 0 exactly like
// the old strconv-error-ignoring handler (the service clamps them).
func (in datasourceTableDataIn) query() (page, pageSize int, sortField, sortOrder string) {
	page, _ = strconv.Atoi(orDefault(in.Page, "1"))
	pageSize, _ = strconv.Atoi(orDefault(in.PageSize, "20"))
	sortField = in.SortField
	sortOrder = orDefault(in.SortOrder, "ASC")
	return
}

// GetTableData handles GET /api/datasources/:id/tables/:table/data
func (h *DatasourceHandler) GetTableData(req router.Request[datasourceTableDataIn], res *router.Response[*entity.TableDataResult]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}

	tableName := req.Ctx.Param("table")
	if tableName == "" {
		return router.NewBusinessError(response.CodeBadRequest, "table name is required")
	}

	page, pageSize, sortField, sortOrder := req.In.query()

	result, err := h.svc.GetTableData(req.Ctx.Request.Context(), id, tableName, page, pageSize, sortField, sortOrder)
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}
