package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

func (r *BaselineRunner) Plan(ctx context.Context, task domain.Task, connection domain.Connection, password string) ([]domain.SyncAction, error) {
	localEntries, err := snapshotLocal(task.LocalPath)
	if err != nil {
		return nil, err
	}

	switch task.Direction {
	case domain.TaskDirectionUpload:
		remoteEntries := map[string]domain.RemoteEntry{}
		if task.DeletePolicy == "mirror" {
			var err error
			remoteEntries, err = r.snapshotRemote(ctx, connection, password, task.RemotePath)
			if err != nil {
				return nil, err
			}
		}
		return planUpload(task, localEntries, remoteEntries), nil
	case domain.TaskDirectionDownload:
		remoteEntries, err := r.snapshotRemote(ctx, connection, password, task.RemotePath)
		if err != nil {
			return nil, err
		}
		return planDownload(task, localEntries, remoteEntries), nil
	case domain.TaskDirectionBidirectional:
		remoteEntries, err := r.snapshotRemote(ctx, connection, password, task.RemotePath)
		if err != nil {
			return nil, err
		}
		previousIndex, _ := r.loadPreviousIndex(ctx, task.ID)
		return r.planBidirectional(ctx, task, connection, password, localEntries, remoteEntries, previousIndex)
	default:
		return nil, domain.ErrInvalidArgument
	}
}

func (r *BaselineRunner) executePlan(ctx context.Context, task domain.Task, connection domain.Connection, password string, actions []domain.SyncAction) error {
	for _, action := range actions {
		switch action.Type {
		case domain.SyncActionCreateDirLocal:
			if err := os.MkdirAll(action.LocalPath, 0o755); err != nil {
				return err
			}
		case domain.SyncActionCreateDirRemote:
			if err := r.remote.MkdirAll(ctx, connection, password, action.RemotePath); err != nil {
				return err
			}
		case domain.SyncActionUploadFile, domain.SyncActionMoveConflictRemote:
			if err := r.uploadFile(ctx, connection, password, action.RemotePath, action.LocalPath); err != nil {
				return err
			}
		case domain.SyncActionDownloadFile, domain.SyncActionMoveConflictLocal:
			if err := r.downloadFile(ctx, connection, password, action.RemotePath, action.LocalPath); err != nil {
				return err
			}
		case domain.SyncActionDeleteLocal:
			if err := os.RemoveAll(action.LocalPath); err != nil {
				return err
			}
		case domain.SyncActionDeleteRemote:
			if err := r.remote.Delete(ctx, connection, password, action.RemotePath, action.IsDir); err != nil {
				return err
			}
		case domain.SyncActionMoveLocal:
			if err := os.MkdirAll(filepath.Dir(action.LocalPath), 0o755); err != nil {
				return err
			}
			if err := os.Rename(action.SourceLocalPath, action.LocalPath); err != nil {
				return err
			}
		case domain.SyncActionMoveRemote:
			if err := r.remote.Move(ctx, connection, password, action.SourceRemotePath, action.RemotePath); err != nil {
				return err
			}
		case domain.SyncActionRefreshMetadata:
			continue
		default:
			return domain.ErrInvalidArgument
		}
	}
	return nil
}

func (r *BaselineRunner) ExecuteAction(ctx context.Context, task domain.Task, connection domain.Connection, password string, action domain.SyncAction) error {
	if err := r.executePlan(ctx, task, connection, password, []domain.SyncAction{action}); err != nil {
		return err
	}
	return r.writeActionResult(ctx, task, action)
}

