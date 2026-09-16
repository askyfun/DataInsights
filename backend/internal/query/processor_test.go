package query

import (
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestAxisProcessor_EmptyData 验证空行数据返回空响应
func TestAxisProcessor_EmptyData(t *testing.T) {
	p := &AxisProcessor{}
	resp, err := p.Process([]map[string]any{}, []string{"category"}, []MetricConfig{{Field: "value", Agg: AggSum}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	if len(axisResp.XAxis) != 0 {
		t.Errorf("expected empty XAxis, got %v", axisResp.XAxis)
	}
	if len(axisResp.Series) != 0 {
		t.Errorf("expected empty Series, got %v", axisResp.Series)
	}
}

// TestAxisProcessor_EmptyDims 验证空维度返回空响应
func TestAxisProcessor_EmptyDims(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{{"category": "A", "value": 100}}
	resp, err := p.Process(rows, []string{}, []MetricConfig{{Field: "value", Agg: AggSum}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	if len(axisResp.XAxis) != 0 {
		t.Errorf("expected empty XAxis, got %v", axisResp.XAxis)
	}
	if len(axisResp.Series) != 0 {
		t.Errorf("expected empty Series, got %v", axisResp.Series)
	}
}

// TestAxisProcessor_EmptyMetrics 验证空指标返回空响应
func TestAxisProcessor_EmptyMetrics(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{{"category": "A", "value": 100}}
	resp, err := p.Process(rows, []string{"category"}, []MetricConfig{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	if len(axisResp.XAxis) != 0 {
		t.Errorf("expected empty XAxis, got %v", axisResp.XAxis)
	}
	if len(axisResp.Series) != 0 {
		t.Errorf("expected empty Series, got %v", axisResp.Series)
	}
}

// TestAxisProcessor_SingleDim_SingleMetric 验证单维度单指标的基本输出
func TestAxisProcessor_SingleDim_SingleMetric(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"category": "A", "value": 100},
		{"category": "B", "value": 200},
	}
	metrics := []MetricConfig{{Field: "value", Agg: AggSum}}
	resp, err := p.Process(rows, []string{"category"}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	if len(axisResp.XAxis) != 2 {
		t.Fatalf("expected XAxis length 2, got %d", len(axisResp.XAxis))
	}
	if axisResp.XAxis[0] != "A" {
		t.Errorf("expected XAxis[0]='A', got %q", axisResp.XAxis[0])
	}
	if axisResp.XAxis[1] != "B" {
		t.Errorf("expected XAxis[1]='B', got %q", axisResp.XAxis[1])
	}

	if len(axisResp.Series) != 1 {
		t.Fatalf("expected Series length 1, got %d", len(axisResp.Series))
	}
	// 无 alias，ResolveAlias 返回 Field
	if axisResp.Series[0].Name != "value" {
		t.Errorf("expected series name 'value', got %q", axisResp.Series[0].Name)
	}
	if len(axisResp.Series[0].Data) != 2 {
		t.Fatalf("expected series data length 2, got %d", len(axisResp.Series[0].Data))
	}
}

// TestAxisProcessor_SingleDim_MultipleMetrics 验证多指标输出
func TestAxisProcessor_SingleDim_MultipleMetrics(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"category": "A", "revenue": 100, "cost": 50},
		{"category": "B", "revenue": 200, "cost": 80},
	}
	metrics := []MetricConfig{
		{Field: "revenue", Agg: AggSum, Alias: "total_revenue"},
		{Field: "cost", Agg: AggSum, Alias: "total_cost"},
	}
	resp, err := p.Process(rows, []string{"category"}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	if len(axisResp.Series) != 2 {
		t.Fatalf("expected Series length 2, got %d", len(axisResp.Series))
	}
	if axisResp.Series[0].Name != "total_revenue" {
		t.Errorf("expected series[0] name 'total_revenue', got %q", axisResp.Series[0].Name)
	}
	if axisResp.Series[1].Name != "total_cost" {
		t.Errorf("expected series[1] name 'total_cost', got %q", axisResp.Series[1].Name)
	}
}

// TestAxisProcessor_TypeConversion 验证不同类型指标值保持原始类型
func TestAxisProcessor_TypeConversion(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"category": "A", "value": float64(100.5)},
		{"category": "B", "value": int64(200)},
		{"category": "C", "value": int(300)},
	}
	metrics := []MetricConfig{{Field: "value", Agg: AggSum}}
	resp, err := p.Process(rows, []string{"category"}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	data := axisResp.Series[0].Data
	if len(data) != 3 {
		t.Fatalf("expected data length 3, got %d", len(data))
	}

	// 值保持原始类型，不做转换
	if _, ok := data[0].(float64); !ok {
		t.Errorf("expected data[0] type float64, got %T", data[0])
	}
	if _, ok := data[1].(int64); !ok {
		t.Errorf("expected data[1] type int64, got %T", data[1])
	}
	if _, ok := data[2].(int); !ok {
		t.Errorf("expected data[2] type int, got %T", data[2])
	}
}

// TestAxisProcessor_WithAlias 验证指标别名作为 series name
func TestAxisProcessor_WithAlias(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"category": "A", "total_revenue": 500},
	}
	metrics := []MetricConfig{{Field: "revenue", Agg: AggSum, Alias: "total_revenue"}}
	resp, err := p.Process(rows, []string{"category"}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	if len(axisResp.Series) != 1 {
		t.Fatalf("expected Series length 1, got %d", len(axisResp.Series))
	}
	if axisResp.Series[0].Name != "total_revenue" {
		t.Errorf("expected series name 'total_revenue', got %q", axisResp.Series[0].Name)
	}
}

// TestAxisProcessor_NullDimension 验证维度值为 nil 时转为空字符串
func TestAxisProcessor_NullDimension(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"category": nil, "value": 100},
		{"category": "B", "value": 200},
	}
	metrics := []MetricConfig{{Field: "value", Agg: AggSum}}
	resp, err := p.Process(rows, []string{"category"}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	if len(axisResp.XAxis) != 2 {
		t.Fatalf("expected XAxis length 2, got %d", len(axisResp.XAxis))
	}
	if axisResp.XAxis[0] != "" {
		t.Errorf("expected XAxis[0]='' for nil dim, got %q", axisResp.XAxis[0])
	}
	if axisResp.XAxis[1] != "B" {
		t.Errorf("expected XAxis[1]='B', got %q", axisResp.XAxis[1])
	}
}

// TestAxisProcessor_MultiDims_TwoDims 验证双维度：第一个维度为 X 轴，第二个维度每个值一条线
func TestAxisProcessor_MultiDims_TwoDims(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"date": "2024-01", "city": "Beijing", "sales": 100},
		{"date": "2024-01", "city": "Shanghai", "sales": 200},
		{"date": "2024-02", "city": "Beijing", "sales": 150},
		{"date": "2024-02", "city": "Shanghai", "sales": 250},
	}
	metrics := []MetricConfig{{Field: "sales", Agg: AggSum}}
	resp, err := p.Process(rows, []string{"date", "city"}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	// X 轴应该是第一个维度的去重值
	if len(axisResp.XAxis) != 2 {
		t.Fatalf("expected XAxis length 2, got %d: %v", len(axisResp.XAxis), axisResp.XAxis)
	}
	if axisResp.XAxis[0] != "2024-01" || axisResp.XAxis[1] != "2024-02" {
		t.Errorf("expected XAxis=['2024-01','2024-02'], got %v", axisResp.XAxis)
	}

	// 每个第二个维度值一条线
	if len(axisResp.Series) != 2 {
		t.Fatalf("expected Series length 2, got %d", len(axisResp.Series))
	}

	// series 按出现顺序：Beijing, Shanghai
	seriesMap := map[string][]any{}
	for _, s := range axisResp.Series {
		seriesMap[s.Name] = s.Data
	}

	beijingData, ok := seriesMap["Beijing"]
	if !ok {
		t.Fatalf("expected series 'Beijing', got names: %v", getSeriesNames(axisResp.Series))
	}
	if beijingData[0] != 100 || beijingData[1] != 150 {
		t.Errorf("expected Beijing data=[100,150], got %v", beijingData)
	}

	shanghaiData, ok := seriesMap["Shanghai"]
	if !ok {
		t.Fatalf("expected series 'Shanghai'")
	}
	if shanghaiData[0] != 200 || shanghaiData[1] != 250 {
		t.Errorf("expected Shanghai data=[200,250], got %v", shanghaiData)
	}
}

// TestPieProcessor_WithMergeOtherBelowRatio 验证饼图可按比例阈值合并“其他”。
func TestPieProcessor_WithMergeOtherBelowRatio(t *testing.T) {
	p := NewPieProcessor()
	p.MergeOtherBelowRatio = 10

	rows := []map[string]any{
		{"category": "A", "value": 80.0},
		{"category": "B", "value": 15.0},
		{"category": "C", "value": 5.0},
	}
	metrics := []MetricConfig{{Field: "value", Agg: AggSum}}

	resp, err := p.Process(rows, []string{"category"}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pieResp, ok := resp.(*PieResponse)
	if !ok {
		t.Fatalf("expected *PieResponse, got %T", resp)
	}

	if len(pieResp.Data) != 3 {
		t.Fatalf("expected merged pie data length 3, got %d", len(pieResp.Data))
	}

	last := pieResp.Data[2]
	if last.Name != "其他" {
		t.Fatalf("expected merged item name '其他', got %q", last.Name)
	}
	if last.Value != 5 {
		t.Fatalf("expected merged item value 5, got %v", last.Value)
	}
	if last.Percentage != 5 {
		t.Fatalf("expected merged item percentage 5, got %v", last.Percentage)
	}
}

// TestPieProcessor_PgNumericValues 先红：SUM(numeric) 经 pgx 返回
// pgtype.Numeric 结构体，此前内联 switch 不识别导致饼图 value 全部归零。
func TestPieProcessor_PgNumericValues(t *testing.T) {
	p := NewPieProcessor()
	rows := []map[string]any{
		{"category": "城镇", "value": pgtype.Numeric{Int: big.NewInt(95380), Exp: 0, Valid: true}},
		{"category": "乡村", "value": pgtype.Numeric{Int: big.NewInt(45109), Exp: 0, Valid: true}},
	}
	metrics := []MetricConfig{{Field: "value", Agg: AggSum}}

	resp, err := p.Process(rows, []string{"category"}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pieResp, ok := resp.(*PieResponse)
	if !ok {
		t.Fatalf("expected *PieResponse, got %T", resp)
	}
	if len(pieResp.Data) != 2 {
		t.Fatalf("expected 2 pie items, got %d", len(pieResp.Data))
	}
	if pieResp.Data[0].Value != 95380 || pieResp.Data[1].Value != 45109 {
		t.Fatalf("expected values 95380/45109, got %v/%v", pieResp.Data[0].Value, pieResp.Data[1].Value)
	}
	if pieResp.Data[0].Percentage == 0 {
		t.Fatalf("percentage must be non-zero, got %v", pieResp.Data[0].Percentage)
	}
}

// TestAxisProcessor_MultiDims_ThreeDims 验证三个维度：后续维度值用 " - " 拼接
func TestAxisProcessor_MultiDims_ThreeDims(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"date": "2024-01", "country": "CN", "city": "Beijing", "sales": 100},
		{"date": "2024-01", "country": "CN", "city": "Shanghai", "sales": 200},
		{"date": "2024-01", "country": "US", "city": "NY", "sales": 150},
	}
	metrics := []MetricConfig{{Field: "sales", Agg: AggSum}}
	resp, err := p.Process(rows, []string{"date", "country", "city"}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	if len(axisResp.XAxis) != 1 {
		t.Fatalf("expected XAxis length 1, got %d", len(axisResp.XAxis))
	}

	// 三个维度时，series name 应为 "CN - Beijing", "CN - Shanghai", "US - NY"
	if len(axisResp.Series) != 3 {
		t.Fatalf("expected Series length 3, got %d", len(axisResp.Series))
	}

	names := getSeriesNames(axisResp.Series)
	expected := []string{"CN - Beijing", "CN - Shanghai", "US - NY"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("expected series[%d] name=%q, got %q", i, expected[i], name)
		}
	}
}

// TestAxisProcessor_MultiDims_MultipleMetrics 验证多维度多指标：每个 metric×dim 组合一条线
func TestAxisProcessor_MultiDims_MultipleMetrics(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"date": "2024-01", "city": "Beijing", "revenue": 100, "cost": 50},
		{"date": "2024-01", "city": "Shanghai", "revenue": 200, "cost": 80},
	}
	metrics := []MetricConfig{
		{Field: "revenue", Agg: AggSum},
		{Field: "cost", Agg: AggSum},
	}
	resp, err := p.Process(rows, []string{"date", "city"}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	// 2 metrics × 2 cities = 4 series
	if len(axisResp.Series) != 4 {
		t.Fatalf("expected Series length 4, got %d: %v", len(axisResp.Series), getSeriesNames(axisResp.Series))
	}

	names := getSeriesNames(axisResp.Series)
	expected := []string{"revenue - Beijing", "revenue - Shanghai", "cost - Beijing", "cost - Shanghai"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("expected series[%d] name=%q, got %q", i, expected[i], name)
		}
	}
}

