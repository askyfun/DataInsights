// Package extract holds the shared semantics of the "extract dataset" feature
// (issue #118): one designated datasource acts as the unified storage for
// extracted data, and local file upload is a special case of it. Ingestion
// (DDL + batched inserts) lives in service/dataset/import.go.
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
	// SelfProvisioned marks a datasource this process created for itself
	// (issue #201 / R-85) because EXTRACT_DATASOURCE_ID was left unset. It
	// changes no guard semantics — the storage is hidden and barred from
	// regular datasets exactly like a user-designated one — it only records
	// provenance so the UI and logs can say where the storage came from.
	SelfProvisioned bool
}

// Enabled reports whether an extract storage datasource has been designated.
func (g Guard) Enabled() bool { return g.DatasourceID > 0 }

// CheckDatasource rejects targeting the extract storage datasource with a
// regular dataset. It is a no-op when the feature is disabled or the ID
// differs.
//
// The message names the storage by role rather than by env var: the storage
// may be self-provisioned (issue #201), in which case EXTRACT_DATASOURCE_ID is
// not set at all and quoting it would send the user looking for a variable
// they never wrote.
func (g Guard) CheckDatasource(datasourceID int) error {
	if g.Enabled() && datasourceID == g.DatasourceID {
		if g.SelfProvisioned {
			return fmt.Errorf(
				"%w: datasource %d is this app's own upload storage and cannot back regular datasets",
				ErrExtractDatasource, datasourceID,
			)
		}
		return fmt.Errorf(
			"%w: datasource %d is designated as the extract storage (EXTRACT_DATASOURCE_ID) and cannot back regular datasets",
			ErrExtractDatasource, datasourceID,
		)
	}
	return nil
}
