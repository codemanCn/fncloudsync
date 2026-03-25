package app_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/app"
	appcrypto "github.com/xiaoxuesen/fn-cloudsync/internal/crypto"
	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

func TestTaskValidateRequiresCoreFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		task domain.Task
	}{
		{
			name: "missing name",
			task: domain.Task{
				ConnectionID: "conn-1",
				LocalPath:    "/tmp/local",
				RemotePath:   "/remote",
				Direction:    domain.TaskDirectionUpload,
			},
		},
		{
			name: "missing connection id",
			task: domain.Task{
				Name:       "sync-home",
				LocalPath:  "/tmp/local",
				RemotePath: "/remote",
				Direction:  domain.TaskDirectionUpload,
			},
		},
		{
			name: "missing local path",
			task: domain.Task{
				Name:         "sync-home",
				ConnectionID: "conn-1",
				RemotePath:   "/remote",
				Direction:    domain.TaskDirectionUpload,
			},
		},
		{
			name: "missing remote path",
			task: domain.Task{
				Name:         "sync-home",
				ConnectionID: "conn-1",
				LocalPath:    "/tmp/local",
				Direction:    domain.TaskDirectionUpload,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := tc.task.Validate(); err == nil {
				t.Fatalf("Validate() error = nil, want invalid argument")
			}
		})
	}
}

func TestTaskValidateRejectsInvalidDirection(t *testing.T) {
	t.Parallel()

	task := domain.Task{
		Name:         "sync-home",
		ConnectionID: "conn-1",
		LocalPath:    "/tmp/local",
		RemotePath:   "/remote",
		Direction:    domain.TaskDirection("sideways"),
	}

	if err := task.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid argument")
	}
}

func TestTaskApplyDefaultsSetsCreatedStatus(t *testing.T) {
	t.Parallel()

	task := domain.Task{
		Name:         "sync-home",
		ConnectionID: "conn-1",
		LocalPath:    "/tmp/local",
		RemotePath:   "/remote",
		Direction:    domain.TaskDirectionUpload,
	}

	task.ApplyDefaults()

	if got, want := task.Status, domain.TaskStatusCreated; got != want {
		t.Fatalf("Status = %q, want %q", got, want)
	}
}

func TestTaskServiceCreateAppliesDefaults(t *testing.T) {
	t.Parallel()

	repo := &stubTaskRepository{}
	service := app.NewTaskService(repo)

	task := domain.Task{
		ID:           "task-1",
		Name:         "sync-home",
		ConnectionID: "conn-1",
		LocalPath:    "/tmp/local",
		RemotePath:   "/remote",
		Direction:    domain.TaskDirectionUpload,
	}

	created, err := service.Create(context.Background(), task)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if got, want := repo.lastCreated.Status, domain.TaskStatusCreated; got != want {
		t.Fatalf("stored Status = %q, want %q", got, want)
	}
	if got, want := created.Status, domain.TaskStatusCreated; got != want {
		t.Fatalf("Create().Status = %q, want %q", got, want)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatal("Create() returned zero timestamps, want normalized timestamps")
	}
}

func TestTaskServiceCreateValidatesInput(t *testing.T) {
	t.Parallel()

	service := app.NewTaskService(&stubTaskRepository{})

	_, err := service.Create(context.Background(), domain.Task{})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("Create() error = %v, want ErrInvalidArgument", err)
	}
}

func TestTaskServiceGetByIDReturnsRepositoryValue(t *testing.T) {
	t.Parallel()

	service := app.NewTaskService(&stubTaskRepository{
		getResult: domain.Task{ID: "task-1", Name: "sync-home"},
	})

	got, err := service.GetByID(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.ID != "task-1" {
		t.Fatalf("GetByID().ID = %q, want %q", got.ID, "task-1")
	}
}

func TestTaskServiceUpdateValidatesInput(t *testing.T) {
	t.Parallel()

	service := app.NewTaskService(&stubTaskRepository{})

	_, err := service.Update(context.Background(), domain.Task{})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("Update() error = %v, want ErrInvalidArgument", err)
	}
}

func TestTaskServiceUpdatePreservesCreatedAtAndStatusWhenUnset(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC)
	service := app.NewTaskService(&stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionUpload,
			Status:       domain.TaskStatusCreated,
			CreatedAt:    createdAt,
			UpdatedAt:    createdAt,
		},
	})

	updated, err := service.Update(context.Background(), domain.Task{
		ID:           "task-1",
		Name:         "sync-home-updated",
		ConnectionID: "conn-1",
		LocalPath:    "/tmp/local",
		RemotePath:   "/remote",
		Direction:    domain.TaskDirectionUpload,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if !updated.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %v, want %v", updated.CreatedAt, createdAt)
	}
	if updated.Status != domain.TaskStatusCreated {
		t.Fatalf("Status = %q, want %q", updated.Status, domain.TaskStatusCreated)
	}
}

