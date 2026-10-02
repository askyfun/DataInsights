package dataset

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"data-insights/internal/datasource"
	"data-insights/internal/domain/entity"
	"data-insights/internal/extract"
	"data-insights/internal/idgen"
	"data-insights/internal/model"
	"data-insights/internal/response"
	"data-insights/internal/router"
)

// importBatchSize caps how many rows go into one INSERT statement, keeping the
// placeholder count (and packet size) bounded for wide uploads.
const importBatchSize = 500

// ImportRequest is one uploaded file destined for the extract storage.
type ImportRequest struct {
	// Name is the dataset name; empty falls back to the file's base name.
	Name     string
	Filename string
	Data     []byte
}

// ImportFile parses an upload, infers its columns and lands the data in the
// designated extract storage datasource as table di_extract_<id>, registering
// the dataset row (mode=extract) that points at it.
func (s *datasetService) ImportFile(ctx context.Context, req ImportRequest) (*entity.Dataset, error) {
	if !s.extract.Enabled() {
		return nil, router.NewBusinessError(response.CodeBadRequest,
			"extract storage is not configured: set EXTRACT_DATASOURCE_ID to the datasource that should hold uploaded data")
	}

	cols, rows, err := parseUpload(req)
	if err != nil {
		return nil, err
	}

	dsModel, dialect, err := s.extractTarget(ctx)
	if err != nil {
		return nil, err
	}

	// The extract table name is derived from the dataset id, so the dataset
	// row is created first to obtain a stable id.
	created, err := s.insertExtractDataset(ctx, datasetName(req), cols)
	if err != nil {
		return nil, err
	}
	tableName := extract.TableName(created.ID)

	conn, err := s.connect(ctx, dsModel)
	if err != nil {
		_ = s.hardDeleteDataset(ctx, created.ID)
		return nil, err
	}
	defer conn.Close()

	if err := s.ingestInto(ctx, conn, dialect, tableName, cols, rows, false); err != nil {
		// Roll back the half-created dataset; a dataset row without its table
		// would otherwise show up in the list and fail on every query.
		_ = s.hardDeleteDataset(ctx, created.ID)
		_, _ = conn.Execute(ctx, extract.DropTableSQL(dialect, tableName))
		return nil, err
	}

	if err := s.setExtractTableName(ctx, created.ID, tableName); err != nil {
		return nil, err
	}
	return s.GetByID(ctx, created.ID)
}

// ReplaceFile re-uploads a file over an existing extract dataset. Column ids
// are preserved by matching display names, so charts referring to them keep
// working; removed columns drop out and new columns get fresh ids.
func (s *datasetService) ReplaceFile(ctx context.Context, id int, req ImportRequest) (*entity.Dataset, error) {
	if !s.extract.Enabled() {
		return nil, router.NewBusinessError(response.CodeBadRequest,
			"extract storage is not configured: set EXTRACT_DATASOURCE_ID to the datasource that should hold uploaded data")
	}

	existing, err := s.getDatasetModelFn(ctx, id)
	if err != nil {
		return nil, router.NewBusinessError(response.CodeNotFound, err.Error())
	}
	if existing.Mode != "extract" || existing.DatasourceID != s.extract.DatasourceID {
		return nil, router.NewBusinessError(response.CodeBadRequest, "only uploaded (extract) datasets can be replaced")
	}

	parsed, rows, err := parseUpload(req)
	if err != nil {
		return nil, err
	}

	// Preserve ids for columns that survive the re-upload (matched on display
	// name); a name that is gone loses its id, a new name gets a new one.
	previous := decodeColumns(existing.Columns)
	idByName := make(map[string]string, len(previous))
	for _, c := range previous {
		if c.ID != "" {
			idByName[c.Name] = c.ID
		}
	}
	cols := make([]entity.DatasetColumn, len(parsed))
	for i, c := range parsed {
		colID := idByName[c.Name]
		if colID == "" {
			colID = idgen.New()
		}
		cols[i] = entity.DatasetColumn{
			ID:   colID,
			Name: c.Name,
			Expr: c.Physical,
			Type: c.Type,
			Role: inferRole(c.Type),
		}
	}

	dsModel, dialect, err := s.extractTarget(ctx)
	if err != nil {
		return nil, err
	}
	conn, err := s.connect(ctx, dsModel)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	tableName := extract.TableName(id)
	if err := s.ingestInto(ctx, conn, dialect, tableName, parsed, rows, true); err != nil {
		return nil, err
	}

	structured, err := marshalColumns(cols)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.NewUpdate().Model((*model.Dataset)(nil)).
		Set("columns = ?", structured).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to update replaced dataset columns: %w", err)
	}
	return s.GetByID(ctx, id)
}

// parseUpload validates the upload envelope and returns physical columns plus
// the normalized rows.
func parseUpload(req ImportRequest) ([]extract.Column, [][]string, error) {
	if len(req.Data) == 0 {
		return nil, nil, router.NewBusinessError(response.CodeBadRequest, "uploaded file is empty")
	}
	if len(req.Data) > extract.MaxUploadBytes {
		return nil, nil, router.NewBusinessError(response.CodeBadRequest,
			fmt.Sprintf("file exceeds the %d MB limit", extract.MaxUploadBytes>>20))
	}
	table, err := extract.Parse(req.Filename, req.Data)
	if err != nil {
		return nil, nil, router.NewBusinessError(response.CodeBadRequest, err.Error())
	}
	cols := extract.AssignPhysical(extract.InferColumns(table))
	if len(cols) == 0 {
		return nil, nil, router.NewBusinessError(response.CodeBadRequest, "file has no columns")
	}
	return cols, table.Rows, nil
}

