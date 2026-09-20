package handler

import (
	"encoding/json"

	"data-insights/internal/domain/entity"
	"data-insights/internal/router"
	"data-insights/internal/service/queryrecord"
)

// QueryHandler handles query-record HTTP requests (R-08' 落库 / R-30' 寻址).
type QueryHandler struct {
	svc queryrecord.Service
}

// NewQueryHandler creates a new QueryHandler
func NewQueryHandler(svc queryrecord.Service) *QueryHandler {
	return &QueryHandler{svc: svc}
}

// querySaveIn is the JSON body of POST /api/queries. It mirrors
// entity.QueryRecordSaveRequest one-to-one (guarded by
// contract_parity_test.go) with form:"-" on every field so the router's
// ShouldBindQuery pass cannot read query params into the body struct (see the
// datasource package doc).
type querySaveIn struct {
	DatasetID  int             `json:"dataset_id" form:"-"`
	ChartID    int             `json:"chart_id" form:"-"`
	Spec       json.RawMessage `json:"spec" form:"-"`
	SourceType string          `json:"source_type" form:"-"`
	RowCount   *int            `json:"row_count" form:"-"`
	DurationMs *int            `json:"duration_ms" form:"-"`
}

// Save handles POST /api/queries: persist one query configuration and return
// the address-bar short id. Deduplication lives in the service (spec_hash
// upsert); the client IP is captured here because only the HTTP layer knows it.
func (h *QueryHandler) Save(
	req router.Request[querySaveIn], res *router.Response[*entity.QueryRecordSaved],
) error {
	in := req.In
	saved, err := h.svc.Save(req.Ctx.Request.Context(), entity.QueryRecordSaveRequest{
		DatasetID:  in.DatasetID,
		ChartID:    in.ChartID,
		Spec:       in.Spec,
		SourceType: in.SourceType,
		RowCount:   in.RowCount,
		DurationMs: in.DurationMs,
	}, req.Ctx.ClientIP())
	if err != nil {
		return err
	}
	res.Out = saved
	return nil
}

// queryPathIn is the In shape for GET /api/queries/:q: the short id arrives via
// the path (req.Ctx.Param("q")), nothing to bind.
type queryPathIn struct{}

// Get handles GET /api/queries/:q: resolve an address-bar short id back to its
// full spec. Expired records are returned like any other (直链保活); the
// expiry verdict travels in the payload instead of turning into an error.
func (h *QueryHandler) Get(
	req router.Request[queryPathIn], res *router.Response[*entity.QueryRecord],
) error {
	record, err := h.svc.GetByShortID(req.Ctx.Request.Context(), req.Ctx.Param("q"))
	if err != nil {
		return err
	}
	res.Out = record
	return nil
}
