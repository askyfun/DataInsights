package dataset

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"data-insights/internal/database"
	"data-insights/internal/datasource"
	"data-insights/internal/domain/entity"
	"data-insights/internal/idgen"
	"data-insights/internal/model"
	"data-insights/internal/query"
	dsservice "data-insights/internal/service/datasource"

	"github.com/uptrace/bun"
)

// Service defines the interface for dataset operations
type Service interface {
	// CRUD operations
	List(ctx context.Context, limit, offset int) ([]entity.Dataset, error)
	GetByID(ctx context.Context, id int) (*entity.Dataset, error)
	Create(ctx context.Context, ds *entity.Dataset) (*entity.Dataset, error)
	Update(ctx context.Context, ds *entity.Dataset) (*entity.Dataset, error)
	Delete(ctx context.Context, id int) error

	// Column operations
	GetColumns(ctx context.Context, id int) ([]entity.DatasetColumn, error)
	UpdateColumns(ctx context.Context, id int, columns []entity.DatasetColumn) (*entity.Dataset, error)

	// Data operations
	Preview(ctx context.Context, id int) (*entity.PreviewResult, error)
	Query(ctx context.Context, id int, config entity.QueryConfig) ([]map[string]any, error)

	// SetSecurityKey injects the 32-byte AES key used to decrypt datasource
	// passwords at rest. A nil key keeps plaintext passthrough (dev mode).
	SetSecurityKey(key []byte)
}

// datasetService implements the Service interface
type datasetService struct {
	db                   *bun.DB
	key                  []byte // 32-byte AES key; nil => plaintext passthrough
	connectFn            func(ctx context.Context, ds *model.Datasource) (datasource.Connection, error)
	dialFn               func(ctx context.Context, ds *model.Datasource, password string) (datasource.Connection, error)
	getDatasetModelFn    func(ctx context.Context, id int) (*model.Dataset, error)
	getDatasourceModelFn func(ctx context.Context, id int) (*model.Datasource, error)
}

// NewService creates a new dataset service
func NewService(db *bun.DB) Service {
	service := &datasetService{db: db}
	service.connectFn = service.connect
	service.dialFn = service.dial
	service.getDatasetModelFn = service.getDatasetModel
	service.getDatasourceModelFn = service.getDatasourceModel
	return service
}

// List returns all datasets with pagination
func (s *datasetService) List(ctx context.Context, limit, offset int) ([]entity.Dataset, error) {
	var datasets []model.Dataset
	// 显式 id 倒序：不写 ORDER BY 时 PostgreSQL 返回的是堆物理序，UPDATE 会写新行
	// 版本追加到堆尾，编辑过的数据集会漂到列表后面（表现为顺序"随机"）。id 倒序让
	// 新建/导入的靠前，且天然稳定。对齐 dashboard 服务的显式排序决策。
	q := s.db.NewSelect().Model(&datasets).Where("deleted_at IS NULL").OrderExpr("id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list datasets: %w", err)
	}
	return toDatasetEntityList(datasets), nil
}

// GetByID returns a dataset by ID
func (s *datasetService) GetByID(ctx context.Context, id int) (*entity.Dataset, error) {
	ds := &model.Dataset{ID: id}
	if err := s.db.NewSelect().Model(ds).WherePK().Where("deleted_at IS NULL").Scan(ctx); err != nil {
		return nil, fmt.Errorf("dataset not found: %w", err)
	}
	return toDatasetEntity(ds), nil
}

// Create creates a new dataset
func (s *datasetService) Create(ctx context.Context, ds *entity.Dataset) (*entity.Dataset, error) {
	m := toDatasetModel(ds)
	m.CreatedAt = sql.NullTime{Time: time.Now(), Valid: true}
	// bun 对零值 sql.NullTime 发显式 NULL（绕过列 DEFAULT CURRENT_TIMESTAMP），
	// 故 updated_at 需与 created_at 一样在插入时显式打戳，否则新建行 updated_at 为空。
	m.UpdatedAt = sql.NullTime{Time: time.Now(), Valid: true}
	if _, err := s.db.NewInsert().Model(m).Returning("*").Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create dataset: %w", err)
	}
	return toDatasetEntity(m), nil
}

