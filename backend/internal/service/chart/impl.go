package chart

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dataray/internal/database"
	"dataray/internal/datasource"
	"dataray/internal/domain/entity"
	"dataray/internal/model"
	"dataray/internal/query"
	"dataray/internal/response"
	"dataray/internal/router"
	dsservice "dataray/internal/service/datasource"

	"github.com/uptrace/bun"
)

// Service defines the interface for chart operations
type Service interface {
	// CRUD operations
	List(ctx context.Context, limit, offset int) ([]entity.Chart, error)
	GetByID(ctx context.Context, id int) (*entity.Chart, error)
	Create(ctx context.Context, chart *entity.Chart) (*entity.Chart, error)
	Update(ctx context.Context, chart *entity.Chart) (*entity.Chart, error)
	Delete(ctx context.Context, id int) error

	// Data operations
	GetData(ctx context.Context, id int) (entity.ChartDataResult, error)
	Query(ctx context.Context, req *entity.ChartQueryRequest) (entity.ChartDataResult, error)

	// SetSecurityKey injects the 32-byte AES key used to decrypt datasource
	// passwords at rest. A nil key keeps plaintext passthrough (dev mode).
	SetSecurityKey(key []byte)
}

// chartService implements the Service interface
type chartService struct {
	db                   *bun.DB
	key                  []byte // 32-byte AES key; nil => plaintext passthrough
	connectFn            func(ctx context.Context, ds *model.Datasource) (datasource.Connection, error)
	dialFn               func(ctx context.Context, ds *model.Datasource, password string) (datasource.Connection, error)
	executorFactory      func(conn datasource.Connection, dataset *model.Dataset, ds *model.Datasource) queryExecutor
	getChartModelFn      func(ctx context.Context, id int) (*model.Chart, error)
	getDatasetModelFn    func(ctx context.Context, id int) (*model.Dataset, error)
	getDatasourceModelFn func(ctx context.Context, id int) (*model.Datasource, error)
}

// queryExecutor 抽象 query.Executor，便于 service 层注入测试替身。
// 调用场景：chartService.Query 在拿到 dataset/datasource/connection 后，通过该接口执行图表查询。
type queryExecutor interface {
	// Execute 执行图表查询并返回图表数据和生成 SQL。
	Execute(ctx context.Context, req *query.ChartQueryRequest) (query.ExecutorResult, error)
}

// NewService creates a new chart service
func NewService(db *bun.DB) Service {
	service := &chartService{db: db}
	service.connectFn = service.connect
	service.dialFn = service.dial
	service.executorFactory = func(conn datasource.Connection, dataset *model.Dataset, ds *model.Datasource) queryExecutor {
		return query.NewExecutor(conn, dataset, ds)
	}
	service.getChartModelFn = service.getChartModel
	service.getDatasetModelFn = service.getDatasetModel
	service.getDatasourceModelFn = service.getDatasourceModel
	return service
}

// List returns all charts with pagination
func (s *chartService) List(ctx context.Context, limit, offset int) ([]entity.Chart, error) {
	var charts []model.Chart
	q := s.db.NewSelect().Model(&charts).Where("deleted_at IS NULL")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list charts: %w", err)
	}
	return toChartEntityList(charts), nil
}

// GetByID returns a chart by ID
func (s *chartService) GetByID(ctx context.Context, id int) (*entity.Chart, error) {
	chart := &model.Chart{ID: id}
	if err := s.db.NewSelect().Model(chart).WherePK().Where("deleted_at IS NULL").Scan(ctx); err != nil {
		return nil, fmt.Errorf("chart not found: %w", err)
	}
	return toChartEntity(chart), nil
}

// Create creates a new chart
func (s *chartService) Create(ctx context.Context, chart *entity.Chart) (*entity.Chart, error) {
	m := toChartModel(chart)
	if _, err := s.db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create chart: %w", err)
	}
	return toChartEntity(m), nil
}

// Update updates an existing chart
func (s *chartService) Update(ctx context.Context, chart *entity.Chart) (*entity.Chart, error) {
	m := toChartModel(chart)
	if _, err := s.db.NewUpdate().Model(m).WherePK().Where("deleted_at IS NULL").ExcludeColumn("deleted_at").Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to update chart: %w", err)
	}
	updated := &model.Chart{ID: chart.ID}
	if err := s.db.NewSelect().Model(updated).WherePK().Where("deleted_at IS NULL").Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to get updated chart: %w", err)
	}
	return toChartEntity(updated), nil
}

