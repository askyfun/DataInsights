package handler

import (
	"context"
	"net/http"
	"testing"

	"data-insights/internal/domain/entity"
	"data-insights/internal/response"
	"data-insights/internal/router"
	"data-insights/internal/service/dashboard"

	"github.com/gin-gonic/gin"
)

// mockDashboardFolderService implements dashboard.FolderService for handler
// tests. The embedded interface is nil on purpose (same style as
// mockDashboardService): calling a non-overridden method fails loudly instead
// of silently returning zero values.
type mockDashboardFolderService struct {
	dashboard.FolderService

	listFunc   func(ctx context.Context) ([]entity.DashboardFolder, error)
	getFunc    func(ctx context.Context, id string) (*entity.DashboardFolder, error)
	createFunc func(ctx context.Context, in entity.DashboardFolderCreateRequest) (*entity.DashboardFolder, error)
	updateFunc func(ctx context.Context, id string, in entity.DashboardFolderUpdateRequest) (*entity.DashboardFolder, error)
	deleteFunc func(ctx context.Context, id string) error
}

func (m *mockDashboardFolderService) ListFolders(ctx context.Context) ([]entity.DashboardFolder, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return nil, nil
}

func (m *mockDashboardFolderService) GetFolder(ctx context.Context, id string) (*entity.DashboardFolder, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockDashboardFolderService) CreateFolder(ctx context.Context, in entity.DashboardFolderCreateRequest) (*entity.DashboardFolder, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, in)
	}
	return nil, nil
}

func (m *mockDashboardFolderService) UpdateFolder(ctx context.Context, id string, in entity.DashboardFolderUpdateRequest) (*entity.DashboardFolder, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, id, in)
	}
	return nil, nil
}

