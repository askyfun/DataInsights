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
// res.Out carries the payload only; response.Success wraps it in the
// envelope. response's normalizer turns nil slices/maps into [] / {} and
// nil root data into {}, so a typed nil slice Out already serializes as [] —
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

	"github.com/gin-gonic/gin"
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

// List handles GET /api/datasources
func (h *DatasourceHandler) List(c *gin.Context) {
	limit, offset := getPaginationParams(c)

	datasources, err := h.svc.List(c.Request.Context(), limit, offset)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if datasources == nil {
		datasources = []entity.Datasource{}
	}
	response.Success(c, datasources)
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

// Create handles POST /api/datasources
func (h *DatasourceHandler) Create(c *gin.Context) {
	var req struct {
		Name         string `json:"name"`
		Type         string `json:"type"`
		Host         string `json:"host"`
		Port         int    `json:"port"`
		DatabaseName string `json:"database_name"`
		Username     string `json:"username"`
		Password     string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.Type == "" {
		req.Type = "postgresql"
	}

	ds := &entity.Datasource{
		Name:         req.Name,
		Type:         req.Type,
		Host:         req.Host,
		Port:         req.Port,
		DatabaseName: req.DatabaseName,
		Username:     req.Username,
		Password:     req.Password,
	}

	result, err := h.svc.Create(c.Request.Context(), ds)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

// Update handles PUT /api/datasources/:id
func (h *DatasourceHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	var req struct {
		Name         string `json:"name"`
		Type         string `json:"type"`
		Host         string `json:"host"`
		Port         int    `json:"port"`
		DatabaseName string `json:"database_name"`
		Username     string `json:"username"`
		Password     string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	ds := &entity.Datasource{
		ID:           id,
		Name:         req.Name,
		Type:         req.Type,
		Host:         req.Host,
		Port:         req.Port,
		DatabaseName: req.DatabaseName,
		Username:     req.Username,
		Password:     req.Password,
	}

	result, err := h.svc.Update(c.Request.Context(), ds)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

// Delete handles DELETE /api/datasources/:id
func (h *DatasourceHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"status": "ok"})
}

// TestConnection handles POST /api/datasources/test
func (h *DatasourceHandler) TestConnection(c *gin.Context) {
	var req struct {
		Type         string `json:"type"`
		Host         string `json:"host"`
		Port         int    `json:"port"`
		DatabaseName string `json:"database_name"`
		Username     string `json:"username"`
		Password     string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.Type == "" {
		req.Type = "postgresql"
	}

	config := entity.DatasourceConnectionConfig{
		Host:         req.Host,
		Port:         req.Port,
		DatabaseName: req.DatabaseName,
		Username:     req.Username,
		Password:     req.Password,
	}

	if err := h.svc.TestConnection(c.Request.Context(), config, req.Type); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"status": "ok"})
}

// GetTables handles GET /api/datasources/:id/tables
func (h *DatasourceHandler) GetTables(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	tables, err := h.svc.GetTables(c.Request.Context(), id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, tables)
}

// GetColumns handles GET /api/datasources/:id/tables/:table/columns
func (h *DatasourceHandler) GetColumns(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	tableName := c.Param("table")
	if tableName == "" {
		response.BadRequest(c, "table name is required")
		return
	}

	columns, err := h.svc.GetColumns(c.Request.Context(), id, tableName)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	result := make([]map[string]interface{}, len(columns))
	for i, col := range columns {
		result[i] = map[string]interface{}{
			"name":      col.Name,
			"data_type": col.DataType,
			"comment":   col.Comment,
		}
	}
	response.Success(c, result)
}

// Preview handles POST /api/datasources/:id/preview
func (h *DatasourceHandler) Preview(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	var req struct {
		TableName string `json:"table_name"`
		QuerySQL  string `json:"query_sql"`
		QueryType string `json:"query_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.Preview(c.Request.Context(), id, req.TableName, req.QuerySQL, req.QueryType)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

// GetFieldDistribution handles POST /api/datasources/:id/field-distribution
func (h *DatasourceHandler) GetFieldDistribution(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	var req struct {
		TableName string `json:"table_name"`
		QuerySQL  string `json:"query_sql"`
		QueryType string `json:"query_type"`
		FieldName string `json:"field_name"`
		Limit     int    `json:"limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.FieldName == "" {
		response.BadRequest(c, "field_name is required")
		return
	}

	result, err := h.svc.GetFieldDistribution(c.Request.Context(), id, req.TableName, req.QuerySQL, req.QueryType, req.FieldName, req.Limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

// GetTableData handles GET /api/datasources/:id/tables/:table/data
func (h *DatasourceHandler) GetTableData(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	tableName := c.Param("table")
	if tableName == "" {
		response.BadRequest(c, "table name is required")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	sortField := c.Query("sort_field")
	sortOrder := c.DefaultQuery("sort_order", "ASC")

	result, err := h.svc.GetTableData(c.Request.Context(), id, tableName, page, pageSize, sortField, sortOrder)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}
