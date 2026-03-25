package sync_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
	appsync "github.com/xiaoxuesen/fn-cloudsync/internal/sync"
)

func TestUploadBaselineSyncCreatesDirectoriesAndUploadsFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "readme.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(readme) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "report.txt"), []byte("payload"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(report) error = %v", err)
	}

	connector := &stubRemoteFS{}
	runner := appsync.NewBaselineRunner(connector)

	err := runner.UploadOnce(context.Background(), domain.Task{
		LocalPath:  root,
		RemotePath: "/remote",
	}, domain.Connection{}, "top-secret")
	if err != nil {
		t.Fatalf("UploadOnce() error = %v", err)
	}

	if len(connector.mkdirs) == 0 {
		t.Fatal("MkdirAll() was not called")
	}
	if len(connector.uploads) != 2 {
		t.Fatalf("len(uploads) = %d, want 2", len(connector.uploads))
	}
	if connector.uploads["/remote/report.txt"] != "payload" {
		t.Fatalf("uploaded /remote/report.txt = %q, want %q", connector.uploads["/remote/report.txt"], "payload")
	}
	if connector.uploads["/remote/docs/readme.txt"] != "hello" {
		t.Fatalf("uploaded /remote/docs/readme.txt = %q, want %q", connector.uploads["/remote/docs/readme.txt"], "hello")
	}
}

func TestDownloadBaselineSyncCreatesLocalDirectoriesAndFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	connector := &stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/docs", IsDir: true, Exists: true},
				{Path: "/remote/report.txt", IsDir: false, Exists: true},
			},
			"/remote/docs": {
				{Path: "/remote/docs/readme.txt", IsDir: false, Exists: true},
			},
		},
		downloads: map[string]string{
			"/remote/report.txt":      "payload",
			"/remote/docs/readme.txt": "hello",
		},
	}
	runner := appsync.NewBaselineRunner(connector)

	err := runner.RunOnce(context.Background(), domain.Task{
		LocalPath:  root,
		RemotePath: "/remote",
		Direction:  domain.TaskDirectionDownload,
	}, domain.Connection{}, "top-secret")
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	report, err := os.ReadFile(filepath.Join(root, "report.txt"))
	if err != nil {
		t.Fatalf("os.ReadFile(report) error = %v", err)
	}
	if string(report) != "payload" {
		t.Fatalf("report.txt = %q, want %q", string(report), "payload")
	}
	readme, err := os.ReadFile(filepath.Join(root, "docs", "readme.txt"))
	if err != nil {
		t.Fatalf("os.ReadFile(readme) error = %v", err)
	}
	if string(readme) != "hello" {
		t.Fatalf("docs/readme.txt = %q, want %q", string(readme), "hello")
	}
}

func TestBidirectionalBaselineSyncUploadsLocalAndDownloadsRemote(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "local.txt"), []byte("local"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(local) error = %v", err)
	}

	connector := &stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/remote.txt", IsDir: false, Exists: true},
			},
		},
		downloads: map[string]string{
			"/remote/remote.txt": "remote",
		},
	}
	runner := appsync.NewBaselineRunner(connector)

	err := runner.RunOnce(context.Background(), domain.Task{
		LocalPath:  root,
		RemotePath: "/remote",
		Direction:  domain.TaskDirectionBidirectional,
	}, domain.Connection{}, "top-secret")
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if connector.uploads["/remote/local.txt"] != "local" {
		t.Fatalf("uploaded /remote/local.txt = %q, want %q", connector.uploads["/remote/local.txt"], "local")
	}
	remote, err := os.ReadFile(filepath.Join(root, "remote.txt"))
	if err != nil {
		t.Fatalf("os.ReadFile(remote) error = %v", err)
	}
	if string(remote) != "remote" {
		t.Fatalf("remote.txt = %q, want %q", string(remote), "remote")
	}
}

