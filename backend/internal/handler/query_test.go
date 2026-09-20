package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"data-insights/internal/domain/entity"
	"data-insights/internal/router"
	"data-insights/internal/service/queryrecord"
)

// mockQueryRecordService implements queryrecord.Service for handler tests. The
// embedded interface is nil on purpose (same style as mockChartService): calling
// a non-overridden method fails loudly instead of silently returning zero values.
type mockQueryRecordService struct {
	queryrecord.Service

	saveFunc func(ctx context.Context, in entity.QueryRecordSaveRequest, ip string) (*entity.QueryRecordSaved, error)
	getFunc  func(ctx context.Context, shortID string) (*entity.QueryRecord, error)
}

func (m *mockQueryRecordService) Save(
	ctx context.Context, in entity.QueryRecordSaveRequest, ip string,
) (*entity.QueryRecordSaved, error) {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, in, ip)
	}
	return nil, nil
}

func (m *mockQueryRecordService) GetByShortID(
	ctx context.Context, shortID string,
) (*entity.QueryRecord, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, shortID)
	}
	return nil, nil
}

// newQueryTestRouter mirrors cmd/routes.go: POST /api/queries (body) and
// GET /api/queries/:q (path param only).
func newQueryTestRouter(h *QueryHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	queries := r.Group("/api/queries")
	router.RegisterPostRoute(queries, "", h.Save)
	router.RegisterGetRoute(queries, "/:q", h.Get)
	return r
}

const (
	queryShortID      = "CSatF9qXyRoQXFtAA3iZS"
	querySavedJSON    = `{"query_id":"CSatF9qXyRoQXFtAA3iZS","created_at":"2026-09-19T12:00:00Z","expires_at":"2026-12-18T12:00:00Z"}`
	querySavedMailbox = `{"code":20000,"msg":"success","trace":"","data":` + querySavedJSON + `}`
)

// TestQuerySave_ForwardsRequest 钉住 handler → service 的字段映射（含客户端 IP 的
// 捕获点：只有 HTTP 层知道它，service 拿不到）。
func TestQuerySave_ForwardsRequest(t *testing.T) {
	var got entity.QueryRecordSaveRequest
	var gotIP string
	h := NewQueryHandler(&mockQueryRecordService{
		saveFunc: func(_ context.Context, in entity.QueryRecordSaveRequest, ip string) (*entity.QueryRecordSaved, error) {
			got, gotIP = in, ip
			expires := "2026-12-18T12:00:00Z"
			return &entity.QueryRecordSaved{
				QueryID:   queryShortID,
				CreatedAt: "2026-09-19T12:00:00Z",
				ExpiresAt: &expires,
			}, nil
		},
	})

	body := `{"dataset_id":3,"chart_id":14,"spec":{"v":1,"document":{"chartType":"bar"}},` +
		`"source_type":"build","row_count":120,"duration_ms":340}`
	w := serve(newQueryTestRouter(h), http.MethodPost, "/api/queries", body)
	assertBody(t, w, querySavedMailbox)

	if got.DatasetID != 3 || got.ChartID != 14 {
		t.Errorf("dataset_id/chart_id 未透传: %+v", got)
	}
	if got.SourceType != "build" {
		t.Errorf("source_type = %q", got.SourceType)
	}
	if got.RowCount == nil || *got.RowCount != 120 || got.DurationMs == nil || *got.DurationMs != 340 {
		t.Errorf("结果元信息未透传: %+v", got)
	}
	var spec entity.QuerySpecEnvelope
	if err := json.Unmarshal(got.Spec, &spec); err != nil {
		t.Fatalf("spec 不是可解析的信封: %v", err)
	}
	if spec.V != 1 || string(spec.Document) != `{"chartType":"bar"}` {
		t.Errorf("spec 未原样透传: %s", string(got.Spec))
	}
	// httptest.NewRequest 的默认 RemoteAddr，gin 在此场景下按远端地址取 IP
	if gotIP != "192.0.2.1" {
		t.Errorf("client ip = %q, want 192.0.2.1", gotIP)
	}
}

// TestQuerySave_UnreportedMetricsStayNil 未上报与上报 0 必须可区分：
// nil 走 COALESCE 保留存量，0 才是真的零。
func TestQuerySave_UnreportedMetricsStayNil(t *testing.T) {
	var got entity.QueryRecordSaveRequest
	h := NewQueryHandler(&mockQueryRecordService{
		saveFunc: func(_ context.Context, in entity.QueryRecordSaveRequest, _ string) (*entity.QueryRecordSaved, error) {
			got = in
			return &entity.QueryRecordSaved{QueryID: queryShortID, CreatedAt: "2026-09-19T12:00:00Z"}, nil
		},
	})
	serve(newQueryTestRouter(h), http.MethodPost, "/api/queries",
		`{"dataset_id":3,"spec":{"v":1,"document":{}}}`)

	if got.RowCount != nil || got.DurationMs != nil {
		t.Errorf("未上报的元信息应为 nil，实际 %+v", got)
	}
}

