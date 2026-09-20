package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"data-insights/internal/domain/entity"
	"data-insights/internal/router"
	"data-insights/internal/service/chart"

	"github.com/gin-gonic/gin"
)

// mockChartService implements chart.Service for handler tests. The embedded
// interface is nil on purpose: calling a method that is not overridden fails
// loudly, which keeps each test's stub surface explicit (same style as
// mockDatasourceService in datasource_test.go).
type mockChartService struct {
	chart.Service

	listFunc    func(ctx context.Context, limit, offset int) ([]entity.Chart, error)
	getByIDFunc func(ctx context.Context, id int) (*entity.Chart, error)
	createFunc  func(ctx context.Context, c *entity.Chart) (*entity.Chart, error)
	updateFunc  func(ctx context.Context, c *entity.Chart) (*entity.Chart, error)
	deleteFunc  func(ctx context.Context, id int) error
	getDataFunc func(ctx context.Context, id int) (entity.ChartDataResult, error)
	queryFunc   func(ctx context.Context, req *entity.ChartQueryRequest) (entity.ChartDataResult, error)
}

func (m *mockChartService) List(ctx context.Context, limit, offset int) ([]entity.Chart, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, limit, offset)
	}
	return nil, nil
}

func (m *mockChartService) GetByID(ctx context.Context, id int) (*entity.Chart, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockChartService) Create(ctx context.Context, c *entity.Chart) (*entity.Chart, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, c)
	}
	return nil, nil
}

func (m *mockChartService) Update(ctx context.Context, c *entity.Chart) (*entity.Chart, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, c)
	}
	return nil, nil
}

func (m *mockChartService) Delete(ctx context.Context, id int) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func (m *mockChartService) GetData(ctx context.Context, id int) (entity.ChartDataResult, error) {
	if m.getDataFunc != nil {
		return m.getDataFunc(ctx, id)
	}
	return entity.ChartDataResult{}, nil
}

func (m *mockChartService) Query(ctx context.Context, req *entity.ChartQueryRequest) (entity.ChartDataResult, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, req)
	}
	return entity.ChartDataResult{}, nil
}

// newChartTestRouter mirrors the chart section of cmd/routes.go. During the
// Batch 2 migration only this wiring helper changes; every response-body
// assertion below must keep passing byte-for-byte before and after migration
// — that is the zero-behavior-change guard (same contract as
// newDatasourceTestRouter in datasource_test.go). serve/assertBody and the
// shared envelope constants come from that file.
func newChartTestRouter(h *ChartHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	charts := r.Group("/api/charts")
	router.RegisterGetRoute(charts, "", h.List)
	router.RegisterPostRoute(charts, "", h.Create)
	router.RegisterGetRoute(charts, "/:id", h.Get)
	router.RegisterPutRoute(charts, "/:id", h.Update)
	router.RegisterDeleteRoute(charts, "/:id", h.Delete)
	router.RegisterGetRoute(charts, "/:id/data", h.GetData)
	router.RegisterPostRoute(charts, "/query", h.Query)
	return r
}

const (
	notFoundChart  = `{"code":20300,"msg":"chart not found: sql: no rows in result set","trace":"","data":{}}`
	chartFoundJSON = `{"id":7,"name":"c","dataset_id":3,"chart_type":"line","config":"{}","created_at":"","updated_at":""}`
)

// ---------------------------------------------------------------- List

