package query

import (
	"fmt"
	"strings"
)

// histogram 两阶段分箱查询（R-57，plan §3.3）的输出列别名。executor/processor
// 按这些名字逐字读取行键（quoteResultAlias 引号保留后与数据库返回的列名一致）。
const (
	histogramMinAlias   = "mn"  // 阶段1 MIN(field)
	histogramMaxAlias   = "mx"  // 阶段1 MAX(field)
	histogramCountAlias = "cnt" // 阶段1 COUNT(*) / 阶段2 每箱计数
	histogramBinAlias   = "bin" // 阶段2 FLOOR 分箱索引
)

// BuildHistogramStatsQuery 生成直方图阶段1（统计）查询：
//
//	SELECT MIN(<field>) AS "mn", MAX(<field>) AS "mx", COUNT(*) AS "cnt"
//	FROM <source> WHERE <filters>   -- 过滤值一律走参数化 args
//
// 被分箱的字段是 histogram 的单个 value 字段（req.Metrics[0].Field，v1 平铺），
// 语义是"把一列原始数值分箱、数每箱行数"，因此这里不做任何聚合包裹。
// 字段解析与其他 builder 同源：先查 ast.ColumnMappings（GetMetricFieldExpr），
// 再过 safeIdentifier 白名单。忽略 ast.Sort/Pagination/Limit：min/max/count
// 必须基于过滤后的完整数据集。
func BuildHistogramStatsQuery(dialect DialectType, ast *QueryAST, valueField string) (string, []any) {
	qb := NewBunQueryBuilder()
	qb.SetDialect(dialect)
	qb.columnMappings = ast.ColumnMappings
	return qb.buildHistogramStatsQuery(ast, valueField)
}

func (qb *BunQueryBuilder) buildHistogramStatsQuery(ast *QueryAST, valueField string) (string, []any) {
	var args []any
	var sb strings.Builder

	field := safeIdentifier(ast.GetMetricFieldExpr(valueField))

	sb.WriteString("SELECT ")
	sb.WriteString(fmt.Sprintf("MIN(%s) AS %s, MAX(%s) AS %s, COUNT(*) AS %s",
		field, qb.quoteResultAlias(histogramMinAlias),
		field, qb.quoteResultAlias(histogramMaxAlias),
		qb.quoteResultAlias(histogramCountAlias)))
	sb.WriteString(" FROM ")
	sb.WriteString(histogramSourceSQL(ast))
	if len(ast.Filters) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(qb.buildWhereClause(ast, &args))
	}

	return sb.String(), args
}

// BuildHistogramBinQuery 生成直方图阶段2（分箱计数）查询：
//
//	SELECT "bin", COUNT(*) AS "cnt"
//	FROM (SELECT FLOOR((<field> - ?) / ?) AS "bin" FROM <source> WHERE <filters>) AS _hist_bins
//	GROUP BY "bin"
//	ORDER BY "bin"
//
// 两个 ? 是 minValue 和 binWidth（executor 由阶段1 结果在 Go 端算出的 float64），
// 一律作为参数化 args 传入——绝不 fmt 拼接裸浮点进 SQL 文本（防注入 + 浮点格式
// 方言差异），且 float 参数天然避免 PG integer/integer 的整数除法截断
// （field - $1 中 $1 为 float8 → 整个表达式提升为浮点）。
// args 顺序与 ? 占位一一对应：[minValue, binWidth, 过滤参数...]。
//
// GROUP BY 采用子查询派生列而非重复表达式或输出别名（真实 PG 实测裁定，
// histogram_correctness_integration_test.go）：
//   - 重复表达式在 PG 上报 42803（"column must appear in the GROUP BY clause"）——
//     驱动把每个 ? 顺序 rebind 成独立 $N 序号，PG 无法证明 SELECT 与 GROUP BY 里
//     的两份表达式恒等；
//   - GROUP BY 输出别名 PG/MySQL 支持，ClickHouse/StarRocks 视版本而定；
//   - GROUP BY 派生表列是各方言共同的安全交集（内层 "bin" 已是真实列）。
//
// ORDER BY "bin" 保证 bin 索引升序输出（processor 按索引补全，顺序无关，
// 但确定性输出便于断言与调试）。忽略 ast.Sort/Pagination/Limit：分箱必须覆盖
// 过滤后的完整数据集。
func BuildHistogramBinQuery(dialect DialectType, ast *QueryAST, valueField string, minValue, binWidth float64) (string, []any) {
	qb := NewBunQueryBuilder()
	qb.SetDialect(dialect)
	qb.columnMappings = ast.ColumnMappings
	return qb.buildHistogramBinQuery(ast, valueField, minValue, binWidth)
}

func (qb *BunQueryBuilder) buildHistogramBinQuery(ast *QueryAST, valueField string, minValue, binWidth float64) (string, []any) {
	var args []any
	var sb strings.Builder

	field := safeIdentifier(ast.GetMetricFieldExpr(valueField))
	binCol := qb.quoteResultAlias(histogramBinAlias)

	// 内层派生表：FLOOR 分箱索引 + 参数化 (minValue, binWidth) + 过滤。
	args = append(args, minValue, binWidth)
	sb.WriteString("SELECT ")
	sb.WriteString(binCol)
	sb.WriteString(fmt.Sprintf(", COUNT(*) AS %s FROM (SELECT FLOOR((%s - ?) / ?) AS %s FROM %s",
		qb.quoteResultAlias(histogramCountAlias), field, binCol, histogramSourceSQL(ast)))
	if len(ast.Filters) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(qb.buildWhereClause(ast, &args))
	}
	sb.WriteString(") AS _hist_bins GROUP BY ")
	sb.WriteString(binCol)
	sb.WriteString(" ORDER BY ")
	sb.WriteString(binCol)

	return sb.String(), args
}

// histogramSourceSQL 渲染 FROM 源：与通用/透视 builder 完全同口径
// （SQL 数据集包一层 _subq 派生表，表数据集过 safeIdentifier）。
func histogramSourceSQL(ast *QueryAST) string {
	if ast.SourceType == SourceTypeSQL {
		return "(" + ast.Source + ") AS _subq"
	}
	return safeIdentifier(ast.Source)
}
