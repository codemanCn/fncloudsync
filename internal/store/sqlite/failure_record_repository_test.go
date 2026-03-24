package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
	sqlitestore "github.com/xiaoxuesen/fn-cloudsync/internal/store/sqlite"
)

func TestFailureRecordRepositoryCreateAndList(t *testing.T) {
	t.Parallel()

	db := openTaskScopedDB(t)
	defer db.Close()

	repo := sqlitestore.NewFailureRecordRepository(db)
	ctx := context.Background()

	record := domain.FailureRecord{
		ID:            "fail-1",
		TaskID:        "task-1",
		Path:          "/remote/report.txt",
		OpType:        "upload",
		ErrorCode:     "temporary",
		ErrorMessage:  "network timeout",
		Retryable:     true,
		FirstFailedAt: time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
		LastFailedAt:  time.Date(2026, 3, 24, 10, 1, 0, 0, time.UTC),
		AttemptCount:  2,
	}
	if err := repo.Create(ctx, record); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	records, err := repo.ListByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListByTaskID() error = %v", err)
	}
	if len(records) != 1 || records[0].ID != record.ID {
		t.Fatalf("ListByTaskID() = %+v, want record %q", records, record.ID)
	}
}

func TestFailureRecordRepositoryResolve(t *testing.T) {
	t.Parallel()

	db := openTaskScopedDB(t)
	defer db.Close()

	repo := sqlitestore.NewFailureRecordRepository(db)
	ctx := context.Background()

	record := domain.FailureRecord{
		ID:            "fail-1",
		TaskID:        "task-1",
		Path:          "report.txt",
		OpType:        "UploadFile",
		ErrorCode:     "temporary",
		ErrorMessage:  "network timeout",
		Retryable:     true,
		FirstFailedAt: time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
		LastFailedAt:  time.Date(2026, 3, 24, 10, 1, 0, 0, time.UTC),
		AttemptCount:  1,
	}
	if err := repo.Create(ctx, record); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	resolvedAt := time.Date(2026, 3, 24, 10, 2, 0, 0, time.UTC)
	if err := repo.Resolve(ctx, "fail-1", resolvedAt); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	got, err := repo.GetByID(ctx, "fail-1")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !got.ResolvedAt.Equal(resolvedAt) {
		t.Fatalf("ResolvedAt = %v, want %v", got.ResolvedAt, resolvedAt)
	}
}