func TestChartList_Defaults(t *testing.T) {
	var gotLimit, gotOffset int
	h := NewChartHandler(&mockChartService{
		listFunc: func(_ context.Context, limit, offset int) ([]entity.Chart, error) {
			gotLimit, gotOffset = limit, offset
			return nil, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodGet, "/api/charts", "")
	assertBody(t, w, okEnvelopeEmptyArray)
	if gotLimit != 100 || gotOffset != 0 {
		t.Errorf("expected defaults limit=100 offset=0, got limit=%d offset=%d", gotLimit, gotOffset)
	}
}

func TestChartList_ExplicitPaging(t *testing.T) {
	var gotLimit, gotOffset int
	h := NewChartHandler(&mockChartService{
		listFunc: func(_ context.Context, limit, offset int) ([]entity.Chart, error) {
			gotLimit, gotOffset = limit, offset
			return []entity.Chart{{ID: 1, Name: "c1", DatasetID: 2, ChartType: "bar", Config: "{}"}}, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodGet, "/api/charts?limit=2&offset=3", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":[{"id":1,"name":"c1","dataset_id":2,"chart_type":"bar","config":"{}","created_at":"","updated_at":""}]}`)
	if gotLimit != 2 || gotOffset != 3 {
		t.Errorf("expected limit=2 offset=3, got limit=%d offset=%d", gotLimit, gotOffset)
	}
}

func TestChartList_InvalidAndClampedPaging(t *testing.T) {
	cases := []struct {
		query     string
		wantLimit int
		wantOff   int
	}{
		// Garbage values must NOT become a 400: the current handler ignores
		// parse errors and falls back to the clamped default.
		{"?limit=abc&offset=xyz", 100, 0},
		{"?limit=5000", 100, 0},
		{"?limit=0", 100, 0},
		{"?limit=-5", 100, 0},
		{"?offset=-1", 100, 0},
		{"?limit=1000", 1000, 0},
	}
	for _, tc := range cases {
		var gotLimit, gotOffset int
		h := NewChartHandler(&mockChartService{
			listFunc: func(_ context.Context, limit, offset int) ([]entity.Chart, error) {
				gotLimit, gotOffset = limit, offset
				return nil, nil
			},
		})
		w := serve(newChartTestRouter(h), http.MethodGet, "/api/charts"+tc.query, "")
		assertBody(t, w, okEnvelopeEmptyArray)
		if gotLimit != tc.wantLimit || gotOffset != tc.wantOff {
			t.Errorf("query %q: expected limit=%d offset=%d, got limit=%d offset=%d",
				tc.query, tc.wantLimit, tc.wantOff, gotLimit, gotOffset)
		}
	}
}

func TestChartList_ServiceError(t *testing.T) {
	h := NewChartHandler(&mockChartService{
		listFunc: func(_ context.Context, _, _ int) ([]entity.Chart, error) {
			return nil, errBoom()
		},
	})
	w := serve(newChartTestRouter(h), http.MethodGet, "/api/charts", "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Get

func TestChartGet_Found(t *testing.T) {
	var gotID int
	h := NewChartHandler(&mockChartService{
		getByIDFunc: func(_ context.Context, id int) (*entity.Chart, error) {
			gotID = id
			return &entity.Chart{ID: 7, Name: "c", DatasetID: 3, ChartType: "line", Config: "{}"}, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodGet, "/api/charts/7", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":`+chartFoundJSON+`}`)
	if gotID != 7 {
		t.Errorf("expected GetByID called with id=7, got %d", gotID)
	}
}

func TestChartGet_NotFound(t *testing.T) {
	h := NewChartHandler(&mockChartService{
		getByIDFunc: func(_ context.Context, _ int) (*entity.Chart, error) {
			return nil, errors.New("chart not found: sql: no rows in result set")
		},
	})
	w := serve(newChartTestRouter(h), http.MethodGet, "/api/charts/999", "")
	assertBody(t, w, notFoundChart)
}

func TestChartGet_InvalidID(t *testing.T) {
	h := NewChartHandler(&mockChartService{})
	w := serve(newChartTestRouter(h), http.MethodGet, "/api/charts/abc", "")
	assertBody(t, w, badRequestInvalidID)
}

// ---------------------------------------------------------------- Create

func TestChartCreate_Success(t *testing.T) {
	var got *entity.Chart
	h := NewChartHandler(&mockChartService{
		createFunc: func(_ context.Context, c *entity.Chart) (*entity.Chart, error) {
			got = c
			c.ID = 8
			return c, nil
		},
	})
	body := `{"name":"new","dataset_id":4,"chart_type":"line","config":"{\"x\":1}"}`
	w := serve(newChartTestRouter(h), http.MethodPost, "/api/charts", body)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"id":8,"name":"new","dataset_id":4,"chart_type":"line","config":"{\"x\":1}","created_at":"","updated_at":""}}`)
	if got == nil || got.Name != "new" || got.DatasetID != 4 || got.ChartType != "line" || got.Config != `{"x":1}` {
		t.Fatalf("service received unexpected entity: %+v", got)
	}
}

func TestChartCreate_ConfigDefault(t *testing.T) {
	// The handler keeps the empty-config fallback: a body without "config"
	// must reach the service as "{}".
	var got *entity.Chart
	h := NewChartHandler(&mockChartService{
		createFunc: func(_ context.Context, c *entity.Chart) (*entity.Chart, error) {
			got = c
			return c, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPost, "/api/charts", `{"name":"n"}`)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"id":0,"name":"n","dataset_id":0,"chart_type":"","config":"{}","created_at":"","updated_at":""}}`)
	if got == nil || got.Config != "{}" {
		t.Fatalf("expected default config \"{}\", got %+v", got)
	}
}

func TestChartCreate_EmptyBody(t *testing.T) {
	// The current handler calls ShouldBindJSON unconditionally: an empty body
	// must answer 20100 with the raw "EOF" binding error, never create.
	h := NewChartHandler(&mockChartService{
		createFunc: func(_ context.Context, _ *entity.Chart) (*entity.Chart, error) {
			t.Fatal("Create must not be called for an empty body")
			return nil, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPost, "/api/charts", "")
	assertBody(t, w, badRequestEOF)
}

func TestChartCreate_TypeMismatchBody(t *testing.T) {
	// Accepted unavoidable diff #1 (datasource package doc): the named In
	// type leaks into the json bind-error text. Pre-migration the handler
	// bound entity.Chart directly, so the old message was
	// "…Go struct field Chart.dataset_id…"; the datasource baseline
	// (datasourceCreateIn.port) pins the same class of change.
	h := NewChartHandler(&mockChartService{
		createFunc: func(_ context.Context, _ *entity.Chart) (*entity.Chart, error) {
			t.Fatal("Create must not be called when the body fails to bind")
			return nil, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPost, "/api/charts", `{"dataset_id":"x"}`)
	assertBody(t, w, `{"code":20100,"msg":"json: cannot unmarshal string into Go struct field chartCreateIn.dataset_id of type int","trace":"","data":{}}`)
}

func TestChartCreate_QueryMustNotPolluteBody(t *testing.T) {
	// Body structs are bound from JSON only; a stray query param must not
	// leak into any In field.
	var got *entity.Chart
	h := NewChartHandler(&mockChartService{
		createFunc: func(_ context.Context, c *entity.Chart) (*entity.Chart, error) {
			got = c
			return c, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPost, "/api/charts?Name=evil&DatasetID=99", `{"chart_type":"line"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", w.Code)
	}
	if got == nil || got.Name != "" || got.DatasetID != 0 {
		t.Fatalf("query param leaked into body struct: %+v", got)
	}
}

func TestChartCreate_ServiceError(t *testing.T) {
	h := NewChartHandler(&mockChartService{
		createFunc: func(_ context.Context, _ *entity.Chart) (*entity.Chart, error) {
			return nil, errBoom()
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPost, "/api/charts", `{"name":"n"}`)
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Update

func TestChartUpdate_BindsDatasetID(t *testing.T) {
	// Successor of the original TestUpdate_BindsDatasetID: same assertions,
	// now expressed through the shared envelope + capture style.
	var got *entity.Chart
	h := NewChartHandler(&mockChartService{
		updateFunc: func(_ context.Context, c *entity.Chart) (*entity.Chart, error) {
			got = c
			return c, nil
		},
	})
	body := `{
		"name": "test chart",
		"dataset_id": 5,
		"chart_type": "pie",
		"config": "{\"chartType\":\"pie\"}"
	}`
	w := serve(newChartTestRouter(h), http.MethodPut, "/api/charts/3", body)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"id":3,"name":"test chart","dataset_id":5,"chart_type":"pie","config":"{\"chartType\":\"pie\"}","created_at":"","updated_at":""}}`)
	if got == nil || got.ID != 3 || got.DatasetID != 5 || got.Name != "test chart" || got.ChartType != "pie" {
		t.Fatalf("expected ID=3 DatasetID=5 Name=\"test chart\" ChartType=\"pie\", got %+v", got)
	}
}

func TestChartUpdate_InvalidIDValidBody(t *testing.T) {
	// Successor of the original TestUpdate_InvalidID: a valid body keeps the
	// pre-migration precedence — the id is checked first, so "invalid id"
	// wins. Unaffected by migration.
	h := NewChartHandler(&mockChartService{})
	w := serve(newChartTestRouter(h), http.MethodPut, "/api/charts/abc", `{}`)
	assertBody(t, w, badRequestInvalidID)
}

func TestChartUpdate_EmptyBody(t *testing.T) {
	// Valid id + empty body: the old unconditional ShouldBindJSON "EOF" bind
	// error; the router's method-gated body bind emits the same bytes.
	h := NewChartHandler(&mockChartService{
		updateFunc: func(_ context.Context, _ *entity.Chart) (*entity.Chart, error) {
			t.Fatal("Update must not be called for an empty body")
			return nil, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPut, "/api/charts/3", "")
	assertBody(t, w, badRequestEOF)
}

func TestChartUpdate_InvalidIDPrefersBodyBindError_EmptyBody(t *testing.T) {
	// Pinned accepted unavoidable diff #2 (datasource package doc): the
	// generic router binds the PUT body before the handler parses the path
	// id, so a request carrying BOTH an unparseable :id AND an empty body
	// answers with the body-bind error ("EOF") where the pre-migration
	// handler answered "invalid id". Same shape as the datasource
	// Test*_InvalidIDPrefersBodyBindError baselines. Requests valid on
	// either input are unaffected (see InvalidIDValidBody / EmptyBody above).
	h := NewChartHandler(&mockChartService{
		updateFunc: func(_ context.Context, _ *entity.Chart) (*entity.Chart, error) {
			t.Fatal("Update must not run when the body fails to bind")
			return nil, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPut, "/api/charts/abc", "")
	assertBody(t, w, badRequestEOF)
}

func TestChartUpdate_InvalidIDPrefersBodyBindError_MalformedBody(t *testing.T) {
	// Same diff #2 flip with a type-mismatch body: the json bind error
	// (carrying the named In type, diff #1) wins over "invalid id".
	h := NewChartHandler(&mockChartService{
		updateFunc: func(_ context.Context, _ *entity.Chart) (*entity.Chart, error) {
			t.Fatal("Update must not run when the body fails to bind")
			return nil, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPut, "/api/charts/abc", `{"dataset_id":"x"}`)
	assertBody(t, w, `{"code":20100,"msg":"json: cannot unmarshal string into Go struct field chartUpdateIn.dataset_id of type int","trace":"","data":{}}`)
}

func TestChartUpdate_TypeMismatchBody(t *testing.T) {
	// Accepted unavoidable diff #1 (datasource package doc): the named In
	// type leaks into the json bind-error text. Pre-migration the handler
	// bound an anonymous struct, so the old message was
	// "…Go struct field .dataset_id…".
	h := NewChartHandler(&mockChartService{
		updateFunc: func(_ context.Context, _ *entity.Chart) (*entity.Chart, error) {
			t.Fatal("Update must not be called when the body fails to bind")
			return nil, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPut, "/api/charts/3", `{"dataset_id":"x"}`)
	assertBody(t, w, `{"code":20100,"msg":"json: cannot unmarshal string into Go struct field chartUpdateIn.dataset_id of type int","trace":"","data":{}}`)
}

func TestChartUpdate_ServiceError(t *testing.T) {
	h := NewChartHandler(&mockChartService{
		updateFunc: func(_ context.Context, _ *entity.Chart) (*entity.Chart, error) {
			return nil, errBoom()
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPut, "/api/charts/3", `{"name":"n"}`)
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Delete

func TestChartDelete_Success(t *testing.T) {
	var gotID int
	h := NewChartHandler(&mockChartService{
		deleteFunc: func(_ context.Context, id int) error {
			gotID = id
			return nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodDelete, "/api/charts/5", "")
	assertBody(t, w, okEnvelopeStatus)
	if gotID != 5 {
		t.Errorf("expected Delete called with id=5, got %d", gotID)
	}
}

func TestChartDelete_InvalidID(t *testing.T) {
	h := NewChartHandler(&mockChartService{})
	w := serve(newChartTestRouter(h), http.MethodDelete, "/api/charts/abc", "")
	assertBody(t, w, badRequestInvalidID)
}

func TestChartDelete_ServiceError(t *testing.T) {
	h := NewChartHandler(&mockChartService{
		deleteFunc: func(_ context.Context, _ int) error { return errBoom() },
	})
	w := serve(newChartTestRouter(h), http.MethodDelete, "/api/charts/5", "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- GetData

func TestChartGetData_Success(t *testing.T) {
	// GetData answers result.Data verbatim (not the whole ChartDataResult).
	var gotID int
	h := NewChartHandler(&mockChartService{
		getDataFunc: func(_ context.Context, id int) (entity.ChartDataResult, error) {
			gotID = id
			return entity.ChartDataResult{Data: map[string]any{"total": 1}, SelectSQL: "SELECT 1"}, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodGet, "/api/charts/7/data", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"total":1}}`)
	if gotID != 7 {
		t.Errorf("expected GetData called with id=7, got %d", gotID)
	}
}

func TestChartGetData_NilData(t *testing.T) {
	h := NewChartHandler(&mockChartService{
		getDataFunc: func(_ context.Context, _ int) (entity.ChartDataResult, error) {
			return entity.ChartDataResult{}, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodGet, "/api/charts/7/data", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{}}`)
}

func TestChartGetData_InvalidID(t *testing.T) {
	h := NewChartHandler(&mockChartService{})
	w := serve(newChartTestRouter(h), http.MethodGet, "/api/charts/abc/data", "")
	assertBody(t, w, badRequestInvalidID)
}

func TestChartGetData_ServiceError(t *testing.T) {
	h := NewChartHandler(&mockChartService{
		getDataFunc: func(_ context.Context, _ int) (entity.ChartDataResult, error) {
			return entity.ChartDataResult{}, errBoom()
		},
	})
	w := serve(newChartTestRouter(h), http.MethodGet, "/api/charts/7/data", "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Query

func TestChartQuery_Success(t *testing.T) {
	// Query answers the whole ChartDataResult (data + select_sql, with
	// count_sql omitted when empty).
	var got *entity.ChartQueryRequest
	h := NewChartHandler(&mockChartService{
		queryFunc: func(_ context.Context, req *entity.ChartQueryRequest) (entity.ChartDataResult, error) {
			got = req
			return entity.ChartDataResult{Data: map[string]any{"k": "v"}, SelectSQL: "SELECT 1"}, nil
		},
	})
	body := `{"dataset_id":4,"chart_type":"bar","dims":["region"],"metrics":[{"field":"amount","agg":"sum","alias":"total"}],"filters":[{"id":"f1","field":"region","operator":"eq","value":"east","logic":"and"}],"pagination":{"page":1,"page_size":10},"sort":{"field":"amount","order":"desc"}}`
	w := serve(newChartTestRouter(h), http.MethodPost, "/api/charts/query", body)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"data":{"k":"v"},"select_sql":"SELECT 1"}}`)
	if got == nil || got.DatasetID != 4 || got.ChartType != "bar" || len(got.Dims) != 1 {
		t.Fatalf("unexpected request: %+v", got)
	}
	if len(got.Metrics) != 1 || got.Metrics[0].Alias != "total" || got.Metrics[0].Agg != "sum" {
		t.Fatalf("metrics not passed through: %+v", got.Metrics)
	}
	if len(got.Filters) != 1 || got.Filters[0].Operator != "eq" {
		t.Fatalf("filters not passed through: %+v", got.Filters)
	}
	if got.Pagination == nil || got.Pagination.Page != 1 || got.Pagination.PageSize != 10 {
		t.Fatalf("pagination not passed through: %+v", got.Pagination)
	}
	if got.Sort == nil || got.Sort.Order != "desc" {
		t.Fatalf("sort not passed through: %+v", got.Sort)
	}
}

func TestChartQuery_EmptyBody(t *testing.T) {
	h := NewChartHandler(&mockChartService{
		queryFunc: func(_ context.Context, _ *entity.ChartQueryRequest) (entity.ChartDataResult, error) {
			t.Fatal("Query must not be called for an empty body")
			return entity.ChartDataResult{}, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPost, "/api/charts/query", "")
	assertBody(t, w, badRequestEOF)
}

func TestChartQuery_TypeMismatchBody(t *testing.T) {
	// Accepted unavoidable diff #1 (datasource package doc): the named In
	// type leaks into the json bind-error text. Pre-migration the handler
	// bound entity.ChartQueryRequest directly, so the old message was
	// "…Go struct field ChartQueryRequest.dataset_id…".
	h := NewChartHandler(&mockChartService{
		queryFunc: func(_ context.Context, _ *entity.ChartQueryRequest) (entity.ChartDataResult, error) {
			t.Fatal("Query must not be called when the body fails to bind")
			return entity.ChartDataResult{}, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPost, "/api/charts/query", `{"dataset_id":"x"}`)
	assertBody(t, w, `{"code":20100,"msg":"json: cannot unmarshal string into Go struct field chartQueryIn.dataset_id of type int","trace":"","data":{}}`)
}

func TestChartQuery_NestedTypeMismatchBody(t *testing.T) {
	// Same diff #1 on the nested-field error text.
	h := NewChartHandler(&mockChartService{
		queryFunc: func(_ context.Context, _ *entity.ChartQueryRequest) (entity.ChartDataResult, error) {
			t.Fatal("Query must not be called when the body fails to bind")
			return entity.ChartDataResult{}, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPost, "/api/charts/query", `{"metrics":[{"agg":[1]}]}`)
	assertBody(t, w, `{"code":20100,"msg":"json: cannot unmarshal array into Go struct field chartQueryIn.metrics.0.agg of type string","trace":"","data":{}}`)
}

func TestChartQuery_QueryMustNotPolluteBody(t *testing.T) {
	var got *entity.ChartQueryRequest
	h := NewChartHandler(&mockChartService{
		queryFunc: func(_ context.Context, req *entity.ChartQueryRequest) (entity.ChartDataResult, error) {
			got = req
			return entity.ChartDataResult{}, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPost, "/api/charts/query?DatasetID=99", `{"chart_type":"line"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", w.Code)
	}
	if got == nil || got.DatasetID != 0 {
		t.Fatalf("query param leaked into body struct: %+v", got)
	}
}

func TestChartQuery_QueryOptionsBindsFromBodyOnly(t *testing.T) {
	// query_options（histogram bin_count/bin_width，R-57）只从 JSON body 绑定；
	// chartQueryIn 的 form:"-" 必须挡住同名 query 参数污染。
	var got *entity.ChartQueryRequest
	h := NewChartHandler(&mockChartService{
		queryFunc: func(_ context.Context, req *entity.ChartQueryRequest) (entity.ChartDataResult, error) {
			got = req
			return entity.ChartDataResult{}, nil
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPost,
		"/api/charts/query?query_options=%7B%22bin_count%22%3A99%7D&bin_count=99",
		`{"chart_type":"histogram","query_options":{"bin_count":10}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	if got == nil || got.QueryOptions == nil {
		t.Fatalf("query_options not bound from body: %+v", got)
	}
	if got.QueryOptions["bin_count"] != float64(10) {
		t.Fatalf("expected body bin_count=10, got %v (query param pollution?)", got.QueryOptions["bin_count"])
	}
	if len(got.QueryOptions) != 1 {
		t.Fatalf("expected exactly the body options, got %+v", got.QueryOptions)
	}
}

func TestChartQuery_ServiceError(t *testing.T) {
	h := NewChartHandler(&mockChartService{
		queryFunc: func(_ context.Context, _ *entity.ChartQueryRequest) (entity.ChartDataResult, error) {
			return entity.ChartDataResult{}, errBoom()
		},
	})
	w := serve(newChartTestRouter(h), http.MethodPost, "/api/charts/query", `{"dataset_id":1}`)
	assertBody(t, w, internalErrorBoom)
}
