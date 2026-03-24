package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type FailureRecordRepository struct {
	db *sql.DB
}

func NewFailureRecordRepository(db *sql.DB) *FailureRecordRepository {
	return &FailureRecordRepository{db: db}
}

func (r *FailureRecordRepository) Create(ctx context.Context, record domain.FailureRecord) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO failure_records (
			id, task_id, path, op_type, error_code, error_message, retryable,
			first_failed_at, last_failed_at, attempt_count, resolved_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		record.ID,
		record.TaskID,
		record.Path,
		record.OpType,
		record.ErrorCode,
		record.ErrorMessage,
		record.Retryable,
		record.FirstFailedAt.UTC().Format(timestampLayout),
		record.LastFailedAt.UTC().Format(timestampLayout),
		record.AttemptCount,
		formatOptionalTimestamp(record.ResolvedAt),
	)
	return mapSQLError(err)
}

func (r *FailureRecordRepository) ListByTaskID(ctx context.Context, taskID string) ([]domain.FailureRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, path, op_type, error_code, error_message, retryable,
		       first_failed_at, last_failed_at, attempt_count, resolved_at
		FROM failure_records
		WHERE task_id = ?
		ORDER BY first_failed_at ASC, id ASC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []domain.FailureRecord
	for rows.Next() {
		var record domain.FailureRecord
		var firstFailedAt string
		var lastFailedAt string
		var resolvedAt string
		if err := rows.Scan(
			&record.ID,
			&record.TaskID,
			&record.Path,
			&record.OpType,
			&record.ErrorCode,
			&record.ErrorMessage,
			&record.Retryable,
			&firstFailedAt,
			&lastFailedAt,
			&record.AttemptCount,
			&resolvedAt,
		); err != nil {
			return nil, err
		}
		record.FirstFailedAt = parseOptionalTimestamp(firstFailedAt)
		record.LastFailedAt = parseOptionalTimestamp(lastFailedAt)
		record.ResolvedAt = parseOptionalTimestamp(resolvedAt)
		records = append(records, record)
	}
	return records, rows.Err()
}

func (r *FailureRecordRepository) GetByID(ctx context.Context, id string) (domain.FailureRecord, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, path, op_type, error_code, error_message, retryable,
		       first_failed_at, last_failed_at, attempt_count, resolved_at
		FROM failure_records WHERE id = ?
	`, id)
	var record domain.FailureRecord
	var firstFailedAt string
	var lastFailedAt string
	var resolvedAt string
	err := row.Scan(
		&record.ID, &record.TaskID, &record.Path, &record.OpType, &record.ErrorCode,
		&record.ErrorMessage, &record.Retryable, &firstFailedAt, &lastFailedAt,
		&record.AttemptCount, &resolvedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FailureRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.FailureRecord{}, err
	}
	record.FirstFailedAt = parseOptionalTimestamp(firstFailedAt)
	record.LastFailedAt = parseOptionalTimestamp(lastFailedAt)
	record.ResolvedAt = parseOptionalTimestamp(resolvedAt)
	return record, nil
}

func (r *FailureRecordRepository) Resolve(ctx context.Context, id string, resolvedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE failure_records
		SET resolved_at = ?
		WHERE id = ?
	`, formatOptionalTimestamp(resolvedAt), id)
	if err != nil {
		return mapSQLError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
