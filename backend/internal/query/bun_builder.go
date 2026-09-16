package query

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	bundialect "github.com/uptrace/bun/dialect"
)

// BunQueryBuilder 使用 bun ORM 的安全查询构建器
type BunQueryBuilder struct {
	columnMappings map[string]string
	dialect        DialectType
}

// NewBunQueryBuilder 创建新的 BunQueryBuilder
func NewBunQueryBuilder() *BunQueryBuilder {
	return &BunQueryBuilder{
		columnMappings: make(map[string]string),
	}
}

// WithColumnMappings 设置列映射
func (qb *BunQueryBuilder) WithColumnMappings(columns string) error {
	if columns == "" {
		return nil
	}

	var cols []ColumnInfo
	if err := unmarshalJSON(columns, &cols); err != nil {
		return err
	}

	for _, col := range cols {
		if col.Expr != "" && col.Name != "" {
			qb.columnMappings[col.Name] = col.Expr
		}
	}
	return nil
}

// SetDialect 设置数据库方言
func (qb *BunQueryBuilder) SetDialect(dialect DialectType) {
	qb.dialect = dialect
}

// Build 使用 bun QueryBuilder 构建查询
func (qb *BunQueryBuilder) Build(
	source string,
	sourceType SourceType,
	dims []string,
	metrics []MetricConfig,
	filters []FilterConfig,
	sort *SortConfig,
	pagination *Pagination,
) *QueryAST {
	ast := &QueryAST{
		Source:         source,
		SourceType:     sourceType,
		Dimensions:     dims,
		Filters:        make([]FilterExpr, 0, len(filters)),
		ColumnMappings: qb.columnMappings,
		Sort:           nil,
		Pagination:     pagination,
		Metrics:        make([]MetricExpr, 0, len(metrics)),
	}

	// 构建指标
	for _, m := range metrics {
		fieldExpr := qb.getFieldExpr(m.Field)
		expr := MetricExpr{
			Field:     m.Field,
			FieldExpr: fieldExpr,
			Agg:       m.Agg,
			Alias:     m.ResolveAlias(),
			IsAgg:     isAggregateFunction(fieldExpr),
		}
		ast.Metrics = append(ast.Metrics, expr)
	}

	// 构建过滤条件
	for _, f := range filters {
		expr := FilterExpr{
			Field:     f.Field,
			FieldExpr: qb.getFieldExpr(f.Field),
			Op:        f.Op,
			Value:     f.Value,
			ValueEnd:  f.ValueEnd,
			Logic:     f.Logic,
		}
		ast.Filters = append(ast.Filters, expr)
	}

	// 构建排序
	if sort != nil {
		ast.Sort = &SortExpr{
			Field:     sort.Field,
			FieldExpr: qb.getFieldExpr(sort.Field),
			Order:     sort.Order,
		}
	}

	return ast
}

// getFieldExpr 获取字段表达式
func (qb *BunQueryBuilder) getFieldExpr(field string) string {
	if expr, ok := qb.columnMappings[field]; ok && expr != "" {
		return expr
	}
	return field
}

// BuildSelectQuery 使用 bun 构建安全的选择查询
// 返回 SQL 字符串和参数列表（用于参数化查询）
func (qb *BunQueryBuilder) BuildSelectQuery(ast *QueryAST) (string, []interface{}) {
	var args []interface{}
	var sb strings.Builder

	sb.WriteString("SELECT ")

	// SELECT 子句
	selectParts := qb.buildSelectParts(ast)
	sb.WriteString(strings.Join(selectParts, ", "))

	// FROM 子句
	sb.WriteString(" FROM ")
	if ast.SourceType == SourceTypeSQL {
		sb.WriteString("(")
		sb.WriteString(ast.Source)
		sb.WriteString(") AS _subq")
	} else {
		sb.WriteString(safeIdentifier(ast.Source))
	}

	// WHERE 子句
	if len(ast.Filters) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(qb.buildWhereClause(ast, &args))
	}

	// GROUP BY 子句
	if len(ast.Dimensions) > 0 {
		sb.WriteString(" GROUP BY ")
		groupByParts := qb.buildGroupByParts(ast)
		sb.WriteString(strings.Join(groupByParts, ", "))
	}

	// ORDER BY 子句
	if ast.Sort != nil {
		sb.WriteString(" ORDER BY ")
		sb.WriteString(qb.renderSortRef(ast))
		sb.WriteString(" ")
		sb.WriteString(normalizeSortOrder(ast.Sort.Order))
	}

	// LIMIT/OFFSET 子句 - 直接嵌入数值，因为某些数据库驱动不支持预处理语句的 ? 占位符用于 LIMIT/OFFSET
	if ast.Pagination != nil {
		pageSize := ast.Pagination.PageSize
		offset := (ast.Pagination.Page - 1) * ast.Pagination.PageSize
		sb.WriteString(fmt.Sprintf(" LIMIT %d OFFSET %d", pageSize, offset))
	} else if ast.Limit > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", ast.Limit))
	}

	return sb.String(), args
}