func TestTaskServiceStartPauseStopTransitions(t *testing.T) {
	t.Parallel()

	repo := &stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionUpload,
			Status:       domain.TaskStatusCreated,
		},
	}
	service := app.NewTaskService(repo)

	started, err := service.Start(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if started.Status != domain.TaskStatusRunning {
		t.Fatalf("Start().Status = %q, want %q", started.Status, domain.TaskStatusRunning)
	}

	repo.getResult = repo.lastUpdated
	paused, err := service.Pause(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	if paused.Status != domain.TaskStatusPaused {
		t.Fatalf("Pause().Status = %q, want %q", paused.Status, domain.TaskStatusPaused)
	}

	repo.getResult = repo.lastUpdated
	stopped, err := service.Stop(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if stopped.Status != domain.TaskStatusStopped {
		t.Fatalf("Stop().Status = %q, want %q", stopped.Status, domain.TaskStatusStopped)
	}
}

func TestTaskServiceStartRunsUploadBaseline(t *testing.T) {
	t.Parallel()

	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}
	passwordCiphertext, err := secrets.EncryptString("top-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	repo := &stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionUpload,
			Status:       domain.TaskStatusCreated,
		},
	}
	connectionRepo := &stubTaskConnectionRepository{
		connection: domain.Connection{
			ID:                 "conn-1",
			Endpoint:           "https://dav.example.com",
			Username:           "alice",
			PasswordCiphertext: passwordCiphertext,
		},
	}
	runner := &stubBaselineRunner{}
	runtimeRepo := &stubTaskRuntimeRepository{}
	service := app.NewTaskService(repo)
	service.SetConnectionRepository(connectionRepo)
	service.SetSecrets(secrets)
	service.SetBaselineRunner(runner)
	service.SetRuntimeRepository(runtimeRepo)

	started, err := service.Start(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if started.Status != domain.TaskStatusRunning {
		t.Fatalf("Start().Status = %q, want %q", started.Status, domain.TaskStatusRunning)
	}
	if runner.password != "top-secret" {
		t.Fatalf("runner password = %q, want %q", runner.password, "top-secret")
	}
	if runner.task.ID != "task-1" {
		t.Fatalf("runner task id = %q, want %q", runner.task.ID, "task-1")
	}
	if len(runtimeRepo.states) < 2 {
		t.Fatalf("len(runtime states) = %d, want at least 2", len(runtimeRepo.states))
	}
	if runtimeRepo.states[0].Phase != "running" {
		t.Fatalf("first runtime phase = %q, want %q", runtimeRepo.states[0].Phase, "running")
	}
	if runtimeRepo.states[len(runtimeRepo.states)-1].LastSuccessAt.IsZero() {
		t.Fatal("last runtime state missing LastSuccessAt")
	}
	if runtimeRepo.states[len(runtimeRepo.states)-1].LastLocalScanAt.IsZero() {
		t.Fatal("last runtime state missing LastLocalScanAt for upload")
	}
}

func TestTaskServiceStartEnqueuesAndExecutesPlannedActions(t *testing.T) {
	t.Parallel()

	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}
	passwordCiphertext, err := secrets.EncryptString("top-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	repo := &stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionUpload,
			Status:       domain.TaskStatusCreated,
		},
	}
	connectionRepo := &stubTaskConnectionRepository{
		connection: domain.Connection{
			ID:                 "conn-1",
			Endpoint:           "https://dav.example.com",
			Username:           "alice",
			PasswordCiphertext: passwordCiphertext,
		},
	}
	runner := &stubBaselineRunner{
		planned: []domain.SyncAction{
			{Type: domain.SyncActionUploadFile, RelativePath: "report.txt", LocalPath: "/tmp/local/report.txt", RemotePath: "/remote/report.txt"},
		},
	}
	queueRepo := &stubOperationQueueRepository{}
	service := app.NewTaskService(repo)
	service.SetConnectionRepository(connectionRepo)
	service.SetSecrets(secrets)
	service.SetBaselineRunner(runner)
	service.SetOperationQueueRepository(queueRepo)

	_, err = service.Start(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(queueRepo.items) != 1 {
		t.Fatalf("len(queue items) = %d, want 1", len(queueRepo.items))
	}
	if got, want := queueRepo.items[0].OpType, string(domain.SyncActionUploadFile); got != want {
		t.Fatalf("queue op type = %q, want %q", got, want)
	}
	if len(runner.executedActions) != 1 || runner.executedActions[0].Type != domain.SyncActionUploadFile {
		t.Fatalf("executed actions = %+v, want upload action", runner.executedActions)
	}
	if len(queueRepo.dequeued) != 1 {
		t.Fatalf("dequeued = %v, want one dequeued item", queueRepo.dequeued)
	}
}

