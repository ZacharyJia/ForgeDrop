package db

import (
	"context"
	"database/sql"
	"fmt"
)

func Migrate(ctx context.Context, sqlDB *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := sqlDB.ExecContext(ctx, s); err != nil {
			return err
		}
	}

	var current int
	if err := sqlDB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return err
	}
	for v := current + 1; v <= latestSchemaVersion; v++ {
		if err := applyMigration(ctx, sqlDB, v); err != nil {
			return fmt.Errorf("migration %d: %w", v, err)
		}
	}
	return nil
}

const latestSchemaVersion = 1

func applyMigration(ctx context.Context, sqlDB *sql.DB, version int) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	switch version {
	case 1:
		if err := migrationV1(ctx, tx); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown schema version %d", version)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, datetime('now'))`, version); err != nil {
		return err
	}
	return tx.Commit()
}

func migrationV1(ctx context.Context, tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash BLOB NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			token_hash BLOB NOT NULL UNIQUE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS api_tokens (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			prefix TEXT NOT NULL,
			token_hash BLOB NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			revoked_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS repos (
			id TEXT PRIMARY KEY,
			full_name TEXT NOT NULL UNIQUE,
			slug TEXT NOT NULL,
			webhook_secret TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS apps (
			id TEXT PRIMARY KEY,
			app_key TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS services (
			id TEXT PRIMARY KEY,
			app_id TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
			service_key TEXT NOT NULL,
			name TEXT NOT NULL,
			image TEXT NOT NULL,
			command TEXT NOT NULL,
			container_port INTEGER NOT NULL,
			run_user TEXT NOT NULL,
			env_json TEXT NOT NULL DEFAULT '{}',
			prod_host TEXT NOT NULL DEFAULT '',
			traefik_entrypoints TEXT NOT NULL DEFAULT 'websecure',
			revision INTEGER NOT NULL DEFAULT 1,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(app_id, service_key)
		)`,
		`CREATE TABLE IF NOT EXISTS slots (
			id TEXT PRIMARY KEY,
			service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
			slot_key TEXT NOT NULL,
			name TEXT NOT NULL,
			repo_id TEXT NOT NULL REFERENCES repos(id) ON DELETE RESTRICT,
			container_path TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(service_id, slot_key)
		)`,
		`CREATE TABLE IF NOT EXISTS envs (
			id TEXT PRIMARY KEY,
			app_id TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
			kind TEXT NOT NULL, -- named|preview
			name TEXT NOT NULL, -- for named envs: prod/staging; for preview: "preview"
			repo_id TEXT REFERENCES repos(id) ON DELETE RESTRICT,
			pr_number INTEGER,
			current_snapshot_id TEXT,
			created_at TEXT NOT NULL,
			deleted_at TEXT,
			UNIQUE(app_id, kind, name, repo_id, pr_number)
		)`,
		`CREATE TABLE IF NOT EXISTS artifacts (
			id TEXT PRIMARY KEY,
			app_id TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
			service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
			slot_id TEXT NOT NULL REFERENCES slots(id) ON DELETE CASCADE,
			repo_id TEXT NOT NULL REFERENCES repos(id) ON DELETE RESTRICT,
			sha TEXT NOT NULL DEFAULT '',
			ref TEXT NOT NULL DEFAULT '',
			pr_number INTEGER,
			original_filename TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			sha256_hex TEXT NOT NULL,
			stored_path TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS snapshots (
			id TEXT PRIMARY KEY,
			env_id TEXT NOT NULL REFERENCES envs(id) ON DELETE CASCADE,
			created_at TEXT NOT NULL,
			created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
			created_by_token_id TEXT REFERENCES api_tokens(id) ON DELETE SET NULL,
			note TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS snapshot_slots (
			snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
			slot_id TEXT NOT NULL REFERENCES slots(id) ON DELETE CASCADE,
			artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE RESTRICT,
			PRIMARY KEY(snapshot_id, slot_id)
		)`,
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}
