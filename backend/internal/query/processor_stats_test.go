package query

import (
	"math"
	"testing"
)

// TestHistogramProcessor_ProcessBins_AssemblyAndEmptyFill 验证 bins 组装：
// BinStart/BinEnd/Count 正确、缺失索引的空 bin 被补全为 Count=0、
// sum(Count)==喂入的总计数（plan §8 验收标准的 processor 侧口径）。
// 行值形态混用 float64/int64/数值字符串（toFloat64 全覆盖：PG FLOOR(float8)
// 返 float8、COUNT(*) 返 bigint，MySQL/驱动差异下也可能是字符串）。
func TestHistogramProcessor_ProcessBins_AssemblyAndEmptyFill(t *testing.T) {
	rows := []map[string]any{
		{"bin": float64(0), "cnt": int64(3)},
		{"bin": "2", "cnt": "5"},
	}
	resp, err := (&HistogramProcessor{}).ProcessBins(rows, 10, 2, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []HistogramBin{
		{BinStart: 10, BinEnd: 12, Count: 3},
		{BinStart: 12, BinEnd: 14, Count: 0}, // 空 bin 补全
		{BinStart: 14, BinEnd: 16, Count: 5},
		{BinStart: 16, BinEnd: 18, Count: 0}, // 空 bin 补全
	}
	if len(resp.Bins) != len(want) {
		t.Fatalf("expected %d bins, got %d: %+v", len(want), len(resp.Bins), resp.Bins)
	}
	var sum int64
	for i, b := range resp.Bins {
		if b != want[i] {
			t.Errorf("bin[%d]: expected %+v, got %+v", i, want[i], b)
		}
		sum += b.Count
	}
	if sum != 8 {
		t.Errorf("expected sum(counts)==8 (no loss, no duplication), got %d", sum)
	}
}

// TestHistogramProcessor_ProcessBins_ClampsFloatBoundary 验证浮点边界钳制：
// FLOOR 误差产生的越界索引（==numBins 的 max 边界行、-1 的负零误差）分别归入
// 末/首 bin，不丢行——sum(Count)==总数 的不变式保持。
func TestHistogramProcessor_ProcessBins_ClampsFloatBoundary(t *testing.T) {
	rows := []map[string]any{
		{"bin": float64(4), "cnt": int64(1)},  // value==max 恰落整数倍边界
		{"bin": float64(-1), "cnt": int64(2)}, // 负零浮点误差
		{"bin": float64(1), "cnt": int64(3)},
	}
	resp, err := (&HistogramProcessor{}).ProcessBins(rows, 0, 10, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Bins) != 4 {
		t.Fatalf("expected 4 bins, got %d", len(resp.Bins))
	}
	if resp.Bins[3].Count != 1 {
		t.Errorf("expected max-boundary row clamped into last bin, got counts %+v", resp.Bins)
	}
	if resp.Bins[0].Count != 2 {
		t.Errorf("expected negative-error row clamped into first bin, got counts %+v", resp.Bins)
	}
	if resp.Bins[1].Count != 3 {
		t.Errorf("expected in-range row in bin 1, got %+v", resp.Bins)
	}
	var sum int64
	for _, b := range resp.Bins {
		sum += b.Count
	}
	if sum != 6 {
		t.Errorf("expected sum==6 (clamping must not drop rows), got %d", sum)
	}
}

// TestHistogramProcessor_ProcessBins_SkipsNullBin 验证 NULL bin（值列 NULL 行经
// FLOOR 归入 NULL 组）被跳过，不计入数值分箱。
func TestHistogramProcessor_ProcessBins_SkipsNullBin(t *testing.T) {
	rows := []map[string]any{
		{"bin": nil, "cnt": int64(7)},
		{"bin": float64(0), "cnt": int64(2)},
	}
	resp, err := (&HistogramProcessor{}).ProcessBins(rows, 0, 5, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Bins[0].Count != 2 || resp.Bins[1].Count != 0 {
		t.Fatalf("expected NULL bin skipped, got %+v", resp.Bins)
	}
}

// TestHistogramProcessor_ProcessBins_Errors 验证显式报错路径（不静默）：
// count 不可解析、numBins<1、binWidth 非正（含 0/负数/NaN）。
func TestHistogramProcessor_ProcessBins_Errors(t *testing.T) {
	p := &HistogramProcessor{}
	if _, err := p.ProcessBins([]map[string]any{{"bin": float64(0), "cnt": "abc"}}, 0, 1, 2); err == nil {
		t.Error("expected error for non-numeric count")
	}
	if _, err := p.ProcessBins(nil, 0, 1, 0); err == nil {
		t.Error("expected error for numBins=0")
	}
	for _, width := range []float64{0, -1, math.NaN()} {
		if _, err := p.ProcessBins(nil, 0, width, 2); err == nil {
			t.Errorf("expected error for binWidth=%v", width)
		}
	}
}

// TestHistogramProcessor_Process_Rejects 验证通用 Process 入口显式报错
// （histogram 走两阶段专门路径，误调用不得静默产出错误形状），且
// GetProcessor 的 histogram case 返回 HistogramProcessor（不落 AxisProcessor 兜底）。
func TestHistogramProcessor_Process_Rejects(t *testing.T) {
	if _, err := (&HistogramProcessor{}).Process(nil, nil, nil, nil); err == nil {
		t.Error("expected error from generic Process")
	}
	if _, ok := GetProcessor(ChartTypeHistogram).(*HistogramProcessor); !ok {
		t.Errorf("GetProcessor(histogram) = %T, want *HistogramProcessor", GetProcessor(ChartTypeHistogram))
	}
}

// TestHistogramBinOptions 验证 query_options 解析：缺省 bin_count=20、
// JSON 数字（float64）与 int/int64/数值字符串形态、非法值回落默认、
// 荒谬 bin_count 钳到 maxHistogramBins（防无界分配）、
// bin_width 仅 >0 生效（用户覆盖）。
func TestHistogramBinOptions(t *testing.T) {
	for _, tc := range []struct {
		name         string
		opts         map[string]any
		wantBinCount int
		wantBinWidth float64
	}{
		{"nil options", nil, 20, 0},
		{"empty options", map[string]any{}, 20, 0},
		{"json float bin_count", map[string]any{"bin_count": float64(10)}, 10, 0},
		{"int bin_count", map[string]any{"bin_count": 7}, 7, 0},
		{"int64 bin_count", map[string]any{"bin_count": int64(5)}, 5, 0},
		{"numeric string bin_count", map[string]any{"bin_count": "8"}, 8, 0},
		{"zero bin_count falls back", map[string]any{"bin_count": float64(0)}, 20, 0},
		{"negative bin_count falls back", map[string]any{"bin_count": float64(-3)}, 20, 0},
		{"garbage bin_count falls back", map[string]any{"bin_count": "abc"}, 20, 0},
		{"bool bin_count falls back", map[string]any{"bin_count": true}, 20, 0},
		{"absurd bin_count clamped", map[string]any{"bin_count": float64(100000000)}, maxHistogramBins, 0},
		{"int-overflowing bin_count clamped", map[string]any{"bin_count": 1e300}, maxHistogramBins, 0},
		{"user bin_width", map[string]any{"bin_width": 2.5}, 20, 2.5},
		{"bin_width overrides with bin_count", map[string]any{"bin_count": float64(4), "bin_width": 2.5}, 4, 2.5},
		{"zero bin_width ignored", map[string]any{"bin_width": float64(0)}, 20, 0},
		{"negative bin_width ignored", map[string]any{"bin_width": float64(-1)}, 20, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binCount, binWidth := histogramBinOptions(tc.opts)
			if binCount != tc.wantBinCount || binWidth != tc.wantBinWidth {
				t.Fatalf("expected (%d, %v), got (%d, %v)", tc.wantBinCount, tc.wantBinWidth, binCount, binWidth)
			}
		})
	}
}

// TestHistogramProcessor_ProcessBinsGrouped 验证分组直方图（2026-09-26）装配：
// 每个维度值组合一份 bins；顶层 Bins 为跨系列总计数；同 bin 多系列计数不丢不重；
// NULL 维度值渲染为空串参与分组；dimCount=0 时与旧 ProcessBins 完全一致（无 groups）。
func TestHistogramProcessor_ProcessBinsGrouped(t *testing.T) {
	rows := []map[string]any{
		{"d0": "East", "bin": float64(0), "cnt": int64(3)},
		{"d0": "West", "bin": float64(0), "cnt": int64(2)},
		{"d0": "East", "bin": float64(1), "cnt": int64(4)},
		{"d0": nil, "bin": float64(1), "cnt": int64(1)},
	}
	resp, err := (&HistogramProcessor{}).ProcessBinsGrouped(rows, 0, 10, 2, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 顶层：跨系列总计数
	wantTotals := []HistogramBin{{BinStart: 0, BinEnd: 10, Count: 5}, {BinStart: 10, BinEnd: 20, Count: 5}}
	for i, b := range resp.Bins {
		if b != wantTotals[i] {
			t.Errorf("totals[%d]: expected %+v, got %+v", i, wantTotals[i], b)
		}
	}

	// 分组：East {3,4}、West {2,0}、NULL→"" {0,1}；顺序按首次出现。
	if len(resp.Groups) != 3 {
		t.Fatalf("expected 3 groups, got %d: %+v", len(resp.Groups), resp.Groups)
	}
	wantGroupBins := map[string][2]int64{"East": {3, 4}, "West": {2, 0}, "": {0, 1}}
	wantOrder := []string{"East", "West", ""}
	for i, g := range resp.Groups {
		if g.Name != wantOrder[i] {
			t.Errorf("group[%d] name: expected %q, got %q", i, wantOrder[i], g.Name)
		}
		want := wantGroupBins[g.Name]
		if len(g.Bins) != 2 || g.Bins[0].Count != want[0] || g.Bins[1].Count != want[1] {
			t.Errorf("group %q bins: expected %v, got %+v", g.Name, want, g.Bins)
		}
	}

	// 不变式：sum(group bins) == totals
	var groupSum int64
	for _, g := range resp.Groups {
		for _, b := range g.Bins {
			groupSum += b.Count
		}
	}
	var totalSum int64
	for _, b := range resp.Bins {
		totalSum += b.Count
	}
	if groupSum != totalSum {
		t.Errorf("invariant broken: group sum %d != total sum %d", groupSum, totalSum)
	}
}

// TestHistogramProcessor_ProcessBinsGrouped_ZeroDims_NoGroups dimCount=0 时
// 行为必须与旧 ProcessBins 完全一致（不产出 groups，顶层即总数）。
func TestHistogramProcessor_ProcessBinsGrouped_ZeroDims_NoGroups(t *testing.T) {
	rows := []map[string]any{{"bin": float64(0), "cnt": int64(7)}}
	resp, err := (&HistogramProcessor{}).ProcessBinsGrouped(rows, 0, 5, 1, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Bins[0].Count != 7 {
		t.Errorf("expected count 7, got %d", resp.Bins[0].Count)
	}
	if len(resp.Groups) != 0 {
		t.Errorf("expected no groups, got %+v", resp.Groups)
	}
}
