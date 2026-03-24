package scheduler

import (
	"context"
	"errors"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type taskRunner interface {
	List(context.Context) ([]domain.Task, error)
	ExecuteRunningTask(context.Context, string) error
}

type runtimeReader interface {
	GetByTaskID(context.Context, string) (domain.TaskRuntimeState, error)
}

type operationQueue interface {
	ListDue(context.Context, time.Time, int) ([]domain.OperationQueueItem, error)
	Dequeue(context.Context, string) error
	Reschedule(context.Context, domain.OperationQueueItem) error
}

type Scheduler struct {
	tasks    taskRunner
	runtime  runtimeReader
	queue    operationQueue
	interval time.Duration
}

func New(tasks taskRunner, runtime runtimeReader, queue operationQueue, interval time.Duration) *Scheduler {
	return &Scheduler{
		tasks:    tasks,
		runtime:  runtime,
		queue:    queue,
		interval: interval,
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	s.Tick(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Tick(ctx)
		}
	}
}

func (s *Scheduler) Tick(ctx context.Context) {
	items, err := s.tasks.List(ctx)
	if err != nil {
		return
	}

	now := time.Now().UTC()
	executed := make(map[string]struct{})
	for _, task := range items {
		if task.Status != domain.TaskStatusRunning {
			continue
		}

		state, err := s.runtime.GetByTaskID(ctx, task.ID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if !shouldRun(now, task, state) {
			continue
		}
		if err := s.tasks.ExecuteRunningTask(ctx, task.ID); err == nil {
			executed[task.ID] = struct{}{}
		}
	}

	if s.queue == nil {
		return
	}

	ops, err := s.queue.ListDue(ctx, now, 64)
	if err != nil {
		return
	}
	for _, op := range ops {
		if _, ok := executed[op.TaskID]; ok {
			continue
		}
		if err := s.tasks.ExecuteRunningTask(ctx, op.TaskID); err != nil {
			op.AttemptCount++
			op.LastError = err.Error()
			op.NextAttemptAt = now.Add(nextBackoff(op.AttemptCount))
			op.UpdatedAt = now
			_ = s.queue.Reschedule(ctx, op)
			continue
		}
		_ = s.queue.Dequeue(ctx, op.ID)
	}
}

func shouldRun(now time.Time, task domain.Task, state domain.TaskRuntimeState) bool {
	if state.LastReconcileAt.IsZero() {
		return true
	}
	interval := time.Duration(task.PollIntervalSec) * time.Second
	if interval <= 0 {
		interval = time.Second
	}
	return now.Sub(state.LastReconcileAt) >= interval
}

func nextBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return time.Second
	}
	backoff := time.Second << min(attempt-1, 8)
	if backoff > 5*time.Minute {
		return 5 * time.Minute
	}
	return backoff
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
