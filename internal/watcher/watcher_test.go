package watcher_test

import (
	"context"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
	"github.com/xiaoxuesen/fn-cloudsync/internal/watcher"
)

func TestWatcherTriggersRunningUploadTaskOnEvent(t *testing.T) {
	t.Parallel()

	backend := newFakeBackend()
	tasks := &stubTaskRunner{
		items: []domain.Task{
			{
				ID:        "task-1",
				LocalPath: "/tmp/sync",
				Direction: domain.TaskDirectionUpload,
				Status:    domain.TaskStatusRunning,
			},
		},
	}

	w := watcher.New(tasks, 20*time.Millisecond, 0)
	w.SetBackendFactory(func() (watcher.Backend, error) {
		return backend, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx)
	}()

	waitForCondition(t, time.Second, func() bool {
		return len(backend.added) == 1 && backend.added[0] == "/tmp/sync"
	})

	backend.events <- "/tmp/sync/docs/readme.txt"
	waitForCondition(t, time.Second, func() bool {
		return len(tasks.executed) == 1 && tasks.executed[0] == "task-1"
	})

	cancel()
	<-done
}

func TestWatcherIgnoresDownloadOnlyTasks(t *testing.T) {
	t.Parallel()

	backend := newFakeBackend()
	tasks := &stubTaskRunner{
		items: []domain.Task{
			{
				ID:        "task-1",
				LocalPath: "/tmp/sync",
				Direction: domain.TaskDirectionDownload,
				Status:    domain.TaskStatusRunning,
			},
		},
	}

	w := watcher.New(tasks, 20*time.Millisecond, 0)
	w.SetBackendFactory(func() (watcher.Backend, error) {
		return backend, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx)
	}()

	time.Sleep(60 * time.Millisecond)
	if len(backend.added) != 0 {
		t.Fatalf("watched paths = %v, want none", backend.added)
	}

	backend.events <- "/tmp/sync/docs/readme.txt"
	time.Sleep(60 * time.Millisecond)
	if len(tasks.executed) != 0 {
		t.Fatalf("executed = %v, want none", tasks.executed)
	}

	cancel()
	<-done
}

type stubTaskRunner struct {
	items     []domain.Task
	executed  []string
}

func (s *stubTaskRunner) List(ctx context.Context) ([]domain.Task, error) {
	return s.items, nil
}

func (s *stubTaskRunner) ExecuteRunningTask(ctx context.Context, taskID string) error {
	s.executed = append(s.executed, taskID)
	return nil
}

type fakeBackend struct {
	added   []string
	removed []string
	events  chan string
	errors  chan error
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		events: make(chan string, 8),
		errors: make(chan error, 1),
	}
}

func (f *fakeBackend) Add(path string) error {
	f.added = append(f.added, path)
	return nil
}

func (f *fakeBackend) Remove(path string) error {
	f.removed = append(f.removed, path)
	return nil
}

func (f *fakeBackend) Close() error {
	return nil
}

func (f *fakeBackend) Events() <-chan string {
	return f.events
}

func (f *fakeBackend) Errors() <-chan error {
	return f.errors
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
