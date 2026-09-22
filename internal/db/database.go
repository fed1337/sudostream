package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var errDatabaseNotConfigured = errors.New("database not configured")

// Database holds shared GORM and database/sql handles.
type Database struct {
	GORM *gorm.DB
	sql  *sql.DB
}

// Open creates GORM with a tuned database/sql pool and verifies connectivity.
func Open(ctx context.Context, databaseURL string, opts PoolOptions) (*Database, error) {
	gormDB, err := openGORM(databaseURL, opts)
	if err != nil {
		return nil, err
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("gorm sql handle: %w", err)
	}

	err = sqlDB.PingContext(ctx)
	if err != nil {
		_ = sqlDB.Close()

		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Database{
		GORM: gormDB,
		sql:  sqlDB,
	}, nil
}

// SQLDB returns the underlying database/sql handle for goose migrations.
func (d *Database) SQLDB() *sql.DB {
	if d == nil {
		return nil
	}

	return d.sql
}

// Ping verifies database connectivity.
func (d *Database) Ping(ctx context.Context) error {
	if d == nil || d.sql == nil {
		return errDatabaseNotConfigured
	}

	err := d.sql.PingContext(ctx)
	if err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	return nil
}

// Close releases pool connections.
func (d *Database) Close() {
	if d != nil && d.sql != nil {
		_ = d.sql.Close()
	}
}
