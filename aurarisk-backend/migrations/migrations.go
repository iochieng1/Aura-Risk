// Package migrations holds the versioned SQL schema migrations. Files are
// named NNN_description.sql and applied in order by database.Migrate.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
