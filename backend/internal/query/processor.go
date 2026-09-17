package query

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// Processor 接口定义
type Processor interface {
	// Process 处理查询结果行。ast 携带 v2 协议的槽位信息
	// （DimensionExprs/MetricExprs 的 GroupName/BindingID，与 dims/metrics 按索引一一对应）；
	// v1 平铺协议下 ast 可能为 nil、无 DimensionExprs，或所有 GroupName 均为空/未知槽位名，
	// 此时实现方必须回退到位置推断逻辑，保证 v1 请求行为完全不变（裁定A）。
	Process(rows []map[string]any, dims []string, metrics []MetricConfig, ast *QueryAST) (ChartQueryResponse, error)
}

// bar/line/area 的维度槽位名（v2 协议 dimension_groups[].name，
// 与 frontend/src/components/ChartBuilder/chartDefinitions.ts 的 fieldGroups[].id 对齐）。
const (
	SlotXAxis      = "x_axis"
	SlotColorGroup = "color_group"
)

// TableProcessor Table 图表处理器
type TableProcessor struct {
	Pagination *Pagination
}

// Process 处理 Table 数据。ast 参数在本任务里不消费（裁定A：只有 AxisProcessor 需要槽位感知）。
func (p *TableProcessor) Process(rows []map[string]any, dims []string, metrics []MetricConfig, ast *QueryAST) (ChartQueryResponse, error) {
	if len(rows) == 0 {
		return &TableResponse{
			Columns:    []string{},
			Data:       []map[string]any{},
			Pagination: TablePagination{},
		}, nil
	}

	// 提取列名
	columns := make([]string, 0)
	for key := range rows[0] {
		columns = append(columns, key)
	}

	// 处理分页
	page := 1
	pageSize := 10
	total := len(rows)

	if p.Pagination != nil {
		page = p.Pagination.Page
		pageSize = p.Pagination.PageSize
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	return &TableResponse{
		Columns: columns,
		Data:    rows,
		Pagination: TablePagination{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	}, nil
}

// PieProcessor Pie 图表处理器
type PieProcessor struct {
	Threshold            int     // 长尾合并阈值，默认 20
	MergeOtherBelowRatio float64 // 百分比低于该阈值时合并为“其他”
}

// NewPieProcessor 创建 Pie 处理器
func NewPieProcessor() *PieProcessor {
	return &PieProcessor{
		Threshold:            20,
		MergeOtherBelowRatio: 0,
	}
}

// Process 处理 Pie 数据。ast 参数在本任务里不消费（裁定A：只有 AxisProcessor 需要槽位感知）。
func (p *PieProcessor) Process(rows []map[string]any, dims []string, metrics []MetricConfig, ast *QueryAST) (ChartQueryResponse, error) {
	if len(rows) == 0 || len(dims) == 0 || len(metrics) == 0 {
		return &PieResponse{
			Data: []PieDataItem{},
		}, nil
	}

	dimField := dims[0]
	metricField := metrics[0].ResolveAlias()

	// 计算总值
	var total float64
	dataItems := make([]PieDataItem, 0, len(rows))

	for _, row := range rows {
		dimValue := row[dimField]
		metricValue := row[metricField]

		var value float64
		// SUM(numeric) 经 pgx 返回 pgtype.Numeric，走共享转换避免归零
		// （与散点图数值转换同一缺陷，见 toFloat64）。
		if f, ok := toFloat64(metricValue); ok {
			value = f
		}

		total += value

		name := ""
		if dimValue != nil {
			name = toString(dimValue)
		}

		dataItems = append(dataItems, PieDataItem{
			Name:  name,
			Value: value,
		})
	}

	// 计算百分比
	for i := range dataItems {
		if total > 0 {
			dataItems[i].Percentage = math.Round(dataItems[i].Value*10000/total) / 100
		}
	}

	// 按百分比阈值合并“其他”
	if p.MergeOtherBelowRatio > 0 {
		mainData := make([]PieDataItem, 0, len(dataItems))
		otherData := make([]PieDataItem, 0)

		for _, item := range dataItems {
			if item.Percentage < p.MergeOtherBelowRatio {
				otherData = append(otherData, item)
				continue
			}
			mainData = append(mainData, item)
		}

		if len(otherData) > 0 {
			var otherValue float64
			var otherPercentage float64
			for _, item := range otherData {
				otherValue += item.Value
				otherPercentage += item.Percentage
			}

			mainData = append(mainData, PieDataItem{
				Name:       "其他",
				Value:      otherValue,
				Percentage: math.Round(otherPercentage*100) / 100,
			})
		}

		dataItems = mainData
	}

	// 长尾合并
	var otherData []PieDataItem
	threshold := p.Threshold
	if len(dataItems) > threshold {
		// 按值排序
		for i := 0; i < len(dataItems)-1; i++ {
			for j := i + 1; j < len(dataItems); j++ {
				if dataItems[i].Value < dataItems[j].Value {
					dataItems[i], dataItems[j] = dataItems[j], dataItems[i]
				}
			}
		}

		// 保留前 N 个，其余合并
		mainData := dataItems[:threshold]
		otherData = dataItems[threshold:]

		// 更新主数据
		dataItems = mainData
	}

	// 合并 Other
	if len(otherData) > 0 {
		var otherValue float64
		var otherPercentage float64
		for _, item := range otherData {
			otherValue += item.Value
			otherPercentage += item.Percentage
		}

		dataItems = append(dataItems, PieDataItem{
			Name:       "其他",
			Value:      otherValue,
			Percentage: math.Round(otherPercentage*100) / 100,
		})
	}

	return &PieResponse{
		Data: dataItems,
	}, nil
}

// AxisProcessor 坐标轴图表处理器 (Bar, Line, Area)
type AxisProcessor struct{}

// Process 处理坐标轴图表数据。
// 槽位感知（裁定A）：若 ast 携带 v2 协议的 x_axis/color_group 槽位名，按槽位名明确区分
// X 轴维度与颜色分组维度，不依赖 dims[0]/dims[1:] 的位置顺序；否则（v1 平铺协议，
// ast 为 nil / 无 DimensionExprs / GroupName 全为空或未知）回退到下面的位置推断逻辑，
// 保证 v1 请求行为完全不变。
func (p *AxisProcessor) Process(rows []map[string]any, dims []string, metrics []MetricConfig, ast *QueryAST) (ChartQueryResponse, error) {
	if len(rows) == 0 || len(dims) == 0 || len(metrics) == 0 {
		return &AxisResponse{
			XAxis:  []string{},
			Series: []AxisSeries{},
		}, nil
	}

	if xAxisDims, colorGroupDims, ok := resolveAxisSlots(dims, ast); ok {
		return processAxisWithSlots(rows, xAxisDims, colorGroupDims, metrics), nil
	}

	dimField := dims[0]

	// 单维度：保持原有逻辑
	if len(dims) == 1 {
		xAxis := make([]string, len(rows))
		for i, row := range rows {
			val := row[dimField]
			if val != nil {
				xAxis[i] = toString(val)
			}
		}

		series := make([]AxisSeries, len(metrics))
		for j, metric := range metrics {
			metricAlias := metric.ResolveAlias()
			data := make([]any, len(rows))
			for i, row := range rows {
				data[i] = row[metricAlias]
			}
			series[j] = AxisSeries{Name: metricAlias, Data: data}
		}

		return &AxisResponse{XAxis: xAxis, Series: series}, nil
	}

	// 多维度：第一个维度为 X 轴，后续维度值组合为 series
	// 1. 收集去重的 X 轴值（保持出现顺序）
	xAxisMap := make(map[string]bool)
	var xAxisOrder []string
	for _, row := range rows {
		xVal := toString(row[dimField])
		if !xAxisMap[xVal] {
			xAxisMap[xVal] = true
			xAxisOrder = append(xAxisOrder, xVal)
		}
	}

	// 2. 构建 series key → 指标数据映射
	type seriesKey struct {
		metricAlias string
		dimCombo    string
	}
	seriesData := make(map[seriesKey][]any)
	seriesOrder := make([]seriesKey, 0)

	// 先按维度组合顺序收集所有 dimCombo
	dimComboOrder := make([]string, 0)
	dimComboSeen := make(map[string]bool)

	for _, row := range rows {
		var dimParts []string
		for _, d := range dims[1:] {
			dimParts = append(dimParts, toString(row[d]))
		}
		dimCombo := strings.Join(dimParts, " - ")
		if !dimComboSeen[dimCombo] {
			dimComboSeen[dimCombo] = true
			dimComboOrder = append(dimComboOrder, dimCombo)
		}
	}

	// 按 metric → dimCombo 顺序构建 series
	for _, metric := range metrics {
		alias := metric.ResolveAlias()
		for _, dimCombo := range dimComboOrder {
			key := seriesKey{metricAlias: alias, dimCombo: dimCombo}
			seriesOrder = append(seriesOrder, key)
			seriesData[key] = make([]any, len(xAxisOrder))
		}
	}

	// 填充数据
	for _, row := range rows {
		xVal := toString(row[dimField])

		var dimParts []string
		for _, d := range dims[1:] {
			dimParts = append(dimParts, toString(row[d]))
		}
		dimCombo := strings.Join(dimParts, " - ")

		for _, metric := range metrics {
			alias := metric.ResolveAlias()
			key := seriesKey{metricAlias: alias, dimCombo: dimCombo}

			for xi, xv := range xAxisOrder {
				if xv == xVal {
					seriesData[key][xi] = row[alias]
					break
				}
			}
		}
	}

	// 3. 构建结果
	series := make([]AxisSeries, len(seriesOrder))
	for i, key := range seriesOrder {
		name := key.dimCombo
		if len(metrics) > 1 {
			name = key.metricAlias + " - " + key.dimCombo
		}
		series[i] = AxisSeries{Name: name, Data: seriesData[key]}
	}

	return &AxisResponse{XAxis: xAxisOrder, Series: series}, nil
}

// resolveAxisSlots 尝试从 AST 里按索引解析 bar/line/area 的 x_axis/color_group 槽位。
// dims 与 ast.DimensionExprs 由 service 层同源的 QuerySpec 分别平铺而来（plannedQuery.Dims
// 与 PlanAST 都按 spec.Dimensions 顺序遍历），因此按索引一一对应；额外校验 Field 名对齐，
// 一旦发现数量或字段名不匹配（理论上不会发生，防御性兜底），返回 ok=false 让调用方回退到
// 位置推断，避免用错槽位名切分数据。返回 ok=false 也覆盖 v1 平铺协议的正常情况
// （ast 为 nil、无 DimensionExprs、所有 GroupName 均为空/未知槽位名，或只有 x_axis
// 槽位而没有 color_group 槽位——见下方裁定B注释）。
func resolveAxisSlots(dims []string, ast *QueryAST) (xAxisDims []string, colorGroupDims []string, ok bool) {
	if ast == nil || len(ast.DimensionExprs) != len(dims) {
		return nil, nil, false
	}
	for i, d := range dims {
		expr := ast.DimensionExprs[i]
		if expr.Field != d {
			return nil, nil, false
		}
		switch expr.GroupName {
		case SlotXAxis:
			xAxisDims = append(xAxisDims, d)
		case SlotColorGroup:
			colorGroupDims = append(colorGroupDims, d)
		}
	}
	if len(xAxisDims) == 0 || len(colorGroupDims) == 0 {
		// 裁定B：前端只在 color_group 真正非空时才发 v2 wire 格式，因此真正的 v2
		// bar/line/area 请求必然带 color_group 槽位。缺少 color_group 说明这是 v1
		// 请求（defaultDimGroupName 会把所有维度标成 "x_axis"）或 color_group 为空的
		// v2 请求，两者都必须走旧的位置推断逻辑（dims[0]=X 轴，dims[1:]=series 拆分），
		// 避免多维度 v1 请求被误拼成复合 X 轴。
		return nil, nil, false
	}
	return xAxisDims, colorGroupDims, true
}

// processAxisWithSlots 按显式槽位名切分 X 轴与 series（不依赖 dims 的位置顺序）：
// xAxisDims 的值组合成类目轴（多于一个字段时用 " - " 连接，与旧多维度逻辑的连接符一致），
// colorGroupDims 的值组合成 series 名（同样用 " - " 连接）。colorGroupDims 为空时退化为
// "每个指标一条 series"，与旧的单维度逻辑一致；命名规则（单指标只用颜色值、多指标用
// "指标别名 - 颜色值"）也与旧多维度逻辑保持一致，便于 v1/v2 两条路径产出可对比的结果。
func processAxisWithSlots(rows []map[string]any, xAxisDims []string, colorGroupDims []string, metrics []MetricConfig) *AxisResponse {
	joinSlotValues := func(row map[string]any, slotDims []string) string {
		parts := make([]string, len(slotDims))
		for i, d := range slotDims {
			parts[i] = toString(row[d])
		}
		return strings.Join(parts, " - ")
	}

	// 1. 收集去重的 X 轴值（保持出现顺序）
	xAxisSeen := make(map[string]bool)
	var xAxisOrder []string
	for _, row := range rows {
		xVal := joinSlotValues(row, xAxisDims)
		if !xAxisSeen[xVal] {
			xAxisSeen[xVal] = true
			xAxisOrder = append(xAxisOrder, xVal)
		}
	}

	if len(colorGroupDims) == 0 {
		series := make([]AxisSeries, len(metrics))
		for j, metric := range metrics {
			alias := metric.ResolveAlias()
			data := make([]any, len(xAxisOrder))
			for _, row := range rows {
				xVal := joinSlotValues(row, xAxisDims)
				for xi, xv := range xAxisOrder {
					if xv == xVal {
						data[xi] = row[alias]
						break
					}
				}
			}
			series[j] = AxisSeries{Name: alias, Data: data}
		}
		return &AxisResponse{XAxis: xAxisOrder, Series: series}
	}

	// 2. 有颜色分组：series = 指标 × 颜色值组合
	type seriesKey struct {
		metricAlias string
		colorCombo  string
	}
	seriesData := make(map[seriesKey][]any)
	seriesOrder := make([]seriesKey, 0)

	colorComboOrder := make([]string, 0)
	colorComboSeen := make(map[string]bool)
	for _, row := range rows {
		colorCombo := joinSlotValues(row, colorGroupDims)
		if !colorComboSeen[colorCombo] {
			colorComboSeen[colorCombo] = true
			colorComboOrder = append(colorComboOrder, colorCombo)
		}
	}

	for _, metric := range metrics {
		alias := metric.ResolveAlias()
		for _, colorCombo := range colorComboOrder {
			key := seriesKey{metricAlias: alias, colorCombo: colorCombo}
			seriesOrder = append(seriesOrder, key)
			seriesData[key] = make([]any, len(xAxisOrder))
		}
	}

	for _, row := range rows {
		xVal := joinSlotValues(row, xAxisDims)
		colorCombo := joinSlotValues(row, colorGroupDims)
		for _, metric := range metrics {
			alias := metric.ResolveAlias()
			key := seriesKey{metricAlias: alias, colorCombo: colorCombo}
			for xi, xv := range xAxisOrder {
				if xv == xVal {
					seriesData[key][xi] = row[alias]
					break
				}
			}
		}
	}

	series := make([]AxisSeries, len(seriesOrder))
	for i, key := range seriesOrder {
		name := key.colorCombo
		if len(metrics) > 1 {
			name = key.metricAlias + " - " + key.colorCombo
		}
		series[i] = AxisSeries{Name: name, Data: seriesData[key]}
	}

	return &AxisResponse{XAxis: xAxisOrder, Series: series}
}

// ScatterProcessor Scatter 图表处理器
type ScatterProcessor struct{}

// Process 处理 Scatter 数据。ast 参数在本任务里不消费（裁定A：只有 AxisProcessor 需要槽位感知）。
func (p *ScatterProcessor) Process(rows []map[string]any, dims []string, metrics []MetricConfig, ast *QueryAST) (ChartQueryResponse, error) {
	if len(rows) == 0 || len(metrics) < 2 {
		return &ScatterResponse{
			Data: [][]float64{},
		}, nil
	}

	xField := metrics[0].ResolveAlias()
	yField := metrics[1].ResolveAlias()

	data := make([][]float64, 0, len(rows))

	for _, row := range rows {
		xVal := row[xField]
		yVal := row[yField]

		x, ok1 := toFloat64(xVal)
		y, ok2 := toFloat64(yVal)

		if ok1 && ok2 {
			data = append(data, []float64{x, y})
		}
	}

	return &ScatterResponse{
		Data: data,
	}, nil
}

// PivotProcessor 透视表处理器
type PivotProcessor struct{}

// Process 处理 Pivot 数据。ast 参数在本任务里不消费（裁定A：只有 AxisProcessor 需要槽位感知）。
func (p *PivotProcessor) Process(rows []map[string]any, dims []string, metrics []MetricConfig, ast *QueryAST) (ChartQueryResponse, error) {
	if len(rows) == 0 {
		return &PivotResponse{
			Columns: []string{},
			Data:    []map[string]any{},
		}, nil
	}

	// 提取列名
	columns := make([]string, 0)
	for key := range rows[0] {
		columns = append(columns, key)
	}

	return &PivotResponse{
		Columns: columns,
		Data:    rows,
	}, nil
}

// KpiProcessor KPI 单值卡处理器（R-51）
type KpiProcessor struct{}

// Process 处理 KPI 单值数据。ast 参数在本任务里不消费（裁定A：只有 AxisProcessor 需要槽位感知）。
// kpi 图型约定：无维度（dims 为空）、单指标；查询在 SQL 层已退化为标量聚合
// （零维度 → BunQueryBuilder 不生成 GROUP BY，聚合查询恰好返回一行），这里从第一行
// 按指标别名取值。边界情况：rows 为空（无数据）、聚合结果为 NULL、或值不可转数值时
// 返回 Value:0 而不是报错或 panic；多行时防御性取第一行。
// 已知限制（Task 1-5 范围内接受）：MetricConfig 只有 Field/Agg/Alias，没有 Unit/Format
// 字段（扩展涉及 wire 协议变更，超出本任务范围），因此 Unit/Format 恒为空
// （omitempty，JSON 里不出现）；前端 KpiCard 的 unit/format 展示信息改从前端配置侧取。
func (p *KpiProcessor) Process(rows []map[string]any, dims []string, metrics []MetricConfig, ast *QueryAST) (ChartQueryResponse, error) {
	if len(metrics) == 0 {
		return &KpiResponse{}, nil
	}

	label := metrics[0].ResolveAlias()
	resp := &KpiResponse{Label: label}

	if len(rows) == 0 {
		return resp, nil
	}

	// SELECT 别名经 quoteResultAlias 引号保留，行键与 ResolveAlias() 逐字一致；
	// toFloat64 覆盖 pgtype.Numeric（SUM(numeric)）、数值字符串（PG numeric 经 pgx）
	// 等形态，NULL（nil）或不可转数值时落回 Value:0。
	if v, ok := toFloat64(rows[0][label]); ok {
		resp.Value = v
	}
	return resp, nil
}

// GetProcessor 获取对应的处理器
func GetProcessor(chartType ChartType) Processor {
	switch chartType {
	case ChartTypeTable:
		return &TableProcessor{}
	case ChartTypePie:
		return NewPieProcessor()
	case ChartTypeFunnel:
		// funnel（漏斗图，R-59）复用 PieProcessor：返回 PieResponse{name,value}，
		// 降序由前端 composeChartQueryRequest 注入的 ORDER BY 决定（PieProcessor 保序）。
		return NewPieProcessor()
	case ChartTypeBar, ChartTypeLine, ChartTypeArea:
		return &AxisProcessor{}
	case ChartTypeCombo:
		// combo（双轴组合图，R-58）复用 AxisProcessor：primary/secondary 的区分在
		// ChartSpec 的 metric 槽位名中，两个槽位的 metrics 合并进同一个 metrics[] 传给
		// processor（plan §3.1），processor 本身无需感知槽位——显式分支而非依赖 default 兜底。
		return &AxisProcessor{}
	case ChartTypeScatter:
		return &ScatterProcessor{}
	case ChartTypePivot:
		return &PivotProcessor{}
	case ChartTypeKpi:
		return &KpiProcessor{}
	case ChartTypeHistogram:
		// histogram 的真实路径是 executor 的两阶段分支（executeHistogram →
		// HistogramProcessor.ProcessBins）；显式 case 避免误落 AxisProcessor 兜底。
		return &HistogramProcessor{}
	default:
		return &AxisProcessor{}
	}
}

// toString 转换为字符串
func toString(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// toFloat64 转换为 float64
func toFloat64(val any) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int64:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case string:
		// PG numeric 列经 pgx 解码为字符串（如 "148730.0"）
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	case []byte:
		f, err := strconv.ParseFloat(strings.TrimSpace(string(v)), 64)
		return f, err == nil
	case pgtype.Numeric:
		// pgx 对 numeric 列（如 SUM(numeric)）返回 pgtype.Numeric 结构体
		f, err := v.Float64Value()
		if err != nil || !f.Valid {
			return 0, false
		}
		return f.Float64, true
	default:
		return 0, false
	}
}
