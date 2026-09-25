package query

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

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
	// ColumnMappings 列 ID → SQL 表达式（权威解析入口）。
	ColumnMappings map[string]string
	// LegacyColumnMappings 列名 → SQL 表达式（过渡态次级索引）。
	// 部署前保存的历史配置在 wire 与 config 里引用的仍是列名；虚拟字段（expr 是
	// 表达式）必须靠这张表才能解析正确，否则列名会被当裸标识符写进 SQL。
	// 一次性反写迁移跑完后不会再被命中。
	LegacyColumnMappings map[string]string
	// ColumnNames 列 ID → 展示名。SQL 输出别名用列名而非列 ID（负载人读友好），
	// 由 ApplyColumnIndex 填充；缺失时别名保持字段标识本身（既有行为）。
	ColumnNames map[string]string
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
	GroupName   string // 来源槽位/组名（v2 协议；v1 路径为空）
	BindingID   string // 来源绑定实例标识（v2 协议；v1 路径为空）
}

// MetricPlanExpr 表示规划后保留展示语义的指标表达式。
// 调用场景：在保留旧 MetricExpr 兼容路径的同时，为后续 QueryAST 能力扩展保存单位、格式等结构化信息。
type MetricPlanExpr struct {
	Field          string
	FieldExpr      string
	Agg            AggregationType
	Alias          string
	Label          string
	Unit           string
	Format         string
	Cumulative     bool
	PercentOfTotal bool
	GroupName      string // 来源槽位/组名（v2 协议；v1 路径为空）
	BindingID      string // 来源绑定实例标识（v2 协议；v1 路径为空）
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
// 本入口只接受 id→expr 一张表（列名不可用），别名解析不可用——供测试与仅需表达式的调用方使用；
// 生产路径走 ApplyColumnIndex。
func (q *QueryAST) ApplyColumnMappings(columnMappings map[string]string) {
	q.ApplyColumnIndex(columnIndex{byID: columnMappings, nameByID: map[string]string{}, byName: map[string]string{}})
}

// ApplyColumnIndex 用完整列索引回填已规划的 AST：表达式按列 ID 解析（次级回落
// 列名），默认输出别名由列 ID 换成列名（展示层据此保持人读）。缺名时别名保持原值，
// 行为与改动前一致。
func (q *QueryAST) ApplyColumnIndex(idx columnIndex) {
	q.ColumnMappings = idx.byID
	q.LegacyColumnMappings = idx.byName
	q.ColumnNames = idx.nameByID

	for i := range q.DimensionExprs {
		dim := &q.DimensionExprs[i]
		dim.FieldExpr = q.resolveFieldExpr(dim.Field, true)
		dim.Alias = idx.displayAlias(dim.Field, dim.Alias, dim.Granularity)
	}

	for i := range q.Metrics {
		metric := &q.Metrics[i]
		metric.FieldExpr = q.resolveFieldExpr(metric.Field, true)
		metric.IsAgg = isAggregateFunction(metric.FieldExpr)
		metric.Alias = idx.displayAlias(metric.Field, metric.Alias, "")
	}

	for i := range q.MetricExprs {
		plan := &q.MetricExprs[i]
		plan.FieldExpr = q.resolveFieldExpr(plan.Field, true)
		plan.Alias = idx.displayAlias(plan.Field, plan.Alias, "")
	}

	for i := range q.Filters {
		q.Filters[i].FieldExpr = q.resolveFieldExpr(q.Filters[i].Field, true)
	}

	if q.Sort != nil {
		// 排序键在 wire 上就是输出别名（见 buildQuerySpecFromEntityRequestV2 的
		// sort 透传），未命中列 ID 属正常情况，不告警。
		q.Sort.FieldExpr = q.resolveFieldExpr(q.Sort.Field, false)
	}
}

// resolveFieldExpr 解析字段标识为 SQL 表达式：列 ID → 列名（历史引用）→ 裸名兜底。
// warnOnMiss 为 true 时只有「两级都落空」才记警告——维度/指标/过滤在 wire 上恒为
// 列 ID 或历史列名，两者都匹配不上说明配置引用的列已被删除，是需要暴露的信号；
// 排序键在 wire 上是输出别名，不是字段引用，故传 false 静默。
func (q *QueryAST) resolveFieldExpr(field string, warnOnMiss bool) string {
	if field == "" {
		return field
	}
	if expr, ok := q.ColumnMappings[field]; ok && expr != "" {
		return expr
	}
	if expr, ok := q.LegacyColumnMappings[field]; ok && expr != "" {
		return expr
	}
	if warnOnMiss {
		slog.Warn("query field matches no dataset column id or name; falling back to raw identifier",
			"field", field)
	}
	return field
}

// displayAlias 把「默认等于字段标识」的输出别名换序列的展示名。
// 用户显式指定过别名（与默认值不同）时原样保留；解析不到列名时也原样保留。
func (idx columnIndex) displayAlias(field, alias, granularity string) string {
	name, ok := idx.displayName(field)
	if !ok {
		return alias
	}
	defaultAlias := field
	if granularity != "" {
		defaultAlias = field + "_" + granularity
	}
	if alias != defaultAlias {
		return alias
	}
	if granularity != "" {
		return name + "_" + granularity
	}
	return name
}

func (q *QueryAST) GetMetricFieldExpr(field string) string {
	return q.resolveFieldExpr(field, false)
}

func (q *QueryAST) GetDimFieldExpr(field string) string {
	return q.resolveFieldExpr(field, false)
}

func (q *QueryAST) GetFilterFieldExpr(field string) string {
	return q.resolveFieldExpr(field, false)
}

func (q *QueryAST) GetSortFieldExpr(field string) string {
	return q.resolveFieldExpr(field, false)
}

type QueryBuilder struct {
	columns columnIndex
}

func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		columns: newColumnIndex(),
	}
}

func (qb *QueryBuilder) WithColumnMappings(columns string) error {
	idx, err := buildColumnIndex(columns)
	if err != nil {
		return err
	}
	qb.columns = idx
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
		ColumnMappings:       qb.columns.byID,
		LegacyColumnMappings: qb.columns.byName,
		ColumnNames:          qb.columns.nameByID,
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
			Alias:     qb.columns.displayAlias(m.Field, m.ResolveAlias(), ""),
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
	expr, _ := qb.columns.expr(field)
	return expr
}

// indexes 暴露列索引，供 executor 在构建 AST 后做展示名翻译与负载键对齐。
func (qb *QueryBuilder) indexes() columnIndex {
	return qb.columns
}

// isAggregateFunction 判断字段表达式本身是否已是聚合形态（来自列映射，如
// "SUM(amount)"）。必须匹配"函数名 + ("，不能用 HasPrefix：min_price、
// summary_count 这类普通列名会以 min/sum 开头，前缀匹配会误判为聚合表达式，
// 导致 renderMetricSelect 跳过聚合、生成裸列 SQL（scatter/无维度图型直接报错）。
var aggregateFuncPattern = regexp.MustCompile(`^(COUNT|SUM|AVG|MIN|MAX|GROUP_CONCAT|JSON_ARRAYAGG|JSON_OBJECTAGG)\s*\(`)

func isAggregateFunction(expr string) bool {
	return aggregateFuncPattern.MatchString(strings.TrimSpace(strings.ToUpper(expr)))
}
