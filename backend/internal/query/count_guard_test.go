package query

// 回归测试（chaos 复现转正，2026-09-20）：钉住多过滤条件下 count SQL 的构造不变式（守卫 count 双 AND 修复）。

import (
	"strings"
	"testing"
)

// TestZZCountSQLMultiFilterMalformed 最小复现：
// buildWhereParts 返回的切片里 logic 标记（"AND"）本身就是一个**独立元素**
// （bun_builder.go:349 `parts = append(parts, logic, part)`），
// 供 buildWhereClause 用 Join(" ") 拼出正确的 "p0 AND p1"；
// 但 BuildCountQuery（bun_builder.go:194-196）改用 Join(" AND ") 拼同一份切片，
// 于是每个 logic 标记两侧各多出一个 AND，产出 "p0 AND AND AND p1"。
func TestZZCountSQLMultiFilterMalformed(t *testing.T) {
	ast := &QueryAST{
		Source:     "t",
		SourceType: SourceTypeTable,
		Filters: []FilterExpr{
			{Field: "c1", FieldExpr: "c1", Op: FilterEq, Value: 1, Logic: "AND"},
			{Field: "c2", FieldExpr: "c2", Op: FilterEq, Value: 2, Logic: "AND"},
		},
	}
	qb := NewBunQueryBuilder()
	_, countSQL := func() (string, string) {
		s, _ := qb.BuildSelectQuery(ast)
		c, _ := qb.BuildCountQuery(ast)
		return s, c
	}()

	t.Logf("count SQL = %s", countSQL)

	if strings.Contains(countSQL, "AND AND AND") {
		t.Fatalf("count SQL 含连续 AND（畸形）：%s", countSQL)
	}
	// 期望形态：WHERE c1 = ? AND c2 = ?
	want := "WHERE c1 = ? AND c2 = ?"
	if !strings.Contains(countSQL, want) {
		t.Fatalf("count SQL 未含期望的 %q\n实际：%s", want, countSQL)
	}
}

// TestZZCountSQLSingleFilterOK 单过滤条件是对照组：形态正确，说明问题只出在 ≥2 个条件。
func TestZZCountSQLSingleFilterOK(t *testing.T) {
	ast := &QueryAST{
		Source:     "t",
		SourceType: SourceTypeTable,
		Filters: []FilterExpr{
			{Field: "c1", FieldExpr: "c1", Op: FilterEq, Value: 1, Logic: "AND"},
		},
	}
	qb := NewBunQueryBuilder()
	countSQL, _ := qb.BuildCountQuery(ast)
	t.Logf("单条件 count SQL = %s", countSQL)
	if strings.Contains(countSQL, "AND AND") {
		t.Fatalf("单条件也畸形：%s", countSQL)
	}
}

// TestZZGranularityGateIsTheOnlyDefense 钉住 Granularity 的注入防线**只在 executor 的
// ValidateGranularity**，builder 自身不做任何校验：绕过该门直接建 SQL 时，粒度串会
// 原样落进 DATE_TRUNC('...') 的引号内 → 可闭合引号后追加任意 SQL。
// 当前生产路径（executor.go:70）先校验后建 SQL，因此这不是现行可达漏洞，属「契约依赖」。
func TestZZGranularityGateIsTheOnlyDefense(t *testing.T) {
	const payload = `day', 1));DROP TABLE t--`
	ast := &QueryAST{
		Source:     "t",
		SourceType: SourceTypeTable,
		DimensionExprs: []DimensionExprAST{
			{Field: "created_at", FieldExpr: "created_at", Alias: "d", Granularity: payload},
		},
	}

	// 防线 1（唯一的一道）：ValidateGranularity 必须拒绝
	if err := ast.ValidateGranularity(DialectPostgreSQL); err == nil {
		t.Fatalf("ValidateGranularity 未拒绝注入形态的粒度 %q", payload)
	} else {
		t.Logf("ValidateGranularity 正确拒绝：%v", err)
	}

	// 防线 2（不存在）：绕过校验直接建 SQL
	sel, _, _ := BuildQueryStringWithBun(DialectPostgreSQL, ast)
	t.Logf("绕过校验后的 SQL = %s", sel)
	if !strings.Contains(sel, payload) {
		t.Logf("注意：builder 未原样透传粒度串（可能已被其他层拦下）")
		return
	}
	t.Logf("确认：builder 自身不做校验，粒度串原样进入 SQL —— 安全完全依赖调用方先过 ValidateGranularity")
}
