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
	qb.withASTIndex(ast)
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

	if where := qb.buildWhereClause(ast, &args); where != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(where)
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

// BuildPivotUnionAllQuery 生成透视表 v2（R-53）的 UNION ALL 回退查询：数据源不支持
// GROUPING SETS（caps.SupportsGroupingSets=false，MySQL/StarRocks）时使用。三个
// UNION ALL 分支各自携带自己的 GROUP BY（明细 rows+cols / 行小计 rows / 合计无
// GROUP BY），聚合值（含 AVG/COUNT(DISTINCT) 的小计/合计）由数据库在每个分组层级
// 独立重算，Go 端（PivotProcessorV2）只装配、不做二次聚合。产出行形状与
// BuildPivotGroupingSetsQuery 完全一致（同维度输出键、同指标别名、同
// __pivot_row_grp_i/__pivot_col_grp_j 标记列，值为字面 0/1），PivotProcessorV2 原样复用。
// 形状：
//
//	SELECT <row_dims>, <col_dims>, <metrics with agg>, 0 AS "__pivot_row_grp_i".., 0 AS "__pivot_col_grp_j"..
//	FROM <source> WHERE <filters> GROUP BY <row_dims>, <col_dims>  -- 明细
//	UNION ALL
//	SELECT <row_dims>, <NULL AS col 输出键>, <metrics>, 0 AS ..row.., 1 AS ..col..
//	FROM <source> WHERE <filters> GROUP BY <row_dims>              -- 行小计
//	UNION ALL
//	SELECT <NULL AS row 输出键>, <NULL AS col 输出键>, <metrics>, 1 AS ..row.., 1 AS ..col..
//	FROM <source> WHERE <filters>                                  -- 合计（无 GROUP BY 全表聚合）
//	ORDER BY <row_dim 输出别名>, <col 标记别名>, <col_dim 输出别名>
//
// 约定与边界：
//   - 末尾 ORDER BY 只能引用 UNION 的输出列别名（第一分支的列名）：UNION 结果是
//     匿名关系，引用底层 GROUP BY 表达式（如 DATE_TRUNC('day', ts)）在多数方言直接
//     报错——这是与 GROUPING SETS builder（单条分组查询，ORDER BY 底层表达式合法）
//     最关键的区别。排序语义与 GROUPING SETS 路径一致：每个 RowKey 分组内明细行
//     （列标记=0）在前、行小计（列标记=1）紧随其后，ColHeaders 出现顺序稳定。
//   - 三个分支各带一份 WHERE，过滤参数由 buildWhereClause 逐分支 append，最终 args =
//     [分支1.., 分支2.., 分支3..] 与 SQL 里 ? 占位从左到右一一对应（同一过滤参数三份）。
//   - 被汇总掉的维度列用 NULL AS <quoteResultAlias(pivotDimOutputKey(dim))> 占位，
//     保证 UNION 各分支列数/顺序/列名一致（UNION 按位置匹配，结果列名取第一分支）；
//     processor 在小计/合计行不读取这些 NULL 值（走哨兵键），只需位置存在 + 列名对齐。
//     合计分支无 GROUP BY，SELECT 里非聚合项均为字面 NULL/整数常量（非裸列引用），
//     在 MySQL ONLY_FULL_GROUP_BY 下合法。
//   - 忽略 ast.Sort / ast.Pagination / ast.Limit：小计/合计必须基于完整数据集计算，
//     LIMIT 会截断分组导致合计错误；ORDER BY 是 builder 固定的确定性排序（与
//     GROUPING SETS 路径同理由）。
//   - UNION ALL + GROUP BY 是最可移植的构造（这正是它作回退的原因）；ClickHouse/
//     StarRocks 的 UNION ALL 列类型统一（NULL 占位的 Nullable 提升与文本/数值列
//     统一）行为未经真实探针确认，待 Task 3-0；本路径主要面向 MySQL/StarRocks。
//   - 复用现有注入防护：标识符过 safeIdentifier（经 renderDimensionGroupBy/
//     renderDimensionSelect/renderMetricSelect），别名过 quoteResultAlias，
//     过滤值经 buildWhereClause 走参数化 args。
func BuildPivotUnionAllQuery(dialect DialectType, ast *QueryAST, rowDims, colDims []DimensionExprAST) (string, []any) {
	qb := NewBunQueryBuilder()
	qb.SetDialect(dialect)
	qb.withASTIndex(ast)
	return qb.buildPivotUnionAllQuery(ast, rowDims, colDims)
}

