//go:build integration

package query

import (
	"context"
	"database/sql"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"dataray/internal/datasource"
	"dataray/internal/model"
)

// 本文件用真实 PostgreSQL 端到端验证透视表小计/合计的 avg / count_distinct 真值正确性
// （plan §8 验收标准，R-53）：
//   - avg 指标的小计/合计 = 该分组所有行的 AVG（不是子分组 AVG 的平均）；
//   - count_distinct 指标的小计/合计 = 该分组所有行的 COUNT(DISTINCT)（不是子分组 COUNT DISTINCT 之和）。
//
// 与既有 mock/字符串断言测试（processor_pivot_test.go / bun_builder_pivot_test.go /
// executor_pivot_test.go）互补：那些测试直接喂 float64/int64 行、只断言 SQL 文本，绕过了
// 真实 PG 的 numeric/bigint 扫描；本测试跑真实 PG 生产驱动 + 真实 builder + 真实
// PivotProcessorV2，断言的是数据库重算后的真值。
//
// 安全约束（binding）：只读、零 DDL、零写入——种子数据用 inline VALUES 派生表作为 dataset
// 的 query_sql，全程只对 PG 跑 SELECT，无需清理、无副作用。凭证只从 TEST_DATABASE_URL 读取。

// openPivotTestConn 用生产 PostgreSQL 驱动连接 TEST_DATABASE_URL 指向的真实库。
// 未设置 TEST_DATABASE_URL 时跳过；
// 本测试不跑 migrations——它不需要任何 app schema，inline VALUES 自带数据。
func openPivotTestConn(t *testing.T) datasource.Connection {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 5432 // PG 默认端口
	}
	var pass string
	if u.User != nil {
		pass, _ = u.User.Password()
	}
	cfg := datasource.ConnectionConfig{
		Host:         u.Hostname(),
		Port:         port,
		DatabaseName: strings.TrimPrefix(u.Path, "/"),
		Username:     userOrEmpty(u),
		Password:     pass,
	}
	drv, err := datasource.NewDriver(datasource.DriverPostgreSQL)
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	conn, err := drv.Connect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("connect (is the dev DB reachable?): %v", err)
	}
	if err := conn.Ping(context.Background()); err != nil {
		t.Fatalf("ping (is the dev DB reachable/auth ok?): %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func userOrEmpty(u *url.URL) string {
	if u.User == nil {
		return ""
	}
	return u.User.Username()
}

// pivotCorrectnessSeedSQL 是判别性种子数据：真值 ≠ Go 端二次聚合值。
// 用 inline VALUES 派生表，全程只读（无 CREATE/DROP/INSERT）。
//
//	region product price user_id
//	E      A       10    u1
//	E      A       20    u2
//	E      A       30    u3
//	E      B       110   u3
//	W      A       40    u3
//	W      B       60    u3
//
// PG 推断 region/product/user_id 为 text、price 为 integer；AVG(integer)→numeric、
// COUNT(DISTINCT text)→bigint、GROUPING()→bigint，均被 toFloat64/toString 覆盖。
func pivotCorrectnessSeedSQL() string {
	return `SELECT * FROM (VALUES
  ('E','A',10,'u1'), ('E','A',20,'u2'), ('E','A',30,'u3'),
  ('E','B',110,'u3'),
  ('W','A',40,'u3'),
  ('W','B',60,'u3')
) AS t(region, product, price, user_id)`
}

// pivotCorrectnessFixture 返回 QueryType="sql" + inline VALUES query_sql 的 dataset，
// 以及 type="postgresql" 的 datasource（executor 据此走 PG 方言 + caps=true 路径）。
func pivotCorrectnessFixture() (*model.Dataset, *model.Datasource) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Pivot Correctness Dataset",
		QueryType: "sql",
		QuerySQL:  sql.NullString{String: pivotCorrectnessSeedSQL(), Valid: true},
	}
	ds := &model.Datasource{ID: 1, Name: "Pivot Correctness DS", Type: "postgresql"}
	return dataset, ds
}

// pivotCorrectnessRequest 构造 v2 管道请求：avg(price)=avg_price + count_distinct(user_id)=uniq_users，
// rows=region / columns=product 槽位。PlannedAST 的 Source 直接填 inline VALUES SQL、
// SourceType=SourceTypeSQL，使 Test A（经 executor，回填为 no-op）与 Test B（直接用 AST）共享同款构造。
func pivotCorrectnessRequest() *ChartQueryRequest {
	spec := &QuerySpec{
		Dimensions: []DimensionExpr{
			{Field: "region", GroupName: SlotRows},
			{Field: "product", GroupName: SlotColumns},
		},
		Metrics: []MetricExpr2{
			{Field: "price", Agg: AggAvg, Alias: "avg_price", GroupName: "values"},
			{Field: "user_id", Agg: AggCountDistinct, Alias: "uniq_users", GroupName: "values"},
		},
	}
	return &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypePivot,
		Dims:      []string{"region", "product"},
		Metrics: []MetricConfig{
			{Field: "price", Agg: AggAvg, Alias: "avg_price"},
			{Field: "user_id", Agg: AggCountDistinct, Alias: "uniq_users"},
		},
		PlannedAST: NewQueryPlanner().PlanAST(pivotCorrectnessSeedSQL(), SourceTypeSQL, spec),
	}
}

