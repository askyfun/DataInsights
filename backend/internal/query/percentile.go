package query

import (
	"fmt"
	"math"

	"data-insights/internal/datasource"
)

// percentile.go — 百分位聚合的 SQL 表达式原语（R-54，plan §4.1-4.2）。
// Task 3-4 用户可见的 median 聚合（p=0.5 固定）与 Task 3-5 箱线图内部
// 需要的 Q1/median/Q3 三次调用共用这里的 BuildPercentileExpr——**这是本
// 文件的抽象层定位**。executor 在建 SQL 之前会先跑 CheckPercentileSupport
// 做前置门，保证不会用不受支持的策略走到 BuildPercentileExpr；builder 层
// 拿到 median 指标时按 percentile_cont 契约产表达式（见 renderMetricSelect 注释）。

// medianPercentile 中位数聚合的百分位数值（AggMedian 固定用 0.5）。
const medianPercentile = 0.5

// BuildPercentileExpr 按 caps 声明的策略生成 percentile SQL 表达式（不含 "AS alias"，
// 由调用方拼接）。field 必须是已过 safeIdentifier 校验的裸/引号列名。
// 当前仅 "percentile_cont" 策略真实落地（PG 走这条，plan §4.1 表格）；其余策略
// **显式报错、不静默近似**（plan §4.2 明确规则）：
//   - "" / "unsupported"：数据源不支持 percentile（CH/MySQL/StarRocks 现状，Task 3-0 保守裁定）；
//   - "quantilesExactInclusive"：ClickHouse 潜在未来路径（返回 ARRAY 需 [1] 索引 + scalar 形状
//     对齐），未实现——**若未来某驱动把策略改成这个值，本函数返回 "not yet implemented"
//     错误**，强制要求同步升级本文件与测试，防止驱动侧声明与 SQL 生成侧脱节；
//   - "window_ntile"：MySQL 8+ / StarRocks 潜在未来路径（PERCENT_RANK + 插值），同上；
//   - 其他未知值：视为配置错误。
func BuildPercentileExpr(field string, p float64, caps *datasource.DialectCapabilities) (string, error) {
	if caps == nil {
		return "", fmt.Errorf("percentile aggregation requires dialect capabilities; got nil")
	}
	// p 校验：NaN 会污染 fmt 输出；[0,1] 是 percentile 语义边界。
	if math.IsNaN(p) || p < 0 || p > 1 {
		return "", fmt.Errorf("percentile p must be in [0,1], got %v", p)
	}
	switch caps.PercentileStrategy {
	case "percentile_cont":
		// PG 标准 SQL：percentile_cont(<p> double precision) WITHIN GROUP (ORDER BY <field>)。
		// %.4g 保证 0.5→"0.5"、0.25→"0.25"、0.75→"0.75"，避免默认 %v 的尾零噪声。
		return fmt.Sprintf("percentile_cont(%.4g) WITHIN GROUP (ORDER BY %s)", p, field), nil
	case "percentile_cont_args_first":
		// StarRocks：percentile_cont(<field>, <p>)——列在前、无 WITHIN GROUP 子句。
		// 2026-09-19 在真实实例（192.168.10.237:9030）实测验证为精确百分位
		// （0.5 分位返回 16.585，与 percentile_approx 的近似值 16.584999 明显区分）。
		return fmt.Sprintf("percentile_cont(%s, %.4g)", field, p), nil
	case "", "unsupported":
		return "", fmt.Errorf("percentile aggregation not supported by this data source")
	case "quantilesExactInclusive", "window_ntile":
		return "", fmt.Errorf("percentile strategy %q is declared but not yet implemented in percentile.go", caps.PercentileStrategy)
	default:
		return "", fmt.Errorf("unknown percentile strategy %q", caps.PercentileStrategy)
	}
}

// AstRequiresPercentile 判断 AST 里是否有任何指标走 percentile 家族（当前只有 AggMedian）。
// executor 用它决定是否触发 caps 探针 + 支持性检查——无 percentile 指标时**不查 caps**、零开销。
func AstRequiresPercentile(ast *QueryAST) bool {
	if ast == nil {
		return false
	}
	for _, m := range ast.Metrics {
		if m.Agg == AggMedian {
			return true
		}
	}
	return false
}

// CheckPercentileSupport 供 executor 前置门用：caps 允许至少产出一个可用 percentile
// 表达式则返回 nil，否则返回描述性错误。**executor 在建 SQL 之前调用本函数**
// （plan §4.2 明确错误规则；比 DB 报语法错误清晰得多）。
func CheckPercentileSupport(caps *datasource.DialectCapabilities) error {
	// 以 p=0.5 探测（AggMedian 的实际入参）；成功即策略可用。
	_, err := BuildPercentileExpr("x", medianPercentile, caps)
	return err
}