// Delete soft-deletes a chart by ID and cascades the soft delete to all of its
// shares, within a single transaction. The row is never physically removed
// (deleted_at is stamped instead).
func (s *chartService) Delete(ctx context.Context, id int) error {
	return database.WithTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		// 软删图表自身（幂等：已删除/不存在影响 0 行不报错）。
		if _, err := tx.NewUpdate().
			Model((*model.Chart)(nil)).
			Set("deleted_at = now()").
			Where("id = ?", id).
			Where("deleted_at IS NULL").
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to delete chart: %w", err)
		}
		// 级联软删其下分享。
		if _, err := tx.NewUpdate().
			Model((*model.Share)(nil)).
			Set("deleted_at = now()").
			Where("chart_id = ?", id).
			Where("deleted_at IS NULL").
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to cascade delete shares: %w", err)
		}
		return nil
	})
}

// GetData returns data for a chart. A persisted v1/v2 config is executed through
// the same aggregation pipeline as POST /api/charts/query, so a shared chart
// renders the same result as the builder preview. Parse failure / legacy
// config / empty groups keep the historical raw-rows fallback (top 100 rows
// of the bound dataset): those configs carry positional field ids whose column
// names are not recoverable server-side, and the share view renders them.
func (s *chartService) GetData(ctx context.Context, id int) (entity.ChartDataResult, error) {
	chart, err := s.getChartModelFn(ctx, id)
	if err != nil {
		return entity.ChartDataResult{}, err
	}

	dataset, err := s.getDatasetModelFn(ctx, chart.DatasetID)
	if err != nil {
		return entity.ChartDataResult{}, err
	}

	dsModel, err := s.getDatasourceModelFn(ctx, dataset.DatasourceID)
	if err != nil {
		return entity.ChartDataResult{}, err
	}

	conn, err := s.connectFn(ctx, dsModel)
	if err != nil {
		return entity.ChartDataResult{}, err
	}
	defer conn.Close()

	if req, ok := chartDataQueryFromConfig(chart); ok {
		return s.executeQueryOnConn(ctx, conn, dataset, dsModel, req)
	}

	// Fallback path (documented above): build query based on dataset type via
	// the query package.
	source := getPlannerSource(dataset)
	if source == "" {
		return entity.ChartDataResult{}, fmt.Errorf("dataset has no valid query_sql or table_name")
	}
	dataSQL := query.WrapPreviewSQL(source, getPlannerSourceType(dataset), 100)
	result, err := conn.Execute(ctx, dataSQL)
	if err != nil {
		return entity.ChartDataResult{}, fmt.Errorf("query failed: %w", err)
	}

	return entity.ChartDataResult{Data: result.Rows}, nil
}

// Query executes a chart query
func (s *chartService) Query(ctx context.Context, req *entity.ChartQueryRequest) (entity.ChartDataResult, error) {
	dataset, err := s.getDatasetModelFn(ctx, req.DatasetID)
	if err != nil {
		return entity.ChartDataResult{}, err
	}

	dsModel, err := s.getDatasourceModelFn(ctx, dataset.DatasourceID)
	if err != nil {
		return entity.ChartDataResult{}, err
	}

	conn, err := s.connectFn(ctx, dsModel)
	if err != nil {
		return entity.ChartDataResult{}, err
	}
	defer conn.Close()

	return s.executeQueryOnConn(ctx, conn, dataset, dsModel, req)
}