// buildPivotUnionAllQuery 是 BuildPivotUnionAllQuery 的实现体，
// 复用 BunQueryBuilder 的维度/指标/过滤渲染方法（与 GROUPING SETS 路径完全同源）。
func (qb *BunQueryBuilder) buildPivotUnionAllQuery(ast *QueryAST, rowDims, colDims []DimensionExprAST) (string, []any) {
	var args []any
	var sb strings.Builder

	// 各分支共用的维度渲染：真实 SELECT / GROUP BY 表达式，与被汇总掉时的 NULL 占位。
	rowSelects := make([]string, 0, len(rowDims))
	rowExprs := make([]string, 0, len(rowDims))
	rowNulls := make([]string, 0, len(rowDims))
	for _, dim := range rowDims {
		rowSelects = append(rowSelects, qb.renderDimensionSelect(dim))
		rowExprs = append(rowExprs, qb.renderDimensionGroupBy(dim))
		rowNulls = append(rowNulls, fmt.Sprintf("NULL AS %s", qb.quoteResultAlias(pivotDimOutputKey(dim))))
	}
	colSelects := make([]string, 0, len(colDims))
	colExprs := make([]string, 0, len(colDims))
	colNulls := make([]string, 0, len(colDims))
	for _, dim := range colDims {
		colSelects = append(colSelects, qb.renderDimensionSelect(dim))
		colExprs = append(colExprs, qb.renderDimensionGroupBy(dim))
		colNulls = append(colNulls, fmt.Sprintf("NULL AS %s", qb.quoteResultAlias(pivotDimOutputKey(dim))))
	}

	metricParts := make([]string, 0, len(ast.Metrics))
	for _, metric := range ast.Metrics {
		metricParts = append(metricParts, qb.renderMetricSelect(metric))
	}

	// 标记列是字面常量（0=真实值，1=被汇总掉），别名与 GROUPING SETS 路径逐字相同
	// （PivotProcessorV2 靠这套名字读取；DB 返回 int64，processor 的 toFloat64 已处理）。
	markerParts := func(rowFlag, colFlag int) []string {
		parts := make([]string, 0, len(rowDims)+len(colDims))
		for i := range rowDims {
			parts = append(parts, fmt.Sprintf("%d AS %s", rowFlag, qb.quoteResultAlias(pivotRowMarkerAlias(i))))
		}
		for j := range colDims {
			parts = append(parts, fmt.Sprintf("%d AS %s", colFlag, qb.quoteResultAlias(pivotColMarkerAlias(j))))
		}
		return parts
	}

	// source 渲染与 GROUPING SETS builder 完全一致，三个分支各写一份。
	source := func() string {
		if ast.SourceType == SourceTypeSQL {
			return "(" + ast.Source + ") AS _subq"
		}
		return safeIdentifier(ast.Source)
	}

	// writeBranch 组装单个 UNION 分支：SELECT 列 + FROM + WHERE（buildWhereClause
	// 把本分支的过滤参数 append 进 args，三分支共调用三次 → 参数天然三份）+ 可选 GROUP BY。
	writeBranch := func(selectParts []string, groupBy []string) {
		sb.WriteString("SELECT ")
		sb.WriteString(strings.Join(selectParts, ", "))
		sb.WriteString(" FROM ")
		sb.WriteString(source())
		if where := qb.buildWhereClause(ast, &args); where != "" {
			sb.WriteString(" WHERE ")
			sb.WriteString(where)
		}
		if len(groupBy) > 0 {
			sb.WriteString(" GROUP BY ")
			sb.WriteString(strings.Join(groupBy, ", "))
		}
	}

	// 分支1：明细（GROUP BY rows+cols；行、列标记全 0）。
	detailSelect := make([]string, 0, len(rowDims)+len(colDims)+len(metricParts)+len(rowDims)+len(colDims))
	detailSelect = append(detailSelect, rowSelects...)
	detailSelect = append(detailSelect, colSelects...)
	detailSelect = append(detailSelect, metricParts...)
	detailSelect = append(detailSelect, markerParts(0, 0)...)
	detailGroupBy := make([]string, 0, len(rowExprs)+len(colExprs))
	detailGroupBy = append(detailGroupBy, rowExprs...)
	detailGroupBy = append(detailGroupBy, colExprs...)
	writeBranch(detailSelect, detailGroupBy)

	sb.WriteString(" UNION ALL ")

	// 分支2：行小计（GROUP BY rows only；列维度被汇总掉 → NULL 占位，列标记全 1）。
	subtotalSelect := make([]string, 0, len(detailSelect))
	subtotalSelect = append(subtotalSelect, rowSelects...)
	subtotalSelect = append(subtotalSelect, colNulls...)
	subtotalSelect = append(subtotalSelect, metricParts...)
	subtotalSelect = append(subtotalSelect, markerParts(0, 1)...)
	writeBranch(subtotalSelect, rowExprs)

	sb.WriteString(" UNION ALL ")

	// 分支3：合计（无 GROUP BY，聚合在过滤后全表重算；行、列标记全 1）。
	grandSelect := make([]string, 0, len(detailSelect))
	grandSelect = append(grandSelect, rowNulls...)
	grandSelect = append(grandSelect, colNulls...)
	grandSelect = append(grandSelect, metricParts...)
	grandSelect = append(grandSelect, markerParts(1, 1)...)
	writeBranch(grandSelect, nil)

	// 末尾 ORDER BY：只引用 UNION 输出列别名（行/列维度输出键、列标记别名），
	// 不引用底层 GROUP BY 表达式（UNION 匿名关系上会报错，见 doc comment）。
	orderParts := make([]string, 0, len(rowDims)+len(colDims)*2)
	for _, dim := range rowDims {
		orderParts = append(orderParts, qb.quoteResultAlias(pivotDimOutputKey(dim)))
	}
	for j := range colDims {
		orderParts = append(orderParts, qb.quoteResultAlias(pivotColMarkerAlias(j)))
	}
	for _, dim := range colDims {
		orderParts = append(orderParts, qb.quoteResultAlias(pivotDimOutputKey(dim)))
	}
	sb.WriteString(" ORDER BY ")
	sb.WriteString(strings.Join(orderParts, ", "))

	return sb.String(), args
}
