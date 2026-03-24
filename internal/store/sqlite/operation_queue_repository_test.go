package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
	sqlitestore "github.com/xiaoxuesen/fn-cloudsync/internal/store/sqlite"
)

func TestOperationQueueRepositoryEnqueueListAndDequeue(t *testing.T) {
	t.Parallel()

	db := openTaskScopedDB(t)
	defer db.Close()

	repo := sqlitestore.NewOperationQueueRepository(db)
	ctx := context.Background()

	item := domain.OperationQueueItem{
		ID:         "op-1",
		TaskID:     "task-1",
		OpType:     "upload",
		TargetPath: "/remote/report.txt",
		Status:     "pending",
		CreatedAt:  time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
	}
	if err := repo.Enqueue(ctx, item); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	items, err := repo.ListByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListByTaskID() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("ListByTaskID() = %+v, want item %q", items, item.ID)
	}

	if err := repo.Dequeue(ctx, item.ID); err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
}

func TestOperationQueueRepositoryListDueAndReschedule(t *testing.T) {
	t.Parallel()

	db := openTaskScopedDB(t)
	defer db.Close()

	repo := sqlitestore.NewOperationQueueRepository(db)
	ctx := context.Background()

	now := time.Date(2026, 3, 24, 11, 0, 0, 0, time.UTC)
	due := domain.OperationQueueItem{
		ID:            "op-due",
		TaskID:        "task-1",
		OpType:        "baseline_sync",
		TargetPath:    "/remote",
		Status:        "pending",
		NextAttemptAt: now.Add(-time.Minute),
		CreatedAt:     now.Add(-2 * time.Minute),
		UpdatedAt:     now.Add(-2 * time.Minute),
	}
	later := domain.OperationQueueItem{
		ID:            "op-later",
		TaskID:        "task-1",
		OpType:        "baseline_sync",
		TargetPath:    "/remote",
		Status:        "pending",
		NextAttemptAt: now.Add(time.Minute),
		CreatedAt:     now.Add(-time.Minute),
		UpdatedAt:     now.Add(-time.Minute),
	}

	if err := repo.Enqueue(ctx, due); err != nil {
		t.Fatalf("Enqueue(due) error = %v", err)
	}
	if err := repo.Enqueue(ctx, later); err != nil {
		t.Fatalf("Enqueue(later) error = %v", err)
	}

	items, err := repo.ListDue(ctx, now, 10)
	if err != nil {
		t.Fatalf("ListDue() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != due.ID {
		t.Fatalf("ListDue() = %+v, want only %q", items, due.ID)
	}

	due.AttemptCount = 2
	due.LastError = "network timeout"
	due.NextAttemptAt = now.Add(5 * time.Minute)
	due.UpdatedAt = now
	if err := repo.Reschedule(ctx, due); err != nil {
		t.Fatalf("Reschedule() error = %v", err)
	}

	rescheduled, err := repo.GetByID(ctx, due.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got, want := rescheduled.AttemptCount, 2; got != want {
		t.Fatalf("AttemptCount = %d, want %d", got, want)
	}
	if got, want := rescheduled.LastError, "network timeout"; got != want {
		t.Fatalf("LastError = %q, want %q", got, want)
	}
	if !rescheduled.NextAttemptAt.Equal(due.NextAttemptAt) {
		t.Fatalf("NextAttemptAt = %v, want %v", rescheduled.NextAttemptAt, due.NextAttemptAt)
	}
}

func TestOperationQueueRepositoryReschedulePersistsStatus(t *testing.T) {
	t.Parallel()

	db := openTaskScopedDB(t)
	defer db.Close()

	repo := sqlitestore.NewOperationQueueRepository(db)
	ctx := context.Background()

	item := domain.OperationQueueItem{
		ID:        "op-1",
		TaskID:    "task-1",
		OpType:    "UploadFile",
		Status:    "pending",
		CreatedAt: time.Date(2026, 3, 24, 11, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 3, 24, 11, 0, 0, 0, time.UTC),
	}
	if err := repo.Enqueue(ctx, item); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	item.Status = "executing"
	item.UpdatedAt = item.UpdatedAt.Add(time.Minute)
	if err := repo.Reschedule(ctx, item); err != nil {
		t.Fatalf("Reschedule() error = %v", err)
	}

	got, err := repo.GetByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != "executing" {
		t.Fatalf("Status = %q, want %q", got.Status, "executing")
	}
}

func TestOperationQueueRepositoryResetRetryableByTaskID(t *testing.T) {
	t.Parallel()

	db := openTaskScopedDB(t)
	defer db.Close()

	repo := sqlitestore.NewOperationQueueRepository(db)
	ctx := context.Background()

	item := domain.OperationQueueItem{
		ID:            "op-1",
		TaskID:        "task-1",
		OpType:        "UploadFile",
		Status:        "retry_wait",
		NextAttemptAt: time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC),
		CreatedAt:     time.Date(2026, 3, 24, 11, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 3, 24, 11, 0, 0, 0, time.UTC),
	}
	if err := repo.Enqueue(ctx, item); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	count, err := repo.ResetRetryableByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatalf("ResetRetryableByTaskID() error = %v", err)
	}
	if got, want := count, 1; got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}

	got, err := repo.GetByID(ctx, "op-1")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != "pending" {
		t.Fatalf("Status = %q, want %q", got.Status, "pending")
	}
	if !got.NextAttemptAt.IsZero() {
		t.Fatalf("NextAttemptAt = %v, want zero", got.NextAttemptAt)
	}
}
