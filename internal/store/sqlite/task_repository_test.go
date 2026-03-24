package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
	sqlitestore "github.com/xiaoxuesen/fn-cloudsync/internal/store/sqlite"
)

func TestTaskRepositoryCRUD(t *testing.T) {
	t.Parallel()

	db := openMigratedDB(t)
	defer db.Close()

	connectionRepo := sqlitestore.NewConnectionRepository(db)
	taskRepo := sqlitestore.NewTaskRepository(db)
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

	older := domain.Task{
		ID:                 "task-1",
		Name:               "sync-home",
		ConnectionID:       connection.ID,
		LocalPath:          "/tmp/local-a",
		RemotePath:         "/remote-a",
		Direction:          domain.TaskDirectionUpload,
		PollIntervalSec:    30,
		ConflictPolicy:     "keep_both",
		DeletePolicy:       "mirror",
		EmptyDirPolicy:     "keep",
		BandwidthLimitKbps: 0,
		MaxWorkers:         2,
		EncryptionEnabled:  false,
		HashMode:           "basic",
		Status:             domain.TaskStatusCreated,
		DesiredState:       "",
		LastError:          "",
		CreatedAt:          time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
	}
	newer := older
	newer.ID = "task-2"
	newer.Name = "sync-backup"
	newer.LocalPath = "/tmp/local-b"
	newer.RemotePath = "/remote-b"
	newer.CreatedAt = older.CreatedAt.Add(time.Minute)
	newer.UpdatedAt = newer.CreatedAt

	if err := taskRepo.Create(ctx, older); err != nil {
		t.Fatalf("Create(older) error = %v", err)
	}
	if err := taskRepo.Create(ctx, newer); err != nil {
		t.Fatalf("Create(newer) error = %v", err)
	}

	got, err := taskRepo.GetByID(ctx, older.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Name != older.Name {
		t.Fatalf("GetByID().Name = %q, want %q", got.Name, older.Name)
	}

	items, err := taskRepo.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("List() len = %d, want 2", len(items))
	}
	if items[0].ID != newer.ID || items[1].ID != older.ID {
		t.Fatalf("List() order = [%q, %q], want [%q, %q]", items[0].ID, items[1].ID, newer.ID, older.ID)
	}

	older.Name = "sync-home-updated"
	older.UpdatedAt = older.UpdatedAt.Add(2 * time.Minute)
	if err := taskRepo.Update(ctx, older); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err = taskRepo.GetByID(ctx, older.ID)
	if err != nil {
		t.Fatalf("GetByID() after update error = %v", err)
	}
	if got.Name != older.Name {
		t.Fatalf("GetByID().Name after update = %q, want %q", got.Name, older.Name)
	}

	if err := taskRepo.Delete(ctx, newer.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = taskRepo.GetByID(ctx, newer.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID() after delete error = %v, want ErrNotFound", err)
	}
}

func TestTaskRepositoryCreateRejectsMissingConnection(t *testing.T) {
	t.Parallel()

	db := openMigratedDB(t)
	defer db.Close()

	repo := sqlitestore.NewTaskRepository(db)

	err := repo.Create(context.Background(), domain.Task{
		ID:                 "task-1",
		Name:               "sync-home",
		ConnectionID:       "missing-conn",
		LocalPath:          "/tmp/local-a",
		RemotePath:         "/remote-a",
		Direction:          domain.TaskDirectionUpload,
		PollIntervalSec:    30,
		ConflictPolicy:     "keep_both",
		DeletePolicy:       "mirror",
		EmptyDirPolicy:     "keep",
		BandwidthLimitKbps: 0,
		MaxWorkers:         2,
		EncryptionEnabled:  false,
		HashMode:           "basic",
		Status:             domain.TaskStatusCreated,
		CreatedAt:          time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Create() error = %v, want ErrConflict", err)
	}
}

func TestTaskRepositoryHasTasksByConnectionID(t *testing.T) {
	t.Parallel()

	db := openMigratedDB(t)
	defer db.Close()

	connectionRepo := sqlitestore.NewConnectionRepository(db)
	taskRepo := sqlitestore.NewTaskRepository(db)
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

	if err := taskRepo.Create(ctx, domain.Task{
		ID:                 "task-1",
		Name:               "sync-home",
		ConnectionID:       connection.ID,
		LocalPath:          "/tmp/local-a",
		RemotePath:         "/remote-a",
		Direction:          domain.TaskDirectionUpload,
		PollIntervalSec:    30,
		ConflictPolicy:     "keep_both",
		DeletePolicy:       "mirror",
		EmptyDirPolicy:     "keep",
		BandwidthLimitKbps: 0,
		MaxWorkers:         2,
		EncryptionEnabled:  false,
		HashMode:           "basic",
		Status:             domain.TaskStatusCreated,
		CreatedAt:          time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Create(task) error = %v", err)
	}

	hasTasks, err := taskRepo.HasTasksByConnectionID(ctx, connection.ID)
	if err != nil {
		t.Fatalf("HasTasksByConnectionID() error = %v", err)
	}
	if !hasTasks {
		t.Fatal("HasTasksByConnectionID() = false, want true")
	}
}

func openTaskScopedDB(t *testing.T) *sql.DB {
	t.Helper()

	db := openMigratedDB(t)
	connectionRepo := sqlitestore.NewConnectionRepository(db)
	taskRepo := sqlitestore.NewTaskRepository(db)
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
		db.Close()
		t.Fatalf("Create(connection) error = %v", err)
	}

	task := domain.Task{
		ID:           "task-1",
		Name:         "sync-home",
		ConnectionID: connection.ID,
		LocalPath:    "/tmp/local",
		RemotePath:   "/remote",
		Direction:    domain.TaskDirectionUpload,
		Status:       domain.TaskStatusCreated,
		CreatedAt:    time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		db.Close()
		t.Fatalf("Create(task) error = %v", err)
	}

	return db
}