// executeQueryOnConn runs the shared entity request → QuerySpec → planner →
// executor pipeline on an already-dialled connection.
// 调用场景：Query（builder 预览）与 GetData（v1 配置分享页）共用，保证两端结果一致。
func (s *chartService) executeQueryOnConn(
	ctx context.Context,
	conn datasource.Connection,
	dataset *model.Dataset,
	dsModel *model.Datasource,
	req *entity.ChartQueryRequest,
) (entity.ChartDataResult, error) {
	if err := validateFilterFields(req.Filters); err != nil {
		return entity.ChartDataResult{}, err
	}

	executor := s.executorFactory(conn, dataset, dsModel)
	// v2 协议（spec_version=2）走槽位保留路径：ChartSpecFromRequestV2 →
	// QuerySpecFromChartSpecV2（GroupName/BindingID 随 PlanAST 传入 AST）；
	// v1/缺失走原有平铺路径，逻辑不变。
	var querySpec *query.QuerySpec
	if req.SpecVersion != nil && *req.SpecVersion == 2 {
		querySpec = buildQuerySpecFromEntityRequestV2(req)
	} else {
		querySpec = buildQuerySpecFromEntityRequest(req)
	}
	plannedQuery := query.NewQueryPlanner().Plan(querySpec)
	plannedAST := query.NewQueryPlanner().PlanAST(getPlannerSource(dataset), getPlannerSourceType(dataset), querySpec)

	queryReq := &query.ChartQueryRequest{
		DatasetID:  req.DatasetID,
		ChartType:  query.ChartType(req.ChartType),
		Dims:       plannedQuery.Dims,
		Metrics:    plannedQuery.Metrics,
		Filters:    plannedQuery.Filters,
		Pagination: plannedQuery.Pagination,
		Sort:       plannedQuery.Sort,
		PlannedAST: plannedAST,
		// histogram 的 bin_count/bin_width 走请求结构体直传 executor（R-57，
		// 不进入 QuerySpec/planner）。buildQuerySpecFromEntityRequest 的中间
		// request 只喂 QuerySpecFromRequest（planner），无需携带。
		QueryOptions: req.QueryOptions,
	}

	result, err := executor.Execute(ctx, queryReq)
	if err != nil {
		return entity.ChartDataResult{}, fmt.Errorf("query execution failed: %w", err)
	}

	return entity.ChartDataResult{
		Data:      result.Data,
		SelectSQL: result.Select,
		CountSQL:  result.Count,
	}, nil
}

// chartConfigV1 是 bi_chart.config 持久化 v1 文档中重建查询所需的最小子集，
// 键名与 frontend/src/lib/chartConfigSchema.ts 的 ChartConfigDocument 对齐。
// 判别走 "query 键存在且组非空"（旧结构用 queryConfig 键，天然落空），
// 因此不读 version。
type chartConfigV1 struct {
	ChartType string `json:"chartType"`
	Query     *struct {
		DimensionGroups []entity.FieldGroup `json:"dimensionGroups"`
		MetricGroups    []entity.FieldGroup `json:"metricGroups"`
		Filters         []chartConfigFilter `json:"filters"`
		Sort            *entity.SortConfig  `json:"sort"`
		Limit           int                 `json:"limit"`
	} `json:"query"`
	FieldMeta map[string]struct {
		Aggregation string `json:"aggregation"`
		Alias       string `json:"alias"`
	} `json:"fieldMeta"`
	// QueryOptions 顶层透传小节（frontend chartConfigSchema.ts 的 queryOptions）：
	// histogram 持久化的 bin_count/bin_width 存这里（R-57 持久化 round-trip）。
	QueryOptions map[string]any `json:"queryOptions"`
}

// chartConfigFilter 兼容文档内的两种 value 区间键：保存自运行时 FilterCondition
// 时为 camelCase（valueEnd），wire 风格 JSON 为 snake_case（value_end）。
type chartConfigFilter struct {
	Field      string `json:"field"`
	Operator   string `json:"operator"`
	Value      any    `json:"value"`
	ValueEnd   any    `json:"value_end"`
	ValueEndCC any    `json:"valueEnd"`
	Logic      string `json:"logic"`
}

// chartConfigV2 是 bi_chart.config 持久化 v2 文档（Task 0-3 起前端保存的形状）中
// 重建查询所需的最小子集：字段组由 fields: string[] 升级为 bindings: BindingInstance[]，
// fieldMeta 键由列名改为 bindingId。键名与 frontend/src/lib/chartConfigSchema.ts 对齐。
type chartConfigV2 struct {
	ChartType string `json:"chartType"`
	Query     *struct {
		DimensionGroups []chartConfigBindingGroup `json:"dimensionGroups"`
		MetricGroups    []chartConfigBindingGroup `json:"metricGroups"`
		Filters         []chartConfigFilter       `json:"filters"`
		Sort            *chartConfigSort          `json:"sort"`
		Limit           int                       `json:"limit"`
	} `json:"query"`
	FieldMeta map[string]struct {
		Aggregation string `json:"aggregation"`
		Alias       string `json:"alias"`
	} `json:"fieldMeta"`
	// QueryOptions 顶层透传小节（与 v1 同款，v2 文档"queryOptions 小节形状不变"，
	// 见 frontend chartConfigSchema.ts）：histogram 的 bin_count/bin_width 持久化处。
	QueryOptions map[string]any `json:"queryOptions"`
}

