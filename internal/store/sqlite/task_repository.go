package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

const timestampLayout = time.RFC3339Nano

type TaskRepository struct {
	db *sql.DB
}

func NewTaskRepository(db *sql.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) Create(ctx context.Context, task domain.Task) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tasks (
			id, name, connection_id, local_path, remote_path, direction, poll_interval_sec, conflict_policy, delete_policy,
			empty_dir_policy, bandwidth_limit_kbps, max_workers, encryption_enabled, hash_mode, status, desired_state,
			last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		task.ID,
		task.Name,
		task.ConnectionID,
		task.LocalPath,
		task.RemotePath,
		task.Direction,
		task.PollIntervalSec,
		task.ConflictPolicy,
		task.DeletePolicy,
		task.EmptyDirPolicy,
		task.BandwidthLimitKbps,
		task.MaxWorkers,
		task.EncryptionEnabled,
		task.HashMode,
		task.Status,
		task.DesiredState,
		task.LastError,
		task.CreatedAt.UTC().Format(timestampLayout),
		task.UpdatedAt.UTC().Format(timestampLayout),
	)
	return mapSQLError(err)
}

func (r *TaskRepository) GetByID(ctx context.Context, id string) (domain.Task, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, connection_id, local_path, remote_path, direction, poll_interval_sec, conflict_policy, delete_policy,
		       empty_dir_policy, bandwidth_limit_kbps, max_workers, encryption_enabled, hash_mode, status, desired_state,
		       last_error, created_at, updated_at
		FROM tasks
		WHERE id = ?
	`, id)

	task, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Task{}, domain.ErrNotFound
	}
	return task, err
}

func (r *TaskRepository) List(ctx context.Context) ([]domain.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, connection_id, local_path, remote_path, direction, poll_interval_sec, conflict_policy, delete_policy,
		       empty_dir_policy, bandwidth_limit_kbps, max_workers, encryption_enabled, hash_mode, status, desired_state,
		       last_error, created_at, updated_at
		FROM tasks
		ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, task)
	}

	return items, rows.Err()
}

func (r *TaskRepository) Update(ctx context.Context, task domain.Task) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET name = ?, connection_id = ?, local_path = ?, remote_path = ?, direction = ?, poll_interval_sec = ?, conflict_policy = ?,
		    delete_policy = ?, empty_dir_policy = ?, bandwidth_limit_kbps = ?, max_workers = ?, encryption_enabled = ?, hash_mode = ?,
		    status = ?, desired_state = ?, last_error = ?, updated_at = ?
		WHERE id = ?
	`,
		task.Name,
		task.ConnectionID,
		task.LocalPath,
		task.RemotePath,
		task.Direction,
		task.PollIntervalSec,
		task.ConflictPolicy,
		task.DeletePolicy,
		task.EmptyDirPolicy,
		task.BandwidthLimitKbps,
		task.MaxWorkers,
		task.EncryptionEnabled,
		task.HashMode,
		task.Status,
		task.DesiredState,
		task.LastError,
		task.UpdatedAt.UTC().Format(timestampLayout),
		task.ID,
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

func (r *TaskRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
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

func (r *TaskRepository) HasTasksByConnectionID(ctx context.Context, connectionID string) (bool, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM tasks WHERE connection_id = ?`, connectionID).Scan(&count); err != nil {
		return false, err
	}

	return count > 0, nil
}

func scanTask(s scanner) (domain.Task, error) {
	var task domain.Task
	var direction string
	var status string
	var createdAt string
	var updatedAt string

	err := s.Scan(
		&task.ID,
		&task.Name,
		&task.ConnectionID,
		&task.LocalPath,
		&task.RemotePath,
		&direction,
		&task.PollIntervalSec,
		&task.ConflictPolicy,
		&task.DeletePolicy,
		&task.EmptyDirPolicy,
		&task.BandwidthLimitKbps,
		&task.MaxWorkers,
		&task.EncryptionEnabled,
		&task.HashMode,
		&status,
		&task.DesiredState,
		&task.LastError,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return domain.Task{}, err
	}

	task.Direction = domain.TaskDirection(direction)
	task.Status = domain.TaskStatus(status)
	task.CreatedAt, err = parseTimestamp(createdAt)
	if err != nil {
		return domain.Task{}, err
	}
	task.UpdatedAt, err = parseTimestamp(updatedAt)
	if err != nil {
		return domain.Task{}, err
	}

	return task, nil
}

func parseTimestamp(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func mapSQLError(err error) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	switch {
	case strings.Contains(message, "FOREIGN KEY constraint failed"):
		return domain.ErrConflict
	case strings.Contains(message, "UNIQUE constraint failed"):
		return domain.ErrConflict
	default:
		return err
	}
}
