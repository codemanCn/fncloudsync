package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
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
	fileIndexRepo  taskFileIndexRepository
	eventRepo      taskEventRepository
	conflictRepo   taskConflictHistoryRepository
	logger         taskLogger
}

var errActionExecutionDegraded = errors.New("action execution degraded")

type taskConnectionRepository interface {
	GetByID(context.Context, string) (domain.Connection, error)
}

type taskLogger interface {
	Printf(string, ...any)
}

type taskBaselineRunner interface {
	RunOnce(context.Context, domain.Task, domain.Connection, string) error
	Plan(context.Context, domain.Task, domain.Connection, string) ([]domain.SyncAction, error)
	ExecuteAction(context.Context, domain.Task, domain.Connection, string, domain.SyncAction) error
}

type taskRuntimeRepository interface {
	Upsert(context.Context, domain.TaskRuntimeState) error
	GetByTaskID(context.Context, string) (domain.TaskRuntimeState, error)
}

type taskFailureRepository interface {
	Create(context.Context, domain.FailureRecord) error
	ListByTaskID(context.Context, string) ([]domain.FailureRecord, error)
	GetByID(context.Context, string) (domain.FailureRecord, error)
	Resolve(context.Context, string, time.Time) error
}

type taskOperationQueueRepository interface {
	Enqueue(context.Context, domain.OperationQueueItem) error
	ListByTaskID(context.Context, string) ([]domain.OperationQueueItem, error)
	Dequeue(context.Context, string) error
	Reschedule(context.Context, domain.OperationQueueItem) error
	ResetRetryableByTaskID(context.Context, string) (int, error)
	MarkFailed(context.Context, string, string) error
}

type taskFileIndexRepository interface {
	GetByTaskIDAndPath(context.Context, string, string) (domain.FileIndexEntry, error)
}

type taskEventRepository interface {
	Create(context.Context, domain.TaskEvent) error
	ListByTaskID(context.Context, string, int) ([]domain.TaskEvent, error)
}

