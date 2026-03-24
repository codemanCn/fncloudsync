package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
	sqlitestore "github.com/xiaoxuesen/fn-cloudsync/internal/store/sqlite"
)

func TestFileIndexRepositoryUpsertAndList(t *testing.T) {
	t.Parallel()

	db := openTaskScopedDB(t)
	defer db.Close()

	repo := sqlitestore.NewFileIndexRepository(db)
	ctx := context.Background()

	entry := domain.FileIndexEntry{
		ID:                "idx-1",
		TaskID:            "task-1",
		RelativePath:      "docs/readme.txt",
		EntryType:         "file",
		LocalExists:       true,
		RemoteExists:      true,
		LocalSize:         5,
		RemoteSize:        5,
		LastSyncDirection: "bidirectional",
		LastSyncAt:        time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC),
		Version:           1,
		SyncState:         "synced",
	}
	if err := repo.Upsert(ctx, entry); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	entry.Version = 2
	entry.DeletedTombstone = true
	entry.RemoteExists = false
	if err := repo.Upsert(ctx, entry); err != nil {
		t.Fatalf("Upsert(update) error = %v", err)
	}

	items, err := repo.ListByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListByTaskID() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if got, want := items[0].Version, 2; got != want {
		t.Fatalf("Version = %d, want %d", got, want)
	}
	if !items[0].DeletedTombstone {
		t.Fatal("DeletedTombstone = false, want true")
	}
}