// chartConfigSort v2 持久化文档的 sort 小节，兼容两种键形态：Task 1-7 起前端保存
// bindingId 键（引用 BindingInstance，与前端 QueryConfig.sort 对齐）；Task 0-3~1-7
// 窗口期保存的文档是 field 键（输出列名/别名）。翻译逻辑见 resolveV2DocSort。
type chartConfigSort struct {
	Field     string `json:"field"`
	BindingID string `json:"bindingId"`
	Order     string `json:"order"`
}

// chartConfigBindingGroup v2 文档字段组：bindings 为带 bindingId 的字段实例
// （对应前端 BindingInstance，键为 camelCase bindingId）。
type chartConfigBindingGroup struct {
	ID       string `json:"id"`
	Bindings []struct {
		BindingID string `json:"bindingId"`
		Field     string `json:"field"`
	} `json:"bindings"`
}

// chartDataQueryFromConfig parses a persisted chart config into the same
// entity request the interactive builder sends (see ChartBuilder's
// composeChartQueryRequest): dims flatten all dimension-group fields, metrics
// flatten all metric-group fields with agg from fieldMeta (default "sum") and
// alias from fieldMeta (default = column name). Returns ok=false for legacy
// (queryConfig + positional ids), malformed, or empty-group configs.
// version 判别（与前端 migrateChartConfig 对称，裁定3）：version==2 走 v2 解析
// 路径（bindings 结构 + bindingId 键 fieldMeta）；version 缺失或 !=2 走下面的
// v1 路径（逻辑与修复前完全一致）。
func chartDataQueryFromConfig(chart *model.Chart) (*entity.ChartQueryRequest, bool) {
	var versionProbe struct {
		Version int `json:"version"`
	}
	// 探针解析失败（损坏 JSON）不单独处理：v1 路径同样会解析失败并返回 ok=false，
	// 与修复前行为一致。
	if json.Unmarshal([]byte(chart.Config), &versionProbe) == nil && versionProbe.Version == 2 {
		return chartDataQueryFromConfigV2(chart)
	}

	var doc chartConfigV1
	if err := json.Unmarshal([]byte(chart.Config), &doc); err != nil || doc.Query == nil {
		return nil, false
	}

	var dims []string
	for _, group := range doc.Query.DimensionGroups {
		dims = append(dims, group.Fields...)
	}
	var metrics []entity.MetricConfig
	for _, group := range doc.Query.MetricGroups {
		for _, name := range group.Fields {
			meta := doc.FieldMeta[name]
			agg := meta.Aggregation
			if agg == "" {
				agg = "sum"
			}
			alias := meta.Alias
			if alias == "" {
				alias = name
			}
			metrics = append(metrics, entity.MetricConfig{Field: name, Agg: agg, Alias: alias})
		}
	}
	if len(dims) == 0 && len(metrics) == 0 {
		return nil, false
	}

	req := buildConfigQueryRequest(chart, doc.ChartType, dims, metrics, doc.Query.Filters, doc.Query.Sort, doc.Query.Limit)
	req.QueryOptions = normalizePersistedQueryOptions(doc.QueryOptions)
	return req, true
}

