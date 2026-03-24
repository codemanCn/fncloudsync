package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type TaskRuntimeRepository struct {
	db *sql.DB
}

func NewTaskRuntimeRepository(db *sql.DB) *TaskRuntimeRepository {
	return &TaskRuntimeRepository{db: db}
}

func (r *TaskRuntimeRepository) GetByTaskID(ctx context.Context, taskID string) (domain.TaskRuntimeState, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT task_id, phase, last_local_scan_at, last_remote_scan_at, last_reconcile_at, last_success_at,
		       backoff_until, retry_streak, last_error, updated_at
		FROM task_runtime_state
		WHERE task_id = ?
	`, taskID)

	var state domain.TaskRuntimeState
	var lastLocalScanAt string
	var lastRemoteScanAt string
	var lastReconcileAt string
	var lastSuccessAt string
	var backoffUntil string
	var updatedAt string
	err := row.Scan(
		&state.TaskID,
		&state.Phase,
		&lastLocalScanAt,
		&lastRemoteScanAt,
		&lastReconcileAt,
		&lastSuccessAt,
		&backoffUntil,
		&state.RetryStreak,
		&state.LastError,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.TaskRuntimeState{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.TaskRuntimeState{}, err
	}

	state.LastLocalScanAt = parseOptionalTimestamp(lastLocalScanAt)
	state.LastRemoteScanAt = parseOptionalTimestamp(lastRemoteScanAt)
	state.LastReconcileAt = parseOptionalTimestamp(lastReconcileAt)
	state.LastSuccessAt = parseOptionalTimestamp(lastSuccessAt)
	state.BackoffUntil = parseOptionalTimestamp(backoffUntil)
	state.UpdatedAt = parseOptionalTimestamp(updatedAt)
	return state, nil
}

func (r *TaskRuntimeRepository) Upsert(ctx context.Context, state domain.TaskRuntimeState) error {
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO task_runtime_state (
			task_id, phase, last_local_scan_at, last_remote_scan_at, last_reconcile_at, last_success_at,
			backoff_until, retry_streak, last_error, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			phase = excluded.phase,
			last_local_scan_at = excluded.last_local_scan_at,
			last_remote_scan_at = excluded.last_remote_scan_at,
			last_reconcile_at = excluded.last_reconcile_at,
			last_success_at = excluded.last_success_at,
			backoff_until = excluded.backoff_until,
			retry_streak = excluded.retry_streak,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at
	`,
		state.TaskID,
		state.Phase,
		formatOptionalTimestamp(state.LastLocalScanAt),
		formatOptionalTimestamp(state.LastRemoteScanAt),
		formatOptionalTimestamp(state.LastReconcileAt),
		formatOptionalTimestamp(state.LastSuccessAt),
		formatOptionalTimestamp(state.BackoffUntil),
		state.RetryStreak,
		state.LastError,
		state.UpdatedAt.UTC().Format(timestampLayout),
	)
	return mapSQLError(err)
}

func formatOptionalTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(timestampLayout)
}

func parseOptionalTimestamp(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