// Update updates an existing dataset
func (s *datasetService) Update(ctx context.Context, ds *entity.Dataset) (*entity.Dataset, error) {
	m := toDatasetModel(ds)
	// 整行 WherePK 更新会把合并基带入的旧 updated_at 原样写回，导致更新后时间戳
	// 不前进（DB 无触发器兜底）。显式打当前时间，让 bun 的整行更新写入新值；
	// created_at 仍走 toDatasetModel 的透传（merge 负责保留）。
	m.UpdatedAt = sql.NullTime{Time: time.Now(), Valid: true}
	if _, err := s.db.NewUpdate().Model(m).WherePK().Where("deleted_at IS NULL").ExcludeColumn("deleted_at").Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to update dataset: %w", err)
	}
	updated := &model.Dataset{ID: ds.ID}
	if err := s.db.NewSelect().Model(updated).WherePK().Where("deleted_at IS NULL").Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to get updated dataset: %w", err)
	}
	return toDatasetEntity(updated), nil
}

// Delete soft-deletes a dataset by ID and cascades the soft delete to all of its
// charts, then to every share under those charts — within a single transaction.
// The row is never physically removed (deleted_at is stamped instead).
func (s *datasetService) Delete(ctx context.Context, id int) error {
	return database.WithTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		// 软删数据集自身（幂等：已删除/不存在影响 0 行不报错）。
		if _, err := tx.NewUpdate().
			Model((*model.Dataset)(nil)).
			Set("deleted_at = now()").
			Where("id = ?", id).
			Where("deleted_at IS NULL").
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to delete dataset: %w", err)
		}
		// 级联软删其下图表。
		if _, err := tx.NewUpdate().
			Model((*model.Chart)(nil)).
			Set("deleted_at = now()").
			Where("dataset_id = ?", id).
			Where("deleted_at IS NULL").
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to cascade delete charts: %w", err)
		}
		// 级联软删这些图表下分享。
		// ⚠️ 子查询刻意不带 deleted_at IS NULL：父行刚在本事务软删，子查询加过滤会断链。
		// 行筛选只靠外层的 IS NULL——勿"顺手"给子查询补过滤。
		if _, err := tx.NewUpdate().
			Model((*model.Share)(nil)).
			Set("deleted_at = now()").
			Where("chart_id IN (SELECT id FROM bi_chart WHERE dataset_id = ?)", id).
			Where("deleted_at IS NULL").
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to cascade delete shares: %w", err)
		}
		return nil
	})
}

