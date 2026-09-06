package query

import (
	"fmt"
	"strings"
)

type DialectType string

const (
	DialectMySQL      DialectType = "mysql"
	DialectPostgreSQL DialectType = "postgresql"
	DialectClickHouse DialectType = "clickhouse"
)

type SQLBuilder interface {
	BuildSelect(ast *QueryAST) string
	BuildCount(ast *QueryAST) string
	Dialect() DialectType
}

type baseSQLBuilder struct{}

func (b *baseSQLBuilder) buildSelectClause(ast *QueryAST) string {
	var parts []string

	for _, dim := range ast.Dimensions {
		parts = append(parts, dim)
	}

	for _, metric := range ast.Metrics {
		var expr string
		if metric.IsAgg {
			expr = fmt.Sprintf("%s AS %s", metric.FieldExpr, metric.Alias)
		} else {
			aggFunc := metric.Agg.GetAggFunc()
			expr = fmt.Sprintf("%s(%s) AS %s", aggFunc, metric.FieldExpr, metric.Alias)
		}
		parts = append(parts, expr)
	}

	if len(parts) == 0 {
		return "*"
	}

	return strings.Join(parts, ", ")
}

func (b *baseSQLBuilder) buildFromClause(ast *QueryAST) string {
	if ast.SourceType == SourceTypeSQL {
		return fmt.Sprintf("FROM (%s) AS _subq", ast.Source)
	}
	return fmt.Sprintf("FROM %s", ast.Source)
}

func (b *baseSQLBuilder) buildWhereClause(ast *QueryAST) string {
	if len(ast.Filters) == 0 {
		return ""
	}

	var parts []string
	for i, f := range ast.Filters {
		clause := b.buildFilterClause(&f)
		if clause != "" {
			parts = append(parts, clause)
		}
		if i > 0 && ast.Filters[i].Logic != "" && ast.Filters[i-1].Logic != "" {
			parts = append(parts, ast.Filters[i].Logic)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	result := strings.Join(parts, " ")
	if len(parts) > 1 {
		result = "(" + result + ")"
	}
	return result
}

func (b *baseSQLBuilder) buildFilterClause(f *FilterExpr) string {
	switch f.Op {
	case FilterIsNull, FilterIsNotNull:
		return fmt.Sprintf("%s %s", f.FieldExpr, f.Op.ToString())
	case FilterIn:
		if vals, ok := f.Value.([]any); ok {
			strVals := make([]string, len(vals))
			for i, v := range vals {
				strVals[i] = fmt.Sprintf("'%v'", v)
			}
			return fmt.Sprintf("%s IN (%s)", f.FieldExpr, strings.Join(strVals, ", "))
		}
		return fmt.Sprintf("%s IN ('%v')", f.FieldExpr, f.Value)
	case FilterBetween:
		return fmt.Sprintf("%s BETWEEN '%v' AND '%v'", f.FieldExpr, f.Value, f.ValueEnd)
	default:
		return fmt.Sprintf("%s %s '%v'", f.FieldExpr, f.Op.ToString(), f.Value)
	}
}

func (b *baseSQLBuilder) buildGroupByClause(ast *QueryAST) string {
	if len(ast.Dimensions) == 0 {
		return ""
	}
	return strings.Join(ast.Dimensions, ", ")
}

func (b *baseSQLBuilder) buildOrderByClause(ast *QueryAST) string {
	if ast.Sort == nil {
		return ""
	}
	return fmt.Sprintf("ORDER BY %s %s", ast.Sort.FieldExpr, normalizeSortOrder(ast.Sort.Order))
}

func (b *baseSQLBuilder) buildLimitClause(ast *QueryAST) string {
	if ast.Pagination == nil {
		return ""
	}
	offset := (ast.Pagination.Page - 1) * ast.Pagination.PageSize
	return fmt.Sprintf("LIMIT %d OFFSET %d", ast.Pagination.PageSize, offset)
}

type mysqlBuilder struct {
	baseSQLBuilder
}

func NewMySQLBuilder() SQLBuilder {
	return &mysqlBuilder{}
}

func (b *mysqlBuilder) BuildSelect(ast *QueryAST) string {
	var sb strings.Builder

	sb.WriteString("SELECT ")
	sb.WriteString(b.buildSelectClause(ast))
	sb.WriteString(" ")
	sb.WriteString(b.buildFromClause(ast))

	where := b.buildWhereClause(ast)
	if where != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(where)
	}

	groupBy := b.buildGroupByClause(ast)
	if groupBy != "" {
		sb.WriteString(" GROUP BY ")
		sb.WriteString(groupBy)
	}

	orderBy := b.buildOrderByClause(ast)
	if orderBy != "" {
		sb.WriteString(" ")
		sb.WriteString(orderBy)
	}

	limit := b.buildLimitClause(ast)
	if limit != "" {
		sb.WriteString(" ")
		sb.WriteString(limit)
	}

	return sb.String()
}

func (b *mysqlBuilder) BuildCount(ast *QueryAST) string {
	var sb strings.Builder

	sb.WriteString("SELECT COUNT(*) AS _total ")
	sb.WriteString(b.buildFromClause(ast))

	where := b.buildWhereClause(ast)
	if where != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(where)
	}

	groupBy := b.buildGroupByClause(ast)
	if groupBy != "" {
		sb.WriteString(" GROUP BY ")
		sb.WriteString(groupBy)
	}

	return sb.String()
}

