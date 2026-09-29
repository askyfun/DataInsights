package query

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
)

// Top N（issue #130，#116 epic 第三步）。
//
// 契约：query_options.top_n = {limit: N, metric?: 列ID, order?: asc|desc}
// （扩展袋，与 histogram 的 bin_count / 同环比的 comparison 同一条链路：前端
// composeChartQueryRequest 写 wire、持久化走 queryOptions 小节、后端
// chartDataQueryFromConfig 透传，executor 从请求直读——不进 QuerySpec/AST/planner）。
//
// 语义：把「按某指标取前 N 个维度值」翻译进查询计划本身——AST 上设 Sort（目标指标
// 的输出别名）+ Limit，由数据库完成排序截断（不是取回全量再在 Go 端切，那样
// 计数/其他图型的行数都是错的）。order 缺省 desc（「取最大的 N 个」是默认心智）。
//
// merge_other（issue #116 验收行「其余合并为其他」）：可选布尔，打开后由 executor
// 再发一条全量分组汇总（见 BuildTopNRestQuery），把被截断的那些取值压成结果末尾
// 一行 TopNOtherLabel。只有**全部指标可加**时才允许（见 topNAdditiveMetrics），
// 否则显式报错——AVG / COUNT(DISTINCT) / MIN / MAX 的「其他」值无法从「全量 − 前 N」
// 反推，硬给一个数就是编造。wire 是 snake_case merge_other，持久化文档小节的
// camelCase mergeOther 一并接受（与下方 topN/top_n 双写同口径：归一只发生在
// queryOptions 顶层，嵌套键没人管）。
//
// 已知边界：请求自带分页时（表格等）SQL 会同时有 LIMIT/OFFSET 与 Top N 的 LIMIT，
// 后者优先——Top N 场景下分页器语义不适用，卡片只在轴类/饼图开放。merge_other
// 继承同一条边界（它挂在 top_n 上，不另开门）。

// TopNOtherLabel 是「其余合并为其他」追加行的维度取值（后端固定，保证 builder 预览 /
// 分享页 / 仪表盘三个渲染面用的是同一个词，与 TableTotalLabel 同约定）。
const TopNOtherLabel = "其他"

// topNConfig 是 query_options.top_n 校验后的形态。
type topNConfig struct {
	Limit      int
	Metric     string // 排名指标的列标识（wire 上是列 ID，空则取首指标）
	Order      string // asc | desc（已归一，缺省 desc）
	MergeOther bool   // 其余取值合并为一行 TopNOtherLabel
}

// parseTopN 从查询选项扩展袋里解析 Top N 配置。返回 nil 表示未启用
// （缺键/类型不对/limit 非法），调用方按「不截断」路径原样执行。
func parseTopN(opts map[string]any) *topNConfig {
	raw, ok := opts["top_n"]
	if !ok {
		// 持久化文档的 camelCase 未经归一时也接受（与 normalizePersistedQueryOptions
		// 双保险，避免「保存后分享页悄悄不生效」）。
		raw, ok = opts["topN"]
		if !ok {
			return nil
		}
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	limit, ok := toPositiveInt(m["limit"])
	if !ok || limit > maxTopNLimits {
		return nil
	}
	order := "desc"
	if o, ok := m["order"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(o)) {
		case "asc":
			order = "asc"
		case "desc", "":
			order = "desc"
		default:
			return nil
		}
	}
	metric := ""
	if f, ok := m["metric"].(string); ok {
		metric = strings.TrimSpace(f)
	}
	return &topNConfig{Limit: limit, Metric: metric, Order: order, MergeOther: parseTopNMergeOther(m)}
}

// parseTopNMergeOther 读取 top_n 小节里的「其余合并为其他」开关。真值形态与
// tableTotalOptions 一致（JSON 布尔 true，或数值 1），其余形态一律关闭；
// snake_case（wire）与 camelCase（持久化文档未经归一时）两个键都认。
func parseTopNMergeOther(m map[string]any) bool {
	for _, key := range []string{"merge_other", "mergeOther"} {
		if v, ok := m[key].(bool); ok {
			return v
		}
		if f, ok := toFloat64(m[key]); ok && f == 1 {
			return true
		}
	}
	return false
}