func getSeriesNames(series []AxisSeries) []string {
	names := make([]string, len(series))
	for i, s := range series {
		names[i] = s.Name
	}
	return names
}

// TestGetProcessor_BarLineArea 验证 GetProcessor 返回正确的处理器类型
func TestGetProcessor_BarLineArea(t *testing.T) {
	cases := []struct {
		name      string
		chartType ChartType
	}{
		{"bar", ChartTypeBar},
		{"line", ChartTypeLine},
		{"area", ChartTypeArea},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			processor := GetProcessor(tc.chartType)
			if processor == nil {
				t.Fatal("expected non-nil processor")
			}
			if _, ok := processor.(*AxisProcessor); !ok {
				t.Fatalf("expected *AxisProcessor, got %T", processor)
			}
		})
	}
}

// TestScatterProcessor_TwoMetrics 验证散点图使用 metrics[0] 作为 X、metrics[1] 作为 Y
func TestScatterProcessor_TwoMetrics(t *testing.T) {
	p := &ScatterProcessor{}
	rows := []map[string]any{
		{"total_revenue": 100.0, "total_cost": 50.0},
		{"total_revenue": 200.0, "total_cost": 80.0},
		{"total_revenue": 150.0, "total_cost": 60.0},
	}
	metrics := []MetricConfig{
		{Field: "revenue", Agg: AggSum, Alias: "total_revenue"},
		{Field: "cost", Agg: AggSum, Alias: "total_cost"},
	}
	resp, err := p.Process(rows, []string{}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	scatterResp, ok := resp.(*ScatterResponse)
	if !ok {
		t.Fatalf("expected *ScatterResponse, got %T", resp)
	}

	if len(scatterResp.Data) != 3 {
		t.Fatalf("expected 3 data points, got %d", len(scatterResp.Data))
	}

	// metrics[0] (total_revenue) → X, metrics[1] (total_cost) → Y
	expected := [][]float64{{100, 50}, {200, 80}, {150, 60}}
	for i, pt := range scatterResp.Data {
		if len(pt) != 2 {
			t.Fatalf("point %d: expected length 2, got %d", i, len(pt))
		}
		if pt[0] != expected[i][0] || pt[1] != expected[i][1] {
			t.Errorf("point %d: expected [%v, %v], got [%v, %v]", i, expected[i][0], expected[i][1], pt[0], pt[1])
		}
	}
}

// TestScatterProcessor_EmptyRows 验证空行返回空数据
func TestScatterProcessor_EmptyRows(t *testing.T) {
	p := &ScatterProcessor{}
	metrics := []MetricConfig{
		{Field: "revenue", Agg: AggSum},
		{Field: "cost", Agg: AggSum},
	}
	resp, err := p.Process([]map[string]any{}, []string{}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	scatterResp := resp.(*ScatterResponse)
	if len(scatterResp.Data) != 0 {
		t.Errorf("expected empty data, got %v", scatterResp.Data)
	}
}

// TestScatterProcessor_NumericStringValues PG numeric 列经 pgx 解码为字符串，
// toFloat64 必须能解析，否则散点图对 numeric 列永远返回空。
func TestScatterProcessor_NumericStringValues(t *testing.T) {
	p := &ScatterProcessor{}
	rows := []map[string]any{
		{"x_value": "100.5", "y_value": "50.25"},
		{"x_value": "200", "y_value": "80"},
	}
	metrics := []MetricConfig{
		{Field: "revenue", Agg: AggSum, Alias: "x_value"},
		{Field: "cost", Agg: AggSum, Alias: "y_value"},
	}
	resp, err := p.Process(rows, []string{}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	scatterResp := resp.(*ScatterResponse)
	if len(scatterResp.Data) != 2 {
		t.Fatalf("expected 2 data points, got %d", len(scatterResp.Data))
	}
	if scatterResp.Data[0][0] != 100.5 || scatterResp.Data[0][1] != 50.25 {
		t.Errorf("point 0: expected [100.5 50.25], got %v", scatterResp.Data[0])
	}
	if scatterResp.Data[1][0] != 200 || scatterResp.Data[1][1] != 80 {
		t.Errorf("point 1: expected [200 80], got %v", scatterResp.Data[1])
	}
}

// TestScatterProcessor_PgNumericValues pgx 对 numeric 列返回 pgtype.Numeric
// 结构体（如 SUM(numeric)），toFloat64 必须支持，否则聚合散点永远为空。
func TestScatterProcessor_PgNumericValues(t *testing.T) {
	p := &ScatterProcessor{}
	x := pgtype.Numeric{Int: big.NewInt(3773000), Exp: -2, Valid: true} // 37730.00
	y := pgtype.Numeric{Int: big.NewInt(250), Exp: 0, Valid: true}     // 250
	rows := []map[string]any{
		{"amount": x, "quantity": y},
	}
	metrics := []MetricConfig{
		{Field: "amount", Agg: AggSum, Alias: "amount"},
		{Field: "quantity", Agg: AggSum, Alias: "quantity"},
	}
	resp, err := p.Process(rows, []string{"region"}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	scatterResp := resp.(*ScatterResponse)
	if len(scatterResp.Data) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(scatterResp.Data))
	}
	if scatterResp.Data[0][0] != 37730 || scatterResp.Data[0][1] != 250 {
		t.Errorf("expected [37730 250], got %v", scatterResp.Data[0])
	}
}

// TestScatterProcessor_LessThanTwoMetrics 验证少于 2 个指标返回空数据
func TestScatterProcessor_LessThanTwoMetrics(t *testing.T) {
	p := &ScatterProcessor{}
	rows := []map[string]any{{"value": 100}}
	metrics := []MetricConfig{{Field: "value", Agg: AggSum}}
	resp, err := p.Process(rows, []string{}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	scatterResp := resp.(*ScatterResponse)
	if len(scatterResp.Data) != 0 {
		t.Errorf("expected empty data for single metric, got %v", scatterResp.Data)
	}
}

// TestScatterProcessor_DimsIgnored 验证 dims 参数不影响散点数据
func TestScatterProcessor_DimsIgnored(t *testing.T) {
	p := &ScatterProcessor{}
	rows := []map[string]any{
		{"city": "Beijing", "revenue": 100.0, "cost": 50.0},
	}
	metrics := []MetricConfig{
		{Field: "revenue", Agg: AggSum},
		{Field: "cost", Agg: AggSum},
	}
	resp, err := p.Process(rows, []string{"city"}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	scatterResp := resp.(*ScatterResponse)
	if len(scatterResp.Data) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(scatterResp.Data))
	}
	if scatterResp.Data[0][0] != 100 || scatterResp.Data[0][1] != 50 {
		t.Errorf("expected [100, 50], got %v", scatterResp.Data[0])
	}
}

// TestScatterProcessor_NonNumericSkipped 验证非数值行被跳过
func TestScatterProcessor_NonNumericSkipped(t *testing.T) {
	p := &ScatterProcessor{}
	rows := []map[string]any{
		{"x": 10.0, "y": 20.0},
		{"x": "not_a_number", "y": 30.0},
		{"x": 40.0, "y": 50.0},
	}
	metrics := []MetricConfig{
		{Field: "x", Agg: AggSum},
		{Field: "y", Agg: AggSum},
	}
	resp, err := p.Process(rows, []string{}, metrics, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	scatterResp := resp.(*ScatterResponse)
	if len(scatterResp.Data) != 2 {
		t.Fatalf("expected 2 data points, got %d", len(scatterResp.Data))
	}
}

// slotAST 构造只带槽位信息的 DimensionExprs（Field/GroupName），供 AxisProcessor 槽位感知测试使用。
func slotAST(dims []string, groupNames []string) *QueryAST {
	exprs := make([]DimensionExprAST, len(dims))
	for i, d := range dims {
		exprs[i] = DimensionExprAST{Field: d, GroupName: groupNames[i]}
	}
	return &QueryAST{DimensionExprs: exprs}
}

// TestAxisProcessor_SlotAware_ColorGroupBasic 验证 x_axis/color_group 槽位名驱动的基础分组，
// 结果应与等价的位置推断（dims=[date,city]，无槽位信息）完全一致。
func TestAxisProcessor_SlotAware_ColorGroupBasic(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"date": "2024-01", "city": "Beijing", "sales": 100},
		{"date": "2024-01", "city": "Shanghai", "sales": 200},
		{"date": "2024-02", "city": "Beijing", "sales": 150},
		{"date": "2024-02", "city": "Shanghai", "sales": 250},
	}
	metrics := []MetricConfig{{Field: "sales", Agg: AggSum}}
	dims := []string{"date", "city"}
	ast := slotAST(dims, []string{SlotXAxis, SlotColorGroup})

	resp, err := p.Process(rows, dims, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	if len(axisResp.XAxis) != 2 || axisResp.XAxis[0] != "2024-01" || axisResp.XAxis[1] != "2024-02" {
		t.Fatalf("expected XAxis=['2024-01','2024-02'], got %v", axisResp.XAxis)
	}
	seriesMap := map[string][]any{}
	for _, s := range axisResp.Series {
		seriesMap[s.Name] = s.Data
	}
	if len(seriesMap) != 2 {
		t.Fatalf("expected 2 series, got %d: %v", len(seriesMap), getSeriesNames(axisResp.Series))
	}
	if seriesMap["Beijing"][0] != 100 || seriesMap["Beijing"][1] != 150 {
		t.Errorf("expected Beijing data=[100,150], got %v", seriesMap["Beijing"])
	}
	if seriesMap["Shanghai"][0] != 200 || seriesMap["Shanghai"][1] != 250 {
		t.Errorf("expected Shanghai data=[200,250], got %v", seriesMap["Shanghai"])
	}
}

// TestAxisProcessor_SlotAware_NotPositional 证明槽位感知路径按 GroupName 而非 dims 的位置切分：
// 故意把 dims 顺序颠倒（city 在前、date 在后），但槽位名标注 city=color_group、date=x_axis，
// 输出仍应与 TestAxisProcessor_SlotAware_ColorGroupBasic 一致（X 轴是 date，series 按 city 拆分）。
func TestAxisProcessor_SlotAware_NotPositional(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"date": "2024-01", "city": "Beijing", "sales": 100},
		{"date": "2024-01", "city": "Shanghai", "sales": 200},
		{"date": "2024-02", "city": "Beijing", "sales": 150},
		{"date": "2024-02", "city": "Shanghai", "sales": 250},
	}
	metrics := []MetricConfig{{Field: "sales", Agg: AggSum}}
	// dims 顺序与槽位名"错位"：若实现仍按 dims[0] 猜 X 轴，这里会错误地把 city 当作 X 轴
	dims := []string{"city", "date"}
	ast := slotAST(dims, []string{SlotColorGroup, SlotXAxis})

	resp, err := p.Process(rows, dims, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	if len(axisResp.XAxis) != 2 || axisResp.XAxis[0] != "2024-01" || axisResp.XAxis[1] != "2024-02" {
		t.Fatalf("X 轴应按槽位名取 date（而非 dims[0]=city），expected ['2024-01','2024-02'], got %v", axisResp.XAxis)
	}
	names := getSeriesNames(axisResp.Series)
	if len(names) != 2 {
		t.Fatalf("expected 2 series, got %d: %v", len(names), names)
	}
	seriesMap := map[string][]any{}
	for _, s := range axisResp.Series {
		seriesMap[s.Name] = s.Data
	}
	if _, ok := seriesMap["Beijing"]; !ok {
		t.Fatalf("series 应按槽位名 color_group=city 拆分，expected 'Beijing' in %v", names)
	}
	if _, ok := seriesMap["Shanghai"]; !ok {
		t.Fatalf("series 应按槽位名 color_group=city 拆分，expected 'Shanghai' in %v", names)
	}
}

// TestAxisProcessor_SlotAware_MultipleMetrics 验证槽位感知 + 多指标：series 名沿用
// "指标别名 - 颜色值"的既有约定（与位置推断的多指标多维度命名一致）。
func TestAxisProcessor_SlotAware_MultipleMetrics(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"date": "2024-01", "city": "Beijing", "revenue": 100, "cost": 50},
		{"date": "2024-01", "city": "Shanghai", "revenue": 200, "cost": 80},
	}
	metrics := []MetricConfig{
		{Field: "revenue", Agg: AggSum},
		{Field: "cost", Agg: AggSum},
	}
	dims := []string{"date", "city"}
	ast := slotAST(dims, []string{SlotXAxis, SlotColorGroup})

	resp, err := p.Process(rows, dims, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	if len(axisResp.Series) != 4 {
		t.Fatalf("expected 4 series (2 metrics × 2 cities), got %d: %v", len(axisResp.Series), getSeriesNames(axisResp.Series))
	}
	names := getSeriesNames(axisResp.Series)
	expected := []string{"revenue - Beijing", "revenue - Shanghai", "cost - Beijing", "cost - Shanghai"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("expected series[%d] name=%q, got %q", i, expected[i], name)
		}
	}
}

