package query

import (
	"reflect"
	"testing"
)

// processor_radar_test.go — RadarProcessor（R-62）单元测试。
// radar 的 SQL 就是一条普通 GROUP BY + AGG，executor 走通用路径调 Process(rows,...)，
// 因此这里用合成 rows + ast 覆盖纯重塑逻辑：槽位解析（v2 GroupName + v1 位置回退）、
// indicator/series 首次出现顺序、跨系列 max（含全负边界）、缺失对补 0 保证等长对齐、
// 空 rows 与错误分支。

// radarAst 构造带 GroupName 的 ast：dims 与 DimensionExprs 按索引一一对应，
// Field 与 dims 相同（processor.Process 会校验 Field 对齐）。
func radarAst(dims []string, groupNames []string) *QueryAST {
	exprs := make([]DimensionExprAST, len(dims))
	for i, d := range dims {
		exprs[i] = DimensionExprAST{Field: d, Alias: d, GroupName: groupNames[i]}
	}
	return &QueryAST{Dimensions: dims, DimensionExprs: exprs}
}

// TestRadarProcessor_SingleSeries 覆盖无 series_group 的单系列路径：
// indicators 按首次出现序、series 恰一条（名=value 别名）、values 与 indicators 等长同序、
// max=该轴唯一值。v2 GroupName=indicators 走槽位解析。
func TestRadarProcessor_SingleSeries(t *testing.T) {
	ast := radarAst([]string{"ind"}, []string{SlotIndicators})
	metrics := []MetricConfig{{Field: "score", Agg: AggAvg, Alias: "avg_score"}}
	rows := []map[string]any{
		{"ind": "speed", "avg_score": float64(80)},
		{"ind": "power", "avg_score": float64(65)},
		{"ind": "range", "avg_score": float64(90)},
	}
	resp, err := (&RadarProcessor{}).Process(rows, []string{"ind"}, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := resp.(*RadarResponse)
	if !ok {
		t.Fatalf("resp type = %T, want *RadarResponse", resp)
	}
	wantIndicators := []RadarIndicator{{Name: "speed", Max: 80}, {Name: "power", Max: 65}, {Name: "range", Max: 90}}
	if !reflect.DeepEqual(r.Indicators, wantIndicators) {
		t.Errorf("indicators = %+v, want %+v", r.Indicators, wantIndicators)
	}
	if len(r.Series) != 1 {
		t.Fatalf("expected 1 series, got %d: %+v", len(r.Series), r.Series)
	}
	if r.Series[0].Name != "avg_score" {
		t.Errorf("series name = %q, want %q (value alias)", r.Series[0].Name, "avg_score")
	}
	wantValues := []float64{80, 65, 90}
	if !reflect.DeepEqual(r.Series[0].Values, wantValues) {
		t.Errorf("series values = %+v, want %+v", r.Series[0].Values, wantValues)
	}
}

// TestRadarProcessor_MultiSeriesViaSeriesGroup 覆盖带 series_group 的多系列：
// 每系列 values 与 indicators 等长同序；(series, indicator) 组合齐全时矩阵完整。
func TestRadarProcessor_MultiSeriesViaSeriesGroup(t *testing.T) {
	ast := radarAst([]string{"ind", "grp"}, []string{SlotIndicators, SlotSeriesGroup})
	metrics := []MetricConfig{{Field: "score", Agg: AggSum, Alias: "total"}}
	rows := []map[string]any{
		{"ind": "A", "grp": "p1", "total": float64(10)},
		{"ind": "B", "grp": "p1", "total": float64(20)},
		{"ind": "A", "grp": "p2", "total": float64(30)},
		{"ind": "B", "grp": "p2", "total": float64(40)},
	}
	resp, err := (&RadarProcessor{}).Process(rows, []string{"ind", "grp"}, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := resp.(*RadarResponse)
	// indicators 首次出现序 A → B；max 跨系列：A=max(10,30)=30，B=max(20,40)=40。
	if !reflect.DeepEqual(r.Indicators, []RadarIndicator{{Name: "A", Max: 30}, {Name: "B", Max: 40}}) {
		t.Errorf("indicators = %+v, want A/max30 B/max40", r.Indicators)
	}
	// series 首次出现序 p1 → p2；每系列 values 长度=2、顺序对齐 A,B。
	if len(r.Series) != 2 {
		t.Fatalf("want 2 series, got %+v", r.Series)
	}
	if r.Series[0].Name != "p1" || !reflect.DeepEqual(r.Series[0].Values, []float64{10, 20}) {
		t.Errorf("series[0] = %+v, want p1/[10,20]", r.Series[0])
	}
	if r.Series[1].Name != "p2" || !reflect.DeepEqual(r.Series[1].Values, []float64{30, 40}) {
		t.Errorf("series[1] = %+v, want p2/[30,40]", r.Series[1])
	}
}

// TestRadarProcessor_MissingPairFillsZeroAndKeepsAlignment 覆盖缺失对补 0：
// p1 无 B 轴数据、p2 无 A 轴数据 → 各自在缺失位取 0.0，两系列 values 仍与 indicators 等长同序。
// 保证 ECharts radar 多边形闭合（用 null 会断点）。
func TestRadarProcessor_MissingPairFillsZeroAndKeepsAlignment(t *testing.T) {
	ast := radarAst([]string{"ind", "grp"}, []string{SlotIndicators, SlotSeriesGroup})
	metrics := []MetricConfig{{Field: "v", Agg: AggSum, Alias: "v"}}
	rows := []map[string]any{
		{"ind": "A", "grp": "p1", "v": float64(5)},
		{"ind": "B", "grp": "p2", "v": float64(7)},
	}
	resp, err := (&RadarProcessor{}).Process(rows, []string{"ind", "grp"}, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := resp.(*RadarResponse)
	// indicators 首次序 A,B；max：A=max(5)=5，B=max(7)=7。
	if !reflect.DeepEqual(r.Indicators, []RadarIndicator{{Name: "A", Max: 5}, {Name: "B", Max: 7}}) {
		t.Errorf("indicators = %+v, want A/5 B/7", r.Indicators)
	}
	// p1 只在 A 有值 → values=[5, 0]；p2 只在 B 有值 → values=[0, 7]。
	if !reflect.DeepEqual(r.Series[0].Values, []float64{5, 0}) {
		t.Errorf("p1 values = %+v, want [5,0]", r.Series[0].Values)
	}
	if !reflect.DeepEqual(r.Series[1].Values, []float64{0, 7}) {
		t.Errorf("p2 values = %+v, want [0,7]", r.Series[1])
	}
	for _, s := range r.Series {
		if len(s.Values) != len(r.Indicators) {
			t.Errorf("series %q values len %d != indicators len %d", s.Name, len(s.Values), len(r.Indicators))
		}
	}
}

// TestRadarProcessor_AllNegativeMaxUsesFirstSeenInitialValue 覆盖全负轴 max 边界：
// 累积初值必须是首次出现值而非 0，否则 [-3,-7] 会被 0 抬高成 0（错误地把最大值
// 呈现为 0 而不是 -3），ECharts 会把整轴画成 0 长度。
func TestRadarProcessor_AllNegativeMaxUsesFirstSeenInitialValue(t *testing.T) {
	ast := radarAst([]string{"ind", "grp"}, []string{SlotIndicators, SlotSeriesGroup})
	metrics := []MetricConfig{{Field: "v", Agg: AggSum, Alias: "v"}}
	rows := []map[string]any{
		{"ind": "neg", "grp": "a", "v": float64(-3)},
		{"ind": "neg", "grp": "b", "v": float64(-7)},
	}
	resp, err := (&RadarProcessor{}).Process(rows, []string{"ind", "grp"}, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := resp.(*RadarResponse)
	if len(r.Indicators) != 1 {
		t.Fatalf("want 1 indicator, got %+v", r.Indicators)
	}
	if r.Indicators[0].Name != "neg" || r.Indicators[0].Max != -3 {
		t.Errorf("indicator = %+v, want {neg, -3} (真实最大负值, 不被 0 抬高)", r.Indicators[0])
	}
}

// TestRadarProcessor_V1FallbackReal 覆盖 v1 位置回退（ast=nil）：v1 平铺请求无
// GroupName 时按位置解析 dims[0]=indicators、dims[1]=series_group，产出与 v2 同形。
// 让后端可先于前端 v2 接线上线。
func TestRadarProcessor_V1FallbackReal(t *testing.T) {
	metrics := []MetricConfig{{Field: "v", Agg: AggSum, Alias: "v"}}
	rows := []map[string]any{
		{"ind": "A", "grp": "p1", "v": float64(1)},
		{"ind": "B", "grp": "p1", "v": float64(2)},
		{"ind": "A", "grp": "p2", "v": float64(3)},
		{"ind": "B", "grp": "p2", "v": float64(4)},
	}
	resp, err := (&RadarProcessor{}).Process(rows, []string{"ind", "grp"}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := resp.(*RadarResponse)
	// 位置回退：dims[0]=indicators → 轴序 A,B；dims[1]=series_group → 系列 p1,p2。
	if !reflect.DeepEqual(r.Indicators, []RadarIndicator{{Name: "A", Max: 3}, {Name: "B", Max: 4}}) {
		t.Errorf("v1 indicators = %+v, want A/3 B/4", r.Indicators)
	}
	if len(r.Series) != 2 {
		t.Fatalf("v1 want 2 series, got %+v", r.Series)
	}
	if !reflect.DeepEqual(r.Series[0].Values, []float64{1, 2}) ||
		!reflect.DeepEqual(r.Series[1].Values, []float64{3, 4}) {
		t.Errorf("v1 series values = %+v, want p1:[1,2] p2:[3,4]", r.Series)
	}
}

// TestRadarProcessor_Errors 覆盖显式错误分支：dims 空（无 indicator 维度）与 metrics 空
// （无 value 指标）必须返回 error，不得静默产出错误形状。
func TestRadarProcessor_Errors(t *testing.T) {
	metrics := []MetricConfig{{Field: "v", Alias: "v"}}
	if _, err := (&RadarProcessor{}).Process([]map[string]any{{"v": float64(1)}}, nil, metrics, nil); err == nil {
		t.Error("expected error for empty dims")
	}
	if _, err := (&RadarProcessor{}).Process(nil, []string{"ind"}, nil, nil); err == nil {
		t.Error("expected error for empty metrics")
	}
}

// TestRadarProcessor_EmptyRows 覆盖空 rows → 空切片（非 nil），响应集合字段序列化为 [] 而不是 null。
func TestRadarProcessor_EmptyRows(t *testing.T) {
	ast := radarAst([]string{"ind"}, []string{SlotIndicators})
	metrics := []MetricConfig{{Field: "v", Alias: "v"}}
	resp, err := (&RadarProcessor{}).Process([]map[string]any{}, []string{"ind"}, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := resp.(*RadarResponse)
	if r.Indicators == nil || r.Series == nil {
		t.Errorf("empty slices must be non-nil: indicators=%v series=%v", r.Indicators, r.Series)
	}
	if len(r.Indicators) != 0 || len(r.Series) != 0 {
		t.Errorf("want 0-length slices, got %+v", r)
	}
}

// TestGetProcessor_Radar 验证 GetProcessor(radar) 返回 *RadarProcessor，
// 不误落 AxisProcessor 兜底（否则前端拿到的 data 形状会是 {x_axis,series} 而非 {indicators,series}）。
func TestGetProcessor_Radar(t *testing.T) {
	if _, ok := GetProcessor(ChartTypeRadar).(*RadarProcessor); !ok {
		t.Errorf("GetProcessor(radar) = %T, want *RadarProcessor", GetProcessor(ChartTypeRadar))
	}
}

// TestRadarProcessor_NonNumericValueFallsBackToZero 覆盖 value 非数值（nil/字符串）时
// metricValue 兜底 0、不中断整张图。
func TestRadarProcessor_NonNumericValueFallsBackToZero(t *testing.T) {
	ast := radarAst([]string{"ind"}, []string{SlotIndicators})
	metrics := []MetricConfig{{Field: "v", Alias: "v"}}
	rows := []map[string]any{
		{"ind": "A", "v": float64(1)},
		{"ind": "B", "v": nil}, // 不可转数值 → 0
	}
	resp, err := (&RadarProcessor{}).Process(rows, []string{"ind"}, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := resp.(*RadarResponse)
	if r.Series[0].Values[1] != 0 || r.Series[0].Values[0] != 1 {
		t.Errorf("values = %+v, want [1,0]", r.Series[0].Values)
	}
	// max 累积：A 首见值 1、B 首见值 0 → max A=1、B=0（0 是真实最大值，非哨兵）。
	if r.Indicators[1].Name != "B" || r.Indicators[1].Max != 0 {
		t.Errorf("B indicator = %+v, want Max=0", r.Indicators[1])
	}
}