// topNAdditiveMetrics 判「这些指标的聚合值能否由全量减去前 N 得到」。只有 SUM 与
// COUNT 成立：二者对分组求和与对明细行求和同值，所以「其余」= 全量 − Σ(前 N)。
// AVG 会变成「平均数的平均数」、COUNT(DISTINCT) 会变成「跨组去重后求和」、
// MIN/MAX 的剩余段值与全量值没有可反推的关系；IsAgg 指标（FieldExpr 已是完整聚合
// 表达式，来自列映射）形态不可知，一并按不可加处理——宁可报错也不给一个错的数。
func topNAdditiveMetrics(metrics []MetricExpr) bool {
	if len(metrics) == 0 {
		return false
	}
	for _, m := range metrics {
		if m.IsAgg {
			return false
		}
		switch m.Agg {
		case AggSum, AggCount:
		default:
			return false
		}
	}
	return true
}

// toPositiveInt 把 JSON 解出来的数值（float64/int/字符串）归一成正整数；
// 非正数或非数值返回 ok=false。
func toPositiveInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if n >= 1 && n == math.Trunc(n) {
			return int(n), true
		}
	case int:
		if n >= 1 {
			return n, true
		}
	case int64:
		if n >= 1 {
			return int(n), true
		}
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
			return toPositiveInt(f)
		}
	}
	return 0, false
}

// maxTopNLimits 是 Top N 的荒谬值钳位（与 histogram 的 bin 钳位同理：float 域
// 钳制，防 int 转换溢出）。
const maxTopNLimits = 10000

// applyTopN 把 Top N 翻译进已规划的 AST：Sort 指向目标指标的输出别名
// （与用户手选排序共用一条渲染链路，resolveSortAlias 按 BindingID/别名解析），
// Limit 交给 builder 渲染。指标解析不到（列 ID 与别名都不匹配）时显式报错，
// 不静默退化成「首指标」——那会让用户看到一份悄悄换了排名依据的图。
func applyTopN(ast *QueryAST, cfg *topNConfig) error {
	if len(ast.Metrics) == 0 {
		return fmt.Errorf("top_n 需要至少一个指标")
	}
	if cfg.MergeOther && !topNAdditiveMetrics(ast.Metrics) {
		return fmt.Errorf("top_n.merge_other 只支持可加指标（sum/count）：当前指标的聚合值无法从「全量 − 前 N」得到")
	}
	field := cfg.Metric
	if field == "" {
		field = ast.Metrics[0].Field
	}
	for i := range ast.Metrics {
		m := &ast.Metrics[i]
		if m.Field == field || m.Alias == field {
			ast.Sort = &SortExpr{Field: m.Alias, Order: cfg.Order}
			ast.Limit = cfg.Limit
			return nil
		}
	}
	return fmt.Errorf("top_n: 找不到排名指标 %q", field)
}

// topNMergeOtherChartTypes 是「其余合并为其他」放行的图型，与前端 TOPN_CHART_TYPES
// 逐字一致（ChartBuilder.tsx）。轴类与饼图之外没有「把剩余压成一行」的读法：交叉表
// 有自己的合计、直方图/箱线图走各自的多次查询编排、表格的分页语义与 Top N 冲突。
var topNMergeOtherChartTypes = map[ChartType]bool{
	ChartTypeBar:  true,
	ChartTypeLine: true,
	ChartTypeArea: true,
	ChartTypePie:  true,
}

// guardTopNMergeOther 在发第一条 SQL 之前挡住「配了 merge_other 但算不出可信值」的
// 形状：图型不在放行清单、或维度不是恰好一个（多个维度没法压成一个标签值，单个
// 维度也没有可贴标签的列）。与 attachComparison 的前置门同款——错误里有实际值，
// 不静默退化成「不合并」。
func guardTopNMergeOther(chartType ChartType, ast *QueryAST) error {
	if !topNMergeOtherChartTypes[chartType] {
		return fmt.Errorf("top_n.merge_other 仅支持 bar/line/area/pie 图型（当前 %s）", chartType)
	}
	if n := len(ast.Dimensions); n != 1 {
		return fmt.Errorf("top_n.merge_other 要求恰好一个维度，当前 %d 个", n)
	}
	return nil
}

