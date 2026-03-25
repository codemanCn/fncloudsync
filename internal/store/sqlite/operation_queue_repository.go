package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type OperationQueueRepository struct {
	db *sql.DB
}

func NewOperationQueueRepository(db *sql.DB) *OperationQueueRepository {
	return &OperationQueueRepository{db: db}
}

func (r *OperationQueueRepository) Enqueue(ctx context.Context, item domain.OperationQueueItem) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO operation_queue (
			id, task_id, op_type, target_path, src_side, reason, payload_json, priority, status,
			attempt_count, next_attempt_at, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		item.ID,
		item.TaskID,
		item.OpType,
		item.TargetPath,
		item.SrcSide,
		item.Reason,
		item.PayloadJSON,
		item.Priority,
		queueStatusOrDefault(item.Status),
		item.AttemptCount,
		formatOptionalTimestamp(item.NextAttemptAt),
		item.LastError,
		item.CreatedAt.UTC().Format(timestampLayout),
		item.UpdatedAt.UTC().Format(timestampLayout),
	)
	return mapSQLError(err)
}

func (r *OperationQueueRepository) ListByTaskID(ctx context.Context, taskID string) ([]domain.OperationQueueItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, op_type, target_path, src_side, reason, payload_json, priority, status,
		       attempt_count, next_attempt_at, last_error, created_at, updated_at
		FROM operation_queue
		WHERE task_id = ?
		ORDER BY created_at ASC, id ASC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.OperationQueueItem
	for rows.Next() {
		var item domain.OperationQueueItem
		var nextAttemptAt string
		var createdAt string
		var updatedAt string
		if err := rows.Scan(
			&item.ID,
			&item.TaskID,
			&item.OpType,
			&item.TargetPath,
			&item.SrcSide,
			&item.Reason,
			&item.PayloadJSON,
			&item.Priority,
			&item.Status,
			&item.AttemptCount,
			&nextAttemptAt,
			&item.LastError,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		item.NextAttemptAt = parseOptionalTimestamp(nextAttemptAt)
		item.CreatedAt = parseOptionalTimestamp(createdAt)
		item.UpdatedAt = parseOptionalTimestamp(updatedAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *OperationQueueRepository) Dequeue(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE operation_queue
		SET status = 'succeeded', updated_at = ?
		WHERE id = ?
	`, time.Now().UTC().Format(timestampLayout), id)
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

func (r *OperationQueueRepository) GetByID(ctx context.Context, id string) (domain.OperationQueueItem, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, op_type, target_path, src_side, reason, payload_json, priority, status,
		       attempt_count, next_attempt_at, last_error, created_at, updated_at
		FROM operation_queue WHERE id = ?
	`, id)
	var item domain.OperationQueueItem
	var nextAttemptAt string
	var createdAt string
	var updatedAt string
	err := row.Scan(
		&item.ID, &item.TaskID, &item.OpType, &item.TargetPath, &item.SrcSide, &item.Reason,
		&item.PayloadJSON, &item.Priority, &item.Status, &item.AttemptCount, &nextAttemptAt,
		&item.LastError, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.OperationQueueItem{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.OperationQueueItem{}, err
	}
	item.NextAttemptAt = parseOptionalTimestamp(nextAttemptAt)
	item.CreatedAt = parseOptionalTimestamp(createdAt)
	item.UpdatedAt = parseOptionalTimestamp(updatedAt)
	return item, nil
}

func (r *OperationQueueRepository) ListDue(ctx context.Context, now time.Time, limit int) ([]domain.OperationQueueItem, error) {
	if limit <= 0 {
		limit = 64
	}

	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, task_id, op_type, target_path, src_side, reason, payload_json, priority, status,
		       attempt_count, next_attempt_at, last_error, created_at, updated_at
		FROM operation_queue
		WHERE status IN ('pending', 'queued', 'retry_wait') AND (next_attempt_at = '' OR next_attempt_at <= ?)
		ORDER BY priority DESC, created_at ASC, id ASC
		LIMIT %d
	`, limit), now.UTC().Format(timestampLayout))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.OperationQueueItem
	for rows.Next() {
		item, err := scanOperationQueueItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *OperationQueueRepository) Reschedule(ctx context.Context, item domain.OperationQueueItem) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE operation_queue
		SET status = ?, attempt_count = ?, next_attempt_at = ?, last_error = ?, updated_at = ?
		WHERE id = ?
	`,
		queueStatusOrDefault(item.Status),
		item.AttemptCount,
		formatOptionalTimestamp(item.NextAttemptAt),
		item.LastError,
		item.UpdatedAt.UTC().Format(timestampLayout),
		item.ID,
	)
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

func (r *OperationQueueRepository) ResetRetryableByTaskID(ctx context.Context, taskID string) (int, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE operation_queue
		SET status = 'queued', next_attempt_at = '', updated_at = ?
		WHERE task_id = ? AND status IN ('retry_wait', 'executing')
	`, time.Now().UTC().Format(timestampLayout), taskID)
	if err != nil {
		return 0, mapSQLError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

type operationQueueScanner interface {
	Scan(dest ...any) error
}

func scanOperationQueueItem(row operationQueueScanner) (domain.OperationQueueItem, error) {
	var item domain.OperationQueueItem
	var nextAttemptAt string
	var createdAt string
	var updatedAt string
	if err := row.Scan(
		&item.ID,
		&item.TaskID,
		&item.OpType,
		&item.TargetPath,
		&item.SrcSide,
		&item.Reason,
		&item.PayloadJSON,
		&item.Priority,
		&item.Status,
		&item.AttemptCount,
		&nextAttemptAt,
		&item.LastError,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.OperationQueueItem{}, err
	}
	item.NextAttemptAt = parseOptionalTimestamp(nextAttemptAt)
	item.CreatedAt = parseOptionalTimestamp(createdAt)
	item.UpdatedAt = parseOptionalTimestamp(updatedAt)
	return item, nil
}

func (r *OperationQueueRepository) MarkFailed(ctx context.Context, id string, lastError string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE operation_queue
		SET status = 'failed', last_error = ?, updated_at = ?
		WHERE id = ?
	`, lastError, time.Now().UTC().Format(timestampLayout), id)
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

func queueStatusOrDefault(status string) string {
	if status == "" || status == "pending" {
		return "queued"
	}
	return status
}
