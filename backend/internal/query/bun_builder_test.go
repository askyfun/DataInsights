package query

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBunQueryBuilder_BasicQuery(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"test_table",
		SourceTypeTable,
		[]string{"project_id"},
		[]MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total_amount"}},
		[]FilterConfig{},
		nil,
		nil,
	)

	sql, args := qb.BuildSelectQuery(ast)

	t.Logf("Generated SQL: %s", sql)
	t.Logf("Args: %v", args)

	if sql == "" {
		t.Error("Expected non-empty SQL")
	}

	expected := "SELECT project_id, SUM(amount) AS `total_amount` FROM test_table GROUP BY project_id"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}
}

func TestBunQueryBuilder_WithPagination(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"orders",
		SourceTypeTable,
		[]string{"status"},
		[]MetricConfig{{Field: "id", Agg: AggCount, Alias: "count"}},
		[]FilterConfig{},
		nil,
		&Pagination{Page: 2, PageSize: 20},
	)

	sql, args := qb.BuildSelectQuery(ast)

	t.Logf("Generated SQL: %s", sql)
	t.Logf("Args: %v", args)

	expected := "SELECT status, COUNT(id) AS `count` FROM orders GROUP BY status LIMIT 20 OFFSET 20"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}

	// LIMIT/OFFSET uses direct embedding for ClickHouse compatibility (not parameterized)
}

func TestBunQueryBuilder_WithFilters(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"users",
		SourceTypeTable,
		[]string{"city"},
		[]MetricConfig{{Field: "age", Agg: AggAvg, Alias: "avg_age"}},
		[]FilterConfig{
			{Field: "status", Op: FilterEq, Value: "active"},
			{Field: "age", Op: FilterGt, Value: 18},
		},
		nil,
		nil,
	)

	sql, args := qb.BuildSelectQuery(ast)

	t.Logf("Generated SQL: %s", sql)
	t.Logf("Args: %v", args)

	expected := "SELECT city, AVG(age) AS `avg_age` FROM users WHERE status = ? AND age > ? GROUP BY city"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}

	if len(args) != 2 {
		t.Errorf("Expected 2 args, got %d", len(args))
	}
}

func TestBunQueryBuilder_WithFilterLogicOr(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"users",
		SourceTypeTable,
		[]string{"city"},
		[]MetricConfig{{Field: "age", Agg: AggAvg, Alias: "avg_age"}},
		[]FilterConfig{
			{Field: "status", Op: FilterEq, Value: "active"},
			{Field: "age", Op: FilterGt, Value: 18, Logic: "or"},
		},
		nil,
		nil,
	)

	sql, args := qb.BuildSelectQuery(ast)

	t.Logf("Generated SQL: %s", sql)
	t.Logf("Args: %v", args)

	expected := "SELECT city, AVG(age) AS `avg_age` FROM users WHERE status = ? OR age > ? GROUP BY city"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}

	if len(args) != 2 {
		t.Errorf("Expected 2 args, got %d", len(args))
	}
}

func TestBunQueryBuilder_WithMixedFilterLogic(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"orders",
		SourceTypeTable,
		[]string{"status"},
		[]MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total_amount"}},
		[]FilterConfig{
			{Field: "country", Op: FilterEq, Value: "CN"},
			{Field: "category", Op: FilterEq, Value: "A", Logic: "and"},
			{Field: "amount", Op: FilterGt, Value: 100, Logic: "or"},
		},
		nil,
		nil,
	)

	sql, args := qb.BuildSelectQuery(ast)

	t.Logf("Generated SQL: %s", sql)
	t.Logf("Args: %v", args)

	expected := "SELECT status, SUM(amount) AS `total_amount` FROM orders WHERE country = ? AND category = ? OR amount > ? GROUP BY status"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}

	if len(args) != 3 {
		t.Errorf("Expected 3 args, got %d", len(args))
	}
}

func TestBunQueryBuilder_WithSort(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"sales",
		SourceTypeTable,
		[]string{"product"},
		[]MetricConfig{{Field: "revenue", Agg: AggSum, Alias: "total_revenue"}},
		[]FilterConfig{},
		&SortConfig{Field: "total_revenue", Order: "desc"},
		nil,
	)

	sql, _ := qb.BuildSelectQuery(ast)

	t.Logf("Generated SQL: %s", sql)

	expected := "SELECT product, SUM(revenue) AS `total_revenue` FROM sales GROUP BY product ORDER BY `total_revenue` DESC"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}
}

