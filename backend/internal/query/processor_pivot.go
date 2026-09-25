package query

import (
	"fmt"
	"log/slog"
	"strings"
)

// pivot 的维度槽位名（v2 协议 dimension_groups[].name，
// 与 frontend/src/components/ChartBuilder/chartDefinitions.ts 的 pivot fieldGroups[].id 对齐）。
const (
	SlotRows    = "rows"
	SlotColumns = "columns"
)

// PivotSubtotalColKey 是小计/合计行在 PivotRow.Values 里使用的哨兵列键。
// GROUPING SETS 语义下，行小计/合计行的列维度被汇总掉（结果行里列维度值为 NULL），
// 不对应任何具体 colHeader；Values 是扁平 map，必须给"跨所有列的汇总值"一个明确的键。
// 不用空字符串（会与"列维度值恰好是空字符串"的真实数据混淆），而用双下划线哨兵。
// 该键是前后端契约的一部分（Task 2-3 前端消费方按此键读取小计/合计值）。
const PivotSubtotalColKey = "__subtotal__"

// pivotValueKeySep 是 Values 键里 colHeader 与 metricAlias 的分隔符。
const pivotValueKeySep = "|"

// PivotValueKey 生成 PivotRow.Values 的键：明细行为 "<colHeader>|<metricAlias>"，
// 小计/合计行为 "<PivotSubtotalColKey>|<metricAlias>"。
// 这是与 Task 2-3 前端消费方约定的唯一键命名规则。
func PivotValueKey(colHeader, metricAlias string) string {
	return colHeader + pivotValueKeySep + metricAlias
}

// pivotRowMarkerAlias / pivotColMarkerAlias 是 GROUPING SETS 查询里 GROUPING() 标记列的
// 输出别名：每个行/列维度各一列（单参 GROUPING()，跨方言最稳），值 1 表示该维度在这一行
// 被汇总掉。SQL 生成（bun_builder_pivot.go）与结果分类（PivotProcessorV2）共用这套命名。
func pivotRowMarkerAlias(i int) string {
	return fmt.Sprintf("__pivot_row_grp_%d", i)
}

func pivotColMarkerAlias(j int) string {
	return fmt.Sprintf("__pivot_col_grp_%d", j)
}

// resolvePivotSlots 尝试从 AST 里按索引解析 pivot 的 rows/columns 槽位
// （与 resolveAxisSlots 同一模式：dims 与 ast.DimensionExprs 由 service 层同源的
// QuerySpec 分别平铺而来，按索引一一对应，额外校验 Field 名对齐）。
// 返回 ok=false 的情况（调用方必须回退到旧的行透传路径）：
//   - v1 平铺协议：ast 为 nil / 无 DimensionExprs / GroupName 全为空或 "rows"
//     （v1 默认组名规则把 pivot 所有维度标成 "rows"，没有 columns 组）；
//   - 数量或字段名不匹配（防御性兜底）；
//   - 出现 rows/columns 之外的未知槽位名；
//   - rows 或 columns 任一槽位为空（缺列维度时交叉表退化为无意义形状）。
func resolvePivotSlots(dims []string, ast *QueryAST) (rowDims, colDims []DimensionExprAST, ok bool) {
	if ast == nil || len(ast.DimensionExprs) != len(dims) || len(dims) == 0 {
		return nil, nil, false
	}
	for i, d := range dims {
		expr := ast.DimensionExprs[i]
		// dims 有两种口径，都要接受：Planner 原样下传的字段标识（列 ID），以及
		// executor 翻译后的展示名（负载口径，见 idx.localizeFields）。两种都对不上
		// 才判定槽位不可解析——保持旧的"顺序必须与 AST 一致"这道守卫不变。
		if d != expr.Field && d != pivotDimOutputKey(expr) {
			return nil, nil, false
		}
		switch expr.GroupName {
		case SlotRows:
			rowDims = append(rowDims, expr)
		case SlotColumns:
			colDims = append(colDims, expr)
		default:
			// v1 请求（GroupName 为空）或未知槽位：不能安全切分行/列维度，回退旧路径。
			return nil, nil, false
		}
	}
	if len(rowDims) == 0 || len(colDims) == 0 {
		return nil, nil, false
	}
	return rowDims, colDims, true
}

