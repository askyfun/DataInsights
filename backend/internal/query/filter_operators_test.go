package query

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"data-insights/internal/domain/entity"
)

// 双端共读用例（issue #204）：同一份用例表也被
// frontend/src/__tests__/lib/filterOperators.cases.test.ts 读取。
type filterOperatorCase struct {
	Name string  `json:"name"`
	Raw  string  `json:"raw"`
	Want *string `json:"want"`
}

func loadFilterOperatorCases(t *testing.T) []filterOperatorCase {
	t.Helper()
	const fixturePath = "../../../frontend/src/lib/__fixtures__/filterOperators.json"
	raw, err := os.ReadFile(filepath.Clean(fixturePath))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture struct {
		Cases []filterOperatorCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}
	return fixture.Cases
}

func TestNormalizeFilterOperatorSharedCases(t *testing.T) {
	for _, tc := range loadFilterOperatorCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			got, ok := NormalizeFilterOperator(tc.Raw)
			if tc.Want == nil {
				if ok {
					t.Fatalf("raw %q 应被拒绝，实际归一化为 %q", tc.Raw, got)
				}
				return
			}
			if !ok {
				t.Fatalf("raw %q 应归一化为 %q，实际被拒绝", tc.Raw, *tc.Want)
			}
			if string(got) != *tc.Want {
				t.Fatalf("raw %q 归一化为 %q，期望 %q", tc.Raw, got, *tc.Want)
			}
		})
	}
}

// 每个规范算子都必须能通过归一化（幂等），保证白名单与常量表一致。
func TestCanonicalFilterOperatorsAreNormalizable(t *testing.T) {
	allowed := AllowedFilterOperators()
	if len(allowed) != len(canonicalFilterOperators) {
		t.Fatalf("allowed=%d canonical=%d", len(allowed), len(canonicalFilterOperators))
	}
	seen := make(map[string]struct{}, len(allowed))
	for _, op := range allowed {
		if _, dup := seen[op]; dup {
			t.Fatalf("规范算子 %q 重复", op)
		}
		seen[op] = struct{}{}
		got, ok := NormalizeFilterOperator(op)
		if !ok || string(got) != op {
			t.Fatalf("规范算子 %q 归一化为 (%q, %v)，应幂等", op, got, ok)
		}
	}
}

// 未知算子必须带定位信息拒绝：字段、收到的算子、允许集合缺一不可。
func TestNormalizeFilterConfigsRejectsUnknownWithLocation(t *testing.T) {
	filters := []FilterConfig{
		{Field: "region", Op: FilterEq, Value: "east"},
		{Field: "amount", Op: FilterOperator("gteee"), Value: 10},
	}
	_, err := NormalizeFilterConfigs(filters)
	if err == nil {
		t.Fatal("expected error for unknown operator")
	}
	opErr, ok := err.(*FilterOperatorError)
	if !ok {
		t.Fatalf("expected *FilterOperatorError, got %T: %v", err, err)
	}
	if opErr.Index != 1 || opErr.Field != "amount" || opErr.Operator != "gteee" {
		t.Fatalf("unexpected location: %+v", opErr)
	}
	msg := opErr.Error()
	for _, want := range []string{`"gteee"`, `"amount"`, "allowed operators are"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
	// 入参不被就地修改：失败时调用方仍能拿到原始请求。
	if filters[1].Op != FilterOperator("gteee") {
		t.Fatalf("NormalizeFilterConfigs mutated input: %q", filters[1].Op)
	}
}

// 别名必须在 NormalizeFilterConfigs 里收成规范算子，下游 builder 只认规范值。
func TestNormalizeFilterConfigsAppliesAliases(t *testing.T) {
	filters := []FilterConfig{
		{Field: "amount", Op: FilterOperator(">="), Value: 10},
		{Field: "status", Op: FilterOperator("NOT IN"), Value: []any{"x"}},
	}
	got, err := NormalizeFilterConfigs(filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Op != FilterGte || got[1].Op != FilterNotIn {
		t.Fatalf("normalized ops = %q, %q; want gte, notIn", got[0].Op, got[1].Op)
	}
}

// 直接构造 AST 的调用方同样受守卫：executor 在生成 SQL 前调用本方法。
func TestQueryASTNormalizeFilterOperators(t *testing.T) {
	ast := &QueryAST{Filters: []FilterExpr{{Field: "amount", Op: FilterOperator("GT")}}}
	if err := ast.NormalizeFilterOperators(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ast.Filters[0].Op != FilterGt {
		t.Fatalf("op=%q, want gt", ast.Filters[0].Op)
	}

	bad := &QueryAST{Filters: []FilterExpr{{Field: "amount", Op: FilterOperator("betweenn")}}}
	if err := bad.NormalizeFilterOperators(); err == nil {
		t.Fatal("expected error for unknown AST operator")
	}
}

// dataset 去重取数入口（过滤弹窗枚举候选值）与图表查询同一条纪律：
// 别名归一化生效、未知算子显式拒绝。
func TestBuildDatasetDistinctQueryFilterOperators(t *testing.T) {
	columns := `[{"id":"c1","name":"region","expr":"region","type":"string"}]`
	base := entity.QueryConfig{
		DimensionGroups: []entity.FieldGroup{{Fields: []string{"c1"}}},
	}

	ok := base
	ok.Filters = []entity.Filter{{Field: "c1", Operator: ">=", Value: "a"}}
	sql, _, err := BuildDatasetDistinctQuery(DialectPostgreSQL, columns, "orders", SourceTypeTable, ok)
	if err != nil {
		t.Fatalf("alias should be accepted: %v", err)
	}
	if !strings.Contains(sql, ">=") {
		t.Fatalf("normalized operator missing from SQL: %s", sql)
	}

	bad := base
	bad.Filters = []entity.Filter{{Field: "c1", Operator: "gteee", Value: "a"}}
	if _, _, err := BuildDatasetDistinctQuery(DialectPostgreSQL, columns, "orders", SourceTypeTable, bad); err == nil {
		t.Fatal("unknown operator must be rejected")
	}
}