func TestBunQueryBuilder_SortWithPagination(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"orders",
		SourceTypeTable,
		[]string{"status"},
		[]MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total_amount"}},
		[]FilterConfig{},
		&SortConfig{Field: "total_amount", Order: "desc"},
		&Pagination{Page: 2, PageSize: 20},
	)

	sql, _ := qb.BuildSelectQuery(ast)

	t.Logf("Generated SQL: %s", sql)

	// 分页查询必须保留 ORDER BY，否则分页结果不确定
	expected := "SELECT status, SUM(amount) AS `total_amount` FROM orders GROUP BY status ORDER BY `total_amount` DESC LIMIT 20 OFFSET 20"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}
}

func TestBunQueryBuilder_PaginationWithoutSort(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"orders",
		SourceTypeTable,
		[]string{"status"},
		[]MetricConfig{{Field: "id", Agg: AggCount, Alias: "count"}},
		[]FilterConfig{},
		nil,
		&Pagination{Page: 1, PageSize: 10},
	)

	sql, _ := qb.BuildSelectQuery(ast)

	t.Logf("Generated SQL: %s", sql)

	// 无 sort 时不加 ORDER BY，结果顺序由数据库决定
	expected := "SELECT status, COUNT(id) AS `count` FROM orders GROUP BY status LIMIT 10 OFFSET 0"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}
}

func TestBunQueryBuilder_CountQuery(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"orders",
		SourceTypeTable,
		[]string{"status"},
		[]MetricConfig{{Field: "id", Agg: AggCount, Alias: "count"}},
		[]FilterConfig{},
		nil,
		nil,
	)

	sql, args := qb.BuildCountQuery(ast)

	t.Logf("Generated Count SQL: %s", sql)
	t.Logf("Args: %v", args)

	expected := "SELECT COUNT(*) AS _total FROM (SELECT 1 FROM orders GROUP BY status) AS _count_query"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}
}

func TestBunQueryBuilder_WithAggregatedColumnMapping(t *testing.T) {
	qb := NewQueryBuilder()
	if err := qb.WithColumnMappings(`[{"name":"cnt","expr":"count(*)","role":"metric","type":"bigint"}]`); err != nil {
		t.Fatalf("unexpected column mapping error: %v", err)
	}

	ast := qb.Build(
		"test_table",
		SourceTypeTable,
		[]string{"project_id"},
		[]MetricConfig{{Field: "cnt", Agg: AggSum, Alias: "cnt"}},
		[]FilterConfig{},
		nil,
		&Pagination{Page: 1, PageSize: 10},
	)

	sql, _ := NewBunSQLBuilder(DialectMySQL).BuildSelect(ast)

	expected := "SELECT project_id, count(*) AS `cnt` FROM test_table GROUP BY project_id LIMIT 10 OFFSET 0"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}
}

func TestBunQueryBuilder_WithSQLSource(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"SELECT * FROM orders WHERE created_at > '2024-01-01'",
		SourceTypeSQL,
		[]string{"status"},
		[]MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total"}},
		[]FilterConfig{},
		nil,
		nil,
	)

	sql, _ := qb.BuildSelectQuery(ast)

	t.Logf("Generated SQL: %s", sql)

	expected := "SELECT status, SUM(amount) AS `total` FROM (SELECT * FROM orders WHERE created_at > '2024-01-01') AS _subq GROUP BY status"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}
}

func TestBunSQLBuilder_BuildSelect(t *testing.T) {
	qb := NewBunQueryBuilder()
	qb.columnMappings = map[string]string{
		"cnt": "count(*)",
	}

	ast := qb.Build(
		"test_table",
		SourceTypeTable,
		[]string{"project_id"},
		[]MetricConfig{{Field: "cnt", Agg: AggSum, Alias: "cnt"}},
		[]FilterConfig{},
		nil,
		&Pagination{Page: 1, PageSize: 10},
	)

	builder := NewBunSQLBuilder(DialectPostgreSQL)
	sql, _ := builder.BuildSelect(ast)

	t.Logf("Generated SQL: %s", sql)

	expected := "SELECT project_id, count(*) AS \"cnt\" FROM test_table GROUP BY project_id LIMIT 10 OFFSET 0"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}
}