// appendTopNOther 把被 Top N 截断的那些取值合并成结果末尾一行「其他」。
//
// 取数：主查询已经只回前 N 行（数据库侧截断），所以另发一条 BuildTopNRestQuery
// 拿「全量分组数 + 各指标在全量上的可加重聚合」，Go 端做
// 其他值 = 全量值 − Σ(前 N 行值)。可加性已由 applyTopN 门挡过，这条恒等式对
// SUM/COUNT 成立。
//
// 不发这条查询的情形：主查询返回的行数没到 N，说明后面根本没有剩余。
// 返回原样 rows 的情形：全量分组数 == 已返回行数（恰好 N 组，没有剩余）。
//
// 维度列的键取自实际返回的行（不属于任何指标别名的那一列），而不是照抄 builder
// 的别名渲染规则——时间粒度维度、v2 槽位协议的输出名各有形态，以驱动真正给出的
// 键为准最不会错，并且顺带验证了「行里确实只有一个非指标列」。
//
// 失败口径：汇总查询报错、或某个指标的全量值是 NULL（该列全空，减法无从谈起）时
// 显式报错，不给 0 或空值——「其他」是一个数，缺一个数比给一个错的数体面，但给
// 错的数是事故。
func (e *Executor) appendTopNOther(
	ctx context.Context, dialect DialectType, ast *QueryAST, rows []map[string]any,
) ([]map[string]any, error) {
	if ast.Limit <= 0 || len(rows) < ast.Limit {
		return rows, nil // 没截断，也就没有「其余」
	}

	dimKey, err := topNRowDimKey(rows[0], ast.Metrics)
	if err != nil {
		return nil, err
	}

	restSQL, restArgs := BuildTopNRestQuery(dialect, ast)
	if restSQL == "" {
		return rows, nil // 无指标：applyTopN 已挡，这里只是防御性不建 SQL
	}
	slog.Debug("executing top_n rest query", "sql", restSQL, "args", restArgs)
	rest, err := e.conn.Execute(ctx, restSQL, restArgs...)
	if err != nil {
		return nil, fmt.Errorf("top_n rest query failed: %v", err)
	}
	if len(rest.Rows) == 0 {
		return rows, nil
	}
	groups, ok := toFloat64(rest.Rows[0][topNGroupsAlias])
	if !ok {
		return nil, fmt.Errorf("top_n rest: 分组数不是数值: %v", rest.Rows[0][topNGroupsAlias])
	}
	if int(groups) <= len(rows) {
		return rows, nil // 恰好 N 组：剩余为空
	}

	other := map[string]any{dimKey: TopNOtherLabel}
	for _, metric := range ast.Metrics {
		total, ok := toFloat64(rest.Rows[0][metric.Alias])
		if !ok {
			return nil, fmt.Errorf("top_n rest: 指标 %q 的全量值不可计算（%v）", metric.Alias, rest.Rows[0][metric.Alias])
		}
		var shown float64
		for _, row := range rows {
			if v, ok := toFloat64(row[metric.Alias]); ok {
				shown += v
			}
		}
		other[metric.Alias] = total - shown
	}
	return append(rows, other), nil
}

// topNRowDimKey 从一行结果里认出维度列的键：指标别名之外的最后一列。恰好一个才
// 认，多于一个说明维度数与 guardTopNMergeOther 的判定不一致（例如处理器改写过行），
// 此时宁缺毋滥地报错，也不要把「其他」贴到某个指标列上。
func topNRowDimKey(row map[string]any, metrics []MetricExpr) (string, error) {
	isMetric := make(map[string]bool, len(metrics))
	for _, m := range metrics {
		isMetric[m.Alias] = true
	}
	var dimKey string
	var candidates int
	for key := range row {
		if isMetric[key] {
			continue
		}
		candidates++
		dimKey = key
	}
	if candidates != 1 {
		return "", fmt.Errorf("top_n.merge_other 需要行内恰好一个维度列，实际 %d 个", candidates)
	}
	return dimKey, nil
}
