package sync

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type remoteFS interface {
	MkdirAll(context.Context, domain.Connection, string, string) error
	Upload(context.Context, domain.Connection, string, string, io.Reader, string) error
	List(context.Context, domain.Connection, string, string) ([]domain.RemoteEntry, error)
	Download(context.Context, domain.Connection, string, string) (io.ReadCloser, domain.RemoteEntry, error)
	Delete(context.Context, domain.Connection, string, string, bool) error
	Move(context.Context, domain.Connection, string, string, string) error
}

type BaselineRunner struct {
	remote    remoteFS
	fileIndex fileIndexRepository
	conflicts conflictHistoryRepository
}

type fileIndexRepository interface {
	ListByTaskID(context.Context, string) ([]domain.FileIndexEntry, error)
	Upsert(context.Context, domain.FileIndexEntry) error
}

type conflictHistoryRepository interface {
	Create(context.Context, domain.ConflictRecord) error
}

type localEntry struct {
	path  string
	isDir bool
	size  int64
	mtime time.Time
}

func NewBaselineRunner(remote remoteFS) *BaselineRunner {
	return &BaselineRunner{remote: remote}
}

func (r *BaselineRunner) SetFileIndexRepository(repo fileIndexRepository) {
	r.fileIndex = repo
}

func (r *BaselineRunner) SetConflictHistoryRepository(repo conflictHistoryRepository) {
	r.conflicts = repo
}

func (r *BaselineRunner) RunOnce(ctx context.Context, task domain.Task, connection domain.Connection, password string) error {
	switch task.Direction {
	case domain.TaskDirectionUpload:
		return r.UploadOnce(ctx, task, connection, password)
	case domain.TaskDirectionDownload:
		return r.DownloadOnce(ctx, task, connection, password)
	case domain.TaskDirectionBidirectional:
		return r.BidirectionalOnce(ctx, task, connection, password)
	default:
		return domain.ErrInvalidArgument
	}
}

func (r *BaselineRunner) UploadOnce(ctx context.Context, task domain.Task, connection domain.Connection, password string) error {
	localEntries, err := snapshotLocal(task.LocalPath)
	if err != nil {
		return err
	}
	remoteEntries := map[string]domain.RemoteEntry{}
	if task.DeletePolicy == "mirror" {
		var err error
		remoteEntries, err = r.snapshotRemote(ctx, connection, password, task.RemotePath)
		if err != nil {
			return err
		}
	}
	actions := planUpload(task, localEntries, remoteEntries)
	if err := r.executePlan(ctx, task, connection, password, actions); err != nil {
		return err
	}
	if r.fileIndex == nil {
		return nil
	}
	finalRemoteEntries, err := r.snapshotRemote(ctx, connection, password, task.RemotePath)
	if err != nil {
		return err
	}
	return r.persistFileIndex(ctx, task, localEntries, finalRemoteEntries)
}

func (r *BaselineRunner) DownloadOnce(ctx context.Context, task domain.Task, connection domain.Connection, password string) error {
	localEntries, err := snapshotLocal(task.LocalPath)
	if err != nil {
		return err
	}
	remoteEntries, err := r.snapshotRemote(ctx, connection, password, task.RemotePath)
	if err != nil {
		return err
	}
	actions := planDownload(task, localEntries, remoteEntries)
	if err := r.executePlan(ctx, task, connection, password, actions); err != nil {
		return err
	}
	if r.fileIndex == nil {
		return nil
	}
	finalLocalEntries, err := snapshotLocal(task.LocalPath)
	if err != nil {
		return err
	}
	return r.persistFileIndex(ctx, task, finalLocalEntries, remoteEntries)
}

func (r *BaselineRunner) BidirectionalOnce(ctx context.Context, task domain.Task, connection domain.Connection, password string) error {
	actions, err := r.Plan(ctx, task, connection, password)
	if err != nil {
		return err
	}
	if err := r.executePlan(ctx, task, connection, password, actions); err != nil {
		return err
	}

	if r.fileIndex == nil {
		return nil
	}
	finalLocalEntries, err := snapshotLocal(task.LocalPath)
	if err != nil {
		return err
	}
	finalRemoteEntries, err := r.snapshotRemote(ctx, connection, password, task.RemotePath)
	if err != nil {
		return err
	}
	return r.persistFileIndex(ctx, task, finalLocalEntries, finalRemoteEntries)
}

