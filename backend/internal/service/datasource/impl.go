package datasource

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"regexp"
	"time"

	"dataray/internal/datasource"
	"dataray/internal/domain/entity"
	"dataray/internal/model"

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
}

// datasourceService implements the Service interface
type datasourceService struct {
	db *bun.DB
}

// NewService creates a new datasource service
func NewService(db *bun.DB) Service {
	return &datasourceService{db: db}
}

// List returns all datasources with pagination
func (s *datasourceService) List(ctx context.Context, limit, offset int) ([]entity.Datasource, error) {
	var datasources []model.Datasource
	q := s.db.NewSelect().Model(&datasources)
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
	if err := s.db.NewSelect().Model(ds).WherePK().Scan(ctx); err != nil {
		return nil, fmt.Errorf("datasource not found: %w", err)
	}
	return toEntity(ds), nil
}

// Create creates a new datasource
func (s *datasourceService) Create(ctx context.Context, ds *entity.Datasource) (*entity.Datasource, error) {
	m := toModel(ds)
	if _, err := s.db.NewInsert().Model(m).Returning("*").Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create datasource: %w", err)
	}
	return toEntity(m), nil
}

// Update updates an existing datasource
func (s *datasourceService) Update(ctx context.Context, ds *entity.Datasource) (*entity.Datasource, error) {
	m := toModel(ds)
	if _, err := s.db.NewUpdate().Model(m).WherePK().Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to update datasource: %w", err)
	}
	updated := &model.Datasource{ID: ds.ID}
	if err := s.db.NewSelect().Model(updated).WherePK().Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to get updated datasource: %w", err)
	}
	return toEntity(updated), nil
}

// Delete deletes a datasource by ID
func (s *datasourceService) Delete(ctx context.Context, id int) error {
	if _, err := s.db.NewDelete().Model(&model.Datasource{}).Where("id = ?", id).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete datasource: %w", err)
	}
	return nil
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
	ds, err := s.getDatasourceModel(ctx, id)
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
	ds, err := s.getDatasourceModel(ctx, id)
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

// buildPreviewSQL builds the preview SQL. For the sql branch, the user-provided
// querySQL is wrapped in a subquery instead of appending LIMIT directly, so user
// SQL containing its own LIMIT clause no longer produces a broken statement.
// Note: querySQL ending with a semicolon or a "--" line comment still yields a
// non-executable statement; the database will reject it and the error is
// returned as-is. For the table branch, tableName must be a valid identifier
// before interpolation.
func buildPreviewSQL(tableName, querySQL, queryType string) (string, error) {
	if queryType == "sql" {
		return fmt.Sprintf("SELECT * FROM (%s) AS _preview LIMIT 10", querySQL), nil
	}
	if !datasource.IsValidIdentifier(tableName) {
		return "", fmt.Errorf("invalid table name: %q", tableName)
	}
	return fmt.Sprintf("SELECT * FROM %s LIMIT 10", tableName), nil
}

