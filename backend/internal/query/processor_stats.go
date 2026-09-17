package query

import (
	"fmt"
	"math"
)

// processor_stats.go 承载统计分布族图型的 processor（plan §7）。
// 本文件当前有 HistogramProcessor（R-57）与 RadarProcessor（R-62）；
// Boxplot 由后续任务（Task 3-5）加入。

// defaultHistogramBinCount query_options.bin_count 缺省值（plan §3.3 裁定）。
const defaultHistogramBinCount = 20

// maxHistogramBins 分箱数上限：bin_count/bin_width 直接来自请求体，荒谬值
// （巨量 bin_count / 极小 bin_width）会放大为 ProcessBins 的无界内存分配
// （bin_count=1e8 单请求即 ~2.4GB）；超过该上限的分箱在视觉上也无意义。
const maxHistogramBins = 10000

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
// 分支调用）：bin_count 缺省/非数值/<1 时回落 defaultHistogramBinCount(20)，
// >maxHistogramBins 时钳到上限（防无界分配，float 域钳制避免荒谬值 int 溢出）；
// bin_width 为可选用户覆盖（仅 >0 生效），返回 0 表示未指定。JSON 数字进
// map[string]any 后是 float64，toFloat64 同时兼容 int/int64/数值字符串等形态。
func histogramBinOptions(opts map[string]any) (binCount int, userBinWidth float64) {
	binCount = defaultHistogramBinCount
	if v, ok := toFloat64(opts["bin_count"]); ok && v >= 1 {
		if v > maxHistogramBins {
			v = maxHistogramBins
		}
		binCount = int(v)
	}
	if v, ok := toFloat64(opts["bin_width"]); ok && v > 0 {
		userBinWidth = v
	}
	return binCount, userBinWidth
}

// radar 的维度槽位名（v2 协议 dimension_groups[].name，与
// frontend/src/components/ChartBuilder/chartDefinitions.ts 的 radar fieldGroups[].id 对齐）。
const (
	SlotIndicators  = "indicators"   // 雷达轴：每个维度值一条轴（minGroups 1 / maxFields 1）
	SlotSeriesGroup = "series_group" // 可选的系列拆分维度（minGroups 0 / maxFields 1）
)

// resolveRadarSlots 解析 radar 的 indicators / series_group 维度槽位，返回它们在结果行
// 里的输出列键（与 resolvePivotSlots 同款：dims 与 ast.DimensionExprs 由 service 层同源的
// QuerySpec 分别平铺而来，按索引一一对应，额外校验 Field 名对齐）。
//   - v2 路径：按 GroupName 归槽，series_group 可缺省（返回空串表示不拆系列）；
//     同一槽位出现多个维度时取首个（协议 maxFields=1，多余字段无从命名）。
//   - v1/位置回退：ast 为 nil、DimensionExprs 与 dims 数量或 Field 名不匹配、或槽位名
//     不在 indicators/series_group 之内（v1 平铺请求经 ChartSpecFromRequest 会把所有维度
//     打成默认组名 "x_axis"）时按位置解析：dims[0]=indicators、dims[1]=series_group。
//     这条回退让后端可以先于前端 v2 接线上线，v1 radar 请求也能产出正确形状。
//
// ok=false 只发生在 dims 为空（连雷达轴都没有）——调用方必须报错，不得静默产空形状。
func resolveRadarSlots(dims []string, ast *QueryAST) (indicatorKey, seriesKey string, ok bool) {
	if len(dims) == 0 {
		return "", "", false
	}
	if ast != nil && len(ast.DimensionExprs) == len(dims) {
		aligned := true
		for i, d := range dims {
			if expr := ast.DimensionExprs[i]; expr.Field != d {
				aligned = false
				break
			} else if expr.GroupName == SlotIndicators && indicatorKey == "" {
				indicatorKey = pivotDimOutputKey(expr)
			} else if expr.GroupName == SlotSeriesGroup && seriesKey == "" {
				seriesKey = pivotDimOutputKey(expr)
			}
		}
		if aligned && indicatorKey != "" {
			return indicatorKey, seriesKey, true
		}
	}
	// v1 平铺协议：无槽位信息可用，按位置推断（不带 alias，输出列名即字段名）。
	if len(dims) > 1 {
		return dims[0], dims[1], true
	}
	return dims[0], "", true
}

