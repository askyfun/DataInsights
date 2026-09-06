// Package migrations embeds the SQL migration files so that the binary
// carries its own schema (no external files needed at deploy time).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
