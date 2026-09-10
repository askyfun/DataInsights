package query

import "testing"

// TestSafeExprAggregateAllowList 锁定聚合表达式白名单回归：
// safeExpr 只应放行"聚合函数(标识符|*)"形态，函数名必须落在系统真实产出的
// 聚合集合内（AggregationType 常量 sum/avg/count/max/min，见 types.go GetAggFunc
// 与 entity/chart.go 契约注释）。非白名单函数名（如 pg_sleep）不得原样返回，
// 必须与非法标识符一样被降级为 _invalid_identifier。
func TestSafeExprAggregateAllowList(t *testing.T) {
	legitAggs := []string{
		"count(*)",
		"COUNT(*)",
		"SUM(amount)",
		"sum(bi_orders.amount)",
		"avg(rate)",
		"MIN(price)",
		"max(`amount`)",
		"sum(\"amount\")",
	}
	for _, expr := range legitAggs {
		if got := safeExpr(expr); got != expr {
			t.Errorf("safeExpr(%q) = %q, want unchanged (legitimate aggregate)", expr, got)
		}
	}

	// 非聚合形态的普通标识符必须与 safeIdentifier 行为一致。
	plainIdentifiers := []string{"plain_col", "bi_orders.amount", "`project_name`"}
	for _, id := range plainIdentifiers {
		if got, want := safeExpr(id), safeIdentifier(id); got != want {
			t.Errorf("safeExpr(%q) = %q, want %q (plain identifier routes to safeIdentifier)", id, got, want)
		}
	}

	// 非白名单函数调用 / 非法结构：不得原样返回。
	malicious := []string{
		"pg_sleep(1)",
		"x(*)",
		"count(*) FROM t",
		"sleep(5)",
		"countd(x)",
		"summarize(x)",
	}
	for _, expr := range malicious {
		if got := safeExpr(expr); got == expr {
			t.Errorf("safeExpr(%q) returned raw expression %q, want rejection (_invalid_identifier)", expr, got)
		} else if got != "_invalid_identifier" {
			t.Errorf("safeExpr(%q) = %q, want %q", expr, got, "_invalid_identifier")
		}
	}
}