func TestSafeIdentifierFallsBackOnInvalid(t *testing.T) {
	if got := safeIdentifier("a;b"); got != "_invalid_identifier" {
		t.Fatalf("expected fallback, got %q", got)
	}
}

func TestSafeExprAllowsAggregateAndRejectsInjection(t *testing.T) {
	cases := map[string]string{
		"count(*)":               "count(*)",
		"SUM(amount)":            "SUM(amount)",
		"sum(bi_orders.amount)":  "sum(bi_orders.amount)",
		"a;b":                    "_invalid_identifier",
		"count(*); DROP TABLE x": "_invalid_identifier",
		"evil() OR 1=1":          "_invalid_identifier",
		"plain_col":              "plain_col",
	}
	for in, want := range cases {
		if got := safeExpr(in); got != want {
			t.Errorf("safeExpr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBunQueryBuilder_SafeIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal_field", "normal_field"},
		{"`project_name`", "`project_name`"},
		{"field_name", "field_name"},
		{"field; DROP TABLE", "_invalid_identifier"},
		{"field' OR '1'='1", "_invalid_identifier"},
		{"field--comment", "_invalid_identifier"},
		{"  spaces  ", "spaces"},
	}

	for _, tt := range tests {
		result := safeIdentifier(tt.input)
		if result != tt.expected {
			t.Errorf("safeIdentifier(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestBunSQLBuilder_BuildSelect_WithQuotedDatasetColumnExpr(t *testing.T) {
	ast := &QueryAST{
		Source:         "test_table",
		SourceType:     SourceTypeTable,
		Dimensions:     []string{"project_name"},
		DimensionExprs: []DimensionExprAST{{Field: "project_name", Alias: "project_name"}},
		Metrics:        []MetricExpr{{Field: "cnt", FieldExpr: "count(*)", Agg: AggSum, Alias: "cnt", IsAgg: true}},
		ColumnMappings: map[string]string{"project_name": "`project_name`", "cnt": "count(*)"},
	}

	ast.ApplyColumnMappings(ast.ColumnMappings)

	sql, _ := NewBunSQLBuilder(DialectMySQL).BuildSelect(ast)

	expected := "SELECT `project_name`, count(*) AS `cnt` FROM test_table GROUP BY `project_name`"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}
}

// TestBunQueryBuilder_MixedCaseMetricAliasIsDialectQuoted 验证指标别名在 SQL 结果名
// 位置按方言正确加引号：不加引号时 Postgres/MySQL 会把 "Revenue" 折叠成小写
// "revenue"，行键与处理器按别名 "Revenue" 的查找对不上，图表数据全为 NULL。
func TestBunQueryBuilder_MixedCaseMetricAliasIsDialectQuoted(t *testing.T) {
	tests := []struct {
		name     string
		dialect  DialectType
		expected string
	}{
		{
			name:     "postgresql double quotes",
			dialect:  DialectPostgreSQL,
			expected: `SELECT region, SUM(amount) AS "Revenue" FROM sales GROUP BY region`,
		},
		{
			name:     "mysql backticks",
			dialect:  DialectMySQL,
			expected: "SELECT region, SUM(amount) AS `Revenue` FROM sales GROUP BY region",
		},
		{
			name:     "clickhouse backticks",
			dialect:  DialectClickHouse,
			expected: "SELECT region, SUM(amount) AS `Revenue` FROM sales GROUP BY region",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewBunQueryBuilder()
			qb.SetDialect(tt.dialect)

			ast := qb.Build(
				"sales",
				SourceTypeTable,
				[]string{"region"},
				[]MetricConfig{{Field: "amount", Agg: AggSum, Alias: "Revenue"}},
				[]FilterConfig{},
				nil,
				nil,
			)

			sql, _ := qb.BuildSelectQuery(ast)
			if sql != tt.expected {
				t.Errorf("Expected:\n%s\nGot:\n%s", tt.expected, sql)
			}
		})
	}
}

// TestBunQueryBuilder_MixedCaseAliasQuotedForPreAggregatedMetric 验证列映射本身已是
// 聚合表达式（IsAgg 分支，直接 "expr AS alias"）时，别名位置同样按方言加引号。
func TestBunQueryBuilder_MixedCaseAliasQuotedForPreAggregatedMetric(t *testing.T) {
	qb := NewBunQueryBuilder()
	qb.SetDialect(DialectPostgreSQL)
	if err := qb.WithColumnMappings(`[{"name":"revenue","expr":"SUM(amount)","role":"metric","type":"numeric"}]`); err != nil {
		t.Fatalf("unexpected column mapping error: %v", err)
	}

	ast := qb.Build(
		"sales",
		SourceTypeTable,
		[]string{"region"},
		[]MetricConfig{{Field: "revenue", Agg: AggSum, Alias: "Revenue"}},
		[]FilterConfig{},
		nil,
		nil,
	)

	sql, _ := qb.BuildSelectQuery(ast)
	expected := `SELECT region, SUM(amount) AS "Revenue" FROM sales GROUP BY region`
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}
}

// TestBunQueryBuilder_OrderByAliasUsesSameQuoting 验证 ORDER BY 引用指标别名时使用
// 与 SELECT 一致的引号（折叠大小写会让 "Revenue" 在输出列里找不到而报错）；
// 同时验证按普通列排序时保持原样、不被加引号。
func TestBunQueryBuilder_OrderByAliasUsesSameQuoting(t *testing.T) {
	qb := NewBunQueryBuilder()
	qb.SetDialect(DialectPostgreSQL)

	ast := qb.Build(
		"sales",
		SourceTypeTable,
		[]string{"region"},
		[]MetricConfig{{Field: "amount", Agg: AggSum, Alias: "Revenue"}},
		[]FilterConfig{},
		&SortConfig{Field: "Revenue", Order: "desc"},
		nil,
	)

	sql, _ := qb.BuildSelectQuery(ast)
	expected := `SELECT region, SUM(amount) AS "Revenue" FROM sales GROUP BY region ORDER BY "Revenue" DESC`
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}

	// 普通维度列排序不加引号（行键与 DB 列名语义保持现状）
	ast2 := qb.Build(
		"sales",
		SourceTypeTable,
		[]string{"region"},
		[]MetricConfig{{Field: "amount", Agg: AggSum, Alias: "Revenue"}},
		[]FilterConfig{},
		&SortConfig{Field: "region", Order: "asc"},
		nil,
	)

	sql2, _ := qb.BuildSelectQuery(ast2)
	expected2 := `SELECT region, SUM(amount) AS "Revenue" FROM sales GROUP BY region ORDER BY region ASC`
	if sql2 != expected2 {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected2, sql2)
	}
}

