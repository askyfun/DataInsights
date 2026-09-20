package query

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"data-insights/internal/datasource"
	"data-insights/internal/model"
)

type GeneratedSQL struct {
	Select string `json:"select_sql"`
	Count  string `json:"count_sql,omitempty"`
}

type ExecutorResult struct {
	Data interface{} `json:"data"`
	GeneratedSQL
}

// Executor 查询执行器
type Executor struct {
	conn       datasource.Connection
	dataset    *model.Dataset
	datasource *model.Datasource
}

// NewExecutor 创建新的执行器
func NewExecutor(conn datasource.Connection, dataset *model.Dataset, ds *model.Datasource) *Executor {
	return &Executor{
		conn:       conn,
		dataset:    dataset,
		datasource: ds,
	}
}

// Execute 执行图表查询
func (e *Executor) Execute(ctx context.Context, req *ChartQueryRequest) (ExecutorResult, error) {
	baseQuery, sourceType := e.getBaseQuery()
	if baseQuery == "" {
		return ExecutorResult{}, fmt.Errorf("dataset has no valid query_sql or table_name")
	}

	dialect := ParseDialect(e.datasource.Type)

	qb := NewQueryBuilder()
	qb.WithColumnMappings(e.dataset.Columns)

	ast := req.PlannedAST
	if ast == nil {
		ast = qb.Build(
			baseQuery,
			sourceType,
			req.Dims,
			req.Metrics,
			req.Filters,
			req.Sort,
			req.Pagination,
		)
	} else {
		ast.ApplyColumnMappings(qb.columnMappings)
		if ast.Source == "" {
			ast.Source = baseQuery
		}
		ast.SourceType = sourceType
	}

	if err := ast.ValidateGranularity(dialect); err != nil {
		return ExecutorResult{}, err
	}

	// pivot v2 分支（R-53）：请求携带显式 rows/columns 槽位（v2 协议，resolvePivotSlots
	// 成功）时走交叉表查询 + PivotProcessorV2；caps 只用于选择 builder——支持
	// GROUPING SETS 时生成单条分组查询，否则（MySQL/StarRocks 的
	// SupportsGroupingSets=false）走 UNION ALL 回退（三分支各自 GROUP BY，聚合值同样
	// 由数据库在每个层级重算）。两条路径产出行形状一致，共享同一份 execute/process
	// 下游。resolvePivotSlots 失败（v1 平铺请求、PlannedAST 为 nil、槽位不可解析）时
	// 落到下方既有通用路径（旧 PivotProcessor 行透传），行为与改动前完全一致。
	// GetProcessor 是按 chartType 静态选择的，感知不到运行时 Capabilities，
	// 因此 v1/v2 处理器选择必须发生在这里。
	if req.ChartType == ChartTypePivot {
		caps, err := e.conn.Capabilities(ctx)
		if err != nil {
			slog.Error("pivot: capabilities probe failed", "error", err)
			return ExecutorResult{}, fmt.Errorf("capabilities probe failed: %v", err)
		}
		if rowDims, colDims, ok := resolvePivotSlots(req.Dims, ast); ok {
			var pivotSQL string
			var pivotArgs []any
			if caps != nil && caps.SupportsGroupingSets {
				pivotSQL, pivotArgs = BuildPivotGroupingSetsQuery(dialect, ast, rowDims, colDims)
			} else {
				pivotSQL, pivotArgs = BuildPivotUnionAllQuery(dialect, ast, rowDims, colDims)
			}
			slog.Debug("executing pivot query", "sql", pivotSQL, "args", pivotArgs)

			result, err := e.conn.Execute(ctx, pivotSQL, pivotArgs...)
			if err != nil {
				return ExecutorResult{}, fmt.Errorf("query failed: %v", err)
			}

			data, err := (&PivotProcessorV2{}).Process(result.Rows, req.Dims, req.Metrics, ast)
			if err != nil {
				return ExecutorResult{}, fmt.Errorf("process failed: %v", err)
			}

			return ExecutorResult{
				Data: data,
				GeneratedSQL: GeneratedSQL{
					Select: pivotSQL,
				},
			}, nil
		}
	}

	// histogram 分支（R-57）：两阶段分箱查询——阶段1 MIN/MAX/COUNT(*)，Go 端算
	// bin 宽，阶段2 FLOOR((field-min)/width) 分箱计数。bins 组装需要阶段1 的
	// min/binWidth，这些只在 executor 内可得（通用 GetProcessor.Process 只见
	// 单阶段 rows），故与 pivot v2 分支同款走专门分支 + 专门 processor 方法。
	// bin_count/bin_width 从 req.QueryOptions 直读（最小 churn 路径，见
	// ChartQueryRequest.QueryOptions 注释），非 histogram 图型不进此分支。
	if req.ChartType == ChartTypeHistogram {
		return e.executeHistogram(ctx, dialect, ast, req)
	}

	// boxplot 分支（R-52）：三查询编排（stats → Go 端算 fence → outliers list LIMIT 1000 +
	// outliers count），与 histogram 两阶段同族。executor 内做 percentile 能力前置门（boxplot
	// stats SQL 内部使用 percentile_cont，AST metric.Agg 不是 AggMedian，故 3-4 的通用
	// AstRequiresPercentile 门不会命中，本分支必须自行 gate）。
	if req.ChartType == ChartTypeBoxplot {
		return e.executeBoxplot(ctx, dialect, ast, req)
	}

	// percentile（R-54）前置门：AST 含 median 指标时**先探 caps、不支持就在建 SQL 前显式错**，
	// 不给 DB 报语法错误或静默近似的机会（plan §4.2 明确规则）。无 median 指标时不查 caps，零开销。
	// builder 层硬编码 percentile_cont 依赖此处的契约不变式（见 renderMetricSelect 注释）。
	if AstRequiresPercentile(ast) {
		pcaps, perr := e.conn.Capabilities(ctx)
		if perr != nil {
			return ExecutorResult{}, fmt.Errorf("percentile capability probe failed: %w", perr)
		}
		if cerr := CheckPercentileSupport(pcaps); cerr != nil {
			return ExecutorResult{}, cerr
		}
	}

	sql, countSQL, args := BuildQueryStringWithBun(dialect, ast)
	slog.Debug("generated SQL", "select", sql, "count", countSQL, "args", args)

	processor := GetProcessor(req.ChartType)

	if req.ChartType == ChartTypeTable && req.Pagination != nil {
		slog.Debug("executing data query", "sql", sql)

		result, err := e.conn.Execute(ctx, sql, args...)
		if err != nil {
			return ExecutorResult{}, fmt.Errorf("query failed: %v", err)
		}

		total := len(result.Rows)
		// 总是执行 countSQL 获取正确的总数（不管返回多少条数据）
		if req.Pagination.PageSize > 0 && countSQL != "" {
			slog.Debug("executing count query", "sql", countSQL)

			countResult, err := e.conn.Execute(ctx, countSQL, args...)
			if err == nil && len(countResult.Rows) > 0 {
				// countSQL 的形状是 `SELECT COUNT(*) AS _total FROM (...) AS _count_query`，
				// 无论有无 GROUP BY 都只返回一行，总数必须取该行的 _total 值。
				// 曾按 len(Rows) 取总数：聚合查询下恒为 1，分页器永远只有一页。
				if count, ok := toFloat64(countResult.Rows[0]["_total"]); ok {
					total = int(count)
				}
			}
		}

		rows := result.Rows
		page := req.Pagination.Page
		pageSize := req.Pagination.PageSize
		totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

		// SQL 已经包含 LIMIT/OFFSET，不需要在 Go 端再次分页
		// 只需要计算 total 和 totalPages

		columns := []string{}

		// 按维度在前、指标在后的顺序构建 columns
		for _, dim := range req.Dims {
			columns = append(columns, dim)
		}
		for _, metric := range req.Metrics {
			alias := metric.ResolveAlias()
			columns = append(columns, alias)
		}

		// 重新排序数据行的字段顺序，使其与 columns 一致
		orderedRows := make([]map[string]any, len(rows))
		for i, row := range rows {
			orderedRow := make(map[string]any)
			for _, col := range columns {
				if val, ok := row[col]; ok {
					orderedRow[col] = val
				}
			}
			orderedRows[i] = orderedRow
		}

		return ExecutorResult{
			Data: &TableResponse{
				Columns: columns,
				Data:    orderedRows,
				Pagination: TablePagination{
					Page:       page,
					PageSize:   pageSize,
					Total:      total,
					TotalPages: totalPages,
				},
			},
			GeneratedSQL: GeneratedSQL{
				Select: sql,
				Count:  countSQL,
			},
		}, nil
	}

	slog.Debug("executing chart query", "sql", sql)

	result, err := e.conn.Execute(ctx, sql, args...)
	if err != nil {
		return ExecutorResult{}, fmt.Errorf("query failed: %v", err)
	}

	data, err := processor.Process(result.Rows, req.Dims, req.Metrics, ast)
	if err != nil {
		return ExecutorResult{}, fmt.Errorf("process failed: %v", err)
	}

	return ExecutorResult{
		Data: data,
		GeneratedSQL: GeneratedSQL{
			Select: sql,
		},
	}, nil
}

