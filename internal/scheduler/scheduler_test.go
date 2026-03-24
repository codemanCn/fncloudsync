package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
	"github.com/xiaoxuesen/fn-cloudsync/internal/scheduler"
)

func TestSchedulerRunsDueRunningTasks(t *testing.T) {
	t.Parallel()

	tasks := &stubTaskRunner{
		items: []domain.Task{
			{ID: "task-1", Status: domain.TaskStatusRunning, PollIntervalSec: 1},
			{ID: "task-2", Status: domain.TaskStatusPaused, PollIntervalSec: 1},
		},
	}
	runtime := &stubRuntimeReader{
		states: map[string]domain.TaskRuntimeState{
			"task-1": {TaskID: "task-1", LastReconcileAt: time.Now().UTC().Add(-2 * time.Second)},
		},
	}

	queue := &stubOperationQueue{}
	s := scheduler.New(tasks, runtime, queue, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	s.Run(ctx)

	if len(tasks.executed) == 0 || tasks.executed[0] != "task-1" {
		t.Fatalf("executed = %v, want task-1", tasks.executed)
	}
}

func TestSchedulerConsumesDueQueueItems(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	tasks := &stubTaskRunner{}
	runtime := &stubRuntimeReader{}
	queue := &stubOperationQueue{
		items: []domain.OperationQueueItem{
			{ID: "op-1", TaskID: "task-queued", OpType: string(domain.SyncActionUploadFile), NextAttemptAt: now.Add(-time.Second)},
		},
	}

	s := scheduler.New(tasks, runtime, queue, time.Hour)
	s.Tick(context.Background())

	if len(tasks.executedQueue) != 1 || tasks.executedQueue[0] != "op-1" {
		t.Fatalf("executed queue = %v, want op-1", tasks.executedQueue)
	}
	if len(queue.dequeued) != 1 || queue.dequeued[0] != "op-1" {
		t.Fatalf("dequeued = %v, want op-1", queue.dequeued)
	}
}

func TestSchedulerReschedulesFailedQueueItems(t *testing.T) {
	t.Parallel()

	tasks := &stubTaskRunner{executeErr: context.DeadlineExceeded}
	runtime := &stubRuntimeReader{}
	queue := &stubOperationQueue{
		items: []domain.OperationQueueItem{
			{ID: "op-1", TaskID: "task-queued"},
		},
	}

	s := scheduler.New(tasks, runtime, queue, time.Hour)
	s.Tick(context.Background())

	if len(queue.rescheduled) != 1 {
		t.Fatalf("rescheduled = %d, want 1", len(queue.rescheduled))
	}
	if queue.rescheduled[0].AttemptCount != 1 {
		t.Fatalf("AttemptCount = %d, want 1", queue.rescheduled[0].AttemptCount)
	}
	if queue.rescheduled[0].NextAttemptAt.IsZero() {
		t.Fatal("NextAttemptAt is zero, want backoff timestamp")
	}
}

type stubTaskRunner struct {
	items       []domain.Task
	executed    []string
	executedQueue []string
	executeErr  error
}

func (s *stubTaskRunner) List(ctx context.Context) ([]domain.Task, error) {
	return s.items, nil
}

func (s *stubTaskRunner) ExecuteRunningTask(ctx context.Context, taskID string) error {
	s.executed = append(s.executed, taskID)
	return s.executeErr
}

func (s *stubTaskRunner) ExecuteQueueOperation(ctx context.Context, item domain.OperationQueueItem) error {
	s.executedQueue = append(s.executedQueue, item.ID)
	return s.executeErr
}

type stubRuntimeReader struct {
	states map[string]domain.TaskRuntimeState
}

func (s *stubRuntimeReader) GetByTaskID(ctx context.Context, taskID string) (domain.TaskRuntimeState, error) {
	state, ok := s.states[taskID]
	if !ok {
		return domain.TaskRuntimeState{}, domain.ErrNotFound
	}
	return state, nil
}

type stubOperationQueue struct {
	items        []domain.OperationQueueItem
	dequeued     []string
	rescheduled  []domain.OperationQueueItem
}

func (s *stubOperationQueue) ListDue(ctx context.Context, now time.Time, limit int) ([]domain.OperationQueueItem, error) {
	if len(s.items) > limit {
		return s.items[:limit], nil
	}
	return s.items, nil
}

func (s *stubOperationQueue) Dequeue(ctx context.Context, id string) error {
	s.dequeued = append(s.dequeued, id)
	return nil
}

func (s *stubOperationQueue) Reschedule(ctx context.Context, item domain.OperationQueueItem) error {
	s.rescheduled = append(s.rescheduled, item)
	return nil
}
