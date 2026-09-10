package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dataray/internal/domain/entity"
	"dataray/internal/service/datasource"

	"github.com/gin-gonic/gin"
)

// mockDatasourceService implements datasource.Service for handler tests.
// The embedded interface is nil on purpose: calling a method that is not
// overridden fails loudly, which keeps each test's stub surface explicit.
type mockDatasourceService struct {
	datasource.Service

	listFunc              func(ctx context.Context, limit, offset int) ([]entity.Datasource, error)
	getByIDFunc           func(ctx context.Context, id int) (*entity.Datasource, error)
	createFunc            func(ctx context.Context, ds *entity.Datasource) (*entity.Datasource, error)
	updateFunc            func(ctx context.Context, ds *entity.Datasource) (*entity.Datasource, error)
	deleteFunc            func(ctx context.Context, id int) error
	testConnectionFunc    func(ctx context.Context, config entity.DatasourceConnectionConfig, driverType string) error
	getTablesFunc         func(ctx context.Context, id int) ([]entity.TableInfo, error)
	getColumnsFunc        func(ctx context.Context, id int, tableName string) ([]entity.ColumnInfo, error)
	previewFunc           func(ctx context.Context, id int, tableName, querySQL, queryType string) (*entity.PreviewResult, error)
	fieldDistributionFunc func(ctx context.Context, id int, tableName, querySQL, queryType, fieldName string, limit int) (*entity.FieldDistribution, error)
	tableDataFunc         func(ctx context.Context, id int, tableName string, page, pageSize int, sortField, sortOrder string) (*entity.TableDataResult, error)
}

func (m *mockDatasourceService) List(ctx context.Context, limit, offset int) ([]entity.Datasource, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, limit, offset)
	}
	return nil, nil
}

func (m *mockDatasourceService) GetByID(ctx context.Context, id int) (*entity.Datasource, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockDatasourceService) Create(ctx context.Context, ds *entity.Datasource) (*entity.Datasource, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, ds)
	}
	return nil, nil
}

func (m *mockDatasourceService) Update(ctx context.Context, ds *entity.Datasource) (*entity.Datasource, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, ds)
	}
	return nil, nil
}

func (m *mockDatasourceService) Delete(ctx context.Context, id int) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func (m *mockDatasourceService) TestConnection(ctx context.Context, config entity.DatasourceConnectionConfig, driverType string) error {
	if m.testConnectionFunc != nil {
		return m.testConnectionFunc(ctx, config, driverType)
	}
	return nil
}

