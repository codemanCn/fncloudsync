package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type TaskEventRepository struct {
	db *sql.DB
}

func NewTaskEventRepository(db *sql.DB) *TaskEventRepository {
	return &TaskEventRepository{db: db}
}

func (r *TaskEventRepository) Create(ctx context.Context, event domain.TaskEvent) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO task_events (
			id, task_id, event_type, level, message, details_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		event.ID,
		event.TaskID,
		event.EventType,
		event.Level,
		event.Message,
		event.DetailsJSON,
		formatOptionalTimestamp(event.CreatedAt),
	)
	return mapSQLError(err)
}

func (r *TaskEventRepository) ListByTaskID(ctx context.Context, taskID string, limit int) ([]domain.TaskEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, task_id, event_type, level, message, details_json, created_at
		FROM task_events
		WHERE task_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT %d
	`, limit), taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.TaskEvent
	for rows.Next() {
		var item domain.TaskEvent
		var createdAt string
		if err := rows.Scan(
			&item.ID,
			&item.TaskID,
			&item.EventType,
			&item.Level,
			&item.Message,
			&item.DetailsJSON,
			&createdAt,
		); err != nil {
			return nil, err
		}
		item.CreatedAt = parseOptionalTimestamp(createdAt)
		items = append(items, item)
	}
	return items, rows.Err()
}
