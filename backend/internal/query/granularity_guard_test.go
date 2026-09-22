package query

// 回归测试（chaos 复现转正，2026-09-20）：粒度值在 builder 层必须有白名单守卫（defense-in-depth）。

import (
	"strings"
	"testing"
)

// TestZZGranularityBuilderHasNoGuard 记录一个「契约依赖」型缺口（当前不可达）：
// renderDimensionGroupBy（bun_builder.go:315）把 dim.Granularity 直接塞进
// `DATE_TRUNC('%s', field)` 的单引号里，builder 内部**不做任何校验**；
// 唯一守卫是 executor 在建 SQL 前调用的 ast.ValidateGranularity（executor.go:70）。
// 一旦新增调用点绕过 executor（或复用 builder 做 SQL 预览/导出），
// 单引号即可闭合、注入任意 SQL。
func TestZZGranularityBuilderHasNoGuard(t *testing.T) {
	payload := "day') /*x*/"
	ast := &QueryAST{
		Source:     "t",
		SourceType: SourceTypeTable,
		DimensionExprs: []DimensionExprAST{
			{Field: "ts", FieldExpr: "ts", Alias: "ts", Granularity: payload},
		},
	}

	// 1) executor 路径：先校验 → 拒绝（这是目前唯一的防线）
	if err := ast.ValidateGranularity(DialectPostgreSQL); err == nil {
		t.Fatalf("ValidateGranularity 竟然放行了 %q", payload)
	} else {
		t.Logf("ValidateGranularity 正确拒绝：%v", err)
	}

	// 2) builder 路径：直接建 SQL → 原始串原样进入 SQL 文本
	sel, _, _ := BuildQueryStringWithBun(DialectPostgreSQL, ast)
	t.Logf("builder 直出 SQL: %s", sel)
	if strings.Contains(sel, payload) {
		t.Errorf("builder 未校验粒度值，原始串 %q 已进入 SQL：%s", payload, sel)
	}
}
