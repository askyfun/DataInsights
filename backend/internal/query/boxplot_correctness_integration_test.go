//go:build integration

package query

import (
	"context"
	"database/sql"
	"math"
	"testing"

	"dataray/internal/model"
)

// 本文件用真实 PostgreSQL 端到端验证箱线图三查询编排的正确性（R-52，plan §8 验收）：
//   - stats SQL（MIN + percentile_cont(0.25/0.5/0.75) + MAX）在真实 PG 语法有效、
//     参数化过滤全链路（rebind ?→$N）；
//   - Go 端 IQR fence（q1-1.5*iqr / q3+1.5*iqr）喂进 outliers list / count 两查询，
//     离群点集合与总数符合手算真值；
//   - percentile_cont 的插值语义在真实库上取值正确。
//
// 与 mock 测试互补：executor_boxplot_test.go 断言编排/参数化/报错路径，本测试跑真实
// PG 驱动 + 真实 builder + executor boxplot 分支 + BoxplotProcessor.Assemble。
//
// 安全约束：只读、零 DDL、零写入——种子用 inline VALUES 派生表作为 dataset 的 query_sql，
// 全程只 SELECT；连接与 skip-if-unset 复用 openPivotTestConn（TEST_DATABASE_URL）。

const boxplotTol = 1e-9

// boxplotCorrectnessSeedSQL 是已知真值的 12 行种子：{-50,1,2,3,4,5,6,7,8,9,10,100}。
// 排序后 percentile_cont 线性插值（pos=p*(n-1)=p*11）：
//   - Q1 (p=0.25): pos=2.75 → idx2=2,idx3=3 → 2.75
//   - 中位 (p=0.5): pos=5.5 → idx5=5,idx6=6 → 5.5
//   - Q3 (p=0.75): pos=8.25 → idx8=8,idx9=9 → 8.25
//   - MIN=-50, MAX=100
// IQR=5.5 → lower=2.75-8.25=-5.5, upper=8.25+8.25=16.5。
// 离群点：v<-5.5 或 v>16.5 → {-50, 100}（升序 [-50,100]），total=2，truncated=false。
func boxplotCorrectnessSeedSQL() string {
	return `SELECT * FROM (VALUES (-50),(1),(2),(3),(4),(5),(6),(7),(8),(9),(10),(100)) AS t(v)`
}

func boxplotCorrectnessFixture() (*model.Dataset, *model.Datasource) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Boxplot Correctness Dataset",
		QueryType: "sql",
		QuerySQL:  sql.NullString{String: boxplotCorrectnessSeedSQL(), Valid: true},
	}
	ds := &model.Datasource{ID: 1, Name: "Boxplot Correctness DS", Type: "postgresql"}
	return dataset, ds
}

func boxplotCorrectnessRequest(filters []FilterConfig) *ChartQueryRequest {
	spec := &QuerySpec{Metrics: []MetricExpr2{{Field: "v", Agg: AggCount}}, Filters: filters}
	return &ChartQueryRequest{
		DatasetID:  1,
		ChartType:  ChartTypeBoxplot,
		Metrics:    []MetricConfig{{Field: "v", Agg: AggCount}},
		PlannedAST: NewQueryPlanner().PlanAST(boxplotCorrectnessSeedSQL(), SourceTypeSQL, spec),
	}
}

func approxEq(a, b float64) bool { return math.Abs(a-b) < boxplotTol }

// TestBoxplotCorrectness_RealPG 跑完整 executor boxplot 生产路径（真实 PG 三查询 +
// Go 端 fence + BoxplotProcessor.Assemble），逐值断言手算真值。
func TestBoxplotCorrectness_RealPG(t *testing.T) {
	conn := openPivotTestConn(t)
	dataset, ds := boxplotCorrectnessFixture()
	executor := NewExecutor(conn, dataset, ds)

	res, err := executor.Execute(context.Background(), boxplotCorrectnessRequest(nil))
	if err != nil {
		t.Fatalf("executor.Execute failed: %v", err)
	}
	resp, ok := res.Data.(*BoxplotResponse)
	if !ok {
		t.Fatalf("expected *BoxplotResponse, got %T", res.Data)
	}
	if !approxEq(resp.WhiskerLow, -50) || !approxEq(resp.Q1, 2.75) || !approxEq(resp.Median, 5.5) ||
		!approxEq(resp.Q3, 8.25) || !approxEq(resp.WhiskerHigh, 100) {
		t.Fatalf("five-number mismatch: %+v", resp)
	}
	if resp.OutlierTotal != 2 || resp.Truncated {
		t.Fatalf("expected total=2 truncated=false, got total=%d truncated=%v", resp.OutlierTotal, resp.Truncated)
	}
	if len(resp.Outliers) != 2 || !approxEq(resp.Outliers[0], -50) || !approxEq(resp.Outliers[1], 100) {
		t.Fatalf("expected outliers [-50,100] ascending, got %+v", resp.Outliers)
	}
}

// TestBoxplotCorrectness_EmptyRealPG 验证空数据集（无匹配行）在真实 PG 上返回退化
// 结构而非报错：过滤条件排除全部行 → stats 单行全 NULL。
func TestBoxplotCorrectness_EmptyRealPG(t *testing.T) {
	conn := openPivotTestConn(t)
	dataset, ds := boxplotCorrectnessFixture()
	executor := NewExecutor(conn, dataset, ds)

	req := boxplotCorrectnessRequest([]FilterConfig{{Field: "v", Op: FilterGt, Value: 1e9}})

	res, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("executor.Execute failed: %v", err)
	}
	resp, ok := res.Data.(*BoxplotResponse)
	if !ok {
		t.Fatalf("expected *BoxplotResponse, got %T", res.Data)
	}
	if resp.Outliers == nil || len(resp.Outliers) != 0 || resp.Q1 != 0 || resp.Truncated {
		t.Fatalf("expected degenerate empty response on empty selection, got %+v", resp)
	}
}
