package chart

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"dataray/internal/datasource"
	"dataray/internal/domain/entity"
	"dataray/internal/model"
	"dataray/internal/query"
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
	q := s.db.NewSelect().Model(&charts)
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
	if err := s.db.NewSelect().Model(chart).WherePK().Scan(ctx); err != nil {
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
	if _, err := s.db.NewUpdate().Model(m).WherePK().Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to update chart: %w", err)
	}
	updated := &model.Chart{ID: chart.ID}
	if err := s.db.NewSelect().Model(updated).WherePK().Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to get updated chart: %w", err)
	}
	return toChartEntity(updated), nil
}

// Delete deletes a chart by ID
func (s *chartService) Delete(ctx context.Context, id int) error {
	if _, err := s.db.NewDelete().Model(&model.Chart{}).Where("id = ?", id).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete chart: %w", err)
	}
	return nil
}

// GetData returns data for a chart. A persisted v1 config is executed through
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
	executor := s.executorFactory(conn, dataset, dsModel)
	querySpec := buildQuerySpecFromEntityRequest(req)
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

// chartDataQueryFromConfig parses a persisted chart config into the same
// entity request the interactive builder sends (see ChartBuilder's
// composeChartQueryRequest): dims flatten all dimension-group fields, metrics
// flatten all metric-group fields with agg from fieldMeta (default "sum") and
// alias from fieldMeta (default = column name). Returns ok=false for legacy
// (queryConfig + positional ids), malformed, or empty-group configs.
func chartDataQueryFromConfig(chart *model.Chart) (*entity.ChartQueryRequest, bool) {
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

	filters := make([]entity.Filter, 0, len(doc.Query.Filters))
	for _, f := range doc.Query.Filters {
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

	chartType := doc.ChartType
	if chartType == "" {
		chartType = chart.ChartType
	}

	req := &entity.ChartQueryRequest{
		DatasetID: chart.DatasetID,
		ChartType: chartType,
		Dims:      dims,
		Metrics:   metrics,
		Filters:   filters,
		Sort:      doc.Query.Sort,
	}
	if doc.Query.Limit > 0 {
		req.Pagination = &entity.Pagination{Page: 1, PageSize: doc.Query.Limit}
	}
	return req, true
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

// Helper functions

func (s *chartService) getChartModel(ctx context.Context, id int) (*model.Chart, error) {
	chart := &model.Chart{ID: id}
	if err := s.db.NewSelect().Model(chart).WherePK().Scan(ctx); err != nil {
		return nil, fmt.Errorf("chart not found: %w", err)
	}
	return chart, nil
}

func (s *chartService) getDatasetModel(ctx context.Context, id int) (*model.Dataset, error) {
	ds := &model.Dataset{ID: id}
	if err := s.db.NewSelect().Model(ds).WherePK().Scan(ctx); err != nil {
		return nil, fmt.Errorf("dataset not found: %w", err)
	}
	return ds, nil
}

func (s *chartService) getDatasourceModel(ctx context.Context, id int) (*model.Datasource, error) {
	ds := &model.Datasource{ID: id}
	if err := s.db.NewSelect().Model(ds).WherePK().Scan(ctx); err != nil {
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
