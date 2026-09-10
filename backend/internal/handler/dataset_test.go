package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"dataray/internal/domain/entity"
	"dataray/internal/router"
	"dataray/internal/service/dataset"

	"github.com/gin-gonic/gin"
)

// mockDatasetService implements dataset.Service for handler tests. The
// embedded interface is nil on purpose: calling a method that is not
// overridden fails loudly, which keeps each test's stub surface explicit
// (same pattern as mockDatasourceService in datasource_test.go).
type mockDatasetService struct {
	dataset.Service

	listFunc          func(ctx context.Context, limit, offset int) ([]entity.Dataset, error)
	getByIDFunc       func(ctx context.Context, id int) (*entity.Dataset, error)
	createFunc        func(ctx context.Context, ds *entity.Dataset) (*entity.Dataset, error)
	deleteFunc        func(ctx context.Context, id int) error
	getColumnsFunc    func(ctx context.Context, id int) ([]entity.DatasetColumn, error)
	updateColumnsFunc func(ctx context.Context, id int, columns []entity.DatasetColumn) (*entity.Dataset, error)
	previewFunc       func(ctx context.Context, id int) (*entity.PreviewResult, error)
	queryFunc         func(ctx context.Context, id int, config entity.QueryConfig) ([]map[string]any, error)
}

func (m *mockDatasetService) List(ctx context.Context, limit, offset int) ([]entity.Dataset, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, limit, offset)
	}
	return nil, nil
}

func (m *mockDatasetService) GetByID(ctx context.Context, id int) (*entity.Dataset, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockDatasetService) Create(ctx context.Context, ds *entity.Dataset) (*entity.Dataset, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, ds)
	}
	return nil, nil
}

func (m *mockDatasetService) Delete(ctx context.Context, id int) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func (m *mockDatasetService) GetColumns(ctx context.Context, id int) ([]entity.DatasetColumn, error) {
	if m.getColumnsFunc != nil {
		return m.getColumnsFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockDatasetService) UpdateColumns(ctx context.Context, id int, columns []entity.DatasetColumn) (*entity.Dataset, error) {
	if m.updateColumnsFunc != nil {
		return m.updateColumnsFunc(ctx, id, columns)
	}
	return nil, nil
}

func (m *mockDatasetService) Preview(ctx context.Context, id int) (*entity.PreviewResult, error) {
	if m.previewFunc != nil {
		return m.previewFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockDatasetService) Query(ctx context.Context, id int, config entity.QueryConfig) ([]map[string]any, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, id, config)
	}
	return nil, nil
}

// newDatasetTestRouter mirrors the dataset section of cmd/routes.go.
// During the Batch 2 migration only this wiring helper changes; every
// response-body assertion below must keep passing byte-for-byte before and
// after migration — that is the zero-behavior-change guard (same contract as
// newDatasourceTestRouter in datasource_test.go).
func newDatasetTestRouter(h *DatasetHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	datasets := r.Group("/api/datasets")
	router.RegisterGetRoute(datasets, "", h.List)
	router.RegisterPostRoute(datasets, "", h.Create)
	router.RegisterGetRoute(datasets, "/:id", h.Get)
	router.RegisterDeleteRoute(datasets, "/:id", h.Delete)
	router.RegisterGetRoute(datasets, "/:id/columns", h.GetColumns)
	router.RegisterPostRoute(datasets, "/:id/columns", h.UpdateColumns)
	router.RegisterGetRoute(datasets, "/:id/preview", h.Preview)
	router.RegisterPostRoute(datasets, "/:id/query", h.Query)
	return r
}

// Pinned dataset entity JSON. Field order follows the entity.Dataset struct
// tags; *string fields serialize as null when unset.
const (
	datasetGetJSON = `{"id":4,"name":"sales","datasource_id":2,"table_name":"orders","query_sql":null,"query_type":"table","mode":"direct","accelerate_config":null,"description":null,"tags":"[]","refresh_strategy":null,"preview_data":null,"quality_rules":"[]","columns":"[]","shard_enabled":false,"shard_keys":"","created_at":"","updated_at":""}`
	notFoundDS     = `{"code":20300,"msg":"dataset not found: sql: no rows in result set","trace":"","data":{}}`
)