func (r *BaselineRunner) writeActionResult(ctx context.Context, task domain.Task, action domain.SyncAction) error {
	if r.fileIndex == nil || task.ID == "" || action.RelativePath == "" {
		return nil
	}

	previousIndex, err := r.loadPreviousIndex(ctx, task.ID)
	if err != nil {
		return err
	}
	previous := previousIndex[action.RelativePath]
	if action.Type == domain.SyncActionMoveLocal || action.Type == domain.SyncActionMoveRemote {
		return r.writeMoveActionResults(ctx, task, action, previousIndex)
	}
	now := time.Now().UTC()

	entry := domain.FileIndexEntry{
		ID:                fileIndexID(task.ID, action.RelativePath),
		TaskID:            task.ID,
		RelativePath:      action.RelativePath,
		EntryType:         entryTypeForAction(action),
		LocalExists:       previous.LocalExists,
		RemoteExists:      previous.RemoteExists,
		LocalSize:         previous.LocalSize,
		RemoteSize:        previous.RemoteSize,
		LocalMTime:        previous.LocalMTime,
		RemoteMTime:       previous.RemoteMTime,
		LocalFileID:       previous.LocalFileID,
		RemoteETag:        previous.RemoteETag,
		ContentHash:       previous.ContentHash,
		LastSyncDirection: string(task.Direction),
		LastSyncAt:        now,
		Version:           max(previous.Version+1, 1),
		SyncState:         previous.SyncState,
		ConflictFlag:      previous.ConflictFlag,
		DeletedTombstone:  previous.DeletedTombstone,
	}

	switch action.Type {
	case domain.SyncActionCreateDirLocal:
		entry.LocalExists = true
		entry.RemoteExists = true
		entry.EntryType = "dir"
		entry.SyncState = "synced"
		entry.ConflictFlag = false
		entry.DeletedTombstone = false
	case domain.SyncActionCreateDirRemote:
		entry.LocalExists = true
		entry.RemoteExists = true
		entry.EntryType = "dir"
		entry.SyncState = "synced"
		entry.ConflictFlag = false
		entry.DeletedTombstone = false
	case domain.SyncActionUploadFile:
		entry.LocalExists = true
		entry.RemoteExists = true
		entry.EntryType = "file"
		entry.SyncState = "synced"
		entry.ConflictFlag = false
		entry.DeletedTombstone = false
		if info, err := os.Stat(action.LocalPath); err == nil {
			entry.LocalSize = info.Size()
			entry.LocalMTime = info.ModTime().UTC()
		}
	case domain.SyncActionDownloadFile:
		entry.LocalExists = true
		entry.RemoteExists = true
		entry.EntryType = "file"
		entry.SyncState = "synced"
		entry.ConflictFlag = false
		entry.DeletedTombstone = false
		if info, err := os.Stat(action.LocalPath); err == nil {
			entry.LocalSize = info.Size()
			entry.LocalMTime = info.ModTime().UTC()
		}
	case domain.SyncActionMoveConflictRemote, domain.SyncActionMoveConflictLocal:
		entry.LocalExists = true
		entry.RemoteExists = true
		entry.EntryType = "file"
		entry.SyncState = "conflicted"
		entry.ConflictFlag = true
		entry.DeletedTombstone = false
		if info, err := os.Stat(action.LocalPath); err == nil {
			entry.LocalSize = info.Size()
			entry.LocalMTime = info.ModTime().UTC()
		}
	case domain.SyncActionDeleteLocal:
		entry.LocalExists = false
		entry.DeletedTombstone = true
	case domain.SyncActionDeleteRemote:
		entry.RemoteExists = false
		entry.DeletedTombstone = true
	case domain.SyncActionRefreshMetadata:
	default:
		return nil
	}

	entry.SyncState = syncStateForEntry(entry.LocalExists, entry.RemoteExists, entry.DeletedTombstone, entry.ConflictFlag)
	if err := r.fileIndex.Upsert(ctx, entry); err != nil {
		return err
	}
	return r.recordConflict(ctx, task, action, entry.LastSyncAt)
}

