package query

// 回归测试（chaos 复现转正，2026-09-20）：钉住过滤算子在建 SQL 层的 fail-closed 不变式。
// 背景：service 层 chart/impl.go 把请求里的原始 operator 字符串强转成 FilterOperator 且不经
// 白名单；若 builder 直接信任，未知算子会被 ToString 的 default 静默退化成 "="（把 gte 当 eq），
// 返回错误数据且全程无 error、无日志。修复后 builder 对未知算子渲染恒假谓词（1 = 0，命中 0 行），
// 与 IN/NotIn 值缺失时的 fail-closed 同构；同时保留 builder 层的畸形 IN 兜底与 between 对照。

import (
	"strings"
	"testing"
)

// zzWhereOpAfterC 取 SQL 里 "WHERE c " 之后的首个算子 token（找不到返回空串）。
func zzWhereOpAfterC(sql string) string {
	i := strings.Index(sql, "WHERE c ")
	if i < 0 {
		return ""
	}
	f := strings.Fields(sql[i+len("WHERE c "):])
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// TestZZKnownFilterOperatorsRenderCorrectly 受支持的标量算子各自映射到正确的 SQL 算子。
func TestZZKnownFilterOperatorsRenderCorrectly(t *testing.T) {
	cases := []struct {
		op   string
		want string
	}{
		{"eq", "="},
		{"neq", "<>"},
		{"gt", ">"},
		{"gte", ">="},
		{"lt", "<"},
		{"lte", "<="},
	}
	for _, c := range cases {
		ast := &QueryAST{
			Source:     "t",
			SourceType: SourceTypeTable,
			Filters: []FilterExpr{
				{Field: "c", FieldExpr: "c", Op: FilterOperator(c.op), Value: 1, Logic: "AND"},
			},
		}
		sql, _, args := BuildQueryStringWithBun(DialectPostgreSQL, ast)
		if got := zzWhereOpAfterC(sql); got != c.want {
			t.Errorf("已知算子 %q 渲染为 %q，期望 %q（sql=%s）", c.op, got, c.want, sql)
		}
		if len(args) != 1 {
			t.Errorf("已知算子 %q args=%d，期望 1", c.op, len(args))
		}
	}
}

// TestZZUnknownFilterOperatorFailsClosed 未知算子（笔误、大小写、符号、带空白、乱码）必须 fail-closed
// 到恒假谓词，绝不静默退化成等值比较、也不携带引用值的占位符。
func TestZZUnknownFilterOperatorFailsClosed(t *testing.T) {
	unknown := []string{"ge", "le", "GT", ">=", "lte ", "betweenn", "contains", "", "; DROP TABLE t"}
	for _, op := range unknown {
		ast := &QueryAST{
			Source:     "t",
			SourceType: SourceTypeTable,
			Filters: []FilterExpr{
				{Field: "c", FieldExpr: "c", Op: FilterOperator(op), Value: 1, Logic: "AND"},
			},
		}
		sql, _, args := BuildQueryStringWithBun(DialectPostgreSQL, ast)
		if !strings.Contains(sql, "1 = 0") {
			t.Errorf("未知算子 %q 未 fail-closed 到恒假谓词：%s", op, sql)
		}
		if strings.Contains(sql, "WHERE c = ?") {
			t.Errorf("未知算子 %q 被静默退化成等值：%s", op, sql)
		}
		if len(args) != 0 {
			t.Errorf("未知算子 %q 仍携带 args=%v，说明生成了引用值的谓词", op, args)
		}
	}
}

// TestZZMalformedInFilterFailsClosed IN/NotIn 值非数组（逗号串/标量）时渲染恒假谓词
// （IN (NULL) 命中 0 行），既不留悬空 WHERE、也不让条件静默消失。
func TestZZMalformedInFilterFailsClosed(t *testing.T) {
	// (a) 只有这一条过滤条件 → 不得留下悬空 WHERE（方言语法错误 → 500）
	for _, op := range []FilterOperator{FilterIn, FilterNotIn} {
		ast := &QueryAST{
			Source:     "t",
			SourceType: SourceTypeTable,
			Filters: []FilterExpr{
				{Field: "status", FieldExpr: "status", Op: op, Value: "active,pending", Logic: "AND"},
			},
		}
		sel, _, _ := BuildQueryStringWithBun(DialectPostgreSQL, ast)
		if strings.HasSuffix(strings.TrimSpace(sel), "WHERE") {
			t.Errorf("[仅一条] op=%s 产出悬空 WHERE：%q", op, sel)
		}
		if !strings.Contains(sel, "NULL") {
			t.Errorf("[仅一条] op=%s 未 fail-closed 到 IN (NULL)：%q", op, sel)
		}
	}

	// (b) 畸形 IN 与一条合法过滤共存 → 畸形那条渲染恒假片段，而非从 WHERE 消失
	for _, op := range []FilterOperator{FilterIn, FilterNotIn} {
		ast := &QueryAST{
			Source:     "t",
			SourceType: SourceTypeTable,
			Filters: []FilterExpr{
				{Field: "c", FieldExpr: "c", Op: FilterEq, Value: 1, Logic: "AND"},
				{Field: "status", FieldExpr: "status", Op: op, Value: "active,pending", Logic: "AND"},
			},
		}
		sel, _, args := BuildQueryStringWithBun(DialectPostgreSQL, ast)
		if !strings.Contains(sel, "status") {
			t.Errorf("[共存] op=%s 的过滤条件被静默丢弃（SQL 无 status 条件）：%q", op, sel)
		}
		if len(args) != 1 {
			t.Errorf("[共存] op=%s args=%d，期望仅 1（恒假片段不消费值参数）", op, len(args))
		}
	}
}

// TestZZBetweenMissingValueEnd 对照组：between 缺 value_end 时仍生成 BETWEEN ? AND ?，
// 两个参数分别来自 Value/ValueEnd（ValueEnd 为 nil → NULL），SQL 结果恒为 NULL ⇒ 0 行。
func TestZZBetweenMissingValueEnd(t *testing.T) {
	ast := &QueryAST{
		Source:     "t",
		SourceType: SourceTypeTable,
		Filters: []FilterExpr{
			{Field: "n", FieldExpr: "n", Op: FilterBetween, Value: 1, Logic: "AND"},
		},
	}
	sel, _, args := BuildQueryStringWithBun(DialectPostgreSQL, ast)
	t.Logf("between 缺 value_end → select=%s | args=%v", sel, args)
	if len(args) != 2 {
		t.Errorf("between 参数个数 = %d，期望 2", len(args))
	}
}