// RadarProcessor 雷达图处理器（R-62，plan §3.3）。
// radar 的 SQL 就是一条普通 `GROUP BY indicators[, series_group] + AGG(value)` 查询
// （无 GROUPING SETS、无两阶段分箱），因此不需要 executor 专属分支：走通用路径拿到
// 平铺结果行后，本处理器只做「行 → 轴 × 系列矩阵」的纯重塑。
// 协议口径：value 取 metrics[0]（前端 radar 的 values 槽位 maxFields=1），多条系列由
// series_group 维度值拆分而非多个指标——与 AxisResponse「一指标一系列」的形态互补。
type RadarProcessor struct{}

// Process 把 GROUP BY 结果行重塑为 RadarResponse。形状裁定（均为有意选择，改动会让
// 前端 ECharts radar 画出错误图形）：
//   - Indicators 按维度值首次出现顺序排列（视觉顺序由 SQL 的 ORDER BY 决定）；
//   - RadarIndicator.Max 是该轴跨所有系列的**真实**最大值，供 ECharts 定轴刻度。
//     累积不能以 0 起步（全负轴会被抬成 0，如 [-3,-7] 应为 -3），这里用「map 键不存在
//     即首次出现、直接取该值」代替 0 或 ±Inf 哨兵；
//   - 每个 Series.Values 与 Indicators 等长同序，该系列在某轴无行时补 0.0：ECharts 把
//     0 画在圆心上保持多边形闭合，用 null 断点则画不出闭合雷达面；
//   - NULL 维度值渲染成空串、NULL/不可转数值的聚合值按 0 计（与 extractDimValues、
//     metricValue 的既有兜底口径一致），不中断整张图；
//   - 空结果返回空切片而非 nil（响应契约要求集合字段序列化为 [] 而不是 null）。
//
// 无 indicators 维度（resolveRadarSlots ok=false）与缺 value 指标一律显式返回 error。
func (p *RadarProcessor) Process(rows []map[string]any, dims []string, metrics []MetricConfig, ast *QueryAST) (ChartQueryResponse, error) {
	indicatorKey, seriesKey, ok := resolveRadarSlots(dims, ast)
	if !ok {
		return nil, fmt.Errorf("radar processor requires at least one indicator dimension")
	}
	if len(metrics) == 0 {
		return nil, fmt.Errorf("radar processor requires at least one value metric")
	}
	valueKey := metrics[0].ResolveAlias()
	// 无 series_group 维度时只有一条系列，名取 value 指标别名。
	defaultSeriesName := valueKey

	// 首次出现顺序 + 去重（雷达轴与系列各自的展示顺序）。
	var indicatorOrder, seriesOrder []string
	indicatorSeen := make(map[string]bool)
	seriesSeen := make(map[string]bool)
	// cells[系列][轴] = 聚合值；maxPerIndicator[轴] = 该轴跨所有系列的最大值。
	cells := make(map[string]map[string]float64)
	maxPerIndicator := make(map[string]float64)

	for _, row := range rows {
		indicator := extractDimValues(row, []string{indicatorKey})[0]
		series := defaultSeriesName
		if seriesKey != "" {
			series = extractDimValues(row, []string{seriesKey})[0]
		}
		value := metricValue(row, valueKey)

		if !indicatorSeen[indicator] {
			indicatorSeen[indicator] = true
			indicatorOrder = append(indicatorOrder, indicator)
		}
		if !seriesSeen[series] {
			seriesSeen[series] = true
			seriesOrder = append(seriesOrder, series)
		}
		if cells[series] == nil {
			cells[series] = make(map[string]float64)
		}
		// 同一 (系列, 轴) 出现重复行时以最后一行为准：GROUP BY 已保证组合唯一，重复只
		// 可能来自非 builder 产出的行集，累加会凭空造数，覆盖至多丢一条脏行。
		cells[series][indicator] = value
		// max 累积：键不存在（该轴首次出现）时取该值为初值，之后取更大者（见 doc comment
		// 的全负轴裁定）。
		if seen, exists := maxPerIndicator[indicator]; !exists || value > seen {
			maxPerIndicator[indicator] = value
		}
	}

	indicators := make([]RadarIndicator, len(indicatorOrder))
	for i, name := range indicatorOrder {
		indicators[i] = RadarIndicator{Name: name, Max: maxPerIndicator[name]}
	}
	seriesList := make([]RadarSeries, len(seriesOrder))
	for i, name := range seriesOrder {
		values := make([]float64, len(indicatorOrder))
		cell := cells[name]
		for j, indicator := range indicatorOrder {
			values[j] = cell[indicator] // 缺失对取 map 零值 0.0，保证与 Indicators 等长同序
		}
		seriesList[i] = RadarSeries{Name: name, Values: values}
	}

	return &RadarResponse{Indicators: indicators, Series: seriesList}, nil
}

