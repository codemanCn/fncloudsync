package sync

import (
	"context"
	"os"
	"path/filepath"

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
		case domain.SyncActionRefreshMetadata:
			continue
		default:
			return domain.ErrInvalidArgument
		}
	}
	return nil
}

func (r *BaselineRunner) ExecuteAction(ctx context.Context, task domain.Task, connection domain.Connection, password string, action domain.SyncAction) error {
	return r.executePlan(ctx, task, connection, password, []domain.SyncAction{action})
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
			actions = append(actions,
				domain.SyncAction{
					Type:         domain.SyncActionMoveConflictLocal,
					RelativePath: relPath,
					LocalPath:    conflictLocalPath(local.path),
					RemotePath:   remote.Path,
				},
				domain.SyncAction{
					Type:         domain.SyncActionMoveConflictRemote,
					RelativePath: relPath,
					LocalPath:    local.path,
					RemotePath:   conflictRemotePath(remotePath),
				},
			)
		}
	}
	return append(actions, planBidirectional(task, localEntries, remoteEntries, previousIndex)...), nil
}
