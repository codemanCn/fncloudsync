package poller_test

import (
	"context"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
	"github.com/xiaoxuesen/fn-cloudsync/internal/poller"
)

func TestPollerRunsDueRemoteTasksOnly(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	tasks := &stubTaskPoller{
		items: []domain.Task{
			{ID: "task-upload", Status: domain.TaskStatusRunning, Direction: domain.TaskDirectionUpload, PollIntervalSec: 1},
			{ID: "task-download", Status: domain.TaskStatusRunning, Direction: domain.TaskDirectionDownload, PollIntervalSec: 1},
			{ID: "task-bidirectional", Status: domain.TaskStatusDegraded, Direction: domain.TaskDirectionBidirectional, PollIntervalSec: 1},
			{ID: "task-paused", Status: domain.TaskStatusPaused, Direction: domain.TaskDirectionDownload, PollIntervalSec: 1},
		},
	}
	runtime := &stubRuntimeReader{
		states: map[string]domain.TaskRuntimeState{
			"task-download":      {TaskID: "task-download", LastRemoteScanAt: now.Add(-2 * time.Second)},
			"task-bidirectional": {TaskID: "task-bidirectional", LastRemoteScanAt: now.Add(-2 * time.Second)},
			"task-paused":        {TaskID: "task-paused", LastRemoteScanAt: now.Add(-2 * time.Second)},
		},
	}

	p := poller.New(tasks, runtime, 10*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	p.Run(ctx)

	if !contains(tasks.polled, "task-download") {
		t.Fatalf("polled = %v, want task-download", tasks.polled)
	}
	if !contains(tasks.polled, "task-bidirectional") {
		t.Fatalf("polled = %v, want task-bidirectional", tasks.polled)
	}
	if contains(tasks.polled, "task-upload") {
		t.Fatalf("polled = %v, should not include upload-only task", tasks.polled)
	}
	if contains(tasks.polled, "task-paused") {
		t.Fatalf("polled = %v, should not include paused task", tasks.polled)
	}
}

func TestPollerSkipsTasksWhenRemoteScanIsFresh(t *testing.T) {
	t.Parallel()

	tasks := &stubTaskPoller{
		items: []domain.Task{
			{ID: "task-download", Status: domain.TaskStatusRunning, Direction: domain.TaskDirectionDownload, PollIntervalSec: 60},
		},
	}
	runtime := &stubRuntimeReader{
		states: map[string]domain.TaskRuntimeState{
			"task-download": {TaskID: "task-download", LastRemoteScanAt: time.Now().UTC()},
		},
	}

	p := poller.New(tasks, runtime, time.Hour)
	p.Tick(context.Background())

	if len(tasks.polled) != 0 {
		t.Fatalf("polled = %v, want none for fresh remote scan", tasks.polled)
	}
}

type stubTaskPoller struct {
	items   []domain.Task
	polled  []string
	pollErr error
}

func (s *stubTaskPoller) List(context.Context) ([]domain.Task, error) {
	return s.items, nil
}

func (s *stubTaskPoller) PollRemoteTask(_ context.Context, taskID string) error {
	s.polled = append(s.polled, taskID)
	return s.pollErr
}

type stubRuntimeReader struct {
	states map[string]domain.TaskRuntimeState
}

func (s *stubRuntimeReader) GetByTaskID(_ context.Context, taskID string) (domain.TaskRuntimeState, error) {
	state, ok := s.states[taskID]
	if !ok {
		return domain.TaskRuntimeState{}, domain.ErrNotFound
	}
	return state, nil
}

func TestPollerHandlesPollRemoteTaskError(t *testing.T) {
	t.Parallel()

	tasks := &stubTaskPoller{
		items: []domain.Task{
			{ID: "task-1", Status: domain.TaskStatusRunning, Direction: domain.TaskDirectionDownload, PollIntervalSec: 1},
		},
		pollErr: context.DeadlineExceeded,
	}
	runtime := &stubRuntimeReader{
		states: map[string]domain.TaskRuntimeState{
			"task-1": {TaskID: "task-1", LastRemoteScanAt: time.Now().UTC().Add(-2 * time.Second)},
		},
	}

	p := poller.New(tasks, runtime, time.Hour)
	p.Tick(context.Background())

	if len(tasks.polled) != 1 || tasks.polled[0] != "task-1" {
		t.Fatalf("polled = %v, want task-1 attempted despite error", tasks.polled)
	}
}

func TestPollerPollsRetryingTasks(t *testing.T) {
	t.Parallel()

	tasks := &stubTaskPoller{
		items: []domain.Task{
			{ID: "task-1", Status: domain.TaskStatusRetrying, Direction: domain.TaskDirectionBidirectional, PollIntervalSec: 1},
		},
	}
	runtime := &stubRuntimeReader{
		states: map[string]domain.TaskRuntimeState{
			"task-1": {TaskID: "task-1", LastRemoteScanAt: time.Now().UTC().Add(-2 * time.Second)},
		},
	}

	p := poller.New(tasks, runtime, time.Hour)
	p.Tick(context.Background())

	if len(tasks.polled) != 1 || tasks.polled[0] != "task-1" {
		t.Fatalf("polled = %v, want task-1", tasks.polled)
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