func (b *mysqlBuilder) Dialect() DialectType {
	return DialectMySQL
}

type postgresBuilder struct {
	baseSQLBuilder
}

func NewPostgreSQLBuilder() SQLBuilder {
	return &postgresBuilder{}
}

func (b *postgresBuilder) BuildSelect(ast *QueryAST) string {
	return (&mysqlBuilder{}).BuildSelect(ast)
}

func (b *postgresBuilder) BuildCount(ast *QueryAST) string {
	return (&mysqlBuilder{}).BuildCount(ast)
}

func (b *postgresBuilder) Dialect() DialectType {
	return DialectPostgreSQL
}

type clickhouseBuilder struct {
	baseSQLBuilder
}

func NewClickHouseBuilder() SQLBuilder {
	return &clickhouseBuilder{}
}

func (b *clickhouseBuilder) BuildSelect(ast *QueryAST) string {
	var sb strings.Builder

	sb.WriteString("SELECT ")
	sb.WriteString(b.buildSelectClause(ast))
	sb.WriteString(" ")
	sb.WriteString(b.buildFromClause(ast))

	where := b.buildWhereClause(ast)
	if where != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(where)
	}

	groupBy := b.buildGroupByClause(ast)
	if groupBy != "" {
		sb.WriteString(" GROUP BY ")
		sb.WriteString(groupBy)
	}

	orderBy := b.buildOrderByClause(ast)
	if orderBy != "" {
		sb.WriteString(" ")
		sb.WriteString(orderBy)
	}

	limit := b.buildLimitClause(ast)
	if limit != "" {
		sb.WriteString(" ")
		sb.WriteString(limit)
	}

	return sb.String()
}

func (b *clickhouseBuilder) BuildCount(ast *QueryAST) string {
	var sb strings.Builder

	sb.WriteString("SELECT count() AS _total ")
	sb.WriteString(b.buildFromClause(ast))

	where := b.buildWhereClause(ast)
	if where != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(where)
	}

	groupBy := b.buildGroupByClause(ast)
	if groupBy != "" {
		sb.WriteString(" GROUP BY ")
		sb.WriteString(groupBy)
	}

	return sb.String()
}

func (b *clickhouseBuilder) Dialect() DialectType {
	return DialectClickHouse
}

func NewSQLBuilder(dialect DialectType) SQLBuilder {
	switch dialect {
	case DialectMySQL:
		return NewMySQLBuilder()
	case DialectPostgreSQL:
		return NewPostgreSQLBuilder()
	case DialectClickHouse:
		return NewClickHouseBuilder()
	default:
		return NewMySQLBuilder()
	}
}

func ParseDialect(s string) DialectType {
	switch strings.ToLower(s) {
	case "mysql", "starrocks":
		return DialectMySQL
	case "postgresql", "postgres":
		return DialectPostgreSQL
	case "clickhouse":
		return DialectClickHouse
	default:
		return DialectMySQL
	}
}

func BuildQueryString(dialect DialectType, ast *QueryAST) (string, string) {
	builder := NewSQLBuilder(dialect)
	selectSQL := builder.BuildSelect(ast)
	countSQL := builder.BuildCount(ast)
	return selectSQL, countSQL
}

// BuildQueryStringWithBun 构建带占位符的 SQL 并返回参数化 args。
// 值经 args 传给驱动，不落入 SQL 文本；sanitizer 仅作为旧路径兜底。
func BuildQueryStringWithBun(dialect DialectType, ast *QueryAST) (string, string, []any) {
	qb := NewBunSQLBuilder(dialect)
	selectSQL, selectArgs := qb.BuildSelect(ast)
	countSQL, countArgs := qb.BuildCount(ast)
	_ = countArgs // count 与 select 的 filter args 相同
	return selectSQL, countSQL, selectArgs
}
