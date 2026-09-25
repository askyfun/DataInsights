package query

import (
	"fmt"
	"strconv"
	"strings"

	"data-insights/internal/datasource"
)

// bun_builder_boxplot.go — 箱线图三查询 builder（R-52，plan §3.3）。
// 结构对齐 bun_builder_histogram.go：列别名常量 + 公开 BuildXxx(dialect, ast, ...)
// 内部建 qb + 设 dialect/columnMappings + 走私有方法。executor 编排 stats + outliers
// list + outliers count 三次查询、fence 由 stats 结果在 Go 端算。

const (
	boxWhiskerLowAlias    = "wlo"  // MIN(field)
	boxQ1Alias            = "q1"   // percentile_cont(0.25)
	boxMedianAlias        = "med"  // percentile_cont(0.5)
	boxQ3Alias            = "q3"   // percentile_cont(0.75)
	boxWhiskerHighAlias   = "whi"  // MAX(field)
	boxOutlierValueAlias  = "val"  // outliers list 每行 field 值列（避免列名歧义）
	boxOutlierCountAlias  = "ocnt" // outliers count 的 COUNT(*)
	boxOutlierDisplayMax  = 1000   // plan 行382：展示截断阈值
	boxIQRMultiplierConst = 1.5    // IQR fence 倍数（Tukey 惯例，plan §3.3 沿用）
)

// BuildBoxplotStatsQuery 生成箱线图阶段1（stats）查询：
//
//	SELECT MIN(<field>) AS "wlo",
//	       percentile_cont(0.25) WITHIN GROUP (ORDER BY <field>) AS "q1",
//	       percentile_cont(0.5)  WITHIN GROUP (ORDER BY <field>) AS "med",
//	       percentile_cont(0.75) WITHIN GROUP (ORDER BY <field>) AS "q3",
//	       MAX(<field>) AS "whi"
//	FROM <source> WHERE <filters>
//
// percentile 表达式通过 BuildPercentileExpr 生成（当前仅 percentile_cont 策略落地，
// executor 已前置门保证 caps 支持）。**注意：MIN/MAX 是全局值**（plan §3.3 字面口径，
// 非 Tukey 钳位须）；executor 用 q1/q3 在 Go 端算 IQR fence，走独立 outliers 查询。
// 忽略 ast.Sort/Pagination/Limit（stats 必须基于过滤后的完整数据集）。
func BuildBoxplotStatsQuery(
	dialect DialectType, ast *QueryAST, valueField string, caps *datasource.DialectCapabilities,
) (string, []any, error) {
	qb := NewBunQueryBuilder()
	qb.SetDialect(dialect)
	qb.withASTIndex(ast)
	return qb.buildBoxplotStatsQuery(ast, valueField, caps)
}

func (qb *BunQueryBuilder) buildBoxplotStatsQuery(
	ast *QueryAST, valueField string, caps *datasource.DialectCapabilities,
) (string, []any, error) {
	var args []any
	var sb strings.Builder

	field := safeIdentifier(ast.GetMetricFieldExpr(valueField))
	// percentile 表达式（三次：q1/median/q3）；executor 前置门保证 caps 支持，
	// 走到这里若仍 error（如调用者绕过 executor）就把 error 抛出、不静默近似。
	q1Expr, err := BuildPercentileExpr(field, 0.25, caps)
	if err != nil {
		return "", nil, fmt.Errorf("boxplot q1: %w", err)
	}
	medExpr, err := BuildPercentileExpr(field, 0.5, caps)
	if err != nil {
		return "", nil, fmt.Errorf("boxplot median: %w", err)
	}
	q3Expr, err := BuildPercentileExpr(field, 0.75, caps)
	if err != nil {
		return "", nil, fmt.Errorf("boxplot q3: %w", err)
	}

	sb.WriteString("SELECT ")
	sb.WriteString(fmt.Sprintf(
		"MIN(%s) AS %s, %s AS %s, %s AS %s, %s AS %s, MAX(%s) AS %s",
		field, qb.quoteResultAlias(boxWhiskerLowAlias),
		q1Expr, qb.quoteResultAlias(boxQ1Alias),
		medExpr, qb.quoteResultAlias(boxMedianAlias),
		q3Expr, qb.quoteResultAlias(boxQ3Alias),
		field, qb.quoteResultAlias(boxWhiskerHighAlias),
	))
	sb.WriteString(" FROM ")
	sb.WriteString(histogramSourceSQL(ast)) // 通用 FROM 源渲染（SQL 数据集包 _subq、表数据集 safeIdentifier）
	if where := qb.buildWhereClause(ast, &args); where != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(where)
	}
	return sb.String(), args, nil
}