type taskConflictHistoryRepository interface {
	ListByTaskID(context.Context, string) ([]domain.ConflictRecord, error)
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

func (s *TaskService) SetFileIndexRepository(repo taskFileIndexRepository) {
	s.fileIndexRepo = repo
}

func (s *TaskService) SetEventRepository(repo taskEventRepository) {
	s.eventRepo = repo
}

func (s *TaskService) SetConflictHistoryRepository(repo taskConflictHistoryRepository) {
	s.conflictRepo = repo
}

func (s *TaskService) SetLogger(logger taskLogger) {
	s.logger = logger
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

func (s *TaskService) ListFailures(ctx context.Context, taskID string) ([]domain.FailureRecord, error) {
	if taskID == "" {
		return nil, domain.ErrInvalidArgument
	}
	if _, err := s.GetByID(ctx, taskID); err != nil {
		return nil, err
	}
	if s.failureRepo == nil {
		return []domain.FailureRecord{}, nil
	}
	return s.failureRepo.ListByTaskID(ctx, taskID)
}

func (s *TaskService) RetryFailures(ctx context.Context, taskID string) (int, error) {
	if taskID == "" {
		return 0, domain.ErrInvalidArgument
	}
	if _, err := s.GetByID(ctx, taskID); err != nil {
		return 0, err
	}
	if s.queueRepo == nil {
		return 0, nil
	}
	return s.queueRepo.ResetRetryableByTaskID(ctx, taskID)
}

func (s *TaskService) RetryFailureByID(ctx context.Context, taskID, failureID string) (int, error) {
	if taskID == "" || failureID == "" {
		return 0, domain.ErrInvalidArgument
	}
	if _, err := s.GetByID(ctx, taskID); err != nil {
		return 0, err
	}
	if s.failureRepo == nil || s.queueRepo == nil {
		return 0, nil
	}
	record, err := s.failureRepo.GetByID(ctx, failureID)
	if err != nil {
		return 0, err
	}
	if record.TaskID != taskID {
		return 0, domain.ErrConflict
	}
	items, err := s.queueRepo.ListByTaskID(ctx, taskID)
	if err != nil {
		return 0, err
	}
	count := 0
	now := time.Now().UTC()
	for _, item := range items {
		if !isRetryableQueueMatch(record, item) {
			continue
		}
		item.Status = "queued"
		item.NextAttemptAt = time.Time{}
		item.UpdatedAt = now
		if err := s.queueRepo.Reschedule(ctx, item); err != nil {
			return count, err
		}
		count++
	}
	if count > 0 {
		if err := s.failureRepo.Resolve(ctx, failureID, now); err != nil {
			return count, err
		}
	}
	return count, nil
}

func (s *TaskService) GetRuntimeView(ctx context.Context, taskID string) (domain.TaskRuntimeView, error) {
	if taskID == "" {
		return domain.TaskRuntimeView{}, domain.ErrInvalidArgument
	}
	task, err := s.GetByID(ctx, taskID)
	if err != nil {
		return domain.TaskRuntimeView{}, err
	}

	view := domain.TaskRuntimeView{Task: task}

	if s.runtimeRepo != nil {
		runtime, err := s.runtimeRepo.GetByTaskID(ctx, taskID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return domain.TaskRuntimeView{}, err
		}
		if err == nil {
			view.Runtime = runtime
		}
	}

	if s.queueRepo != nil {
		items, err := s.queueRepo.ListByTaskID(ctx, taskID)
		if err != nil {
			return domain.TaskRuntimeView{}, err
		}
		view.Queue = items
		view.QueueSummary = summarizeQueue(items)
	}

	if s.failureRepo != nil {
		items, err := s.failureRepo.ListByTaskID(ctx, taskID)
		if err != nil {
			return domain.TaskRuntimeView{}, err
		}
		view.Failures = items
		view.FailureSummary = summarizeFailures(items)
	}

	return view, nil
}

func (s *TaskService) GetMetrics(ctx context.Context) (domain.TaskMetrics, error) {
	tasks, err := s.List(ctx)
	if err != nil {
		return domain.TaskMetrics{}, err
	}

	metrics := domain.TaskMetrics{
		TaskStates: make(map[string]int),
	}
	for _, task := range tasks {
		metrics.TaskStates[string(task.Status)]++

		if s.queueRepo != nil {
			items, err := s.queueRepo.ListByTaskID(ctx, task.ID)
			if err != nil {
				return domain.TaskMetrics{}, err
			}
			summary := summarizeQueue(items)
			metrics.Queue.Total += summary.Total
			metrics.Queue.Queued += summary.Queued
			metrics.Queue.Executing += summary.Executing
			metrics.Queue.RetryWait += summary.RetryWait
			metrics.Queue.Succeeded += summary.Succeeded
			metrics.Queue.Failed += summary.Failed
		}

		if s.failureRepo != nil {
			items, err := s.failureRepo.ListByTaskID(ctx, task.ID)
			if err != nil {
				return domain.TaskMetrics{}, err
			}
			for _, item := range items {
				metrics.Failures.Total++
				if item.ResolvedAt.IsZero() {
					metrics.Failures.Open++
					if item.Retryable {
						metrics.Failures.RetryableOpen++
					}
					continue
				}
				metrics.Failures.Resolved++
			}
		}
	}

	return metrics, nil
}

func (s *TaskService) ListEvents(ctx context.Context, taskID string, limit int) ([]domain.TaskEvent, error) {
	if taskID == "" {
		return nil, domain.ErrInvalidArgument
	}
	if _, err := s.GetByID(ctx, taskID); err != nil {
		return nil, err
	}
	if s.eventRepo == nil {
		return []domain.TaskEvent{}, nil
	}
	return s.eventRepo.ListByTaskID(ctx, taskID, limit)
}

func (s *TaskService) ListConflicts(ctx context.Context, taskID string) ([]domain.ConflictRecord, error) {
	if taskID == "" {
		return nil, domain.ErrInvalidArgument
	}
	if _, err := s.GetByID(ctx, taskID); err != nil {
		return nil, err
	}
	if s.conflictRepo == nil {
		return []domain.ConflictRecord{}, nil
	}
	return s.conflictRepo.ListByTaskID(ctx, taskID)
}

func (s *TaskService) Start(ctx context.Context, id string) (domain.Task, error) {
	task, err := s.transitionStatus(ctx, id, domain.TaskStatusRunning)
	if err != nil {
		return domain.Task{}, err
	}
	s.recordTaskEvent(ctx, task.ID, "task_started", "info", "task started", map[string]any{"status": task.Status})
	return s.executeTask(ctx, task)
}

func (s *TaskService) Pause(ctx context.Context, id string) (domain.Task, error) {
	task, err := s.transitionStatus(ctx, id, domain.TaskStatusPaused)
	if err == nil {
		s.recordTaskEvent(ctx, task.ID, "task_paused", "info", "task paused", map[string]any{"status": task.Status})
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
		s.recordTaskEvent(ctx, task.ID, "task_stopped", "info", "task stopped", map[string]any{"status": task.Status})
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
	s.recordTaskEvent(ctx, task.ID, "task_status_changed", "info", fmt.Sprintf("task status changed to %s", status), map[string]any{
		"status": status,
	})

	return task, nil
}

func (s *TaskService) ExecuteRunningTask(ctx context.Context, id string) error {
	task, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if !isSchedulableTaskStatus(task.Status) {
		return domain.ErrConflict
	}
	s.logf("task execution started task_id=%s status=%s direction=%s", task.ID, task.Status, task.Direction)
	_, err = s.executeTask(ctx, task)
	if err != nil {
		s.logf("task execution failed task_id=%s error=%v", task.ID, err)
		return err
	}
	s.logf("task execution finished task_id=%s status=%s", task.ID, task.Status)
	return err
}

func (s *TaskService) PollRemoteTask(ctx context.Context, id string) error {
	task, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if !isSchedulableTaskStatus(task.Status) {
		return domain.ErrConflict
	}
	if task.Direction == domain.TaskDirectionUpload {
		return domain.ErrConflict
	}

	now := time.Now().UTC()
	state := domain.TaskRuntimeState{
		TaskID:         task.ID,
		Phase:          "polling_remote",
		CheckpointJSON: fmt.Sprintf(`{"remote":{"trigger":"poller","polled_at":"%s"}}`, now.Format(time.RFC3339Nano)),
		UpdatedAt:      now,
	}
	if s.runtimeRepo != nil {
		existing, err := s.runtimeRepo.GetByTaskID(ctx, task.ID)
		if err == nil {
			state.LastLocalScanAt = existing.LastLocalScanAt
			state.LastRemoteScanAt = existing.LastRemoteScanAt
			state.LastReconcileAt = existing.LastReconcileAt
			state.LastSuccessAt = existing.LastSuccessAt
			state.BackoffUntil = existing.BackoffUntil
			state.RetryStreak = existing.RetryStreak
			state.LastError = existing.LastError
		}
	}
	s.upsertRuntimeState(ctx, state)
	s.recordTaskEvent(ctx, task.ID, "remote_poll_started", "info", "remote poll triggered", map[string]any{
		"checkpoint": state.CheckpointJSON,
	})
	s.logf("remote poll started task_id=%s direction=%s", task.ID, task.Direction)

	_, err = s.executeTask(ctx, task)
	return err
}

func (s *TaskService) ExecuteQueueOperation(ctx context.Context, item domain.OperationQueueItem) error {
	task, connection, password, err := s.resolveTaskExecution(ctx, item.TaskID)
	if err != nil {
		return err
	}
	s.logf("queue operation started task_id=%s queue_op_id=%s op_type=%s", item.TaskID, item.ID, item.OpType)
	return s.executeQueueOperationResolved(ctx, task, connection, password, item)
}

func (s *TaskService) executeQueueOperationResolved(ctx context.Context, task domain.Task, connection domain.Connection, password string, item domain.OperationQueueItem) error {
	var action domain.SyncAction
	if err := json.Unmarshal([]byte(item.PayloadJSON), &action); err != nil {
		return err
	}
	if s.shouldSkipActionExecution(ctx, item, action) {
		return nil
	}
	if s.queueRepo != nil {
		item.Status = "executing"
		item.UpdatedAt = time.Now().UTC()
		_ = s.queueRepo.Reschedule(ctx, item)
	}
	if err := s.baselineRunner.ExecuteAction(ctx, task, connection, password, action); err != nil {
		s.recordActionFailure(ctx, task, item, action, err)
		s.logf("queue action failed task_id=%s queue_op_id=%s op_type=%s path=%s error=%v", task.ID, item.ID, item.OpType, action.RelativePath, err)
		return err
	}
	s.logf("queue action succeeded task_id=%s queue_op_id=%s op_type=%s path=%s", task.ID, item.ID, item.OpType, action.RelativePath)
	return nil
}

func (s *TaskService) executeTask(ctx context.Context, task domain.Task) (domain.Task, error) {
	s.upsertRuntimeState(ctx, domain.TaskRuntimeState{
		TaskID:    task.ID,
		Phase:     "running",
		UpdatedAt: time.Now().UTC(),
	})

	if s.connectionRepo != nil && s.secrets != nil && s.baselineRunner != nil {
		connection, password, err := s.resolveConnection(ctx, task)
		if err != nil {
			task.Status = domain.TaskStatusFailed
			task.LastError = err.Error()
			task.UpdatedAt = time.Now().UTC()
			_ = s.repo.Update(ctx, task)
			return domain.Task{}, err
		}

		var existingRuntime domain.TaskRuntimeState
		if s.runtimeRepo != nil {
			existingRuntime, _ = s.runtimeRepo.GetByTaskID(ctx, task.ID)
		}
		if err := s.executePlannedActions(ctx, task, connection, password); err != nil {
			if errors.Is(err, errActionExecutionDegraded) {
				newStreak := existingRuntime.RetryStreak + 1
				if newStreak > 1 {
					task.Status = domain.TaskStatusRetrying
				} else {
					task.Status = domain.TaskStatusDegraded
				}
				task.LastError = err.Error()
				task.UpdatedAt = time.Now().UTC()
				_ = s.repo.Update(ctx, task)
				s.upsertRuntimeState(ctx, domain.TaskRuntimeState{
					TaskID:      task.ID,
					Phase:       "degraded",
					RetryStreak: newStreak,
					LastError:   err.Error(),
					UpdatedAt:   time.Now().UTC(),
				})
				return task, nil
			}
			task.Status = domain.TaskStatusFailed
			task.LastError = err.Error()
			task.UpdatedAt = time.Now().UTC()
			_ = s.repo.Update(ctx, task)
			s.upsertRuntimeState(ctx, domain.TaskRuntimeState{
				TaskID:      task.ID,
				Phase:       "failed",
				RetryStreak: existingRuntime.RetryStreak + 1,
				LastError:   err.Error(),
				UpdatedAt:   time.Now().UTC(),
			})
			s.recordFailure(ctx, task, err)
			return domain.Task{}, err
		}
	}

	if task.Status == domain.TaskStatusDegraded {
		task.Status = domain.TaskStatusRunning
		task.LastError = ""
		task.UpdatedAt = time.Now().UTC()
		_ = s.repo.Update(ctx, task)
	}

	s.upsertRuntimeState(ctx, domain.TaskRuntimeState{
		TaskID:           task.ID,
		Phase:            "idle",
		RetryStreak:      0,
		LastError:        "",
		LastLocalScanAt:  scanTimestampForLocal(task.Direction),
		LastRemoteScanAt: scanTimestampForRemote(task.Direction),
		LastSuccessAt:    time.Now().UTC(),
		LastReconcileAt:  time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	})

	return task, nil
}

func (s *TaskService) executePlannedActions(ctx context.Context, task domain.Task, connection domain.Connection, password string) error {
	if s.queueRepo == nil {
		return s.baselineRunner.RunOnce(ctx, task, connection, password)
	}

	actions, err := s.baselineRunner.Plan(ctx, task, connection, password)
	if err != nil {
		return err
	}
	s.logf("task planned actions task_id=%s planned_actions=%d direction=%s", task.ID, len(actions), task.Direction)
	s.updateRemoteCheckpointPaths(ctx, task.ID, actions)
	now := time.Now().UTC()
	for index, action := range actions {
		payload, err := json.Marshal(action)
		if err != nil {
			return err
		}
		if err := s.queueRepo.Enqueue(ctx, domain.OperationQueueItem{
			ID:           fmt.Sprintf("%s-action-%d-%d", task.ID, now.UnixNano(), index),
			TaskID:       task.ID,
			OpType:       string(action.Type),
			TargetPath:   action.RemotePath,
			SrcSide:      actionSourceSide(task.Direction, action.Type),
			Reason:       "planned sync action",
			PayloadJSON:  string(payload),
			Status:       "queued",
			AttemptCount: 0,
			CreatedAt:    now,
			UpdatedAt:    now,
		}); err != nil {
			return err
		}
		s.logf("queue operation enqueued task_id=%s queue_index=%d op_type=%s target_path=%s", task.ID, index, action.Type, action.RemotePath)
	}

	items, err := s.queueRepo.ListByTaskID(ctx, task.ID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !isActionQueueItem(item.OpType) {
			continue
		}
		if !isExecutableQueueStatus(item.Status) {
			continue
		}
		if err := s.executeQueueOperationResolved(ctx, task, connection, password, item); err != nil {
			s.logf("queue operation degraded task_id=%s queue_op_id=%s error=%v", task.ID, item.ID, err)
			item.AttemptCount++
			item.LastError = err.Error()
			item.NextAttemptAt = time.Now().UTC().Add(time.Second)
			item.Status = "retry_wait"
			item.UpdatedAt = time.Now().UTC()
			_ = s.queueRepo.Reschedule(ctx, item)
			return errors.Join(errActionExecutionDegraded, err)
		}
		s.logf("queue operation completed task_id=%s queue_op_id=%s", task.ID, item.ID)
		_ = s.queueRepo.Dequeue(ctx, item.ID)
	}
	return nil
}

func (s *TaskService) resolveTaskExecution(ctx context.Context, taskID string) (domain.Task, domain.Connection, string, error) {
	task, err := s.GetByID(ctx, taskID)
	if err != nil {
		return domain.Task{}, domain.Connection{}, "", err
	}
	if task.Status != domain.TaskStatusRunning {
		return domain.Task{}, domain.Connection{}, "", domain.ErrConflict
	}
	connection, password, err := s.resolveConnection(ctx, task)
	if err != nil {
		return domain.Task{}, domain.Connection{}, "", err
	}
	return task, connection, password, nil
}

func (s *TaskService) resolveConnection(ctx context.Context, task domain.Task) (domain.Connection, string, error) {
	connection, err := s.connectionRepo.GetByID(ctx, task.ConnectionID)
	if err != nil {
		return domain.Connection{}, "", err
	}
	password, err := s.secrets.DecryptString(connection.PasswordCiphertext)
	if err != nil {
		return domain.Connection{}, "", err
	}
	return connection, password, nil
}

func scanTimestampForLocal(direction domain.TaskDirection) time.Time {
	switch direction {
	case domain.TaskDirectionUpload, domain.TaskDirectionBidirectional:
		return time.Now().UTC()
	default:
		return time.Time{}
	}
}

func scanTimestampForRemote(direction domain.TaskDirection) time.Time {
	switch direction {
	case domain.TaskDirectionDownload, domain.TaskDirectionBidirectional:
		return time.Now().UTC()
	default:
		return time.Time{}
	}
}

func (s *TaskService) upsertRuntimeState(ctx context.Context, state domain.TaskRuntimeState) {
	if s.runtimeRepo == nil {
		return
	}
	if state.CheckpointJSON == "" {
		existing, err := s.runtimeRepo.GetByTaskID(ctx, state.TaskID)
		if err == nil {
			state.CheckpointJSON = existing.CheckpointJSON
		}
	}
	_ = s.runtimeRepo.Upsert(ctx, state)
}

func isSchedulableTaskStatus(status domain.TaskStatus) bool {
	return status == domain.TaskStatusRunning || status == domain.TaskStatusDegraded || status == domain.TaskStatusRetrying
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
	s.recordTaskEvent(ctx, task.ID, "task_failed", "error", runErr.Error(), map[string]any{
		"op_type": "baseline_sync",
	})
	if s.queueRepo != nil {
		_ = s.queueRepo.Enqueue(ctx, domain.OperationQueueItem{
			ID:           fmt.Sprintf("%s-queue-%d", task.ID, now.UnixNano()),
			TaskID:       task.ID,
			OpType:       "baseline_sync",
			TargetPath:   task.RemotePath,
			Reason:       "retry failed baseline sync",
			Status:       "queued",
			AttemptCount: 0,
			LastError:    runErr.Error(),
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}
}

func (s *TaskService) recordActionFailure(ctx context.Context, task domain.Task, item domain.OperationQueueItem, action domain.SyncAction, runErr error) {
	if s.failureRepo == nil {
		return
	}
	now := time.Now().UTC()
	path := action.RelativePath
	if path == "" {
		path = item.TargetPath
	}
	_ = s.failureRepo.Create(ctx, domain.FailureRecord{
		ID:            fmt.Sprintf("%s-action-failure-%d", task.ID, now.UnixNano()),
		TaskID:        task.ID,
		Path:          path,
		OpType:        item.OpType,
		ErrorCode:     "action_failed",
		ErrorMessage:  runErr.Error(),
		Retryable:     true,
		FirstFailedAt: now,
		LastFailedAt:  now,
		AttemptCount:  maxInt(item.AttemptCount+1, 1),
	})
	s.recordTaskEvent(ctx, task.ID, "action_failed", "error", runErr.Error(), map[string]any{
		"path":    path,
		"op_type": item.OpType,
	})
}

func isActionQueueItem(opType string) bool {
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

func actionSourceSide(direction domain.TaskDirection, actionType domain.SyncActionType) string {
	switch actionType {
	case domain.SyncActionUploadFile, domain.SyncActionCreateDirRemote, domain.SyncActionDeleteRemote, domain.SyncActionMoveRemote, domain.SyncActionMoveConflictRemote:
		return "local"
	case domain.SyncActionDownloadFile, domain.SyncActionCreateDirLocal, domain.SyncActionDeleteLocal, domain.SyncActionMoveLocal, domain.SyncActionMoveConflictLocal:
		return "remote"
	default:
		if direction == domain.TaskDirectionUpload {
			return "local"
		}
		return "remote"
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func summarizeQueue(items []domain.OperationQueueItem) domain.TaskQueueSummary {
	summary := domain.TaskQueueSummary{Total: len(items)}
	for _, item := range items {
		switch item.Status {
		case "queued":
			summary.Queued++
			summary.Pending++
		case "pending":
			summary.Pending++
		case "executing":
			summary.Executing++
		case "retry_wait":
			summary.RetryWait++
		case "succeeded":
			summary.Succeeded++
		case "failed":
			summary.Failed++
		}
	}
	return summary
}

func summarizeFailures(items []domain.FailureRecord) domain.TaskFailureSummary {
	summary := domain.TaskFailureSummary{Total: len(items)}
	for _, item := range items {
		if item.ResolvedAt.IsZero() {
			summary.Open++
			continue
		}
		summary.Resolved++
	}
	return summary
}

func isRetryableQueueMatch(record domain.FailureRecord, item domain.OperationQueueItem) bool {
	if item.Status != "retry_wait" && item.Status != "executing" {
		return false
	}
	if item.OpType != record.OpType {
		return false
	}
	if record.Path == "" {
		return true
	}
	return item.TargetPath == record.Path || strings.HasSuffix(item.TargetPath, "/"+record.Path)
}

func isExecutableQueueStatus(status string) bool {
	return status == "queued" || status == "pending" || status == "retry_wait"
}

func (s *TaskService) recordTaskEvent(ctx context.Context, taskID, eventType, level, message string, details map[string]any) {
	if s.eventRepo == nil || taskID == "" {
		return
	}
	detailsJSON := ""
	if len(details) > 0 {
		if raw, err := json.Marshal(details); err == nil {
			detailsJSON = string(raw)
		}
	}
	now := time.Now().UTC()
	_ = s.eventRepo.Create(ctx, domain.TaskEvent{
		ID:          fmt.Sprintf("%s-event-%d", taskID, now.UnixNano()),
		TaskID:      taskID,
		EventType:   eventType,
		Level:       level,
		Message:     message,
		DetailsJSON: detailsJSON,
		CreatedAt:   now,
	})
}

func (s *TaskService) updateRemoteCheckpointPaths(ctx context.Context, taskID string, actions []domain.SyncAction) {
	if s.runtimeRepo == nil || taskID == "" {
		return
	}
	paths := remoteDiscoveredPaths(actions)
	if len(paths) == 0 {
		return
	}
	state, err := s.runtimeRepo.GetByTaskID(ctx, taskID)
	if err != nil || state.CheckpointJSON == "" {
		return
	}

	payload := map[string]any{}
	if err := json.Unmarshal([]byte(state.CheckpointJSON), &payload); err != nil {
		return
	}
	remotePayload, _ := payload["remote"].(map[string]any)
	if remotePayload == nil {
		remotePayload = map[string]any{}
	}
	remotePayload["changed_paths"] = paths
	payload["remote"] = remotePayload

	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	state.CheckpointJSON = string(encoded)
	state.UpdatedAt = time.Now().UTC()
	s.upsertRuntimeState(ctx, state)
}

func (s *TaskService) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}

func remoteDiscoveredPaths(actions []domain.SyncAction) []string {
	seen := make(map[string]struct{})
	for _, action := range actions {
		if !isRemoteDiscoveredAction(action.Type) {
			continue
		}
		if action.RelativePath == "" {
			continue
		}
		seen[action.RelativePath] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func isRemoteDiscoveredAction(actionType domain.SyncActionType) bool {
	switch actionType {
	case domain.SyncActionCreateDirLocal,
		domain.SyncActionDownloadFile,
		domain.SyncActionDeleteLocal,
		domain.SyncActionMoveLocal,
		domain.SyncActionMoveConflictLocal:
		return true
	default:
		return false
	}
}

func (s *TaskService) shouldSkipActionExecution(ctx context.Context, item domain.OperationQueueItem, action domain.SyncAction) bool {
	if item.AttemptCount <= 0 || s.fileIndexRepo == nil || item.TaskID == "" || action.RelativePath == "" {
		return false
	}

	entry, err := s.fileIndexRepo.GetByTaskIDAndPath(ctx, item.TaskID, action.RelativePath)
	if err != nil || entry.LastSyncAt.IsZero() {
		return false
	}

	switch action.Type {
	case domain.SyncActionCreateDirLocal, domain.SyncActionCreateDirRemote:
		return entry.EntryType == "dir" && entry.LocalExists && entry.RemoteExists && !entry.DeletedTombstone && !entry.ConflictFlag
	case domain.SyncActionUploadFile, domain.SyncActionDownloadFile:
		return entry.EntryType == "file" && entry.LocalExists && entry.RemoteExists && entry.SyncState == "synced" && !entry.DeletedTombstone && !entry.ConflictFlag
	case domain.SyncActionMoveLocal, domain.SyncActionMoveRemote:
		return entry.EntryType == "file" && entry.LocalExists && entry.RemoteExists && entry.SyncState == "synced" && !entry.DeletedTombstone && !entry.ConflictFlag
	case domain.SyncActionDeleteLocal:
		return !entry.LocalExists && entry.DeletedTombstone
	case domain.SyncActionDeleteRemote:
		return !entry.RemoteExists && entry.DeletedTombstone
	case domain.SyncActionMoveConflictLocal, domain.SyncActionMoveConflictRemote:
		return entry.ConflictFlag && entry.SyncState == "conflicted"
	case domain.SyncActionRefreshMetadata:
		return true
	default:
		return false
	}
}
