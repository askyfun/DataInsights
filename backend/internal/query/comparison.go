package query

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"
)

// 同环比分析配置（issue #129，#116 epic 第二步）。
//
// 契约：query_options.comparison = {type: mom|yoy, field?: 列ID}（扩展袋，与 histogram 的
// bin_count 同一条链路：前端 composeChartQueryRequest 写入 wire、持久化走 queryOptions 小节、
// 后端 chartDataQueryFromConfig 透传，executor 从请求直读——不进 QuerySpec/AST/planner）。
//
// 语义（窗口平移 + 日历对齐，双端注释共读此段）：
//   - 当前窗口 [lo, hi] 取自请求中对比日期列上的边界条件（between / gte / lte；
//     日期筛选意图已在两侧物化成具体日期后到达这里，所以两端行为一致）；
//     单侧缺失时按本次结果的日期轴补齐。
//   - 环比 mom：整窗前移 span+1 天（span = hi-lo 的整天数），上一等长周期；
//   - 同比 yoy：窗口与对齐均前移一个日历年（AddDate(-1,0,0)，2-29 归一到 2-28/3-1）。
//   - 基线查询与当前查询同 AST（维度/指标/其余过滤不动），仅替换对比列的边界条件、
//     去分页去排序；基线桶值按 +Δ 平移回当前桶**按键对齐**（缺数桶得 null，不串行）。
//
// 落地范围：bar/line/area（AxisResponse 追加「(上期)」「(增长率%)」系列）与
// table（追加同名两列）。增长率 =（当前-上期)/|上期|×100，上期缺失或为 0 → null。

// dateLayoutDay / dateLayoutSec 与 processor.toString 的日期字符串化口径一致，
// 是「x 轴桶键 ↔ 过滤边界」互转的唯一格式。
const (
	dateLayoutDay = "2006-01-02"
	dateLayoutSec = "2006-01-02 15:04:05"
)

// comparisonConfig 是 query_options.comparison 校验后的形态。
type comparisonConfig struct {
	// Type 为 "mom" 或 "yoy"（parseComparison 已归一为小写并校验）。
	Type string
	// Field 是对比日期列标识（wire 上是列 ID，可空——空则取首维度）。
	Field string
}

// parseComparison 从查询选项扩展袋里解析同环比配置。返回 nil 表示未启用
// （缺键/类型不对/字段非法），调用方按「不产比」路径原样返回数据。
func parseComparison(opts map[string]any) *comparisonConfig {
	raw, ok := opts["comparison"]
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	typ, ok := m["type"].(string)
	if !ok {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "mom", "yoy":
	default:
		slog.Warn("query comparison: unknown type ignored", "type", typ)
		return nil
	}
	field := ""
	if f, ok := m["field"].(string); ok {
		field = strings.TrimSpace(f)
	}
	return &comparisonConfig{Type: strings.ToLower(strings.TrimSpace(typ)), Field: field}
}

// parseBucketTime 把桶键/边界值解析为日期（按本地时区，与 datefilter.go 的
// 「今天」口径一致）。支持 "2006-01-02"、"2006-01-02 15:04:05"、ISO T 分隔形态
// （截断到秒位）与 time.Time 直扫形态。
func parseBucketTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case string:
		s := strings.TrimSpace(t)
		if len(s) >= 19 && (s[10] == ' ' || s[10] == 'T') {
			s = s[:10] + " " + s[11:19]
			if dt, err := time.ParseInLocation(dateLayoutSec, s, time.Local); err == nil {
				return dt, true
			}
			s = s[:10]
		} else if len(s) >= 10 {
			s = s[:10]
		}
		if dt, err := time.ParseInLocation(dateLayoutDay, s, time.Local); err == nil {
			return dt, true
		}
	}
	return time.Time{}, false
}

// unshiftBucketKey 把基线桶键映射到它所服务的当前桶键（+Δ 天 / +一年）。
// 不可解析的键原样返回。Δ 与 sec 同源（buildComparisonPlan），保证与基线过滤
// 窗口的日界算术一致；yoy 只看 Δ 的符号、走日历年分支。
func unshiftBucketKey(key string, deltaDays int, yoy bool, sec bool) string {
	return offsetBucketKey(key, deltaDays, yoy, sec)
}

func offsetBucketKey(key string, dayDelta int, yoy bool, sec bool) string {
	t, ok := parseBucketTime(key)
	if !ok {
		return key
	}
	if yoy {
		years := 1
		if dayDelta < 0 {
			years = -1
		}
		t = t.AddDate(years, 0, 0)
	} else {
		t = t.AddDate(0, 0, dayDelta)
	}
	if sec {
		return t.Format(dateLayoutSec)
	}
	return t.Format(dateLayoutDay)
}

