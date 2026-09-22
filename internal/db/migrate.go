// Package db provides database migration helpers.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"sync"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// gooseMu serializes goose global setters (SetBaseFS / SetDialect) used by Migrate.
// Parallel integration tests each call Migrate on isolated schemas; without this,
// the race detector fails on concurrent SetBaseFS.
var gooseMu sync.Mutex

// Migrate runs embedded SQL migrations against the database.
func Migrate(sqlDB *sql.DB) error {
	gooseMu.Lock()
	defer gooseMu.Unlock()

	goose.SetBaseFS(migrations)

	err := goose.SetDialect("postgres")
	if err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	err = goose.Up(sqlDB, "migrations")
	if err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}