func testDataset() *entity.Dataset {
	tableName := "orders"
	return &entity.Dataset{
		ID:           4,
		Name:         "sales",
		DatasourceID: 2,
		TableName:    &tableName,
		QueryType:    "table",
		Mode:         "direct",
		Tags:         "[]",
		QualityRules: "[]",
		Columns:      "[]",
	}
}

// ---------------------------------------------------------------- List

func TestDatasetList_Defaults(t *testing.T) {
	var gotLimit, gotOffset int
	h := NewDatasetHandler(&mockDatasetService{
		listFunc: func(_ context.Context, limit, offset int) ([]entity.Dataset, error) {
			gotLimit, gotOffset = limit, offset
			return nil, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodGet, "/api/datasets", "")
	assertBody(t, w, okEnvelopeEmptyArray)
	if gotLimit != 100 || gotOffset != 0 {
		t.Errorf("expected defaults limit=100 offset=0, got limit=%d offset=%d", gotLimit, gotOffset)
	}
}

func TestDatasetList_ExplicitPaging(t *testing.T) {
	var gotLimit, gotOffset int
	h := NewDatasetHandler(&mockDatasetService{
		listFunc: func(_ context.Context, limit, offset int) ([]entity.Dataset, error) {
			gotLimit, gotOffset = limit, offset
			d := *testDataset()
			return []entity.Dataset{d}, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodGet, "/api/datasets?limit=2&offset=3", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":[`+datasetGetJSON+`]}`)
	if gotLimit != 2 || gotOffset != 3 {
		t.Errorf("expected limit=2 offset=3, got limit=%d offset=%d", gotLimit, gotOffset)
	}
}

func TestDatasetList_InvalidAndClampedPaging(t *testing.T) {
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
		h := NewDatasetHandler(&mockDatasetService{
			listFunc: func(_ context.Context, limit, offset int) ([]entity.Dataset, error) {
				gotLimit, gotOffset = limit, offset
				return nil, nil
			},
		})
		w := serve(newDatasetTestRouter(h), http.MethodGet, "/api/datasets"+tc.query, "")
		assertBody(t, w, okEnvelopeEmptyArray)
		if gotLimit != tc.wantLimit || gotOffset != tc.wantOff {
			t.Errorf("query %q: expected limit=%d offset=%d, got limit=%d offset=%d",
				tc.query, tc.wantLimit, tc.wantOff, gotLimit, gotOffset)
		}
	}
}

func TestDatasetList_ServiceError(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{
		listFunc: func(_ context.Context, _, _ int) ([]entity.Dataset, error) {
			return nil, errBoom()
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodGet, "/api/datasets", "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Get

func TestDatasetGet_Found(t *testing.T) {
	var gotID int
	h := NewDatasetHandler(&mockDatasetService{
		getByIDFunc: func(_ context.Context, id int) (*entity.Dataset, error) {
			gotID = id
			return testDataset(), nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodGet, "/api/datasets/4", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":`+datasetGetJSON+`}`)
	if gotID != 4 {
		t.Errorf("expected GetByID called with id=4, got %d", gotID)
	}
}

func TestDatasetGet_NotFound(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{
		getByIDFunc: func(_ context.Context, _ int) (*entity.Dataset, error) {
			return nil, errors.New("dataset not found: sql: no rows in result set")
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodGet, "/api/datasets/999", "")
	assertBody(t, w, notFoundDS)
}

func TestDatasetGet_InvalidID(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{})
	w := serve(newDatasetTestRouter(h), http.MethodGet, "/api/datasets/abc", "")
	assertBody(t, w, badRequestInvalidID)
}

// ---------------------------------------------------------------- Create

func TestDatasetCreate_Success(t *testing.T) {
	var got *entity.Dataset
	h := NewDatasetHandler(&mockDatasetService{
		createFunc: func(_ context.Context, ds *entity.Dataset) (*entity.Dataset, error) {
			got = ds
			ds.ID = 9
			return ds, nil
		},
	})
	body := `{"name":"sales","datasource_id":2,"table_name":"orders","query_type":"table","mode":"direct","tags":"[]","quality_rules":"[]","columns":"[]","shard_enabled":false,"shard_keys":""}`
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets", body)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"id":9,"name":"sales","datasource_id":2,"table_name":"orders","query_sql":null,"query_type":"table","mode":"direct","accelerate_config":null,"description":null,"tags":"[]","refresh_strategy":null,"preview_data":null,"quality_rules":"[]","columns":"[]","shard_enabled":false,"shard_keys":"","created_at":"","updated_at":""}}`)
	if got == nil || got.DatasourceID != 2 || got.TableName == nil || *got.TableName != "orders" {
		t.Fatalf("service received unexpected entity: %+v", got)
	}
}

func TestDatasetCreate_SQLModeDefaults(t *testing.T) {
	// query_sql provided, table_name omitted: the pointer fields follow the
	// non-empty rule; query_type/mode/tags/quality_rules/columns keep their
	// handler-side defaults.
	var got *entity.Dataset
	h := NewDatasetHandler(&mockDatasetService{
		createFunc: func(_ context.Context, ds *entity.Dataset) (*entity.Dataset, error) {
			got = ds
			return ds, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets",
		`{"name":"q","datasource_id":1,"query_sql":"SELECT 1","query_type":"sql"}`)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"id":0,"name":"q","datasource_id":1,"table_name":null,"query_sql":"SELECT 1","query_type":"sql","mode":"direct","accelerate_config":null,"description":null,"tags":"[]","refresh_strategy":null,"preview_data":null,"quality_rules":"[]","columns":"[]","shard_enabled":false,"shard_keys":"","created_at":"","updated_at":""}}`)
	if got == nil || got.TableName != nil || got.QuerySQL == nil || *got.QuerySQL != "SELECT 1" {
		t.Fatalf("unexpected pointer-field handling: %+v", got)
	}
}

func TestDatasetCreate_DescriptionPointer(t *testing.T) {
	var got *entity.Dataset
	h := NewDatasetHandler(&mockDatasetService{
		createFunc: func(_ context.Context, ds *entity.Dataset) (*entity.Dataset, error) {
			got = ds
			return ds, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets",
		`{"name":"d","description":"hello"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}
	if got == nil || got.Description == nil || *got.Description != "hello" {
		t.Fatalf("description pointer not set: %+v", got)
	}
}

func TestDatasetCreate_EmptyBody(t *testing.T) {
	// The current handler calls ShouldBindJSON unconditionally: an empty body
	// must answer 20100 with the raw "EOF" binding error, never create.
	h := NewDatasetHandler(&mockDatasetService{
		createFunc: func(_ context.Context, _ *entity.Dataset) (*entity.Dataset, error) {
			t.Fatal("Create must not be called for an empty body")
			return nil, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets", "")
	assertBody(t, w, badRequestEOF)
}

func TestDatasetCreate_TypeMismatchBody(t *testing.T) {
	// Accepted unavoidable diff #1 (datasource package doc): the named In
	// type leaks into the json bind-error text. Pre-migration the handler
	// bound an anonymous struct, so the old message was
	// "…Go struct field .datasource_id…"; the datasource baseline
	// (datasourceUpdateIn.port) pins the same class of change.
	h := NewDatasetHandler(&mockDatasetService{
		createFunc: func(_ context.Context, _ *entity.Dataset) (*entity.Dataset, error) {
			t.Fatal("Create must not be called when the body fails to bind")
			return nil, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets", `{"name":"x","datasource_id":"abc"}`)
	assertBody(t, w, `{"code":20100,"msg":"json: cannot unmarshal string into Go struct field datasetCreateIn.datasource_id of type int","trace":"","data":{}}`)
}

func TestDatasetCreate_QueryMustNotPolluteBody(t *testing.T) {
	// Body structs are bound from JSON only; a stray query param must not
	// leak into any In field.
	var got *entity.Dataset
	h := NewDatasetHandler(&mockDatasetService{
		createFunc: func(_ context.Context, ds *entity.Dataset) (*entity.Dataset, error) {
			got = ds
			return ds, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets?Name=evil", `{"datasource_id":3}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}
	if got == nil || got.Name != "" {
		t.Fatalf("query param leaked into body struct: %+v", got)
	}
}

func TestDatasetCreate_ServiceError(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{
		createFunc: func(_ context.Context, _ *entity.Dataset) (*entity.Dataset, error) {
			return nil, errBoom()
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets", `{"name":"x"}`)
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Delete

func TestDatasetDelete_Success(t *testing.T) {
	var gotID int
	h := NewDatasetHandler(&mockDatasetService{
		deleteFunc: func(_ context.Context, id int) error {
			gotID = id
			return nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodDelete, "/api/datasets/5", "")
	assertBody(t, w, okEnvelopeStatus)
	if gotID != 5 {
		t.Errorf("expected Delete called with id=5, got %d", gotID)
	}
}

func TestDatasetDelete_InvalidID(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{})
	w := serve(newDatasetTestRouter(h), http.MethodDelete, "/api/datasets/abc", "")
	assertBody(t, w, badRequestInvalidID)
}

func TestDatasetDelete_ServiceError(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{
		deleteFunc: func(_ context.Context, _ int) error { return errBoom() },
	})
	w := serve(newDatasetTestRouter(h), http.MethodDelete, "/api/datasets/5", "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- GetColumns

func TestDatasetGetColumns_Success(t *testing.T) {
	var gotID int
	h := NewDatasetHandler(&mockDatasetService{
		getColumnsFunc: func(_ context.Context, id int) ([]entity.DatasetColumn, error) {
			gotID = id
			return []entity.DatasetColumn{
				{Name: "id", Expr: "id", Type: "int8", Comment: "", Role: "dimension"},
				{Name: "amount", Expr: "SUM(amount)", Type: "float", TypeConfig: entity.TypeConfig{Precision: 2, Scale: 1}, Comment: "c", Role: "metric"},
			}, nil
		},
	})
	// Key order follows the entity.DatasetColumn struct tags.
	w := serve(newDatasetTestRouter(h), http.MethodGet, "/api/datasets/7/columns", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":[{"name":"id","expr":"id","type":"int8","type_config":{"precision":0,"scale":0},"comment":"","role":"dimension"},{"name":"amount","expr":"SUM(amount)","type":"float","type_config":{"precision":2,"scale":1},"comment":"c","role":"metric"}]}`)
	if gotID != 7 {
		t.Errorf("expected GetColumns called with id=7, got %d", gotID)
	}
}

func TestDatasetGetColumns_Empty(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{
		getColumnsFunc: func(_ context.Context, _ int) ([]entity.DatasetColumn, error) {
			return nil, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodGet, "/api/datasets/7/columns", "")
	assertBody(t, w, okEnvelopeEmptyArray)
}

func TestDatasetGetColumns_InvalidID(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{})
	w := serve(newDatasetTestRouter(h), http.MethodGet, "/api/datasets/abc/columns", "")
	assertBody(t, w, badRequestInvalidID)
}

func TestDatasetGetColumns_ServiceError(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{
		getColumnsFunc: func(_ context.Context, _ int) ([]entity.DatasetColumn, error) {
			return nil, errBoom()
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodGet, "/api/datasets/7/columns", "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- UpdateColumns

func TestDatasetUpdateColumns_Success(t *testing.T) {
	var gotID int
	var gotCols []entity.DatasetColumn
	h := NewDatasetHandler(&mockDatasetService{
		updateColumnsFunc: func(_ context.Context, id int, columns []entity.DatasetColumn) (*entity.Dataset, error) {
			gotID, gotCols = id, columns
			d := *testDataset()
			return &d, nil
		},
	})
	// Bare array body — not a wrapper object.
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets/4/columns",
		`[{"name":"id","expr":"id","type":"int8","type_config":{"precision":0,"scale":0},"comment":"","role":"dimension"}]`)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":`+datasetGetJSON+`}`)
	if gotID != 4 || len(gotCols) != 1 || gotCols[0].Name != "id" || gotCols[0].Role != "dimension" {
		t.Fatalf("unexpected UpdateColumns args: id=%d cols=%+v", gotID, gotCols)
	}
}

func TestDatasetUpdateColumns_EmptyBody(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{
		updateColumnsFunc: func(_ context.Context, _ int, _ []entity.DatasetColumn) (*entity.Dataset, error) {
			t.Fatal("UpdateColumns must not be called for an empty body")
			return nil, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets/4/columns", "")
	assertBody(t, w, badRequestEOF)
}

func TestDatasetUpdateColumns_InvalidIDValidBody(t *testing.T) {
	// Invalid id with a VALID body keeps the pre-migration "invalid id"
	// answer: the path id is still checked before anything else inside the
	// handler (only doubly-invalid requests drift, see the pair below).
	h := NewDatasetHandler(&mockDatasetService{
		updateColumnsFunc: func(_ context.Context, _ int, _ []entity.DatasetColumn) (*entity.Dataset, error) {
			t.Fatal("UpdateColumns must not run for an invalid id")
			return nil, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets/abc/columns", `[]`)
	assertBody(t, w, badRequestInvalidID)
}

func TestDatasetUpdateColumns_InvalidIDPrefersBodyBindError_EmptyBody(t *testing.T) {
	// Pinned accepted unavoidable diff #2 (datasource package doc): the
	// generic router binds the POST body before the handler parses the path
	// id, so a request carrying BOTH an unparseable :id AND an empty body
	// answers with the body-bind error ("EOF") where the pre-migration
	// handler answered "invalid id". Same shape as the datasource
	// Test*_InvalidIDPrefersBodyBindError baselines. Requests valid on
	// either input are unaffected (see InvalidIDValidBody / EmptyBody above).
	h := NewDatasetHandler(&mockDatasetService{
		updateColumnsFunc: func(_ context.Context, _ int, _ []entity.DatasetColumn) (*entity.Dataset, error) {
			t.Fatal("UpdateColumns must not run when both inputs are invalid")
			return nil, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets/abc/columns", "")
	assertBody(t, w, badRequestEOF)
}

func TestDatasetUpdateColumns_TypeMismatchBody(t *testing.T) {
	// UpdateColumns binds []entity.DatasetColumn directly (bare array), so
	// the json error carries no handler-struct name: unlike Create/Query the
	// message is expected to survive migration byte-for-byte.
	h := NewDatasetHandler(&mockDatasetService{
		updateColumnsFunc: func(_ context.Context, _ int, _ []entity.DatasetColumn) (*entity.Dataset, error) {
			t.Fatal("UpdateColumns must not be called when the body fails to bind")
			return nil, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets/4/columns", `[1]`)
	assertBody(t, w, `{"code":20100,"msg":"json: cannot unmarshal number into .0 of type entity.DatasetColumn","trace":"","data":{}}`)
}

func TestDatasetUpdateColumns_ServiceError(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{
		updateColumnsFunc: func(_ context.Context, _ int, _ []entity.DatasetColumn) (*entity.Dataset, error) {
			return nil, errBoom()
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets/4/columns", `[]`)
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Preview
// Dataset Preview is a GET with NO request body (unlike the datasource
// Preview): it is not a path+body shape, so it never exhibits the
// bind-before-path error-priority drift.

func TestDatasetPreview_Success(t *testing.T) {
	var gotID int
	h := NewDatasetHandler(&mockDatasetService{
		previewFunc: func(_ context.Context, id int) (*entity.PreviewResult, error) {
			gotID = id
			return &entity.PreviewResult{Columns: []string{"id"}, Data: []map[string]any{{"id": 1}}}, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodGet, "/api/datasets/4/preview", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"columns":["id"],"data":[{"id":1}]}}`)
	if gotID != 4 {
		t.Errorf("expected Preview called with id=4, got %d", gotID)
	}
}

func TestDatasetPreview_InvalidID(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{})
	w := serve(newDatasetTestRouter(h), http.MethodGet, "/api/datasets/abc/preview", "")
	assertBody(t, w, badRequestInvalidID)
}

func TestDatasetPreview_ServiceError(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{
		previewFunc: func(_ context.Context, _ int) (*entity.PreviewResult, error) {
			return nil, errBoom()
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodGet, "/api/datasets/4/preview", "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Query

func TestDatasetQuery_Success(t *testing.T) {
	var gotID int
	var gotCfg entity.QueryConfig
	h := NewDatasetHandler(&mockDatasetService{
		queryFunc: func(_ context.Context, id int, config entity.QueryConfig) ([]map[string]any, error) {
			gotID, gotCfg = id, config
			return []map[string]any{{"status": "paid", "cnt": 3}}, nil
		},
	})
	body := `{"dimension_groups":[{"id":"g1","fields":["status"]}],"limit":10,"filters":[{"id":"f1","field":"status","operator":"eq","value":"paid","logic":"and"}]}`
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets/4/query", body)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":[{"cnt":3,"status":"paid"}]}`)
	if gotID != 4 || gotCfg.Limit != 10 || len(gotCfg.DimensionGroups) != 1 ||
		gotCfg.DimensionGroups[0].ID != "g1" || len(gotCfg.Filters) != 1 || gotCfg.Filters[0].Operator != "eq" {
		t.Fatalf("unexpected Query args: id=%d cfg=%+v", gotID, gotCfg)
	}
}

func TestDatasetQuery_EmptyBody(t *testing.T) {
	// Empty body with a valid id: ShouldBindJSON answers EOF pre-migration;
	// the router's unconditional POST bind keeps the same answer.
	h := NewDatasetHandler(&mockDatasetService{
		queryFunc: func(_ context.Context, _ int, _ entity.QueryConfig) ([]map[string]any, error) {
			t.Fatal("Query must not be called for an empty body")
			return nil, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets/4/query", "")
	assertBody(t, w, badRequestEOF)
}

func TestDatasetQuery_InvalidIDValidBody(t *testing.T) {
	// Invalid id with a VALID body keeps "invalid id" across migration — only
	// doubly-invalid requests drift, see the test below.
	h := NewDatasetHandler(&mockDatasetService{
		queryFunc: func(_ context.Context, _ int, _ entity.QueryConfig) ([]map[string]any, error) {
			t.Fatal("Query must not run for an invalid id")
			return nil, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets/abc/query", `{}`)
	assertBody(t, w, badRequestInvalidID)
}

func TestDatasetQuery_InvalidIDPrefersBodyBindError_EmptyBody(t *testing.T) {
	// Pinned accepted unavoidable diff #2 (see the UpdateColumns twin):
	// invalid :id + empty body now answers "EOF" (body bound before the
	// path id), where the pre-migration handler answered "invalid id".
	h := NewDatasetHandler(&mockDatasetService{
		queryFunc: func(_ context.Context, _ int, _ entity.QueryConfig) ([]map[string]any, error) {
			t.Fatal("Query must not run when both inputs are invalid")
			return nil, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets/abc/query", "")
	assertBody(t, w, badRequestEOF)
}

func TestDatasetQuery_TypeMismatchBody(t *testing.T) {
	// Accepted unavoidable diff #1 (datasource package doc): the migration
	// binds a handler-local mirror struct with form:"-" (entity.QueryConfig
	// cannot carry the tags), so the struct name in the json error changes
	// from "QueryConfig.limit" to "datasetQueryIn.limit". Using
	// entity.QueryConfig directly as In would keep this message identical
	// but let query params leak into the body (ShouldBindQuery falls back to
	// Go field names), a worse contract break; the tradeoff is ruled in
	// task-5-report.md.
	h := NewDatasetHandler(&mockDatasetService{
		queryFunc: func(_ context.Context, _ int, _ entity.QueryConfig) ([]map[string]any, error) {
			t.Fatal("Query must not be called when the body fails to bind")
			return nil, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets/4/query", `{"limit":"abc"}`)
	assertBody(t, w, `{"code":20100,"msg":"json: cannot unmarshal string into Go struct field datasetQueryIn.limit of type int","trace":"","data":{}}`)
}

func TestDatasetQuery_ServiceError(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{
		queryFunc: func(_ context.Context, _ int, _ entity.QueryConfig) ([]map[string]any, error) {
			return nil, errBoom()
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets/4/query", `{}`)
	assertBody(t, w, internalErrorBoom)
}

func TestDatasetQuery_NilResultNormalizesToEmptyArray(t *testing.T) {
	h := NewDatasetHandler(&mockDatasetService{
		queryFunc: func(_ context.Context, _ int, _ entity.QueryConfig) ([]map[string]any, error) {
			return nil, nil
		},
	})
	w := serve(newDatasetTestRouter(h), http.MethodPost, "/api/datasets/4/query", `{}`)
	assertBody(t, w, okEnvelopeEmptyArray)
}