// windowBounds 从 AST 过滤条件里提取对比列的日期窗口边界：
// lower 取最大下界（gte/gt/between 前值/eq），upper 取最小上界（lte/lt/between 后值/eq）。
// between 与 eq 同时填充两侧。全部边界值不可解析时按缺失处理（宁缺勿错窗）。
func windowBounds(ast *QueryAST, fieldExpr string) (lo, hi time.Time, hasLo, hasHi bool) {
	betterLo := func(t time.Time) {
		if !hasLo || t.After(lo) {
			lo, hasLo = t, true
		}
	}
	betterHi := func(t time.Time) {
		if !hasHi || t.Before(hi) {
			hi, hasHi = t, true
		}
	}
	for i := range ast.Filters {
		f := &ast.Filters[i]
		FE := f.FieldExpr
		if FE == "" {
			FE = f.Field
		}
		if FE != fieldExpr {
			continue
		}
		switch f.Op {
		case FilterBetween:
			if t, ok := parseBucketTime(f.Value); ok {
				betterLo(t)
			}
			if t, ok := parseBucketTime(f.ValueEnd); ok {
				betterHi(t)
			}
		case FilterGte, FilterGt:
			if t, ok := parseBucketTime(f.Value); ok {
				betterLo(t)
			}
		case FilterLte, FilterLt:
			if t, ok := parseBucketTime(f.Value); ok {
				betterHi(t)
			}
		case FilterEq:
			if t, ok := parseBucketTime(f.Value); ok {
				betterLo(t)
				betterHi(t)
			}
		}
	}
	return
}

// comparisonPlan 是一次同环比取数所需的全部算术结果。
type comparisonPlan struct {
	FieldExpr string // 对比列 SQL 表达式（克隆基线 AST 时按它筛除旧边界条件）
	LowerKey  string // 基线窗口下界（格式化字符串，进 between 绑定参数）
	UpperKey  string // 基线上界
	Sec       bool   // 上界含非零时刻 → 边界按秒级渲染
	// DeltaDays / YoY：基线桶 → 当前桶的平移参数（与窗口平移同源；yoy 用日历年，
	// mom 用 span+1 天）。
	DeltaDays int
	YoY       bool
}

// buildComparisonPlan 计算基线窗口与平移参数。currentKeys 是本次结果的日期桶键
// （单侧边界缺失时用来补齐窗口）。
func buildComparisonPlan(cmp *comparisonConfig, ast *QueryAST, currentKeys []string) (*comparisonPlan, error) {
	// 对比列标识 → SQL 表达式：与 resolveFieldExpr 同一入口（列 ID 权威、列名过渡态兜底）。
	field := cmp.Field
	if field == "" && len(ast.Dimensions) > 0 {
		field = ast.Dimensions[0]
	}
	if field == "" {
		return nil, fmt.Errorf("comparison: no dimension to compare on")
	}
	fieldExpr := ast.GetDimFieldExpr(field)

	lo, hi, hasLo, hasHi := windowBounds(ast, fieldExpr)
	// 单侧缺失时按本次结果的日期轴补齐（日期芯片「最近 N 天」上界=昨天、下界开放等形态）。
	if (!hasLo || !hasHi) && len(currentKeys) > 0 {
		var minT, maxT time.Time
		for _, k := range currentKeys {
			if t, ok := parseBucketTime(k); ok {
				if minT.IsZero() || t.Before(minT) {
					minT = t
				}
				if maxT.IsZero() || t.After(maxT) {
					maxT = t
				}
			}
		}
		if !hasLo && !minT.IsZero() {
			lo, hasLo = minT, true
		}
		if !hasHi && !maxT.IsZero() {
			hi, hasHi = maxT, true
		}
	}
	if !hasLo || !hasHi {
		return nil, fmt.Errorf("comparison 需要日期维度上带上下界的日期筛选（如「最近 N 天」「本月」、固定区间），当前窗口无法闭合")
	}
	if hi.Before(lo) {
		return nil, fmt.Errorf("comparison: 日期窗口上下界颠倒")
	}

	spanDays := int(hi.Sub(lo).Hours() / 24)
	sec := hi.Hour() != 0 || hi.Minute() != 0 || hi.Second() != 0

	plan := &comparisonPlan{FieldExpr: fieldExpr, Sec: sec, YoY: cmp.Type == "yoy"}
	if plan.YoY {
		plan.LowerKey = lo.AddDate(-1, 0, 0).Format(dateLayoutDay)
		plan.UpperKey = hi.AddDate(-1, 0, 0).Format(dateLayoutDay)
		if sec {
			plan.UpperKey = hi.AddDate(-1, 0, 0).Format(dateLayoutSec)
		}
		plan.DeltaDays = 1 // yoy 对齐走日历年分支（DeltaDays 只贡献符号）
	} else {
		delta := spanDays + 1
		if delta < 1 {
			delta = 1 // 窗口同日（eq / 零长 between）时退化为「前一日」
		}
		plan.DeltaDays = delta
		plan.LowerKey = lo.AddDate(0, 0, -delta).Format(dateLayoutDay)
		plan.UpperKey = hi.AddDate(0, 0, -delta).Format(dateLayoutDay)
		if sec {
			plan.UpperKey = hi.AddDate(0, 0, -delta).Format(dateLayoutSec)
		}
	}
	return plan, nil
}

