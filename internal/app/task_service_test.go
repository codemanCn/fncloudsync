package app_test

import (
	"context"
	"errors"
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
	task       domain.Task
	connection domain.Connection
	password   string
	err        error
}

func (s *stubBaselineRunner) RunOnce(_ context.Context, task domain.Task, connection domain.Connection, password string) error {
	s.task = task
	s.connection = connection
	s.password = password
	return s.err
}

type stubTaskRuntimeRepository struct {
	states []domain.TaskRuntimeState
}

func (s *stubTaskRuntimeRepository) Upsert(_ context.Context, state domain.TaskRuntimeState) error {
	s.states = append(s.states, state)
	return nil
}

type stubFailureRecordRepository struct {
	records []domain.FailureRecord
}

func (s *stubFailureRecordRepository) Create(_ context.Context, record domain.FailureRecord) error {
	s.records = append(s.records, record)
	return nil
}

type stubOperationQueueRepository struct {
	items []domain.OperationQueueItem
}

func (s *stubOperationQueueRepository) Enqueue(_ context.Context, item domain.OperationQueueItem) error {
	s.items = append(s.items, item)
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
