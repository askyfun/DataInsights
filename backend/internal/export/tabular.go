// Package export 把图表查询的**已处理负载**（processor 输出，即用户看到的那些行）
// 摊平成二维表，并编码为 CSV / XLSX 流。它是 issue #120「下载数据」的唯一实现点：
// 导出必须与图表展示同源（同一个 POST /api/charts/query 管道），不允许另写一条
// 绕过 processor（TopN / 同环比 / 透视重塑）的取数路径。
package export

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"data-insights/internal/domain/entity"
	"data-insights/internal/query"
)

// Table 是导出的中间形态：列名（与图表展示的列名逐字一致）+ 行。
// 单元格保留原始类型（string/float64/...），由编码器决定如何落盘——
// CSV 里数字写成裸数字、XLSX 里写成数值单元格，而不是全部字符串化。
type Table struct {
	Columns []string
	Rows    [][]any
}

// Tabularize 把 processor 输出摊平成二维表。
// req 只用于给「响应本身不带列名」的形状补列名（axis/pie/scatter 的维度与指标名）；
// names 是列 id → 列名映射：v1 wire 的 dims 发的是列的稳定 id（0000i52x），而展示
// 层与 table 响应用的都是列名，导出必须翻译一次才算「列名与图表展示一致」。
// 认不出的形状返回 nil（调用方据此 400，绝不导出一份空文件冒充成功）。
func Tabularize(req *entity.ChartQueryRequest, data any, names map[string]string) *Table {
	// 空负载（processor 对无数据/无指标返回 nil interface 或空结构）不是错误：
	// 导出只有表头的文件，让用户拿到稳定列名。
	if data == nil {
		return &Table{Columns: columnsFromRequest(req, names)}
	}
	switch v := data.(type) {
	case *query.TableResponse:
		return fromMaps(v.Columns, v.Data)
	case query.TableResponse:
		return fromMaps(v.Columns, v.Data)
	case *query.PivotResponse:
		return fromMaps(v.Columns, v.Data)
	case query.PivotResponse:
		return fromMaps(v.Columns, v.Data)
	case *query.PivotResponseV2:
		return pivotV2(v)
	case *query.AxisResponse:
		return axis(req, v, names)
	case *query.PieResponse:
		return pie(req, v, names)
	case *query.ScatterResponse:
		return scatter(req, v)
	case *query.HistogramResponse:
		return histogram(v)
	case *query.RadarResponse:
		return radar(v)
	case *query.BoxplotResponse:
		return boxplot(v)
	case *query.KpiResponse:
		return &Table{Columns: []string{"label", "value"}, Rows: [][]any{{v.Label, v.Value}}}
	case []map[string]any:
		// legacy 裸行（fallback 预览）：按首个出现的键并集出列，顺序稳定。
		return fromMaps(unionColumns(v), v)
	default:
		return nil
	}
}

// columnsFromRequest 是「无数据」时的列名兜底：与请求里的维度/指标同序，
// 与有数据时的 TableResponse 列序一致（维度在前、指标别名在后）。
func metricAlias(m entity.MetricConfig) string {
	if m.Alias != "" {
		return m.Alias
	}
	return m.Field
}

func columnsFromRequest(req *entity.ChartQueryRequest, names map[string]string) []string {
	if req == nil {
		return nil
	}
	columns := make([]string, 0, len(req.Dims)+len(req.Metrics))
	for _, dim := range req.Dims {
		columns = append(columns, resolveName(dim, names))
	}
	for _, m := range req.Metrics {
		columns = append(columns, metricAlias(m))
	}
	return columns
}

// resolveName 优先用调用方给的列名（导出请求携带 id→name），缺省原样返回 id。
func resolveName(id string, names map[string]string) string {
	if name, ok := names[id]; ok && name != "" {
		return name
	}
	return id
}

func fromMaps(columns []string, rows []map[string]any) *Table {
	out := &Table{Columns: append([]string(nil), columns...), Rows: make([][]any, 0, len(rows))}
	for _, row := range rows {
		cells := make([]any, len(out.Columns))
		for i, col := range out.Columns {
			cells[i] = row[col]
		}
		out.Rows = append(out.Rows, cells)
	}
	return out
}

// unionColumns 按首次出现顺序收集所有键，再排序补上未在首行出现的键？——
// 不排序：首行的键序即展示序（原始 JSON 已是列顺序），后续行多出的键追加在尾部。
func unionColumns(rows []map[string]any) []string {
	seen := make(map[string]bool)
	columns := make([]string, 0)
	for _, row := range rows {
		keys := make([]string, 0, len(row))
		for k := range row {
			keys = append(keys, k)
		}
		// map 迭代无序：单行内部按字典序，跨行按首次出现顺序（首行决定主列序）。
		sort.Strings(keys)
		for _, k := range keys {
			if !seen[k] {
				seen[k] = true
				columns = append(columns, k)
			}
		}
	}
	return columns
}

func pivotV2(p *query.PivotResponseV2) *Table {
	columns := make([]string, 0, len(p.RowHeaders)+len(p.ColHeaders)*len(p.MetricNames))
	columns = append(columns, p.RowHeaders...)
	for _, ch := range p.ColHeaders {
		for _, mn := range p.MetricNames {
			columns = append(columns, ch+"|"+mn)
		}
	}
	rows := make([][]any, 0, len(p.Cells))
	for _, cell := range p.Cells {
		row := make([]any, 0, len(columns))
		for _, h := range p.RowHeaders {
			row = append(row, cellValueByHeader(p, cell, h))
		}
		for _, ch := range p.ColHeaders {
			for _, mn := range p.MetricNames {
				row = append(row, cell.Values[ch+"|"+mn])
			}
		}
		rows = append(rows, row)
	}
	return &Table{Columns: columns, Rows: rows}
}