// executeHistogram 执行直方图两阶段查询（R-57）：
//  1. 阶段1 统计：MIN/MAX/COUNT(*)（过滤后全量），toFloat64 解析（覆盖 PG
//     numeric/bigint 形态）；
//  2. Go 端算 bin 宽/箱数，边界显式处理：
//     - total==0（空数据）→ 直接返回空 Bins（不报错，不跑阶段2）；
//     - 有行但 MIN/MAX 为 NULL（值列全 NULL）→ 显式报错，不静默；
//     - 用户 query_options.bin_width（>0）→ 覆盖 bin_count 推算的宽度，
//     箱数 = ceil((mx-mn)/bin_width)（至少 1，钳到 maxHistogramBins，钳后
//     按钳定箱数重算宽度）；
//     - mx==mn（所有值相同）→ 宽度会算出 0，兜底 bin_width=1、单箱
//     [mn, mn+1)，杜绝除 0 / NaN；
//     - 否则 bin_width = (mx-mn)/bin_count（bin_count 缺省 20）。
//  3. 阶段2 分箱：FLOOR((field-?)/?)，min/bin_width 为参数化 float args；
//  4. HistogramProcessor.ProcessBins 组装（补全空 bin + 浮点边界钳制）。
//
// GeneratedSQL 只有一个 Select 字段：放阶段2 的分箱 SQL（最终塑形查询）；
// 阶段1 统计 SQL 不外显（仅 Debug 日志）。
func (e *Executor) executeHistogram(ctx context.Context, dialect DialectType, ast *QueryAST, req *ChartQueryRequest) (ExecutorResult, error) {
	if len(req.Metrics) == 0 || req.Metrics[0].Field == "" {
		return ExecutorResult{}, fmt.Errorf("histogram requires a value field (metrics[0])")
	}
	valueField := req.Metrics[0].Field
	binCount, userBinWidth := histogramBinOptions(req.QueryOptions)

	statsSQL, statsArgs := BuildHistogramStatsQuery(dialect, ast, valueField)
	slog.Debug("executing histogram stats query", "sql", statsSQL, "args", statsArgs)
	statsResult, err := e.conn.Execute(ctx, statsSQL, statsArgs...)
	if err != nil {
		return ExecutorResult{}, fmt.Errorf("histogram stats query failed: %v", err)
	}

	// 聚合查询恒返一行；防御性地把 0 行按空数据处理（与 total==0 同口径）。
	var total, mn, mx float64
	if len(statsResult.Rows) > 0 {
		row := statsResult.Rows[0]
		var totalOK, mnOK, mxOK bool
		total, totalOK = toFloat64(row[histogramCountAlias])
		if !totalOK {
			return ExecutorResult{}, fmt.Errorf("histogram stats: count is not numeric: %v", row[histogramCountAlias])
		}
		mn, mnOK = toFloat64(row[histogramMinAlias])
		mx, mxOK = toFloat64(row[histogramMaxAlias])
		if total > 0 && (!mnOK || !mxOK) {
			return ExecutorResult{}, fmt.Errorf("histogram value field %q has no numeric values", valueField)
		}
	}
	if total == 0 {
		return ExecutorResult{
			Data:         &HistogramResponse{Bins: []HistogramBin{}},
			GeneratedSQL: GeneratedSQL{Select: statsSQL},
		}, nil
	}

	binWidth, numBins := userBinWidth, binCount
	switch {
	case userBinWidth > 0:
		numBinsF := math.Ceil((mx - mn) / userBinWidth)
		if numBinsF > maxHistogramBins {
			// 极小 bin_width 会把箱数放大到无界分配（钳制在 float 域做，
			// 荒谬值如 1e300 不会经 int 转换溢出）；钳后按钳定箱数重算宽度，
			// 保证 bins 仍连续铺满 [mn, mx]（sum(count)==total 不变式保持）。
			numBinsF = maxHistogramBins
			binWidth = (mx - mn) / numBinsF
		}
		numBins = int(numBinsF)
		if numBins < 1 {
			numBins = 1 // mx==mn（或宽度大于值域）：单箱
		}
	case mx == mn:
		binWidth, numBins = 1, 1 // 全同值：兜底宽 1，单箱 [mn, mn+1)，不除 0
	default:
		binWidth = (mx - mn) / float64(binCount)
	}

	binSQL, binArgs := BuildHistogramBinQuery(dialect, ast, valueField, mn, binWidth)
	slog.Debug("executing histogram bin query", "sql", binSQL, "args", binArgs)
	binResult, err := e.conn.Execute(ctx, binSQL, binArgs...)
	if err != nil {
		return ExecutorResult{}, fmt.Errorf("histogram bin query failed: %v", err)
	}

	resp, err := (&HistogramProcessor{}).ProcessBins(binResult.Rows, mn, binWidth, numBins)
	if err != nil {
		return ExecutorResult{}, fmt.Errorf("process failed: %v", err)
	}

	return ExecutorResult{
		Data: resp,
		GeneratedSQL: GeneratedSQL{
			Select: binSQL,
		},
	}, nil
}

