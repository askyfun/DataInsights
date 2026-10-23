package query

import "testing"

// TestGetProcessor_WaterfallRouting 验证瀑布图显式路由到 AxisProcessor，
// 而不是依赖 default 分支的兜底——显式 case 是对「waterfall 与 bar 同构」这一
// 取数决策的钉死，避免将来 default 行为调整时静默改变瀑布图语义。
func TestGetProcessor_WaterfallRouting(t *testing.T) {
	if _, ok := GetProcessor(ChartTypeWaterfall).(*AxisProcessor); !ok {
		t.Fatalf("expected *AxisProcessor, got %T", GetProcessor(ChartTypeWaterfall))
	}
}

// TestWaterfall_ProcessShape 验证瀑布图走的是 bar 同款取数形状：
// 维度值进 XAxis，指标值按别名进单条 series；累计/落差不在后端计算。
func TestWaterfall_ProcessShape(t *testing.T) {
	p := GetProcessor(ChartTypeWaterfall)
	rows := []map[string]any{
		{"month": "1月", "amount": 120.0},
		{"month": "2月", "amount": -30.0},
		{"month": "3月", "amount": 45.0},
	}
	resp, err := p.Process(rows, []string{"month"}, []MetricConfig{{Field: "amount", Agg: AggSum}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	axisResp, ok := resp.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", resp)
	}
	if len(axisResp.XAxis) != 3 || axisResp.XAxis[0] != "1月" || axisResp.XAxis[2] != "3月" {
		t.Fatalf("unexpected x_axis: %v", axisResp.XAxis)
	}
	if len(axisResp.Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(axisResp.Series))
	}
	if axisResp.Series[0].Name != "amount" {
		t.Errorf("series name = %q, want amount", axisResp.Series[0].Name)
	}
	if len(axisResp.Series[0].Data) != 3 {
		t.Fatalf("expected 3 data points, got %d", len(axisResp.Series[0].Data))
	}
	if axisResp.Series[0].Data[1] != -30.0 {
		t.Errorf("data[1] = %v, want -30（后端保留原始增减值，不做累计）", axisResp.Series[0].Data[1])
	}
}