func TestUploadBaselineSyncMirrorDeletesRemoteExtras(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(keep) error = %v", err)
	}

	connector := &stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/keep.txt", Exists: true},
				{Path: "/remote/old.txt", Exists: true},
			},
		},
	}
	runner := appsync.NewBaselineRunner(connector)

	err := runner.UploadOnce(context.Background(), domain.Task{
		LocalPath:    root,
		RemotePath:   "/remote",
		DeletePolicy: "mirror",
	}, domain.Connection{}, "top-secret")
	if err != nil {
		t.Fatalf("UploadOnce() error = %v", err)
	}

	if len(connector.deletes) != 1 || connector.deletes[0] != "/remote/old.txt" {
		t.Fatalf("deletes = %v, want /remote/old.txt", connector.deletes)
	}
}

func TestDownloadBaselineSyncMirrorDeletesLocalExtras(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "old.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(old) error = %v", err)
	}

	connector := &stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/keep.txt", IsDir: false, Exists: true},
			},
		},
		downloads: map[string]string{
			"/remote/keep.txt": "keep",
		},
	}
	runner := appsync.NewBaselineRunner(connector)

	err := runner.DownloadOnce(context.Background(), domain.Task{
		LocalPath:    root,
		RemotePath:   "/remote",
		DeletePolicy: "mirror",
	}, domain.Connection{}, "top-secret")
	if err != nil {
		t.Fatalf("DownloadOnce() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("old.txt stat error = %v, want not exist", err)
	}
}

func TestBidirectionalBaselineSyncPreferRemoteOverwritesConflicts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("local"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(shared) error = %v", err)
	}

	connector := &stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/shared.txt", IsDir: false, Exists: true},
			},
		},
		downloads: map[string]string{
			"/remote/shared.txt": "remote",
		},
	}
	runner := appsync.NewBaselineRunner(connector)

	err := runner.RunOnce(context.Background(), domain.Task{
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		ConflictPolicy: "prefer_remote",
	}, domain.Connection{}, "top-secret")
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	body, err := os.ReadFile(filepath.Join(root, "shared.txt"))
	if err != nil {
		t.Fatalf("os.ReadFile(shared) error = %v", err)
	}
	if string(body) != "remote" {
		t.Fatalf("shared.txt = %q, want %q", string(body), "remote")
	}
}

func TestBidirectionalBaselineSyncPreferLocalUploadsConflicts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("local"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(shared) error = %v", err)
	}

	connector := &stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/shared.txt", IsDir: false, Exists: true},
			},
		},
		downloads: map[string]string{
			"/remote/shared.txt": "remote",
		},
	}
	runner := appsync.NewBaselineRunner(connector)

	err := runner.RunOnce(context.Background(), domain.Task{
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		ConflictPolicy: "prefer_local",
	}, domain.Connection{}, "top-secret")
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if connector.uploads["/remote/shared.txt"] != "local" {
		t.Fatalf("uploaded shared conflict = %q, want %q", connector.uploads["/remote/shared.txt"], "local")
	}
}

func TestBidirectionalMirrorDeletesRemoteWhenIndexedLocalDeletionDetected(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	indexRepo := &stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{
				TaskID:       "task-1",
				RelativePath: "gone.txt",
				LocalExists:  true,
				RemoteExists: true,
				SyncState:    "synced",
				Version:      1,
			},
		},
	}
	connector := &stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/gone.txt", Exists: true},
			},
		},
		downloads: map[string]string{
			"/remote/gone.txt": "remote",
		},
	}
	runner := appsync.NewBaselineRunner(connector)
	runner.SetFileIndexRepository(indexRepo)

	err := runner.RunOnce(context.Background(), domain.Task{
		ID:             "task-1",
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		DeletePolicy:   "mirror",
		ConflictPolicy: "prefer_local",
	}, domain.Connection{}, "top-secret")
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if len(connector.deletes) != 1 || connector.deletes[0] != "/remote/gone.txt" {
		t.Fatalf("deletes = %v, want /remote/gone.txt", connector.deletes)
	}
}

