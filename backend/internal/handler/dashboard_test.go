package handler

import (
	"context"
	"net/http"
	"testing"

	"data-insights/internal/domain/entity"
	"data-insights/internal/router"
	"data-insights/internal/service/dashboard"

	"github.com/gin-gonic/gin"
)

// mockDashboardService implements dashboard.Service for handler tests. The
// embedded interface is nil on purpose (same style as mockChartService):
// calling a non-overridden method fails loudly instead of silently returning
// zero values.
type mockDashboardService struct {
	dashboard.Service

	listFunc   func(ctx context.Context, limit, offset int) ([]entity.Dashboard, error)
	getFunc    func(ctx context.Context, id string) (*entity.Dashboard, error)
	createFunc func(ctx context.Context, in entity.DashboardCreateRequest) (*entity.Dashboard, error)
	updateFunc func(ctx context.Context, id string, in entity.DashboardUpdateRequest) (*entity.Dashboard, error)
	deleteFunc func(ctx context.Context, id string) error
	refsFunc   func(ctx context.Context, chartID int) ([]entity.Dashboard, error)
	queryFunc  func(ctx context.Context, id string, in entity.DashboardQueryRequest) (*entity.DashboardQueryResult, error)
}

func (m *mockDashboardService) List(ctx context.Context, limit, offset int) ([]entity.Dashboard, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, limit, offset)
	}
	return nil, nil
}

func (m *mockDashboardService) Get(ctx context.Context, id string) (*entity.Dashboard, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockDashboardService) Create(ctx context.Context, in entity.DashboardCreateRequest) (*entity.Dashboard, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, in)
	}
	return nil, nil
}

func (m *mockDashboardService) Update(ctx context.Context, id string, in entity.DashboardUpdateRequest) (*entity.Dashboard, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, id, in)
	}
	return nil, nil
}

func (m *mockDashboardService) Delete(ctx context.Context, id string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func (m *mockDashboardService) CountChartReferences(ctx context.Context, chartID int) ([]entity.Dashboard, error) {
	if m.refsFunc != nil {
		return m.refsFunc(ctx, chartID)
	}
	return nil, nil
}

func (m *mockDashboardService) Query(ctx context.Context, id string, in entity.DashboardQueryRequest) (*entity.DashboardQueryResult, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, id, in)
	}
	return nil, nil
}

// newDashboardTestRouter mirrors the dashboard/chart-reference wiring of
// cmd/routes.go. serve/assertBody and the shared envelope constants come from
// datasource_test.go.
func newDashboardTestRouter(h *DashboardHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	dashboards := r.Group("/api/dashboards")
	router.RegisterGetRoute(dashboards, "", h.List)
	router.RegisterPostRoute(dashboards, "", h.Create)
	router.RegisterGetRoute(dashboards, "/:id", h.Get)
	router.RegisterPutRoute(dashboards, "/:id", h.Update)
	router.RegisterDeleteRoute(dashboards, "/:id", h.Delete)
	router.RegisterPostRoute(dashboards, "/:id/query", h.Query)
	charts := r.Group("/api/charts")
	router.RegisterGetRoute(charts, "/:id/references", h.ListChartReferences)
	return r
}

const (
	dashID = "0198f2c3-4d5e-7a6b-8c9d-0e1f2a3b4c5d"
	// dashLayout 是内存/请求体里的原形态；dashLayoutJSON 是嵌进期望响应字符串时
	// 的转义形态（内嵌双引号必须转义）。
	dashLayout       = `{"version":1,"widgets":[]}`
	dashLayoutJSON   = `{\"version\":1,\"widgets\":[]}`
	dashFoundJSON    = `{"id":"` + dashID + `","name":"月度经营总览","description":null,"layout_json":"` + dashLayoutJSON + `","status":"draft","created_at":"","updated_at":""}`
	dashNotFound     = `{"code":20300,"msg":"dashboard not found","trace":"","data":{}}`
	dashBadLayout    = `{"code":20100,"msg":"layout_json 必须是合法 JSON","trace":"","data":{}}`
	refEmptyEnvelope = `{"code":20000,"msg":"success","trace":"","data":{"count":0,"dashboards":[]}}`
)

// ---------------------------------------------------------------- List