func TestBunQueryBuilder_ColumnMappings(t *testing.T) {
	qb := NewBunQueryBuilder()

	columns := `[{"name":"revenue","expr":"SUM(amount)","role":"metric","type":"bigint"}]`
	if err := qb.WithColumnMappings(columns); err != nil {
		t.Fatalf("Failed to parse column mappings: %v", err)
	}

	ast := qb.Build(
		"sales",
		SourceTypeTable,
		[]string{"product"},
		[]MetricConfig{{Field: "revenue", Agg: AggSum}},
		[]FilterConfig{},
		nil,
		nil,
	)

	sql, _ := qb.BuildSelectQuery(ast)

	t.Logf("Generated SQL: %s", sql)

	expected := "SELECT product, SUM(amount) AS `revenue` FROM sales GROUP BY product"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}
}

func TestBuildQueryStringWithBun(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"test_table",
		SourceTypeTable,
		[]string{"category"},
		[]MetricConfig{{Field: "price", Agg: AggAvg, Alias: "avg_price"}},
		[]FilterConfig{},
		nil,
		nil,
	)

	selectSQL, countSQL, _ := BuildQueryStringWithBun(DialectPostgreSQL, ast)

	t.Logf("Select SQL: %s", selectSQL)
	t.Logf("Count SQL: %s", countSQL)

	expectedSelect := "SELECT category, AVG(price) AS \"avg_price\" FROM test_table GROUP BY category"
	if selectSQL != expectedSelect {
		t.Errorf("Expected select:\n%s\nGot:\n%s", expectedSelect, selectSQL)
	}

	expectedCount := "SELECT COUNT(*) AS _total FROM (SELECT 1 FROM test_table GROUP BY category) AS _count_query"
	if countSQL != expectedCount {
		t.Errorf("Expected count:\n%s\nGot:\n%s", expectedCount, countSQL)
	}
}