func (r *BaselineRunner) writeMoveActionResults(ctx context.Context, task domain.Task, action domain.SyncAction, previousIndex map[string]domain.FileIndexEntry) error {
	now := time.Now().UTC()
	source := previousIndex[action.SourceRelativePath]
	dest := previousIndex[action.RelativePath]

	sourceEntry := domain.FileIndexEntry{
		ID:                fileIndexID(task.ID, action.SourceRelativePath),
		TaskID:            task.ID,
		RelativePath:      action.SourceRelativePath,
		EntryType:         entryTypeForAction(action),
		LocalExists:       false,
		RemoteExists:      false,
		LocalSize:         source.LocalSize,
		RemoteSize:        source.RemoteSize,
		LocalMTime:        source.LocalMTime,
		RemoteMTime:       source.RemoteMTime,
		LocalFileID:       source.LocalFileID,
		RemoteETag:        source.RemoteETag,
		ContentHash:       source.ContentHash,
		LastSyncDirection: string(task.Direction),
		LastSyncAt:        now,
		Version:           max(source.Version+1, 1),
		SyncState:         "missing",
		ConflictFlag:      false,
		DeletedTombstone:  false,
	}

	destVersion := max(max(source.Version, dest.Version)+1, 1)
	destEntry := domain.FileIndexEntry{
		ID:                fileIndexID(task.ID, action.RelativePath),
		TaskID:            task.ID,
		RelativePath:      action.RelativePath,
		EntryType:         entryTypeForAction(action),
		LocalExists:       true,
		RemoteExists:      true,
		LocalSize:         source.LocalSize,
		RemoteSize:        source.RemoteSize,
		LocalMTime:        source.LocalMTime,
		RemoteMTime:       source.RemoteMTime,
		LocalFileID:       source.LocalFileID,
		RemoteETag:        source.RemoteETag,
		ContentHash:       source.ContentHash,
		LastSyncDirection: string(task.Direction),
		LastSyncAt:        now,
		Version:           destVersion,
		SyncState:         "synced",
		ConflictFlag:      false,
		DeletedTombstone:  false,
	}
	if info, err := os.Stat(action.LocalPath); err == nil {
		destEntry.LocalSize = info.Size()
		destEntry.LocalMTime = info.ModTime().UTC()
	}

	if err := r.fileIndex.Upsert(ctx, sourceEntry); err != nil {
		return err
	}
	return r.fileIndex.Upsert(ctx, destEntry)
}

func entryTypeForAction(action domain.SyncAction) string {
	if action.IsDir || action.Type == domain.SyncActionCreateDirLocal || action.Type == domain.SyncActionCreateDirRemote {
		return "dir"
	}
	return "file"
}

func planUpload(task domain.Task, localEntries map[string]localEntry, remoteEntries map[string]domain.RemoteEntry) []domain.SyncAction {
	var actions []domain.SyncAction
	for relPath, entry := range localEntries {
		remotePath := joinRemotePath(task.RemotePath, relPath)
		if entry.isDir {
			actions = append(actions, domain.SyncAction{
				Type:         domain.SyncActionCreateDirRemote,
				RelativePath: relPath,
				LocalPath:    entry.path,
				RemotePath:   remotePath,
				IsDir:        true,
			})
			continue
		}
		actions = append(actions, domain.SyncAction{
			Type:         domain.SyncActionUploadFile,
			RelativePath: relPath,
			LocalPath:    entry.path,
			RemotePath:   remotePath,
		})
	}

	if task.DeletePolicy == "mirror" {
		for relPath, remote := range remoteEntries {
			if _, ok := localEntries[relPath]; ok {
				continue
			}
			actions = append(actions, domain.SyncAction{
				Type:         domain.SyncActionDeleteRemote,
				RelativePath: relPath,
				RemotePath:   remote.Path,
				IsDir:        remote.IsDir,
			})
		}
	}
	return actions
}