func TestTaskServiceStartFailurePersistsFailureAndQueue(t *testing.T) {
	t.Parallel()

	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}
	passwordCiphertext, err := secrets.EncryptString("top-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	repo := &stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionUpload,
			Status:       domain.TaskStatusCreated,
		},
	}
	connectionRepo := &stubTaskConnectionRepository{
		connection: domain.Connection{
			ID:                 "conn-1",
			Endpoint:           "https://dav.example.com",
			Username:           "alice",
			PasswordCiphertext: passwordCiphertext,
		},
	}
	runner := &stubBaselineRunner{err: errors.New("network timeout")}
	runner.planErr = errors.New("network timeout")
	runtimeRepo := &stubTaskRuntimeRepository{}
	failureRepo := &stubFailureRecordRepository{}
	queueRepo := &stubOperationQueueRepository{}
	service := app.NewTaskService(repo)
	service.SetConnectionRepository(connectionRepo)
	service.SetSecrets(secrets)
	service.SetBaselineRunner(runner)
	service.SetRuntimeRepository(runtimeRepo)
	service.SetFailureRepository(failureRepo)
	service.SetOperationQueueRepository(queueRepo)

	_, err = service.Start(context.Background(), "task-1")
	if err == nil {
		t.Fatal("Start() error = nil, want failure")
	}
	if len(failureRepo.records) != 1 {
		t.Fatalf("len(failure records) = %d, want 1", len(failureRepo.records))
	}
	if failureRepo.records[0].TaskID != "task-1" {
		t.Fatalf("failure task id = %q, want %q", failureRepo.records[0].TaskID, "task-1")
	}
	if len(queueRepo.items) != 1 {
		t.Fatalf("len(queue items) = %d, want 1", len(queueRepo.items))
	}
	if queueRepo.items[0].OpType != "baseline_sync" {
		t.Fatalf("queue op type = %q, want %q", queueRepo.items[0].OpType, "baseline_sync")
	}
}

func TestTaskServiceExecuteRunningTaskRunsWithoutStatusTransition(t *testing.T) {
	t.Parallel()

	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}
	passwordCiphertext, err := secrets.EncryptString("top-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	repo := &stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionDownload,
			Status:       domain.TaskStatusRunning,
		},
	}
	connectionRepo := &stubTaskConnectionRepository{
		connection: domain.Connection{
			ID:                 "conn-1",
			Endpoint:           "https://dav.example.com",
			Username:           "alice",
			PasswordCiphertext: passwordCiphertext,
		},
	}
	runner := &stubBaselineRunner{}
	service := app.NewTaskService(repo)
	service.SetConnectionRepository(connectionRepo)
	service.SetSecrets(secrets)
	service.SetBaselineRunner(runner)

	if err := service.ExecuteRunningTask(context.Background(), "task-1"); err != nil {
		t.Fatalf("ExecuteRunningTask() error = %v", err)
	}
	if runner.task.ID != "task-1" {
		t.Fatalf("runner task id = %q, want %q", runner.task.ID, "task-1")
	}
}

func TestTaskServiceExecuteRunningTaskTracksRemoteScanForDownload(t *testing.T) {
	t.Parallel()

	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}
	passwordCiphertext, err := secrets.EncryptString("top-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	repo := &stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionDownload,
			Status:       domain.TaskStatusRunning,
		},
	}
	connectionRepo := &stubTaskConnectionRepository{
		connection: domain.Connection{
			ID:                 "conn-1",
			Endpoint:           "https://dav.example.com",
			Username:           "alice",
			PasswordCiphertext: passwordCiphertext,
		},
	}
	runner := &stubBaselineRunner{}
	runtimeRepo := &stubTaskRuntimeRepository{}
	service := app.NewTaskService(repo)
	service.SetConnectionRepository(connectionRepo)
	service.SetSecrets(secrets)
	service.SetBaselineRunner(runner)
	service.SetRuntimeRepository(runtimeRepo)

	if err := service.ExecuteRunningTask(context.Background(), "task-1"); err != nil {
		t.Fatalf("ExecuteRunningTask() error = %v", err)
	}
	state := runtimeRepo.states[len(runtimeRepo.states)-1]
	if state.LastRemoteScanAt.IsZero() {
		t.Fatal("LastRemoteScanAt is zero, want timestamp for download")
	}
	if !state.LastLocalScanAt.IsZero() {
		t.Fatalf("LastLocalScanAt = %v, want zero for download", state.LastLocalScanAt)
	}
}

func TestTaskServiceExecuteRunningTaskLogsPlannedActions(t *testing.T) {
	t.Parallel()

	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}
	passwordCiphertext, err := secrets.EncryptString("top-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	repo := &stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionUpload,
			Status:       domain.TaskStatusRunning,
		},
	}
	connectionRepo := &stubTaskConnectionRepository{
		connection: domain.Connection{
			ID:                 "conn-1",
			Endpoint:           "https://dav.example.com",
			Username:           "alice",
			PasswordCiphertext: passwordCiphertext,
		},
	}
	runner := &stubBaselineRunner{
		planned: []domain.SyncAction{
			{Type: domain.SyncActionUploadFile, RelativePath: "report.txt", LocalPath: "/tmp/local/report.txt", RemotePath: "/remote/report.txt"},
		},
	}
	queueRepo := &stubOperationQueueRepository{}
	logger := &stubTaskServiceLogger{}

	service := app.NewTaskService(repo)
	service.SetConnectionRepository(connectionRepo)
	service.SetSecrets(secrets)
	service.SetBaselineRunner(runner)
	service.SetOperationQueueRepository(queueRepo)
	service.SetLogger(logger)

	if err := service.ExecuteRunningTask(context.Background(), "task-1"); err != nil {
		t.Fatalf("ExecuteRunningTask() error = %v", err)
	}

	joined := strings.Join(logger.lines, "\n")
	if !strings.Contains(joined, "task_id=task-1") {
		t.Fatalf("logs = %q, want task_id", joined)
	}
	if !strings.Contains(joined, "planned_actions=1") {
		t.Fatalf("logs = %q, want planned_actions count", joined)
	}
}

