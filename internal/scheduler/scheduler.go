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
	ExecuteQueueOperation(context.Context, domain.OperationQueueItem) error
}

type runtimeReader interface {
	GetByTaskID(context.Context, string) (domain.TaskRuntimeState, error)
}

type operationQueue interface {
	ListDue(context.Context, time.Time, int) ([]domain.OperationQueueItem, error)
	Dequeue(context.Context, string) error
	Reschedule(context.Context, domain.OperationQueueItem) error
	MarkFailed(context.Context, string, string) error
}

type Scheduler struct {
	tasks    taskRunner
	runtime  runtimeReader
	queue    operationQueue
	interval time.Duration
	logger   schedulerLogger
}

type schedulerLogger interface {
	Printf(string, ...any)
}

func New(tasks taskRunner, runtime runtimeReader, queue operationQueue, interval time.Duration) *Scheduler {
	return &Scheduler{
		tasks:    tasks,
		runtime:  runtime,
		queue:    queue,
		interval: interval,
	}
}

func (s *Scheduler) SetLogger(logger schedulerLogger) {
	s.logger = logger
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
		if task.Status != domain.TaskStatusRunning && task.Status != domain.TaskStatusDegraded && task.Status != domain.TaskStatusRetrying {
			continue
		}

		state, err := s.runtime.GetByTaskID(ctx, task.ID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if !shouldRun(now, task, state) {
			continue
		}
		s.logf("scheduler task triggered task_id=%s status=%s", task.ID, task.Status)
		if err := s.tasks.ExecuteRunningTask(ctx, task.ID); err == nil {
			executed[task.ID] = struct{}{}
			s.logf("scheduler task finished task_id=%s", task.ID)
		} else {
			s.logf("scheduler task failed task_id=%s error=%v", task.ID, err)
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
		s.logf("scheduler queue triggered task_id=%s queue_op_id=%s op_type=%s", op.TaskID, op.ID, op.OpType)
		var err error
		if isActionOp(op.OpType) {
			err = s.tasks.ExecuteQueueOperation(ctx, op)
		} else {
			err = s.tasks.ExecuteRunningTask(ctx, op.TaskID)
		}
		if err != nil {
			s.logf("scheduler queue failed task_id=%s queue_op_id=%s error=%v", op.TaskID, op.ID, err)
			op.AttemptCount++
			op.LastError = err.Error()
			if op.AttemptCount >= 10 {
				_ = s.queue.MarkFailed(ctx, op.ID, err.Error())
			} else {
				op.NextAttemptAt = now.Add(nextBackoff(op.AttemptCount))
				op.UpdatedAt = now
				_ = s.queue.Reschedule(ctx, op)
			}
			continue
		}
		s.logf("scheduler queue finished task_id=%s queue_op_id=%s", op.TaskID, op.ID)
		_ = s.queue.Dequeue(ctx, op.ID)
	}
}

func (s *Scheduler) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}

func isActionOp(opType string) bool {
	switch domain.SyncActionType(opType) {
	case domain.SyncActionCreateDirLocal,
		domain.SyncActionCreateDirRemote,
		domain.SyncActionUploadFile,
		domain.SyncActionDownloadFile,
		domain.SyncActionDeleteLocal,
		domain.SyncActionDeleteRemote,
		domain.SyncActionMoveLocal,
		domain.SyncActionMoveRemote,
		domain.SyncActionMoveConflictLocal,
		domain.SyncActionMoveConflictRemote,
		domain.SyncActionRefreshMetadata:
		return true
	default:
		return false
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
