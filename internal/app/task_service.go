package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
}

type taskConnectionRepository interface {
	GetByID(context.Context, string) (domain.Connection, error)
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
		item.Status = "pending"
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

func (s *TaskService) ExecuteQueueOperation(ctx context.Context, item domain.OperationQueueItem) error {
	task, connection, password, err := s.resolveTaskExecution(ctx, item.TaskID)
	if err != nil {
		return err
	}
	return s.executeQueueOperationResolved(ctx, task, connection, password, item)
}

func (s *TaskService) executeQueueOperationResolved(ctx context.Context, task domain.Task, connection domain.Connection, password string, item domain.OperationQueueItem) error {
	if s.queueRepo != nil {
		item.Status = "executing"
		item.UpdatedAt = time.Now().UTC()
		_ = s.queueRepo.Reschedule(ctx, item)
	}
	var action domain.SyncAction
	if err := json.Unmarshal([]byte(item.PayloadJSON), &action); err != nil {
		return err
	}
	if err := s.baselineRunner.ExecuteAction(ctx, task, connection, password, action); err != nil {
		s.recordActionFailure(ctx, task, item, action, err)
		return err
	}
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

		if err := s.executePlannedActions(ctx, task, connection, password); err != nil {
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
		LastLocalScanAt: scanTimestampForLocal(task.Direction),
		LastRemoteScanAt: scanTimestampForRemote(task.Direction),
		LastSuccessAt:   time.Now().UTC(),
		LastReconcileAt: time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
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
			Status:       "pending",
			AttemptCount: 0,
			CreatedAt:    now,
			UpdatedAt:    now,
		}); err != nil {
			return err
		}
	}

	items, err := s.queueRepo.ListByTaskID(ctx, task.ID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !isActionQueueItem(item.OpType) {
			continue
		}
		if err := s.executeQueueOperationResolved(ctx, task, connection, password, item); err != nil {
			item.AttemptCount++
			item.LastError = err.Error()
			item.NextAttemptAt = time.Now().UTC().Add(time.Second)
			item.Status = "retry_wait"
			item.UpdatedAt = time.Now().UTC()
			_ = s.queueRepo.Reschedule(ctx, item)
			return err
		}
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
}

func isActionQueueItem(opType string) bool {
	switch domain.SyncActionType(opType) {
	case domain.SyncActionCreateDirLocal,
		domain.SyncActionCreateDirRemote,
		domain.SyncActionUploadFile,
		domain.SyncActionDownloadFile,
		domain.SyncActionDeleteLocal,
		domain.SyncActionDeleteRemote,
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
	case domain.SyncActionUploadFile, domain.SyncActionCreateDirRemote, domain.SyncActionDeleteRemote, domain.SyncActionMoveConflictRemote:
		return "local"
	case domain.SyncActionDownloadFile, domain.SyncActionCreateDirLocal, domain.SyncActionDeleteLocal, domain.SyncActionMoveConflictLocal:
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
		case "pending":
			summary.Pending++
		case "executing":
			summary.Executing++
		case "retry_wait":
			summary.RetryWait++
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
