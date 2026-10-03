package query

import (
	"strings"
	"testing"
)

// issue #170：列索引是所有消费 bi_dataset.columns 的查询路径（图表查询、
// 数据集去重取数、分享页、仪表盘）的单一汇聚点。历史脏数据或绕过写入口
// 落库的恶意表达式必须在这里 fail-closed，查询期兜底与写入口校验构成双闸。
func TestBuildColumnIndexRejectsMaliciousExpr(t *testing.T) {
	malicious := []string{
		"price; DROP TABLE users",
		"(SELECT max(id) FROM users)",
		"pg_sleep(10)",
		"price -- comment",
		"amount UNION SELECT password FROM users",
	}
	for _, expr := range malicious {
		_, err := buildColumnIndex(`[{"id":"c1","name":"evil","expr":` + quoteGo(expr) + `}]`)
		if err == nil {
			t.Errorf("buildColumnIndex(expr=%q) 必须拒绝", expr)
		}
	}
}

func TestBuildColumnIndexAcceptsLegitimateExpr(t *testing.T) {
	idx, err := buildColumnIndex(`[
		{"id":"c1","name":"amount","expr":"amount"},
		{"id":"c2","name":"total","expr":"price * quantity"},
		{"id":"c3","name":"cnt","expr":"count(*)"},
		{"id":"c4","name":"q","expr":"\"quoted.col\""},
		{"id":"c5","name":"b","expr":"` + "`db.tbl.col`" + `"}
	]`)
	if err != nil {
		t.Fatalf("合法列表达式被误拒: %v", err)
	}
	if got, _ := idx.expr("c2"); got != "price * quantity" {
		t.Fatalf("expr(c2) = %q", got)
	}
}

func quoteGo(s string) string {
	// 测试内的小工具：把任意字符串安全嵌进 JSON 字面量。
	b := []byte{'"'}
	for _, r := range s {
		switch r {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		default:
			b = append(b, byte(r))
		}
	}
	return string(append(b, '"'))
}

// 回归（issue #170 E2E 发现）：复合标量表达式（temp_max - temp_min）此前在指标位
// 被 safeIdentifier 退化成 _invalid_identifier，聚合查询报「列无法解析」。过闸
// 表达式必须能进聚合括号。
func TestBuildSelectQueryCompoundExprMetric(t *testing.T) {
	qb := NewBunQueryBuilder()
	if err := qb.WithColumnMappings(`[{"id":"c1","name":"daily_range","expr":"temp_max - temp_min","role":"metric","type":"float"}]`); err != nil {
		t.Fatalf("WithColumnMappings: %v", err)
	}
	ast := qb.Build(
		"raw_city_weather_2026",
		SourceTypeTable,
		nil,
		[]MetricConfig{{Field: "c1", Agg: AggSum, Alias: "daily_range"}},
		[]FilterConfig{},
		nil,
		&Pagination{Page: 1, PageSize: 10},
	)
	sql, _ := NewBunSQLBuilder(DialectPostgreSQL).BuildSelect(ast)
	if !strings.Contains(sql, "SUM(temp_max - temp_min)") {
		t.Fatalf("SQL 应包含 SUM(temp_max - temp_min)，got: %s", sql)
	}
	if strings.Contains(sql, "_invalid_identifier") {
		t.Fatalf("合法复合表达式不得退化为 _invalid_identifier，got: %s", sql)
	}
}