func TestTaskServiceExecuteRunningTaskAllowsDegradedTask(t *testing.T) {
	t.Parallel()

	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}
	passwordCiphertext, err := secrets.EncryptString("top-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	repo := &stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionUpload,
			Status:       domain.TaskStatusDegraded,
		},
	}
	connectionRepo := &stubTaskConnectionRepository{
		connection: domain.Connection{
			ID:                 "conn-1",
			Endpoint:           "https://dav.example.com",
			Username:           "alice",
			PasswordCiphertext: passwordCiphertext,
		},
	}
	runner := &stubBaselineRunner{}
	service := app.NewTaskService(repo)
	service.SetConnectionRepository(connectionRepo)
	service.SetSecrets(secrets)
	service.SetBaselineRunner(runner)

	if err := service.ExecuteRunningTask(context.Background(), "task-1"); err != nil {
		t.Fatalf("ExecuteRunningTask() error = %v, want nil for degraded task", err)
	}
	if got, want := runner.task.ID, "task-1"; got != want {
		t.Fatalf("runner task id = %q, want %q", got, want)
	}
}

func TestTaskServicePollRemoteTaskWritesCheckpoint(t *testing.T) {
	t.Parallel()

	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}
	passwordCiphertext, err := secrets.EncryptString("top-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	repo := &stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionDownload,
			Status:       domain.TaskStatusRetrying,
		},
	}
	connectionRepo := &stubTaskConnectionRepository{
		connection: domain.Connection{
			ID:                 "conn-1",
			Endpoint:           "https://dav.example.com",
			Username:           "alice",
			PasswordCiphertext: passwordCiphertext,
		},
	}
	runner := &stubBaselineRunner{}
	runtimeRepo := &stubTaskRuntimeRepository{}
	service := app.NewTaskService(repo)
	service.SetConnectionRepository(connectionRepo)
	service.SetSecrets(secrets)
	service.SetBaselineRunner(runner)
	service.SetRuntimeRepository(runtimeRepo)

	if err := service.PollRemoteTask(context.Background(), "task-1"); err != nil {
		t.Fatalf("PollRemoteTask() error = %v", err)
	}
	foundCheckpoint := false
	for _, state := range runtimeRepo.states {
		if state.TaskID == "task-1" && state.CheckpointJSON != "" {
			foundCheckpoint = true
			break
		}
	}
	if !foundCheckpoint {
		t.Fatalf("runtime states = %+v, want remote poll checkpoint", runtimeRepo.states)
	}
}

func TestTaskServicePollRemoteTaskStoresRemoteDiscoveredPathsInCheckpoint(t *testing.T) {
	t.Parallel()

	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}
	passwordCiphertext, err := secrets.EncryptString("top-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	repo := &stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionDownload,
			Status:       domain.TaskStatusRunning,
		},
	}
	connectionRepo := &stubTaskConnectionRepository{
		connection: domain.Connection{
			ID:                 "conn-1",
			Endpoint:           "https://dav.example.com",
			Username:           "alice",
			PasswordCiphertext: passwordCiphertext,
		},
	}
	runner := &stubBaselineRunner{
		planned: []domain.SyncAction{
			{Type: domain.SyncActionDownloadFile, RelativePath: "report.txt", LocalPath: "/tmp/local/report.txt", RemotePath: "/remote/report.txt"},
			{Type: domain.SyncActionCreateDirLocal, RelativePath: "docs", LocalPath: "/tmp/local/docs", RemotePath: "/remote/docs", IsDir: true},
		},
	}
	runtimeRepo := &stubTaskRuntimeRepository{}
	queueRepo := &stubOperationQueueRepository{}
	service := app.NewTaskService(repo)
	service.SetConnectionRepository(connectionRepo)
	service.SetSecrets(secrets)
	service.SetBaselineRunner(runner)
	service.SetRuntimeRepository(runtimeRepo)
	service.SetOperationQueueRepository(queueRepo)

	if err := service.PollRemoteTask(context.Background(), "task-1"); err != nil {
		t.Fatalf("PollRemoteTask() error = %v", err)
	}

	foundCheckpoint := false
	for _, state := range runtimeRepo.states {
		if state.TaskID != "task-1" {
			continue
		}
		if strings.Contains(state.CheckpointJSON, `"changed_paths":["docs","report.txt"]`) {
			foundCheckpoint = true
			break
		}
	}
	if !foundCheckpoint {
		t.Fatalf("runtime states = %+v, want changed_paths checkpoint", runtimeRepo.states)
	}
}

