package query

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type ColumnInfo struct {
	Name string `json:"name"`
	Expr string `json:"expr"`
	Role string `json:"role"`
	Type string `json:"type"`
}

type QueryAST struct {
	Source         string
	SourceType     SourceType
	Dimensions     []string
	DimensionExprs []DimensionExprAST
	Metrics        []MetricExpr
	MetricExprs    []MetricPlanExpr
	Filters        []FilterExpr
	Sort           *SortExpr
	Pagination     *Pagination
	Limit          int
	ColumnMappings map[string]string
}

// DimensionExprAST 表示规划后可直接参与 SQL 生成的维度表达式。
// 调用场景：QueryPlanner 将 QuerySpec 中的结构化维度映射为 AST 节点，供不同 SQL builder 生成方言表达式。
type DimensionExprAST struct {
	Field       string
	FieldExpr   string
	Alias       string
	Label       string
	Granularity string
	Bucket      string
	Format      string
}

// MetricPlanExpr 表示规划后保留展示语义的指标表达式。
// 调用场景：在保留旧 MetricExpr 兼容路径的同时，为后续 QueryAST 能力扩展保存单位、格式等结构化信息。
type MetricPlanExpr struct {
	Field     string
	FieldExpr string
	Agg       AggregationType
	Alias     string
	Label     string
	Unit      string
	Format    string
	Cumulative bool
	PercentOfTotal bool
}

type SourceType int

const (
	SourceTypeTable SourceType = iota
	SourceTypeSQL
)

type MetricExpr struct {
	Field     string
	FieldExpr string
	Agg       AggregationType
	Alias     string
	IsAgg     bool
}

type FilterExpr struct {
	Field     string
	FieldExpr string
	Op        FilterOperator
	Value     interface{}
	ValueEnd  interface{}
	Logic     string
}

type SortExpr struct {
	Field     string
	FieldExpr string
	Order     string
}

// granularityTokenPattern 限制时间粒度为纯小写字母/下划线 token。
// 粒度值来自请求 JSON 且会被拼入 SQL 表达式（如 DATE_TRUNC('%s', ...)），必须先过白名单形态。
var granularityTokenPattern = regexp.MustCompile(`^[a-z_]+$`)

// ValidateGranularity 校验 AST 中带时间粒度的维度在目标方言下可被正确渲染。
// 调用场景：executor 在生成 SQL 前调用，使 MySQL/ClickHouse 下不支持的粒度
// （如 week/month）显式报错，而不是静默降级为原始字段并产出错误的分组结果；
// 同时拒绝含引号/括号等注入字符的粒度值。
func (q *QueryAST) ValidateGranularity(dialect DialectType) error {
	for _, dim := range q.DimensionExprs {
		if dim.Granularity == "" {
			continue
		}
		if !granularityTokenPattern.MatchString(dim.Granularity) {
			return fmt.Errorf("invalid time granularity: %q", dim.Granularity)
		}
		switch dialect {
		case DialectPostgreSQL:
			// DATE_TRUNC 支持任意标准粒度，由数据库校验具体值。
		case DialectMySQL, DialectClickHouse:
			if dim.Granularity != "day" {
				return fmt.Errorf("time granularity %q is not supported on dialect %s: only \"day\" is supported", dim.Granularity, dialect)
			}
		default:
			if dim.Granularity != "day" {
				return fmt.Errorf("time granularity %q is not supported on dialect %s: only \"day\" is supported", dim.Granularity, dialect)
			}
		}
	}
	return nil
}

// ApplyColumnMappings 将列映射回填到已规划的 AST 上。
// 调用场景：service 层先生成 PlannedAST，executor 拿到 dataset columns 后再把真实字段表达式补齐到 AST。
// 主要逻辑：同步刷新维度、指标、过滤和排序表达式；若指标表达式本身已聚合，标记 IsAgg 避免重复包裹聚合函数。
func (q *QueryAST) ApplyColumnMappings(columnMappings map[string]string) {
	q.ColumnMappings = columnMappings

	for i := range q.DimensionExprs {
		q.DimensionExprs[i].FieldExpr = q.GetDimFieldExpr(q.DimensionExprs[i].Field)
	}

	for i := range q.Metrics {
		q.Metrics[i].FieldExpr = q.GetMetricFieldExpr(q.Metrics[i].Field)
		q.Metrics[i].IsAgg = isAggregateFunction(q.Metrics[i].FieldExpr)
	}

	for i := range q.MetricExprs {
		q.MetricExprs[i].FieldExpr = q.GetMetricFieldExpr(q.MetricExprs[i].Field)
	}

	for i := range q.Filters {
		q.Filters[i].FieldExpr = q.GetFilterFieldExpr(q.Filters[i].Field)
	}

	if q.Sort != nil {
		q.Sort.FieldExpr = q.GetSortFieldExpr(q.Sort.Field)
	}
}

func (q *QueryAST) GetMetricFieldExpr(field string) string {
	if expr, ok := q.ColumnMappings[field]; ok && expr != "" {
		return expr
	}
	return field
}

func (q *QueryAST) GetDimFieldExpr(field string) string {
	if expr, ok := q.ColumnMappings[field]; ok && expr != "" {
		return expr
	}
	return field
}

func (q *QueryAST) GetFilterFieldExpr(field string) string {
	if expr, ok := q.ColumnMappings[field]; ok && expr != "" {
		return expr
	}
	return field
}

func (q *QueryAST) GetSortFieldExpr(field string) string {
	if expr, ok := q.ColumnMappings[field]; ok && expr != "" {
		return expr
	}
	return field
}

type QueryBuilder struct {
	columnMappings map[string]string
}

func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		columnMappings: make(map[string]string),
	}
}

func (qb *QueryBuilder) WithColumnMappings(columns string) error {
	if columns == "" {
		return nil
	}

	var cols []ColumnInfo
	if err := json.Unmarshal([]byte(columns), &cols); err != nil {
		return err
	}

	for _, col := range cols {
		if col.Expr != "" && col.Name != "" {
			qb.columnMappings[col.Name] = col.Expr
		}
	}
	return nil
}

func (qb *QueryBuilder) Build(
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

	if sort != nil {
		ast.Sort = &SortExpr{
			Field:     sort.Field,
			FieldExpr: qb.getFieldExpr(sort.Field),
			Order:     sort.Order,
		}
	}

	return ast
}

func (qb *QueryBuilder) getFieldExpr(field string) string {
	if expr, ok := qb.columnMappings[field]; ok && expr != "" {
		return expr
	}
	return field
}

func isAggregateFunction(expr string) bool {
	upper := strings.ToUpper(expr)
	return strings.HasPrefix(upper, "COUNT") ||
		strings.HasPrefix(upper, "SUM") ||
		strings.HasPrefix(upper, "AVG") ||
		strings.HasPrefix(upper, "MIN") ||
		strings.HasPrefix(upper, "MAX") ||
		strings.HasPrefix(upper, "GROUP_CONCAT") ||
		strings.HasPrefix(upper, "JSON_ARRAYAGG") ||
		strings.HasPrefix(upper, "JSON_OBJECTAGG")
}