// cellValueByHeader 取行头单元格：v2 PivotRow.RowKey 与 RowHeaders 同序同长。
func cellValueByHeader(p *query.PivotResponseV2, cell query.PivotRow, header string) any {
	for i, h := range p.RowHeaders {
		if h == header && i < len(cell.RowKey) {
			return cell.RowKey[i]
		}
	}
	return nil
}

func axis(req *entity.ChartQueryRequest, v *query.AxisResponse, names map[string]string) *Table {
	xName := "x_axis"
	if req != nil && len(req.Dims) > 0 {
		xName = resolveName(req.Dims[0], names)
	}
	columns := []string{xName}
	for _, s := range v.Series {
		columns = append(columns, s.Name)
	}
	rows := make([][]any, 0, len(v.XAxis))
	for i, x := range v.XAxis {
		row := make([]any, 0, len(columns))
		row = append(row, x)
		for _, s := range v.Series {
			if i < len(s.Data) {
				row = append(row, s.Data[i])
			} else {
				row = append(row, nil)
			}
		}
		rows = append(rows, row)
	}
	return &Table{Columns: columns, Rows: rows}
}

func pie(req *entity.ChartQueryRequest, v *query.PieResponse, names map[string]string) *Table {
	name := "name"
	if req != nil && len(req.Dims) > 0 {
		name = resolveName(req.Dims[0], names)
	}
	t := &Table{Columns: []string{name, "value", "percentage"}}
	for _, item := range v.Data {
		t.Rows = append(t.Rows, []any{item.Name, item.Value, item.Percentage})
	}
	if v.Other != nil {
		t.Rows = append(t.Rows, []any{v.Other.Name, v.Other.Value, v.Other.Percentage})
	}
	return t
}

func scatter(req *entity.ChartQueryRequest, v *query.ScatterResponse) *Table {
	xName, yName := "x", "y"
	if req != nil && len(req.Metrics) > 0 {
		xName = metricAlias(req.Metrics[0])
	}
	if req != nil && len(req.Metrics) > 1 {
		yName = metricAlias(req.Metrics[1])
	}
	t := &Table{Columns: []string{xName, yName}}
	for _, point := range v.Data {
		if len(point) < 2 {
			t.Rows = append(t.Rows, []any{point[0], nil})
			continue
		}
		t.Rows = append(t.Rows, []any{point[0], point[1]})
	}
	return t
}

func histogram(v *query.HistogramResponse) *Table {
	t := &Table{Columns: []string{"bin_start", "bin_end", "count"}}
	if len(v.Groups) == 0 {
		for _, bin := range v.Bins {
			t.Rows = append(t.Rows, []any{bin.BinStart, bin.BinEnd, bin.Count})
		}
		return t
	}
	// 分组直方图：series 列在前，分箱计数展开成长表，避免为每个分组各出一组列
	// （列名会随维度值变化，表头不稳定）。
	t.Columns = []string{"series", "bin_start", "bin_end", "count"}
	for _, g := range v.Groups {
		for _, bin := range g.Bins {
			t.Rows = append(t.Rows, []any{g.Name, bin.BinStart, bin.BinEnd, bin.Count})
		}
	}
	return t
}

func radar(v *query.RadarResponse) *Table {
	columns := []string{"series"}
	for _, ind := range v.Indicators {
		columns = append(columns, ind.Name)
	}
	t := &Table{Columns: columns}
	for _, s := range v.Series {
		row := make([]any, 0, len(columns))
		row = append(row, s.Name)
		for i := range v.Indicators {
			if i < len(s.Values) {
				row = append(row, s.Values[i])
			} else {
				row = append(row, nil)
			}
		}
		t.Rows = append(t.Rows, row)
	}
	return t
}

func boxplot(v *query.BoxplotResponse) *Table {
	outliers := ""
	for i, o := range v.Outliers {
		if i > 0 {
			outliers += ";"
		}
		outliers += fmt.Sprint(o)
	}
	return &Table{
		Columns: []string{
			"whisker_low", "q1", "median", "q3", "whisker_high",
			"outlier_total", "truncated", "outliers",
		},
		Rows: [][]any{{
			v.WhiskerLow, v.Q1, v.Median, v.Q3, v.WhiskerHigh,
			v.OutlierTotal, v.Truncated, outliers,
		}},
	}
}

// CellText 把单元格转成文本（CSV 用）。数字保持裸形态（不加上千分位、不四舍五入），
// 复杂值走 JSON 编码，时间统一成与查询响应一致的格式。
func CellText(v any) string {
	if v == nil {
		return ""
	}
	switch tv := v.(type) {
	case string:
		return tv
	case []byte:
		return string(tv)
	case time.Time:
		if tv.Hour() == 0 && tv.Minute() == 0 && tv.Second() == 0 {
			return tv.Format("2006-01-02")
		}
		return tv.Format("2006-01-02 15:04:05")
	case bool:
		if tv {
			return "true"
		}
		return "false"
	case float64:
		return formatFloat(tv)
	case float32:
		return formatFloat(float64(tv))
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(tv)
	case json.Number:
		return tv.String()
	}
	if raw, err := json.Marshal(v); err == nil {
		if len(raw) > 0 && raw[0] == '"' {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				return s
			}
		}
		return string(raw)
	}
	return fmt.Sprint(v)
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