func TestBidirectionalMirrorDoesNotDeleteWithoutPriorFileIndexEvidence(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	connector := &stubRemoteFS{
		listings: map[string][]domain.RemoteEntry{
			"/remote": {
				{Path: "/remote/new-remote.txt", Exists: true},
			},
		},
		downloads: map[string]string{
			"/remote/new-remote.txt": "remote",
		},
	}
	runner := appsync.NewBaselineRunner(connector)
	runner.SetFileIndexRepository(&stubFileIndexRepo{})

	err := runner.RunOnce(context.Background(), domain.Task{
		ID:             "task-1",
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		DeletePolicy:   "mirror",
		ConflictPolicy: "prefer_local",
	}, domain.Connection{}, "top-secret")
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if len(connector.deletes) != 0 {
		t.Fatalf("deletes = %v, want none", connector.deletes)
	}
	body, err := os.ReadFile(filepath.Join(root, "new-remote.txt"))
	if err != nil {
		t.Fatalf("os.ReadFile(new-remote) error = %v", err)
	}
	if string(body) != "remote" {
		t.Fatalf("new-remote.txt = %q, want %q", string(body), "remote")
	}
}

func TestExecuteActionUploadWritesFileIndexEntry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	localPath := filepath.Join(root, "report.txt")
	if err := os.WriteFile(localPath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	indexRepo := &stubFileIndexRepo{}
	runner := appsync.NewBaselineRunner(&stubRemoteFS{})
	runner.SetFileIndexRepository(indexRepo)

	err := runner.ExecuteAction(context.Background(), domain.Task{
		ID:         "task-1",
		LocalPath:  root,
		RemotePath: "/remote",
		Direction:  domain.TaskDirectionUpload,
	}, domain.Connection{}, "top-secret", domain.SyncAction{
		Type:         domain.SyncActionUploadFile,
		RelativePath: "report.txt",
		LocalPath:    localPath,
		RemotePath:   "/remote/report.txt",
	})
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}

	if len(indexRepo.upserts) == 0 {
		t.Fatal("file index upserts = 0, want action writeback")
	}
	last := indexRepo.upserts[len(indexRepo.upserts)-1]
	if !last.LocalExists || !last.RemoteExists {
		t.Fatalf("file index exists flags = local:%v remote:%v, want true/true", last.LocalExists, last.RemoteExists)
	}
	if got, want := last.RelativePath, "report.txt"; got != want {
		t.Fatalf("RelativePath = %q, want %q", got, want)
	}
}

func TestExecuteActionDeleteRemoteWritesTombstone(t *testing.T) {
	t.Parallel()

	indexRepo := &stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{TaskID: "task-1", RelativePath: "old.txt", LocalExists: true, RemoteExists: true, Version: 2},
		},
	}
	connector := &stubRemoteFS{}
	runner := appsync.NewBaselineRunner(connector)
	runner.SetFileIndexRepository(indexRepo)

	err := runner.ExecuteAction(context.Background(), domain.Task{
		ID:         "task-1",
		LocalPath:  t.TempDir(),
		RemotePath: "/remote",
		Direction:  domain.TaskDirectionBidirectional,
	}, domain.Connection{}, "top-secret", domain.SyncAction{
		Type:         domain.SyncActionDeleteRemote,
		RelativePath: "old.txt",
		RemotePath:   "/remote/old.txt",
	})
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}

	last := indexRepo.upserts[len(indexRepo.upserts)-1]
	if last.RemoteExists {
		t.Fatalf("RemoteExists = %v, want false", last.RemoteExists)
	}
	if !last.DeletedTombstone {
		t.Fatal("DeletedTombstone = false, want true")
	}
	if got, want := last.SyncState, "tombstoned"; got != want {
		t.Fatalf("SyncState = %q, want %q", got, want)
	}
	if got, want := last.Version, 3; got != want {
		t.Fatalf("Version = %d, want %d", got, want)
	}
}

