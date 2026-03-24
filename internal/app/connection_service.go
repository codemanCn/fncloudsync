package app

import (
	"context"
	"errors"
	"time"

	appcrypto "github.com/xiaoxuesen/fn-cloudsync/internal/crypto"
	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type connectionRepository interface {
	Create(context.Context, domain.Connection) error
	GetByID(context.Context, string) (domain.Connection, error)
	List(context.Context) ([]domain.Connection, error)
	Update(context.Context, domain.Connection) error
	Delete(context.Context, string) error
	HasTasks(context.Context, string) (bool, error)
}

type ConnectionService struct {
	repo    connectionRepository
	secrets *appcrypto.SecretManager
}

func NewConnectionService(repo connectionRepository, secrets *appcrypto.SecretManager) (*ConnectionService, error) {
	if repo == nil {
		return nil, errors.Join(domain.ErrInvalidArgument, errors.New("repository is required"))
	}
	if secrets == nil {
		return nil, errors.Join(domain.ErrInvalidArgument, errors.New("secret manager is required"))
	}

	return &ConnectionService{
		repo:    repo,
		secrets: secrets,
	}, nil
}

func (s *ConnectionService) Create(ctx context.Context, connection domain.Connection, plaintextPassword string) (domain.Connection, error) {
	if err := connection.Validate(); err != nil {
		return domain.Connection{}, err
	}

	now := time.Now().UTC()
	if connection.CreatedAt.IsZero() {
		connection.CreatedAt = now
	}
	if connection.UpdatedAt.IsZero() {
		connection.UpdatedAt = connection.CreatedAt
	}

	ciphertext, err := s.secrets.EncryptString(plaintextPassword)
	if err != nil {
		return domain.Connection{}, err
	}

	connection.PasswordCiphertext = ciphertext
	if err := s.repo.Create(ctx, connection); err != nil {
		return domain.Connection{}, err
	}

	return connection, nil
}

func (s *ConnectionService) Delete(ctx context.Context, id string) error {
	if id == "" {
		return errors.Join(domain.ErrInvalidArgument, errors.New("id is required"))
	}

	hasTasks, err := s.repo.HasTasks(ctx, id)
	if err != nil {
		return err
	}
	if hasTasks {
		return domain.ErrReferencedResource
	}

	return s.repo.Delete(ctx, id)
}

func (s *ConnectionService) List(ctx context.Context) ([]domain.Connection, error) {
	return s.repo.List(ctx)
}

func (s *ConnectionService) GetByID(ctx context.Context, id string) (domain.Connection, error) {
	if id == "" {
		return domain.Connection{}, errors.Join(domain.ErrInvalidArgument, errors.New("id is required"))
	}

	return s.repo.GetByID(ctx, id)
}

func (s *ConnectionService) Update(ctx context.Context, connection domain.Connection, plaintextPassword string) (domain.Connection, error) {
	existing, err := s.GetByID(ctx, connection.ID)
	if err != nil {
		return domain.Connection{}, err
	}

	connection.CreatedAt = existing.CreatedAt
	if connection.Status == "" {
		connection.Status = existing.Status
	}
	if connection.RootPath == "" {
		connection.RootPath = existing.RootPath
	}
	if connection.TimeoutSec == 0 {
		connection.TimeoutSec = existing.TimeoutSec
	}
	if connection.TLSMode == "" {
		connection.TLSMode = existing.TLSMode
	}
	if err := connection.Validate(); err != nil {
		return domain.Connection{}, err
	}
	if connection.UpdatedAt.IsZero() {
		connection.UpdatedAt = time.Now().UTC()
	}

	if plaintextPassword != "" {
		ciphertext, err := s.secrets.EncryptString(plaintextPassword)
		if err != nil {
			return domain.Connection{}, err
		}
		connection.PasswordCiphertext = ciphertext
	} else {
		connection.PasswordCiphertext = existing.PasswordCiphertext
	}

	if err := s.repo.Update(ctx, connection); err != nil {
		return domain.Connection{}, err
	}

	return connection, nil
}