// GetColumns returns columns for a dataset
func (s *datasetService) GetColumns(ctx context.Context, id int) ([]entity.DatasetColumn, error) {
	ds, err := s.getDatasetModelFn(ctx, id)
	if err != nil {
		return nil, err
	}

	// If columns are already saved, return them（历史 number/json/unknown 词在此归一）
	if ds.Columns != "" && ds.Columns != "[]" {
		var savedColumns []entity.DatasetColumn
		if err := json.Unmarshal([]byte(ds.Columns), &savedColumns); err == nil && len(savedColumns) > 0 {
			for i := range savedColumns {
				savedColumns[i].Type = string(model.NormalizeStandardType(savedColumns[i].Type))
			}
			// 历史数据（本特性之前保存的列）没有 ID：补齐并回写，否则图表配置
			// 与 shard_keys 没有可引用的稳定标识。
			if assignColumnIDs(savedColumns) {
				if err := s.persistColumns(ctx, ds, savedColumns); err != nil {
					return nil, err
				}
			}
			return savedColumns, nil
		}
	}

	// Fetch columns from datasource
	dsModel, err := s.getDatasourceModelFn(ctx, ds.DatasourceID)
	if err != nil {
		return nil, err
	}

	conn, err := s.connectFn(ctx, dsModel)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var dbColumns []datasource.ColumnInfo
	if ds.QueryType == "sql" && ds.QuerySQL.Valid {
		// SQL 型数据集：querySQL 不能作为表名传给 GetColumns（会被驱动
		// 标识符校验拒绝，且本就是注入面）。改经 query 包单一通道执行一次
		// 包装后的查询，从结果列名推导列，所有驱动行为一致。
		previewSQL := query.WrapPreviewSQL(ds.QuerySQL.String, query.SourceTypeSQL, 1)
		result, err := conn.Execute(ctx, previewSQL)
		if err != nil {
			return nil, fmt.Errorf("failed to query: %w", err)
		}
		for _, name := range result.Columns {
			dbColumns = append(dbColumns, datasource.ColumnInfo{Name: name})
		}
	} else if ds.TableName.Valid {
		dbColumns, err = conn.GetColumns(ctx, ds.TableName.String)
		if err != nil {
			return nil, fmt.Errorf("failed to get columns: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no table or query defined")
	}

	// 列 ID 是图表配置 / shard_keys 的引用键，没有落库就没有稳定 ID 可引用，
	// 因此首次读取即分配 ID 并物化（幂等）。物化后列集合以落库为准——与用户
	// 手动保存过一次列定义后的行为一致。
	columns := mapDatasetColumns(dbColumns, dsModel.Type)
	assignColumnIDs(columns)
	if err := s.persistColumns(ctx, ds, columns); err != nil {
		return nil, err
	}
	return columns, nil
}

// persistColumns 只写 bi_dataset.columns 一列：GET 路径上的物化不应顺手改动
// updated_at（那是用户显式保存的语义）。
func (s *datasetService) persistColumns(ctx context.Context, ds *model.Dataset, columns []entity.DatasetColumn) error {
	columnsJSON, err := json.Marshal(columns)
	if err != nil {
		return fmt.Errorf("failed to marshal columns: %w", err)
	}
	ds.Columns = string(columnsJSON)

	if _, err := s.db.NewUpdate().Model(ds).Column("columns").WherePK().Where("deleted_at IS NULL").Exec(ctx); err != nil {
		return fmt.Errorf("failed to persist columns: %w", err)
	}
	return nil
}

// assignColumnIDs 给缺 ID 或 ID 重复的列补发稳定 ID（幂等），返回是否有改动。
// 已有 ID 一律原样保留——列 ID 落地后被图表配置引用，换 ID 等于断链。
// 列集合不存在"增量合并"（只要有落库就以落库为准），故无需跨请求的 ID 复用逻辑。
func assignColumnIDs(columns []entity.DatasetColumn) bool {
	used := make(map[string]struct{}, len(columns))
	changed := false
	for i := range columns {
		if id := columns[i].ID; id != "" {
			if _, dup := used[id]; !dup {
				used[id] = struct{}{}
				continue
			}
		}
		columns[i].ID = newColumnID(used)
		used[columns[i].ID] = struct{}{}
		changed = true
	}
	return changed
}

// newColumnID 生成一个未被 used 占用的短 ID。idgen 保证进程内唯一，跨重启靠随机
// 起始偏移错开；这里的 used 检查兜住极小概率的启动偏移撞车。
func newColumnID(used map[string]struct{}) string {
	for {
		id := idgen.New()
		if _, dup := used[id]; !dup {
			return id
		}
	}
}

// mapDatasetColumns converts driver columns to entity columns with inferred
// roles. The type mapper is chosen from the datasource's own driver type so
// PG/MySQL information_schema type names (integer/numeric/text) map to
// standard types instead of falling through to "unknown"; a driver type the
// factory does not know falls back to the StarRocks mapper (previous
// behavior).
func mapDatasetColumns(dbColumns []datasource.ColumnInfo, dsType string) []entity.DatasetColumn {
	mapper, err := model.NewDataTypeMapper(dsType)
	if err != nil {
		mapper, _ = model.NewDataTypeMapper("starrocks")
	}
	result := make([]entity.DatasetColumn, len(dbColumns))
	for i, col := range dbColumns {
		stdType, typeConfig, _ := mapper.ToStandard(col.Type)
		// 统一归一为规范词表（float/integer/boolean/string/date/datetime/array/map），
		// 失配/unknown 折叠 string，兼容历史 number/json 词。
		stdType = model.NormalizeStandardType(string(stdType))
		result[i] = entity.DatasetColumn{
			Name: col.Name,
			// 裸标识符：方言引号由查询层（safeIdentifier/方言 builder）负责，
			// 这里带反引号会在 PostgreSQL 下原样渲染导致语法错误。
			Expr:       col.Name,
			Type:       string(stdType),
			TypeConfig: entity.TypeConfig{Precision: typeConfig.Precision, Scale: typeConfig.Scale},
			Comment:    "",
			Role:       inferRole(string(stdType)),
		}
	}
	return result
}

// UpdateColumns updates columns for a dataset
func (s *datasetService) UpdateColumns(ctx context.Context, id int, columns []entity.DatasetColumn) (*entity.Dataset, error) {
	ds, err := s.getDatasetModel(ctx, id)
	if err != nil {
		return nil, err
	}

	// 前端在新建虚拟字段时不会带 ID；这里补发，保证落库的列恒有稳定标识。
	assignColumnIDs(columns)

	columnsJSON, err := json.Marshal(columns)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal columns: %w", err)
	}
	ds.Columns = string(columnsJSON)

	if _, err := s.db.NewUpdate().Model(ds).WherePK().Where("deleted_at IS NULL").ExcludeColumn("deleted_at").Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to update columns: %w", err)
	}

	return toDatasetEntity(ds), nil
}