// BoxplotProcessor 箱线图处理器（R-52，plan §3.3）。
// boxplot 是多查询编排（stats → Go 端算 fence → outliers list + outliers count），
// 三段结果只在 executor 内同时可得，通用 Process 签名（只见单次 rows）表达不了，
// 因此组装走专门的 Assemble，由 executor 的 executeBoxplot 调用（对齐 HistogramProcessor
// 的 ProcessBins 组织方式）。GetProcessor 返回本类型只为避免误落 AxisProcessor 兜底。
type BoxplotProcessor struct{}

// Process 通用 Processor 接口实现：boxplot 不走该入口（见类型注释），显式报错而非静默
// 产出错误形状。
func (p *BoxplotProcessor) Process(rows []map[string]any, dims []string, metrics []MetricConfig, ast *QueryAST) (ChartQueryResponse, error) {
	return nil, fmt.Errorf("boxplot requires the multi-query executor path (BoxplotProcessor.Assemble)")
}

// Assemble 把 stats 行 + outliers 行 + outlier 总数组装成 BoxplotResponse。
//   - statsRow 为空或 q1/median/q3 均非数值（空数据集下 MIN/percentile/MAX 全 NULL）
//     → 返回退化结构（五值 0、空 outliers、total 0、truncated false），不报错；
//   - outliers 从每行的 boxOutlierValueAlias（"val"）列读取，非数值跳过（防御，正常不发生）；
//   - truncated = outlierTotal > len(outliers)（outliers list 查询 LIMIT 1000 截断，total 是截断前计数）。
func (p *BoxplotProcessor) Assemble(
	statsRow map[string]any, outlierRows []map[string]any, outlierTotal int64,
) (*BoxplotResponse, error) {
	resp := &BoxplotResponse{Outliers: []float64{}}
	if statsRow == nil {
		return resp, nil // 空数据：退化箱
	}
	q1, q1OK := toFloat64(statsRow[boxQ1Alias])
	med, medOK := toFloat64(statsRow[boxMedianAlias])
	q3, q3OK := toFloat64(statsRow[boxQ3Alias])
	if !q1OK && !medOK && !q3OK {
		// 空数据集：percentile/MIN/MAX 全 NULL。三轴皆无值即判空，产退化结构。
		return resp, nil
	}
	resp.Q1 = q1
	resp.Median = med
	resp.Q3 = q3
	// whisker_low/high 用全局 MIN/MAX（plan 字面口径）；NULL（空/非数值）落 0。
	resp.WhiskerLow, _ = toFloat64(statsRow[boxWhiskerLowAlias])
	resp.WhiskerHigh, _ = toFloat64(statsRow[boxWhiskerHighAlias])

	outliers := make([]float64, 0, len(outlierRows))
	for _, row := range outlierRows {
		if v, ok := toFloat64(row[boxOutlierValueAlias]); ok {
			outliers = append(outliers, v)
		}
	}
	resp.Outliers = outliers
	resp.OutlierTotal = outlierTotal
	resp.Truncated = outlierTotal > int64(len(outliers))
	return resp, nil
}
