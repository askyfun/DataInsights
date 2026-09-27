package query

import (
	"fmt"
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
// 已知边界：请求自带分页时（表格等）SQL 会同时有 LIMIT/OFFSET 与 Top N 的 LIMIT，
// 后者优先——Top N 场景下分页器语义不适用，卡片只在轴类/饼图开放。

// topNConfig 是 query_options.top_n 校验后的形态。
type topNConfig struct {
	Limit  int
	Metric string // 排名指标的列标识（wire 上是列 ID，空则取首指标）
	Order  string // asc | desc（已归一，缺省 desc）
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
	return &topNConfig{Limit: limit, Metric: metric, Order: order}
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