func TestExecuteActionUploadClearsTombstoneOnRecovery(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	localPath := filepath.Join(root, "old.txt")
	if err := os.WriteFile(localPath, []byte("restored"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	indexRepo := &stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{TaskID: "task-1", RelativePath: "old.txt", LocalExists: true, RemoteExists: false, DeletedTombstone: true, Version: 3},
		},
	}
	runner := appsync.NewBaselineRunner(&stubRemoteFS{})
	runner.SetFileIndexRepository(indexRepo)

	err := runner.ExecuteAction(context.Background(), domain.Task{
		ID:         "task-1",
		LocalPath:  root,
		RemotePath: "/remote",
		Direction:  domain.TaskDirectionBidirectional,
	}, domain.Connection{}, "top-secret", domain.SyncAction{
		Type:         domain.SyncActionUploadFile,
		RelativePath: "old.txt",
		LocalPath:    localPath,
		RemotePath:   "/remote/old.txt",
	})
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}

	last := indexRepo.upserts[len(indexRepo.upserts)-1]
	if last.DeletedTombstone {
		t.Fatal("DeletedTombstone = true, want false after recovery upload")
	}
	if got, want := last.SyncState, "synced"; got != want {
		t.Fatalf("SyncState = %q, want %q", got, want)
	}
	if got, want := last.Version, 4; got != want {
		t.Fatalf("Version = %d, want %d", got, want)
	}
}

func TestExecuteActionDownloadClearsTombstoneOnRemoteRecovery(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	localPath := filepath.Join(root, "old.txt")
	indexRepo := &stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{TaskID: "task-1", RelativePath: "old.txt", LocalExists: false, RemoteExists: true, DeletedTombstone: true, Version: 3},
		},
	}
	runner := appsync.NewBaselineRunner(&stubRemoteFS{
		downloads: map[string]string{
			"/remote/old.txt": "restored",
		},
	})
	runner.SetFileIndexRepository(indexRepo)

	err := runner.ExecuteAction(context.Background(), domain.Task{
		ID:         "task-1",
		LocalPath:  root,
		RemotePath: "/remote",
		Direction:  domain.TaskDirectionBidirectional,
	}, domain.Connection{}, "top-secret", domain.SyncAction{
		Type:         domain.SyncActionDownloadFile,
		RelativePath: "old.txt",
		LocalPath:    localPath,
		RemotePath:   "/remote/old.txt",
	})
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}

	body, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(body) != "restored" {
		t.Fatalf("old.txt = %q, want %q", string(body), "restored")
	}

	last := indexRepo.upserts[len(indexRepo.upserts)-1]
	if last.DeletedTombstone {
		t.Fatal("DeletedTombstone = true, want false after recovery download")
	}
	if got, want := last.SyncState, "synced"; got != want {
		t.Fatalf("SyncState = %q, want %q", got, want)
	}
	if got, want := last.Version, 4; got != want {
		t.Fatalf("Version = %d, want %d", got, want)
	}
}

func TestExecuteActionMoveConflictRemoteMarksConflictAndStoresHistory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("local"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	indexRepo := &stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{TaskID: "task-1", RelativePath: "shared.txt", LocalExists: true, RemoteExists: true, Version: 7},
		},
	}
	conflictRepo := &stubConflictHistoryRepo{}
	connector := &stubRemoteFS{}
	runner := appsync.NewBaselineRunner(connector)
	runner.SetFileIndexRepository(indexRepo)
	runner.SetConflictHistoryRepository(conflictRepo)

	err := runner.ExecuteAction(context.Background(), domain.Task{
		ID:             "task-1",
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		ConflictPolicy: "keep_both",
	}, domain.Connection{}, "top-secret", domain.SyncAction{
		Type:         domain.SyncActionMoveConflictRemote,
		RelativePath: "shared.txt",
		LocalPath:    filepath.Join(root, "shared.txt"),
		RemotePath:   "/remote/shared.local-conflict-v8.txt",
		ConflictPath: filepath.Join(root, "shared.remote-conflict-v8.txt"),
	})
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}

	last := indexRepo.upserts[len(indexRepo.upserts)-1]
	if !last.ConflictFlag {
		t.Fatal("ConflictFlag = false, want true")
	}
	if got, want := last.SyncState, "conflicted"; got != want {
		t.Fatalf("SyncState = %q, want %q", got, want)
	}
	if got, want := last.Version, 8; got != want {
		t.Fatalf("Version = %d, want %d", got, want)
	}
	if len(conflictRepo.records) != 1 {
		t.Fatalf("len(conflict records) = %d, want 1", len(conflictRepo.records))
	}
	if got, want := conflictRepo.records[0].LocalConflictPath, filepath.Join(root, "shared.remote-conflict-v8.txt"); got != want {
		t.Fatalf("LocalConflictPath = %q, want %q", got, want)
	}
}