func TestTaskServiceExecuteQueueOperationRunsSingleAction(t *testing.T) {
	t.Parallel()

	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}
	passwordCiphertext, err := secrets.EncryptString("top-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	repo := &stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionUpload,
			Status:       domain.TaskStatusRunning,
		},
	}
	connectionRepo := &stubTaskConnectionRepository{
		connection: domain.Connection{
			ID:                 "conn-1",
			Endpoint:           "https://dav.example.com",
			Username:           "alice",
			PasswordCiphertext: passwordCiphertext,
		},
	}
	runner := &stubBaselineRunner{}
	service := app.NewTaskService(repo)
	service.SetConnectionRepository(connectionRepo)
	service.SetSecrets(secrets)
	service.SetBaselineRunner(runner)

	err = service.ExecuteQueueOperation(context.Background(), domain.OperationQueueItem{
		ID:          "op-1",
		TaskID:      "task-1",
		OpType:      string(domain.SyncActionUploadFile),
		PayloadJSON: `{"Type":"UploadFile","RelativePath":"report.txt","LocalPath":"/tmp/local/report.txt","RemotePath":"/remote/report.txt"}`,
	})
	if err != nil {
		t.Fatalf("ExecuteQueueOperation() error = %v", err)
	}
	if len(runner.executedActions) != 1 || runner.executedActions[0].RelativePath != "report.txt" {
		t.Fatalf("executed actions = %+v, want queued report.txt action", runner.executedActions)
	}
}

func TestTaskServiceExecuteQueueOperationSkipsRetriedActionWhenFileIndexAlreadyApplied(t *testing.T) {
	t.Parallel()

	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}
	passwordCiphertext, err := secrets.EncryptString("top-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	repo := &stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionUpload,
			Status:       domain.TaskStatusRunning,
		},
	}
	connectionRepo := &stubTaskConnectionRepository{
		connection: domain.Connection{
			ID:                 "conn-1",
			Endpoint:           "https://dav.example.com",
			Username:           "alice",
			PasswordCiphertext: passwordCiphertext,
		},
	}
	runner := &stubBaselineRunner{}
	fileIndexRepo := &stubTaskFileIndexRepository{
		items: map[string]domain.FileIndexEntry{
			"task-1/report.txt": {
				TaskID:           "task-1",
				RelativePath:     "report.txt",
				EntryType:        "file",
				LocalExists:      true,
				RemoteExists:     true,
				DeletedTombstone: false,
				ConflictFlag:     false,
				SyncState:        "synced",
				LastSyncAt:       time.Date(2026, 3, 24, 11, 0, 0, 0, time.UTC),
			},
		},
	}

	service := app.NewTaskService(repo)
	service.SetConnectionRepository(connectionRepo)
	service.SetSecrets(secrets)
	service.SetBaselineRunner(runner)
	service.SetFileIndexRepository(fileIndexRepo)

	err = service.ExecuteQueueOperation(context.Background(), domain.OperationQueueItem{
		ID:           "op-1",
		TaskID:       "task-1",
		OpType:       string(domain.SyncActionUploadFile),
		AttemptCount: 1,
		PayloadJSON:  `{"Type":"UploadFile","RelativePath":"report.txt","LocalPath":"/tmp/local/report.txt","RemotePath":"/remote/report.txt"}`,
	})
	if err != nil {
		t.Fatalf("ExecuteQueueOperation() error = %v", err)
	}
	if len(runner.executedActions) != 0 {
		t.Fatalf("executed actions = %+v, want skipped retry", runner.executedActions)
	}
}