func (r *BaselineRunner) snapshotRemote(ctx context.Context, connection domain.Connection, password, root string) (map[string]domain.RemoteEntry, error) {
	items := make(map[string]domain.RemoteEntry)
	if err := r.walkRemote(ctx, connection, password, root, root, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *BaselineRunner) walkRemote(ctx context.Context, connection domain.Connection, password, root, current string, items map[string]domain.RemoteEntry) error {
	entries, err := r.remote.List(ctx, connection, password, current)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		relPath := strings.TrimPrefix(strings.TrimPrefix(entry.Path, root), "/")
		if relPath == "" {
			continue
		}
		items[filepath.ToSlash(relPath)] = entry
		if entry.IsDir {
			if err := r.walkRemote(ctx, connection, password, root, entry.Path, items); err != nil {
				return err
			}
		}
	}

	return nil
}

func snapshotLocal(root string) (map[string]localEntry, error) {
	items := make(map[string]localEntry)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		record := localEntry{
			path:  path,
			isDir: entry.IsDir(),
		}
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			record.size = info.Size()
			record.mtime = info.ModTime().UTC()
		}
		items[filepath.ToSlash(relPath)] = record
		return nil
	})
	return items, err
}

func (r *BaselineRunner) deleteRemoteExtras(ctx context.Context, connection domain.Connection, password, root string, localEntries map[string]localEntry, remoteEntries map[string]domain.RemoteEntry) error {
	paths := make([]string, 0, len(remoteEntries))
	for relPath := range remoteEntries {
		if _, ok := localEntries[relPath]; ok {
			continue
		}
		paths = append(paths, relPath)
	}
	sort.Slice(paths, func(i, j int) bool {
		return strings.Count(paths[i], "/") > strings.Count(paths[j], "/")
	})

	for _, relPath := range paths {
		targetPath := joinRemotePath(root, relPath)
		if err := r.remote.Delete(ctx, connection, password, targetPath, remoteEntries[relPath].IsDir); err != nil {
			return err
		}
	}
	return nil
}

func deleteLocalExtras(root string, localEntries map[string]localEntry, remoteEntries map[string]domain.RemoteEntry) error {
	paths := make([]string, 0, len(localEntries))
	for relPath := range localEntries {
		if _, ok := remoteEntries[relPath]; ok {
			continue
		}
		paths = append(paths, relPath)
	}
	sort.Slice(paths, func(i, j int) bool {
		return strings.Count(paths[i], "/") > strings.Count(paths[j], "/")
	})

	for _, relPath := range paths {
		if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(relPath))); err != nil {
			return err
		}
	}
	return nil
}

func (r *BaselineRunner) loadPreviousIndex(ctx context.Context, taskID string) (map[string]domain.FileIndexEntry, error) {
	if r.fileIndex == nil || taskID == "" {
		return map[string]domain.FileIndexEntry{}, nil
	}
	items, err := r.fileIndex.ListByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	index := make(map[string]domain.FileIndexEntry, len(items))
	for _, item := range items {
		index[item.RelativePath] = item
	}
	return index, nil
}