// TestQuerySave_QueryParamsCannotPolluteBody 泛型路由对 POST 会先跑一遍
// ShouldBindQuery；In 镜像上的 form:"-" 是唯一挡住 query 参数混入 body 的东西。
// 这条测试就是那个保护的回归哨兵。
func TestQuerySave_QueryParamsCannotPolluteBody(t *testing.T) {
	var got entity.QueryRecordSaveRequest
	h := NewQueryHandler(&mockQueryRecordService{
		saveFunc: func(_ context.Context, in entity.QueryRecordSaveRequest, _ string) (*entity.QueryRecordSaved, error) {
			got = in
			return &entity.QueryRecordSaved{QueryID: queryShortID, CreatedAt: "2026-09-19T12:00:00Z"}, nil
		},
	})
	serve(newQueryTestRouter(h), http.MethodPost,
		"/api/queries?dataset_id=999&source_type=hacked&row_count=7",
		`{"dataset_id":3,"spec":{"v":1,"document":{}}}`)

	if got.DatasetID != 3 {
		t.Errorf("query 参数污染了 body: dataset_id = %d, want 3", got.DatasetID)
	}
	if got.SourceType != "" {
		t.Errorf("query 参数污染了 body: source_type = %q, want 空", got.SourceType)
	}
	if got.RowCount != nil {
		t.Errorf("query 参数污染了 body: row_count = %v, want nil", *got.RowCount)
	}
}

func TestQuerySave_EmptyBody(t *testing.T) {
	h := NewQueryHandler(&mockQueryRecordService{})
	w := serve(newQueryTestRouter(h), http.MethodPost, "/api/queries", "")
	assertBody(t, w, badRequestEOF)
}

func TestQuerySave_ErrorMapping(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{"业务错误", router.NewBusinessError(20100, "spec 不能为空"), `{"code":20100,"msg":"spec 不能为空","trace":"","data":{}}`},
		{"内部错误", errBoom(), internalErrorBoom},
	}
	for _, c := range cases {
		h := NewQueryHandler(&mockQueryRecordService{
			saveFunc: func(_ context.Context, _ entity.QueryRecordSaveRequest, _ string) (*entity.QueryRecordSaved, error) {
				return nil, c.err
			},
		})
		w := serve(newQueryTestRouter(h), http.MethodPost, "/api/queries", `{"dataset_id":1,"spec":{"v":1,"document":{}}}`)
		assertBody(t, w, c.wantMsg)
	}
}

// TestQueryGet_ForwardsShortID 短码经 path 参数送达 service，响应为记录全量。
func TestQueryGet_ForwardsShortID(t *testing.T) {
	var gotShortID string
	h := NewQueryHandler(&mockQueryRecordService{
		getFunc: func(_ context.Context, shortID string) (*entity.QueryRecord, error) {
			gotShortID = shortID
			chartID := 14
			return &entity.QueryRecord{
				QueryID:   shortID,
				Spec:      entity.QuerySpecEnvelope{V: 1, Document: json.RawMessage(`{"chartType":"bar"}`)},
				DatasetID: 3,
				ChartID:   &chartID,
				CreatedAt: "2026-01-02T03:04:05Z",
				Expired:   true,
				HitCount:  5,
			}, nil
		},
	})
	w := serve(newQueryTestRouter(h), http.MethodGet, "/api/queries/"+queryShortID, "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{`+
		`"query_id":"`+queryShortID+`",`+
		`"spec":{"v":1,"document":{"chartType":"bar"}},`+
		`"dataset_id":3,"chart_id":14,"created_at":"2026-01-02T03:04:05Z",`+
		`"expires_at":null,"expired":true,"hit_count":5}}`)
	if gotShortID != queryShortID {
		t.Errorf("短码未透传: %q", gotShortID)
	}
}

func TestQueryGet_ErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"非法短码", router.NewBusinessError(20100, "分享链接无效：id 格式不正确"),
			`{"code":20100,"msg":"分享链接无效：id 格式不正确","trace":"","data":{}}`},
		{"记录不存在", router.NewBusinessError(20300, "查询记录不存在或链接已失效"),
			`{"code":20300,"msg":"查询记录不存在或链接已失效","trace":"","data":{}}`},
		{"内部错误", errBoom(), internalErrorBoom},
	}
	for _, c := range cases {
		h := NewQueryHandler(&mockQueryRecordService{
			getFunc: func(_ context.Context, _ string) (*entity.QueryRecord, error) { return nil, c.err },
		})
		w := serve(newQueryTestRouter(h), http.MethodGet, "/api/queries/"+queryShortID, "")
		assertBody(t, w, c.want)
	}
}
