package query

import (
	"fmt"
	"strings"
)

// BuildPivotGroupingSetsQuery 生成透视表 v2（R-53）的 GROUPING SETS 查询：
// 数据库在每个分组层级上自动重算聚合值（AVG/COUNT(DISTINCT) 的小计/合计正确性关键，
// 不能在 Go 端对子分组结果做二次聚合）。形状：
//
//	SELECT <row_dims>, <col_dims>, <metrics with agg>,
//	       GROUPING(<row_dim_i>) AS "__pivot_row_grp_i", ...
//	       GROUPING(<col_dim_j>) AS "__pivot_col_grp_j", ...
//	FROM <source>
//	WHERE <filters>            -- 值一律走参数化 args
//	GROUP BY GROUPING SETS (
//	  (<row_dims>, <col_dims>), -- 明细
//	  (<row_dims>),             -- 行小计（把所有列维度汇总掉，保留所有行维度）
//	  ()                        -- 合计（行、列维度都汇总掉）
//	)
//	ORDER BY <row_dims>, <col markers>, <col_dims>
//
// 约定与边界：
//   - 多维度自然扩展：2 行维度 + 1 列维度 → ((r1, r2, c1), (r1, r2), ())。
//     "行小计"是把所有列维度汇总掉但保留所有行维度，不做逐行维度层级的完整
//     CUBE/ROLLUP（plan §3.2 未要求该复杂度）。
//   - GROUPING() 用单参形式（每维度一个标记列），这是 PG 与 ClickHouse 共同的
//     最小交集：PG 的 GROUPING(x) 返回 0/1；ClickHouse 也提供 grouping()（返回
//     位掩码，单参即 0/1），但其 GROUPING SETS + grouping() 的组合行为尚未经真实
//     探针验证（Task 0-7 的 stub 静态声明 SupportsGroupingSets=true），待 Task 3-0
//     实测确认；多参位掩码形式因方言间位序语义不确定而弃用。
//   - ORDER BY 保证确定性输出：每个 RowKey 分组内明细行在前（列标记=0）、行小计在
//     后（列标记=1），ColHeaders 的"出现顺序"因此稳定；合计行的行维度值为 NULL，
//     各方言 NULL 排序位置不同，但 PivotProcessorV2 按标记列单独提取它，位置无关紧要。
//   - 忽略 ast.Sort / ast.Pagination / ast.Limit：小计/合计必须基于完整数据集计算，
//     LIMIT 会截断分组导致合计错误；透视表的排序语义由前端消费方处理（plan 未要求
//     SQL 级用户排序）。
//   - 复用现有注入防护：标识符过 safeIdentifier（经 renderDimensionGroupBy/
//     renderDimensionSelect/renderMetricSelect），别名过 quoteResultAlias，
//     过滤值经 buildWhereClause 走参数化 args。
func BuildPivotGroupingSetsQuery(dialect DialectType, ast *QueryAST, rowDims, colDims []DimensionExprAST) (string, []any) {
	qb := NewBunQueryBuilder()
	qb.SetDialect(dialect)
	qb.columnMappings = ast.ColumnMappings
	return qb.buildPivotGroupingSetsQuery(ast, rowDims, colDims)
}

// buildPivotGroupingSetsQuery 是 BuildPivotGroupingSetsQuery 的实现体，
// 复用 BunQueryBuilder 的维度/指标/过滤渲染方法（与通用路径完全同源）。
func (qb *BunQueryBuilder) buildPivotGroupingSetsQuery(ast *QueryAST, rowDims, colDims []DimensionExprAST) (string, []any) {
	var args []any
	var sb strings.Builder

	rowExprs := make([]string, 0, len(rowDims))
	for _, dim := range rowDims {
		rowExprs = append(rowExprs, qb.renderDimensionGroupBy(dim))
	}
	colExprs := make([]string, 0, len(colDims))
	for _, dim := range colDims {
		colExprs = append(colExprs, qb.renderDimensionGroupBy(dim))
	}

	parts := make([]string, 0, len(rowDims)+len(colDims)+len(ast.Metrics)*2)
	for _, dim := range rowDims {
		parts = append(parts, qb.renderDimensionSelect(dim))
	}
	for _, dim := range colDims {
		parts = append(parts, qb.renderDimensionSelect(dim))
	}
	for _, metric := range ast.Metrics {
		parts = append(parts, qb.renderMetricSelect(metric))
	}
	// GROUPING() 标记列：GROUPING 的参数必须与 GROUPING SETS 里的分组表达式逐字一致。
	for i := range rowDims {
		parts = append(parts, fmt.Sprintf("GROUPING(%s) AS %s", rowExprs[i], qb.quoteResultAlias(pivotRowMarkerAlias(i))))
	}
	for j := range colDims {
		parts = append(parts, fmt.Sprintf("GROUPING(%s) AS %s", colExprs[j], qb.quoteResultAlias(pivotColMarkerAlias(j))))
	}

	sb.WriteString("SELECT ")
	sb.WriteString(strings.Join(parts, ", "))

	sb.WriteString(" FROM ")
	if ast.SourceType == SourceTypeSQL {
		sb.WriteString("(")
		sb.WriteString(ast.Source)
		sb.WriteString(") AS _subq")
	} else {
		sb.WriteString(safeIdentifier(ast.Source))
	}

	if len(ast.Filters) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(qb.buildWhereClause(ast, &args))
	}

	detailSet := make([]string, 0, len(rowExprs)+len(colExprs))
	detailSet = append(detailSet, rowExprs...)
	detailSet = append(detailSet, colExprs...)
	sb.WriteString(" GROUP BY GROUPING SETS (")
	sb.WriteString("(")
	sb.WriteString(strings.Join(detailSet, ", "))
	sb.WriteString("), (")
	sb.WriteString(strings.Join(rowExprs, ", "))
	sb.WriteString("), ())")

	orderParts := make([]string, 0, len(rowExprs)+len(colDims)+len(colExprs))
	orderParts = append(orderParts, rowExprs...)
	for j := range colDims {
		orderParts = append(orderParts, qb.quoteResultAlias(pivotColMarkerAlias(j)))
	}
	orderParts = append(orderParts, colExprs...)
	sb.WriteString(" ORDER BY ")
	sb.WriteString(strings.Join(orderParts, ", "))

	return sb.String(), args
}