func (m *mockDatasourceService) GetTables(ctx context.Context, id int) ([]entity.TableInfo, error) {
	if m.getTablesFunc != nil {
		return m.getTablesFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockDatasourceService) GetColumns(ctx context.Context, id int, tableName string) ([]entity.ColumnInfo, error) {
	if m.getColumnsFunc != nil {
		return m.getColumnsFunc(ctx, id, tableName)
	}
	return nil, nil
}

func (m *mockDatasourceService) Preview(ctx context.Context, id int, tableName, querySQL, queryType string) (*entity.PreviewResult, error) {
	if m.previewFunc != nil {
		return m.previewFunc(ctx, id, tableName, querySQL, queryType)
	}
	return nil, nil
}

func (m *mockDatasourceService) GetFieldDistribution(ctx context.Context, id int, tableName, querySQL, queryType, fieldName string, limit int) (*entity.FieldDistribution, error) {
	if m.fieldDistributionFunc != nil {
		return m.fieldDistributionFunc(ctx, id, tableName, querySQL, queryType, fieldName, limit)
	}
	return nil, nil
}

func (m *mockDatasourceService) GetTableData(ctx context.Context, id int, tableName string, page, pageSize int, sortField, sortOrder string) (*entity.TableDataResult, error) {
	if m.tableDataFunc != nil {
		return m.tableDataFunc(ctx, id, tableName, page, pageSize, sortField, sortOrder)
	}
	return nil, nil
}

// newDatasourceTestRouter mirrors the datasource section of cmd/routes.go.
// During the Batch 2 migration only this wiring helper changes; every
// response-body assertion below must keep passing byte-for-byte before and
// after migration — that is the zero-behavior-change guard.
func newDatasourceTestRouter(h *DatasourceHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ds := r.Group("/api/datasources")
	ds.GET("", h.List)
	ds.POST("", h.Create)
	ds.GET("/:id", h.Get)
	ds.PUT("/:id", h.Update)
	ds.DELETE("/:id", h.Delete)
	ds.POST("/test", h.TestConnection)
	ds.GET("/:id/tables", h.GetTables)
	ds.GET("/:id/tables/:table/columns", h.GetColumns)
	ds.GET("/:id/tables/:table/data", h.GetTableData)
	ds.POST("/:id/preview", h.Preview)
	ds.POST("/:id/field-distribution", h.GetFieldDistribution)
	return r
}

// serve issues an HTTP request against the test router; body may be "" for a
// truly empty body (ContentLength 0, same as a real bodyless POST).
func serve(r *gin.Engine, method, target, body string) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rd)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// assertBody compares the full JSON response body byte-for-byte (ignoring a
// possible trailing newline), pinning the exact envelope contract
// {code, msg, trace, data} the datasource handlers currently emit.
func assertBody(t *testing.T, w *httptest.ResponseRecorder, want string) {
	t.Helper()
	got := strings.TrimSpace(w.Body.String())
	if got != want {
		t.Fatalf("response body mismatch:\n got: %s\nwant: %s", got, want)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP status 200 (unified envelope), got %d", w.Code)
	}
}

const (
	okEnvelopeEmptyArray    = `{"code":20000,"msg":"success","trace":"","data":[]}`
	okEnvelopeStatus        = `{"code":20000,"msg":"success","trace":"","data":{"status":"ok"}}`
	badRequestInvalidID     = `{"code":20100,"msg":"invalid id","trace":"","data":{}}`
	badRequestEOF           = `{"code":20100,"msg":"EOF","trace":"","data":{}}`
	internalErrorBoom       = `{"code":50000,"msg":"boom","trace":"","data":{}}`
	notFoundEnvelope        = `{"code":20300,"msg":"datasource not found: sql: no rows in result set","trace":"","data":{}}`
	entityGetJSON           = `{"id":7,"name":"pg","type":"postgresql","host":"localhost","port":5432,"database_name":"db","username":"user","created_at":"","updated_at":""}`
)

func errBoom() error { return errors.New("boom") }

// ---------------------------------------------------------------- List

