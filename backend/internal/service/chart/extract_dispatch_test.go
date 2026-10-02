package chart

import (
	"context"
	"testing"

	"data-insights/internal/domain/entity"
	"data-insights/internal/model"
)

// TestQueryDispatchesExtractMode 钉死抽取数据集的查询路径（#138 落地灌数后）：
// mode=extract 不再被拦截——上传建的数据集自身已指向抽取存储数据源（DatasourceID）
// 与抽取表（TableName=di_extract_<id>），沿普通直连取数路径即可；direct/空串历史行同理。
func TestQueryDispatchesExtractMode(t *testing.T) {
	req := &entity.ChartQueryRequest{DatasetID: 3}

	for _, mode := range []string{"extract", "direct", ""} {
		probed := false
		s := &chartService{}
		s.getDatasetModelFn = func(context.Context, int) (*model.Dataset, error) {
			return &model.Dataset{ID: 3, Mode: mode, DatasourceID: 1}, nil
		}
		s.getDatasourceModelFn = func(context.Context, int) (*model.Datasource, error) {
			probed = true
			return nil, context.Canceled
		}
		if _, err := s.Query(context.Background(), req); err == nil {
			t.Fatalf("mode %q: expected pass-through to the datasource step", mode)
		}
		if !probed {
			t.Fatalf("mode %q must not be blocked by a dispatch hook", mode)
		}
	}
}