func TestBuildQueryStringWithBun_PostgreSQLGranularityAndLimit(t *testing.T) {
	ast := &QueryAST{
		Source:     "orders",
		SourceType: SourceTypeTable,
		Dimensions: []string{"created_at_day", "region"},
		DimensionExprs: []DimensionExprAST{
			{Field: "created_at", Granularity: "day", Alias: "created_at_day"},
			{Field: "region", Alias: "region"},
		},
		Metrics: []MetricExpr{{Field: "amount", FieldExpr: "amount", Agg: AggSum, Alias: "total_amount"}},
		MetricExprs: []MetricPlanExpr{{Field: "amount", Agg: AggSum, Alias: "total_amount"}},
		Limit:       10,
	}

	selectSQL, countSQL, _ := BuildQueryStringWithBun(DialectPostgreSQL, ast)

	expectedSelect := "SELECT DATE_TRUNC('day', created_at) AS \"created_at_day\", region, SUM(amount) AS \"total_amount\" FROM orders GROUP BY DATE_TRUNC('day', created_at), region LIMIT 10"
	if selectSQL != expectedSelect {
		t.Errorf("Expected select:\n%s\nGot:\n%s", expectedSelect, selectSQL)
	}

	expectedCount := "SELECT COUNT(*) AS _total FROM (SELECT 1 FROM orders GROUP BY DATE_TRUNC('day', created_at), region LIMIT 10) AS _count_query"
	if countSQL != expectedCount {
		t.Errorf("Expected count:\n%s\nGot:\n%s", expectedCount, countSQL)
	}
}

func TestBuildQueryStringWithBun_MySQLDayGranularity(t *testing.T) {
	ast := &QueryAST{
		Source:     "orders",
		SourceType: SourceTypeTable,
		Dimensions: []string{"created_at_day"},
		DimensionExprs: []DimensionExprAST{
			{Field: "created_at", Granularity: "day", Alias: "created_at_day"},
		},
		Metrics: []MetricExpr{{Field: "amount", FieldExpr: "amount", Agg: AggSum, Alias: "total_amount"}},
	}

	selectSQL, _, _ := BuildQueryStringWithBun(DialectMySQL, ast)

	expectedSelect := "SELECT DATE(created_at) AS `created_at_day`, SUM(amount) AS `total_amount` FROM orders GROUP BY DATE(created_at)"
	if selectSQL != expectedSelect {
		t.Errorf("Expected select:\n%s\nGot:\n%s", expectedSelect, selectSQL)
	}
}

func TestBuildQueryStringWithBun_ClickHouseDayGranularity(t *testing.T) {
	ast := &QueryAST{
		Source:     "orders",
		SourceType: SourceTypeTable,
		Dimensions: []string{"created_at_day"},
		DimensionExprs: []DimensionExprAST{
			{Field: "created_at", Granularity: "day", Alias: "created_at_day"},
		},
		Metrics: []MetricExpr{{Field: "amount", FieldExpr: "amount", Agg: AggSum, Alias: "total_amount"}},
	}

	selectSQL, _, _ := BuildQueryStringWithBun(DialectClickHouse, ast)

	expectedSelect := "SELECT toDate(created_at) AS `created_at_day`, SUM(amount) AS `total_amount` FROM orders GROUP BY toDate(created_at)"
	if selectSQL != expectedSelect {
		t.Errorf("Expected select:\n%s\nGot:\n%s", expectedSelect, selectSQL)
	}
}

func TestBunQueryBuilder_WithFilterIn(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"products",
		SourceTypeTable,
		[]string{"category"},
		[]MetricConfig{{Field: "id", Agg: AggCount, Alias: "count"}},
		[]FilterConfig{
			{Field: "status", Op: FilterIn, Value: []any{"active", "pending", "draft"}},
		},
		nil,
		nil,
	)

	sql, args := qb.BuildSelectQuery(ast)

	t.Logf("Generated SQL: %s", sql)
	t.Logf("Args: %v", args)

	expected := "SELECT category, COUNT(id) AS `count` FROM products WHERE status IN (?, ?, ?) GROUP BY category"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}

	if len(args) != 3 {
		t.Errorf("Expected 3 args, got %d", len(args))
	}
}