// assertPivotCorrectness 断言 DB 重算的真值（明细/行小计/合计），并显式断言真值 ≠ 错误的
// Go 端二次聚合值——证明小计/合计是数据库重算的，而非 Go 对子分组结果二次聚合。
// Test A（GROUPING SETS）与 Test B（UNION ALL）共享同一份真值表（两路径产出行形状一致）。
func assertPivotCorrectness(t *testing.T, pv *PivotResponseV2) {
	t.Helper()

	// 明细行：E/A=20|3, E/B=110|1, W/A=40|1, W/B=60|1
	eDetail := findCell(t, pv, "E", false)
	assertPivotValue(t, "detail E/A avg_price", cellValue(t, eDetail, PivotValueKey("A", "avg_price")), 20)
	assertPivotValue(t, "detail E/A uniq_users", cellValue(t, eDetail, PivotValueKey("A", "uniq_users")), 3)
	assertPivotValue(t, "detail E/B avg_price", cellValue(t, eDetail, PivotValueKey("B", "avg_price")), 110)
	assertPivotValue(t, "detail E/B uniq_users", cellValue(t, eDetail, PivotValueKey("B", "uniq_users")), 1)

	wDetail := findCell(t, pv, "W", false)
	assertPivotValue(t, "detail W/A avg_price", cellValue(t, wDetail, PivotValueKey("A", "avg_price")), 40)
	assertPivotValue(t, "detail W/A uniq_users", cellValue(t, wDetail, PivotValueKey("A", "uniq_users")), 1)
	assertPivotValue(t, "detail W/B avg_price", cellValue(t, wDetail, PivotValueKey("B", "avg_price")), 60)
	assertPivotValue(t, "detail W/B uniq_users", cellValue(t, wDetail, PivotValueKey("B", "uniq_users")), 1)

	// 行小计 E：avg=(10+20+30+110)/4=42.5（不是 avg-of-avgs=(20+110)/2=65）；
	//          uniq={u1,u2,u3}=3（不是 sum-of-cds=3+1=4）。
	eSub := findCell(t, pv, "E", true)
	eSubAvg := cellValue(t, eSub, PivotValueKey(PivotSubtotalColKey, "avg_price"))
	eSubUniq := cellValue(t, eSub, PivotValueKey(PivotSubtotalColKey, "uniq_users"))
	assertPivotValue(t, "subtotal E avg_price", eSubAvg, 42.5)
	assertPivotValue(t, "subtotal E uniq_users", eSubUniq, 3)
	assertNotGoReaggregated(t, "subtotal E avg_price", eSubAvg, 65, "avg 是子分组平均的平均，说明 Go 端二次聚合了")
	assertNotGoReaggregated(t, "subtotal E uniq_users", eSubUniq, 4, "count_distinct 是子分组 COUNT DISTINCT 之和，说明 Go 端二次聚合了")

	// 行小计 W：avg=(40+60)/2=50；uniq={u3}=1（不是 sum-of-cds=1+1=2）。
	wSub := findCell(t, pv, "W", true)
	wSubAvg := cellValue(t, wSub, PivotValueKey(PivotSubtotalColKey, "avg_price"))
	wSubUniq := cellValue(t, wSub, PivotValueKey(PivotSubtotalColKey, "uniq_users"))
	assertPivotValue(t, "subtotal W avg_price", wSubAvg, 50)
	assertPivotValue(t, "subtotal W uniq_users", wSubUniq, 1)
	assertNotGoReaggregated(t, "subtotal W uniq_users", wSubUniq, 2, "count_distinct 是子分组 COUNT DISTINCT 之和，说明 Go 端二次聚合了")

	// 合计：avg=(10+20+30+110+40+60)/6=45（不是 avg-of-avgs=(20+110+40+60)/4=57.5）；
	//       uniq={u1,u2,u3}=3（不是 sum-of-cds=3+1+1+1=6，也不是行小计之和=3+1=4）。
	if pv.GrandTotal == nil {
		t.Fatalf("expected non-nil GrandTotal, got nil; cells=%+v", pv.Cells)
	}
	gAvg := cellValue(t, *pv.GrandTotal, PivotValueKey(PivotSubtotalColKey, "avg_price"))
	gUniq := cellValue(t, *pv.GrandTotal, PivotValueKey(PivotSubtotalColKey, "uniq_users"))
	assertPivotValue(t, "grand avg_price", gAvg, 45)
	assertPivotValue(t, "grand uniq_users", gUniq, 3)
	assertNotGoReaggregated(t, "grand avg_price", gAvg, 57.5, "avg 是子分组平均的平均，说明 Go 端二次聚合了")
	assertNotGoReaggregated(t, "grand uniq_users", gUniq, 6, "count_distinct 是子分组 COUNT DISTINCT 之和，说明 Go 端二次聚合了")
	assertNotGoReaggregated(t, "grand uniq_users", gUniq, 4, "grand uniq_users 等于行小计之和(3+1=4)，说明 Go 端用行小计二次聚合了合计")
}