// pivotDimOutputKey 返回维度在结果行 map 里的键（SELECT 输出列名）：
// 带别名（改名/时间粒度）的维度输出 AS 别名，普通维度输出字段名本身。
func pivotDimOutputKey(dim DimensionExprAST) string {
	if dim.Alias != "" {
		return dim.Alias
	}
	return dim.Field
}

// PivotProcessorV2 透视表 v2 处理器（R-53）：消费 GROUPING SETS 或 UNION ALL 查询的结果行
// （两种 builder 产出行形状相同），组装交叉表形状 PivotResponseV2。核心不变量：所有聚合值
// （含小计/合计）都直接取数据库在每个分组层级上重算的结果（AVG/COUNT(DISTINCT) 的小计正确性
// 由此保证），Go 端绝不对子分组结果做二次聚合。
// 只要 resolvePivotSlots 成功即被调用；caps.SupportsGroupingSets 只决定 executor 用哪个 builder
// （true→GROUPING SETS 单条分组查询，false→UNION ALL 三分支回退），两者产出相同的行形状
// （同款 __pivot_row_grp_i/__pivot_col_grp_j 标记列 + 指标别名）供本 processor 消费。
// resolvePivotSlots 失败（v1 平铺请求、无 columns 槽位）才走旧的 PivotProcessor 行透传。
type PivotProcessorV2 struct{}

// Process 处理 GROUPING SETS 或 UNION ALL 结果行（行形状相同）。每行按 GROUPING() 标记列分类：
//   - 所有标记为 0：明细行 → 按 RowKey 合并进 Cells（同一 RowKey 的多条明细
//     交叉进同一个 PivotRow 的 Values，键 "<colHeader>|<metricAlias>"）；
//   - 列维度标记全 1、行维度标记全 0：行小计 → 追加一个 IsSubtotal=true 的 PivotRow，
//     RowKey 仍是具体行维度值，Values 键用哨兵 "__subtotal__|<metricAlias>"；
//   - 全部标记为 1：合计 → GrandTotal（RowKey 为空数组，IsSubtotal=true，Values 同用哨兵键）；
//   - 其他部分汇总组合：本处理器的 SQL 生成不会产出（两种 builder 都只产出
//     (rows+cols)/(rows)/() 三个组合，不会出现部分汇总形状），
//     出现即视为契约破坏，记录日志并返回错误（禁止静默吞掉）。
func (p *PivotProcessorV2) Process(rows []map[string]any, dims []string, metrics []MetricConfig, ast *QueryAST) (ChartQueryResponse, error) {
	rowDims, colDims, ok := resolvePivotSlots(dims, ast)
	if !ok {
		// executor 只在 resolvePivotSlots 成功后才路由到本处理器，正常不可达；
		// 防御性返回错误而不是静默产出错误形状。
		return nil, fmt.Errorf("pivot v2 processor requires resolvable rows/columns slots in AST")
	}

	rowKeys := make([]string, len(rowDims))
	for i, d := range rowDims {
		rowKeys[i] = pivotDimOutputKey(d)
	}
	colKeys := make([]string, len(colDims))
	for j, d := range colDims {
		colKeys[j] = pivotDimOutputKey(d)
	}
	metricAliases := make([]string, len(metrics))
	for k, m := range metrics {
		metricAliases[k] = m.ResolveAlias()
	}

	resp := &PivotResponseV2{
		RowHeaders:  rowKeys,
		ColHeaders:  []string{},
		MetricNames: metricAliases,
		Cells:       []PivotRow{},
	}
	if len(rows) == 0 {
		return resp, nil
	}

	colHeaderSeen := make(map[string]bool)
	cellIndex := make(map[string]int) // RowKey 组合串 → Cells 下标（明细行交叉合并）

	for _, row := range rows {
		rowFlags, err := readGroupingFlags(row, len(rowDims), pivotRowMarkerAlias)
		if err != nil {
			return nil, err
		}
		colFlags, err := readGroupingFlags(row, len(colDims), pivotColMarkerAlias)
		if err != nil {
			return nil, err
		}
		rowAllOn, rowAllOff := allTrue(rowFlags), allFalse(rowFlags)
		colAllOn, colAllOff := allTrue(colFlags), allFalse(colFlags)

		values := make(map[string]float64, len(metricAliases))
		switch {
		case rowAllOn && colAllOn:
			// 合计行：行、列维度都被汇总掉。
			for _, alias := range metricAliases {
				values[PivotValueKey(PivotSubtotalColKey, alias)] = metricValue(row, alias)
			}
			resp.GrandTotal = &PivotRow{RowKey: []string{}, IsSubtotal: true, Values: values}
		case rowAllOff && colAllOn:
			// 行小计：列维度被汇总掉，行维度保留具体值。
			rowKey := extractDimValues(row, rowKeys)
			for _, alias := range metricAliases {
				values[PivotValueKey(PivotSubtotalColKey, alias)] = metricValue(row, alias)
			}
			resp.Cells = append(resp.Cells, PivotRow{RowKey: rowKey, IsSubtotal: true, Values: values})
		case rowAllOff && colAllOff:
			// 明细行：交叉合并进同一 RowKey 的 PivotRow。
			rowKey := extractDimValues(row, rowKeys)
			colHeader := strings.Join(extractDimValues(row, colKeys), " - ")
			if !colHeaderSeen[colHeader] {
				colHeaderSeen[colHeader] = true
				resp.ColHeaders = append(resp.ColHeaders, colHeader)
			}
			mergeKey := strings.Join(rowKey, "\x00")
			idx, exists := cellIndex[mergeKey]
			if !exists {
				idx = len(resp.Cells)
				cellIndex[mergeKey] = idx
				resp.Cells = append(resp.Cells, PivotRow{
					RowKey:     rowKey,
					IsSubtotal: false,
					Values:     map[string]float64{},
				})
			}
			for _, alias := range metricAliases {
				resp.Cells[idx].Values[PivotValueKey(colHeader, alias)] = metricValue(row, alias)
			}
		default:
			// 部分汇总（如行维度被汇总但列维度保留）：BuildPivotGroupingSetsQuery 与
			// BuildPivotUnionAllQuery 都只产出 (rows+cols)/(rows)/() 三个组合，不会出现该形状。
			err := fmt.Errorf("unexpected partial grouping row (rowFlags=%v, colFlags=%v)", rowFlags, colFlags)
			slog.Warn("pivot v2: dropping malformed grouping-sets row", "error", err)
			return nil, err
		}
	}

	return resp, nil
}