func (m *mockDashboardFolderService) DeleteFolder(ctx context.Context, id string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

// newDashboardFolderTestRouter mirrors the folder wiring of cmd/routes.go.
func newDashboardFolderTestRouter(h *DashboardFolderHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	folders := r.Group("/api/dashboard-folders")
	router.RegisterGetRoute(folders, "", h.List)
	router.RegisterPostRoute(folders, "", h.Create)
	router.RegisterGetRoute(folders, "/:id", h.Get)
	router.RegisterPutRoute(folders, "/:id", h.Update)
	router.RegisterDeleteRoute(folders, "/:id", h.Delete)
	return r
}

const (
	folderID       = "0198f2c3-4d5e-7a6b-8c9d-0e1f2a3b4c5e"
	folderChildID  = "0198f2c3-4d5e-7a6b-8c9d-0e1f2a3b4c5f"
	folderRootJSON = `{"id":"` + folderID + `","name":"经营分析","parent_id":null,"created_at":"","updated_at":""}`
	folderKidJSON  = `{"id":"` + folderChildID + `","name":"周报","parent_id":"` + folderID + `","created_at":"","updated_at":""}`
	folderNotFound = `{"code":20300,"msg":"dashboard folder not found","trace":"","data":{}}`
	folderNotEmpty = `{"code":20400,"msg":"文件夹下还有 2 个仪表盘，请先移动或删除它们","trace":"","data":{}}`
)

// ---------------------------------------------------------------- List

func TestDashboardFolderList_EmptyIsNormalizedToArray(t *testing.T) {
	h := NewDashboardFolderHandler(&mockDashboardFolderService{
		listFunc: func(_ context.Context) ([]entity.DashboardFolder, error) { return nil, nil },
	})
	w := serve(newDashboardFolderTestRouter(h), http.MethodGet, "/api/dashboard-folders", "")
	assertBody(t, w, okEnvelopeEmptyArray)
}

func TestDashboardFolderList_FlatOrderPreserved(t *testing.T) {
	// 契约核心：后端回**扁平**数组（不组树），顺序即 service 给的顺序。
	h := NewDashboardFolderHandler(&mockDashboardFolderService{
		listFunc: func(_ context.Context) ([]entity.DashboardFolder, error) {
			return []entity.DashboardFolder{
				{ID: folderID, Name: "经营分析"},
				{ID: folderChildID, Name: "周报", ParentID: strPtr(folderID)},
			}, nil
		},
	})
	w := serve(newDashboardFolderTestRouter(h), http.MethodGet, "/api/dashboard-folders", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":[`+folderRootJSON+`,`+folderKidJSON+`]}`)
}

// ---------------------------------------------------------------- Get

func TestDashboardFolderGet_Found(t *testing.T) {
	h := NewDashboardFolderHandler(&mockDashboardFolderService{
		getFunc: func(_ context.Context, _ string) (*entity.DashboardFolder, error) {
			return &entity.DashboardFolder{ID: folderID, Name: "经营分析"}, nil
		},
	})
	w := serve(newDashboardFolderTestRouter(h), http.MethodGet, "/api/dashboard-folders/"+folderID, "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":`+folderRootJSON+`}`)
}

func TestDashboardFolderGet_InvalidID(t *testing.T) {
	h := NewDashboardFolderHandler(&mockDashboardFolderService{
		getFunc: func(_ context.Context, _ string) (*entity.DashboardFolder, error) {
			t.Fatal("Get must not run for a non-UUID id")
			return nil, nil
		},
	})
	w := serve(newDashboardFolderTestRouter(h), http.MethodGet, "/api/dashboard-folders/not-a-uuid", "")
	assertBody(t, w, badRequestInvalidID)
}

func TestDashboardFolderGet_NotFound(t *testing.T) {
	h := NewDashboardFolderHandler(&mockDashboardFolderService{
		getFunc: func(_ context.Context, _ string) (*entity.DashboardFolder, error) {
			return nil, router.NewBusinessError(response.CodeNotFound, "dashboard folder not found")
		},
	})
	w := serve(newDashboardFolderTestRouter(h), http.MethodGet, "/api/dashboard-folders/"+folderID, "")
	assertBody(t, w, folderNotFound)
}

// ---------------------------------------------------------------- Create

func TestDashboardFolderCreate_RootLevel(t *testing.T) {
	var got entity.DashboardFolderCreateRequest
	h := NewDashboardFolderHandler(&mockDashboardFolderService{
		createFunc: func(_ context.Context, in entity.DashboardFolderCreateRequest) (*entity.DashboardFolder, error) {
			got = in
			return &entity.DashboardFolder{ID: folderID, Name: in.Name}, nil
		},
	})
	w := serve(newDashboardFolderTestRouter(h), http.MethodPost, "/api/dashboard-folders", `{"name":"经营分析"}`)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":`+folderRootJSON+`}`)
	// 缺省的 parent_id 必须原样是空串（= 根级），handler 不做默认值决策。
	if got.Name != "经营分析" || got.ParentID != "" {
		t.Errorf("请求体未原样透传: %+v", got)
	}
}

func TestDashboardFolderCreate_WithParent(t *testing.T) {
	var got entity.DashboardFolderCreateRequest
	h := NewDashboardFolderHandler(&mockDashboardFolderService{
		createFunc: func(_ context.Context, in entity.DashboardFolderCreateRequest) (*entity.DashboardFolder, error) {
			got = in
			return &entity.DashboardFolder{ID: folderChildID, Name: in.Name, ParentID: strPtr(in.ParentID)}, nil
		},
	})
	w := serve(newDashboardFolderTestRouter(h), http.MethodPost, "/api/dashboard-folders",
		`{"name":"周报","parent_id":"`+folderID+`"}`)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":`+folderKidJSON+`}`)
	if got.ParentID != folderID {
		t.Errorf("parent_id 未透传: %q", got.ParentID)
	}
}

// TestDashboardFolderCreate_QueryMustNotPolluteBody 泛型路由对 POST 会先跑
// ShouldBindQuery；In 镜像上的 form:"-" 是挡住 query 混进 body 的唯一东西。
func TestDashboardFolderCreate_QueryMustNotPolluteBody(t *testing.T) {
	var got entity.DashboardFolderCreateRequest
	h := NewDashboardFolderHandler(&mockDashboardFolderService{
		createFunc: func(_ context.Context, in entity.DashboardFolderCreateRequest) (*entity.DashboardFolder, error) {
			got = in
			return &entity.DashboardFolder{ID: folderID, Name: in.Name}, nil
		},
	})
	w := serve(newDashboardFolderTestRouter(h), http.MethodPost,
		"/api/dashboard-folders?Name=evil&ParentID=0", `{"name":"ok"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	if got.Name != "ok" || got.ParentID != "" {
		t.Fatalf("query 参数污染了 body struct: %+v", got)
	}
}

// ---------------------------------------------------------------- Update

// TestDashboardFolderUpdate_ThreeStateParent 是本期契约最容易写错的一条：
// 缺省 / null / "" 三种形态分别对应「保留父级」「保留父级」「移到根级」，
// handler 必须把后两者区分开传给 service。
func TestDashboardFolderUpdate_ThreeStateParent(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantNil     bool
		wantParent  *string
		wantNameNil bool
	}{
		{name: "只改名：parent_id 必须是 nil", body: `{"name":"新名"}`, wantNil: true, wantNameNil: false},
		{name: "显式 null：等同保留", body: `{"parent_id":null}`, wantNil: true, wantNameNil: true},
		{name: "空串：移到根级", body: `{"parent_id":""}`, wantNil: false, wantParent: strPtr(""), wantNameNil: true},
		{name: "UUID：移到该夹下", body: `{"parent_id":"` + folderChildID + `"}`, wantNil: false, wantParent: strPtr(folderChildID), wantNameNil: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got entity.DashboardFolderUpdateRequest
			h := NewDashboardFolderHandler(&mockDashboardFolderService{
				updateFunc: func(_ context.Context, id string, in entity.DashboardFolderUpdateRequest) (*entity.DashboardFolder, error) {
					got = in
					return &entity.DashboardFolder{ID: id, Name: "新名"}, nil
				},
			})
			w := serve(newDashboardFolderTestRouter(h), http.MethodPut, "/api/dashboard-folders/"+folderID, tc.body)
			if w.Code != http.StatusOK {
				t.Fatalf("expected HTTP 200, got %d (body: %s)", w.Code, w.Body.String())
			}
			if tc.wantNil {
				if got.ParentID != nil {
					t.Errorf("未提供/null 的 parent_id 必须是 nil, 实际 %q", *got.ParentID)
				}
			} else {
				if got.ParentID == nil {
					t.Fatalf("parent_id 被丢成了 nil（service 会当成保留存量，移动静默失效）")
				}
				if *got.ParentID != *tc.wantParent {
					t.Errorf("parent_id 未原样透传: got %q want %q", *got.ParentID, *tc.wantParent)
				}
			}
			if tc.wantNameNil != (got.Name == nil) {
				t.Errorf("name 的提供形态错了: got %v", got.Name)
			}
		})
	}
}

func TestDashboardFolderUpdate_CycleRejected(t *testing.T) {
	// 环判定在 service；handler 只负责把 20400 原样抬成信封。
	h := NewDashboardFolderHandler(&mockDashboardFolderService{
		updateFunc: func(_ context.Context, _ string, _ entity.DashboardFolderUpdateRequest) (*entity.DashboardFolder, error) {
			return nil, router.NewBusinessError(response.CodeBusinessError, "文件夹不能移动到自身的子文件夹下")
		},
	})
	w := serve(newDashboardFolderTestRouter(h), http.MethodPut, "/api/dashboard-folders/"+folderID,
		`{"parent_id":"`+folderChildID+`"}`)
	assertBody(t, w, `{"code":20400,"msg":"文件夹不能移动到自身的子文件夹下","trace":"","data":{}}`)
}

func TestDashboardFolderUpdate_InvalidID(t *testing.T) {
	h := NewDashboardFolderHandler(&mockDashboardFolderService{
		updateFunc: func(_ context.Context, _ string, _ entity.DashboardFolderUpdateRequest) (*entity.DashboardFolder, error) {
			t.Fatal("Update must not run for a non-UUID id")
			return nil, nil
		},
	})
	w := serve(newDashboardFolderTestRouter(h), http.MethodPut, "/api/dashboard-folders/abc", `{}`)
	assertBody(t, w, badRequestInvalidID)
}

// ---------------------------------------------------------------- Delete

func TestDashboardFolderDelete_OK(t *testing.T) {
	var gotID string
	h := NewDashboardFolderHandler(&mockDashboardFolderService{
		deleteFunc: func(_ context.Context, id string) error { gotID = id; return nil },
	})
	w := serve(newDashboardFolderTestRouter(h), http.MethodDelete, "/api/dashboard-folders/"+folderID, "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"status":"ok"}}`)
	if gotID != folderID {
		t.Errorf("path id 未透传: %q", gotID)
	}
}

func TestDashboardFolderDelete_RefusesNonEmpty(t *testing.T) {
	h := NewDashboardFolderHandler(&mockDashboardFolderService{
		deleteFunc: func(_ context.Context, _ string) error {
			return router.NewBusinessError(response.CodeBusinessError,
				"文件夹下还有 2 个仪表盘，请先移动或删除它们")
		},
	})
	w := serve(newDashboardFolderTestRouter(h), http.MethodDelete, "/api/dashboard-folders/"+folderID, "")
	assertBody(t, w, folderNotEmpty)
}

func TestDashboardFolderDelete_InvalidID(t *testing.T) {
	h := NewDashboardFolderHandler(&mockDashboardFolderService{
		deleteFunc: func(_ context.Context, _ string) error {
			t.Fatal("Delete must not run for a non-UUID id")
			return nil
		},
	})
	w := serve(newDashboardFolderTestRouter(h), http.MethodDelete, "/api/dashboard-folders/abc", "")
	assertBody(t, w, badRequestInvalidID)
}