// baselineAST 克隆当前 AST 并替换对比列的边界条件：该列上的所有条件整组丢弃
// （含「包含空日期」的 OR IS NULL 链——空值行属于当期数据质量语义，不带进基线窗），
// 追加一条 between；去分页去排序（对齐按键进行，无需顺序保证）。
func baselineAST(ast *QueryAST, plan *comparisonPlan) *QueryAST {
	clone := *ast
	filters := make([]FilterExpr, 0, len(ast.Filters)+1)
	for i := range ast.Filters {
		f := ast.Filters[i]
		FE := f.FieldExpr
		if FE == "" {
			FE = f.Field
		}
		if FE == plan.FieldExpr {
			continue
		}
		filters = append(filters, f)
	}
	filters = append(filters, FilterExpr{
		Field:     plan.FieldExpr,
		FieldExpr: plan.FieldExpr,
		Op:        FilterBetween,
		Value:     plan.LowerKey,
		ValueEnd:  plan.UpperKey,
		Logic:     "and",
	})
	clone.Filters = filters
	clone.Pagination = nil
	clone.Sort = nil
	return &clone
}

// attachComparison 在通用查询路径上执行同环比取数并把「(上期)」「(增长率%)」并入响应。
// data 必须是 AxisResponse 或 TableResponse 且当前查询为单维度（compose 侧与卡片均已门控，
// 图型不符/多出的入参在此再兜底显式报错，不静默降级成「配了没效果」）。
func (e *Executor) attachComparison(
	ctx context.Context,
	dialect DialectType,
	ast *QueryAST,
	req *ChartQueryRequest,
	cmp *comparisonConfig,
	data interface{},
) (interface{}, error) {
	switch req.ChartType {
	case ChartTypeBar, ChartTypeLine, ChartTypeArea, ChartTypeTable:
	default:
		return nil, fmt.Errorf("comparison 仅支持 bar/line/area/table 图型（当前 %s）", req.ChartType)
	}
	if len(req.Dims) != 1 {
		return nil, fmt.Errorf("comparison 要求恰好一个维度（日期），当前 %d 个", len(req.Dims))
	}
	if len(req.Metrics) == 0 {
		return nil, fmt.Errorf("comparison 要求至少一个指标")
	}
	if len(req.Dims) == 0 {
		// 先判空再取 dims[0]（空维度时基线处理器会越界）。
		return nil, fmt.Errorf("comparison 要求恰好一个维度（日期），当前 0 个")
	}

	var currentKeys []string
	switch resp := data.(type) {
	case *AxisResponse:
		currentKeys = resp.XAxis
	case *TableResponse:
		dim := req.Dims[0]
		currentKeys = make([]string, 0, len(resp.Data))
		for _, row := range resp.Data {
			currentKeys = append(currentKeys, toString(row[dim]))
		}
	default:
		return nil, fmt.Errorf("comparison: 不支持的响应负载 %T", data)
	}

	plan, err := buildComparisonPlan(cmp, ast, currentKeys)
	if err != nil {
		return nil, err
	}

	baseAST := baselineAST(ast, plan)
	sql, _, args := BuildQueryStringWithBun(dialect, baseAST)
	slog.Debug("executing comparison baseline query", "sql", sql, "args", args)
	baseRes, err := e.conn.Execute(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("comparison baseline query failed: %v", err)
	}
	processor := GetProcessor(req.ChartType)
	baseData, err := processor.Process(baseRes.Rows, req.Dims, req.Metrics, ast)
	if err != nil {
		return nil, fmt.Errorf("comparison baseline process failed: %v", err)
	}

	switch resp := data.(type) {
	case *AxisResponse:
		return attachAxisComparison(resp, baseData, req.Dims, req.Metrics, plan)
	case *TableResponse:
		return attachTableComparison(resp, baseData, req.Dims, req.Metrics, plan)
	}
	return data, nil
}