func TestBunQueryBuilder_WithFilterBetween(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"orders",
		SourceTypeTable,
		[]string{"status"},
		[]MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total"}},
		[]FilterConfig{
			{Field: "created_at", Op: FilterBetween, Value: "2024-01-01", ValueEnd: "2024-12-31"},
		},
		nil,
		nil,
	)

	sql, args := qb.BuildSelectQuery(ast)

	t.Logf("Generated SQL: %s", sql)
	t.Logf("Args: %v", args)

	expected := "SELECT status, SUM(amount) AS `total` FROM orders WHERE created_at BETWEEN ? AND ? GROUP BY status"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}

	if len(args) != 2 {
		t.Errorf("Expected 2 args, got %d", len(args))
	}
}

func TestBunQueryBuilder_WithFilterLike(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"users",
		SourceTypeTable,
		[]string{"city"},
		[]MetricConfig{{Field: "id", Agg: AggCount, Alias: "count"}},
		[]FilterConfig{
			{Field: "name", Op: FilterLike, Value: "John"},
		},
		nil,
		nil,
	)

	sql, args := qb.BuildSelectQuery(ast)

	t.Logf("Generated SQL: %s", sql)
	t.Logf("Args: %v", args)

	expected := "SELECT city, COUNT(id) AS `count` FROM users WHERE name LIKE ? GROUP BY city"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}

	if len(args) != 1 {
		t.Errorf("Expected 1 arg, got %d", len(args))
	}
}

func TestBunQueryBuilder_WithFilterNull(t *testing.T) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"orders",
		SourceTypeTable,
		[]string{"status"},
		[]MetricConfig{{Field: "id", Agg: AggCount, Alias: "count"}},
		[]FilterConfig{
			{Field: "deleted_at", Op: FilterIsNull},
		},
		nil,
		nil,
	)

	sql, args := qb.BuildSelectQuery(ast)

	t.Logf("Generated SQL: %s", sql)
	t.Logf("Args: %v", args)

	expected := "SELECT status, COUNT(id) AS `count` FROM orders WHERE deleted_at IS NULL GROUP BY status"
	if sql != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, sql)
	}

	if len(args) != 0 {
		t.Errorf("Expected 0 args, got %d", len(args))
	}
}

func BenchmarkBunQueryBuilder_BuildSelect(b *testing.B) {
	qb := NewBunQueryBuilder()

	ast := qb.Build(
		"orders",
		SourceTypeTable,
		[]string{"status", "category"},
		[]MetricConfig{
			{Field: "amount", Agg: AggSum, Alias: "total_amount"},
			{Field: "quantity", Agg: AggAvg, Alias: "avg_quantity"},
		},
		[]FilterConfig{
			{Field: "status", Op: FilterEq, Value: "active"},
			{Field: "amount", Op: FilterGt, Value: 100},
		},
		&SortConfig{Field: "total_amount", Order: "desc"},
		&Pagination{Page: 1, PageSize: 20},
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		qb.BuildSelectQuery(ast)
	}
}

func init() {
	_ = json.Marshal
}

func TestBuildQueryStringWithBunCollectsArgs(t *testing.T) {
	ast := &QueryAST{
		Source:     "orders",
		SourceType: SourceTypeTable,
		Dimensions: []string{"region"},
		Filters: []FilterExpr{
			{FieldExpr: "region", Op: FilterEq, Value: "'; DROP TABLE users; --"},
		},
	}
	selectSQL, _, args := BuildQueryStringWithBun(DialectPostgreSQL, ast)

	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d: args=%v", len(args), args)
	}
	if args[0] != "'; DROP TABLE users; --" {
		t.Fatalf("malicious value must travel as arg, got %v", args[0])
	}
	if strings.Contains(selectSQL, "DROP TABLE") {
		t.Fatalf("value leaked into SQL text: %s", selectSQL)
	}
	if !strings.Contains(selectSQL, "?") {
		t.Fatalf("expected ? placeholder in SQL: %s", selectSQL)
	}
}

func TestUnmarshalJSONActuallyParses(t *testing.T) {
	var cols []map[string]any
	err := unmarshalJSON(`[{"name":"a","expr":"b"}]`, &cols)
	if err != nil {
		t.Fatalf("valid json must parse, got: %v", err)
	}
	if len(cols) != 1 || cols[0]["name"] != "a" {
		t.Fatalf("parsed content wrong: %+v", cols)
	}
}
