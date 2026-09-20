package datasource

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"data-insights/internal/crypto"
	"data-insights/internal/database"
	"data-insights/internal/datasource"
	"data-insights/internal/domain/entity"
	"data-insights/internal/model"
	"data-insights/internal/query"

	"github.com/uptrace/bun"
)

// Service defines the interface for datasource operations
type Service interface {
	// CRUD operations
	List(ctx context.Context, limit, offset int) ([]entity.Datasource, error)
	GetByID(ctx context.Context, id int) (*entity.Datasource, error)
	Create(ctx context.Context, ds *entity.Datasource) (*entity.Datasource, error)
	Update(ctx context.Context, ds *entity.Datasource) (*entity.Datasource, error)
	Delete(ctx context.Context, id int) error

	// Connection testing
	TestConnection(ctx context.Context, config entity.DatasourceConnectionConfig, driverType string) error

	// Schema operations
	GetTables(ctx context.Context, id int) ([]entity.TableInfo, error)
	GetColumns(ctx context.Context, id int, tableName string) ([]entity.ColumnInfo, error)

	// Preview operations
	Preview(ctx context.Context, id int, tableName, querySQL, queryType string) (*entity.PreviewResult, error)
	GetFieldDistribution(ctx context.Context, id int, tableName, querySQL, queryType, fieldName string, limit int) (*entity.FieldDistribution, error)

	// Table data operations
	GetTableData(ctx context.Context, id int, tableName string, page, pageSize int, sortField, sortOrder string) (*entity.TableDataResult, error)

	// SetSecurityKey injects the 32-byte AES key used to encrypt datasource
	// passwords at rest. A nil key keeps plaintext passthrough (dev mode).
	SetSecurityKey(key []byte)
}

// datasourceService implements the Service interface
type datasourceService struct {
	db  *bun.DB
	key []byte // 32-byte AES key; nil => plaintext passthrough

	connectFn            func(ctx context.Context, ds *model.Datasource) (datasource.Connection, error)
	getDatasourceModelFn func(ctx context.Context, id int) (*model.Datasource, error)
}

// NewService creates a new datasource service
func NewService(db *bun.DB) Service {
	service := &datasourceService{db: db}
	service.connectFn = service.connect
	service.getDatasourceModelFn = service.getDatasourceModel
	return service
}

// SetSecurityKey injects the AES key; nil/empty disables encryption.
func (s *datasourceService) SetSecurityKey(key []byte) {
	s.key = key
}

// List returns all datasources with pagination
func (s *datasourceService) List(ctx context.Context, limit, offset int) ([]entity.Datasource, error) {
	var datasources []model.Datasource
	q := s.db.NewSelect().Model(&datasources).Where("deleted_at IS NULL")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list datasources: %w", err)
	}
	return toEntityList(datasources), nil
}

// GetByID returns a datasource by ID
func (s *datasourceService) GetByID(ctx context.Context, id int) (*entity.Datasource, error) {
	ds := &model.Datasource{ID: id}
	if err := s.db.NewSelect().Model(ds).WherePK().Where("deleted_at IS NULL").Scan(ctx); err != nil {
		return nil, fmt.Errorf("datasource not found: %w", err)
	}
	return toEntity(ds), nil
}

// Create creates a new datasource
func (s *datasourceService) Create(ctx context.Context, ds *entity.Datasource) (*entity.Datasource, error) {
	m := toModel(ds)
	if err := s.encryptPassword(m); err != nil {
		return nil, err
	}
	// bun 对零值 sql.NullTime 发显式 NULL（绕过列 DEFAULT CURRENT_TIMESTAMP），
	// 此前 Create 两个时间戳都为空。对齐 dataset.Create：插入时显式打戳。
	m.CreatedAt = sql.NullTime{Time: time.Now(), Valid: true}
	m.UpdatedAt = sql.NullTime{Time: time.Now(), Valid: true}
	if _, err := s.db.NewInsert().Model(m).Returning("*").Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create datasource: %w", err)
	}
	return toEntity(m), nil
}

