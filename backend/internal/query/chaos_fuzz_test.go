package query

// fuzz 回归探针（chaos 测试转正，2026-09-20）：钉住 SQL 构造的注入不变式（白名单/参数化/引号配平）。

import (
	"encoding/json"
	"strings"
	"testing"

	"data-insights/internal/datasource"
)

// datasourceCapsStub 让 boxplot 的 percentile 分支可达（策略取 PG 的 percentile_cont）。
var datasourceCapsStub = datasource.DialectCapabilities{
	SupportsGroupingSets:   true,
	SupportsPercentileCont: true,
	PercentileStrategy:     "percentile_cont",
}

// allowedIdent 是「白名单形态」：裸标识符或成对反引号/双引号包裹。
func zzAllowedIdent(s string) bool {
	return identifierTokenPattern.MatchString(strings.TrimSpace(s))
}

// FuzzZZSafeIdentifier 钉住 safeIdentifier 的值域：只可能原样返回或退化为哨兵。
func FuzzZZSafeIdentifier(f *testing.F) {
	for _, s := range []string{
		"", " ", "a", "a.b", "a..b", ".", "..", "a.", ".a", "1a", "_x",
		"`a`", "`a`b`", "\"a\"", "'a'", "a b", "a;b", "a--", "a/*b*/",
		"a'b", "a\"b", "a`b", "a(b)", "a,b", "a=b", "a\nb", "a\x00b",
		"*", "count(*)", "SUM(amount)", "1);DROP TABLE x;--",
		"日本語", "😀", "_invalid_identifier",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got := safeIdentifier(in)
		if got == "_invalid_identifier" {
			return // 退化为哨兵，安全
		}
		// 未退化 ⇒ 必须是白名单形态，且逐字等于输入（trim 后）
		if !zzAllowedIdent(in) {
			t.Fatalf("safeIdentifier(%q) = %q：输入未过白名单却原样返回", in, got)
		}
		if got != strings.TrimSpace(in) {
			t.Fatalf("safeIdentifier(%q) = %q：非原样返回", in, got)
		}
		// 未退化形态不得含 SQL 分隔/注释/引号半开字符
		for _, c := range []string{";", "'", "--", "/*", "*/", "(", ")", " ", "\n", "\x00"} {
			if strings.Contains(got, c) {
				t.Fatalf("safeIdentifier(%q) = %q：含危险片段 %q", in, got, c)
			}
		}
	})
}

// FuzzZZSafeExpr 钉住聚合表达式白名单：非白名单形态必须退化为 safeIdentifier 结果，
// 绝不允许任意函数名/多语句透传。
func FuzzZZSafeExpr(f *testing.F) {
	for _, s := range []string{
		"count(*)", "COUNT( *)", "sum(amount)", "AVG(a.b)", "min(`x`)", "max(\"y\")",
		"pg_sleep(10)", "count(*) FROM x;--", "count(count(*))", "count(*)(*)",
		"count(*) AS x", "count(a,b)", "count(1)", "percentile_cont(0.5)",
		"sum(amount);DROP TABLE t", "count\u00a0(*)", "ＣＯＵＮＴ(*)",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got := safeExpr(in)
		if aggExprPattern.MatchString(strings.TrimSpace(in)) {
			if got != strings.TrimSpace(in) {
				t.Fatalf("safeExpr(%q) = %q：已过聚合白名单却非原样返回", in, got)
			}
			return
		}
		// 未过聚合白名单 ⇒ 必须与 safeIdentifier 同口径
		if want := safeIdentifier(in); got != want {
			t.Fatalf("safeExpr(%q) = %q，want safeIdentifier=%q", in, got, want)
		}
	})
}

// FuzzZZBuildSQLIdentifiers 端到端不变式：
// 把随机串塞进「本该过 safeIdentifier 的每一个位置」，若该串未过白名单，
// 则生成的 SQL 必须与「用哨兵替换后」生成的 SQL 逐字相同（哨兵引号归一化后）
// —— 即原始串一个字符都没漏进去。
//
// 注意：不做 `strings.Contains(sql, in)` 这类子串断言。fuzz 会造出 " " / "EEN" / " 1 "
// 这种**天然子串**，子串断言全是假阳性。结构等价比较才是精确判据。
func FuzzZZBuildSQLIdentifiers(f *testing.F) {
	for _, s := range []string{
		"", "a", "a.b", "count(*)", "a;DROP TABLE t", "a' OR 1=1--",
		"a/*x*/", "a`b", "a\"b", "1 OR 1=1", "x) UNION SELECT 1--",
		" ", "EEN", " 1 ", "'", "\"", "`", "\\", "\n", "\x00",
	} {
		f.Add(s)
	}
	dialects := []DialectType{DialectPostgreSQL, DialectMySQL, DialectClickHouse}
	f.Fuzz(func(t *testing.T, in string) {
		if zzAllowedIdent(in) {
			return // 合法标识符，原样出现是预期
		}
		for _, d := range dialects {
			raw := buildAllPositions(d, in)
			safe := buildAllPositions(d, safeIdentifier(in)) // 哨兵即 safeIdentifier 的退化值
			if zzNormSentinel(raw) != zzNormSentinel(safe) {
				t.Fatalf("方言 %s：未过白名单的 %q 改变了 SQL 结构\nraw : %s\nsafe: %s", d, in, raw, safe)
			}
			// 引号配平：任何一个未配对的引号都可能意味着某处逃逸出了字面量。
			for _, q := range []string{"'", "\"", "`"} {
				if strings.Count(raw, q)%2 != 0 {
					t.Fatalf("方言 %s：输入 %q 后 SQL 的 %s 数量为奇数（引号未配平）\n%s", d, in, q, raw)
				}
			}
		}
	})
}