func TestTaskServiceListFailuresReturnsTaskRecords(t *testing.T) {
	t.Parallel()

	service := app.NewTaskService(&stubTaskRepository{
		getResult: domain.Task{ID: "task-1"},
	})
	failureRepo := &stubFailureRecordRepository{
		records: []domain.FailureRecord{
			{ID: "fail-1", TaskID: "task-1", Path: "report.txt", OpType: "UploadFile"},
		},
	}
	service.SetFailureRepository(failureRepo)

	items, err := service.ListFailures(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("ListFailures() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "fail-1" {
		t.Fatalf("ListFailures() = %+v, want fail-1", items)
	}
}

func TestTaskServiceRetryFailuresResetsQueueItems(t *testing.T) {
	t.Parallel()

	service := app.NewTaskService(&stubTaskRepository{
		getResult: domain.Task{ID: "task-1"},
	})
	queueRepo := &stubOperationQueueRepository{retryResetCount: 2}
	service.SetOperationQueueRepository(queueRepo)

	count, err := service.RetryFailures(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("RetryFailures() error = %v", err)
	}
	if got, want := count, 2; got != want {
		t.Fatalf("RetryFailures() count = %d, want %d", got, want)
	}
	if got, want := queueRepo.retryResetTaskID, "task-1"; got != want {
		t.Fatalf("retryResetTaskID = %q, want %q", got, want)
	}
}

func TestTaskServiceRetryFailureByIDReschedulesMatchingQueueItemAndResolvesRecord(t *testing.T) {
	t.Parallel()

	service := app.NewTaskService(&stubTaskRepository{
		getResult: domain.Task{ID: "task-1"},
	})
	failureRepo := &stubFailureRecordRepository{
		records: []domain.FailureRecord{
			{ID: "fail-1", TaskID: "task-1", Path: "report.txt", OpType: "UploadFile"},
		},
	}
	queueRepo := &stubOperationQueueRepository{
		items: []domain.OperationQueueItem{
			{ID: "op-1", TaskID: "task-1", OpType: "UploadFile", TargetPath: "/remote/report.txt", Status: "retry_wait"},
		},
	}
	service.SetFailureRepository(failureRepo)
	service.SetOperationQueueRepository(queueRepo)

	count, err := service.RetryFailureByID(context.Background(), "task-1", "fail-1")
	if err != nil {
		t.Fatalf("RetryFailureByID() error = %v", err)
	}
	if got, want := count, 1; got != want {
		t.Fatalf("RetryFailureByID() count = %d, want %d", got, want)
	}
	if len(queueRepo.rescheduled) == 0 || queueRepo.rescheduled[len(queueRepo.rescheduled)-1].Status != "queued" {
		t.Fatalf("rescheduled queue items = %+v, want queued item", queueRepo.rescheduled)
	}
	if got, want := failureRepo.resolvedID, "fail-1"; got != want {
		t.Fatalf("resolvedID = %q, want %q", got, want)
	}
}

func TestTaskServiceGetRuntimeViewAggregatesRuntimeQueueAndFailures(t *testing.T) {
	t.Parallel()

	service := app.NewTaskService(&stubTaskRepository{
		getResult: domain.Task{ID: "task-1", Status: domain.TaskStatusRunning},
	})
	service.SetRuntimeRepository(&stubTaskRuntimeRepository{
		states: []domain.TaskRuntimeState{
			{TaskID: "task-1", Phase: "idle"},
		},
	})
	service.SetOperationQueueRepository(&stubOperationQueueRepository{
		items: []domain.OperationQueueItem{
			{ID: "op-1", TaskID: "task-1", Status: "queued"},
			{ID: "op-2", TaskID: "task-1", Status: "retry_wait"},
			{ID: "op-3", TaskID: "task-1", Status: "succeeded"},
		},
	})
	service.SetFailureRepository(&stubFailureRecordRepository{
		records: []domain.FailureRecord{
			{ID: "fail-1", TaskID: "task-1"},
			{ID: "fail-2", TaskID: "task-1", ResolvedAt: time.Now().UTC()},
		},
	})

	view, err := service.GetRuntimeView(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("GetRuntimeView() error = %v", err)
	}
	if got, want := view.Runtime.Phase, "idle"; got != want {
		t.Fatalf("Runtime.Phase = %q, want %q", got, want)
	}
	if got, want := view.QueueSummary.Total, 3; got != want {
		t.Fatalf("QueueSummary.Total = %d, want %d", got, want)
	}
	if got, want := view.QueueSummary.Queued, 1; got != want {
		t.Fatalf("QueueSummary.Queued = %d, want %d", got, want)
	}
	if got, want := view.QueueSummary.RetryWait, 1; got != want {
		t.Fatalf("QueueSummary.RetryWait = %d, want %d", got, want)
	}
	if got, want := view.QueueSummary.Succeeded, 1; got != want {
		t.Fatalf("QueueSummary.Succeeded = %d, want %d", got, want)
	}
	if got, want := view.FailureSummary.Open, 1; got != want {
		t.Fatalf("FailureSummary.Open = %d, want %d", got, want)
	}
}

func TestTaskServiceGetMetricsAggregatesTaskQueueAndFailureCounts(t *testing.T) {
	t.Parallel()

	service := app.NewTaskService(&stubTaskRepository{
		listResult: []domain.Task{
			{ID: "task-1", Status: domain.TaskStatusRunning},
			{ID: "task-2", Status: domain.TaskStatusDegraded},
			{ID: "task-3", Status: domain.TaskStatusRunning},
		},
	})
	service.SetOperationQueueRepository(&stubOperationQueueRepository{
		items: []domain.OperationQueueItem{
			{ID: "op-1", TaskID: "task-1", Status: "queued"},
			{ID: "op-2", TaskID: "task-1", Status: "retry_wait"},
			{ID: "op-3", TaskID: "task-2", Status: "succeeded"},
		},
	})
	service.SetFailureRepository(&stubFailureRecordRepository{
		records: []domain.FailureRecord{
			{ID: "fail-1", TaskID: "task-1", Retryable: true},
			{ID: "fail-2", TaskID: "task-1", Retryable: false, ResolvedAt: time.Now().UTC()},
			{ID: "fail-3", TaskID: "task-2", Retryable: true},
		},
	})

	metrics, err := service.GetMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetMetrics() error = %v", err)
	}
	if got, want := metrics.TaskStates[string(domain.TaskStatusRunning)], 2; got != want {
		t.Fatalf("TaskStates[running] = %d, want %d", got, want)
	}
	if got, want := metrics.TaskStates[string(domain.TaskStatusDegraded)], 1; got != want {
		t.Fatalf("TaskStates[degraded] = %d, want %d", got, want)
	}
	if got, want := metrics.Queue.Total, 3; got != want {
		t.Fatalf("Queue.Total = %d, want %d", got, want)
	}
	if got, want := metrics.Queue.RetryWait, 1; got != want {
		t.Fatalf("Queue.RetryWait = %d, want %d", got, want)
	}
	if got, want := metrics.Failures.Total, 3; got != want {
		t.Fatalf("Failures.Total = %d, want %d", got, want)
	}
	if got, want := metrics.Failures.RetryableOpen, 2; got != want {
		t.Fatalf("Failures.RetryableOpen = %d, want %d", got, want)
	}
}