func planDownload(task domain.Task, localEntries map[string]localEntry, remoteEntries map[string]domain.RemoteEntry) []domain.SyncAction {
	var actions []domain.SyncAction
	for relPath, entry := range remoteEntries {
		localPath := filepath.Join(task.LocalPath, filepath.FromSlash(relPath))
		if entry.IsDir {
			actions = append(actions, domain.SyncAction{
				Type:         domain.SyncActionCreateDirLocal,
				RelativePath: relPath,
				LocalPath:    localPath,
				RemotePath:   entry.Path,
				IsDir:        true,
			})
			continue
		}
		actions = append(actions, domain.SyncAction{
			Type:         domain.SyncActionDownloadFile,
			RelativePath: relPath,
			LocalPath:    localPath,
			RemotePath:   entry.Path,
		})
	}

	if task.DeletePolicy == "mirror" {
		for relPath, local := range localEntries {
			if _, ok := remoteEntries[relPath]; ok {
				continue
			}
			actions = append(actions, domain.SyncAction{
				Type:         domain.SyncActionDeleteLocal,
				RelativePath: relPath,
				LocalPath:    local.path,
				IsDir:        local.isDir,
			})
		}
	}
	return actions
}

func planBidirectional(task domain.Task, localEntries map[string]localEntry, remoteEntries map[string]domain.RemoteEntry, previousIndex map[string]domain.FileIndexEntry) []domain.SyncAction {
	var actions []domain.SyncAction
	for relPath, local := range localEntries {
		remote, ok := remoteEntries[relPath]
		remotePath := joinRemotePath(task.RemotePath, relPath)
		if local.isDir {
			actions = append(actions, domain.SyncAction{
				Type:         domain.SyncActionCreateDirRemote,
				RelativePath: relPath,
				LocalPath:    local.path,
				RemotePath:   remotePath,
				IsDir:        true,
			})
			continue
		}
		if !ok {
			if shouldDeleteLocalFromIndex(task, previousIndex[relPath]) {
				actions = append(actions, domain.SyncAction{
					Type:         domain.SyncActionDeleteLocal,
					RelativePath: relPath,
					LocalPath:    local.path,
				})
			} else {
				actions = append(actions, domain.SyncAction{
					Type:         domain.SyncActionUploadFile,
					RelativePath: relPath,
					LocalPath:    local.path,
					RemotePath:   remotePath,
				})
			}
			continue
		}
		if remote.IsDir {
			continue
		}
	}

	for relPath, remote := range remoteEntries {
		if _, ok := localEntries[relPath]; ok {
			continue
		}
		localPath := filepath.Join(task.LocalPath, filepath.FromSlash(relPath))
		if shouldDeleteRemoteFromIndex(task, previousIndex[relPath]) {
			actions = append(actions, domain.SyncAction{
				Type:         domain.SyncActionDeleteRemote,
				RelativePath: relPath,
				RemotePath:   remote.Path,
				IsDir:        remote.IsDir,
			})
			continue
		}
		if remote.IsDir {
			actions = append(actions, domain.SyncAction{
				Type:         domain.SyncActionCreateDirLocal,
				RelativePath: relPath,
				LocalPath:    localPath,
				RemotePath:   remote.Path,
				IsDir:        true,
			})
			continue
		}
		actions = append(actions, domain.SyncAction{
			Type:         domain.SyncActionDownloadFile,
			RelativePath: relPath,
			LocalPath:    localPath,
			RemotePath:   remote.Path,
		})
	}
	return actions
}