// zzNormSentinel 抹掉「哨兵是否被引号包裹」这一唯一允许的差异：
// renderSortRef 的 referencesResultAlias 比较的是**原始**别名，退化后是否加引号
// 取决于原始串，两种形态都经过 safeIdentifier/quoteResultAlias，无注入风险。
func zzNormSentinel(s string) string {
	s = strings.ReplaceAll(s, `"_invalid_identifier"`, "_invalid_identifier")
	s = strings.ReplaceAll(s, "`_invalid_identifier`", "_invalid_identifier")
	return s
}

// buildAllPositions 把所有「本该过 safeIdentifier」的位置都填成 v，返回 SQL 文本拼接。
func buildAllPositions(d DialectType, v string) string {
	ast := &QueryAST{
		Source:     v,
		SourceType: SourceTypeTable,
		Dimensions: []string{v},
		DimensionExprs: []DimensionExprAST{
			{Field: v, FieldExpr: v, Alias: v},
		},
		Metrics: []MetricExpr{
			{Field: v, FieldExpr: v, Agg: AggSum, Alias: v},
			{Field: v, FieldExpr: v, Agg: AggCountDistinct, Alias: v},
			{Field: v, FieldExpr: v, Agg: AggMedian, Alias: v},
		},
		MetricExprs: []MetricPlanExpr{{Field: v, FieldExpr: v, Agg: AggSum, Alias: v}},
		Filters: []FilterExpr{
			{Field: v, FieldExpr: v, Op: FilterEq, Value: "p", Logic: "AND"},
			{Field: v, FieldExpr: v, Op: FilterIsNull},
			{Field: v, FieldExpr: v, Op: FilterIn, Value: []any{"p", "q"}},
			{Field: v, FieldExpr: v, Op: FilterBetween, Value: 1, ValueEnd: 2},
			{Field: v, FieldExpr: v, Op: FilterLike, Value: "p"},
		},
		Sort:       &SortExpr{Field: v, FieldExpr: v, Order: v},
		Pagination: &Pagination{Page: 1, PageSize: 10},
		Limit:      5,
	}
	sel, cnt, _ := BuildQueryStringWithBun(d, ast)
	pivotGS, _ := BuildPivotGroupingSetsQuery(d, ast,
		[]DimensionExprAST{{Field: v, FieldExpr: v, Alias: v}},
		[]DimensionExprAST{{Field: v, FieldExpr: v, Alias: v}})
	pivotUA, _ := BuildPivotUnionAllQuery(d, ast,
		[]DimensionExprAST{{Field: v, FieldExpr: v, Alias: v}},
		[]DimensionExprAST{{Field: v, FieldExpr: v, Alias: v}})
	histStats, _ := BuildHistogramStatsQuery(d, ast, v)
	histBin, _ := BuildHistogramBinQuery(d, ast, v, 0.5, 1.5)
	boxStats, _, _ := BuildBoxplotStatsQuery(d, ast, v, &capsStubPtr)
	boxOut, _ := BuildBoxplotOutliersQuery(d, ast, v, 1.0, 2.0)
	boxCnt, _ := BuildBoxplotOutlierCountQuery(d, ast, v, 1.0, 2.0)
	return strings.Join([]string{sel, cnt, pivotGS, pivotUA, histStats, histBin, boxStats, boxOut, boxCnt}, "\n")
}

var capsStubPtr = datasourceCapsStub