func TestTaskServiceListEventsReturnsTaskTimeline(t *testing.T) {
	t.Parallel()

	service := app.NewTaskService(&stubTaskRepository{
		getResult: domain.Task{ID: "task-1"},
	})
	eventRepo := &stubTaskEventRepository{
		events: []domain.TaskEvent{
			{ID: "event-1", TaskID: "task-1", EventType: "task_started"},
		},
	}
	service.SetEventRepository(eventRepo)

	items, err := service.ListEvents(context.Background(), "task-1", 10)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "event-1" {
		t.Fatalf("ListEvents() = %+v, want event-1", items)
	}
}

func TestTaskServiceStartRecordsTaskStartedEvent(t *testing.T) {
	t.Parallel()

	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}
	passwordCiphertext, err := secrets.EncryptString("top-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	repo := &stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionUpload,
			Status:       domain.TaskStatusCreated,
		},
	}
	connectionRepo := &stubTaskConnectionRepository{
		connection: domain.Connection{
			ID:                 "conn-1",
			Endpoint:           "https://dav.example.com",
			Username:           "alice",
			PasswordCiphertext: passwordCiphertext,
		},
	}
	eventRepo := &stubTaskEventRepository{}
	service := app.NewTaskService(repo)
	service.SetConnectionRepository(connectionRepo)
	service.SetSecrets(secrets)
	service.SetBaselineRunner(&stubBaselineRunner{})
	service.SetEventRepository(eventRepo)

	if _, err := service.Start(context.Background(), "task-1"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(eventRepo.events) == 0 {
		t.Fatal("events = 0, want task_started event")
	}
	found := false
	for _, event := range eventRepo.events {
		if event.EventType == "task_started" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events = %+v, want task_started event", eventRepo.events)
	}
}

func TestTaskServiceStartActionFailureMarksTaskDegradedAndQueuesRetry(t *testing.T) {
	t.Parallel()

	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}
	passwordCiphertext, err := secrets.EncryptString("top-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	repo := &stubTaskRepository{
		getResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionUpload,
			Status:       domain.TaskStatusCreated,
		},
	}
	connectionRepo := &stubTaskConnectionRepository{
		connection: domain.Connection{
			ID:                 "conn-1",
			Endpoint:           "https://dav.example.com",
			Username:           "alice",
			PasswordCiphertext: passwordCiphertext,
		},
	}
	runner := &stubBaselineRunner{
		planned: []domain.SyncAction{
			{Type: domain.SyncActionUploadFile, RelativePath: "report.txt", LocalPath: "/tmp/local/report.txt", RemotePath: "/remote/report.txt"},
		},
		actionErr: errors.New("put failed"),
	}
	queueRepo := &stubOperationQueueRepository{}
	failureRepo := &stubFailureRecordRepository{}
	service := app.NewTaskService(repo)
	service.SetConnectionRepository(connectionRepo)
	service.SetSecrets(secrets)
	service.SetBaselineRunner(runner)
	service.SetOperationQueueRepository(queueRepo)
	service.SetFailureRepository(failureRepo)

	started, err := service.Start(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Start() error = %v, want nil for degraded task", err)
	}
	if got, want := started.Status, domain.TaskStatusDegraded; got != want {
		t.Fatalf("Start().Status = %q, want %q", got, want)
	}
	if len(queueRepo.rescheduled) == 0 {
		t.Fatal("rescheduled queue items = 0, want retry_wait item")
	}
	last := queueRepo.rescheduled[len(queueRepo.rescheduled)-1]
	if got, want := last.Status, "retry_wait"; got != want {
		t.Fatalf("queue status = %q, want %q", got, want)
	}
	foundActionFailure := false
	for _, record := range failureRepo.records {
		if record.Path == "report.txt" && record.OpType == string(domain.SyncActionUploadFile) {
			foundActionFailure = true
			break
		}
	}
	if !foundActionFailure {
		t.Fatalf("failure records = %+v, want action-level failure for report.txt", failureRepo.records)
	}
	if got, want := repo.lastUpdated.Status, domain.TaskStatusDegraded; got != want {
		t.Fatalf("stored task status = %q, want %q", got, want)
	}
}

type stubTaskRepository struct {
	lastCreated domain.Task
	lastUpdated domain.Task
	getResult   domain.Task
	getErr      error
	listResult  []domain.Task
	listErr     error
}

type stubTaskConnectionRepository struct {
	connection domain.Connection
	err        error
}

func (s *stubTaskConnectionRepository) GetByID(_ context.Context, _ string) (domain.Connection, error) {
	return s.connection, s.err
}

type stubBaselineRunner struct {
	task            domain.Task
	connection      domain.Connection
	password        string
	err             error
	planErr         error
	actionErr       error
	planned         []domain.SyncAction
	executedActions []domain.SyncAction
}

func (s *stubBaselineRunner) RunOnce(_ context.Context, task domain.Task, connection domain.Connection, password string) error {
	s.task = task
	s.connection = connection
	s.password = password
	return s.err
}

