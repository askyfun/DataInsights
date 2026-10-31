package chart

import (
	"context"
	"errors"
	"testing"

	"data-insights/internal/datasource"
	"data-insights/internal/domain/entity"
	"data-insights/internal/model"
	"data-insights/internal/response"
	"data-insights/internal/router"
)

// R-99（issue #224）稳定码映射用例：每个稳定码至少一条，断言「映射自哪种
// 内部错误」。信封侧的形状断言在 internal/response/errcode_test.go。

// stableChartService 起一个最小可用的 chartService，错误从注入点冒出。
func stableChartService(t *testing.T) *chartService {
	t.Helper()
	return NewService(nil).(*chartService)
}

// TestDialMapsDatasourceUnreachable 数据源连接失败 → DATASOURCE_UNREACHABLE。
// 用 MySQL 驱动：其 Connect 立即 Ping（127.0.0.1:1 真实 ECONNREFUSED，毫秒级、
// 无外部依赖）；PG 的 pgxpool.New 是懒连接，Connect 阶段不会拨号。
func TestDialMapsDatasourceUnreachable(t *testing.T) {
	service := stableChartService(t)
	_, err := service.dial(context.Background(), &model.Datasource{
		ID:   1,
		Type: "mysql",
		Host: "127.0.0.1",
		Port: 1,
	}, "")
	coded, ok := response.CodedFrom(err)
	if !ok {
		t.Fatalf("expected CodedError, got %v", err)
	}
	if coded.StableCode != response.StableDatasourceUnreachable ||
		coded.HintKey != response.HintDatasourceUnreachable {
		t.Fatalf("stable = %s/%s, want DATASOURCE_UNREACHABLE",
			coded.StableCode, coded.HintKey)
	}
}

// TestQueryMapsQueryExecFailed executor 执行失败（SQL 报错）→
// QUERY_EXEC_FAILED；业务 400 错误不被盖写（保持原分类）。
func TestQueryMapsQueryExecFailed(t *testing.T) {
	service := stableChartService(t)
	service.getDatasetModelFn = func(ctx context.Context, id int) (*model.Dataset, error) {
		return &model.Dataset{ID: id, DatasourceID: 1, QueryType: "table"}, nil
	}
	service.getDatasourceModelFn = func(ctx context.Context, id int) (*model.Datasource, error) {
		return &model.Datasource{ID: id, Type: "postgresql"}, nil
	}
	service.connectFn = func(ctx context.Context, ds *model.Datasource) (datasource.Connection, error) {
		return &stubConnection{}, nil
	}
	service.executorFactory = func(conn datasource.Connection, dataset *model.Dataset, ds *model.Datasource) queryExecutor {
		return &stubExecutor{err: errors.New(`syntax error at or near "FROM"`)}
	}

	_, err := service.Query(context.Background(), &entity.ChartQueryRequest{
		DatasetID: 1,
		ChartType: "table",
	})
	coded, ok := response.CodedFrom(err)
	if !ok {
		t.Fatalf("expected CodedError, got %v", err)
	}
	if coded.StableCode != response.StableQueryExecFailed ||
		coded.HintKey != response.HintQueryExecFailed {
		t.Fatalf("stable = %s/%s, want QUERY_EXEC_FAILED",
			coded.StableCode, coded.HintKey)
	}
}

// TestQueryKeepsBusinessErrorCodes 业务错误（400 族）穿过执行链时不得被
// QUERY_EXEC_FAILED 覆盖。
func TestQueryKeepsBusinessErrorCodes(t *testing.T) {
	service := stableChartService(t)
	service.getDatasetModelFn = func(ctx context.Context, id int) (*model.Dataset, error) {
		return &model.Dataset{ID: id, DatasourceID: 1, QueryType: "table"}, nil
	}
	service.getDatasourceModelFn = func(ctx context.Context, id int) (*model.Datasource, error) {
		return &model.Datasource{ID: id, Type: "postgresql"}, nil
	}
	service.connectFn = func(ctx context.Context, ds *model.Datasource) (datasource.Connection, error) {
		return &stubConnection{}, nil
	}
	bizErr := router.NewBusinessError(response.CodeBadRequest, "operator not supported")
	service.executorFactory = func(conn datasource.Connection, dataset *model.Dataset, ds *model.Datasource) queryExecutor {
		return &stubExecutor{err: bizErr}
	}

	_, err := service.Query(context.Background(), &entity.ChartQueryRequest{
		DatasetID: 1,
		ChartType: "table",
	})
	if !errors.Is(err, bizErr) {
		t.Fatalf("business error must pass through unwrapped, got %v", err)
	}
	if _, ok := response.CodedFrom(err); ok {
		t.Fatalf("business error must not carry an internal stable code, got %v", err)
	}
}

// TestDialMapsInvalidConfig 驱动类型不支持（配置性错误）→ INVALID_CONFIG。
func TestDialMapsInvalidConfig(t *testing.T) {
	service := stableChartService(t)
	_, err := service.dial(context.Background(), &model.Datasource{ID: 1, Type: "bogus"}, "")
	coded, ok := response.CodedFrom(err)
	if !ok {
		t.Fatalf("expected CodedError, got %v", err)
	}
	if coded.StableCode != response.StableInvalidConfig ||
		coded.HintKey != response.HintInvalidConfig {
		t.Fatalf("stable = %s/%s, want INVALID_CONFIG", coded.StableCode, coded.HintKey)
	}
}
