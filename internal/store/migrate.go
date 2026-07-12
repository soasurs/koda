package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

const currentSchemaVersion = 1

func initSchema(ctx context.Context, db *sqlx.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS koda_schema_migrations (
			version INTEGER PRIMARY KEY
		)
	`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	for version := 1; version <= currentSchemaVersion; version++ {
		if err := applyMigration(ctx, db, version); err != nil {
			return fmt.Errorf("apply migration v%d: %w", version, err)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sqlx.DB, version int) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // sql.ErrTxDone is expected after Commit.

	var applied int
	err = tx.GetContext(ctx, &applied, `
		SELECT 1
		FROM koda_schema_migrations
		WHERE version = $1
	`, version)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	for _, statement := range migrationSQL(version) {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO koda_schema_migrations (version)
		VALUES ($1)
	`, version); err != nil {
		return err
	}
	return tx.Commit()
}

func migrationSQL(version int) []string {
	switch version {
	case 1:
		return []string{
			`
				CREATE TABLE koda_sessions (
					id                TEXT PRIMARY KEY,
					title             TEXT NOT NULL DEFAULT '',
					workdir           TEXT NOT NULL,
					provider_id       TEXT NOT NULL,
					model_id          TEXT NOT NULL,
					reasoning_effort  TEXT NOT NULL DEFAULT '',
					file_access       TEXT NOT NULL,
					shell_access      TEXT NOT NULL,
					created_at        BIGINT NOT NULL,
					updated_at        BIGINT NOT NULL,
					deleted_at        BIGINT NOT NULL DEFAULT 0
				)
			`,
			`
				CREATE INDEX idx_koda_sessions_active_updated
				ON koda_sessions (deleted_at, updated_at DESC, id ASC)
			`,
		}
	default:
		panic(fmt.Sprintf("unknown store migration version %d", version))
	}
}