func TestExecuteActionUploadClearsConflictFlagAfterRecovery(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	localPath := filepath.Join(root, "shared.txt")
	if err := os.WriteFile(localPath, []byte("resolved"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	indexRepo := &stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{
				TaskID:           "task-1",
				RelativePath:     "shared.txt",
				LocalExists:      true,
				RemoteExists:     true,
				Version:          8,
				ConflictFlag:     true,
				SyncState:        "conflicted",
				DeletedTombstone: false,
			},
		},
	}
	runner := appsync.NewBaselineRunner(&stubRemoteFS{})
	runner.SetFileIndexRepository(indexRepo)

	err := runner.ExecuteAction(context.Background(), domain.Task{
		ID:         "task-1",
		LocalPath:  root,
		RemotePath: "/remote",
		Direction:  domain.TaskDirectionBidirectional,
	}, domain.Connection{}, "top-secret", domain.SyncAction{
		Type:         domain.SyncActionUploadFile,
		RelativePath: "shared.txt",
		LocalPath:    localPath,
		RemotePath:   "/remote/shared.txt",
	})
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}

	last := indexRepo.upserts[len(indexRepo.upserts)-1]
	if last.ConflictFlag {
		t.Fatal("ConflictFlag = true, want false after recovery upload")
	}
	if got, want := last.SyncState, "synced"; got != want {
		t.Fatalf("SyncState = %q, want %q", got, want)
	}
	if got, want := last.Version, 9; got != want {
		t.Fatalf("Version = %d, want %d", got, want)
	}
}

func TestExecuteActionDeleteRemotePersistsRepeatedDeleteRecoveryCycle(t *testing.T) {
	t.Parallel()

	indexRepo := &stubFileIndexRepo{
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
	}
	runner := appsync.NewBaselineRunner(&stubRemoteFS{})
	runner.SetFileIndexRepository(indexRepo)

	err := runner.ExecuteAction(context.Background(), domain.Task{
		ID:         "task-1",
		LocalPath:  t.TempDir(),
		RemotePath: "/remote",
		Direction:  domain.TaskDirectionBidirectional,
	}, domain.Connection{}, "top-secret", domain.SyncAction{
		Type:         domain.SyncActionDeleteRemote,
		RelativePath: "gone.txt",
		RemotePath:   "/remote/gone.txt",
	})
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}

	last := indexRepo.upserts[len(indexRepo.upserts)-1]
	if !last.DeletedTombstone {
		t.Fatal("DeletedTombstone = false, want true on repeated delete cycle")
	}
	if got, want := last.SyncState, "tombstoned"; got != want {
		t.Fatalf("SyncState = %q, want %q", got, want)
	}
	if got, want := last.Version, 5; got != want {
		t.Fatalf("Version = %d, want %d", got, want)
	}
}

