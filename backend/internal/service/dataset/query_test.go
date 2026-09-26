package dataset

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"data-insights/internal/datasource"
	"data-insights/internal/domain/entity"
	"data-insights/internal/model"
)

// 本文件覆盖 POST /api/datasets/:id/query（datasetService.Query）的 SQL 构造契约。
//
// 回归背景（issue #110）：该端点曾完全忽略请求里的 dimension_groups / filters / sort，
// 无论调用方要什么，执行的都是 `SELECT * FROM <source> LIMIT n`——把整行（全部列）×
// n 行回传，再由前端本地去重。唯一调用方（图表过滤器的枚举候选值）实际只需要 1 列。
// 修复后：维度组决定 SELECT / GROUP BY 的列，filters / sort / limit 照常生效，
// 枚举取值变成**服务端去重**（上限见 query.DatasetQueryLimitMax）。

// datasetColumnsFixture 是 bi_dataset.columns 的落库形态：列 ID 为权威引用，
// 列名可变，expr 为 SQL 表达式。
const datasetColumnsFixture = `[` +
	`{"id":"0000i529","name":"brand_name","expr":"brand_name","type":"string","role":"dimension"},` +
	`{"id":"0000i530","name":"retail_sales","expr":"retail_sales","type":"double","role":"metric"}` +
	`]`

// newQueryHarness 装配一个 dataset/datasource/连接全打桩的 service，并返回 stub
// 以便读回真正执行的 SQL 与参数。
func newQueryHarness(t *testing.T, columns string) (*datasetService, *stubConnection) {
	t.Helper()

	conn := &stubConnection{}
	s := &datasetService{}
	s.connectFn = func(context.Context, *model.Datasource) (datasource.Connection, error) {
		return conn, nil
	}
	s.getDatasetModelFn = func(context.Context, int) (*model.Dataset, error) {
		return &model.Dataset{
			ID:           1,
			DatasourceID: 2,
			TableName:    sql.NullString{String: "retail_sales", Valid: true},
			Columns:      columns,
		}, nil
	}
	s.getDatasourceModelFn = func(context.Context, int) (*model.Datasource, error) {
		return &model.Datasource{ID: 2, Type: "starrocks"}, nil
	}
	return s, conn
}

// TestDatasetQueryProjectsOnlyRequestedDimensions 先红：枚举候选值只声明了一个维度组
// 字段，执行的 SQL 必须只取该列并做服务端去重（GROUP BY），而不是 `SELECT *` 全列取样。
func TestDatasetQueryProjectsOnlyRequestedDimensions(t *testing.T) {
	s, conn := newQueryHarness(t, datasetColumnsFixture)

	if _, err := s.Query(context.Background(), 1, entity.QueryConfig{
		DimensionGroups: []entity.FieldGroup{{ID: "enum-candidates", Fields: []string{"brand_name"}}},
		Limit:           1000,
	}); err != nil {
		t.Fatalf("Query: %v", err)
	}

	want := "SELECT brand_name FROM retail_sales GROUP BY brand_name LIMIT 1000"
	if conn.executeSQL != want {
		t.Fatalf("executed SQL mismatch\n got: %s\nwant: %s", conn.executeSQL, want)
	}
}

// TestDatasetQueryResolvesColumnIDsAndParameterizesFilters 请求引用列 ID 时解析成
// 对应表达式，filters / sort / limit 一律生效，过滤值经 args 传递而不落进 SQL 文本。
func TestDatasetQueryResolvesColumnIDsAndParameterizesFilters(t *testing.T) {
	s, conn := newQueryHarness(t, datasetColumnsFixture)

	if _, err := s.Query(context.Background(), 1, entity.QueryConfig{
		DimensionGroups: []entity.FieldGroup{{ID: "g", Fields: []string{"0000i529"}}},
		Filters: []entity.Filter{
			{ID: "f1", Field: "0000i529", Operator: "neq", Value: "fixture-secret-value", Logic: "and"},
		},
		Sort:  &entity.SortConfig{Field: "brand_name", Order: "desc"},
		Limit: 50,
	}); err != nil {
		t.Fatalf("Query: %v", err)
	}

	// 列 ID（0000i529）解析成表达式 brand_name，输出别名换成展示名并加引号保留大小写；
	// 排序键命中该输出别名，故同样走引号形态。
	want := "SELECT brand_name AS `brand_name` FROM retail_sales WHERE brand_name <> ? GROUP BY brand_name ORDER BY `brand_name` DESC LIMIT 50"
	if conn.executeSQL != want {
		t.Fatalf("executed SQL mismatch\n got: %s\nwant: %s", conn.executeSQL, want)
	}
	if strings.Contains(conn.executeSQL, "fixture-secret-value") {
		t.Fatalf("filter value must be parameterized, leaked into SQL: %s", conn.executeSQL)
	}
	if len(conn.executeArgs) != 1 || conn.executeArgs[0] != "fixture-secret-value" {
		t.Fatalf("unexpected args: %#v", conn.executeArgs)
	}
}

