package query

// fuzz 回归探针（chaos 测试转正，2026-09-20）：处理器吃畸形结果行不得 panic。

import (
	"math"
	"testing"
)

var zzChartTypes = []ChartType{
	ChartTypeTable, ChartTypeBar, ChartTypeLine, ChartTypePie, ChartTypeArea,
	ChartTypeScatter, ChartTypePivot, ChartTypeCombo, ChartTypeHistogram,
	ChartTypeBoxplot, ChartTypeFunnel, ChartTypeRadar, ChartTypeKpi, ChartType("bogus"),
}

// FuzzZZProcessorNoPanic 让每个 processor 吃「值类型千奇百怪 + 键缺失 + 键多余」的结果行。
// 目标只有一条：不 panic（panic 会直接把 HTTP 请求打成 500）。
func FuzzZZProcessorNoPanic(f *testing.F) {
	f.Add("a")
	f.Add("1")
	f.Add("")
	f.Add("__pivot_row_grp_0")
	f.Fuzz(func(t *testing.T, in string) {
		vals := []any{
			in, nil, 1.0, -0.0, math.NaN(), math.Inf(1), math.Inf(-1),
			float64(math.MaxFloat64), []any{in}, map[string]any{"k": in},
			true, int64(-1), int32(2), []byte(in), "", "0", "-1e309",
		}
		rows := make([]map[string]any, 0, len(vals)+4)
		for _, v := range vals {
			rows = append(rows, map[string]any{
				"d0": v, "d1": v, "m0": v, "m1": v, "": v, in: v,
			})
		}
		rows = append(rows,
			map[string]any{},
			map[string]any{"d0": nil},
			map[string]any{"__pivot_row_grp_0": 1, "__pivot_col_grp_0": 1},
			map[string]any{"val": in, "ocnt": in, "mn": in, "mx": in, "cnt": in, "bin": in},
		)

		dims := []string{"d0", "d1", ""}
		metrics := []MetricConfig{{Field: "m0", Agg: AggSum}, {Field: "m1", Agg: AggCount}}

		ast := &QueryAST{
			Source:     in,
			SourceType: SourceTypeTable,
			Dimensions: dims,
			DimensionExprs: []DimensionExprAST{
				{Field: "d0", FieldExpr: "d0", Alias: in, GroupName: SlotRows},
				{Field: "d1", FieldExpr: "d1", Alias: in, GroupName: SlotColumns},
				{Field: "", FieldExpr: "", Alias: in, GroupName: in},
			},
			Metrics: []MetricExpr{
				{Field: "m0", FieldExpr: "m0", Agg: AggSum, Alias: in},
				{Field: "m1", FieldExpr: "m1", Agg: AggCount, Alias: in},
			},
			MetricExprs: []MetricPlanExpr{
				{Field: "m0", FieldExpr: "m0", Agg: AggSum, Alias: in, BindingID: in},
				{Field: "m1", FieldExpr: "m1", Agg: AggCount, Alias: in, BindingID: in},
			},
		}

		for _, ct := range zzChartTypes {
			p := GetProcessor(ct)
			_, _ = p.Process(rows, dims, metrics, ast)
			_, _ = p.Process(rows, nil, nil, nil)
			_, _ = p.Process(nil, dims, metrics, ast)
			// dims 与 ast.DimensionExprs 数量不匹配（防御路径）
			_, _ = p.Process(rows, []string{in}, metrics, ast)
		}

		// pivot v2 与 histogram / boxplot 的专有入口
		_, _ = (&PivotProcessorV2{}).Process(rows, dims, metrics, ast)
		_, _ = (&PivotProcessorV2{}).Process(rows, nil, nil, ast)
		_, _ = (&HistogramProcessor{}).ProcessBins(rows, math.NaN(), math.Inf(1), -5)
		_, _ = (&HistogramProcessor{}).ProcessBins(rows, 0, 1, 0)
		_, _ = (&HistogramProcessor{}).ProcessBins(rows, 0, 0, 3)
		for _, r := range rows {
			_, _ = (&BoxplotProcessor{}).Assemble(r, rows, int64(-1))
			_, _ = (&BoxplotProcessor{}).Assemble(nil, rows, 1<<62)
		}
	})
}