// baselineLookup 把基线负载折成「当前桶键 → 指标别名 → 值」的查表。
func baselineLookup(baseData interface{}, dims []string, plan *comparisonPlan) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	switch base := baseData.(type) {
	case *AxisResponse:
		if len(base.Series) == 0 {
			return out, nil
		}
		for i, key := range base.XAxis {
			m := unshiftBucketKey(key, plan.DeltaDays, plan.YoY, plan.Sec)
			vals := map[string]any{}
			for _, s := range base.Series {
				var v any
				if i < len(s.Data) {
					v = s.Data[i]
				}
				vals[s.Name] = v
			}
			out[m] = vals
		}
	case *TableResponse:
		dim := dims[0]
		for _, row := range base.Data {
			m := unshiftBucketKey(toString(row[dim]), plan.DeltaDays, plan.YoY, plan.Sec)
			vals := map[string]any{}
			for k, v := range row {
				vals[k] = v
			}
			out[m] = vals
		}
	default:
		return nil, fmt.Errorf("comparison: 基线负载形状意外 %T", baseData)
	}
	return out, nil
}

// comparisonGrowth 计算增长率百分数：任一侧非数值、或上期为 0 → nil（不猜）。
func comparisonGrowth(cur, prev any) any {
	c, ok1 := toFloat64(cur)
	p, ok2 := toFloat64(prev)
	if !ok1 || !ok2 || p == 0 {
		return nil
	}
	return math.Round((c-p)/math.Abs(p)*100*100) / 100
}

func attachAxisComparison(resp *AxisResponse, baseData interface{}, dims []string, metrics []MetricConfig, plan *comparisonPlan) (interface{}, error) {
	lookup, err := baselineLookup(baseData, dims, plan)
	if err != nil {
		return nil, err
	}
	extraPrior := make([]AxisSeries, len(resp.Series))
	extraGrowth := make([]AxisSeries, len(resp.Series))
	for j, s := range resp.Series {
		prior := make([]any, len(s.Data))
		growth := make([]any, len(s.Data))
		for i := range s.Data {
			var key string
			if i < len(resp.XAxis) {
				key = resp.XAxis[i]
			}
			var prev any
			if vals, ok := lookup[key]; ok {
				prev = vals[s.Name]
			}
			prior[i] = prev
			growth[i] = comparisonGrowth(s.Data[i], prev)
		}
		extraPrior[j] = AxisSeries{Name: s.Name + "(上期)", Data: prior}
		extraGrowth[j] = AxisSeries{Name: s.Name + "(增长率%)", Data: growth}
	}
	out := *resp
	out.Series = append(append(append([]AxisSeries{}, resp.Series...), extraPrior...), extraGrowth...)
	return &out, nil
}

func attachTableComparison(resp *TableResponse, baseData interface{}, dims []string, metrics []MetricConfig, plan *comparisonPlan) (interface{}, error) {
	lookup, err := baselineLookup(baseData, dims, plan)
	if err != nil {
		return nil, err
	}
	dim := dims[0]
	newCols := make([]string, 0, len(metrics)*2)
	for _, m := range metrics {
		alias := m.ResolveAlias()
		newCols = append(newCols, alias+"(上期)", alias+"(增长率%)")
	}
	rows := make([]map[string]any, 0, len(resp.Data))
	for _, row := range resp.Data {
		vals := lookup[toString(row[dim])]
		nr := make(map[string]any, len(row)+len(newCols))
		for k, v := range row {
			nr[k] = v
		}
		for _, col := range newCols {
			switch {
			case strings.HasSuffix(col, "(增长率%)"):
				base := strings.TrimSuffix(col, "(增长率%)")
				var cur, prev any
				cur = row[base]
				if vals != nil {
					prev = vals[base]
				}
				nr[col] = comparisonGrowth(cur, prev)
			default:
				base := strings.TrimSuffix(col, "(上期)")
				if vals != nil {
					nr[col] = vals[base]
				}
			}
		}
		rows = append(rows, nr)
	}
	out := *resp
	out.Columns = append(append([]string{}, resp.Columns...), newCols...)
	out.Data = rows
	return &out, nil
}