// Preview returns preview data for a dataset
func (s *datasetService) Preview(ctx context.Context, id int) (*entity.PreviewResult, error) {
	ds, err := s.getDatasetModel(ctx, id)
	if err != nil {
		return nil, err
	}

	dsModel, err := s.getDatasourceModel(ctx, ds.DatasourceID)
	if err != nil {
		return nil, err
	}

	conn, err := s.connect(ctx, dsModel)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Build preview SQL via the query package: sql branch wraps the user SQL
	// in a subquery, table branch sanitizes the identifier.
	var source string
	sourceType := query.SourceTypeTable
	if ds.QueryType == "sql" && ds.QuerySQL.Valid {
		source = ds.QuerySQL.String
		sourceType = query.SourceTypeSQL
	} else if ds.TableName.Valid {
		source = ds.TableName.String
	} else {
		return nil, fmt.Errorf("no table or query defined")
	}

	sql := query.WrapPreviewSQL(source, sourceType, 10)
	result, err := conn.Execute(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}

	return &entity.PreviewResult{
		Columns: result.Columns,
		Data:    result.Rows,
	}, nil
}

// Query executes a query on a dataset.
//
// 只服务「维度去重取数」：SQL 由 query.BuildDatasetDistinctQuery 按 config 构造
// （dimension_groups 决定 SELECT/GROUP BY，filters/sort/limit 照常生效，列引用按列 ID
// 解析）。此前这里完全忽略 config，只发 `SELECT * FROM <source> LIMIT n`，把整行全列
// 回传再由前端本地去重（issue #110）。
func (s *datasetService) Query(ctx context.Context, id int, config entity.QueryConfig) ([]map[string]any, error) {
	ds, err := s.getDatasetModelFn(ctx, id)
	if err != nil {
		return nil, err
	}

	dsModel, err := s.getDatasourceModelFn(ctx, ds.DatasourceID)
	if err != nil {
		return nil, err
	}

	conn, err := s.connectFn(ctx, dsModel)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var source string
	sourceType := query.SourceTypeTable
	if ds.QueryType == "sql" && ds.QuerySQL.Valid {
		source = ds.QuerySQL.String
		sourceType = query.SourceTypeSQL
	} else if ds.TableName.Valid {
		source = ds.TableName.String
	}

	sql, args, err := query.BuildDatasetDistinctQuery(
		query.ParseDialect(dsModel.Type),
		ds.Columns,
		source,
		sourceType,
		config,
	)
	if err != nil {
		return nil, err
	}

	result, err := conn.Execute(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	return result.Rows, nil
}

// Helper functions

func (s *datasetService) getDatasetModel(ctx context.Context, id int) (*model.Dataset, error) {
	ds := &model.Dataset{ID: id}
	if err := s.db.NewSelect().Model(ds).WherePK().Where("deleted_at IS NULL").Scan(ctx); err != nil {
		return nil, fmt.Errorf("dataset not found: %w", err)
	}
	return ds, nil
}

func (s *datasetService) getDatasourceModel(ctx context.Context, id int) (*model.Datasource, error) {
	ds := &model.Datasource{ID: id}
	if err := s.db.NewSelect().Model(ds).WherePK().Where("deleted_at IS NULL").Scan(ctx); err != nil {
		return nil, fmt.Errorf("datasource not found: %w", err)
	}
	return ds, nil
}

// connect resolves the stored password (decrypt / legacy auto-upgrade) before
// dialing, so encrypted credentials work on every chart/dataset query path.
func (s *datasetService) connect(ctx context.Context, ds *model.Datasource) (datasource.Connection, error) {
	password, err := dsservice.ResolvePassword(ctx, s.db, ds, s.key)
	if err != nil {
		return nil, err
	}
	return s.dialFn(ctx, ds, password)
}

// SetSecurityKey injects the AES key; nil/empty disables decryption.
func (s *datasetService) SetSecurityKey(key []byte) {
	s.key = key
}

func (s *datasetService) dial(ctx context.Context, ds *model.Datasource, password string) (datasource.Connection, error) {
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

func toDatasetEntity(m *model.Dataset) *entity.Dataset {
	e := &entity.Dataset{
		ID:           m.ID,
		Name:         m.Name,
		DatasourceID: m.DatasourceID,
		QueryType:    m.QueryType,
		Mode:         m.Mode,
		Tags:         m.Tags,
		QualityRules: m.QualityRules,
		Columns:      m.Columns,
		ShardEnabled: m.ShardEnabled,
		ShardKeys:    m.ShardKeys,
	}
	if m.TableName.Valid {
		e.TableName = &m.TableName.String
	}
	if m.QuerySQL.Valid {
		e.QuerySQL = &m.QuerySQL.String
	}
	if m.AccelerateConfig.Valid {
		e.AccelerateConfig = &m.AccelerateConfig.String
	}
	if m.Description.Valid {
		e.Description = &m.Description.String
	}
	if m.RefreshStrategy.Valid {
		e.RefreshStrategy = &m.RefreshStrategy.String
	}
	if m.PreviewData.Valid {
		e.PreviewData = &m.PreviewData.String
	}
	if m.CreatedAt.Valid {
		e.CreatedAt = m.CreatedAt.Time.Format(time.RFC3339)
	}
	if m.UpdatedAt.Valid {
		e.UpdatedAt = m.UpdatedAt.Time.Format(time.RFC3339)
	}
	return e
}

func toDatasetEntityList(models []model.Dataset) []entity.Dataset {
	result := make([]entity.Dataset, len(models))
	for i, m := range models {
		e := toDatasetEntity(&m)
		result[i] = *e
	}
	return result
}

func toDatasetModel(e *entity.Dataset) *model.Dataset {
	m := &model.Dataset{
		ID:           e.ID,
		Name:         e.Name,
		DatasourceID: e.DatasourceID,
		QueryType:    e.QueryType,
		Mode:         e.Mode,
		Tags:         e.Tags,
		QualityRules: e.QualityRules,
		Columns:      e.Columns,
		ShardEnabled: e.ShardEnabled,
		ShardKeys:    e.ShardKeys,
	}
	// JSONB 列不接受空字符串（PG 报 invalid input syntax），
	// 空 JSON 字段归一为空数组，与空集合序列化为 [] 的响应契约一致。
	for _, dst := range []*string{&m.Tags, &m.QualityRules, &m.Columns, &m.ShardKeys} {
		if strings.TrimSpace(*dst) == "" {
			*dst = "[]"
		}
	}
	if e.TableName != nil {
		m.TableName = sql.NullString{String: *e.TableName, Valid: true}
	}
	if e.QuerySQL != nil {
		m.QuerySQL = sql.NullString{String: *e.QuerySQL, Valid: true}
	}
	if e.AccelerateConfig != nil {
		m.AccelerateConfig = sql.NullString{String: *e.AccelerateConfig, Valid: strings.TrimSpace(*e.AccelerateConfig) != ""}
	}
	if e.Description != nil {
		m.Description = sql.NullString{String: *e.Description, Valid: true}
	}
	if e.RefreshStrategy != nil {
		m.RefreshStrategy = sql.NullString{String: *e.RefreshStrategy, Valid: strings.TrimSpace(*e.RefreshStrategy) != ""}
	}
	if e.PreviewData != nil {
		m.PreviewData = sql.NullString{String: *e.PreviewData, Valid: strings.TrimSpace(*e.PreviewData) != ""}
	}
	// 时间戳随实体透传（与 datasource 服务 toModel 同一映射）：Update 的
	// 合并基带入取回行的 created_at/updated_at，整行更新才能原样写回。
	if e.CreatedAt != "" {
		t, _ := time.Parse(time.RFC3339, e.CreatedAt)
		m.CreatedAt = sql.NullTime{Time: t, Valid: true}
	}
	if e.UpdatedAt != "" {
		t, _ := time.Parse(time.RFC3339, e.UpdatedAt)
		m.UpdatedAt = sql.NullTime{Time: t, Valid: true}
	}
	return m
}

func inferRole(dataType string) string {
	if model.StandardDataType(dataType).IsNumeric() {
		return "metric"
	}
	return "dimension"
}
