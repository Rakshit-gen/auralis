// Package migrations embeds the playback service SQL migrations.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