func TestExecuteActionMoveConflictRemotePersistsRepeatedConflictAfterRecovery(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	localPath := filepath.Join(root, "shared.txt")
	if err := os.WriteFile(localPath, []byte("local"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	indexRepo := &stubFileIndexRepo{
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
	}
	conflictRepo := &stubConflictHistoryRepo{}
	runner := appsync.NewBaselineRunner(&stubRemoteFS{})
	runner.SetFileIndexRepository(indexRepo)
	runner.SetConflictHistoryRepository(conflictRepo)

	err := runner.ExecuteAction(context.Background(), domain.Task{
		ID:             "task-1",
		LocalPath:      root,
		RemotePath:     "/remote",
		Direction:      domain.TaskDirectionBidirectional,
		ConflictPolicy: "keep_both",
	}, domain.Connection{}, "top-secret", domain.SyncAction{
		Type:         domain.SyncActionMoveConflictRemote,
		RelativePath: "shared.txt",
		LocalPath:    localPath,
		RemotePath:   "/remote/shared.local-conflict-v10.txt",
		ConflictPath: filepath.Join(root, "shared.remote-conflict-v10.txt"),
	})
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}

	last := indexRepo.upserts[len(indexRepo.upserts)-1]
	if !last.ConflictFlag {
		t.Fatal("ConflictFlag = false, want true on repeated conflict cycle")
	}
	if got, want := last.SyncState, "conflicted"; got != want {
		t.Fatalf("SyncState = %q, want %q", got, want)
	}
	if got, want := last.Version, 10; got != want {
		t.Fatalf("Version = %d, want %d", got, want)
	}
	if len(conflictRepo.records) != 1 {
		t.Fatalf("len(conflict records) = %d, want 1", len(conflictRepo.records))
	}
	if got, want := conflictRepo.records[0].RemoteConflictPath, "/remote/shared.local-conflict-v10.txt"; got != want {
		t.Fatalf("RemoteConflictPath = %q, want %q", got, want)
	}
}

func TestExecuteActionMoveRemoteRenamesRemoteAndUpdatesFileIndex(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	localPath := filepath.Join(root, "renamed.txt")
	if err := os.WriteFile(localPath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	connector := &stubRemoteFS{}
	indexRepo := &stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{
				TaskID:       "task-1",
				RelativePath: "old.txt",
				LocalExists:  true,
				RemoteExists: true,
				LocalSize:    int64(len("payload")),
				RemoteSize:   int64(len("payload")),
				Version:      3,
				SyncState:    "synced",
			},
		},
	}
	runner := appsync.NewBaselineRunner(connector)
	runner.SetFileIndexRepository(indexRepo)

	err := runner.ExecuteAction(context.Background(), domain.Task{
		ID:         "task-1",
		LocalPath:  root,
		RemotePath: "/remote",
		Direction:  domain.TaskDirectionBidirectional,
	}, domain.Connection{}, "top-secret", domain.SyncAction{
		Type:               domain.SyncActionMoveRemote,
		RelativePath:       "renamed.txt",
		SourceRelativePath: "old.txt",
		LocalPath:          localPath,
		RemotePath:         "/remote/renamed.txt",
		SourceRemotePath:   "/remote/old.txt",
	})
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}
	if len(connector.moves) != 1 || connector.moves[0] != [2]string{"/remote/old.txt", "/remote/renamed.txt"} {
		t.Fatalf("moves = %v, want remote rename", connector.moves)
	}
	if len(indexRepo.upserts) != 2 {
		t.Fatalf("upserts = %d, want source+dest", len(indexRepo.upserts))
	}
	if got, want := indexRepo.upserts[0].SyncState, "missing"; got != want {
		t.Fatalf("source SyncState = %q, want %q", got, want)
	}
	if got, want := indexRepo.upserts[1].RelativePath, "renamed.txt"; got != want {
		t.Fatalf("dest RelativePath = %q, want %q", got, want)
	}
	if got, want := indexRepo.upserts[1].SyncState, "synced"; got != want {
		t.Fatalf("dest SyncState = %q, want %q", got, want)
	}
}