// BuildCountQuery 构建计数查询
func (qb *BunQueryBuilder) BuildCountQuery(ast *QueryAST) (string, []interface{}) {
	var args []interface{}
	var sb strings.Builder

	sb.WriteString("SELECT COUNT(*) AS _total FROM (")

	// 内层查询
	sb.WriteString("SELECT 1")

	// FROM 子句
	sb.WriteString(" FROM ")
	if ast.SourceType == SourceTypeSQL {
		sb.WriteString("(")
		sb.WriteString(ast.Source)
		sb.WriteString(") AS _subq")
	} else {
		sb.WriteString(safeIdentifier(ast.Source))
	}

	// WHERE 子句
	if len(ast.Filters) > 0 {
		sb.WriteString(" WHERE ")
		whereParts := qb.buildWhereParts(ast, &args)
		sb.WriteString(strings.Join(whereParts, " AND "))
	}

	// GROUP BY 子句
	if len(ast.Dimensions) > 0 {
		sb.WriteString(" GROUP BY ")
		groupByParts := qb.buildGroupByParts(ast)
		sb.WriteString(strings.Join(groupByParts, ", "))
	}

	if ast.Pagination == nil && ast.Limit > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", ast.Limit))
	}

	sb.WriteString(") AS _count_query")

	return sb.String(), args
}

// buildSelectParts 构建 SELECT 部分
func (qb *BunQueryBuilder) buildSelectParts(ast *QueryAST) []string {
	var parts []string

	// 维度
	if len(ast.DimensionExprs) > 0 {
		for _, dim := range ast.DimensionExprs {
			parts = append(parts, qb.renderDimensionSelect(dim))
		}
	} else {
		for _, dim := range ast.Dimensions {
			parts = append(parts, safeIdentifier(dim))
		}
	}

	// 指标
	for _, metric := range ast.Metrics {
		var expr string
		if metric.IsAgg {
			expr = fmt.Sprintf("%s AS %s", safeExpr(metric.FieldExpr), qb.quoteResultAlias(metric.Alias))
		} else if metric.Agg == AggCountDistinct {
			// COUNT(DISTINCT field) 无法套用通用 "%s(%s)" 模板（会拼出括号不配对的
			// "COUNT(DISTINCT(field)" 畸形 SQL），单独生成。
			expr = fmt.Sprintf("COUNT(DISTINCT %s) AS %s", safeIdentifier(metric.FieldExpr), qb.quoteResultAlias(metric.Alias))
		} else {
			aggFunc := metric.Agg.GetAggFunc()
			expr = fmt.Sprintf("%s(%s) AS %s", aggFunc, safeIdentifier(metric.FieldExpr), qb.quoteResultAlias(metric.Alias))
		}
		parts = append(parts, expr)
	}

	if len(parts) == 0 {
		return []string{"*"}
	}

	return parts
}

// buildGroupByParts 构建 GROUP BY 字段列表。
// 调用场景：增强 QueryAST 后，时间粒度等结构化维度需要与 SELECT 子句一致的底层表达式，而不是直接按别名分组。
func (qb *BunQueryBuilder) buildGroupByParts(ast *QueryAST) []string {
	if len(ast.DimensionExprs) == 0 {
		groupByParts := make([]string, len(ast.Dimensions))
		for i, dim := range ast.Dimensions {
			groupByParts[i] = safeIdentifier(dim)
		}
		return groupByParts
	}

	groupByParts := make([]string, 0, len(ast.DimensionExprs))
	for _, dim := range ast.DimensionExprs {
		groupByParts = append(groupByParts, qb.renderDimensionGroupBy(dim))
	}
	return groupByParts
}

