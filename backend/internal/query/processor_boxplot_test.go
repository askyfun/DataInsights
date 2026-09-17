package query

import (
	"testing"
)

// boxplotStatsRow 构造一组 stats 行（列名与 builder 别名逐字一致）。
func boxplotStatsRow(wlo, q1, med, q3, whi any) map[string]any {
	return map[string]any{
		boxWhiskerLowAlias:  wlo,
		boxQ1Alias:          q1,
		boxMedianAlias:      med,
		boxQ3Alias:          q3,
		boxWhiskerHighAlias: whi,
	}
}

// TestBoxplotProcessor_Assemble_HappyPath 验证正常组装：五值 + outliers + total +
// truncated=false（total 等于展示数）。
func TestBoxplotProcessor_Assemble_HappyPath(t *testing.T) {
	stats := boxplotStatsRow(float64(1), float64(2), float64(3), float64(4), float64(9))
	outliers := []map[string]any{{boxOutlierValueAlias: float64(0.5)}, {boxOutlierValueAlias: float64(20)}}

	resp, err := (&BoxplotProcessor{}).Assemble(stats, outliers, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.WhiskerLow != 1 || resp.Q1 != 2 || resp.Median != 3 || resp.Q3 != 4 || resp.WhiskerHigh != 9 {
		t.Fatalf("five-number mismatch: %+v", resp)
	}
	if len(resp.Outliers) != 2 || resp.Outliers[0] != 0.5 || resp.Outliers[1] != 20 {
		t.Fatalf("outliers mismatch: %+v", resp.Outliers)
	}
	if resp.OutlierTotal != 2 || resp.Truncated {
		t.Fatalf("expected total=2 truncated=false, got total=%d truncated=%v", resp.OutlierTotal, resp.Truncated)
	}
}

// TestBoxplotProcessor_Assemble_Truncated 验证截断判定：展示数 < 总数时 truncated=true。
func TestBoxplotProcessor_Assemble_Truncated(t *testing.T) {
	stats := boxplotStatsRow(float64(1), float64(2), float64(3), float64(4), float64(99))
	outliers := []map[string]any{{boxOutlierValueAlias: float64(50)}}

	resp, err := (&BoxplotProcessor{}).Assemble(stats, outliers, 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.OutlierTotal != 5000 || !resp.Truncated {
		t.Fatalf("expected total=5000 truncated=true, got total=%d truncated=%v", resp.OutlierTotal, resp.Truncated)
	}
}

// TestBoxplotProcessor_Assemble_NilStats 验证 nil stats 行 → 退化结构（非 nil 空
// outliers、total 0、truncated false），不报错。
func TestBoxplotProcessor_Assemble_NilStats(t *testing.T) {
	resp, err := (&BoxplotProcessor{}).Assemble(nil, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Outliers == nil || len(resp.Outliers) != 0 {
		t.Fatalf("expected non-nil empty outliers, got %+v", resp.Outliers)
	}
	if resp.Q1 != 0 || resp.Median != 0 || resp.Q3 != 0 || resp.Truncated {
		t.Fatalf("expected degenerate zero struct, got %+v", resp)
	}
}

// TestBoxplotProcessor_Assemble_AllNullQuartiles 验证空数据集（q1/med/q3 全 NULL）
// → 退化结构，即使 MIN/MAX 有残留也不产错误形状。
func TestBoxplotProcessor_Assemble_AllNullQuartiles(t *testing.T) {
	stats := boxplotStatsRow(nil, nil, nil, nil, nil)
	resp, err := (&BoxplotProcessor{}).Assemble(stats, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Q1 != 0 || resp.Median != 0 || resp.Q3 != 0 {
		t.Fatalf("expected all-zero quartiles, got %+v", resp)
	}
	if resp.Outliers == nil || len(resp.Outliers) != 0 {
		t.Fatalf("expected non-nil empty outliers, got %+v", resp.Outliers)
	}
}

// TestBoxplotProcessor_Assemble_SkipsNonNumericOutliers 验证离群点列读取的防御：
// 非数值行跳过（正常不发生，但 Assemble 不应 panic 或塞 0）。
func TestBoxplotProcessor_Assemble_SkipsNonNumericOutliers(t *testing.T) {
	stats := boxplotStatsRow(float64(1), float64(2), float64(3), float64(4), float64(9))
	outliers := []map[string]any{
		{boxOutlierValueAlias: float64(50)},
		{boxOutlierValueAlias: "not-a-number"},
		{boxOutlierValueAlias: nil},
		{boxOutlierValueAlias: float64(60)},
	}
	resp, err := (&BoxplotProcessor{}).Assemble(stats, outliers, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Outliers) != 2 || resp.Outliers[0] != 50 || resp.Outliers[1] != 60 {
		t.Fatalf("expected non-numeric rows skipped, got %+v", resp.Outliers)
	}
}

// TestBoxplotProcessor_ProcessRejects 验证通用 Process 入口显式报错（boxplot 只走 Assemble）。
func TestBoxplotProcessor_ProcessRejects(t *testing.T) {
	_, err := (&BoxplotProcessor{}).Process(nil, nil, nil, nil)
	if err == nil {
		t.Fatalf("expected Process to reject for boxplot")
	}
}

// TestGetProcessor_BoxplotRouting 验证 GetProcessor 显式路由到 BoxplotProcessor，
// 不落入 AxisProcessor 兜底。
func TestGetProcessor_BoxplotRouting(t *testing.T) {
	if _, ok := GetProcessor(ChartTypeBoxplot).(*BoxplotProcessor); !ok {
		t.Fatalf("expected *BoxplotProcessor, got %T", GetProcessor(ChartTypeBoxplot))
	}
}