// Update updates an existing datasource. An empty incoming password means the
// client did not send one (passwords are omitted from API responses), so the
// stored value is preserved verbatim instead of being wiped or re-encrypted.
func (s *datasourceService) Update(ctx context.Context, ds *entity.Datasource) (*entity.Datasource, error) {
	m := toModel(ds)
	if m.Password != "" {
		if err := s.encryptPassword(m); err != nil {
			return nil, err
		}
	} else {
		existing, err := s.getDatasourceModelFn(ctx, ds.ID)
		if err != nil {
			return nil, err
		}
		m.Password = existing.Password
	}
	// 整行 WherePK 更新此前不刷新 updated_at（DB 无触发器兜底）。显式打当前时间，
	// 让 bun 的整行更新写入新值，与 dataset 服务 Update 保持一致。
	m.UpdatedAt = sql.NullTime{Time: time.Now(), Valid: true}
	if _, err := s.db.NewUpdate().Model(m).WherePK().Where("deleted_at IS NULL").ExcludeColumn("deleted_at").Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to update datasource: %w", err)
	}
	updated := &model.Datasource{ID: ds.ID}
	if err := s.db.NewSelect().Model(updated).WherePK().Where("deleted_at IS NULL").Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to get updated datasource: %w", err)
	}
	return toEntity(updated), nil
}

// Delete soft-deletes a datasource by ID and cascades the soft delete to all of
// its datasets, then to every chart under those datasets, then to every share
// under those charts — all within a single transaction. The row stays in the
// table (deleted_at is stamped, never physically removed), matching the
// product requirement that nothing is ever hard-deleted.
func (s *datasourceService) Delete(ctx context.Context, id int) error {
	return database.WithTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		// 软删数据源自身（幂等：已删除/不存在影响 0 行不报错）。
		if _, err := tx.NewUpdate().
			Model((*model.Datasource)(nil)).
			Set("deleted_at = now()").
			Where("id = ?", id).
			Where("deleted_at IS NULL").
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to delete datasource: %w", err)
		}
		// 级联软删其下数据集。
		if _, err := tx.NewUpdate().
			Model((*model.Dataset)(nil)).
			Set("deleted_at = now()").
			Where("datasource_id = ?", id).
			Where("deleted_at IS NULL").
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to cascade delete datasets: %w", err)
		}
		// 级联软删这些数据集下图表。
		// ⚠️ 子查询刻意不带 deleted_at IS NULL：父行刚在本事务软删，子查询加过滤会断链。
		// 行筛选只靠外层的 IS NULL——勿"顺手"给子查询补过滤。
		if _, err := tx.NewUpdate().
			Model((*model.Chart)(nil)).
			Set("deleted_at = now()").
			Where("dataset_id IN (SELECT id FROM bi_dataset WHERE datasource_id = ?)", id).
			Where("deleted_at IS NULL").
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to cascade delete charts: %w", err)
		}
		// 级联软删这些图表下分享（子查询同样刻意不带 IS NULL，同上）。
		if _, err := tx.NewUpdate().
			Model((*model.Share)(nil)).
			Set("deleted_at = now()").
			Where("chart_id IN (SELECT id FROM bi_chart WHERE dataset_id IN (SELECT id FROM bi_dataset WHERE datasource_id = ?))", id).
			Where("deleted_at IS NULL").
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to cascade delete shares: %w", err)
		}
		return nil
	})
}

// TestConnection tests the connection to a datasource
func (s *datasourceService) TestConnection(ctx context.Context, config entity.DatasourceConnectionConfig, driverType string) error {
	driver, err := datasource.NewDriver(datasource.DriverType(driverType))
	if err != nil {
		return fmt.Errorf("unsupported driver type: %s", driverType)
	}

	connConfig := datasource.ConnectionConfig{
		Host:         config.Host,
		Port:         config.Port,
		DatabaseName: config.DatabaseName,
		Username:     config.Username,
		Password:     config.Password,
	}

	if err := driver.TestConnection(ctx, connConfig); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	return nil
}

// GetTables returns all tables from a datasource
func (s *datasourceService) GetTables(ctx context.Context, id int) ([]entity.TableInfo, error) {
	ds, err := s.getDatasourceModelFn(ctx, id)
	if err != nil {
		return nil, err
	}

	conn, err := s.connect(ctx, ds)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	tables, err := conn.GetTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}
	return toTableInfoList(tables), nil
}

// GetColumns returns all columns for a table
func (s *datasourceService) GetColumns(ctx context.Context, id int, tableName string) ([]entity.ColumnInfo, error) {
	ds, err := s.getDatasourceModelFn(ctx, id)
	if err != nil {
		return nil, err
	}

	conn, err := s.connect(ctx, ds)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	columns, err := conn.GetColumns(ctx, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}
	return toColumnInfoList(columns), nil
}

// Preview returns preview data from a datasource
func (s *datasourceService) Preview(ctx context.Context, id int, tableName, querySQL, queryType string) (*entity.PreviewResult, error) {
	source, sourceType, err := previewSource(tableName, querySQL, queryType)
	if err != nil {
		return nil, err
	}
	sql := query.WrapPreviewSQL(source, sourceType, 10)

	ds, err := s.getDatasourceModelFn(ctx, id)
	if err != nil {
		return nil, err
	}

	conn, err := s.connect(ctx, ds)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	result, err := conn.Execute(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}

	return &entity.PreviewResult{
		Columns: result.Columns,
		Data:    result.Rows,
	}, nil
}

