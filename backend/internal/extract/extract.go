// Package extract holds the shared semantics of the "extract dataset"
// reservation (issue #118): one designated StarRocks datasource acts as the
// unified storage for extracted data, and local file upload is a special case
// of it. This phase reserves the architecture only — no data ingestion is
// implemented yet.
//
// The package is the single source of truth for:
//   - the guard that keeps the extract storage datasource out of regular use
//     (no datasets may be created on it or re-pointed to it);
//   - the table-name convention that keeps extract tables namespaced away
//     from user tables.
package extract

import (
	"errors"
	"fmt"
)

// ErrExtractDatasource is the sentinel wrapped by Guard.CheckDatasource so
// callers (services, handlers, tests) can identify the violation with
// errors.Is. The router renders plain errors as 50000, so handlers that must
// answer 20100 check errors.Is and re-emit as a BusinessError.
var ErrExtractDatasource = errors.New("extract storage datasource violation")

// TablePrefix namespaces every extract table inside the storage datasource,
// so ingestion can never collide with user tables that live in the same
// database.
const TablePrefix = "di_extract_"

// TableName returns the physical table name holding the extracted data of one
// dataset. The ID is stable and numeric, hence no identifier escaping is
// needed at the reserved layer; the ingestion phase must still build SQL
// through the query package's identifier whitelist.
func TableName(datasetID int) string {
	return fmt.Sprintf("%s%d", TablePrefix, datasetID)
}

// Guard carries the configured extract storage datasource ID (0 = feature
// disabled, the default when EXTRACT_DATASOURCE_ID is unset).
type Guard struct {
	DatasourceID int
}

// Enabled reports whether an extract storage datasource has been designated.
func (g Guard) Enabled() bool { return g.DatasourceID > 0 }

// CheckDatasource rejects targeting the extract storage datasource with a
// regular dataset. It is a no-op when the feature is disabled or the ID
// differs.
func (g Guard) CheckDatasource(datasourceID int) error {
	if g.Enabled() && datasourceID == g.DatasourceID {
		return fmt.Errorf(
			"%w: datasource %d is designated as the extract storage (EXTRACT_DATASOURCE_ID) and cannot back regular datasets",
			ErrExtractDatasource, datasourceID,
		)
	}
	return nil
}