func (r *BaselineRunner) planBidirectional(ctx context.Context, task domain.Task, connection domain.Connection, password string, localEntries map[string]localEntry, remoteEntries map[string]domain.RemoteEntry, previousIndex map[string]domain.FileIndexEntry) ([]domain.SyncAction, error) {
	moveActions, consumedLocal, consumedRemote, err := r.detectBidirectionalMoves(ctx, task, connection, password, localEntries, remoteEntries, previousIndex)
	if err != nil {
		return nil, err
	}
	actions := append([]domain.SyncAction(nil), moveActions...)
	for relPath, local := range localEntries {
		if _, skip := consumedLocal[relPath]; skip {
			continue
		}
		remote, ok := remoteEntries[relPath]
		remotePath := joinRemotePath(task.RemotePath, relPath)
		if local.isDir {
			actions = append(actions, domain.SyncAction{
				Type:         domain.SyncActionCreateDirRemote,
				RelativePath: relPath,
				LocalPath:    local.path,
				RemotePath:   remotePath,
				IsDir:        true,
			})
			continue
		}
		if !ok {
			if shouldDeleteLocalFromIndex(task, previousIndex[relPath]) {
				actions = append(actions, domain.SyncAction{
					Type:         domain.SyncActionDeleteLocal,
					RelativePath: relPath,
					LocalPath:    local.path,
				})
			} else {
				actions = append(actions, domain.SyncAction{
					Type:         domain.SyncActionUploadFile,
					RelativePath: relPath,
					LocalPath:    local.path,
					RemotePath:   remotePath,
				})
			}
			continue
		}
		if remote.IsDir {
			continue
		}

		same, err := r.sameFile(ctx, connection, password, remote.Path, local.path)
		if err != nil {
			return nil, err
		}
		if same {
			continue
		}

		switch task.ConflictPolicy {
		case "prefer_remote":
			actions = append(actions, domain.SyncAction{
				Type:         domain.SyncActionDownloadFile,
				RelativePath: relPath,
				LocalPath:    local.path,
				RemotePath:   remote.Path,
			})
		case "prefer_local":
			actions = append(actions, domain.SyncAction{
				Type:         domain.SyncActionUploadFile,
				RelativePath: relPath,
				LocalPath:    local.path,
				RemotePath:   remotePath,
			})
		default:
			conflictVersion := nextConflictVersion(previousIndex[relPath])
			localConflict := conflictLocalPath(local.path, conflictVersion)
			remoteConflict := conflictRemotePath(remotePath, conflictVersion)
			actions = append(actions,
				domain.SyncAction{
					Type:         domain.SyncActionMoveConflictLocal,
					RelativePath: relPath,
					LocalPath:    localConflict,
					RemotePath:   remote.Path,
					ConflictPath: remoteConflict,
				},
				domain.SyncAction{
					Type:         domain.SyncActionMoveConflictRemote,
					RelativePath: relPath,
					LocalPath:    local.path,
					RemotePath:   remoteConflict,
					ConflictPath: localConflict,
				},
			)
		}
	}
	for relPath, remote := range remoteEntries {
		if _, skip := consumedRemote[relPath]; skip {
			continue
		}
		if _, ok := localEntries[relPath]; ok {
			continue
		}
		localPath := filepath.Join(task.LocalPath, filepath.FromSlash(relPath))
		if shouldDeleteRemoteFromIndex(task, previousIndex[relPath]) {
			actions = append(actions, domain.SyncAction{
				Type:         domain.SyncActionDeleteRemote,
				RelativePath: relPath,
				RemotePath:   remote.Path,
				IsDir:        remote.IsDir,
			})
			continue
		}
		if remote.IsDir {
			actions = append(actions, domain.SyncAction{
				Type:         domain.SyncActionCreateDirLocal,
				RelativePath: relPath,
				LocalPath:    localPath,
				RemotePath:   remote.Path,
				IsDir:        true,
			})
			continue
		}
		actions = append(actions, domain.SyncAction{
			Type:         domain.SyncActionDownloadFile,
			RelativePath: relPath,
			LocalPath:    localPath,
			RemotePath:   remote.Path,
		})
	}
	return actions, nil
}

