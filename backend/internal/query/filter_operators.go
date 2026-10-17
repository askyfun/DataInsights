package query

import (
	"fmt"
	"strings"
)

// filterOperatorAliases 是过滤算子归一化的**唯一事实源**：键为小写、去空白后的
// 输入形态，值为规范算子。前端镜像 frontend/src/lib/filterOperators.ts，两份实现
// 由共读用例 frontend/src/lib/__fixtures__/filterOperators.json 钉住（go test 与
// vitest 各跑一遍），改表必须同时改两边。
//
// 归一化只接受明确列出的别名：无法识别的写法一律拒绝，绝不退化成等值比较
// （issue #204：`ge` / `GT` / `>=` 过去全部被当成 `=`，返回错数据且无 error）。
var filterOperatorAliases = map[string]FilterOperator{
	"eq":        FilterEq,
	"=":         FilterEq,
	"==":        FilterEq,
	"neq":       FilterNeq,
	"ne":        FilterNeq,
	"!=":        FilterNeq,
	"<>":        FilterNeq,
	"notequal":  FilterNeq,
	"not_equal": FilterNeq,

	"gt":  FilterGt,
	">":   FilterGt,
	"gte": FilterGte,
	"ge":  FilterGte,
	">=":  FilterGte,
	"lt":  FilterLt,
	"<":   FilterLt,
	"lte": FilterLte,
	"le":  FilterLte,
	"<=":  FilterLte,

	"like":    FilterLike,
	"in":      FilterIn,
	"notin":   FilterNotIn,
	"not_in":  FilterNotIn,
	"between": FilterBetween,

	"isnull":              FilterIsNull,
	"is_null":             FilterIsNull,
	"isnotnull":           FilterIsNotNull,
	"is_not_null":         FilterIsNotNull,
	"startswith":          FilterStartsWith,
	"starts_with":         FilterStartsWith,
	"endswith":            FilterEndsWith,
	"ends_with":           FilterEndsWith,
	"isemptystring":       FilterIsEmptyString,
	"is_empty_string":     FilterIsEmptyString,
	"isnotemptystring":    FilterIsNotEmptyString,
	"is_not_empty_string": FilterIsNotEmptyString,
}

// canonicalFilterOperators 是规范算子的有序列表（错误信息与 openapi 枚举同源）。
var canonicalFilterOperators = []FilterOperator{
	FilterEq,
	FilterNeq,
	FilterGt,
	FilterGte,
	FilterLt,
	FilterLte,
	FilterLike,
	FilterIn,
	FilterNotIn,
	FilterBetween,
	FilterIsNull,
	FilterIsNotNull,
	FilterStartsWith,
	FilterEndsWith,
	FilterIsEmptyString,
	FilterIsNotEmptyString,
}

// AllowedFilterOperators 返回规范算子列表（按声明顺序），用于错误提示与契约文档。
func AllowedFilterOperators() []string {
	out := make([]string, len(canonicalFilterOperators))
	for i, op := range canonicalFilterOperators {
		out[i] = string(op)
	}
	return out
}

// NormalizeFilterOperator 把外部算子的别名/大小写/空白归一化到规范算子。
// 返回 false 表示不在白名单内——调用方必须拒绝，不得回退等值比较。
func NormalizeFilterOperator(raw string) (FilterOperator, bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	// 忽略内部空白，让 "is null" / "not in" / "> = " 这类写法与紧凑写法等价。
	key = strings.Join(strings.Fields(key), "")
	if key == "" {
		return "", false
	}
	op, ok := filterOperatorAliases[key]
	return op, ok
}

// FilterOperatorError 定位一条被拒绝的过滤条件：字段、收到的原始算子、允许集合。
// 查询链路把它作为业务错误（20100）返回，避免只给一句无法排查的 "invalid filter"。
type FilterOperatorError struct {
	Index    int
	Field    string
	Operator string
}

func (e *FilterOperatorError) Error() string {
	return fmt.Sprintf(
		"unknown filter operator %q for field %q (filter %d): allowed operators are %s",
		e.Operator, e.Field, e.Index, strings.Join(AllowedFilterOperators(), ", "),
	)
}

// NormalizeFilterConfigs 归一化一组查询层过滤条件；任一算子无法识别即整体拒绝。
// 返回新切片，不修改入参，方便调用方在失败时保留原始请求用于报错。
func NormalizeFilterConfigs(filters []FilterConfig) ([]FilterConfig, error) {
	out := make([]FilterConfig, len(filters))
	copy(out, filters)
	for i := range out {
		op, ok := NormalizeFilterOperator(string(out[i].Op))
		if !ok {
			return nil, &FilterOperatorError{Index: i, Field: out[i].Field, Operator: string(out[i].Op)}
		}
		out[i].Op = op
	}
	return out, nil
}

// NormalizeFilterOperators 就地归一化 AST 上的过滤条件。
// 调用场景：executor 在生成 SQL 前做最后一道守卫——即使调用方绕过 service 层
// 直接构造 AST，未知算子也会显式报错，而不是落到 builder 的 fail-closed 恒假谓词。
func (q *QueryAST) NormalizeFilterOperators() error {
	for i := range q.Filters {
		op, ok := NormalizeFilterOperator(string(q.Filters[i].Op))
		if !ok {
			return &FilterOperatorError{Index: i, Field: q.Filters[i].Field, Operator: string(q.Filters[i].Op)}
		}
		q.Filters[i].Op = op
	}
	return nil
}
