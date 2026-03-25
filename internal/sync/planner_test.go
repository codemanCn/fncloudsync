package sync_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
	appsync "github.com/xiaoxuesen/fn-cloudsync/internal/sync"
)

func TestPlannerUploadReturnsStandardActions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "readme.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	runner := appsync.NewBaselineRunner(&stubRemoteFS{})
	actions, err := runner.Plan(context.Background(), domain.Task{
		LocalPath:  root,
		RemotePath: "/remote",
		Direction:  domain.TaskDirectionUpload,
	}, domain.Connection{}, "secret")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if !hasAction(actions, domain.SyncActionCreateDirRemote, "docs") {
		t.Fatalf("actions = %+v, want CreateDirRemote docs", actions)
	}
	if !hasAction(actions, domain.SyncActionUploadFile, "docs/readme.txt") {
		t.Fatalf("actions = %+v, want UploadFile docs/readme.txt", actions)
	}
}

func TestPlannerBidirectionalSkipsSameFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("same"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	connector := &stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/shared.txt", Exists: true},
			},
		},
		downloads: map[string]string{
			"/remote/shared.txt": "same",
		},
	}
	runner := appsync.NewBaselineRunner(connector)

	actions, err := runner.Plan(context.Background(), domain.Task{
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		ConflictPolicy: "prefer_local",
	}, domain.Connection{}, "secret")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("actions = %+v, want none for same file", actions)
	}
}

func TestPlannerKeepBothUsesStableConflictSuffixFromFileIndexVersion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("local"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	connector := &stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/shared.txt", Exists: true},
			},
		},
		downloads: map[string]string{
			"/remote/shared.txt": "remote",
		},
	}
	indexRepo := &stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{TaskID: "task-1", RelativePath: "shared.txt", Version: 7},
		},
	}
	runner := appsync.NewBaselineRunner(connector)
	runner.SetFileIndexRepository(indexRepo)

	actions, err := runner.Plan(context.Background(), domain.Task{
		ID:             "task-1",
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		ConflictPolicy: "keep_both",
	}, domain.Connection{}, "secret")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if !hasActionPath(actions, domain.SyncActionMoveConflictLocal, filepath.Join(root, "shared.remote-conflict-v8.txt")) {
		t.Fatalf("actions = %+v, want stable local conflict path", actions)
	}
	if !hasActionPath(actions, domain.SyncActionMoveConflictRemote, "/remote/shared.local-conflict-v8.txt") {
		t.Fatalf("actions = %+v, want stable remote conflict path", actions)
	}
}

func TestPlannerBidirectionalTreatsTombstonedRemoteAbsenceAsRecoveryUpload(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "old.txt"), []byte("restored"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	runner := appsync.NewBaselineRunner(&stubRemoteFS{})
	runner.SetFileIndexRepository(&stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{TaskID: "task-1", RelativePath: "old.txt", LocalExists: true, RemoteExists: false, DeletedTombstone: true, Version: 3},
		},
	})

	actions, err := runner.Plan(context.Background(), domain.Task{
		ID:             "task-1",
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		DeletePolicy:   "mirror",
		ConflictPolicy: "prefer_local",
	}, domain.Connection{}, "secret")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if !hasAction(actions, domain.SyncActionUploadFile, "old.txt") {
		t.Fatalf("actions = %+v, want UploadFile old.txt for recovery", actions)
	}
	if hasAction(actions, domain.SyncActionDeleteLocal, "old.txt") {
		t.Fatalf("actions = %+v, should not repeat DeleteLocal for tombstoned recovery", actions)
	}
}

