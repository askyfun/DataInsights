package query

import "testing"

// #270：MySQL JSON 箭头表达式（col->>'$.key' / col->'$.key'）必须能通过下游两处闸：
//  1. buildColumnIndex（查询期列索引兜底）用 expr.Validate 校验虚拟字段 expr；
//  2. safeAggArg 用 expr.ValidateArg 判定后**原样内插**聚合实参。
// 二者放行即证明箭头表达式不会被降级成 _invalid_identifier，能真正进入 SQL。
func TestArrowExprPassesDownstreamGates(t *testing.T) {
	cases := []string{
		"attrs->>'$.name'",
		"attrs->'$.name'",
		"payload->'$.a'->>'b'",
	}
	for _, e := range cases {
		// 1) 列索引闸
		cols := `[{"id":"c1","name":"v","expr":` + quoteGo(e) + `}]`
		if _, err := buildColumnIndex(cols); err != nil {
			t.Fatalf("buildColumnIndex 拒绝箭头表达式 %q: %v", e, err)
		}
		// 2) 聚合实参原样内插
		if got := safeAggArg(e); got != e {
			t.Fatalf("safeAggArg(%q) = %q，期望原样内插", e, got)
		}
	}
}

// 反向：非法箭头形态仍必须被闸拒绝（不放宽边界）。
func TestIllegalArrowExprRejected(t *testing.T) {
	for _, e := range []string{"attrs->>", "attrs->>key", "attrs->>5", "a->>'x'--b"} {
		if got := safeAggArg(e); got != "_invalid_identifier" {
			t.Fatalf("safeAggArg(%q) = %q，期望被拒为 _invalid_identifier", e, got)
		}
	}
}
