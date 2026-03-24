package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type FileIndexRepository struct {
	db *sql.DB
}

func NewFileIndexRepository(db *sql.DB) *FileIndexRepository {
	return &FileIndexRepository{db: db}
}

func (r *FileIndexRepository) Upsert(ctx context.Context, entry domain.FileIndexEntry) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO file_index (
			id, task_id, relative_path, entry_type, local_exists, remote_exists, local_size, remote_size,
			local_mtime, remote_mtime, local_file_id, remote_etag, content_hash, last_sync_direction,
			last_sync_at, version, sync_state, conflict_flag, deleted_tombstone
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, relative_path) DO UPDATE SET
			entry_type = excluded.entry_type,
			local_exists = excluded.local_exists,
			remote_exists = excluded.remote_exists,
			local_size = excluded.local_size,
			remote_size = excluded.remote_size,
			local_mtime = excluded.local_mtime,
			remote_mtime = excluded.remote_mtime,
			local_file_id = excluded.local_file_id,
			remote_etag = excluded.remote_etag,
			content_hash = excluded.content_hash,
			last_sync_direction = excluded.last_sync_direction,
			last_sync_at = excluded.last_sync_at,
			version = excluded.version,
			sync_state = excluded.sync_state,
			conflict_flag = excluded.conflict_flag,
			deleted_tombstone = excluded.deleted_tombstone
	`,
		entry.ID,
		entry.TaskID,
		entry.RelativePath,
		entry.EntryType,
		boolToInt(entry.LocalExists),
		boolToInt(entry.RemoteExists),
		entry.LocalSize,
		entry.RemoteSize,
		formatOptionalTimestamp(entry.LocalMTime),
		formatOptionalTimestamp(entry.RemoteMTime),
		entry.LocalFileID,
		entry.RemoteETag,
		entry.ContentHash,
		entry.LastSyncDirection,
		formatOptionalTimestamp(entry.LastSyncAt),
		entry.Version,
		entry.SyncState,
		boolToInt(entry.ConflictFlag),
		boolToInt(entry.DeletedTombstone),
	)
	return mapSQLError(err)
}

func (r *FileIndexRepository) ListByTaskID(ctx context.Context, taskID string) ([]domain.FileIndexEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, relative_path, entry_type, local_exists, remote_exists, local_size, remote_size,
		       local_mtime, remote_mtime, local_file_id, remote_etag, content_hash, last_sync_direction,
		       last_sync_at, version, sync_state, conflict_flag, deleted_tombstone
		FROM file_index
		WHERE task_id = ?
		ORDER BY relative_path ASC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.FileIndexEntry
	for rows.Next() {
		item, err := scanFileIndexEntry(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *FileIndexRepository) GetByTaskIDAndPath(ctx context.Context, taskID, relativePath string) (domain.FileIndexEntry, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, relative_path, entry_type, local_exists, remote_exists, local_size, remote_size,
		       local_mtime, remote_mtime, local_file_id, remote_etag, content_hash, last_sync_direction,
		       last_sync_at, version, sync_state, conflict_flag, deleted_tombstone
		FROM file_index
		WHERE task_id = ? AND relative_path = ?
	`, taskID, relativePath)
	item, err := scanFileIndexEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FileIndexEntry{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.FileIndexEntry{}, err
	}
	return item, nil
}

type fileIndexScanner interface {
	Scan(dest ...any) error
}

func scanFileIndexEntry(row fileIndexScanner) (domain.FileIndexEntry, error) {
	var item domain.FileIndexEntry
	var localExists int
	var remoteExists int
	var localMTime string
	var remoteMTime string
	var lastSyncAt string
	var conflictFlag int
	var deletedTombstone int
	err := row.Scan(
		&item.ID,
		&item.TaskID,
		&item.RelativePath,
		&item.EntryType,
		&localExists,
		&remoteExists,
		&item.LocalSize,
		&item.RemoteSize,
		&localMTime,
		&remoteMTime,
		&item.LocalFileID,
		&item.RemoteETag,
		&item.ContentHash,
		&item.LastSyncDirection,
		&lastSyncAt,
		&item.Version,
		&item.SyncState,
		&conflictFlag,
		&deletedTombstone,
	)
	if err != nil {
		return domain.FileIndexEntry{}, err
	}
	item.LocalExists = intToBool(localExists)
	item.RemoteExists = intToBool(remoteExists)
	item.LocalMTime = parseOptionalTimestamp(localMTime)
	item.RemoteMTime = parseOptionalTimestamp(remoteMTime)
	item.LastSyncAt = parseOptionalTimestamp(lastSyncAt)
	item.ConflictFlag = intToBool(conflictFlag)
	item.DeletedTombstone = intToBool(deletedTombstone)
	return item, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func intToBool(value int) bool {
	return value != 0
}