// extractTarget loads the designated extract datasource and its dialect.
func (s *datasetService) extractTarget(ctx context.Context) (*model.Datasource, extract.Dialect, error) {
	dsModel, err := s.getDatasourceModelFn(ctx, s.extract.DatasourceID)
	if err != nil {
		return nil, "", router.NewBusinessError(response.CodeBadRequest,
			fmt.Sprintf("extract storage datasource %d is not usable: %v", s.extract.DatasourceID, err))
	}
	dialect, err := extract.ParseDialect(dsModel.Type)
	if err != nil {
		return nil, "", router.NewBusinessError(response.CodeBadRequest, err.Error())
	}
	return dsModel, dialect, nil
}

// ingestInto builds the table (optionally dropping an existing one first) and
// streams the rows in bounded batches. Every value travels as an arg; only
// identifiers derived by the extract package are interpolated.
func (s *datasetService) ingestInto(
	ctx context.Context,
	conn datasource.Connection,
	dialect extract.Dialect,
	tableName string,
	cols []extract.Column,
	rows [][]string,
	dropFirst bool,
) error {
	if dropFirst {
		if _, err := conn.Execute(ctx, extract.DropTableSQL(dialect, tableName)); err != nil {
			return fmt.Errorf("failed to drop existing extract table: %w", err)
		}
	}
	createSQL, err := extract.CreateTableSQL(dialect, tableName, cols)
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, err.Error())
	}
	if _, err := conn.Execute(ctx, createSQL); err != nil {
		return fmt.Errorf("failed to create extract table: %w", err)
	}

	for start := 0; start < len(rows); start += importBatchSize {
		end := start + importBatchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[start:end]

		argRows, err := extract.RowArgs(dialect, cols, batch)
		if err != nil {
			return router.NewBusinessError(response.CodeBadRequest, err.Error())
		}
		insertSQL, err := extract.InsertSQL(dialect, tableName, cols, len(batch))
		if err != nil {
			return err
		}
		flat := make([]any, 0, len(argRows)*len(cols))
		for _, row := range argRows {
			flat = append(flat, row...)
		}
		if _, err := conn.Execute(ctx, insertSQL, flat...); err != nil {
			return fmt.Errorf("failed to insert rows: %w", err)
		}
	}
	return nil
}

func (s *datasetService) insertExtractDataset(ctx context.Context, name string, cols []extract.Column) (*model.Dataset, error) {
	structured, err := marshalColumns(physicalToEntityColumns(cols))
	if err != nil {
		return nil, err
	}
	ds := &entity.Dataset{
		Name:         name,
		DatasourceID: s.extract.DatasourceID,
		QueryType:    "table",
		Mode:         "extract",
		Columns:      structured,
	}
	m := toDatasetModel(ds)
	m.CreatedAt = sql.NullTime{Time: time.Now(), Valid: true}
	m.UpdatedAt = sql.NullTime{Time: time.Now(), Valid: true}
	// Insert directly rather than through Create: Create applies the extract
	// guard, which exists precisely to stop ordinary datasets from targeting
	// the extract storage — this is the one legitimate exception.
	if _, err := s.db.NewInsert().Model(m).Returning("*").Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create extract dataset: %w", err)
	}
	return m, nil
}

// physicalToEntityColumns assigns stable ids to freshly parsed columns.
func physicalToEntityColumns(cols []extract.Column) []entity.DatasetColumn {
	out := make([]entity.DatasetColumn, len(cols))
	for i, c := range cols {
		out[i] = entity.DatasetColumn{
			ID:   idgen.New(),
			Name: c.Name,
			Expr: c.Physical,
			Type: c.Type,
			Role: inferRole(c.Type),
		}
	}
	return out
}

func (s *datasetService) setExtractTableName(ctx context.Context, id int, tableName string) error {
	if _, err := s.db.NewUpdate().Model((*model.Dataset)(nil)).
		Set("table_name = ?", tableName).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to record extract table name: %w", err)
	}
	return nil
}

func (s *datasetService) hardDeleteDataset(ctx context.Context, id int) error {
	if _, err := s.db.NewDelete().Model((*model.Dataset)(nil)).Where("id = ?", id).Exec(ctx); err != nil {
		return fmt.Errorf("failed to roll back dataset %d: %w", id, err)
	}
	return nil
}

// dropExtractTable removes the physical table of an extract dataset. Used by
// Delete so an upload's storage is released with its dataset.
func (s *datasetService) dropExtractTable(ctx context.Context, id int) error {
	dsModel, err := s.getDatasourceModelFn(ctx, s.extract.DatasourceID)
	if err != nil {
		return fmt.Errorf("failed to load extract storage datasource: %w", err)
	}
	dialect, err := extract.ParseDialect(dsModel.Type)
	if err != nil {
		return err
	}
	conn, err := s.connect(ctx, dsModel)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.Execute(ctx, extract.DropTableSQL(dialect, extract.TableName(id))); err != nil {
		return fmt.Errorf("failed to drop extract table: %w", err)
	}
	return nil
}

func datasetName(req ImportRequest) string {
	if name := strings.TrimSpace(req.Name); name != "" {
		return name
	}
	base := strings.TrimSuffix(filepath.Base(req.Filename), filepath.Ext(req.Filename))
	if base == "" {
		return "uploaded dataset"
	}
	return base
}

func marshalColumns(cols []entity.DatasetColumn) (string, error) {
	b, err := json.Marshal(cols)
	if err != nil {
		return "", fmt.Errorf("failed to marshal columns: %w", err)
	}
	return string(b), nil
}

func decodeColumns(raw string) []entity.DatasetColumn {
	if raw == "" || raw == "[]" {
		return nil
	}
	var cols []entity.DatasetColumn
	if err := json.Unmarshal([]byte(raw), &cols); err != nil {
		return nil
	}
	return cols
}
