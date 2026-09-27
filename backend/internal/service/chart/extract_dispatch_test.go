package chart

import (
	"context"
	"testing"

	"data-insights/internal/domain/entity"
	"data-insights/internal/model"
	"data-insights/internal/response"
	"data-insights/internal/router"
)

// TestQueryDispatchesExtractMode 验证抽取数据集的查询分派位（issue #118 预留）：
// mode=extract 在建连之前就被显式拒绝（20100），绝不静默按直连执行；
// mode=direct（含空串的历史行）走原路径不受影响。
func TestQueryDispatchesExtractMode(t *testing.T) {
	newSvc := func(mode string) *chartService {
		s := &chartService{}
		s.getDatasetModelFn = func(context.Context, int) (*model.Dataset, error) {
			return &model.Dataset{ID: 3, Mode: mode, DatasourceID: 1}, nil
		}
		s.getDatasourceModelFn = func(context.Context, int) (*model.Datasource, error) {
			t.Fatal("extract-mode Query must not dial a datasource")
			return nil, nil
		}
		return s
	}

	req := &entity.ChartQueryRequest{DatasetID: 3}

	_, err := newSvc("extract").Query(context.Background(), req)
	bizErr, ok := err.(router.BusinessError)
	if !ok || bizErr.Code != response.CodeBadRequest {
		t.Fatalf("extract mode must surface as %d BusinessError, got %#v", response.CodeBadRequest, err)
	}

	// direct / 空串：分派位放行，继续走到取数据源（此处以 panic 探针验证"确实继续"）。
	probed := false
	for _, mode := range []string{"direct", ""} {
		s := newSvc(mode)
		s.getDatasourceModelFn = func(context.Context, int) (*model.Datasource, error) {
			probed = true
			return nil, context.Canceled
		}
		if _, err := s.Query(context.Background(), req); err == nil {
			t.Fatalf("mode %q: expected pass-through to datasource step", mode)
		}
		if !probed {
			t.Fatalf("mode %q must not be blocked by the dispatch hook", mode)
		}
	}
}
