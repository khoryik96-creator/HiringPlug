// Package db embeds the SQL migrations into every binary, so `hp migrate` and
// the startup check always use exactly the migrations the code was built with.
package db

import (
	"embed"
	"io/fs"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrations returns the migration files (00001_platform.sql, ...).
func Migrations() fs.FS {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}
