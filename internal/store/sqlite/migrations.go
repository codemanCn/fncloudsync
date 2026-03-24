package sqlite

import (
	"context"
	"database/sql"
	"fmt"
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
		capabilities_json TEXT NOT NULL DEFAULT '',
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
	`
	CREATE TABLE IF NOT EXISTS task_runtime_state (
		task_id TEXT PRIMARY KEY,
		phase TEXT NOT NULL DEFAULT '',
		last_local_scan_at TEXT NOT NULL DEFAULT '',
		last_remote_scan_at TEXT NOT NULL DEFAULT '',
		last_reconcile_at TEXT NOT NULL DEFAULT '',
		last_success_at TEXT NOT NULL DEFAULT '',
		backoff_until TEXT NOT NULL DEFAULT '',
		retry_streak INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);
	`,
	`
	CREATE TABLE IF NOT EXISTS operation_queue (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		op_type TEXT NOT NULL,
		target_path TEXT NOT NULL,
		src_side TEXT NOT NULL DEFAULT '',
		reason TEXT NOT NULL DEFAULT '',
		payload_json TEXT NOT NULL DEFAULT '',
		priority INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'pending',
		attempt_count INTEGER NOT NULL DEFAULT 0,
		next_attempt_at TEXT NOT NULL DEFAULT '',
		last_error TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);
	`,
	`
	CREATE TABLE IF NOT EXISTS failure_records (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		path TEXT NOT NULL,
		op_type TEXT NOT NULL,
		error_code TEXT NOT NULL DEFAULT '',
		error_message TEXT NOT NULL DEFAULT '',
		retryable INTEGER NOT NULL DEFAULT 0,
		first_failed_at TEXT NOT NULL,
		last_failed_at TEXT NOT NULL,
		attempt_count INTEGER NOT NULL DEFAULT 1,
		resolved_at TEXT NOT NULL DEFAULT '',
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
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

	exists, err := columnExists(ctx, tx, "connections", "capabilities_json")
	if err != nil {
		tx.Rollback()
		return err
	}
	if !exists {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE connections ADD COLUMN capabilities_json TEXT NOT NULL DEFAULT ''`); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func columnExists(ctx context.Context, tx *sql.Tx, tableName, columnName string) (bool, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}

	return false, rows.Err()
}