func TestDatasourceList_Defaults(t *testing.T) {
	var gotLimit, gotOffset int
	h := NewDatasourceHandler(&mockDatasourceService{
		listFunc: func(_ context.Context, limit, offset int) ([]entity.Datasource, error) {
			gotLimit, gotOffset = limit, offset
			return nil, nil
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources", "")
	assertBody(t, w, okEnvelopeEmptyArray)
	if gotLimit != 100 || gotOffset != 0 {
		t.Errorf("expected defaults limit=100 offset=0, got limit=%d offset=%d", gotLimit, gotOffset)
	}
}

func TestDatasourceList_ExplicitPaging(t *testing.T) {
	var gotLimit, gotOffset int
	h := NewDatasourceHandler(&mockDatasourceService{
		listFunc: func(_ context.Context, limit, offset int) ([]entity.Datasource, error) {
			gotLimit, gotOffset = limit, offset
			return []entity.Datasource{{ID: 1, Name: "a", Type: "postgresql"}}, nil
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources?limit=2&offset=3", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":[{"id":1,"name":"a","type":"postgresql","host":"","port":0,"database_name":"","username":"","created_at":"","updated_at":""}]}`)
	if gotLimit != 2 || gotOffset != 3 {
		t.Errorf("expected limit=2 offset=3, got limit=%d offset=%d", gotLimit, gotOffset)
	}
}

func TestDatasourceList_InvalidAndClampedPaging(t *testing.T) {
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
		h := NewDatasourceHandler(&mockDatasourceService{
			listFunc: func(_ context.Context, limit, offset int) ([]entity.Datasource, error) {
				gotLimit, gotOffset = limit, offset
				return nil, nil
			},
		})
		w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources"+tc.query, "")
		assertBody(t, w, okEnvelopeEmptyArray)
		if gotLimit != tc.wantLimit || gotOffset != tc.wantOff {
			t.Errorf("query %q: expected limit=%d offset=%d, got limit=%d offset=%d",
				tc.query, tc.wantLimit, tc.wantOff, gotLimit, gotOffset)
		}
	}
}

func TestDatasourceList_ServiceError(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{
		listFunc: func(_ context.Context, _, _ int) ([]entity.Datasource, error) {
			return nil, errBoom()
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources", "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Get

func TestDatasourceGet_Found(t *testing.T) {
	var gotID int
	h := NewDatasourceHandler(&mockDatasourceService{
		getByIDFunc: func(_ context.Context, id int) (*entity.Datasource, error) {
			gotID = id
			return &entity.Datasource{ID: 7, Name: "pg", Type: "postgresql", Host: "localhost", Port: 5432, DatabaseName: "db", Username: "user"}, nil
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources/7", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":`+entityGetJSON+`}`)
	if gotID != 7 {
		t.Errorf("expected GetByID called with id=7, got %d", gotID)
	}
}

func TestDatasourceGet_NotFound(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{
		getByIDFunc: func(_ context.Context, _ int) (*entity.Datasource, error) {
			return nil, errors.New("datasource not found: sql: no rows in result set")
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources/999", "")
	assertBody(t, w, notFoundEnvelope)
}

func TestDatasourceGet_InvalidID(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources/abc", "")
	assertBody(t, w, badRequestInvalidID)
}

// ---------------------------------------------------------------- Create

func TestDatasourceCreate_Success(t *testing.T) {
	var got *entity.Datasource
	h := NewDatasourceHandler(&mockDatasourceService{
		createFunc: func(_ context.Context, ds *entity.Datasource) (*entity.Datasource, error) {
			got = ds
			ds.ID = 9
			return ds, nil
		},
	})
	body := `{"name":"new","type":"mysql","host":"h","port":3306,"database_name":"d","username":"u","password":"p"}`
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources", body)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"id":9,"name":"new","type":"mysql","host":"h","port":3306,"database_name":"d","username":"u","created_at":"","updated_at":""}}`)
	if got == nil || got.Password != "p" || got.Type != "mysql" {
		t.Fatalf("service received unexpected entity: %+v", got)
	}
}

func TestDatasourceCreate_DefaultType(t *testing.T) {
	var got *entity.Datasource
	h := NewDatasourceHandler(&mockDatasourceService{
		createFunc: func(_ context.Context, ds *entity.Datasource) (*entity.Datasource, error) {
			got = ds
			return ds, nil
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources", `{"name":"x"}`)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"id":0,"name":"x","type":"postgresql","host":"","port":0,"database_name":"","username":"","created_at":"","updated_at":""}}`)
	if got == nil || got.Type != "postgresql" {
		t.Fatalf("expected default type postgresql, got %+v", got)
	}
}

func TestDatasourceCreate_EmptyBody(t *testing.T) {
	// The current handler calls ShouldBindJSON unconditionally: an empty body
	// must answer 20100 with the raw "EOF" binding error, never create.
	h := NewDatasourceHandler(&mockDatasourceService{
		createFunc: func(_ context.Context, ds *entity.Datasource) (*entity.Datasource, error) {
			t.Fatal("Create must not be called for an empty body")
			return nil, nil
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources", "")
	assertBody(t, w, badRequestEOF)
}

func TestDatasourceCreate_QueryMustNotPolluteBody(t *testing.T) {
	// Body structs are bound from JSON only; a stray query param must not
	// leak into any In field.
	var got *entity.Datasource
	h := NewDatasourceHandler(&mockDatasourceService{
		createFunc: func(_ context.Context, ds *entity.Datasource) (*entity.Datasource, error) {
			got = ds
			return ds, nil
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources?Name=evil", `{"type":"mysql"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", w.Code)
	}
	if got == nil || got.Name != "" {
		t.Fatalf("query param leaked into body struct: %+v", got)
	}
}

func TestDatasourceCreate_ServiceError(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{
		createFunc: func(_ context.Context, _ *entity.Datasource) (*entity.Datasource, error) {
			return nil, errBoom()
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources", `{"name":"x"}`)
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Update

func TestDatasourceUpdate_EmptyPasswordPassthrough(t *testing.T) {
	var got *entity.Datasource
	h := NewDatasourceHandler(&mockDatasourceService{
		updateFunc: func(_ context.Context, ds *entity.Datasource) (*entity.Datasource, error) {
			got = ds
			ds.ID = 3
			return ds, nil
		},
	})
	body := `{"name":"upd","type":"postgresql","host":"h","port":5432,"database_name":"d","username":"u"}`
	w := serve(newDatasourceTestRouter(h), http.MethodPut, "/api/datasources/3", body)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"id":3,"name":"upd","type":"postgresql","host":"h","port":5432,"database_name":"d","username":"u","created_at":"","updated_at":""}}`)
	// The handler forwards an omitted password as "" verbatim; preserving the
	// stored value for an empty password is the service's contract.
	if got == nil || got.ID != 3 || got.Password != "" {
		t.Fatalf("expected ID=3 Password=\"\" passthrough, got %+v", got)
	}
}

func TestDatasourceUpdate_InvalidID(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{})
	w := serve(newDatasourceTestRouter(h), http.MethodPut, "/api/datasources/abc", `{"name":"x"}`)
	assertBody(t, w, badRequestInvalidID)
}

func TestDatasourceUpdate_EmptyBody(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{
		updateFunc: func(_ context.Context, _ *entity.Datasource) (*entity.Datasource, error) {
			t.Fatal("Update must not be called for an empty body")
			return nil, nil
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodPut, "/api/datasources/3", "")
	assertBody(t, w, badRequestEOF)
}

func TestDatasourceUpdate_ServiceError(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{
		updateFunc: func(_ context.Context, _ *entity.Datasource) (*entity.Datasource, error) {
			return nil, errBoom()
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodPut, "/api/datasources/3", `{"name":"x"}`)
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Delete

func TestDatasourceDelete_Success(t *testing.T) {
	var gotID int
	h := NewDatasourceHandler(&mockDatasourceService{
		deleteFunc: func(_ context.Context, id int) error {
			gotID = id
			return nil
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodDelete, "/api/datasources/5", "")
	assertBody(t, w, okEnvelopeStatus)
	if gotID != 5 {
		t.Errorf("expected Delete called with id=5, got %d", gotID)
	}
}

func TestDatasourceDelete_InvalidID(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{})
	w := serve(newDatasourceTestRouter(h), http.MethodDelete, "/api/datasources/abc", "")
	assertBody(t, w, badRequestInvalidID)
}

func TestDatasourceDelete_ServiceError(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{
		deleteFunc: func(_ context.Context, _ int) error { return errBoom() },
	})
	w := serve(newDatasourceTestRouter(h), http.MethodDelete, "/api/datasources/5", "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- TestConnection

func TestDatasourceTestConnection_Success(t *testing.T) {
	var gotCfg entity.DatasourceConnectionConfig
	var gotType string
	h := NewDatasourceHandler(&mockDatasourceService{
		testConnectionFunc: func(_ context.Context, cfg entity.DatasourceConnectionConfig, driverType string) error {
			gotCfg, gotType = cfg, driverType
			return nil
		},
	})
	body := `{"type":"mysql","host":"h","port":3306,"database_name":"d","username":"u","password":"p"}`
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources/test", body)
	assertBody(t, w, okEnvelopeStatus)
	if gotType != "mysql" || gotCfg.Host != "h" || gotCfg.Port != 3306 || gotCfg.Password != "p" {
		t.Fatalf("unexpected config: type=%q cfg=%+v", gotType, gotCfg)
	}
}

func TestDatasourceTestConnection_DefaultType(t *testing.T) {
	var gotType string
	h := NewDatasourceHandler(&mockDatasourceService{
		testConnectionFunc: func(_ context.Context, _ entity.DatasourceConnectionConfig, driverType string) error {
			gotType = driverType
			return nil
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources/test", `{}`)
	assertBody(t, w, okEnvelopeStatus)
	if gotType != "postgresql" {
		t.Errorf("expected default driverType postgresql, got %q", gotType)
	}
}

func TestDatasourceTestConnection_FailureMapsToBadRequest(t *testing.T) {
	// Connection failures are a 400-level business error, not a 500.
	h := NewDatasourceHandler(&mockDatasourceService{
		testConnectionFunc: func(_ context.Context, _ entity.DatasourceConnectionConfig, _ string) error {
			return errors.New("connection failed: dial tcp refused")
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources/test", `{"type":"postgresql"}`)
	assertBody(t, w, `{"code":20100,"msg":"connection failed: dial tcp refused","trace":"","data":{}}`)
}

func TestDatasourceTestConnection_EmptyBody(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{})
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources/test", "")
	assertBody(t, w, badRequestEOF)
}

// ---------------------------------------------------------------- GetTables

func TestDatasourceGetTables_Success(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{
		getTablesFunc: func(_ context.Context, id int) ([]entity.TableInfo, error) {
			if id != 2 {
				t.Errorf("expected id=2, got %d", id)
			}
			return []entity.TableInfo{{Name: "t1", Comment: "c1"}, {Name: "t2"}}, nil
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources/2/tables", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":[{"name":"t1","comment":"c1"},{"name":"t2","comment":""}]}`)
}

func TestDatasourceGetTables_InvalidID(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources/abc/tables", "")
	assertBody(t, w, badRequestInvalidID)
}

func TestDatasourceGetTables_ServiceError(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{
		getTablesFunc: func(_ context.Context, _ int) ([]entity.TableInfo, error) { return nil, errBoom() },
	})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources/2/tables", "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- GetColumns

func TestDatasourceGetColumns_Success(t *testing.T) {
	var gotTable string
	h := NewDatasourceHandler(&mockDatasourceService{
		getColumnsFunc: func(_ context.Context, id int, tableName string) ([]entity.ColumnInfo, error) {
			gotTable = tableName
			return []entity.ColumnInfo{{Name: "id", DataType: "int8", Comment: ""}, {Name: "name", DataType: "text", Comment: "nm"}}, nil
		},
	})
	// The map projection keys serialize in sorted order: comment, data_type, name.
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources/2/tables/orders/columns", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":[{"comment":"","data_type":"int8","name":"id"},{"comment":"nm","data_type":"text","name":"name"}]}`)
	if gotTable != "orders" {
		t.Errorf("expected table 'orders', got %q", gotTable)
	}
}

func TestDatasourceGetColumns_InvalidID(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources/abc/tables/orders/columns", "")
	assertBody(t, w, badRequestInvalidID)
}

func TestDatasourceGetColumns_ServiceError(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{
		getColumnsFunc: func(_ context.Context, _ int, _ string) ([]entity.ColumnInfo, error) { return nil, errBoom() },
	})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources/2/tables/orders/columns", "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Preview

func TestDatasourcePreview_Success(t *testing.T) {
	var gotID int
	var gotTable, gotSQL, gotType string
	h := NewDatasourceHandler(&mockDatasourceService{
		previewFunc: func(_ context.Context, id int, tableName, querySQL, queryType string) (*entity.PreviewResult, error) {
			gotID, gotTable, gotSQL, gotType = id, tableName, querySQL, queryType
			return &entity.PreviewResult{Columns: []string{"id"}, Data: []map[string]any{{"id": 1}}}, nil
		},
	})
	body := `{"table_name":"orders","query_sql":"","query_type":"table"}`
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources/4/preview", body)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"columns":["id"],"data":[{"id":1}]}}`)
	if gotID != 4 || gotTable != "orders" || gotSQL != "" || gotType != "table" {
		t.Fatalf("unexpected preview args: id=%d table=%q sql=%q type=%q", gotID, gotTable, gotSQL, gotType)
	}
}

func TestDatasourcePreview_InvalidID(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{})
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources/abc/preview", `{"table_name":"orders"}`)
	assertBody(t, w, badRequestInvalidID)
}

func TestDatasourcePreview_EmptyBody(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{})
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources/4/preview", "")
	assertBody(t, w, badRequestEOF)
}

func TestDatasourcePreview_ServiceError(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{
		previewFunc: func(_ context.Context, _ int, _, _, _ string) (*entity.PreviewResult, error) {
			return nil, errBoom()
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources/4/preview", `{"table_name":"orders"}`)
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- GetFieldDistribution

func TestDatasourceFieldDistribution_Success(t *testing.T) {
	var gotID int
	var gotField string
	var gotLimit int
	h := NewDatasourceHandler(&mockDatasourceService{
		fieldDistributionFunc: func(_ context.Context, id int, tableName, querySQL, queryType, fieldName string, limit int) (*entity.FieldDistribution, error) {
			gotID, gotField, gotLimit = id, fieldName, limit
			return &entity.FieldDistribution{
				FieldName:    fieldName,
				TotalCount:   4,
				UniqueCount:  2,
				Distribution: []entity.FieldValueCount{{Value: "a", Count: 3, Percentage: 75}, {Value: "b", Count: 1, Percentage: 25}},
			}, nil
		},
	})
	body := `{"table_name":"orders","query_type":"table","field_name":"status","limit":5}`
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources/6/field-distribution", body)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"field_name":"status","total_count":4,"unique_count":2,"distribution":[{"value":"a","count":3,"percentage":75},{"value":"b","count":1,"percentage":25}]}}`)
	if gotID != 6 || gotField != "status" || gotLimit != 5 {
		t.Fatalf("unexpected distribution args: id=%d field=%q limit=%d", gotID, gotField, gotLimit)
	}
}

func TestDatasourceFieldDistribution_RequiresFieldName(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{
		fieldDistributionFunc: func(_ context.Context, _ int, _, _, _, _ string, _ int) (*entity.FieldDistribution, error) {
			t.Fatal("GetFieldDistribution must not be called without field_name")
			return nil, nil
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources/6/field-distribution", `{"table_name":"orders"}`)
	assertBody(t, w, `{"code":20100,"msg":"field_name is required","trace":"","data":{}}`)
}

func TestDatasourceFieldDistribution_InvalidID(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{})
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources/abc/field-distribution", `{"field_name":"x"}`)
	assertBody(t, w, badRequestInvalidID)
}

func TestDatasourceFieldDistribution_EmptyBody(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{})
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources/6/field-distribution", "")
	assertBody(t, w, badRequestEOF)
}

func TestDatasourceFieldDistribution_ServiceError(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{
		fieldDistributionFunc: func(_ context.Context, _ int, _, _, _, _ string, _ int) (*entity.FieldDistribution, error) {
			return nil, errBoom()
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodPost, "/api/datasources/6/field-distribution", `{"field_name":"x"}`)
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- GetTableData

func TestDatasourceGetTableData_Defaults(t *testing.T) {
	var gotPage, gotPageSize int
	var gotSortField, gotSortOrder string
	h := NewDatasourceHandler(&mockDatasourceService{
		tableDataFunc: func(_ context.Context, id int, tableName string, page, pageSize int, sortField, sortOrder string) (*entity.TableDataResult, error) {
			gotPage, gotPageSize, gotSortField, gotSortOrder = page, pageSize, sortField, sortOrder
			return &entity.TableDataResult{
				Columns:     []string{"id"},
				Total:       0,
				PrimaryKeys: []string{"id"},
				Page:        page,
				PageSize:    pageSize,
			}, nil
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources/1/tables/orders/data", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"columns":["id"],"data":[],"total":0,"primary_keys":["id"],"page":1,"page_size":20}}`)
	if gotPage != 1 || gotPageSize != 20 || gotSortField != "" || gotSortOrder != "ASC" {
		t.Errorf("expected defaults page=1 page_size=20 sort_field=\"\" sort_order=ASC, got %d/%d/%q/%q",
			gotPage, gotPageSize, gotSortField, gotSortOrder)
	}
}

func TestDatasourceGetTableData_ExplicitParams(t *testing.T) {
	var gotPage, gotPageSize int
	var gotSortField, gotSortOrder string
	h := NewDatasourceHandler(&mockDatasourceService{
		tableDataFunc: func(_ context.Context, id int, tableName string, page, pageSize int, sortField, sortOrder string) (*entity.TableDataResult, error) {
			gotPage, gotPageSize, gotSortField, gotSortOrder = page, pageSize, sortField, sortOrder
			return &entity.TableDataResult{Page: page, PageSize: pageSize}, nil
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources/1/tables/orders/data?page=2&page_size=10&sort_field=id&sort_order=desc", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"columns":[],"data":[],"total":0,"primary_keys":[],"page":2,"page_size":10}}`)
	if gotPage != 2 || gotPageSize != 10 || gotSortField != "id" || gotSortOrder != "desc" {
		t.Errorf("got %d/%d/%q/%q", gotPage, gotPageSize, gotSortField, gotSortOrder)
	}
}

func TestDatasourceGetTableData_NonPrimaryKeySortFieldMapsTo50000(t *testing.T) {
	// The real service rejects sort fields outside the primary key set; the
	// handler must surface that as 50000 with the service message verbatim.
	h := NewDatasourceHandler(&mockDatasourceService{
		tableDataFunc: func(_ context.Context, _ int, _ string, _ int, _ int, _, _ string) (*entity.TableDataResult, error) {
			return nil, errors.New("sort field must be a primary key column: notes")
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources/1/tables/orders/data?sort_field=notes", "")
	assertBody(t, w, `{"code":50000,"msg":"sort field must be a primary key column: notes","trace":"","data":{}}`)
}

func TestDatasourceGetTableData_GarbagePagingStillSucceeds(t *testing.T) {
	// Garbage page/page_size must not 400; parse errors fall back to 0 and
	// the service clamps them (observable output identical to defaults).
	h := NewDatasourceHandler(&mockDatasourceService{
		tableDataFunc: func(_ context.Context, _ int, _ string, _, _ int, _, _ string) (*entity.TableDataResult, error) {
			return &entity.TableDataResult{Page: 1, PageSize: 20}, nil
		},
	})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources/1/tables/orders/data?page=abc&page_size=xyz", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"columns":[],"data":[],"total":0,"primary_keys":[],"page":1,"page_size":20}}`)
}

func TestDatasourceGetTableData_InvalidID(t *testing.T) {
	h := NewDatasourceHandler(&mockDatasourceService{})
	w := serve(newDatasourceTestRouter(h), http.MethodGet, "/api/datasources/abc/tables/orders/data", "")
	assertBody(t, w, badRequestInvalidID)
}