// TestDatasetQueryCapsLimit 服务端给唯一值数量设硬上限：客户端要多少都不会超过
// query.DatasetQueryLimitMax（防止大基数列把响应撑爆）。
func TestDatasetQueryCapsLimit(t *testing.T) {
	s, conn := newQueryHarness(t, datasetColumnsFixture)

	if _, err := s.Query(context.Background(), 1, entity.QueryConfig{
		DimensionGroups: []entity.FieldGroup{{Fields: []string{"brand_name"}}},
		Limit:           1_000_000,
	}); err != nil {
		t.Fatalf("Query: %v", err)
	}

	if !strings.HasSuffix(conn.executeSQL, "LIMIT 1000") {
		t.Fatalf("limit must be capped, got: %s", conn.executeSQL)
	}
}

// TestDatasetQueryRequiresDimensionGroups 没有任何维度字段时显式报错，
// 而不是退回「取全表」——静默退回正是本次修复要消灭的行为。
func TestDatasetQueryRequiresDimensionGroups(t *testing.T) {
	s, conn := newQueryHarness(t, datasetColumnsFixture)

	if _, err := s.Query(context.Background(), 1, entity.QueryConfig{Limit: 10}); err == nil {
		t.Fatal("Query must fail without dimension fields")
	}
	if conn.executeSQL != "" {
		t.Fatalf("no SQL should be executed, got: %s", conn.executeSQL)
	}
}

// TestDatasetQueryRejectsMetricGroups entity.QueryConfig 的 FieldGroup 不携带聚合配置，
// 带指标组的请求无法被正确执行；必须显式报错而不是丢弃指标静默去重。
func TestDatasetQueryRejectsMetricGroups(t *testing.T) {
	s, conn := newQueryHarness(t, datasetColumnsFixture)

	if _, err := s.Query(context.Background(), 1, entity.QueryConfig{
		DimensionGroups: []entity.FieldGroup{{Fields: []string{"brand_name"}}},
		MetricGroups:    []entity.FieldGroup{{Fields: []string{"retail_sales"}}},
		Limit:           10,
	}); err == nil {
		t.Fatal("Query must fail when metric_groups is present")
	}
	if conn.executeSQL != "" {
		t.Fatalf("no SQL should be executed, got: %s", conn.executeSQL)
	}
}

// TestDatasetQueryWrapsSQLDatasetSource SQL 型数据集（query_sql）必须被包成子查询，
// 与 preview 的既有语义一致，不能让内层 LIMIT 把外层语法带坏。
func TestDatasetQueryWrapsSQLDatasetSource(t *testing.T) {
	conn := &stubConnection{}
	s := &datasetService{}
	s.connectFn = func(context.Context, *model.Datasource) (datasource.Connection, error) {
		return conn, nil
	}
	s.getDatasetModelFn = func(context.Context, int) (*model.Dataset, error) {
		return &model.Dataset{
			ID:           1,
			DatasourceID: 2,
			QueryType:    "sql",
			QuerySQL:     sql.NullString{String: "SELECT brand_name FROM raw_sales", Valid: true},
			Columns:      datasetColumnsFixture,
		}, nil
	}
	s.getDatasourceModelFn = func(context.Context, int) (*model.Datasource, error) {
		return &model.Datasource{ID: 2, Type: "starrocks"}, nil
	}

	if _, err := s.Query(context.Background(), 1, entity.QueryConfig{
		DimensionGroups: []entity.FieldGroup{{Fields: []string{"brand_name"}}},
		Limit:           10,
	}); err != nil {
		t.Fatalf("Query: %v", err)
	}

	want := "SELECT brand_name FROM (SELECT brand_name FROM raw_sales) AS _subq GROUP BY brand_name LIMIT 10"
	if conn.executeSQL != want {
		t.Fatalf("executed SQL mismatch\n got: %s\nwant: %s", conn.executeSQL, want)
	}
}