// TestAxisProcessor_SlotAware_XAxisOnly 验证只有 x_axis 槽位、没有 color_group 时，
// 退化为"每个指标一条 series"（与单维度旧逻辑一致）。
func TestAxisProcessor_SlotAware_XAxisOnly(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"category": "A", "value": 100},
		{"category": "B", "value": 200},
	}
	metrics := []MetricConfig{{Field: "value", Agg: AggSum}}
	dims := []string{"category"}
	ast := slotAST(dims, []string{SlotXAxis})

	resp, err := p.Process(rows, dims, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	if len(axisResp.XAxis) != 2 || axisResp.XAxis[0] != "A" || axisResp.XAxis[1] != "B" {
		t.Fatalf("expected XAxis=['A','B'], got %v", axisResp.XAxis)
	}
	if len(axisResp.Series) != 1 || axisResp.Series[0].Name != "value" {
		t.Fatalf("expected 1 series named 'value', got %v", getSeriesNames(axisResp.Series))
	}
}

// TestAxisProcessor_SlotAware_XAxisOnlyMultiDimFallback 复现向后兼容性 bug：v1 平铺协议的
// 多维度 bar/line/area 请求经 ChartSpecFromRequest 的 defaultDimGroupName 会把**所有**维度
// 标成 GroupName="x_axis"（没有 color_group 维度）。此时必须回退到旧的位置推断逻辑
// （X 轴 = dims[0]，series 按 dims[1:] 拆分），而不是走槽位感知路径把所有维度拼成复合 X 轴。
func TestAxisProcessor_SlotAware_XAxisOnlyMultiDimFallback(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"date": "2024-01", "city": "Beijing", "sales": 100},
		{"date": "2024-01", "city": "Shanghai", "sales": 200},
		{"date": "2024-02", "city": "Beijing", "sales": 150},
		{"date": "2024-02", "city": "Shanghai", "sales": 250},
	}
	metrics := []MetricConfig{{Field: "sales", Agg: AggSum}}
	dims := []string{"date", "city"}
	// 模拟 v1 请求：defaultDimGroupName 对 bar/line/area 一律返回 "x_axis"
	ast := slotAST(dims, []string{SlotXAxis, SlotXAxis})

	resp, err := p.Process(rows, dims, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	// 旧位置推断逻辑：X 轴取 dims[0]=date 的值；
	// 若误走槽位感知路径，X 轴会变成复合拼接值（如 "2024-01 - Beijing"）
	if len(axisResp.XAxis) != 2 || axisResp.XAxis[0] != "2024-01" || axisResp.XAxis[1] != "2024-02" {
		t.Fatalf("expected XAxis=['2024-01','2024-02'] (positional fallback), got %v", axisResp.XAxis)
	}
	// series 按 dims[1:]=city 拆分，而不是退化成单条指标 series
	names := getSeriesNames(axisResp.Series)
	expected := []string{"Beijing", "Shanghai"}
	if len(names) != len(expected) {
		t.Fatalf("expected series %v (one per dims[1:] value), got %v", expected, names)
	}
	for i := range expected {
		if names[i] != expected[i] {
			t.Errorf("expected series[%d]=%q, got %q", i, expected[i], names[i])
		}
	}
	seriesMap := map[string][]any{}
	for _, s := range axisResp.Series {
		seriesMap[s.Name] = s.Data
	}
	if seriesMap["Beijing"][0] != 100 || seriesMap["Beijing"][1] != 150 {
		t.Errorf("expected Beijing data=[100,150], got %v", seriesMap["Beijing"])
	}
	if seriesMap["Shanghai"][0] != 200 || seriesMap["Shanghai"][1] != 250 {
		t.Errorf("expected Shanghai data=[200,250], got %v", seriesMap["Shanghai"])
	}
}

