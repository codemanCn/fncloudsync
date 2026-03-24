package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/app"
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

type stubTaskRepository struct {
	lastCreated domain.Task
	lastUpdated domain.Task
	getResult   domain.Task
	getErr      error
	listResult  []domain.Task
	listErr     error
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
