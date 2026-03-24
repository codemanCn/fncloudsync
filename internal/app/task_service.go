package app

import (
	"context"
	"fmt"
	"time"

	appcrypto "github.com/xiaoxuesen/fn-cloudsync/internal/crypto"
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
	repo           taskRepository
	connectionRepo taskConnectionRepository
	secrets        *appcrypto.SecretManager
	baselineRunner taskBaselineRunner
	runtimeRepo    taskRuntimeRepository
	failureRepo    taskFailureRepository
	queueRepo      taskOperationQueueRepository
}

type taskConnectionRepository interface {
	GetByID(context.Context, string) (domain.Connection, error)
}

type taskBaselineRunner interface {
	RunOnce(context.Context, domain.Task, domain.Connection, string) error
}

type taskRuntimeRepository interface {
	Upsert(context.Context, domain.TaskRuntimeState) error
}

type taskFailureRepository interface {
	Create(context.Context, domain.FailureRecord) error
}

type taskOperationQueueRepository interface {
	Enqueue(context.Context, domain.OperationQueueItem) error
}

func NewTaskService(repo taskRepository) *TaskService {
	return &TaskService{repo: repo}
}

func (s *TaskService) SetConnectionRepository(repo taskConnectionRepository) {
	s.connectionRepo = repo
}

func (s *TaskService) SetSecrets(secrets *appcrypto.SecretManager) {
	s.secrets = secrets
}

func (s *TaskService) SetBaselineRunner(runner taskBaselineRunner) {
	s.baselineRunner = runner
}

func (s *TaskService) SetRuntimeRepository(repo taskRuntimeRepository) {
	s.runtimeRepo = repo
}

func (s *TaskService) SetFailureRepository(repo taskFailureRepository) {
	s.failureRepo = repo
}

func (s *TaskService) SetOperationQueueRepository(repo taskOperationQueueRepository) {
	s.queueRepo = repo
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

func (s *TaskService) Start(ctx context.Context, id string) (domain.Task, error) {
	task, err := s.transitionStatus(ctx, id, domain.TaskStatusRunning)
	if err != nil {
		return domain.Task{}, err
	}
	return s.executeTask(ctx, task)
}

func (s *TaskService) Pause(ctx context.Context, id string) (domain.Task, error) {
	task, err := s.transitionStatus(ctx, id, domain.TaskStatusPaused)
	if err == nil {
		s.upsertRuntimeState(ctx, domain.TaskRuntimeState{
			TaskID:    task.ID,
			Phase:     "paused",
			UpdatedAt: time.Now().UTC(),
		})
	}
	return task, err
}

func (s *TaskService) Stop(ctx context.Context, id string) (domain.Task, error) {
	task, err := s.transitionStatus(ctx, id, domain.TaskStatusStopped)
	if err == nil {
		s.upsertRuntimeState(ctx, domain.TaskRuntimeState{
			TaskID:    task.ID,
			Phase:     "stopped",
			UpdatedAt: time.Now().UTC(),
		})
	}
	return task, err
}

func (s *TaskService) transitionStatus(ctx context.Context, id string, status domain.TaskStatus) (domain.Task, error) {
	task, err := s.GetByID(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}

	task.Status = status
	task.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, task); err != nil {
		return domain.Task{}, err
	}

	return task, nil
}

func (s *TaskService) ExecuteRunningTask(ctx context.Context, id string) error {
	task, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if task.Status != domain.TaskStatusRunning {
		return domain.ErrConflict
	}
	_, err = s.executeTask(ctx, task)
	return err
}

func (s *TaskService) executeTask(ctx context.Context, task domain.Task) (domain.Task, error) {
	s.upsertRuntimeState(ctx, domain.TaskRuntimeState{
		TaskID:    task.ID,
		Phase:     "running",
		UpdatedAt: time.Now().UTC(),
	})

	if s.connectionRepo != nil && s.secrets != nil && s.baselineRunner != nil {
		connection, err := s.connectionRepo.GetByID(ctx, task.ConnectionID)
		if err != nil {
			task.Status = domain.TaskStatusFailed
			task.LastError = err.Error()
			task.UpdatedAt = time.Now().UTC()
			_ = s.repo.Update(ctx, task)
			return domain.Task{}, err
		}

		password, err := s.secrets.DecryptString(connection.PasswordCiphertext)
		if err != nil {
			task.Status = domain.TaskStatusFailed
			task.LastError = err.Error()
			task.UpdatedAt = time.Now().UTC()
			_ = s.repo.Update(ctx, task)
			return domain.Task{}, err
		}

		if err := s.baselineRunner.RunOnce(ctx, task, connection, password); err != nil {
			task.Status = domain.TaskStatusFailed
			task.LastError = err.Error()
			task.UpdatedAt = time.Now().UTC()
			_ = s.repo.Update(ctx, task)
			s.upsertRuntimeState(ctx, domain.TaskRuntimeState{
				TaskID:      task.ID,
				Phase:       "failed",
				RetryStreak: 1,
				LastError:   err.Error(),
				UpdatedAt:   time.Now().UTC(),
			})
			s.recordFailure(ctx, task, err)
			return domain.Task{}, err
		}
	}

	s.upsertRuntimeState(ctx, domain.TaskRuntimeState{
		TaskID:          task.ID,
		Phase:           "idle",
		LastSuccessAt:   time.Now().UTC(),
		LastReconcileAt: time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	})

	return task, nil
}

func (s *TaskService) upsertRuntimeState(ctx context.Context, state domain.TaskRuntimeState) {
	if s.runtimeRepo == nil {
		return
	}
	_ = s.runtimeRepo.Upsert(ctx, state)
}

func (s *TaskService) recordFailure(ctx context.Context, task domain.Task, runErr error) {
	now := time.Now().UTC()
	if s.failureRepo != nil {
		_ = s.failureRepo.Create(ctx, domain.FailureRecord{
			ID:            fmt.Sprintf("%s-failure-%d", task.ID, now.UnixNano()),
			TaskID:        task.ID,
			Path:          task.RemotePath,
			OpType:        "baseline_sync",
			ErrorCode:     "baseline_failed",
			ErrorMessage:  runErr.Error(),
			Retryable:     true,
			FirstFailedAt: now,
			LastFailedAt:  now,
			AttemptCount:  1,
		})
	}
	if s.queueRepo != nil {
		_ = s.queueRepo.Enqueue(ctx, domain.OperationQueueItem{
			ID:           fmt.Sprintf("%s-queue-%d", task.ID, now.UnixNano()),
			TaskID:       task.ID,
			OpType:       "baseline_sync",
			TargetPath:   task.RemotePath,
			Reason:       "retry failed baseline sync",
			Status:       "pending",
			AttemptCount: 0,
			LastError:    runErr.Error(),
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}
}