// chartDataQueryFromConfigV2 解析 v2 持久化文档（Task 0-3 起前端保存的形状）并
// 重建平铺 entity.ChartQueryRequest：dims 平铺各组 bindings[].field，metrics 平铺
// bindings[].field 并按 bindingId 查 fieldMeta 取 aggregation（默认 sum）/alias
// （默认列名），与 v1 路径按列名查 fieldMeta 的逻辑对称。槽位语义暂不下传：
// GetData 路径的现有消费方仍按 chartType 走位置推断（裁定3，后续任务再评估升级）。
// 空组/损坏文档返回 ok=false（回退裸数据行，与 v1 口径一致）。
func chartDataQueryFromConfigV2(chart *model.Chart) (*entity.ChartQueryRequest, bool) {
	var doc chartConfigV2
	if err := json.Unmarshal([]byte(chart.Config), &doc); err != nil || doc.Query == nil {
		return nil, false
	}

	var dims []string
	for _, group := range doc.Query.DimensionGroups {
		for _, b := range group.Bindings {
			dims = append(dims, b.Field)
		}
	}
	var metrics []entity.MetricConfig
	for _, group := range doc.Query.MetricGroups {
		for _, b := range group.Bindings {
			meta := doc.FieldMeta[b.BindingID]
			agg := meta.Aggregation
			if agg == "" {
				agg = "sum"
			}
			alias := meta.Alias
			if alias == "" {
				alias = b.Field
			}
			metrics = append(metrics, entity.MetricConfig{Field: b.Field, Agg: agg, Alias: alias})
		}
	}
	if len(dims) == 0 && len(metrics) == 0 {
		return nil, false
	}

	req := buildConfigQueryRequest(chart, doc.ChartType, dims, metrics, doc.Query.Filters, resolveV2DocSort(doc.Query.Sort, &doc), doc.Query.Limit)
	req.QueryOptions = normalizePersistedQueryOptions(doc.QueryOptions)
	return req, true
}

// normalizePersistedQueryOptions 把持久化 config 文档 queryOptions 小节的键归一为
// wire snake_case（bin_count/bin_width），供 executor 的 histogramBinOptions 统一
// 读取：前端 store/文档惯例是 camelCase（binCount/binWidth，见 chartConfigSchema.ts
// 的 camelCase 文档键），3-1b 无论按哪种风格保存都能 round-trip；已是 snake_case
// 的键原样透传，未知键保留（扩展袋语义）。空/缺失小节返回 nil（请求不带
// query_options，executor 回落 bin_count=20 默认）。
func normalizePersistedQueryOptions(opts map[string]any) map[string]any {
	if len(opts) == 0 {
		return nil
	}
	out := make(map[string]any, len(opts))
	for k, v := range opts {
		switch k {
		case "binCount":
			k = "bin_count"
		case "binWidth":
			k = "bin_width"
		}
		out[k] = v
	}
	return out
}

// resolveV2DocSort 把 v2 持久化文档的 sort 小节翻译为平铺请求可用的 entity.SortConfig：
//   - bindingId 键（Task 1-7 起前端保存的形状）：先查指标组再查维度组，命中后取该绑定的
//     输出列名（指标 = fieldMeta[bindingId].alias 或列名，与 chartDataQueryFromConfigV2
//     构造 metrics 的 alias 逻辑对称；维度 = 列名）——平铺 v1 请求的 AST 不携带
//     BindingID，后端 resolveSortAlias 无从反查，必须还原为输出列名；
//   - 悬挂的 bindingId（绑定已被移除）：丢弃 sort（返回 nil），避免后端把无法解析的
//     引用渲染成 _invalid_identifier 导致 SQL 报错；
//   - field 键（Task 0-3~1-7 窗口期保存的文档）：原样透传，行为与改动前一致。
func resolveV2DocSort(sort *chartConfigSort, doc *chartConfigV2) *entity.SortConfig {
	if sort == nil {
		return nil
	}
	if sort.BindingID == "" {
		return &entity.SortConfig{Field: sort.Field, Order: sort.Order}
	}
	for _, group := range doc.Query.MetricGroups {
		for _, b := range group.Bindings {
			if b.BindingID != sort.BindingID {
				continue
			}
			field := b.Field
			if meta, ok := doc.FieldMeta[sort.BindingID]; ok && meta.Alias != "" {
				field = meta.Alias
			}
			return &entity.SortConfig{Field: field, Order: sort.Order}
		}
	}
	for _, group := range doc.Query.DimensionGroups {
		for _, b := range group.Bindings {
			if b.BindingID == sort.BindingID {
				return &entity.SortConfig{Field: b.Field, Order: sort.Order}
			}
		}
	}
	return nil
}

