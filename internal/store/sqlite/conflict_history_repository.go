package sqlite

import (
	"context"
	"database/sql"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type ConflictHistoryRepository struct {
	db *sql.DB
}

func NewConflictHistoryRepository(db *sql.DB) *ConflictHistoryRepository {
	return &ConflictHistoryRepository{db: db}
}

func (r *ConflictHistoryRepository) Create(ctx context.Context, record domain.ConflictRecord) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO conflict_history (
			id, task_id, relative_path, local_conflict_path, remote_conflict_path, policy, detected_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		record.ID,
		record.TaskID,
		record.RelativePath,
		record.LocalConflictPath,
		record.RemoteConflictPath,
		record.Policy,
		formatOptionalTimestamp(record.DetectedAt),
	)
	return mapSQLError(err)
}

func (r *ConflictHistoryRepository) ListByTaskID(ctx context.Context, taskID string) ([]domain.ConflictRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, relative_path, local_conflict_path, remote_conflict_path, policy, detected_at
		FROM conflict_history
		WHERE task_id = ?
		ORDER BY detected_at DESC, id DESC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.ConflictRecord
	for rows.Next() {
		var item domain.ConflictRecord
		var detectedAt string
		if err := rows.Scan(
			&item.ID,
			&item.TaskID,
			&item.RelativePath,
			&item.LocalConflictPath,
			&item.RemoteConflictPath,
			&item.Policy,
			&detectedAt,
		); err != nil {
			return nil, err
		}
		item.DetectedAt = parseOptionalTimestamp(detectedAt)
		items = append(items, item)
	}
	return items, rows.Err()
}
