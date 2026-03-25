package poller

import (
	"context"
	"errors"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type taskPoller interface {
	List(context.Context) ([]domain.Task, error)
	PollRemoteTask(context.Context, string) error
}

type runtimeReader interface {
	GetByTaskID(context.Context, string) (domain.TaskRuntimeState, error)
}

type Poller struct {
	tasks    taskPoller
	runtime  runtimeReader
	interval time.Duration
	logger   pollerLogger
}

type pollerLogger interface {
	Printf(string, ...any)
}

func New(tasks taskPoller, runtime runtimeReader, interval time.Duration) *Poller {
	return &Poller{
		tasks:    tasks,
		runtime:  runtime,
		interval: interval,
	}
}

func (p *Poller) SetLogger(logger pollerLogger) {
	p.logger = logger
}

func (p *Poller) Run(ctx context.Context) {
	p.Tick(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.Tick(ctx)
		}
	}
}

func (p *Poller) Tick(ctx context.Context) {
	items, err := p.tasks.List(ctx)
	if err != nil {
		return
	}

	now := time.Now().UTC()
	for _, task := range items {
		if task.Direction == domain.TaskDirectionUpload {
			continue
		}
		if !isPollableTaskStatus(task.Status) {
			continue
		}

		state, err := p.runtime.GetByTaskID(ctx, task.ID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if !shouldPoll(now, task, state) {
			continue
		}
		p.logf("poller trigger task_id=%s direction=%s", task.ID, task.Direction)
		_ = p.tasks.PollRemoteTask(ctx, task.ID)
	}
}

func (p *Poller) logf(format string, args ...any) {
	if p.logger != nil {
		p.logger.Printf(format, args...)
	}
}

func shouldPoll(now time.Time, task domain.Task, state domain.TaskRuntimeState) bool {
	if state.LastRemoteScanAt.IsZero() {
		return true
	}
	interval := time.Duration(task.PollIntervalSec) * time.Second
	if interval <= 0 {
		interval = time.Second
	}
	return now.Sub(state.LastRemoteScanAt) >= interval
}

func isPollableTaskStatus(status domain.TaskStatus) bool {
	return status == domain.TaskStatusRunning || status == domain.TaskStatusDegraded || status == domain.TaskStatusRetrying
}
