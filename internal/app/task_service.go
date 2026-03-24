package app

import (
	"context"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type taskRepository interface {
	Create(context.Context, domain.Task) error
	GetByID(context.Context, string) (domain.Task, error)
	List(context.Context) ([]domain.Task, error)
	Update(context.Context, domain.Task) error
	Delete(context.Context, string) error
}

type TaskService struct {
	repo taskRepository
}

func NewTaskService(repo taskRepository) *TaskService {
	return &TaskService{repo: repo}
}

func (s *TaskService) Create(ctx context.Context, task domain.Task) (domain.Task, error) {
	task.ApplyDefaults()
	if err := task.Validate(); err != nil {
		return domain.Task{}, err
	}
	now := time.Now().UTC()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}

	if err := s.repo.Create(ctx, task); err != nil {
		return domain.Task{}, err
	}

	return task, nil
}

func (s *TaskService) List(ctx context.Context) ([]domain.Task, error) {
	return s.repo.List(ctx)
}

func (s *TaskService) GetByID(ctx context.Context, id string) (domain.Task, error) {
	if id == "" {
		return domain.Task{}, domain.ErrInvalidArgument
	}

	return s.repo.GetByID(ctx, id)
}

func (s *TaskService) Update(ctx context.Context, task domain.Task) (domain.Task, error) {
	existing, err := s.GetByID(ctx, task.ID)
	if err != nil {
		return domain.Task{}, err
	}

	task.CreatedAt = existing.CreatedAt
	if task.Status == "" {
		task.Status = existing.Status
	}
	if task.PollIntervalSec == 0 {
		task.PollIntervalSec = existing.PollIntervalSec
	}
	if task.ConflictPolicy == "" {
		task.ConflictPolicy = existing.ConflictPolicy
	}
	if task.DeletePolicy == "" {
		task.DeletePolicy = existing.DeletePolicy
	}
	if task.EmptyDirPolicy == "" {
		task.EmptyDirPolicy = existing.EmptyDirPolicy
	}
	if task.HashMode == "" {
		task.HashMode = existing.HashMode
	}
	if task.MaxWorkers == 0 {
		task.MaxWorkers = existing.MaxWorkers
	}
	if err := task.Validate(); err != nil {
		return domain.Task{}, err
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = time.Now().UTC()
	}

	if err := s.repo.Update(ctx, task); err != nil {
		return domain.Task{}, err
	}

	return task, nil
}

func (s *TaskService) Delete(ctx context.Context, id string) error {
	if id == "" {
		return domain.ErrInvalidArgument
	}

	return s.repo.Delete(ctx, id)
}
