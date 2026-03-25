package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
	sqlitestore "github.com/xiaoxuesen/fn-cloudsync/internal/store/sqlite"
)

func TestTaskEventRepositoryCreateAndList(t *testing.T) {
	t.Parallel()

	db := openTaskScopedDB(t)
	defer db.Close()

	repo := sqlitestore.NewTaskEventRepository(db)
	ctx := context.Background()

	event := domain.TaskEvent{
		ID:          "event-1",
		TaskID:      "task-1",
		EventType:   "task_started",
		Level:       "info",
		Message:     "task started",
		DetailsJSON: `{"status":"running"}`,
		CreatedAt:   time.Date(2026, 3, 24, 14, 0, 0, 0, time.UTC),
	}
	if err := repo.Create(ctx, event); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	items, err := repo.ListByTaskID(ctx, "task-1", 10)
	if err != nil {
		t.Fatalf("ListByTaskID() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if got, want := items[0].EventType, "task_started"; got != want {
		t.Fatalf("EventType = %q, want %q", got, want)
	}
	if got, want := items[0].DetailsJSON, `{"status":"running"}`; got != want {
		t.Fatalf("DetailsJSON = %q, want %q", got, want)
	}
}
