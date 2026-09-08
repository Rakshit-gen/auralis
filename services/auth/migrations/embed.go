// Package migrations embeds the auth service SQL migrations.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