// TestAxisProcessor_SlotAware_EmptyGroupNameFallback 验证 AST 存在但 GroupName 全为空
// （模拟 v1 平铺协议经 PlanAST 产出的 AST，GroupName 不会被填充）时，回退到位置推断逻辑。
func TestAxisProcessor_SlotAware_EmptyGroupNameFallback(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"date": "2024-01", "city": "Beijing", "sales": 100},
		{"date": "2024-01", "city": "Shanghai", "sales": 200},
		{"date": "2024-02", "city": "Beijing", "sales": 150},
		{"date": "2024-02", "city": "Shanghai", "sales": 250},
	}
	metrics := []MetricConfig{{Field: "sales", Agg: AggSum}}
	dims := []string{"date", "city"}
	ast := slotAST(dims, []string{"", ""})

	resp, err := p.Process(rows, dims, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	// 应与 TestAxisProcessor_MultiDims_TwoDims（无槽位信息的位置推断）结果完全一致
	if len(axisResp.XAxis) != 2 || axisResp.XAxis[0] != "2024-01" || axisResp.XAxis[1] != "2024-02" {
		t.Fatalf("expected XAxis=['2024-01','2024-02'], got %v", axisResp.XAxis)
	}
	names := getSeriesNames(axisResp.Series)
	expected := []string{"Beijing", "Shanghai"}
	if len(names) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, names)
	}
	for i := range expected {
		if names[i] != expected[i] {
			t.Errorf("expected series[%d]=%q, got %q", i, expected[i], names[i])
		}
	}
}