// getBaseQuery 获取基础查询 SQL
func (e *Executor) getBaseQuery() (string, SourceType) {
	if e.dataset.QueryType == "sql" && e.dataset.QuerySQL.Valid {
		return e.dataset.QuerySQL.String, SourceTypeSQL
	}
	if e.dataset.TableName.Valid {
		return e.dataset.TableName.String, SourceTypeTable
	}
	return "", SourceTypeTable
}

// ExecuteRawQuery 执行原始查询
func (e *Executor) ExecuteRawQuery(ctx context.Context, sql string) ([]map[string]any, error) {
	result, err := e.conn.Execute(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("query failed: %v", err)
	}
	return result.Rows, nil
}

// Close 关闭连接
func (e *Executor) Close() error {
	return e.conn.Close()
}

// executeBoxplot 执行箱线图三查询编排（R-52）：
//  1. **前置门**：先探 dialect capabilities，若 PercentileStrategy 不支持 percentile_cont
//     → 立即返回明确 error（plan §4.2 + 行569 文案，boxplot stats SQL 内部使用了 percentile_cont，
//     不支持的 dialect 走不到建 SQL 那一步）。CH/MySQL/StarRocks 现状（Task 3-0 保守裁定
//     strategy="unsupported"）都走这条 error 分支，不会拿到近似值或 DB 语法错。
//  2. **stats**：`SELECT MIN, percentile_cont(0.25), percentile_cont(0.5), percentile_cont(0.75), MAX`。
//     空/全 NULL 结果 → 退化 BoxplotResponse（五值 0、空 outliers、total 0、truncated false），
//     不跑后两查询（避免对无意义数据发多余 SQL）。
//  3. **fence（Go 端算）**：`iqr = q3-q1; lower = q1 - 1.5*iqr; upper = q3 + 1.5*iqr`。
//  4. **outliers list** + **outliers count**：两次独立查询共享同一 fence WHERE 谓词，
//     全部参数化传入，绝不 fmt 拼裸浮点（防注入 + 浮点字符串化方言差异）。
//     list 受 LIMIT 1000 截断（boxOutlierDisplayMax 常量），count 给真实总数与 truncated 判定。
//  5. **Assemble**：BoxplotProcessor.Assemble 纯函数装配（可独立单测）。
func (e *Executor) executeBoxplot(
	ctx context.Context, dialect DialectType, ast *QueryAST, req *ChartQueryRequest,
) (ExecutorResult, error) {
	if len(req.Metrics) == 0 || req.Metrics[0].Field == "" {
		return ExecutorResult{}, fmt.Errorf("boxplot requires a value field (metrics[0])")
	}
	valueField := req.Metrics[0].Field

	// 前置门：boxplot stats 内部使用 percentile_cont，dialect 不支持时必须显式失败。
	caps, err := e.conn.Capabilities(ctx)
	if err != nil {
		return ExecutorResult{}, fmt.Errorf("boxplot capability probe failed: %w", err)
	}
	if perr := CheckPercentileSupport(caps); perr != nil {
		strategy := "unknown"
		if caps != nil {
			strategy = caps.PercentileStrategy
		}
		return ExecutorResult{}, fmt.Errorf(
			"当前数据源不支持箱线图（需要 percentile 能力，策略 %q）：%w", strategy, perr)
	}

	statsSQL, statsArgs, berr := BuildBoxplotStatsQuery(dialect, ast, valueField, caps)
	if berr != nil {
		return ExecutorResult{}, fmt.Errorf("boxplot stats SQL: %w", berr)
	}
	slog.Debug("executing boxplot stats query", "sql", statsSQL, "args", statsArgs)
	statsResult, err := e.conn.Execute(ctx, statsSQL, statsArgs...)
	if err != nil {
		return ExecutorResult{}, fmt.Errorf("boxplot stats query failed: %v", err)
	}

	// 空/退化：stats 无行或 q1/median/q3 全 NULL → 直接产退化结构，不跑后两查询。
	var statsRow map[string]any
	if len(statsResult.Rows) > 0 {
		statsRow = statsResult.Rows[0]
	}
	if isEmptyBoxplotStats(statsRow) {
		empty := &BoxplotResponse{Outliers: []float64{}}
		return ExecutorResult{
			Data:         empty,
			GeneratedSQL: GeneratedSQL{Select: statsSQL},
		}, nil
	}

	q1, _ := toFloat64(statsRow[boxQ1Alias])
	q3, _ := toFloat64(statsRow[boxQ3Alias])
	iqr := q3 - q1
	lower := q1 - boxIQRMultiplierConst*iqr
	upper := q3 + boxIQRMultiplierConst*iqr

	outlierSQL, outlierArgs := BuildBoxplotOutliersQuery(dialect, ast, valueField, lower, upper)
	slog.Debug("executing boxplot outliers list query", "sql", outlierSQL, "args", outlierArgs)
	outlierResult, err := e.conn.Execute(ctx, outlierSQL, outlierArgs...)
	if err != nil {
		return ExecutorResult{}, fmt.Errorf("boxplot outliers list query failed: %v", err)
	}

	countSQL, countArgs := BuildBoxplotOutlierCountQuery(dialect, ast, valueField, lower, upper)
	slog.Debug("executing boxplot outliers count query", "sql", countSQL, "args", countArgs)
	countResult, err := e.conn.Execute(ctx, countSQL, countArgs...)
	if err != nil {
		return ExecutorResult{}, fmt.Errorf("boxplot outliers count query failed: %v", err)
	}
	var outlierTotal int64
	if len(countResult.Rows) > 0 {
		if f, ok := toFloat64(countResult.Rows[0][boxOutlierCountAlias]); ok {
			outlierTotal = int64(f)
		}
	}

	resp, err := (&BoxplotProcessor{}).Assemble(statsRow, outlierResult.Rows, outlierTotal)
	if err != nil {
		return ExecutorResult{}, fmt.Errorf("boxplot assemble failed: %v", err)
	}
	return ExecutorResult{
		Data:         resp,
		GeneratedSQL: GeneratedSQL{Select: statsSQL},
	}, nil
}

// isEmptyBoxplotStats 判 stats 行是否代表空数据集：nil 或 q1/median/q3 三键都不可转数值
// （PG 上空集聚合返回 1 行、percentile_cont 与 MIN/MAX 均为 NULL；toFloat64 对 NULL 返回 false）。
func isEmptyBoxplotStats(row map[string]any) bool {
	if row == nil {
		return true
	}
	_, q1OK := toFloat64(row[boxQ1Alias])
	_, medOK := toFloat64(row[boxMedianAlias])
	_, q3OK := toFloat64(row[boxQ3Alias])
	return !q1OK && !medOK && !q3OK
}