func (r *BaselineRunner) persistFileIndex(ctx context.Context, task domain.Task, localEntries map[string]localEntry, remoteEntries map[string]domain.RemoteEntry) error {
	if r.fileIndex == nil || task.ID == "" {
		return nil
	}

	if localEntries == nil {
		var err error
		localEntries, err = snapshotLocal(task.LocalPath)
		if err != nil {
			return err
		}
	}
	if remoteEntries == nil {
		remoteEntries = map[string]domain.RemoteEntry{}
	}

	previousIndex, err := r.loadPreviousIndex(ctx, task.ID)
	if err != nil {
		return err
	}

	paths := make(map[string]struct{})
	for relPath := range localEntries {
		paths[relPath] = struct{}{}
	}
	for relPath := range remoteEntries {
		paths[relPath] = struct{}{}
	}
	for relPath := range previousIndex {
		paths[relPath] = struct{}{}
	}

	now := time.Now().UTC()
	for relPath := range paths {
		localEntry, hasLocal := localEntries[relPath]
		remoteEntry, hasRemote := remoteEntries[relPath]
		previous := previousIndex[relPath]
		entryType := "file"
		if (hasLocal && localEntry.isDir) || (hasRemote && remoteEntry.IsDir) {
			entryType = "dir"
		}

		item := domain.FileIndexEntry{
			ID:                fileIndexID(task.ID, relPath),
			TaskID:            task.ID,
			RelativePath:      relPath,
			EntryType:         entryType,
			LocalExists:       hasLocal,
			RemoteExists:      hasRemote,
			LastSyncDirection: string(task.Direction),
			LastSyncAt:        now,
			Version:           max(previous.Version+1, 1),
			SyncState:         syncStateForEntry(hasLocal, hasRemote, previous.DeletedTombstone, false),
			DeletedTombstone:  previous.DeletedTombstone,
		}
		if hasLocal {
			item.LocalSize = localEntry.size
			if info, err := os.Stat(localEntry.path); err == nil {
				item.LocalMTime = info.ModTime().UTC()
			}
		}
		if hasRemote {
			item.RemoteSize = remoteEntry.Size
			item.RemoteMTime = remoteEntry.MTime.UTC()
			item.RemoteETag = remoteEntry.ETag
		}
		if !hasLocal || !hasRemote {
			item.DeletedTombstone = previous.LocalExists || previous.RemoteExists
		}
		if hasLocal && hasRemote {
			item.DeletedTombstone = false
		}
		item.SyncState = syncStateForEntry(hasLocal, hasRemote, item.DeletedTombstone, false)
		if err := r.fileIndex.Upsert(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (r *BaselineRunner) uploadFile(ctx context.Context, connection domain.Connection, password, remotePath, localPath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	return r.remote.Upload(ctx, connection, password, remotePath, file, detectContentType(localPath))
}

func (r *BaselineRunner) downloadFile(ctx context.Context, connection domain.Connection, password, remotePath, localPath string) error {
	reader, _, err := r.remote.Download(ctx, connection, password, remotePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	file, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	return err
}

func (r *BaselineRunner) sameFile(ctx context.Context, connection domain.Connection, password, remotePath, localPath string) (bool, error) {
	localBody, err := os.ReadFile(localPath)
	if err != nil {
		return false, err
	}
	reader, _, err := r.remote.Download(ctx, connection, password, remotePath)
	if err != nil {
		return false, err
	}
	defer reader.Close()

	remoteBody, err := io.ReadAll(reader)
	if err != nil {
		return false, err
	}
	return string(localBody) == string(remoteBody), nil
}

func joinRemotePath(base, rel string) string {
	base = strings.TrimRight(base, "/")
	rel = strings.TrimLeft(rel, "/")
	if base == "" {
		return "/" + rel
	}
	return base + "/" + rel
}

func detectContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func conflictLocalPath(path string, version int) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	return fmt.Sprintf("%s.remote-conflict-v%d%s", base, max(version, 1), ext)
}

func conflictRemotePath(path string, version int) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	return fmt.Sprintf("%s.local-conflict-v%d%s", base, max(version, 1), ext)
}

func shouldDeleteRemoteFromIndex(task domain.Task, previous domain.FileIndexEntry) bool {
	return task.Direction == domain.TaskDirectionBidirectional &&
		task.DeletePolicy == "mirror" &&
		previous.LocalExists &&
		previous.RemoteExists &&
		!previous.DeletedTombstone
}

func shouldDeleteLocalFromIndex(task domain.Task, previous domain.FileIndexEntry) bool {
	return task.Direction == domain.TaskDirectionBidirectional &&
		task.DeletePolicy == "mirror" &&
		previous.LocalExists &&
		previous.RemoteExists &&
		!previous.DeletedTombstone
}

func syncStateForEntry(localExists, remoteExists, deletedTombstone, conflictFlag bool) string {
	switch {
	case conflictFlag:
		return "conflicted"
	case localExists && remoteExists:
		return "synced"
	case deletedTombstone:
		return "tombstoned"
	case localExists || remoteExists:
		return "pending"
	default:
		return "missing"
	}
}

func fileIndexID(taskID, relPath string) string {
	if relPath == "" {
		relPath = "."
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_")
	return fmt.Sprintf("%s-%s", taskID, replacer.Replace(relPath))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
