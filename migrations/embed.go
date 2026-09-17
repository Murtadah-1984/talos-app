// Package migrations embeds the platform's SQL migration files so binaries
// (platform-api, platform-worker) can apply them without shipping a
// separate migrations directory alongside the compiled artifact.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
