package query

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"data-insights/internal/model"
)

// --- ParseDialect 显式映射 ---

func TestParseDialectStarRocksMapsToMySQL(t *testing.T) {
	if got := ParseDialect("starrocks"); got != DialectMySQL {
		t.Fatalf("ParseDialect(starrocks) = %v, want %v", got, DialectMySQL)
	}
	if got := ParseDialect("StarRocks"); got != DialectMySQL {
		t.Fatalf("ParseDialect(StarRocks) = %v, want %v", got, DialectMySQL)
	}
}

// --- 时间粒度校验：MySQL/ClickHouse 下非 day 粒度显式报错，不再静默降级 ---

func TestValidateGranularity(t *testing.T) {
	cases := []struct {
		name        string
		dialect     DialectType
		granularity string
		wantErr     bool
	}{
		{"mysql day ok", DialectMySQL, "day", false},
		{"mysql week rejected", DialectMySQL, "week", true},
		{"mysql month rejected", DialectMySQL, "month", true},
		{"clickhouse day ok", DialectClickHouse, "day", false},
		{"clickhouse week rejected", DialectClickHouse, "week", true},
		{"postgresql day ok", DialectPostgreSQL, "day", false},
		{"postgresql week ok", DialectPostgreSQL, "week", false},
		{"postgresql month ok", DialectPostgreSQL, "month", false},
		{"no granularity ok", DialectMySQL, "", false},
		{"injection granularity rejected", DialectPostgreSQL, "day', (SELECT 1)) --", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ast := &QueryAST{
				DimensionExprs: []DimensionExprAST{
					{Field: "created_at", Alias: "created_at_" + tc.granularity, Granularity: tc.granularity},
				},
			}
			err := ast.ValidateGranularity(tc.dialect)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for granularity %q on %v", tc.granularity, tc.dialect)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// --- Sort.Order 校验：只允许 ASC/DESC（大小写不敏感，规范化后使用） ---

func TestBuildSelectQueryNormalizesSortOrder(t *testing.T) {
	qb := NewBunQueryBuilder()
	ast := qb.Build(
		"orders",
		SourceTypeTable,
		[]string{"status"},
		[]MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total_amount"}},
		nil,
		&SortConfig{Field: "total_amount", Order: "desc; DROP TABLE orders"},
		nil,
	)

	sql, _ := qb.BuildSelectQuery(ast)
	if strings.Contains(sql, ";") || strings.Contains(sql, "DROP") {
		t.Fatalf("sort order was interpolated without validation: %s", sql)
	}
	// 非法方向回退默认 ASC
	if !strings.HasSuffix(sql, "ORDER BY `total_amount` ASC") {
		t.Fatalf("unexpected sql: %s", sql)
	}
}

func TestBuildSelectQueryNormalizesLowercaseSortOrder(t *testing.T) {
	qb := NewBunQueryBuilder()
	ast := qb.Build(
		"orders",
		SourceTypeTable,
		nil,
		nil,
		nil,
		&SortConfig{Field: "status", Order: "desc"},
		nil,
	)

	sql, _ := qb.BuildSelectQuery(ast)
	if !strings.HasSuffix(sql, "ORDER BY status DESC") {
		t.Fatalf("unexpected sql: %s", sql)
	}
}

// --- Executor 集成：MySQL 数据源 + week 粒度必须报错，而不是静默产出错误结果 ---

func TestExecutorRejectsUnsupportedGranularity(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
	}
	ds := &model.Datasource{ID: 1, Type: "mysql"}

	conn := &MockConnection{
		rows: []map[string]any{{"day": "2024-01-01", "total": 1.0}},
	}
	executor := NewExecutor(conn, dataset, ds)

	spec := &QuerySpec{
		Dimensions: []DimensionExpr{
			{Field: "created_at", Granularity: "week"},
		},
		Metrics: []MetricExpr2{{Field: "amount", Agg: AggSum}},
	}
	plannedAST := NewQueryPlanner().PlanAST("orders", SourceTypeTable, spec)

	req := &ChartQueryRequest{
		DatasetID:  1,
		ChartType:  ChartTypeBar,
		Dims:       []string{"created_at_week"},
		Metrics:    []MetricConfig{{Field: "amount", Agg: AggSum}},
		PlannedAST: plannedAST,
	}

	if _, err := executor.Execute(context.Background(), req); err == nil {
		t.Fatal("expected error for week granularity on mysql dialect")
	}
}
