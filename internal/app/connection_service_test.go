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

func TestConnectionValidateRequiresNameEndpointAndUsername(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		connection domain.Connection
	}{
		{
			name: "missing name",
			connection: domain.Connection{
				Endpoint: "https://example.com/dav",
				Username: "alice",
			},
		},
		{
			name: "missing endpoint",
			connection: domain.Connection{
				Name:     "primary",
				Username: "alice",
			},
		},
		{
			name: "missing username",
			connection: domain.Connection{
				Name:     "primary",
				Endpoint: "https://example.com/dav",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := tc.connection.Validate(); err == nil {
				t.Fatalf("Validate() error = nil, want invalid argument")
			}
		})
	}
}

func TestConnectionServiceCreateEncryptsPassword(t *testing.T) {
	t.Parallel()

	repo := &stubConnectionRepository{}
	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}

	service, err := app.NewConnectionService(repo, secrets)
	if err != nil {
		t.Fatalf("NewConnectionService() error = %v", err)
	}

	connection := domain.Connection{
		ID:         "conn-1",
		Name:       "primary",
		Endpoint:   "https://dav.example.com/root",
		Username:   "alice",
		RootPath:   "/",
		TLSMode:    domain.TLSModeStrict,
		TimeoutSec: 30,
		Status:     "active",
	}

	created, err := service.Create(context.Background(), connection, "top-secret")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if repo.lastCreated.PasswordCiphertext == "" {
		t.Fatal("Create() stored empty ciphertext, want encrypted password")
	}
	if repo.lastCreated.PasswordCiphertext == "top-secret" {
		t.Fatal("Create() stored plaintext password, want ciphertext")
	}
	if created.ID != connection.ID {
		t.Fatalf("Create().ID = %q, want %q", created.ID, connection.ID)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatal("Create() returned zero timestamps, want normalized timestamps")
	}
}

func TestNewConnectionServiceRejectsEmptySecretKey(t *testing.T) {
	t.Parallel()

	_, err := app.NewConnectionService(&stubConnectionRepository{}, nil)
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("NewConnectionService() error = %v, want ErrInvalidArgument", err)
	}
}

func TestConnectionServiceDeleteRejectsReferencedConnection(t *testing.T) {
	t.Parallel()

	repo := &stubConnectionRepository{hasTasks: true}
	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}

	service, err := app.NewConnectionService(repo, secrets)
	if err != nil {
		t.Fatalf("NewConnectionService() error = %v", err)
	}

	err = service.Delete(context.Background(), "conn-1")
	if !errors.Is(err, domain.ErrReferencedResource) {
		t.Fatalf("Delete() error = %v, want ErrReferencedResource", err)
	}
}

func TestConnectionServiceGetByIDReturnsRepositoryValue(t *testing.T) {
	t.Parallel()

	repo := &stubConnectionRepository{
		getResult: domain.Connection{ID: "conn-1", Name: "primary"},
	}
	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}

	service, err := app.NewConnectionService(repo, secrets)
	if err != nil {
		t.Fatalf("NewConnectionService() error = %v", err)
	}

	got, err := service.GetByID(context.Background(), "conn-1")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.ID != "conn-1" {
		t.Fatalf("GetByID().ID = %q, want %q", got.ID, "conn-1")
	}
}

func TestConnectionServiceUpdateEncryptsPassword(t *testing.T) {
	t.Parallel()

	repo := &stubConnectionRepository{}
	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}

	service, err := app.NewConnectionService(repo, secrets)
	if err != nil {
		t.Fatalf("NewConnectionService() error = %v", err)
	}

	connection := domain.Connection{
		ID:         "conn-1",
		Name:       "updated",
		Endpoint:   "https://dav.example.com/root",
		Username:   "alice",
		RootPath:   "/",
		TLSMode:    domain.TLSModeStrict,
		TimeoutSec: 30,
		Status:     "active",
		CreatedAt:  time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
	}

	updated, err := service.Update(context.Background(), connection, "top-secret")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if repo.lastUpdated.PasswordCiphertext == "" {
		t.Fatal("Update() stored empty ciphertext, want encrypted password")
	}
	if updated.ID != "conn-1" {
		t.Fatalf("Update().ID = %q, want %q", updated.ID, "conn-1")
	}
}

func TestConnectionServiceUpdatePreservesCreatedAtAndCiphertextWhenPasswordEmpty(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC)
	repo := &stubConnectionRepository{
		getResult: domain.Connection{
			ID:                 "conn-1",
			Name:               "primary",
			Endpoint:           "https://dav.example.com/root",
			Username:           "alice",
			PasswordCiphertext: "existing-ciphertext",
			RootPath:           "/",
			TLSMode:            domain.TLSModeStrict,
			TimeoutSec:         30,
			Status:             "active",
			CreatedAt:          createdAt,
			UpdatedAt:          createdAt,
		},
	}
	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}

	service, err := app.NewConnectionService(repo, secrets)
	if err != nil {
		t.Fatalf("NewConnectionService() error = %v", err)
	}

	updated, err := service.Update(context.Background(), domain.Connection{
		ID:         "conn-1",
		Name:       "updated",
		Endpoint:   "https://dav.example.com/root",
		Username:   "alice",
		RootPath:   "/",
		TLSMode:    domain.TLSModeStrict,
		TimeoutSec: 60,
		Status:     "active",
	}, "")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if repo.lastUpdated.PasswordCiphertext != "existing-ciphertext" {
		t.Fatalf("PasswordCiphertext = %q, want preserved ciphertext", repo.lastUpdated.PasswordCiphertext)
	}
	if !updated.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %v, want %v", updated.CreatedAt, createdAt)
	}
}

func TestConnectionServiceCreateValidatesInput(t *testing.T) {
	t.Parallel()

	repo := &stubConnectionRepository{}
	secrets, err := appcrypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}

	service, err := app.NewConnectionService(repo, secrets)
	if err != nil {
		t.Fatalf("NewConnectionService() error = %v", err)
	}

	_, err = service.Create(context.Background(), domain.Connection{}, "top-secret")
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("Create() error = %v, want ErrInvalidArgument", err)
	}
}

type stubConnectionRepository struct {
	lastCreated domain.Connection
	lastUpdated domain.Connection
	deleteErr   error
	getResult   domain.Connection
	getErr      error
	listResult  []domain.Connection
	listErr     error
	hasTasks    bool
}

func (s *stubConnectionRepository) Create(_ context.Context, connection domain.Connection) error {
	s.lastCreated = connection
	return nil
}

func (s *stubConnectionRepository) GetByID(_ context.Context, _ string) (domain.Connection, error) {
	return s.getResult, s.getErr
}

func (s *stubConnectionRepository) List(_ context.Context) ([]domain.Connection, error) {
	return s.listResult, s.listErr
}

func (s *stubConnectionRepository) Update(_ context.Context, connection domain.Connection) error {
	s.lastUpdated = connection
	return nil
}

func (s *stubConnectionRepository) Delete(_ context.Context, _ string) error {
	return s.deleteErr
}

func (s *stubConnectionRepository) HasTasks(_ context.Context, _ string) (bool, error) {
	return s.hasTasks, nil
}
