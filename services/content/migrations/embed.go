// Package migrations embeds the content service SQL migrations.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
