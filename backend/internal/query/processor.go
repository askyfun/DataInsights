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
	Process(rows []map[string]any, dims []string, metrics []MetricConfig) (ChartQueryResponse, error)
}

// TableProcessor Table 图表处理器
type TableProcessor struct {
	Pagination *Pagination
}

// Process 处理 Table 数据
func (p *TableProcessor) Process(rows []map[string]any, dims []string, metrics []MetricConfig) (ChartQueryResponse, error) {
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

// Process 处理 Pie 数据
func (p *PieProcessor) Process(rows []map[string]any, dims []string, metrics []MetricConfig) (ChartQueryResponse, error) {
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

// Process 处理坐标轴图表数据
func (p *AxisProcessor) Process(rows []map[string]any, dims []string, metrics []MetricConfig) (ChartQueryResponse, error) {
	if len(rows) == 0 || len(dims) == 0 || len(metrics) == 0 {
		return &AxisResponse{
			XAxis:  []string{},
			Series: []AxisSeries{},
		}, nil
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

// ScatterProcessor Scatter 图表处理器
type ScatterProcessor struct{}

// Process 处理 Scatter 数据
func (p *ScatterProcessor) Process(rows []map[string]any, dims []string, metrics []MetricConfig) (ChartQueryResponse, error) {
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

// Process 处理 Pivot 数据
func (p *PivotProcessor) Process(rows []map[string]any, dims []string, metrics []MetricConfig) (ChartQueryResponse, error) {
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

// GetProcessor 获取对应的处理器
func GetProcessor(chartType ChartType) Processor {
	switch chartType {
	case ChartTypeTable:
		return &TableProcessor{}
	case ChartTypePie:
		return NewPieProcessor()
	case ChartTypeBar, ChartTypeLine, ChartTypeArea:
		return &AxisProcessor{}
	case ChartTypeScatter:
		return &ScatterProcessor{}
	case ChartTypePivot:
		return &PivotProcessor{}
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
