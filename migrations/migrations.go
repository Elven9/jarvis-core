// Package migrations embeds the SQLite schema migrations run by goose at startup.
//
// Before the first release only 00001_init.sql exists and may be edited freely
// (delete the dev database after each edit). From the first release on, a shipped
// file is never edited: every schema change is a new, higher-numbered file.
package migrations

import "embed"

// FS holds the migration files.
//
//go:embed *.sql
var FS embed.FS
