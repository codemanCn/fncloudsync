package sqlite

import (
	"context"
	"database/sql"
)

var migrations = []string{
	`
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY
	);
	`,
	`
	CREATE TABLE IF NOT EXISTS connections (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		endpoint TEXT NOT NULL,
		username TEXT NOT NULL,
		password_ciphertext TEXT NOT NULL DEFAULT '',
		root_path TEXT NOT NULL DEFAULT '',
		tls_mode TEXT NOT NULL DEFAULT 'strict',
		timeout_sec INTEGER NOT NULL DEFAULT 30,
		status TEXT NOT NULL DEFAULT 'active',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	`,
	`
	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		connection_id TEXT NOT NULL,
		local_path TEXT NOT NULL,
		remote_path TEXT NOT NULL,
		direction TEXT NOT NULL,
		poll_interval_sec INTEGER NOT NULL DEFAULT 30,
		conflict_policy TEXT NOT NULL DEFAULT 'keep_both',
		delete_policy TEXT NOT NULL DEFAULT 'mirror',
		empty_dir_policy TEXT NOT NULL DEFAULT 'keep',
		bandwidth_limit_kbps INTEGER NOT NULL DEFAULT 0,
		max_workers INTEGER NOT NULL DEFAULT 1,
		encryption_enabled INTEGER NOT NULL DEFAULT 0,
		hash_mode TEXT NOT NULL DEFAULT 'basic',
		status TEXT NOT NULL DEFAULT 'created',
		desired_state TEXT NOT NULL DEFAULT '',
		last_error TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE RESTRICT
	);
	`,
}

func Migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	for _, stmt := range migrations {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}
