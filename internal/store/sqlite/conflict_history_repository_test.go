package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
	sqlitestore "github.com/xiaoxuesen/fn-cloudsync/internal/store/sqlite"
)

func TestConflictHistoryRepositoryCreateAndList(t *testing.T) {
	t.Parallel()

	db := openTaskScopedDB(t)
	defer db.Close()

	repo := sqlitestore.NewConflictHistoryRepository(db)
	ctx := context.Background()

	record := domain.ConflictRecord{
		ID:                 "conflict-1",
		TaskID:             "task-1",
		RelativePath:       "shared.txt",
		LocalConflictPath:  "/tmp/local/shared.remote-conflict-v8.txt",
		RemoteConflictPath: "/remote/shared.local-conflict-v8.txt",
		Policy:             "keep_both",
		DetectedAt:         time.Date(2026, 3, 24, 13, 0, 0, 0, time.UTC),
	}
	if err := repo.Create(ctx, record); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	items, err := repo.ListByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListByTaskID() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if got, want := items[0].RelativePath, "shared.txt"; got != want {
		t.Fatalf("RelativePath = %q, want %q", got, want)
	}
	if got, want := items[0].Policy, "keep_both"; got != want {
		t.Fatalf("Policy = %q, want %q", got, want)
	}
}
