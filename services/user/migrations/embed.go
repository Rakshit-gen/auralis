// Package migrations embeds the user service SQL migrations.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
