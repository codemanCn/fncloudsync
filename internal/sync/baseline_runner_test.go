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

type stubRemoteFS struct {
	mkdirs    []string
	uploads   map[string]string
	listings  map[string][]domain.RemoteEntry
	downloads map[string]string
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
