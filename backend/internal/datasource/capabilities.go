package datasource

// DialectCapabilities 描述一个数据源连接的查询能力（懒加载，首次调用时探测并缓存——
// 缓存逻辑本身是 Task 3-0 的工作，本任务只定义类型，不做真实探测）。
type DialectCapabilities struct {
	SupportsGroupingSets    bool
	SupportsPercentileCont  bool
	SupportsWindowFunctions bool
	// PercentileStrategy 取值："percentile_cont"（PG）| "percentile_cont_args_first"（StarRocks）| "quantilesExactInclusive" | "window_ntile" | "unsupported"
	PercentileStrategy string
}