// TestAxisProcessor_SlotAware_FieldMismatchFallback 验证 AST 的 DimensionExprs 与 dims
// 字段名对不上（防御性检查）时，回退到位置推断逻辑而不是用错槽位名切分。
func TestAxisProcessor_SlotAware_FieldMismatchFallback(t *testing.T) {
	p := &AxisProcessor{}
	rows := []map[string]any{
		{"date": "2024-01", "city": "Beijing", "sales": 100},
		{"date": "2024-01", "city": "Shanghai", "sales": 200},
	}
	metrics := []MetricConfig{{Field: "sales", Agg: AggSum}}
	dims := []string{"date", "city"}
	// AST 里的 Field 与 dims 不匹配（第二个维度写成了 country），应触发防御性回退
	ast := slotAST([]string{"date", "country"}, []string{SlotXAxis, SlotColorGroup})

	resp, err := p.Process(rows, dims, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}

	// 回退到位置推断：dims[0]=date 作 X 轴，dims[1]=city 作 series（与 ast 里错误的 country 无关）
	if len(axisResp.XAxis) != 1 || axisResp.XAxis[0] != "2024-01" {
		t.Fatalf("expected XAxis=['2024-01'], got %v", axisResp.XAxis)
	}
	names := getSeriesNames(axisResp.Series)
	expected := []string{"Beijing", "Shanghai"}
	if len(names) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, names)
	}
	for i := range expected {
		if names[i] != expected[i] {
			t.Errorf("expected series[%d]=%q, got %q", i, expected[i], names[i])
		}
	}
}