func TestExecuteActionMoveLocalRenamesLocalFileAndUpdatesFileIndex(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	oldPath := filepath.Join(root, "old.txt")
	newPath := filepath.Join(root, "renamed.txt")
	if err := os.WriteFile(oldPath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	indexRepo := &stubFileIndexRepo{
		items: []domain.FileIndexEntry{
			{
				TaskID:       "task-1",
				RelativePath: "old.txt",
				LocalExists:  true,
				RemoteExists: true,
				LocalSize:    int64(len("payload")),
				RemoteSize:   int64(len("payload")),
				Version:      5,
				SyncState:    "synced",
			},
		},
	}
	runner := appsync.NewBaselineRunner(&stubRemoteFS{})
	runner.SetFileIndexRepository(indexRepo)

	err := runner.ExecuteAction(context.Background(), domain.Task{
		ID:         "task-1",
		LocalPath:  root,
		RemotePath: "/remote",
		Direction:  domain.TaskDirectionBidirectional,
	}, domain.Connection{}, "top-secret", domain.SyncAction{
		Type:               domain.SyncActionMoveLocal,
		RelativePath:       "renamed.txt",
		SourceRelativePath: "old.txt",
		LocalPath:          newPath,
		SourceLocalPath:    oldPath,
		RemotePath:         "/remote/renamed.txt",
	})
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old path stat error = %v, want moved away", err)
	}
	body, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(body) != "payload" {
		t.Fatalf("renamed.txt = %q, want %q", string(body), "payload")
	}
	if len(indexRepo.upserts) != 2 {
		t.Fatalf("upserts = %d, want source+dest", len(indexRepo.upserts))
	}
	if got, want := indexRepo.upserts[1].RelativePath, "renamed.txt"; got != want {
		t.Fatalf("dest RelativePath = %q, want %q", got, want)
	}
}

type stubRemoteFS struct {
	mkdirs    []string
	uploads   map[string]string
	listings  map[string][]domain.RemoteEntry
	downloads map[string]string
	deletes   []string
	moves     [][2]string
}

type stubFileIndexRepo struct {
	items   []domain.FileIndexEntry
	upserts []domain.FileIndexEntry
}

type stubConflictHistoryRepo struct {
	records []domain.ConflictRecord
}

func (s *stubRemoteFS) MkdirAll(_ context.Context, _ domain.Connection, _ string, targetPath string) error {
	s.mkdirs = append(s.mkdirs, targetPath)
	return nil
}

func (s *stubRemoteFS) Upload(_ context.Context, _ domain.Connection, _ string, targetPath string, reader io.Reader, _ string) error {
	if s.uploads == nil {
		s.uploads = make(map[string]string)
	}
	body, _ := io.ReadAll(reader)
	s.uploads[targetPath] = string(body)
	return nil
}

func (s *stubRemoteFS) List(_ context.Context, _ domain.Connection, _ string, targetPath string) ([]domain.RemoteEntry, error) {
	return s.listings[targetPath], nil
}

func (s *stubRemoteFS) Download(_ context.Context, _ domain.Connection, _ string, targetPath string) (io.ReadCloser, domain.RemoteEntry, error) {
	value := s.downloads[targetPath]
	return io.NopCloser(strings.NewReader(value)), domain.RemoteEntry{Path: targetPath, Exists: true}, nil
}

func (s *stubRemoteFS) Delete(_ context.Context, _ domain.Connection, _ string, targetPath string, _ bool) error {
	s.deletes = append(s.deletes, targetPath)
	return nil
}

func (s *stubRemoteFS) Move(_ context.Context, _ domain.Connection, _ string, srcPath string, dstPath string) error {
	s.moves = append(s.moves, [2]string{srcPath, dstPath})
	return nil
}

func (s *stubFileIndexRepo) ListByTaskID(_ context.Context, _ string) ([]domain.FileIndexEntry, error) {
	return append([]domain.FileIndexEntry(nil), s.items...), nil
}

func (s *stubFileIndexRepo) Upsert(_ context.Context, item domain.FileIndexEntry) error {
	s.upserts = append(s.upserts, item)
	return nil
}

func (s *stubConflictHistoryRepo) Create(_ context.Context, record domain.ConflictRecord) error {
	s.records = append(s.records, record)
	return nil
}
