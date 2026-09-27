package query

import (
	"strings"
)

// TableTotalLabel 合计行在第一个维度列上展示的标签（后端固定，保证 builder 预览 /
// 分享页 / 仪表盘三个渲染面用的是同一个词）。
const TableTotalLabel = "合计"

// tableTotalOptions 从请求的 query_options 读取「合计行」开关（executor 的 table
// 分支调用）：wire 键是 snake_case show_total（与 histogram 的 bin_count 同口径），
// camelCase showTotal 一并接受——持久化文档小节是 camelCase，任何未经
// normalizePersistedQueryOptions 归一的调用面（手写请求、将来新增的取数入口）都能
// 打开该能力。JSON 布尔进 map[string]any 后是 bool，toFloat64 兼容的是数值形态，
// 这里两者都认（true/1 为开），其余形态一律视为关闭。
func tableTotalOptions(opts map[string]any) bool {
	if v, ok := opts["show_total"].(bool); ok {
		return v
	}
	if v, ok := opts["showTotal"].(bool); ok {
		return v
	}
	for _, key := range []string{"show_total", "showTotal"} {
		if f, ok := toFloat64(opts[key]); ok && f == 1 {
			return true
		}
	}
	return false
}

// BuildTableTotalQuery 生成表格「合计行」查询（issue #131）：
//
//	SELECT <metrics with agg> FROM <source> WHERE <filters>
//
// 主要逻辑：**无维度、无 GROUP BY、无 ORDER BY、无 LIMIT/OFFSET**。合计必须基于
// 过滤后的完整数据集重算——带 LIMIT 只汇总当前页，按明细行相加又会让 AVG 变成
// 「平均数的平均数」、COUNT(DISTINCT) 变成「跨页去重后求和」。因此聚合值一律交给
// 数据库在整集上重算，与透视表 v2 的合计分支同口径。
//
// 约定与边界：
//   - 指标别名复用渲染合计行的同一套引用（renderMetricSelect + quoteResultAlias），
//     所以合计行的键与明细行的列名逐字一致，前端按列名取值即可；
//   - 无指标时没有可合计的量，返回空 SQL，调用方据此跳过（不建 SQL、不发查询）；
//   - 合计行的维度列不在 SELECT 里：调用方用 TableTotalLabel 补展示标签，
//     明细行的维度值不参与合计；
//   - 复用现有注入防护：标识符走 renderMetricSelect 内部的 safeIdentifier/safeExpr，
//     别名走 quoteResultAlias，过滤值经 buildWhereClause 参数化。
func BuildTableTotalQuery(dialect DialectType, ast *QueryAST) (string, []any) {
	qb := NewBunQueryBuilder()
	qb.SetDialect(dialect)
	qb.withASTIndex(ast)
	return qb.buildTableTotalQuery(ast)
}

// buildTableTotalQuery 是 BuildTableTotalQuery 的实现体，与通用路径共用
// BunQueryBuilder 的指标/过滤渲染方法。
func (qb *BunQueryBuilder) buildTableTotalQuery(ast *QueryAST) (string, []any) {
	var args []any
	if len(ast.Metrics) == 0 {
		return "", nil
	}

	metricParts := make([]string, 0, len(ast.Metrics))
	for _, metric := range ast.Metrics {
		metricParts = append(metricParts, qb.renderMetricSelect(metric))
	}

	var sb strings.Builder
	sb.WriteString("SELECT ")
	sb.WriteString(strings.Join(metricParts, ", "))
	sb.WriteString(" FROM ")
	if ast.SourceType == SourceTypeSQL {
		sb.WriteString("(")
		sb.WriteString(ast.Source)
		sb.WriteString(") AS _subq")
	} else {
		sb.WriteString(safeIdentifier(ast.Source))
	}

	if where := qb.buildWhereClause(ast, &args); where != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(where)
	}

	return sb.String(), args
}
