package db

import (
	"context"
	"fmt"
)

// CreateSchema creates a dedicated Postgres schema on databaseURL.
func CreateSchema(ctx context.Context, databaseURL, schema string) error {
	return execSchemaDDL(ctx, databaseURL, "CREATE SCHEMA "+schema)
}

// DropSchema drops a Postgres schema and all contained objects.
func DropSchema(ctx context.Context, databaseURL, schema string) error {
	return execSchemaDDL(ctx, databaseURL, "DROP SCHEMA "+schema+" CASCADE")
}

func execSchemaDDL(ctx context.Context, databaseURL, query string) error {
	database, err := Open(ctx, databaseURL, PoolOptions{})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	_, err = database.SQLDB().ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("exec schema ddl: %w", err)
	}

	return nil
}