// findCell 按 RowKey（单行维度值）+ IsSubtotal 定位一个 PivotRow；找不到即 Fatal。
func findCell(t *testing.T, pv *PivotResponseV2, rowKey string, isSubtotal bool) PivotRow {
	t.Helper()
	for _, c := range pv.Cells {
		if c.IsSubtotal == isSubtotal && len(c.RowKey) == 1 && c.RowKey[0] == rowKey {
			return c
		}
	}
	t.Fatalf("no cell with RowKey=[%s] isSubtotal=%v; cells=%+v", rowKey, isSubtotal, pv.Cells)
	return PivotRow{}
}

// cellValue 从 Values map 取值；键缺失即 Fatal（区分"值为 0"与"键根本不存在"）。
func cellValue(t *testing.T, row PivotRow, key string) float64 {
	t.Helper()
	v, ok := row.Values[key]
	if !ok {
		t.Fatalf("missing value key %q in row (RowKey=%v isSubtotal=%v); values=%+v", key, row.RowKey, row.IsSubtotal, row.Values)
	}
	return v
}

func assertPivotValue(t *testing.T, ctx string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s: got %v, want %v", ctx, got, want)
	}
}

// assertNotGoReaggregated 断言真值 got 不等于错误的 Go 端二次聚合值 wrong。
// 若相等，说明数据库没有重算、退化成了 Go 端对子分组结果的二次聚合——给出诊断信息。
func assertNotGoReaggregated(t *testing.T, ctx string, got, wrong float64, why string) {
	t.Helper()
	if math.Abs(got-wrong) < 1e-9 {
		t.Errorf("%s: got %v which equals the wrong Go-reaggregate value %v (%s)", ctx, got, wrong, why)
	}
}

// TestPivotCorrectness_GroupingSets_RealPG（测试 A）：跑完整 executor pivot 生产路径
// （真实 PG caps=true → BuildPivotGroupingSetsQuery → 真实 PG 执行 → 真实 PivotProcessorV2），
// 断言 avg/count_distinct 的明细/小计/合计真值，并确认走了 GROUPING SETS 路径。
func TestPivotCorrectness_GroupingSets_RealPG(t *testing.T) {
	conn := openPivotTestConn(t)
	dataset, ds := pivotCorrectnessFixture()
	executor := NewExecutor(conn, dataset, ds)

	res, err := executor.Execute(context.Background(), pivotCorrectnessRequest())
	if err != nil {
		t.Fatalf("executor.Execute failed: %v", err)
	}
	if !strings.Contains(res.Select, "GROUP BY GROUPING SETS") {
		t.Errorf("expected GROUPING SETS SQL (PG caps.SupportsGroupingSets=true), got: %s", res.Select)
	}

	pv, ok := res.Data.(*PivotResponseV2)
	if !ok {
		t.Fatalf("expected *PivotResponseV2, got %T", res.Data)
	}
	assertPivotCorrectness(t, pv)
}

// TestPivotCorrectness_UnionAll_RealPG（测试 B）：直接跑 UNION ALL builder + 真实驱动 +
// PivotProcessorV2，在同一份种子上断言与测试 A 完全相同的真值——证明 Task 2-2 的 UNION SQL
// 在真实库上语法有效且值正确。（PG 不是 UNION ALL 路径的生产目标——那是 MySQL/StarRocks——
// 但 PG 能执行 UNION ALL；真实 MySQL/StarRocks 验证归 Task 3-0。）
func TestPivotCorrectness_UnionAll_RealPG(t *testing.T) {
	conn := openPivotTestConn(t)
	req := pivotCorrectnessRequest()
	ast := req.PlannedAST

	rowDims, colDims, ok := resolvePivotSlots(req.Dims, ast)
	if !ok {
		t.Fatalf("resolvePivotSlots failed; dims=%v ast.DimensionExprs=%+v", req.Dims, ast.DimensionExprs)
	}

	sqlStr, args := BuildPivotUnionAllQuery(ParseDialect("postgresql"), ast, rowDims, colDims)
	if !strings.Contains(sqlStr, "UNION ALL") {
		t.Errorf("expected UNION ALL SQL, got: %s", sqlStr)
	}
	if strings.Contains(sqlStr, "GROUPING SETS") {
		t.Errorf("UNION ALL fallback must not contain GROUPING SETS, got: %s", sqlStr)
	}

	result, err := conn.Execute(context.Background(), sqlStr, args...)
	if err != nil {
		t.Fatalf("conn.Execute failed (sql=%s): %v", sqlStr, err)
	}

	resp, err := (&PivotProcessorV2{}).Process(result.Rows, req.Dims, req.Metrics, ast)
	if err != nil {
		t.Fatalf("PivotProcessorV2.Process failed: %v", err)
	}
	pv, ok := resp.(*PivotResponseV2)
	if !ok {
		t.Fatalf("expected *PivotResponseV2, got %T", resp)
	}
	assertPivotCorrectness(t, pv)
}