func (s *stubBaselineRunner) Plan(_ context.Context, task domain.Task, connection domain.Connection, password string) ([]domain.SyncAction, error) {
	s.task = task
	s.connection = connection
	s.password = password
	return s.planned, s.planErr
}

func (s *stubBaselineRunner) ExecuteAction(_ context.Context, task domain.Task, connection domain.Connection, password string, action domain.SyncAction) error {
	s.task = task
	s.connection = connection
	s.password = password
	s.executedActions = append(s.executedActions, action)
	return s.actionErr
}

type stubTaskRuntimeRepository struct {
	states []domain.TaskRuntimeState
}

func (s *stubTaskRuntimeRepository) Upsert(_ context.Context, state domain.TaskRuntimeState) error {
	s.states = append(s.states, state)
	return nil
}

func (s *stubTaskRuntimeRepository) GetByTaskID(_ context.Context, taskID string) (domain.TaskRuntimeState, error) {
	for _, state := range s.states {
		if state.TaskID == taskID {
			return state, nil
		}
	}
	return domain.TaskRuntimeState{}, domain.ErrNotFound
}

type stubFailureRecordRepository struct {
	records    []domain.FailureRecord
	resolvedID string
}

func (s *stubFailureRecordRepository) Create(_ context.Context, record domain.FailureRecord) error {
	s.records = append(s.records, record)
	return nil
}

func (s *stubFailureRecordRepository) ListByTaskID(_ context.Context, taskID string) ([]domain.FailureRecord, error) {
	var records []domain.FailureRecord
	for _, record := range s.records {
		if record.TaskID == taskID {
			records = append(records, record)
		}
	}
	return records, nil
}

func (s *stubFailureRecordRepository) GetByID(_ context.Context, id string) (domain.FailureRecord, error) {
	for _, record := range s.records {
		if record.ID == id {
			return record, nil
		}
	}
	return domain.FailureRecord{}, domain.ErrNotFound
}

func (s *stubFailureRecordRepository) Resolve(_ context.Context, id string, resolvedAt time.Time) error {
	s.resolvedID = id
	for index := range s.records {
		if s.records[index].ID == id {
			s.records[index].ResolvedAt = resolvedAt
			return nil
		}
	}
	return domain.ErrNotFound
}

type stubOperationQueueRepository struct {
	items            []domain.OperationQueueItem
	dequeued         []string
	rescheduled      []domain.OperationQueueItem
	retryResetTaskID string
	retryResetCount  int
}

type stubTaskFileIndexRepository struct {
	items map[string]domain.FileIndexEntry
}

type stubTaskEventRepository struct {
	events []domain.TaskEvent
}

type stubTaskServiceLogger struct {
	lines []string
}

func (s *stubTaskServiceLogger) Printf(format string, args ...any) {
	s.lines = append(s.lines, fmt.Sprintf(format, args...))
}

func (s *stubTaskFileIndexRepository) GetByTaskIDAndPath(_ context.Context, taskID, relativePath string) (domain.FileIndexEntry, error) {
	item, ok := s.items[taskID+"/"+relativePath]
	if !ok {
		return domain.FileIndexEntry{}, domain.ErrNotFound
	}
	return item, nil
}

func (s *stubTaskEventRepository) Create(_ context.Context, event domain.TaskEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *stubTaskEventRepository) ListByTaskID(_ context.Context, taskID string, limit int) ([]domain.TaskEvent, error) {
	if limit <= 0 || limit > len(s.events) {
		limit = len(s.events)
	}
	var items []domain.TaskEvent
	for _, event := range s.events {
		if event.TaskID == taskID {
			items = append(items, event)
		}
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *stubOperationQueueRepository) Enqueue(_ context.Context, item domain.OperationQueueItem) error {
	s.items = append(s.items, item)
	return nil
}

func (s *stubOperationQueueRepository) ListByTaskID(_ context.Context, taskID string) ([]domain.OperationQueueItem, error) {
	var items []domain.OperationQueueItem
	for _, item := range s.items {
		if item.TaskID == taskID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *stubOperationQueueRepository) Dequeue(_ context.Context, id string) error {
	s.dequeued = append(s.dequeued, id)
	return nil
}

func (s *stubOperationQueueRepository) Reschedule(_ context.Context, item domain.OperationQueueItem) error {
	s.rescheduled = append(s.rescheduled, item)
	return nil
}

func (s *stubOperationQueueRepository) ResetRetryableByTaskID(_ context.Context, taskID string) (int, error) {
	s.retryResetTaskID = taskID
	return s.retryResetCount, nil
}

func (s *stubOperationQueueRepository) MarkFailed(_ context.Context, id string, lastError string) error {
	return nil
}

func (s *stubTaskRepository) Create(_ context.Context, task domain.Task) error {
	s.lastCreated = task
	return nil
}

func (s *stubTaskRepository) GetByID(_ context.Context, _ string) (domain.Task, error) {
	return s.getResult, s.getErr
}

func (s *stubTaskRepository) List(_ context.Context) ([]domain.Task, error) {
	return s.listResult, s.listErr
}

func (s *stubTaskRepository) Update(_ context.Context, task domain.Task) error {
	s.lastUpdated = task
	return nil
}

func (s *stubTaskRepository) Delete(_ context.Context, _ string) error {
	return nil
}