func TestPlannerBidirectionalTreatsTombstonedLocalAbsenceAsRecoveryDownload(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runner := appsync.NewBaselineRunner(&stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/old.txt", Exists: true},
			},
		},
		downloads: map[string]string{
			"/remote/old.txt": "restored",
		},
	})
	runner.SetFileIndexRepository(&stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{TaskID: "task-1", RelativePath: "old.txt", LocalExists: false, RemoteExists: true, DeletedTombstone: true, Version: 3},
		},
	})

	actions, err := runner.Plan(context.Background(), domain.Task{
		ID:             "task-1",
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		DeletePolicy:   "mirror",
		ConflictPolicy: "prefer_remote",
	}, domain.Connection{}, "secret")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if !hasAction(actions, domain.SyncActionDownloadFile, "old.txt") {
		t.Fatalf("actions = %+v, want DownloadFile old.txt for remote recovery", actions)
	}
	if hasAction(actions, domain.SyncActionDeleteRemote, "old.txt") {
		t.Fatalf("actions = %+v, should not repeat DeleteRemote for tombstoned remote recovery", actions)
	}
}

func TestPlannerBidirectionalDeletesRemoteAgainAfterRecoveryCycle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runner := appsync.NewBaselineRunner(&stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/gone.txt", Exists: true},
			},
		},
	})
	runner.SetFileIndexRepository(&stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{
				TaskID:           "task-1",
				RelativePath:     "gone.txt",
				LocalExists:      true,
				RemoteExists:     true,
				DeletedTombstone: false,
				Version:          4,
				SyncState:        "synced",
			},
		},
	})

	actions, err := runner.Plan(context.Background(), domain.Task{
		ID:             "task-1",
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		DeletePolicy:   "mirror",
		ConflictPolicy: "prefer_local",
	}, domain.Connection{}, "secret")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if !hasAction(actions, domain.SyncActionDeleteRemote, "gone.txt") {
		t.Fatalf("actions = %+v, want DeleteRemote gone.txt after recovery cycle", actions)
	}
}

func TestPlannerKeepBothUsesNextConflictVersionAfterRecoveryCycle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("local"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	connector := &stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/shared.txt", Exists: true},
			},
		},
		downloads: map[string]string{
			"/remote/shared.txt": "remote",
		},
	}
	runner := appsync.NewBaselineRunner(connector)
	runner.SetFileIndexRepository(&stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{
				TaskID:           "task-1",
				RelativePath:     "shared.txt",
				LocalExists:      true,
				RemoteExists:     true,
				ConflictFlag:     false,
				DeletedTombstone: false,
				Version:          9,
				SyncState:        "synced",
			},
		},
	})

	actions, err := runner.Plan(context.Background(), domain.Task{
		ID:             "task-1",
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		ConflictPolicy: "keep_both",
	}, domain.Connection{}, "secret")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if !hasActionPath(actions, domain.SyncActionMoveConflictLocal, filepath.Join(root, "shared.remote-conflict-v10.txt")) {
		t.Fatalf("actions = %+v, want next local conflict path after recovery cycle", actions)
	}
	if !hasActionPath(actions, domain.SyncActionMoveConflictRemote, "/remote/shared.local-conflict-v10.txt") {
		t.Fatalf("actions = %+v, want next remote conflict path after recovery cycle", actions)
	}
}

func TestPlannerBidirectionalDetectsLocalRenameAsRemoteMove(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	renamedPath := filepath.Join(root, "renamed.txt")
	if err := os.WriteFile(renamedPath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	runner := appsync.NewBaselineRunner(&stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/old.txt", Exists: true},
			},
		},
		downloads: map[string]string{
			"/remote/old.txt": "payload",
		},
	})
	runner.SetFileIndexRepository(&stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{
				TaskID:       "task-1",
				RelativePath: "old.txt",
				LocalExists:  true,
				RemoteExists: true,
				LocalSize:    int64(len("payload")),
				RemoteSize:   int64(len("payload")),
				SyncState:    "synced",
				Version:      3,
			},
		},
	})

	actions, err := runner.Plan(context.Background(), domain.Task{
		ID:             "task-1",
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		DeletePolicy:   "mirror",
		ConflictPolicy: "prefer_local",
	}, domain.Connection{}, "secret")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if !hasAction(actions, domain.SyncActionMoveRemote, "renamed.txt") {
		t.Fatalf("actions = %+v, want MoveRemote renamed.txt", actions)
	}
	if hasAction(actions, domain.SyncActionUploadFile, "renamed.txt") || hasAction(actions, domain.SyncActionDeleteRemote, "old.txt") {
		t.Fatalf("actions = %+v, should avoid upload+delete for rename", actions)
	}
}