func TestDashboardList_Defaults(t *testing.T) {
	var gotLimit, gotOffset int
	h := NewDashboardHandler(&mockDashboardService{
		listFunc: func(_ context.Context, limit, offset int) ([]entity.Dashboard, error) {
			gotLimit, gotOffset = limit, offset
			return nil, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodGet, "/api/dashboards", "")
	assertBody(t, w, okEnvelopeEmptyArray)
	if gotLimit != 100 || gotOffset != 0 {
		t.Errorf("expected defaults limit=100 offset=0, got limit=%d offset=%d", gotLimit, gotOffset)
	}
}

func TestDashboardList_ExplicitPaging(t *testing.T) {
	var gotLimit, gotOffset int
	h := NewDashboardHandler(&mockDashboardService{
		listFunc: func(_ context.Context, limit, offset int) ([]entity.Dashboard, error) {
			gotLimit, gotOffset = limit, offset
			return []entity.Dashboard{{ID: dashID, Name: "月度经营总览", LayoutJSON: dashLayout, Status: "draft"}}, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodGet, "/api/dashboards?limit=2&offset=3", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":[`+dashFoundJSON+`]}`)
	if gotLimit != 2 || gotOffset != 3 {
		t.Errorf("expected limit=2 offset=3, got limit=%d offset=%d", gotLimit, gotOffset)
	}
}

func TestDashboardList_InvalidAndClampedPaging(t *testing.T) {
	cases := []struct {
		query     string
		wantLimit int
		wantOff   int
	}{
		// Garbage must NOT become a 400: the handler ignores parse errors and
		// falls back to the clamped default (same as chart/datasource lists).
		{"?limit=abc&offset=xyz", 100, 0},
		{"?limit=5000", 100, 0},
		{"?limit=0", 100, 0},
		{"?limit=-5", 100, 0},
		{"?offset=-1", 100, 0},
		{"?limit=1000", 1000, 0},
	}
	for _, tc := range cases {
		var gotLimit, gotOffset int
		h := NewDashboardHandler(&mockDashboardService{
			listFunc: func(_ context.Context, limit, offset int) ([]entity.Dashboard, error) {
				gotLimit, gotOffset = limit, offset
				return nil, nil
			},
		})
		w := serve(newDashboardTestRouter(h), http.MethodGet, "/api/dashboards"+tc.query, "")
		assertBody(t, w, okEnvelopeEmptyArray)
		if gotLimit != tc.wantLimit || gotOffset != tc.wantOff {
			t.Errorf("query %q: expected limit=%d offset=%d, got limit=%d offset=%d",
				tc.query, tc.wantLimit, tc.wantOff, gotLimit, gotOffset)
		}
	}
}

func TestDashboardList_ServiceError(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		listFunc: func(_ context.Context, _, _ int) ([]entity.Dashboard, error) {
			return nil, errBoom()
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodGet, "/api/dashboards", "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Get

func TestDashboardGet_Found(t *testing.T) {
	var gotID string
	h := NewDashboardHandler(&mockDashboardService{
		getFunc: func(_ context.Context, id string) (*entity.Dashboard, error) {
			gotID = id
			return &entity.Dashboard{ID: id, Name: "月度经营总览", LayoutJSON: dashLayout, Status: "draft"}, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodGet, "/api/dashboards/"+dashID, "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":`+dashFoundJSON+`}`)
	if gotID != dashID {
		t.Errorf("expected Get called with id=%q, got %q", dashID, gotID)
	}
}

func TestDashboardGet_NotFound(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		getFunc: func(_ context.Context, _ string) (*entity.Dashboard, error) {
			return nil, router.NewBusinessError(20300, "dashboard not found")
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodGet, "/api/dashboards/"+dashID, "")
	assertBody(t, w, dashNotFound)
}

// TestDashboardGet_InvalidID 非 UUID 的 id 必须在 handler 就挡掉：bi_dashboard.id
// 是 UUID 列，放开会让 PostgreSQL 的转型错误以 500 的形式冒出来。
func TestDashboardGet_InvalidID(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		getFunc: func(_ context.Context, _ string) (*entity.Dashboard, error) {
			t.Fatal("Get must not run for a non-UUID id")
			return nil, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodGet, "/api/dashboards/not-a-uuid", "")
	assertBody(t, w, badRequestInvalidID)
}

// ---------------------------------------------------------------- Create

func TestDashboardCreate_Success(t *testing.T) {
	var got entity.DashboardCreateRequest
	h := NewDashboardHandler(&mockDashboardService{
		createFunc: func(_ context.Context, in entity.DashboardCreateRequest) (*entity.Dashboard, error) {
			got = in
			return &entity.Dashboard{ID: dashID, Name: in.Name, LayoutJSON: dashLayout, Status: "draft"}, nil
		},
	})
	body := `{"name":"月度经营总览","description":"口径说明","layout_json":"{\"version\":1,\"widgets\":[]}"}`
	w := serve(newDashboardTestRouter(h), http.MethodPost, "/api/dashboards", body)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":`+dashFoundJSON+`}`)

	if got.Name != "月度经营总览" {
		t.Errorf("name 未透传: %q", got.Name)
	}
	if got.Description == nil || *got.Description != "口径说明" {
		t.Errorf("description 未透传: %+v", got.Description)
	}
	if got.LayoutJSON != dashLayout {
		t.Errorf("layout_json 未透传: %q", got.LayoutJSON)
	}
	// 缺省值由 service 负责，handler 必须原样传空串而不是替 service 做决定。
	if got.Status != "" {
		t.Errorf("未提供的 status 应原样传空串, 实际 %q", got.Status)
	}
}

func TestDashboardCreate_EmptyBody(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		createFunc: func(_ context.Context, _ entity.DashboardCreateRequest) (*entity.Dashboard, error) {
			t.Fatal("Create must not be called for an empty body")
			return nil, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodPost, "/api/dashboards", "")
	assertBody(t, w, badRequestEOF)
}

func TestDashboardCreate_TypeMismatchBody(t *testing.T) {
	// Accepted unavoidable diff #1 (datasource package doc): the named In type
	// leaks into the json bind-error text.
	h := NewDashboardHandler(&mockDashboardService{
		createFunc: func(_ context.Context, _ entity.DashboardCreateRequest) (*entity.Dashboard, error) {
			t.Fatal("Create must not be called when the body fails to bind")
			return nil, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodPost, "/api/dashboards", `{"name":[1]}`)
	assertBody(t, w, `{"code":20100,"msg":"json: cannot unmarshal array into Go struct field dashboardCreateIn.name of type string","trace":"","data":{}}`)
}

// TestDashboardCreate_QueryMustNotPolluteBody 泛型路由对 POST 会先跑一遍
// ShouldBindQuery；In 镜像上的 form:"-" 是唯一挡住 query 参数混入 body 的东西。
func TestDashboardCreate_QueryMustNotPolluteBody(t *testing.T) {
	var got entity.DashboardCreateRequest
	h := NewDashboardHandler(&mockDashboardService{
		createFunc: func(_ context.Context, in entity.DashboardCreateRequest) (*entity.Dashboard, error) {
			got = in
			return &entity.Dashboard{ID: dashID, Name: in.Name}, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodPost,
		"/api/dashboards?Name=evil&LayoutJSON=%7B%7D&Status=published", `{"name":"ok"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	if got.Name != "ok" || got.LayoutJSON != "" || got.Status != "" {
		t.Fatalf("query 参数污染了 body struct: %+v", got)
	}
}

func TestDashboardCreate_ServiceError(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		createFunc: func(_ context.Context, _ entity.DashboardCreateRequest) (*entity.Dashboard, error) {
			return nil, router.NewBusinessError(20100, "layout_json 必须是合法 JSON")
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodPost, "/api/dashboards", `{"name":"x","layout_json":"{bad"}`)
	assertBody(t, w, dashBadLayout)
}

// ---------------------------------------------------------------- Update

// TestDashboardUpdate_UnprovidedFieldsStayNil 钉住「未提供则保留」在 wire 上的形态：
// JSON 里没出现的键必须解成 nil 指针，service 才知道该保留存量。
func TestDashboardUpdate_UnprovidedFieldsStayNil(t *testing.T) {
	var gotID string
	var got entity.DashboardUpdateRequest
	h := NewDashboardHandler(&mockDashboardService{
		updateFunc: func(_ context.Context, id string, in entity.DashboardUpdateRequest) (*entity.Dashboard, error) {
			gotID, got = id, in
			return &entity.Dashboard{ID: id, Name: *in.Name, LayoutJSON: dashLayout, Status: "draft"}, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodPut, "/api/dashboards/"+dashID, `{"name":"改名后的盘"}`)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"id":"`+dashID+`","name":"改名后的盘","description":null,"layout_json":"`+dashLayoutJSON+`","status":"draft","created_at":"","updated_at":""}}`)

	if gotID != dashID {
		t.Errorf("path id 未透传: %q", gotID)
	}
	if got.Name == nil || *got.Name != "改名后的盘" {
		t.Fatalf("name 未透传: %+v", got.Name)
	}
	if got.Description != nil || got.LayoutJSON != nil || got.Status != nil {
		t.Errorf("未提供的字段必须是 nil（保留存量），实际 %+v", got)
	}
}

func TestDashboardUpdate_InvalidID(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		updateFunc: func(_ context.Context, _ string, _ entity.DashboardUpdateRequest) (*entity.Dashboard, error) {
			t.Fatal("Update must not run for a non-UUID id")
			return nil, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodPut, "/api/dashboards/abc", `{}`)
	assertBody(t, w, badRequestInvalidID)
}

func TestDashboardUpdate_EmptyBody(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		updateFunc: func(_ context.Context, _ string, _ entity.DashboardUpdateRequest) (*entity.Dashboard, error) {
			t.Fatal("Update must not be called for an empty body")
			return nil, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodPut, "/api/dashboards/"+dashID, "")
	assertBody(t, w, badRequestEOF)
}

func TestDashboardUpdate_ServiceError(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		updateFunc: func(_ context.Context, _ string, _ entity.DashboardUpdateRequest) (*entity.Dashboard, error) {
			return nil, router.NewBusinessError(20300, "dashboard not found")
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodPut, "/api/dashboards/"+dashID, `{"name":"x"}`)
	assertBody(t, w, dashNotFound)
}

// ---------------------------------------------------------------- Delete

func TestDashboardDelete_Success(t *testing.T) {
	var gotID string
	h := NewDashboardHandler(&mockDashboardService{
		deleteFunc: func(_ context.Context, id string) error {
			gotID = id
			return nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodDelete, "/api/dashboards/"+dashID, "")
	assertBody(t, w, okEnvelopeStatus)
	if gotID != dashID {
		t.Errorf("expected Delete called with id=%q, got %q", dashID, gotID)
	}
}

func TestDashboardDelete_InvalidID(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		deleteFunc: func(_ context.Context, _ string) error {
			t.Fatal("Delete must not run for a non-UUID id")
			return nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodDelete, "/api/dashboards/not-a-uuid", "")
	assertBody(t, w, badRequestInvalidID)
}

func TestDashboardDelete_ServiceError(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		deleteFunc: func(_ context.Context, _ string) error { return errBoom() },
	})
	w := serve(newDashboardTestRouter(h), http.MethodDelete, "/api/dashboards/"+dashID, "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Chart references

func TestDashboardListChartReferences_Success(t *testing.T) {
	var gotChartID int
	h := NewDashboardHandler(&mockDashboardService{
		refsFunc: func(_ context.Context, chartID int) ([]entity.Dashboard, error) {
			gotChartID = chartID
			return []entity.Dashboard{
				{ID: dashID, Name: "月度经营总览", LayoutJSON: dashLayout, Status: "draft"},
				{ID: "0198f2c3-4d5e-7a6b-8c9d-0e1f2a3b4c5e", Name: "运营日报", LayoutJSON: dashLayout, Status: "draft"},
			}, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodGet, "/api/charts/42/references", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"count":2,"dashboards":[`+
		`{"id":"`+dashID+`","name":"月度经营总览"},`+
		`{"id":"0198f2c3-4d5e-7a6b-8c9d-0e1f2a3b4c5e","name":"运营日报"}]}}`)
	if gotChartID != 42 {
		t.Errorf("expected chartID=42, got %d", gotChartID)
	}
}

// TestDashboardListChartReferences_Empty 无引用时 data 是空对象仍是数组形态
// （count=0 + dashboards:[]），前端不需要处理 null。
func TestDashboardListChartReferences_Empty(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		refsFunc: func(_ context.Context, _ int) ([]entity.Dashboard, error) { return nil, nil },
	})
	w := serve(newDashboardTestRouter(h), http.MethodGet, "/api/charts/42/references", "")
	assertBody(t, w, refEmptyEnvelope)
}

func TestDashboardListChartReferences_InvalidID(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		refsFunc: func(_ context.Context, _ int) ([]entity.Dashboard, error) {
			t.Fatal("CountChartReferences must not run for a non-numeric chart id")
			return nil, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodGet, "/api/charts/abc/references", "")
	assertBody(t, w, badRequestInvalidID)
}

func TestDashboardListChartReferences_ServiceError(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		refsFunc: func(_ context.Context, _ int) ([]entity.Dashboard, error) { return nil, errBoom() },
	})
	w := serve(newDashboardTestRouter(h), http.MethodGet, "/api/charts/42/references", "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Query

// TestDashboardQuery_ForwardsFiltersAndId 钉住 handler 在新契约下的职责边界：只把
// path id 与请求体（镜像 struct → entity）转交 service，取数与筛选合并全在 service。
func TestDashboardQuery_ForwardsFiltersAndId(t *testing.T) {
	var gotID string
	var got entity.DashboardQueryRequest
	h := NewDashboardHandler(&mockDashboardService{
		queryFunc: func(_ context.Context, id string, in entity.DashboardQueryRequest) (*entity.DashboardQueryResult, error) {
			gotID, got = id, in
			return &entity.DashboardQueryResult{Results: []entity.DashboardQueryBlock{{
				WidgetID:      "w-1",
				ChartID:       42,
				Status:        entity.DashboardBlockOK,
				Data:          &entity.ChartDataResult{Data: []map[string]any{{"region": "华东"}}},
				AppliedFields: []string{"region"},
			}}}, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodPost, "/api/dashboards/"+dashID+"/query",
		`{"filters":[{"widgetId":"w-1","value":["华东"]}]}`)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"results":[`+
		`{"widgetId":"w-1","chartId":42,"status":"ok","data":{"data":[{"region":"华东"}]},"appliedFields":["region"]}]}}`)

	if gotID != dashID {
		t.Errorf("path id 未透传: %q", gotID)
	}
	if len(got.Filters) != 1 || got.Filters[0].WidgetID != "w-1" {
		t.Fatalf("filters 未透传: %+v", got.Filters)
	}
	if len(got.Filters[0].Value) != 1 || got.Filters[0].Value[0] != "华东" {
		t.Errorf("筛选器取值未透传: %+v", got.Filters[0].Value)
	}
}

// TestDashboardQuery_StatusOnlyBody 无激活筛选器时请求体是 {"filters":[]} 或 {}：
// 两者都必须解成空筛选器集而不是 bind 错误（空数组 = 全未激活，PRD §8.3 步骤 2）。
func TestDashboardQuery_StatusOnlyBody(t *testing.T) {
	for _, body := range []string{`{}`, `{"filters":[]}`} {
		var got entity.DashboardQueryRequest
		h := NewDashboardHandler(&mockDashboardService{
			queryFunc: func(_ context.Context, _ string, in entity.DashboardQueryRequest) (*entity.DashboardQueryResult, error) {
				got = in
				return &entity.DashboardQueryResult{Results: []entity.DashboardQueryBlock{}}, nil
			},
		})
		w := serve(newDashboardTestRouter(h), http.MethodPost, "/api/dashboards/"+dashID+"/query", body)
		assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"results":[]}}`)
		if len(got.Filters) != 0 {
			t.Errorf("body %q: 期望空筛选器集，实际 %+v", body, got.Filters)
		}
	}
}

// TestDashboardQuery_QueryMustNotPolluteBody 与 Create 同款：泛型路由对 POST 会先跑
// ShouldBindQuery，In 镜像上的 form:"-" 是唯一挡住 query 参数混入 body 的东西。
func TestDashboardQuery_QueryMustNotPolluteBody(t *testing.T) {
	var got entity.DashboardQueryRequest
	h := NewDashboardHandler(&mockDashboardService{
		queryFunc: func(_ context.Context, _ string, in entity.DashboardQueryRequest) (*entity.DashboardQueryResult, error) {
			got = in
			return &entity.DashboardQueryResult{Results: []entity.DashboardQueryBlock{}}, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodPost,
		"/api/dashboards/"+dashID+"/query?Filters=evil",
		`{"filters":[{"widgetId":"w-1","value":["华东"]}]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	if len(got.Filters) != 1 || got.Filters[0].WidgetID != "w-1" {
		t.Fatalf("query 参数污染了 body struct: %+v", got.Filters)
	}
}

func TestDashboardQuery_InvalidID(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		queryFunc: func(_ context.Context, _ string, _ entity.DashboardQueryRequest) (*entity.DashboardQueryResult, error) {
			t.Fatal("Query must not run for a non-UUID id")
			return nil, nil
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodPost, "/api/dashboards/not-a-uuid/query", `{}`)
	assertBody(t, w, badRequestInvalidID)
}

// TestDashboardQuery_NotWired 未接线（service 返回业务错误）时必须是结构化错误，
// 不能是 panic 后的 500。
func TestDashboardQuery_NotWired(t *testing.T) {
	h := NewDashboardHandler(&mockDashboardService{
		queryFunc: func(_ context.Context, _ string, _ entity.DashboardQueryRequest) (*entity.DashboardQueryResult, error) {
			return nil, router.NewBusinessError(50000, "图表取数服务未接线")
		},
	})
	w := serve(newDashboardTestRouter(h), http.MethodPost, "/api/dashboards/"+dashID+"/query", `{}`)
	assertBody(t, w, `{"code":50000,"msg":"图表取数服务未接线","trace":"","data":{}}`)
}