// FuzzZZFilterValuesParameterized 过滤值一律走 args，绝不允许改变 SQL 文本。
//
// 判据用**双构建结构等价**：把同一份 AST 的值换成另一个哨兵串，SQL 文本必须逐字相同；
// 同时确认该值确实流进了 args。这样既精确又不会像子串断言那样被 "EEN"（BETWEEN 的子串）
// 这类天然子串打成假阳性。
func FuzzZZFilterValuesParameterized(f *testing.F) {
	for _, s := range []string{
		"", "a", "a' OR 1=1--", "x;DROP TABLE t", "%", "_", "1) UNION SELECT 1--",
		"EEN", " 1 ", "AND", "SELECT",
	} {
		f.Add(s)
	}
	const sentinel = "ZZVALZZ-SENTINEL"
	build := func(v any) (string, string, []any) {
		ast := &QueryAST{
			Source:     "t",
			SourceType: SourceTypeTable,
			Filters: []FilterExpr{
				{Field: "c", FieldExpr: "c", Op: FilterEq, Value: v},
				{Field: "c", FieldExpr: "c", Op: FilterLike, Value: v},
				{Field: "c", FieldExpr: "c", Op: FilterBetween, Value: v, ValueEnd: v},
				{Field: "c", FieldExpr: "c", Op: FilterIn, Value: []any{v, v}},
			},
		}
		return BuildQueryStringWithBun(DialectPostgreSQL, ast)
	}
	f.Fuzz(func(t *testing.T, in string) {
		sel, cnt, args := build(in)
		sel2, cnt2, _ := build(sentinel)
		if sel != sel2 || cnt != cnt2 {
			t.Fatalf("过滤值 %q 改变了 SQL 文本（应全部参数化）\nsel   : %s\nsel(s): %s\ncnt   : %s\ncnt(s): %s",
				in, sel, sel2, cnt, cnt2)
		}
		// 值必须真的到达 args（否则上面的等价是因为被丢弃而非参数化）
		found := false
		for _, a := range args {
			if s, ok := a.(string); ok && strings.Contains(s, in) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("过滤值 %q 既不在 SQL 也不在 args（被静默丢弃）：args=%#v", in, args)
		}
	})
}

// FuzzZZPlanAndBuild 端到端：随机 JSON → ChartQueryRequest → 规划 → 任一方言建 SQL。
// 要求：① 不 panic；② 生成的 SQL 引号配平（任何未配对引号都意味着逃出了字面量/标识符）；
// ③ 非法 granularity 在建 SQL 前被 ValidateGranularity 拦下（与 executor 同序）。
//
// 这里刻意不再做 `strings.Contains(sql, 字段名)` 断言：fuzz 造出的 " "（空格）这类字段名
// 天然是 SQL 的子串，子串断言 100% 假阳性。精确判据见 FuzzZZBuildSQLIdentifiers。
func FuzzZZPlanAndBuild(f *testing.F) {
	f.Add(`{}`)
	f.Add(`{"chart_type":"bar","dims":["a"],"metrics":[{"field":"b","agg":"sum"}]}`)
	f.Add(`{"filters":[{"field":"a","op":"ge","value":1}]}`)
	f.Add(`{"filters":[{"field":"a","op":"gte","value":1},{"field":"b","op":"lte","value":2}]}`)
	f.Add(`{"sort":{"field":"a","order":"desc; DROP TABLE t"}}`)
	f.Add(`{"pagination":{"page":-1,"page_size":0}}`)
	f.Add(`{"query_options":{"bin_count":-5}}`)
	f.Add(`{"metrics":[{"field":"a","agg":"median"}]}`)
	f.Add(`{"dims":["a\nb"],"metrics":[{"field":"x);DROP TABLE t--","agg":"sum"}]}`)
	f.Add(`{"dims":[{"field":"d","granularity":"day', 1));DROP TABLE t--"}]}`)

	f.Fuzz(func(t *testing.T, doc string) {
		var req ChartQueryRequest
		if err := json.Unmarshal([]byte(doc), &req); err != nil {
			return
		}
		spec := QuerySpecFromRequest(&req)
		planner := NewQueryPlanner()
		ast := planner.PlanAST("t", SourceTypeTable, spec)
		ast.ApplyColumnMappings(map[string]string{})
		for _, d := range []DialectType{DialectPostgreSQL, DialectMySQL, DialectClickHouse} {
			if err := ast.ValidateGranularity(d); err != nil {
				continue // executor 同序：不合法粒度在建 SQL 前就返回错误
			}
			sel, cnt, _ := BuildQueryStringWithBun(d, ast)
			if sel == "" || cnt == "" {
				t.Fatalf("方言 %s 生成了空 SQL", d)
			}
			for _, q := range []string{"'", "\"", "`"} {
				if strings.Count(sel, q)%2 != 0 {
					t.Fatalf("方言 %s：SQL 的 %s 数量为奇数（引号未配平）\n%s", d, q, sel)
				}
			}
		}
	})
}