func TestPlannerBidirectionalDetectsRemoteRenameAsLocalMove(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	oldLocalPath := filepath.Join(root, "old.txt")
	if err := os.WriteFile(oldLocalPath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	runner := appsync.NewBaselineRunner(&stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/renamed.txt", Exists: true, Size: int64(len("payload")), ETag: `"etag-1"`},
			},
		},
		downloads: map[string]string{
			"/remote/renamed.txt": "payload",
		},
	})
	runner.SetFileIndexRepository(&stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{
				TaskID:       "task-1",
				RelativePath: "old.txt",
				LocalExists:  true,
				RemoteExists: true,
				LocalSize:    int64(len("payload")),
				RemoteSize:   int64(len("payload")),
				RemoteETag:   `"etag-1"`,
				SyncState:    "synced",
				Version:      5,
			},
		},
	})

	actions, err := runner.Plan(context.Background(), domain.Task{
		ID:             "task-1",
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		DeletePolicy:   "mirror",
		ConflictPolicy: "prefer_remote",
	}, domain.Connection{}, "secret")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if !hasAction(actions, domain.SyncActionMoveLocal, "renamed.txt") {
		t.Fatalf("actions = %+v, want MoveLocal renamed.txt", actions)
	}
	if hasAction(actions, domain.SyncActionDownloadFile, "renamed.txt") || hasAction(actions, domain.SyncActionDeleteLocal, "old.txt") {
		t.Fatalf("actions = %+v, should avoid download+delete for rename", actions)
	}
}

func TestPlannerPreferLocalOverwritesWithoutConflictSuffix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("local"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	connector := &stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {{Path: "/remote/file.txt", Exists: true}},
		},
		downloads: map[string]string{"/remote/file.txt": "remote"},
	}
	runner := appsync.NewBaselineRunner(connector)

	actions, err := runner.Plan(context.Background(), domain.Task{
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		ConflictPolicy: "prefer_local",
	}, domain.Connection{}, "secret")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if !hasAction(actions, domain.SyncActionUploadFile, "file.txt") {
		t.Fatalf("actions = %+v, want UploadFile file.txt", actions)
	}
	if hasAction(actions, domain.SyncActionMoveConflictLocal, "file.txt") {
		t.Fatalf("actions = %+v, should not create conflict suffix for prefer_local", actions)
	}
}

func TestPlannerPreferRemoteOverwritesWithoutConflictSuffix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("local"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	connector := &stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {{Path: "/remote/file.txt", Exists: true}},
		},
		downloads: map[string]string{"/remote/file.txt": "remote"},
	}
	runner := appsync.NewBaselineRunner(connector)

	actions, err := runner.Plan(context.Background(), domain.Task{
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		ConflictPolicy: "prefer_remote",
	}, domain.Connection{}, "secret")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if !hasAction(actions, domain.SyncActionDownloadFile, "file.txt") {
		t.Fatalf("actions = %+v, want DownloadFile file.txt", actions)
	}
	if hasAction(actions, domain.SyncActionMoveConflictRemote, "file.txt") {
		t.Fatalf("actions = %+v, should not create conflict suffix for prefer_remote", actions)
	}
}

func hasAction(actions []domain.SyncAction, actionType domain.SyncActionType, relPath string) bool {
	for _, action := range actions {
		if action.Type == actionType && action.RelativePath == relPath {
			return true
		}
	}
	return false
}

func hasActionPath(actions []domain.SyncAction, actionType domain.SyncActionType, path string) bool {
	for _, action := range actions {
		if action.Type != actionType {
			continue
		}
		if action.LocalPath == path || action.RemotePath == path {
			return true
		}
	}
	return false
}
