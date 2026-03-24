package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

type connectionCreator interface {
	Create(context.Context, domain.Connection, string) (domain.Connection, error)
	List(context.Context) ([]domain.Connection, error)
	GetByID(context.Context, string) (domain.Connection, error)
	Update(context.Context, domain.Connection, string) (domain.Connection, error)
	Delete(context.Context, string) error
	TestConnection(context.Context, string) (domain.ConnectionTestResult, error)
}

type createConnectionRequest struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Endpoint   string `json:"endpoint"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	RootPath   string `json:"root_path"`
	TLSMode    string `json:"tls_mode"`
	TimeoutSec int    `json:"timeout_sec"`
	Status     string `json:"status"`
}

type connectionResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Endpoint   string    `json:"endpoint"`
	Username   string    `json:"username"`
	RootPath   string    `json:"root_path"`
	TLSMode    string    `json:"tls_mode"`
	TimeoutSec int       `json:"timeout_sec"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func CreateConnection(service connectionCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request createConnectionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}

		connection, err := service.Create(r.Context(), domain.Connection{
			ID:         request.ID,
			Name:       request.Name,
			Endpoint:   request.Endpoint,
			Username:   request.Username,
			RootPath:   request.RootPath,
			TLSMode:    domain.TLSMode(request.TLSMode),
			TimeoutSec: request.TimeoutSec,
			Status:     request.Status,
		}, request.Password)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, connectionResponse{
			ID:         connection.ID,
			Name:       connection.Name,
			Endpoint:   connection.Endpoint,
			Username:   connection.Username,
			RootPath:   connection.RootPath,
			TLSMode:    string(connection.TLSMode),
			TimeoutSec: connection.TimeoutSec,
			Status:     connection.Status,
			CreatedAt:  connection.CreatedAt,
			UpdatedAt:  connection.UpdatedAt,
		})
	}
}

func ListConnections(service connectionCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := service.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		response := make([]connectionResponse, 0, len(items))
		for _, connection := range items {
			response = append(response, toConnectionResponse(connection))
		}

		writeJSON(w, http.StatusOK, response)
	}
}

func GetConnection(service connectionCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		connection, err := service.GetByID(r.Context(), chi.URLParam(r, "connectionID"))
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, toConnectionResponse(connection))
	}
}

func UpdateConnection(service connectionCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request createConnectionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}

		connection, err := service.Update(r.Context(), domain.Connection{
			ID:         chi.URLParam(r, "connectionID"),
			Name:       request.Name,
			Endpoint:   request.Endpoint,
			Username:   request.Username,
			RootPath:   request.RootPath,
			TLSMode:    domain.TLSMode(request.TLSMode),
			TimeoutSec: request.TimeoutSec,
			Status:     request.Status,
		}, request.Password)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, domain.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, toConnectionResponse(connection))
	}
}

func DeleteConnection(service connectionCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := service.Delete(r.Context(), chi.URLParam(r, "connectionID"))
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, domain.ErrNotFound):
				status = http.StatusNotFound
			case errors.Is(err, domain.ErrReferencedResource):
				status = http.StatusConflict
			}
			writeError(w, status, err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func TestConnection(service connectionCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.TestConnection(r.Context(), chi.URLParam(r, "connectionID"))
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, domain.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}

func toConnectionResponse(connection domain.Connection) connectionResponse {
	return connectionResponse{
		ID:         connection.ID,
		Name:       connection.Name,
		Endpoint:   connection.Endpoint,
		Username:   connection.Username,
		RootPath:   connection.RootPath,
		TLSMode:    string(connection.TLSMode),
		TimeoutSec: connection.TimeoutSec,
		Status:     connection.Status,
		CreatedAt:  connection.CreatedAt,
		UpdatedAt:  connection.UpdatedAt,
	}
}