// renderDimensionSelect 渲染维度 SELECT 片段。
// 调用场景：普通维度直接输出字段，带时间粒度的维度输出方言表达式并附带稳定别名。
func (qb *BunQueryBuilder) renderDimensionSelect(dim DimensionExprAST) string {
	groupExpr := qb.renderDimensionGroupBy(dim)
	if dim.Alias != "" && dim.Alias != dim.Field {
		return fmt.Sprintf("%s AS %s", groupExpr, qb.quoteResultAlias(dim.Alias))
	}
	if dim.Granularity != "" {
		return fmt.Sprintf("%s AS %s", groupExpr, qb.quoteResultAlias(dim.Alias))
	}
	return groupExpr
}

// renderDimensionGroupBy 渲染维度 GROUP BY 表达式。
// 调用场景：按方言处理时间粒度；其余场景退化为安全字段标识符。
func (qb *BunQueryBuilder) renderDimensionGroupBy(dim DimensionExprAST) string {
	fieldExpr := dim.FieldExpr
	if fieldExpr == "" {
		fieldExpr = dim.Field
	}

	if dim.Granularity == "" {
		return safeIdentifier(fieldExpr)
	}

	safeField := safeIdentifier(fieldExpr)
	switch qb.dialect {
	case DialectPostgreSQL:
		return fmt.Sprintf("DATE_TRUNC('%s', %s)", dim.Granularity, safeField)
	case DialectMySQL:
		if dim.Granularity == "day" {
			return fmt.Sprintf("DATE(%s)", safeField)
		}
	case DialectClickHouse:
		if dim.Granularity == "day" {
			return fmt.Sprintf("toDate(%s)", safeField)
		}
	}

	return safeField
}

// buildWhereParts 构建 WHERE 部分，使用参数化查询
func (qb *BunQueryBuilder) buildWhereParts(ast *QueryAST, args *[]interface{}) []string {
	var parts []string

	for i := range ast.Filters {
		part := qb.buildFilterPart(&ast.Filters[i], args)
		if part == "" {
			continue
		}

		if len(parts) == 0 {
			parts = append(parts, part)
			continue
		}

		logic := strings.ToUpper(strings.TrimSpace(ast.Filters[i].Logic))
		if logic != "OR" {
			logic = "AND"
		}

		parts = append(parts, logic, part)
	}

	return parts
}

// buildWhereClause 构建完整 WHERE 子句，按过滤条件的 logic 连接。
func (qb *BunQueryBuilder) buildWhereClause(ast *QueryAST, args *[]interface{}) string {
	whereParts := qb.buildWhereParts(ast, args)
	return strings.Join(whereParts, " ")
}

// buildFilterPart 构建单个过滤条件，使用参数化查询
func (qb *BunQueryBuilder) buildFilterPart(f *FilterExpr, args *[]interface{}) string {
	field := safeIdentifier(f.FieldExpr)

	switch f.Op {
	case FilterIsNull:
		return fmt.Sprintf("%s IS NULL", field)
	case FilterIsNotNull:
		return fmt.Sprintf("%s IS NOT NULL", field)
	case FilterIn:
		if vals, ok := f.Value.([]any); ok && len(vals) > 0 {
			placeholders := make([]string, len(vals))
			for i := range vals {
				placeholders[i] = "?"
				*args = append(*args, vals[i])
			}
			return fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ", "))
		}
		return ""
	case FilterBetween:
		*args = append(*args, f.Value, f.ValueEnd)
		return fmt.Sprintf("%s BETWEEN ? AND ?", field)
	case FilterLike:
		*args = append(*args, "%"+fmt.Sprintf("%v", f.Value)+"%")
		return fmt.Sprintf("%s LIKE ?", field)
	default:
		*args = append(*args, f.Value)
		return fmt.Sprintf("%s %s ?", field, f.Op.ToString())
	}
}

// safeIdentifier 安全地处理标识符（表名、列名），防止 SQL 注入。
// 允许裸标识符或成对反引号/双引号包裹的标识符（数据集列表达式的受支持特性），
// 内部仅限 [a-zA-Z0-9_.]，引号不成对即拒绝。数据源连接信息等裸名场景
// 仍使用 datasource.IsValidIdentifier 的严格白名单。
var identifierTokenPattern = regexp.MustCompile("^(?:`[a-zA-Z0-9_.]+`|\"[a-zA-Z0-9_.]+\"|[a-zA-Z0-9_.]+)$")

func safeIdentifier(name string) string {
	name = strings.TrimSpace(name)
	if !identifierTokenPattern.MatchString(name) {
		return "_invalid_identifier"
	}
	return name
}

