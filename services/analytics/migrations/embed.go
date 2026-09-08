// Package migrations embeds the analytics service SQL migrations.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