// buildConfigQueryRequest 组装持久化 config 解析出的平铺请求（v1/v2 共用尾段）：
// 过滤条件转换（valueEnd/value_end 双键兼容）、chartType 缺失时回退 chart 行的
// chart_type、limit>0 转为 page=1 的分页。
func buildConfigQueryRequest(
	chart *model.Chart,
	docChartType string,
	dims []string,
	metrics []entity.MetricConfig,
	rawFilters []chartConfigFilter,
	sort *entity.SortConfig,
	limit int,
) *entity.ChartQueryRequest {
	filters := make([]entity.Filter, 0, len(rawFilters))
	for _, f := range rawFilters {
		valueEnd := f.ValueEnd
		if valueEnd == nil {
			valueEnd = f.ValueEndCC
		}
		filters = append(filters, entity.Filter{
			Field:    f.Field,
			Operator: f.Operator,
			Value:    f.Value,
			ValueEnd: valueEnd,
			Logic:    f.Logic,
		})
	}

	chartType := docChartType
	if chartType == "" {
		chartType = chart.ChartType
	}

	req := &entity.ChartQueryRequest{
		DatasetID: chart.DatasetID,
		ChartType: chartType,
		Dims:      dims,
		Metrics:   metrics,
		Filters:   filters,
		Sort:      sort,
	}
	if limit > 0 {
		req.Pagination = &entity.Pagination{Page: 1, PageSize: limit}
	}
	return req
}

// getPlannerSource 获取 QueryPlanner 生成 AST 所需的数据源。
// 调用场景：service 层在 executor 执行前先规划 QueryAST，复用与 executor 相同的基础查询来源判定。
func getPlannerSource(dataset *model.Dataset) string {
	if dataset.QueryType == "sql" && dataset.QuerySQL.Valid {
		return dataset.QuerySQL.String
	}
	if dataset.TableName.Valid {
		return dataset.TableName.String
	}
	return ""
}

// getPlannerSourceType 获取 QueryPlanner 生成 AST 所需的 source type。
// 调用场景：与 getPlannerSource 配套，保证 AST 规划与 executor 实际查询源一致。
func getPlannerSourceType(dataset *model.Dataset) query.SourceType {
	if dataset.QueryType == "sql" && dataset.QuerySQL.Valid {
		return query.SourceTypeSQL
	}
	return query.SourceTypeTable
}

// buildQuerySpecFromEntityRequest 将 service 层 entity 请求转换为 QuerySpec。
// 调用场景：chart 查询主链路统一先进入 Query 语义层，再通过兼容 adapter 下沉到旧 executor。
func buildQuerySpecFromEntityRequest(req *entity.ChartQueryRequest) *query.QuerySpec {
	queryReq := &query.ChartQueryRequest{
		DatasetID:  req.DatasetID,
		ChartType:  query.ChartType(req.ChartType),
		Dims:       req.Dims,
		Metrics:    convertMetrics(req.Metrics),
		Filters:    convertFilters(req.Filters),
		Pagination: convertPagination(req.Pagination),
		Sort:       convertSort(req.Sort),
	}

	return query.QuerySpecFromRequest(queryReq)
}

// buildQuerySpecFromEntityRequestV2 将 v2 协议（spec_version=2）请求转换为 QuerySpec。
// 调用场景：executeQueryOnConn 的 v2 分支。与 v1 路径不同，维度/指标来自显式槽位组，
// GroupName/BindingID 保留在 DimensionExpr/MetricExpr2 上（由 PlanAST 传入 AST）；
// ChartSpec 不承载 filters/sort/pagination，这里从请求补齐（与 v1 转换器口径一致）。
func buildQuerySpecFromEntityRequestV2(req *entity.ChartQueryRequest) *query.QuerySpec {
	chartSpec := query.ChartSpecFromRequestV2(req)
	querySpec := query.QuerySpecFromChartSpecV2(chartSpec)
	querySpec.Filters = convertFilters(req.Filters)
	querySpec.Sort = convertSort(req.Sort)
	querySpec.Pagination = convertPagination(req.Pagination)
	return querySpec
}

// Helper functions

func (s *chartService) getChartModel(ctx context.Context, id int) (*model.Chart, error) {
	chart := &model.Chart{ID: id}
	if err := s.db.NewSelect().Model(chart).WherePK().Where("deleted_at IS NULL").Scan(ctx); err != nil {
		return nil, fmt.Errorf("chart not found: %w", err)
	}
	return chart, nil
}

func (s *chartService) getDatasetModel(ctx context.Context, id int) (*model.Dataset, error) {
	ds := &model.Dataset{ID: id}
	if err := s.db.NewSelect().Model(ds).WherePK().Where("deleted_at IS NULL").Scan(ctx); err != nil {
		return nil, fmt.Errorf("dataset not found: %w", err)
	}
	return ds, nil
}