// previewSource resolves the preview source. For the sql branch, querySQL is
// the product-allowed arbitrary user SQL (wrapped into a subquery by the query
// package). For the table branch, tableName must be a valid identifier before
// any SQL is built.
func previewSource(tableName, querySQL, queryType string) (string, query.SourceType, error) {
	if queryType == "sql" {
		return querySQL, query.SourceTypeSQL, nil
	}
	if !datasource.IsValidIdentifier(tableName) {
		return "", query.SourceTypeTable, fmt.Errorf("invalid table name: %q", tableName)
	}
	return tableName, query.SourceTypeTable, nil
}

// buildDistributionSQLs builds both the distribution and the total SQL from
// the resolved source, mirroring previewSource: for SQL-type datasets the
// product-allowed querySQL is the source (wrapped in a subquery), for table
// datasets the validated tableName is used. Both SQLs are built up front so
// identifier validation happens before any DB access.
func buildDistributionSQLs(fieldName, tableName, querySQL, queryType string, limit int) (distSQL, totalSQL string, err error) {
	source, sourceType, err := previewSource(tableName, querySQL, queryType)
	if err != nil {
		return "", "", err
	}
	distSQL, err = query.BuildFieldDistributionSQL(fieldName, source, sourceType, limit)
	if err != nil {
		return "", "", err
	}
	return distSQL, query.WrapCountSQL(source, sourceType), nil
}

// GetFieldDistribution returns field value distribution
func (s *datasourceService) GetFieldDistribution(ctx context.Context, id int, tableName, querySQL, queryType, fieldName string, limit int) (*entity.FieldDistribution, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	distSQL, totalSQL, err := buildDistributionSQLs(fieldName, tableName, querySQL, queryType, limit)
	if err != nil {
		return nil, err
	}

	ds, err := s.getDatasourceModelFn(ctx, id)
	if err != nil {
		return nil, err
	}

	conn, err := s.connect(ctx, ds)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	result, err := conn.Execute(ctx, distSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}

	// Get total count
	var totalCount int64
	totalResult, err := conn.Execute(ctx, totalSQL)
	if err == nil && len(totalResult.Rows) > 0 {
		if val, ok := totalResult.Rows[0]["_total"]; ok {
			switch v := val.(type) {
			case int64:
				totalCount = v
			case float64:
				totalCount = int64(v)
			}
		}
	}

	// Format distribution
	distribution := make([]entity.FieldValueCount, 0, len(result.Rows))
	for _, row := range result.Rows {
		value := row[fieldName]
		count := row["_count"]

		var countInt int64
		switch v := count.(type) {
		case int64:
			countInt = v
		case float64:
			countInt = int64(v)
		}

		var percentage float64
		if totalCount > 0 {
			percentage = math.Round(float64(countInt)*10000/float64(totalCount)) / 100
		}

		distribution = append(distribution, entity.FieldValueCount{
			Value:      value,
			Count:      countInt,
			Percentage: percentage,
		})
	}

	return &entity.FieldDistribution{
		FieldName:    fieldName,
		TotalCount:   totalCount,
		UniqueCount:  len(result.Rows),
		Distribution: distribution,
	}, nil
}

// Helper functions

func (s *datasourceService) getDatasourceModel(ctx context.Context, id int) (*model.Datasource, error) {
	ds := &model.Datasource{ID: id}
	if err := s.db.NewSelect().Model(ds).WherePK().Where("deleted_at IS NULL").Scan(ctx); err != nil {
		return nil, fmt.Errorf("datasource not found: %w", err)
	}
	return ds, nil
}