// BuildBoxplotOutliersQuery 生成箱线图离群点列表查询：
//
//	SELECT <field> AS "val" FROM <source>
//	WHERE (<field> < ? OR <field> > ?) [AND <filters>]
//	ORDER BY <field>
//	LIMIT 1000
//
// 两个 ? 是 lower/upper fence（executor 由 stats 结果在 Go 端算）；参数化传入
// 防注入 + 避免浮点字符串化的方言差异（同 histogram bin 查询参数化裁定）。
// args 顺序 [lower, upper, filterArgs...]——fence 两参数先入 args，
// 与 SQL 文本里 ? 出现顺序一一对应；buildWhereClause 追加 filter args 在其后。
// LIMIT 是常量（boxOutlierDisplayMax），直接拼进 SQL 文本，非用户输入。
// **ORDER BY field 提供确定性输出**（同 fence 的多行按值升序），便于测试断言。
func BuildBoxplotOutliersQuery(
	dialect DialectType, ast *QueryAST, valueField string, lower, upper float64,
) (string, []any) {
	qb := NewBunQueryBuilder()
	qb.SetDialect(dialect)
	qb.withASTIndex(ast)
	return qb.buildBoxplotOutliersQuery(ast, valueField, lower, upper)
}

func (qb *BunQueryBuilder) buildBoxplotOutliersQuery(
	ast *QueryAST, valueField string, lower, upper float64,
) (string, []any) {
	var args []any
	var sb strings.Builder

	field := safeIdentifier(ast.GetMetricFieldExpr(valueField))
	// fence 参数先入 args（与 ? 出现顺序一致）。
	args = append(args, lower, upper)
	sb.WriteString("SELECT ")
	sb.WriteString(fmt.Sprintf("%s AS %s FROM %s", field, qb.quoteResultAlias(boxOutlierValueAlias), histogramSourceSQL(ast)))
	sb.WriteString(" WHERE ")
	sb.WriteString(fmt.Sprintf("(%s < ? OR %s > ?)", field, field))
	if where := qb.buildWhereClause(ast, &args); where != "" {
		sb.WriteString(" AND ")
		sb.WriteString(where)
	}
	sb.WriteString(" ORDER BY ")
	sb.WriteString(field)
	sb.WriteString(" LIMIT ")
	sb.WriteString(strconv.Itoa(boxOutlierDisplayMax))
	return sb.String(), args
}

// BuildBoxplotOutlierCountQuery 生成箱线图离群点总数查询（不受 LIMIT 影响，
// 给 truncated 判定 + outlier_total 展示）。args 顺序同 outliers list。
func BuildBoxplotOutlierCountQuery(
	dialect DialectType, ast *QueryAST, valueField string, lower, upper float64,
) (string, []any) {
	qb := NewBunQueryBuilder()
	qb.SetDialect(dialect)
	qb.withASTIndex(ast)
	return qb.buildBoxplotOutlierCountQuery(ast, valueField, lower, upper)
}

func (qb *BunQueryBuilder) buildBoxplotOutlierCountQuery(
	ast *QueryAST, valueField string, lower, upper float64,
) (string, []any) {
	var args []any
	var sb strings.Builder

	field := safeIdentifier(ast.GetMetricFieldExpr(valueField))
	args = append(args, lower, upper)
	sb.WriteString("SELECT ")
	sb.WriteString(fmt.Sprintf("COUNT(*) AS %s FROM %s", qb.quoteResultAlias(boxOutlierCountAlias), histogramSourceSQL(ast)))
	sb.WriteString(" WHERE ")
	sb.WriteString(fmt.Sprintf("(%s < ? OR %s > ?)", field, field))
	if where := qb.buildWhereClause(ast, &args); where != "" {
		sb.WriteString(" AND ")
		sb.WriteString(where)
	}
	return sb.String(), args
}