func (s *chartService) getDatasourceModel(ctx context.Context, id int) (*model.Datasource, error) {
	ds := &model.Datasource{ID: id}
	if err := s.db.NewSelect().Model(ds).WherePK().Where("deleted_at IS NULL").Scan(ctx); err != nil {
		return nil, fmt.Errorf("datasource not found: %w", err)
	}
	return ds, nil
}

// connect resolves the stored password (decrypt / legacy auto-upgrade) before
// dialing, so encrypted credentials work on every chart/dataset query path.
func (s *chartService) connect(ctx context.Context, ds *model.Datasource) (datasource.Connection, error) {
	password, err := dsservice.ResolvePassword(ctx, s.db, ds, s.key)
	if err != nil {
		return nil, err
	}
	return s.dialFn(ctx, ds, password)
}

// SetSecurityKey injects the AES key; nil/empty disables decryption.
func (s *chartService) SetSecurityKey(key []byte) {
	s.key = key
}

func (s *chartService) dial(ctx context.Context, ds *model.Datasource, password string) (datasource.Connection, error) {
	driver, err := datasource.NewDriver(datasource.DriverType(ds.Type))
	if err != nil {
		return nil, fmt.Errorf("unsupported driver type: %s", ds.Type)
	}

	config := datasource.ConnectionConfig{
		Host:         ds.Host,
		Port:         ds.Port,
		DatabaseName: ds.DatabaseName,
		Username:     ds.Username,
		Password:     password,
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := driver.Connect(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	return conn, nil
}

// Conversion functions

func toChartEntity(m *model.Chart) *entity.Chart {
	e := &entity.Chart{
		ID:        m.ID,
		Name:      m.Name,
		DatasetID: m.DatasetID,
		ChartType: m.ChartType,
		Config:    m.Config,
	}
	if m.CreatedAt.Valid {
		e.CreatedAt = m.CreatedAt.Time.Format(time.RFC3339)
	}
	if m.UpdatedAt.Valid {
		e.UpdatedAt = m.UpdatedAt.Time.Format(time.RFC3339)
	}
	return e
}

func toChartEntityList(models []model.Chart) []entity.Chart {
	result := make([]entity.Chart, len(models))
	for i, m := range models {
		e := toChartEntity(&m)
		result[i] = *e
	}
	return result
}

func toChartModel(e *entity.Chart) *model.Chart {
	m := &model.Chart{
		ID:        e.ID,
		Name:      e.Name,
		DatasetID: e.DatasetID,
		ChartType: e.ChartType,
		Config:    e.Config,
	}
	if e.Config == "" {
		m.Config = "{}"
	}
	return m
}

// convertMetrics converts entity metrics to query metrics
func convertMetrics(metrics []entity.MetricConfig) []query.MetricConfig {
	result := make([]query.MetricConfig, len(metrics))
	for i, m := range metrics {
		result[i] = query.MetricConfig{
			Field: m.Field,
			Agg:   query.AggregationType(m.Agg),
			Alias: m.Alias,
		}
	}
	return result
}

// validateFilterFields 拒绝字段为空的过滤条件：空字段会生成 _invalid_identifier
// 占位符导致 SQL 执行失败（静默 500），这里在查询前提前转为 400。
func validateFilterFields(filters []entity.Filter) error {
	for _, f := range filters {
		if strings.TrimSpace(f.Field) == "" {
			return router.NewBusinessError(response.CodeBadRequest, "filter field is required")
		}
	}
	return nil
}

// convertFilters converts entity filters to query filters
func convertFilters(filters []entity.Filter) []query.FilterConfig {
	result := make([]query.FilterConfig, len(filters))
	for i, f := range filters {
		result[i] = query.FilterConfig{
			Field:    f.Field,
			Op:       query.FilterOperator(f.Operator),
			Value:    f.Value,
			ValueEnd: f.ValueEnd,
			Logic:    f.Logic,
		}
	}
	return result
}

// convertPagination converts entity pagination to query pagination
func convertPagination(p *entity.Pagination) *query.Pagination {
	if p == nil {
		return nil
	}
	return &query.Pagination{
		Page:     p.Page,
		PageSize: p.PageSize,
	}
}

// convertSort converts entity sort to query sort
func convertSort(s *entity.SortConfig) *query.SortConfig {
	if s == nil {
		return nil
	}
	return &query.SortConfig{
		Field: s.Field,
		Order: s.Order,
	}
}