// aggExprPattern 校验聚合表达式（来自列映射的 FieldExpr，如 count(*)、SUM(amount)）：
// 仅允许"聚合函数(单个标识符 token 或*)"形态，括号内的标识符同样过白名单。
// 函数名收敛为显式聚合白名单（与 query.AggregationType / GetAggFunc 及
// entity/chart.go 的 agg 契约一致），防止 pg_sleep 等任意函数名透传。
var aggExprPattern = regexp.MustCompile(`(?i)^(count|sum|avg|min|max)\(\s*(\*|` + "`[a-z0-9_.]+`" + `|"[a-z0-9_.]+"|[a-z0-9_.]+)\s*\)$`)

// safeExpr 处理可能为聚合表达式的字段表达式（区别于纯标识符）。
func safeExpr(expr string) string {
	expr = strings.TrimSpace(expr)
	if aggExprPattern.MatchString(expr) {
		return expr
	}
	return safeIdentifier(expr)
}

// quoteResultAlias 把用户提供的结果别名渲染为方言正确的带引号标识符。
// 不加引号时数据库会把标识符折叠为小写（Postgres 把 AS Revenue 折叠成 revenue），
// 结果行键与处理器按别名逐字的查找对不上，图表数据全为 NULL；加引号后行键与
// 别名逐字一致。引号字符与 bun 各方言 Dialect.IdentQuote 一致
// （pgdialect 为双引号，MySQL/StarRocks/ClickHouse 为反引号，见 ParseDialect：
// starrocks 归入 DialectMySQL），复用 bun 导出的 dialect.AppendIdent 做转义，
// 不手写引号拼接。safeIdentifier 已保证入参只能是裸标识符或成对引号包裹形态，
// 后者保持既有透传行为，避免二次包裹。
func (qb *BunQueryBuilder) quoteResultAlias(alias string) string {
	name := safeIdentifier(alias)
	if strings.HasPrefix(name, `"`) || strings.HasPrefix(name, "`") {
		return name
	}
	quote := byte('`')
	if qb.dialect == DialectPostgreSQL {
		quote = '"'
	}
	return string(bundialect.AppendIdent(nil, name, quote))
}

// renderSortRef 渲染 ORDER BY 引用：排序键指向 SELECT 输出别名时，必须使用与
// SELECT 相同的引号（折叠大小写后就匹配不到带引号保留的输出列）；普通列排序
// 保持 safeIdentifier 原样输出。
func (qb *BunQueryBuilder) renderSortRef(ast *QueryAST) string {
	name := ast.Sort.FieldExpr
	if name == "" {
		name = ast.Sort.Field
	}
	if qb.referencesResultAlias(ast, name) {
		return qb.quoteResultAlias(name)
	}
	return safeIdentifier(name)
}

// referencesResultAlias 判断排序键是否等于某个会被引号保留的输出别名：
// 指标别名，或实际渲染出 AS 别名的维度（改名或带时间粒度）。
func (qb *BunQueryBuilder) referencesResultAlias(ast *QueryAST, name string) bool {
	if name == "" {
		return false
	}
	for _, m := range ast.Metrics {
		if m.Alias == name {
			return true
		}
	}
	for _, dim := range ast.DimensionExprs {
		if dim.Alias != name {
			continue
		}
		if dim.Alias != dim.Field || dim.Granularity != "" {
			return true
		}
	}
	return false
}

// unmarshalJSON 解析 JSON
func unmarshalJSON(data string, v interface{}) error {
	return json.Unmarshal([]byte(data), v)
}

// BunSQLBuilder 使用 bun 方式的 SQL 构建器
type BunSQLBuilder struct {
	dialect DialectType
}

// NewBunSQLBuilder 创建新的 BunSQLBuilder
func NewBunSQLBuilder(dialect DialectType) *BunSQLBuilder {
	return &BunSQLBuilder{dialect: dialect}
}

// BuildSelect 构建选择查询，同时返回参数化 args
func (b *BunSQLBuilder) BuildSelect(ast *QueryAST) (string, []interface{}) {
	qb := NewBunQueryBuilder()
	qb.SetDialect(b.dialect)
	qb.columnMappings = ast.ColumnMappings

	return qb.BuildSelectQuery(ast)
}

// BuildCount 构建计数查询，同时返回参数化 args（与 select 的 filter args 相同）
func (b *BunSQLBuilder) BuildCount(ast *QueryAST) (string, []interface{}) {
	qb := NewBunQueryBuilder()
	qb.SetDialect(b.dialect)
	qb.columnMappings = ast.ColumnMappings

	return qb.BuildCountQuery(ast)
}

// Dialect 返回方言类型
func (b *BunSQLBuilder) Dialect() DialectType {
	return b.dialect
}
