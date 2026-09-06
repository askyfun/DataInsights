package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"dataray/internal/domain/entity"

	"github.com/gin-gonic/gin"
)

// mockChartService implements chart.Service for testing
type mockChartService struct {
	lastUpdateChart *entity.Chart
}

func (m *mockChartService) List(_ context.Context, _, _ int) ([]entity.Chart, error) {
	return nil, nil
}
func (m *mockChartService) GetByID(_ context.Context, _ int) (*entity.Chart, error) {
	return nil, nil
}
func (m *mockChartService) Create(_ context.Context, _ *entity.Chart) (*entity.Chart, error) {
	return nil, nil
}
func (m *mockChartService) Update(_ context.Context, chart *entity.Chart) (*entity.Chart, error) {
	m.lastUpdateChart = chart
	return chart, nil
}
func (m *mockChartService) Delete(_ context.Context, _ int) error { return nil }
func (m *mockChartService) GetData(_ context.Context, _ int) (entity.ChartDataResult, error) {
	return entity.ChartDataResult{}, nil
}
func (m *mockChartService) Query(_ context.Context, _ *entity.ChartQueryRequest) (entity.ChartDataResult, error) {
	return entity.ChartDataResult{}, nil
}

func (m *mockChartService) SetSecurityKey(_ []byte) {}

func TestUpdate_BindsDatasetID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mock := &mockChartService{}
	h := NewChartHandler(mock)

	router := gin.New()
	router.PUT("/api/charts/:id", h.Update)

	body := `{
		"name": "test chart",
		"dataset_id": 5,
		"chart_type": "pie",
		"config": "{\"chartType\":\"pie\"}"
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/charts/3", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if mock.lastUpdateChart == nil {
		t.Fatal("Update was not called")
	}
	if mock.lastUpdateChart.DatasetID != 5 {
		t.Errorf("expected DatasetID=5, got %d", mock.lastUpdateChart.DatasetID)
	}
	if mock.lastUpdateChart.Name != "test chart" {
		t.Errorf("expected Name='test chart', got %q", mock.lastUpdateChart.Name)
	}
	if mock.lastUpdateChart.ChartType != "pie" {
		t.Errorf("expected ChartType='pie', got %q", mock.lastUpdateChart.ChartType)
	}
	if mock.lastUpdateChart.ID != 3 {
		t.Errorf("expected ID=3, got %d", mock.lastUpdateChart.ID)
	}
}

func TestUpdate_InvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mock := &mockChartService{}
	h := NewChartHandler(mock)

	router := gin.New()
	router.PUT("/api/charts/:id", h.Update)

	req := httptest.NewRequest(http.MethodPut, "/api/charts/abc", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (JSON error), got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(20100) {
		t.Errorf("expected BadRequest code 20100, got %v", resp["code"])
	}
}
