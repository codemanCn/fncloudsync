package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
	sqlitestore "github.com/xiaoxuesen/fn-cloudsync/internal/store/sqlite"
)

func TestTaskRuntimeRepositoryUpsertAndGet(t *testing.T) {
	t.Parallel()

	db := openMigratedDB(t)
	defer db.Close()

	connectionRepo := sqlitestore.NewConnectionRepository(db)
	taskRepo := sqlitestore.NewTaskRepository(db)
	runtimeRepo := sqlitestore.NewTaskRuntimeRepository(db)
	ctx := context.Background()

	connection := domain.Connection{
		ID:                 "conn-1",
		Name:               "primary",
		Endpoint:           "https://dav.example.com/root",
		Username:           "alice",
		PasswordCiphertext: "ciphertext-1",
		RootPath:           "/",
		TLSMode:            domain.TLSModeStrict,
		TimeoutSec:         30,
		Status:             "active",
		CreatedAt:          time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
	}
	if err := connectionRepo.Create(ctx, connection); err != nil {
		t.Fatalf("Create(connection) error = %v", err)
	}

	task := domain.Task{
		ID:           "task-1",
		Name:         "sync-home",
		ConnectionID: "conn-1",
		LocalPath:    "/tmp/local",
		RemotePath:   "/remote",
		Direction:    domain.TaskDirectionUpload,
		Status:       domain.TaskStatusCreated,
		CreatedAt:    time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}

	input := domain.TaskRuntimeState{
		TaskID:          task.ID,
		Phase:           "running",
		LastLocalScanAt: time.Date(2026, 3, 24, 10, 1, 0, 0, time.UTC),
		LastSuccessAt:   time.Date(2026, 3, 24, 10, 2, 0, 0, time.UTC),
		RetryStreak:     1,
		LastError:       "temporary",
		UpdatedAt:       time.Date(2026, 3, 24, 10, 3, 0, 0, time.UTC),
	}
	if err := runtimeRepo.Upsert(ctx, input); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	got, err := runtimeRepo.GetByTaskID(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetByTaskID() error = %v", err)
	}
	if got.Phase != input.Phase {
		t.Fatalf("Phase = %q, want %q", got.Phase, input.Phase)
	}
	if !got.LastSuccessAt.Equal(input.LastSuccessAt) {
		t.Fatalf("LastSuccessAt = %v, want %v", got.LastSuccessAt, input.LastSuccessAt)
	}
}

func TestTaskRuntimeRepositoryGetMissingReturnsNotFound(t *testing.T) {
	t.Parallel()

	db := openMigratedDB(t)
	defer db.Close()

	repo := sqlitestore.NewTaskRuntimeRepository(db)
	_, err := repo.GetByTaskID(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByTaskID() error = %v, want ErrNotFound", err)
	}
}
