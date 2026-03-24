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

func hasAction(actions []domain.SyncAction, actionType domain.SyncActionType, relPath string) bool {
	for _, action := range actions {
		if action.Type == actionType && action.RelativePath == relPath {
			return true
		}
	}
	return false
}
