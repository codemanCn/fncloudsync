package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type ConnectionRepository struct {
	db *sql.DB
}

func NewConnectionRepository(db *sql.DB) *ConnectionRepository {
	return &ConnectionRepository{db: db}
}

func (r *ConnectionRepository) Create(ctx context.Context, connection domain.Connection) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO connections (
			id, name, endpoint, username, password_ciphertext, root_path, tls_mode, timeout_sec, capabilities_json, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		connection.ID,
		connection.Name,
		connection.Endpoint,
		connection.Username,
		connection.PasswordCiphertext,
		connection.RootPath,
		connection.TLSMode,
		connection.TimeoutSec,
		connection.CapabilitiesJSON,
		connection.Status,
		connection.CreatedAt.UTC().Format(timestampLayout),
		connection.UpdatedAt.UTC().Format(timestampLayout),
	)
	return mapSQLError(err)
}

func (r *ConnectionRepository) GetByID(ctx context.Context, id string) (domain.Connection, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, endpoint, username, password_ciphertext, root_path, tls_mode, timeout_sec, capabilities_json, status, created_at, updated_at
		FROM connections
		WHERE id = ?
	`, id)

	connection, err := scanConnection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Connection{}, domain.ErrNotFound
	}
	return connection, err
}

func (r *ConnectionRepository) List(ctx context.Context) ([]domain.Connection, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, endpoint, username, password_ciphertext, root_path, tls_mode, timeout_sec, capabilities_json, status, created_at, updated_at
		FROM connections
		ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.Connection
	for rows.Next() {
		connection, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, connection)
	}

	return items, rows.Err()
}

func (r *ConnectionRepository) Update(ctx context.Context, connection domain.Connection) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE connections
		SET name = ?, endpoint = ?, username = ?, password_ciphertext = ?, root_path = ?, tls_mode = ?, timeout_sec = ?, capabilities_json = ?, status = ?, updated_at = ?
		WHERE id = ?
	`,
		connection.Name,
		connection.Endpoint,
		connection.Username,
		connection.PasswordCiphertext,
		connection.RootPath,
		connection.TLSMode,
		connection.TimeoutSec,
		connection.CapabilitiesJSON,
		connection.Status,
		connection.UpdatedAt.UTC().Format(timestampLayout),
		connection.ID,
	)
	if err != nil {
		return mapSQLError(err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *ConnectionRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM connections WHERE id = ?`, id)
	if err != nil {
		return mapSQLError(err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *ConnectionRepository) HasTasks(ctx context.Context, id string) (bool, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM tasks WHERE connection_id = ?`, id).Scan(&count); err != nil {
		return false, err
	}

	return count > 0, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanConnection(s scanner) (domain.Connection, error) {
	var connection domain.Connection
	var createdAt string
	var updatedAt string
	var tlsMode string

	err := s.Scan(
		&connection.ID,
		&connection.Name,
		&connection.Endpoint,
		&connection.Username,
		&connection.PasswordCiphertext,
		&connection.RootPath,
		&tlsMode,
		&connection.TimeoutSec,
		&connection.CapabilitiesJSON,
		&connection.Status,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return domain.Connection{}, err
	}

	connection.TLSMode = domain.TLSMode(tlsMode)
	connection.CreatedAt, err = parseTimestamp(createdAt)
	if err != nil {
		return domain.Connection{}, err
	}
	connection.UpdatedAt, err = parseTimestamp(updatedAt)
	if err != nil {
		return domain.Connection{}, err
	}

	return connection, nil
}