// FuzzZZChartSpecJSON 随机 JSON → ChartSpec/QuerySpec/entity 请求，要求不 panic。
func FuzzZZChartSpecJSON(f *testing.F) {
	f.Add(`{"chart_type":"pie","dimension_groups":[{"name":"x","fields":[{"field":"a","granularity":"day","binding_id":"b"}]}],"metric_groups":[{"name":"y","fields":[{"field":"b","agg":"sum"}]}]}`)
	f.Add(`null`)
	f.Add(`[]`)
	f.Add(`{"dimension_groups":[{"fields":[null]}]}`)
	f.Fuzz(func(t *testing.T, doc string) {
		var spec ChartSpec
		if err := json.Unmarshal([]byte(doc), &spec); err != nil {
			return
		}
		qs := QuerySpecFromChartSpecV2(&spec)
		dims, metrics, filters := QuerySpecToBuildArgs(qs)
		planner := NewQueryPlanner()
		ast := planner.PlanAST("t", SourceTypeTable, qs)
		if err := ast.ValidateGranularity(DialectPostgreSQL); err != nil {
			return // 与 executor 同序：粒度不合法则不再建 SQL
		}
		sel, cnt, _ := BuildQueryStringWithBun(DialectPostgreSQL, ast)
		if sel == "" || cnt == "" {
			t.Fatalf("生成空 SQL（输入 %q）", doc)
		}
		for _, q := range []string{"'", "\"", "`"} {
			if strings.Count(sel, q)%2 != 0 {
				t.Fatalf("SQL 的 %s 数量为奇数（引号未配平）：%s", q, sel)
			}
		}
		_ = dims
		_ = metrics
		_ = filters
	})
}

// FuzzZZFilterOpToString 钉住操作符映射：任何输入都只可能映射到已知 SQL 操作符，
// 绝不会把原始输入拼进 SQL。
func FuzzZZFilterOpToString(f *testing.F) {
	known := map[string]bool{
		"=": true, "<>": true, ">": true, ">=": true, "<": true, "<=": true,
		"LIKE": true, "IN": true, "NOT IN": true, "BETWEEN": true,
		"IS NULL": true, "IS NOT NULL": true,
	}
	for _, s := range []string{
		"eq", "neq", "gt", "gte", "lt", "lte", "like", "in", "notIn",
		"between", "isNull", "isNotNull", "ge", "le", "ne", "==", "=",
		"; DROP TABLE t", "OR 1=1", "IN (SELECT 1)", "",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got := FilterOperator(in).ToString()
		if !known[got] {
			t.Fatalf("ToString(%q) = %q：映射到未知操作符", in, got)
		}
		// 未知算子必须退化为等值 —— 记录该行为是否发生变化
		if !zzKnownOp(FilterOperator(in)) && got != "=" {
			t.Fatalf("未知算子 %q 未退化为 =，实际 %q", in, got)
		}
	})
}

func zzKnownOp(op FilterOperator) bool {
	switch op {
	case FilterEq, FilterNeq, FilterGt, FilterGte, FilterLt, FilterLte,
		FilterLike, FilterIn, FilterNotIn, FilterBetween, FilterIsNull, FilterIsNotNull:
		return true
	}
	return false
}

// FuzzZZRawSQLInterpolation 端到端：datasource.IsValidIdentifier 放行的名字被拼进裸
// SQL 后，SQL 文本里不得出现引号/分号/注释等可用于逃逸的片段。
func FuzzZZRawSQLInterpolation(f *testing.F) {
	for _, s := range []string{
		"t", "t.a", "a..b", "..", ".", "1", "t;", "t'", "t--", "t/**/",
		"t UNION SELECT 1", "已删除的schema", "t\n", "t\u00a0", "t`",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		sql, err := BuildTableDataSQL(in, in, in, 1, 0)
		if err != nil {
			return // 正确拒绝
		}
		for _, bad := range []string{"'", "\"", "`", ";", "--", "/*"} {
			if strings.Contains(sql, bad) {
				t.Fatalf("BuildTableDataSQL 放行 %q 后 SQL 含 %q：%s", in, bad, sql)
			}
		}
		dist, err := BuildFieldDistributionSQL(in, in, SourceTypeTable, 10)
		if err != nil {
			return
		}
		for _, bad := range []string{"'", "\"", "`", ";", "--", "/*"} {
			if strings.Contains(dist, bad) {
				t.Fatalf("BuildFieldDistributionSQL 放行 %q 后 SQL 含 %q：%s", in, bad, dist)
			}
		}
		if prev := WrapPreviewSQL(in, SourceTypeTable, 10); strings.Contains(prev, ";") {
			t.Fatalf("WrapPreviewSQL 放行 %q 后 SQL 含 ;：%s", in, prev)
		}
		if cnt := WrapCountSQL(in, SourceTypeTable); strings.Contains(cnt, ";") {
			t.Fatalf("WrapCountSQL 放行 %q 后 SQL 含 ;：%s", in, cnt)
		}
	})
}
