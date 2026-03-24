package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	appcrypto "github.com/xiaoxuesen/fn-cloudsync/internal/crypto"
	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
	sqlitestore "github.com/xiaoxuesen/fn-cloudsync/internal/store/sqlite"
	"github.com/xiaoxuesen/fn-cloudsync/testutil/testdb"
)

func TestOpenEnablesWALMode(t *testing.T) {
	t.Parallel()

	dbPath := testdb.Path(t)

	db, err := sqlitestore.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode;").Scan(&mode); err != nil {
		t.Fatalf("QueryRow(PRAGMA journal_mode) error = %v", err)
	}

	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestMigrateCreatesCoreTables(t *testing.T) {
	t.Parallel()

	dbPath := testdb.Path(t)

	db, err := sqlitestore.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := sqlitestore.Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	assertTableExists(t, db, "schema_migrations")
	assertTableExists(t, db, "connections")
	assertTableExists(t, db, "tasks")
}

func TestConnectionRepositoryCRUD(t *testing.T) {
	t.Parallel()

	db := openMigratedDB(t)
	defer db.Close()

	repo := sqlitestore.NewConnectionRepository(db)
	ctx := context.Background()

	older := domain.Connection{
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
	newer := older
	newer.ID = "conn-2"
	newer.Name = "backup"
	newer.Endpoint = "https://dav.example.com/backup"
	newer.PasswordCiphertext = "ciphertext-2"
	newer.CreatedAt = older.CreatedAt.Add(time.Minute)
	newer.UpdatedAt = newer.CreatedAt

	if err := repo.Create(ctx, older); err != nil {
		t.Fatalf("Create(older) error = %v", err)
	}
	if err := repo.Create(ctx, newer); err != nil {
		t.Fatalf("Create(newer) error = %v", err)
	}

	got, err := repo.GetByID(ctx, older.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Name != older.Name {
		t.Fatalf("GetByID().Name = %q, want %q", got.Name, older.Name)
	}

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("List() len = %d, want 2", len(items))
	}
	if items[0].ID != newer.ID || items[1].ID != older.ID {
		t.Fatalf("List() order = [%q, %q], want [%q, %q]", items[0].ID, items[1].ID, newer.ID, older.ID)
	}

	older.Name = "primary-updated"
	older.UpdatedAt = older.UpdatedAt.Add(2 * time.Minute)
	if err := repo.Update(ctx, older); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err = repo.GetByID(ctx, older.ID)
	if err != nil {
		t.Fatalf("GetByID() after update error = %v", err)
	}
	if got.Name != older.Name {
		t.Fatalf("GetByID().Name after update = %q, want %q", got.Name, older.Name)
	}
	if got.PasswordCiphertext != older.PasswordCiphertext {
		t.Fatalf("GetByID().PasswordCiphertext after update = %q, want %q", got.PasswordCiphertext, older.PasswordCiphertext)
	}

	if err := repo.Delete(ctx, newer.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(ctx, newer.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID() after delete error = %v, want ErrNotFound", err)
	}
}

func TestConnectionRepositoryStoresEncryptedCiphertext(t *testing.T) {
	t.Parallel()

	db := openMigratedDB(t)
	defer db.Close()

	repo := sqlitestore.NewConnectionRepository(db)
	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}

	ciphertext, err := secrets.EncryptString("top-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	connection := domain.Connection{
		ID:                 "conn-1",
		Name:               "primary",
		Endpoint:           "https://dav.example.com/root",
		Username:           "alice",
		PasswordCiphertext: ciphertext,
		RootPath:           "/",
		TLSMode:            domain.TLSModeStrict,
		TimeoutSec:         30,
		Status:             "active",
		CreatedAt:          time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
	}

	if err := repo.Create(context.Background(), connection); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByID(context.Background(), connection.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.PasswordCiphertext == "top-secret" {
		t.Fatal("GetByID().PasswordCiphertext returned plaintext, want ciphertext")
	}

	plaintext, err := secrets.DecryptString(got.PasswordCiphertext)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}
	if plaintext != "top-secret" {
		t.Fatalf("DecryptString() = %q, want %q", plaintext, "top-secret")
	}
}

func assertTableExists(t *testing.T, db *sql.DB, tableName string) {
	t.Helper()

	var name string
	err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?",
		tableName,
	).Scan(&name)
	if err != nil {
		t.Fatalf("table %q not found: %v", tableName, err)
	}

	if name != tableName {
		t.Fatalf("table name = %q, want %q", name, tableName)
	}
}

func openMigratedDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sqlitestore.Open(testdb.Path(t))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if err := sqlitestore.Migrate(context.Background(), db); err != nil {
		db.Close()
		t.Fatalf("Migrate() error = %v", err)
	}

	return db
}
