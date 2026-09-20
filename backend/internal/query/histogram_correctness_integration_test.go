//go:build integration

package query

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"data-insights/internal/model"
)

// 本文件用真实 PostgreSQL 端到端验证直方图两阶段分箱查询的正确性（R-57，
// plan §8 验收标准）：
//   - 两阶段 SQL（MIN/MAX/COUNT → FLOOR((v-?)/?) 分箱）在真实 PG 上语法有效
//     （含参数化 float args、GROUP BY 派生表列（子查询）、rebind ?→$N 全链路）；
//   - 所有 bin 的 count 之和 == 种子总行数（无遗漏、无重复）；
//   - bin 边界连续无重叠：BinEnd[i] == BinStart[i+1]，首 bin BinStart==mn、
//     末 bin BinEnd>=mx；
//   - 等于最大值的边界行归入末 bin（浮点钳制路径在真实库上触发：
//     FLOOR((40-0)/8)==5 越界 → 钳进 bin 4）。
//
// 与 mock/字符串断言测试（executor_histogram_test.go / bun_builder_histogram_test.go /
// processor_stats_test.go）互补：那些测试断言 SQL 文本与 Go 端组装，本测试跑
// 真实 PG 生产驱动 + 真实 builder + executor histogram 分支 + HistogramProcessor。
//
// 安全约束（binding，照 pivot_correctness_integration_test.go 约定）：只读、
// 零 DDL、零写入——种子数据用 inline VALUES 派生表作为 dataset 的 query_sql，
// 全程只对 PG 跑 SELECT；连接与 skip-if-unset 复用 openPivotTestConn
// （TEST_DATABASE_URL，凭证绝不硬编码）。

// histogramCorrectnessSeedSQL 是已知真值的种子数据：9 行，v ∈ {0,5,...,40}。
// bin_count=5 → mn=0, mx=40, width=8 → 期望各箱计数 [2,2,1,2,2]（40 恰落
// FLOOR 边界 5，钳入末箱）；bin_width=10 → numBins=4 → 期望 [2,2,2,3]。
// PG 推断 v 为 integer——阶段2 的 float 参数化 args 同时验证 integer 列
// 不被整数除法截断（field - $1 提升为 float8）。
func histogramCorrectnessSeedSQL() string {
	return `SELECT * FROM (VALUES (0),(5),(10),(15),(20),(25),(30),(35),(40)) AS t(v)`
}

func histogramCorrectnessFixture() (*model.Dataset, *model.Datasource) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Histogram Correctness Dataset",
		QueryType: "sql",
		QuerySQL:  sql.NullString{String: histogramCorrectnessSeedSQL(), Valid: true},
	}
	ds := &model.Datasource{ID: 1, Name: "Histogram Correctness DS", Type: "postgresql"}
	return dataset, ds
}

func histogramCorrectnessRequest(queryOptions map[string]any) *ChartQueryRequest {
	spec := &QuerySpec{Metrics: []MetricExpr2{{Field: "v", Agg: AggCount}}}
	return &ChartQueryRequest{
		DatasetID:    1,
		ChartType:    ChartTypeHistogram,
		Metrics:      []MetricConfig{{Field: "v", Agg: AggCount}},
		QueryOptions: queryOptions,
		PlannedAST:   NewQueryPlanner().PlanAST(histogramCorrectnessSeedSQL(), SourceTypeSQL, spec),
	}
}

// assertHistogramCorrectness 断言通用不变式：sum(count)==种子总行数、bin 边界
// 连续无重叠、首 bin BinStart==wantMin、末 bin BinEnd>=wantMax，以及逐箱计数真值。
func assertHistogramCorrectness(t *testing.T, resp *HistogramResponse, wantCounts []int64, wantMin, wantMax float64) {
	t.Helper()
	if len(resp.Bins) != len(wantCounts) {
		t.Fatalf("expected %d bins, got %d: %+v", len(wantCounts), len(resp.Bins), resp.Bins)
	}
	var sum int64
	for i, b := range resp.Bins {
		if b.Count != wantCounts[i] {
			t.Errorf("bin[%d] [%v,%v): expected count %d, got %d", i, b.BinStart, b.BinEnd, wantCounts[i], b.Count)
		}
		sum += b.Count
		if i > 0 && b.BinStart != resp.Bins[i-1].BinEnd {
			t.Errorf("bins not contiguous at %d: BinEnd=%v != BinStart=%v", i, resp.Bins[i-1].BinEnd, b.BinStart)
		}
	}
	// plan §8 验收标准：所有 bin 的 count 之和 == 总行数（无遗漏、无重复）
	if sum != 9 {
		t.Errorf("expected sum(bin counts)==9 (total seed rows), got %d", sum)
	}
	if resp.Bins[0].BinStart != wantMin {
		t.Errorf("expected first BinStart==%v (mn), got %v", wantMin, resp.Bins[0].BinStart)
	}
	last := resp.Bins[len(resp.Bins)-1]
	if last.BinEnd < wantMax {
		t.Errorf("expected last BinEnd>=%v (mx), got %v", wantMax, last.BinEnd)
	}
}

// TestHistogramCorrectness_BinCount_RealPG 跑完整 executor histogram 生产路径
// （真实 PG 两阶段查询 + HistogramProcessor.ProcessBins），bin_count=5。
func TestHistogramCorrectness_BinCount_RealPG(t *testing.T) {
	conn := openPivotTestConn(t)
	dataset, ds := histogramCorrectnessFixture()
	executor := NewExecutor(conn, dataset, ds)

	res, err := executor.Execute(context.Background(), histogramCorrectnessRequest(map[string]any{"bin_count": float64(5)}))
	if err != nil {
		t.Fatalf("executor.Execute failed: %v", err)
	}
	if !strings.Contains(res.Select, "FLOOR(") || !strings.Contains(res.Select, "GROUP BY") {
		t.Errorf("expected phase-2 FLOOR/GROUP BY SQL in GeneratedSQL.Select, got: %s", res.Select)
	}
	resp, ok := res.Data.(*HistogramResponse)
	if !ok {
		t.Fatalf("expected *HistogramResponse, got %T", res.Data)
	}
	// width=(40-0)/5=8：[0,8)={0,5} [8,16)={10,15} [16,24)={20} [24,32)={25,30} [32,40]={35,40}
	assertHistogramCorrectness(t, resp, []int64{2, 2, 1, 2, 2}, 0, 40)
}

// TestHistogramCorrectness_UserBinWidth_RealPG 验证用户 bin_width 覆盖路径
// 在真实 PG 上同样成立：width=10 → numBins=ceil(40/10)=4，40 恰落 FLOOR 整数
// 倍边界（索引 4）被钳入末箱。
func TestHistogramCorrectness_UserBinWidth_RealPG(t *testing.T) {
	conn := openPivotTestConn(t)
	dataset, ds := histogramCorrectnessFixture()
	executor := NewExecutor(conn, dataset, ds)

	res, err := executor.Execute(context.Background(), histogramCorrectnessRequest(map[string]any{"bin_width": float64(10)}))
	if err != nil {
		t.Fatalf("executor.Execute failed: %v", err)
	}
	resp, ok := res.Data.(*HistogramResponse)
	if !ok {
		t.Fatalf("expected *HistogramResponse, got %T", res.Data)
	}
	// [0,10)={0,5} [10,20)={10,15} [20,30)={20,25} [30,40]={30,35,40}
	assertHistogramCorrectness(t, resp, []int64{2, 2, 2, 3}, 0, 40)
}
