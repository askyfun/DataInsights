package query

import (
	"fmt"
	"strings"

	"data-insights/internal/domain/entity"
)

// DatasetQueryLimitMax 是「维度去重取数」的服务端唯一值硬上限。
//
// 该端点的用途是给过滤器枚举模式取候选值（前端每次最多展示这么多项），
// 因此 limit 在这里是**去重后的行数**上限，而不是源表取样行数：
// 客户端传多大的 limit 都不会让响应超过这个值。
const DatasetQueryLimitMax = 1000

// BuildDatasetDistinctQuery 按 entity.QueryConfig 构造「维度去重取数」SQL。
//
// 调用场景：POST /api/datasets/:id/query（datasetService.Query）。该端点只服务
// 「取一组维度字段的去重组合」——前端唯一调用方是过滤弹窗枚举模式的
// `datasetsApi.queryDistinct`。契约因此是：
//   - dimension_groups 里的字段全部展平进 SELECT + GROUP BY，0 指标即 SQL 语义上的 DISTINCT；
//   - filters / sort / limit 与图表查询同一套语义，过滤值经返回的 args 参数化下发；
//   - 字段引用走与图表查询同一套列索引解析（列 ID 权威、历史列名形态次级回落）；
//   - limit 收敛到 [1, DatasetQueryLimitMax]，<=0 视为上限。
//
// metric_groups 非空时显式报错而不是静默丢弃：entity.QueryConfig 的 FieldGroup 不携带
// 聚合配置，任何带指标的请求都无法被正确执行，而「忽略请求参数、退回去取全表」正是本
// 函数取代的旧行为（issue #110）。
func BuildDatasetDistinctQuery(
	dialect DialectType,
	columnsJSON string,
	source string,
	sourceType SourceType,
	config entity.QueryConfig,
) (string, []any, error) {
	if len(config.MetricGroups) > 0 {
		return "", nil, fmt.Errorf("dataset query does not support metric_groups: QueryConfig carries no aggregation")
	}
	if source == "" {
		return "", nil, fmt.Errorf("dataset has no valid query_sql or table_name")
	}

	dims := make([]DimensionExpr, 0)
	for _, group := range config.DimensionGroups {
		for _, field := range group.Fields {
			if field != "" {
				dims = append(dims, DimensionExpr{Field: field})
			}
		}
	}
	if len(dims) == 0 {
		return "", nil, fmt.Errorf("dataset query requires at least one dimension field")
	}

	limit := config.Limit
	if limit <= 0 || limit > DatasetQueryLimitMax {
		limit = DatasetQueryLimitMax
	}

	spec := &QuerySpec{Dimensions: dims, Limit: limit}
	for _, f := range config.Filters {
		spec.Filters = append(spec.Filters, FilterConfig{
			Field:    f.Field,
			Op:       FilterOperator(strings.TrimSpace(f.Operator)),
			Value:    f.Value,
			ValueEnd: f.ValueEnd,
			Logic:    f.Logic,
		})
	}
	if config.Sort != nil {
		spec.Sort = &SortConfig{Field: config.Sort.Field, Order: config.Sort.Order}
	}

	qb := NewQueryBuilder()
	if err := qb.WithColumnMappings(columnsJSON); err != nil {
		return "", nil, fmt.Errorf("invalid dataset columns: %w", err)
	}

	ast := NewQueryPlanner().PlanAST(source, sourceType, spec)
	ast.ApplyColumnIndex(qb.columns)

	sql, _, args := BuildQueryStringWithBun(dialect, ast)
	return sql, args, nil
}
