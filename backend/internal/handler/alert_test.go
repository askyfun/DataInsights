package handler

import (
	"context"
	"net/http"
	"testing"

	"data-insights/internal/domain/entity"
	"data-insights/internal/router"
	"data-insights/internal/service/alert"

	"github.com/gin-gonic/gin"
)

// mockAlertService implements alert.Service for handler tests. The embedded
// interface is nil on purpose (same style as mockDashboardFolderService):
// calling a non-overridden method fails loudly.
type mockAlertService struct {
	alert.Service

	listFunc   func(ctx context.Context) ([]entity.AlertRule, error)
	getFunc    func(ctx context.Context, id string) (*entity.AlertRule, error)
	createFunc func(ctx context.Context, in entity.AlertRuleCreateRequest) (*entity.AlertRule, error)
	updateFunc func(ctx context.Context, id string, in entity.AlertRuleUpdateRequest) (*entity.AlertRule, error)
	deleteFunc func(ctx context.Context, id string) error
	triggersFn func(ctx context.Context, id string) ([]entity.AlertTrigger, error)
}

func (m *mockAlertService) ListRules(ctx context.Context) ([]entity.AlertRule, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return nil, nil
}

func (m *mockAlertService) GetRule(ctx context.Context, id string) (*entity.AlertRule, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockAlertService) CreateRule(ctx context.Context, in entity.AlertRuleCreateRequest) (*entity.AlertRule, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, in)
	}
	return nil, nil
}

func (m *mockAlertService) UpdateRule(ctx context.Context, id string, in entity.AlertRuleUpdateRequest) (*entity.AlertRule, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, id, in)
	}
	return nil, nil
}

func (m *mockAlertService) DeleteRule(ctx context.Context, id string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func (m *mockAlertService) ListTriggers(ctx context.Context, id string) ([]entity.AlertTrigger, error) {
	if m.triggersFn != nil {
		return m.triggersFn(ctx, id)
	}
	return nil, nil
}

// newAlertTestRouter mirrors the alert wiring of cmd/routes.go.
func newAlertTestRouter(h *AlertHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	alerts := r.Group("/api/alerts")
	router.RegisterGetRoute(alerts, "", h.List)
	router.RegisterPostRoute(alerts, "", h.Create)
	router.RegisterGetRoute(alerts, "/:id", h.Get)
	router.RegisterPutRoute(alerts, "/:id", h.Update)
	router.RegisterDeleteRoute(alerts, "/:id", h.Delete)
	router.RegisterGetRoute(alerts, "/:id/triggers", h.ListTriggers)
	return r
}

// alert 的 id 是 uuid 字符串（非自增整数），测试用固定 uuid 形态。
const alertTestID = "0198f2c3-4d5e-7a6b-8c9d-0e1f2a3b4c5e"

func TestAlertList_EmptyIsNormalizedToArray(t *testing.T) {
	h := NewAlertHandler(&mockAlertService{
		listFunc: func(_ context.Context) ([]entity.AlertRule, error) { return nil, nil },
	})
	w := serve(newAlertTestRouter(h), http.MethodGet, "/api/alerts", "")
	assertBody(t, w, okEnvelopeEmptyArray)
}

func TestAlertCreate_PassesFieldsThrough(t *testing.T) {
	var got entity.AlertRuleCreateRequest
	h := NewAlertHandler(&mockAlertService{
		createFunc: func(_ context.Context, in entity.AlertRuleCreateRequest) (*entity.AlertRule, error) {
			got = in
			return &entity.AlertRule{ID: alertTestID, Name: in.Name, Operator: in.Operator}, nil
		},
	})
	w := serve(newAlertTestRouter(h), http.MethodPost, "/api/alerts",
		`{"name":"收入预警","chart_id":7,"metric":"revenue","operator":"gt","threshold":100}`)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"id":"`+alertTestID+`","name":"收入预警","chart_id":0,"metric":"","operator":"gt","threshold":0,"enabled":false,"created_at":"","updated_at":""}}`)
	if got.Name != "收入预警" || got.ChartID != 7 || got.Metric != "revenue" ||
		got.Operator != entity.AlertOperatorGT || got.Threshold != 100 {
		t.Errorf("service got %+v", got)
	}
}

func TestAlertUpdate_MergeSemanticsCarriedToService(t *testing.T) {
	var got entity.AlertRuleUpdateRequest
	h := NewAlertHandler(&mockAlertService{
		updateFunc: func(_ context.Context, id string, in entity.AlertRuleUpdateRequest) (*entity.AlertRule, error) {
			if id != alertTestID {
				t.Errorf("path id = %q", id)
			}
			got = in
			return &entity.AlertRule{ID: id}, nil
		},
	})
	// 只发 enabled=false：其余字段必须是 nil（未提供则保留）。
	w := serve(newAlertTestRouter(h), http.MethodPut, "/api/alerts/"+alertTestID, `{"enabled":false}`)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"id":"`+alertTestID+`","name":"","chart_id":0,"metric":"","operator":"","threshold":0,"enabled":false,"created_at":"","updated_at":""}}`)
	if got.Enabled == nil || *got.Enabled != false {
		t.Errorf("Enabled = %v, want false", got.Enabled)
	}
	if got.Name != nil || got.ChartID != nil || got.Metric != nil || got.Operator != nil || got.Threshold != nil {
		t.Errorf("unprovided fields must be nil, got %+v", got)
	}
}

func TestAlertTriggers_NormalizedToArray(t *testing.T) {
	h := NewAlertHandler(&mockAlertService{
		triggersFn: func(_ context.Context, id string) ([]entity.AlertTrigger, error) {
			if id != alertTestID {
				t.Errorf("path id = %q", id)
			}
			return nil, nil
		},
	})
	w := serve(newAlertTestRouter(h), http.MethodGet, "/api/alerts/"+alertTestID+"/triggers", "")
	assertBody(t, w, okEnvelopeEmptyArray)
}

func TestAlertDelete_StatusOk(t *testing.T) {
	var deleted string
	h := NewAlertHandler(&mockAlertService{
		deleteFunc: func(_ context.Context, id string) error { deleted = id; return nil },
	})
	w := serve(newAlertTestRouter(h), http.MethodDelete, "/api/alerts/"+alertTestID, "")
	assertBody(t, w, okEnvelopeStatus)
	if deleted != alertTestID {
		t.Errorf("deleted id = %q", deleted)
	}
}