// Preview returns preview data from a datasource
func (s *datasourceService) Preview(ctx context.Context, id int, tableName, querySQL, queryType string) (*entity.PreviewResult, error) {
	sql, err := buildPreviewSQL(tableName, querySQL, queryType)
	if err != nil {
		return nil, err
	}

	ds, err := s.getDatasourceModel(ctx, id)
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

// buildFieldDistributionSQL builds the field distribution SQL. fieldName is
// always validated as an identifier before interpolation. In the table branch,
// source (the table name) is validated too; in the sql branch, source is the
// user-provided querySQL, which the product intentionally allows to be
// arbitrary SQL, so it is not identifier-validated.
func buildFieldDistributionSQL(fieldName, source, queryType string, limit int) (string, error) {
	if !datasource.IsValidIdentifier(fieldName) {
		return "", fmt.Errorf("invalid field name: %q", fieldName)
	}
	if queryType == "sql" {
		return fmt.Sprintf("SELECT %s, COUNT(*) as _count FROM (%s) as _subquery GROUP BY %s ORDER BY _count DESC LIMIT %d",
			fieldName, source, fieldName, limit), nil
	}
	if !datasource.IsValidIdentifier(source) {
		return "", fmt.Errorf("invalid table name: %q", source)
	}
	return fmt.Sprintf("SELECT %s, COUNT(*) as _count FROM %s GROUP BY %s ORDER BY _count DESC LIMIT %d",
		fieldName, source, fieldName, limit), nil
}

// GetFieldDistribution returns field value distribution
func (s *datasourceService) GetFieldDistribution(ctx context.Context, id int, tableName, querySQL, queryType, fieldName string, limit int) (*entity.FieldDistribution, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	// Validate identifiers and build SQL before any DB access.
	sql, err := buildFieldDistributionSQL(fieldName, tableName, queryType, limit)
	if err != nil {
		return nil, err
	}

	ds, err := s.getDatasourceModel(ctx, id)
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

	// Get total count
	var totalQuery string
	if queryType == "sql" {
		totalQuery = fmt.Sprintf("SELECT COUNT(*) as _total FROM (%s) as _subquery", querySQL)
	} else {
		totalQuery = fmt.Sprintf("SELECT COUNT(*) as _total FROM %s", tableName)
	}

	var totalCount int64
	totalResult, err := conn.Execute(ctx, totalQuery)
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
	if err := s.db.NewSelect().Model(ds).WherePK().Scan(ctx); err != nil {
		return nil, fmt.Errorf("datasource not found: %w", err)
	}
	return ds, nil
}

func (s *datasourceService) connect(ctx context.Context, ds *model.Datasource) (datasource.Connection, error) {
	driver, err := datasource.NewDriver(datasource.DriverType(ds.Type))
	if err != nil {
		return nil, fmt.Errorf("unsupported driver type: %s", ds.Type)
	}

	config := datasource.ConnectionConfig{
		Host:         ds.Host,
		Port:         ds.Port,
		DatabaseName: ds.DatabaseName,
		Username:     ds.Username,
		Password:     ds.Password,
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := driver.Connect(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	return conn, nil
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

	// Validate table name to prevent SQL injection
	if !isValidSQLIdentifier(tableName) {
		return nil, fmt.Errorf("invalid table name: %s", tableName)
	}

	// Validate sortOrder
	sortOrder = normalizeSortOrder(sortOrder)

	ds, err := s.getDatasourceModel(ctx, id)
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
		if !isValidSQLIdentifier(sortField) {
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

	// Build ORDER BY clause
	orderByClause := ""
	if effectiveSortField != "" {
		orderByClause = fmt.Sprintf(" ORDER BY %s %s", effectiveSortField, sortOrder)
	}

	// Query total count
	countSQL := fmt.Sprintf("SELECT COUNT(*) AS _total FROM %s", tableName)
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
	offset := (page - 1) * pageSize
	dataSQL := fmt.Sprintf("SELECT * FROM %s%s LIMIT %d OFFSET %d", tableName, orderByClause, pageSize, offset)
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

// isValidSQLIdentifier validates that a SQL identifier contains only safe characters [a-zA-Z0-9_.]
func isValidSQLIdentifier(name string) bool {
	if name == "" {
		return false
	}
	re := regexp.MustCompile(`^[a-zA-Z0-9_.]+$`)
	return re.MatchString(name)
}

// normalizeSortOrder validates and normalizes sort order, defaults to ASC
func normalizeSortOrder(order string) string {
	if order == "" {
		return "ASC"
	}
	upper := ""
	for _, c := range order {
		if c >= 'a' && c <= 'z' {
			upper += string(c - 32)
		} else {
			upper += string(c)
		}
	}
	if upper == "DESC" {
		return "DESC"
	}
	return "ASC"
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
