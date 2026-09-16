package query

import (
	"fmt"
	"math"
)

// processor_stats.go 承载统计分布族图型的 processor（plan §7）。
// 本文件当前只有 HistogramProcessor（R-57）；Boxplot/Radar 由后续任务
// （Task 3-5 / 3-3）加入。

// defaultHistogramBinCount query_options.bin_count 缺省值（plan §3.3 裁定）。
const defaultHistogramBinCount = 20

// HistogramProcessor 直方图处理器（R-57）。
// histogram 是两阶段查询：阶段1 MIN/MAX/COUNT → Go 端算 bin 宽 → 阶段2 FLOOR
// 分箱计数。bins 组装需要阶段1 的 minValue/binWidth——这些只在 executor 的
// histogram 分支可得，通用 Process 签名（只见阶段2 rows）表达不了，因此组装
// 走专门的 ProcessBins，由 executor 调用（与 pivot 分支直接调 PivotProcessorV2
// 的组织方式同款，适配两阶段）。
type HistogramProcessor struct{}

// Process 通用 Processor 接口实现：histogram 不走该入口（见类型注释），
// 显式报错而非静默产出错误形状——GetProcessor 的 histogram case 返回本类型
// 只是为了避免误落 AxisProcessor 兜底，正常路径不会调用到这里。
func (p *HistogramProcessor) Process(rows []map[string]any, dims []string, metrics []MetricConfig, ast *QueryAST) (ChartQueryResponse, error) {
	return nil, fmt.Errorf("histogram requires the two-phase executor path (HistogramProcessor.ProcessBins)")
}

// ProcessBins 把阶段2 的分箱计数行（{"bin": 索引, "cnt": 计数}）组装为
// HistogramResponse：
//   - 补全空 bin：输出 numBins 个连续 bin（索引 0..numBins-1），阶段2 没有行的
//     bin 以 Count=0 占位——x 轴连续（前端渲染需要），且 sum(Count)==数值行总数
//     的不变式保持（空 bin 加 0）；
//   - BinStart = minValue + i*binWidth，BinEnd = BinStart + binWidth；
//   - 边界钳制：FLOOR 的浮点误差可能让 value==maxValue 的行落到索引 numBins
//     （如 (mx-mn)/binWidth 恰为整数时），负零误差可能落到 -1；越界索引一律
//     钳进 [0, numBins-1]（首/末 bin 吸收），不丢行——保证 sum(Count)==总数；
//   - NULL bin（值列为 NULL 的行经 FLOOR 归入 NULL 组）不计入数值分箱，跳过；
//     直方图只对数值分箱，该口径下"总数"指非 NULL 数值行。
func (p *HistogramProcessor) ProcessBins(rows []map[string]any, minValue, binWidth float64, numBins int) (*HistogramResponse, error) {
	if numBins < 1 {
		return nil, fmt.Errorf("histogram numBins must be >= 1, got %d", numBins)
	}
	if !(binWidth > 0) { // 同时挡住 0、负数与 NaN
		return nil, fmt.Errorf("histogram bin_width must be positive, got %v", binWidth)
	}

	counts := make([]int64, numBins)
	for _, row := range rows {
		binVal, ok := toFloat64(row[histogramBinAlias])
		if !ok {
			continue // NULL bin（值列 NULL 行），见 doc comment
		}
		cnt, ok := toFloat64(row[histogramCountAlias])
		if !ok {
			return nil, fmt.Errorf("histogram bin count is not numeric: %v", row[histogramCountAlias])
		}
		idx := int(math.Floor(binVal))
		if idx < 0 {
			idx = 0
		} else if idx >= numBins {
			idx = numBins - 1
		}
		counts[idx] += int64(cnt)
	}

	bins := make([]HistogramBin, numBins)
	for i := 0; i < numBins; i++ {
		bins[i] = HistogramBin{
			BinStart: minValue + float64(i)*binWidth,
			BinEnd:   minValue + float64(i+1)*binWidth,
			Count:    counts[i],
		}
	}
	return &HistogramResponse{Bins: bins}, nil
}

// histogramBinOptions 从请求的 query_options 解析分箱参数（executor histogram
// 分支调用）：bin_count 缺省/非数值/<1 时回落 defaultHistogramBinCount(20)；
// bin_width 为可选用户覆盖（仅 >0 生效），返回 0 表示未指定。JSON 数字进
// map[string]any 后是 float64，toFloat64 同时兼容 int/int64/数值字符串等形态。
func histogramBinOptions(opts map[string]any) (binCount int, userBinWidth float64) {
	binCount = defaultHistogramBinCount
	if v, ok := toFloat64(opts["bin_count"]); ok && v >= 1 {
		binCount = int(v)
	}
	if v, ok := toFloat64(opts["bin_width"]); ok && v > 0 {
		userBinWidth = v
	}
	return binCount, userBinWidth
}