func (s *datasourceService) connect(ctx context.Context, ds *model.Datasource) (datasource.Connection, error) {
	driver, err := datasource.NewDriver(datasource.DriverType(ds.Type))
	if err != nil {
		return nil, fmt.Errorf("unsupported driver type: %s", ds.Type)
	}

	password, err := s.resolvePassword(ctx, ds)
	if err != nil {
		return nil, err
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

// encryptPassword encrypts the model password before it is written to the
// database. Without a security key (or with an empty password) it is a no-op.
func (s *datasourceService) encryptPassword(m *model.Datasource) error {
	if s.key == nil || m.Password == "" {
		return nil
	}
	ct, err := crypto.Encrypt(s.key, m.Password)
	if err != nil {
		return fmt.Errorf("encrypt datasource password: %w", err)
	}
	m.Password = ct
	return nil
}

// resolvePassword delegates to the shared ResolvePassword so every consumer of
// a stored datasource password decrypts it exactly once.
func (s *datasourceService) resolvePassword(ctx context.Context, ds *model.Datasource) (string, error) {
	return ResolvePassword(ctx, s.db, ds, s.key)
}

// GetTableData returns paginated table data with primary key sorting
func (s *datasourceService) GetTableData(ctx context.Context, id int, tableName string, page, pageSize int, sortField, sortOrder string) (*entity.TableDataResult, error) {
	// Validate and apply defaults for pagination
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// Validate table name to prevent SQL injection (fail fast before DB access)
	if !datasource.IsValidIdentifier(tableName) {
		return nil, fmt.Errorf("invalid table name: %s", tableName)
	}

	// Sort direction is normalized inside query.BuildTableDataSQL.

	ds, err := s.getDatasourceModelFn(ctx, id)
	if err != nil {
		return nil, err
	}

	conn, err := s.connect(ctx, ds)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Get primary keys (graceful - tables without PKs should still be previewable)
	primaryKeys, _ := conn.GetPrimaryKeys(ctx, tableName)

	// Determine sort field: use provided sortField if valid, otherwise use first primary key
	effectiveSortField := ""
	if sortField != "" {
		if !datasource.IsValidIdentifier(sortField) {
			return nil, fmt.Errorf("invalid sort field: %s", sortField)
		}
		// sortField must be in primary keys to prevent full table scans
		if !containsString(primaryKeys, sortField) {
			return nil, fmt.Errorf("sort field must be a primary key column: %s", sortField)
		}
		effectiveSortField = sortField
	} else if len(primaryKeys) > 0 {
		effectiveSortField = primaryKeys[0]
	}

	// Build data and count SQL via the query package (identifier validation
	// and sort direction normalization happen there).
	offset := (page - 1) * pageSize
	dataSQL, err := query.BuildTableDataSQL(tableName, effectiveSortField, sortOrder, pageSize, offset)
	if err != nil {
		return nil, err
	}

	// Query total count
	countSQL := query.WrapCountSQL(tableName, query.SourceTypeTable)
	countResult, err := conn.Execute(ctx, countSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to count rows: %w", err)
	}

	var total int64
	if len(countResult.Rows) > 0 {
		if val, ok := countResult.Rows[0]["_total"]; ok {
			switch v := val.(type) {
			case int64:
				total = v
			case float64:
				total = int64(v)
			}
		}
	}

	// Query data with pagination
	dataResult, err := conn.Execute(ctx, dataSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query data: %w", err)
	}

	return &entity.TableDataResult{
		Columns:     dataResult.Columns,
		Data:        dataResult.Rows,
		Total:       total,
		PrimaryKeys: primaryKeys,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}

// containsString checks if a string slice contains a value
func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// Conversion functions

func toEntity(m *model.Datasource) *entity.Datasource {
	e := &entity.Datasource{
		ID:           m.ID,
		Name:         m.Name,
		Type:         m.Type,
		Host:         m.Host,
		Port:         m.Port,
		DatabaseName: m.DatabaseName,
		Username:     m.Username,
		Password:     m.Password,
	}
	if m.CreatedAt.Valid {
		e.CreatedAt = m.CreatedAt.Time.Format(time.RFC3339)
	}
	if m.UpdatedAt.Valid {
		e.UpdatedAt = m.UpdatedAt.Time.Format(time.RFC3339)
	}
	return e
}

func toEntityList(models []model.Datasource) []entity.Datasource {
	result := make([]entity.Datasource, len(models))
	for i, m := range models {
		e := toEntity(&m)
		result[i] = *e
	}
	return result
}

func toModel(e *entity.Datasource) *model.Datasource {
	m := &model.Datasource{
		ID:           e.ID,
		Name:         e.Name,
		Type:         e.Type,
		Host:         e.Host,
		Port:         e.Port,
		DatabaseName: e.DatabaseName,
		Username:     e.Username,
		Password:     e.Password,
	}
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

func toTableInfoList(tables []datasource.TableInfo) []entity.TableInfo {
	result := make([]entity.TableInfo, len(tables))
	for i, t := range tables {
		result[i] = entity.TableInfo{Name: t.Name, Comment: t.Comment}
	}
	return result
}

func toColumnInfoList(columns []datasource.ColumnInfo) []entity.ColumnInfo {
	result := make([]entity.ColumnInfo, len(columns))
	for i, c := range columns {
		result[i] = entity.ColumnInfo{Name: c.Name, DataType: c.Type, Comment: c.Comment}
	}
	return result
}