// readGroupingFlags 读取一行里 n 个 GROUPING() 标记列的值（1=该维度被汇总掉）。
// 标记列缺失或不可转数值视为契约破坏（行不是 GROUPING SETS 查询的产物），返回错误。
func readGroupingFlags(row map[string]any, n int, alias func(int) string) ([]bool, error) {
	flags := make([]bool, n)
	for i := 0; i < n; i++ {
		key := alias(i)
		raw, exists := row[key]
		if !exists {
			return nil, fmt.Errorf("pivot v2: grouping marker column %q missing from result row", key)
		}
		f, ok := toFloat64(raw)
		if !ok {
			return nil, fmt.Errorf("pivot v2: grouping marker column %q is not numeric: %v", key, raw)
		}
		flags[i] = f == 1
	}
	return flags, nil
}

// extractDimValues 按输出键提取维度值组合；NULL（含 GROUPING SETS 汇总掉的维度
// 与数据本身的 NULL）统一渲染为空字符串，与 PieProcessor 对 NULL 类目名的处理一致。
func extractDimValues(row map[string]any, keys []string) []string {
	vals := make([]string, len(keys))
	for i, k := range keys {
		if v := row[k]; v != nil {
			vals[i] = toString(v)
		}
	}
	return vals
}

// metricValue 读取指标聚合值；NULL（如空分组上的 AVG）或不可转数值时落 0，
// 与 KpiProcessor 的 Value:0 兜底约定一致。
func metricValue(row map[string]any, alias string) float64 {
	if v, ok := toFloat64(row[alias]); ok {
		return v
	}
	return 0
}

func allTrue(flags []bool) bool {
	for _, f := range flags {
		if !f {
			return false
		}
	}
	return true
}

func allFalse(flags []bool) bool {
	for _, f := range flags {
		if f {
			return false
		}
	}
	return true
}