func (r *BaselineRunner) detectBidirectionalMoves(ctx context.Context, task domain.Task, connection domain.Connection, password string, localEntries map[string]localEntry, remoteEntries map[string]domain.RemoteEntry, previousIndex map[string]domain.FileIndexEntry) ([]domain.SyncAction, map[string]struct{}, map[string]struct{}, error) {
	localConsumed := make(map[string]struct{})
	remoteConsumed := make(map[string]struct{})
	var actions []domain.SyncAction

	for oldRel, remoteOld := range remoteEntries {
		if !isMoveCandidate(previousIndex[oldRel]) {
			continue
		}
		if _, hasLocal := localEntries[oldRel]; hasLocal {
			continue
		}
		for newRel, localNew := range localEntries {
			if newRel == oldRel || localNew.isDir {
				continue
			}
			if _, existsRemote := remoteEntries[newRel]; existsRemote {
				continue
			}
			if _, used := localConsumed[newRel]; used {
				continue
			}
			same, err := r.sameFile(ctx, connection, password, remoteOld.Path, localNew.path)
			if err != nil {
				return nil, nil, nil, err
			}
			if !same {
				continue
			}
			actions = append(actions, domain.SyncAction{
				Type:               domain.SyncActionMoveRemote,
				RelativePath:       newRel,
				SourceRelativePath: oldRel,
				LocalPath:          localNew.path,
				RemotePath:         joinRemotePath(task.RemotePath, newRel),
				SourceRemotePath:   remoteOld.Path,
				IsDir:              localNew.isDir,
			})
			localConsumed[newRel] = struct{}{}
			remoteConsumed[oldRel] = struct{}{}
			break
		}
	}

	for oldRel, localOld := range localEntries {
		if !isMoveCandidate(previousIndex[oldRel]) {
			continue
		}
		if _, hasRemote := remoteEntries[oldRel]; hasRemote {
			continue
		}
		if _, used := localConsumed[oldRel]; used {
			continue
		}
		for newRel, remoteNew := range remoteEntries {
			if newRel == oldRel || remoteNew.IsDir {
				continue
			}
			if _, existsLocal := localEntries[newRel]; existsLocal {
				continue
			}
			if _, used := remoteConsumed[newRel]; used {
				continue
			}
			same, err := r.sameFile(ctx, connection, password, remoteNew.Path, localOld.path)
			if err != nil {
				return nil, nil, nil, err
			}
			if !same {
				continue
			}
			actions = append(actions, domain.SyncAction{
				Type:               domain.SyncActionMoveLocal,
				RelativePath:       newRel,
				SourceRelativePath: oldRel,
				LocalPath:          filepath.Join(task.LocalPath, filepath.FromSlash(newRel)),
				SourceLocalPath:    localOld.path,
				RemotePath:         remoteNew.Path,
				IsDir:              remoteNew.IsDir,
			})
			localConsumed[oldRel] = struct{}{}
			remoteConsumed[newRel] = struct{}{}
			break
		}
	}

	return actions, localConsumed, remoteConsumed, nil
}

func nextConflictVersion(previous domain.FileIndexEntry) int {
	if previous.Version <= 0 {
		return 1
	}
	return previous.Version + 1
}

func isMoveCandidate(previous domain.FileIndexEntry) bool {
	return previous.LocalExists && previous.RemoteExists && !previous.DeletedTombstone && !previous.ConflictFlag && previous.SyncState == "synced"
}

func (r *BaselineRunner) recordConflict(ctx context.Context, task domain.Task, action domain.SyncAction, detectedAt time.Time) error {
	if r.conflicts == nil || action.Type != domain.SyncActionMoveConflictRemote {
		return nil
	}
	return r.conflicts.Create(ctx, domain.ConflictRecord{
		ID:                 fmt.Sprintf("%s-conflict-%s-%d", task.ID, action.RelativePath, detectedAt.UnixNano()),
		TaskID:             task.ID,
		RelativePath:       action.RelativePath,
		LocalConflictPath:  action.ConflictPath,
		RemoteConflictPath: action.RemotePath,
		Policy:             task.ConflictPolicy,
		DetectedAt:         detectedAt,
	})
}
