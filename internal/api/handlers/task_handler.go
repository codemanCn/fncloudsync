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

type taskCreator interface {
	Create(context.Context, domain.Task) (domain.Task, error)
	List(context.Context) ([]domain.Task, error)
	GetByID(context.Context, string) (domain.Task, error)
	Update(context.Context, domain.Task) (domain.Task, error)
	Delete(context.Context, string) error
	Start(context.Context, string) (domain.Task, error)
	Pause(context.Context, string) (domain.Task, error)
	Stop(context.Context, string) (domain.Task, error)
}

type createTaskRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ConnectionID string `json:"connection_id"`
	LocalPath    string `json:"local_path"`
	RemotePath   string `json:"remote_path"`
	Direction    string `json:"direction"`
}

type taskResponse struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ConnectionID string    `json:"connection_id"`
	LocalPath    string    `json:"local_path"`
	RemotePath   string    `json:"remote_path"`
	Direction    string    `json:"direction"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func CreateTask(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request createTaskRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}

		task, err := service.Create(r.Context(), domain.Task{
			ID:           request.ID,
			Name:         request.Name,
			ConnectionID: request.ConnectionID,
			LocalPath:    request.LocalPath,
			RemotePath:   request.RemotePath,
			Direction:    domain.TaskDirection(request.Direction),
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, taskResponse{
			ID:           task.ID,
			Name:         task.Name,
			ConnectionID: task.ConnectionID,
			LocalPath:    task.LocalPath,
			RemotePath:   task.RemotePath,
			Direction:    string(task.Direction),
			Status:       string(task.Status),
			CreatedAt:    task.CreatedAt,
			UpdatedAt:    task.UpdatedAt,
		})
	}
}

func ListTasks(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := service.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		response := make([]taskResponse, 0, len(items))
		for _, task := range items {
			response = append(response, toTaskResponse(task))
		}

		writeJSON(w, http.StatusOK, response)
	}
}

func GetTask(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		task, err := service.GetByID(r.Context(), chi.URLParam(r, "taskID"))
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, toTaskResponse(task))
	}
}

func UpdateTask(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request createTaskRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}

		task, err := service.Update(r.Context(), domain.Task{
			ID:           chi.URLParam(r, "taskID"),
			Name:         request.Name,
			ConnectionID: request.ConnectionID,
			LocalPath:    request.LocalPath,
			RemotePath:   request.RemotePath,
			Direction:    domain.TaskDirection(request.Direction),
		})
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, domain.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, toTaskResponse(task))
	}
}

func DeleteTask(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := service.Delete(r.Context(), chi.URLParam(r, "taskID"))
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func StartTask(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		task, err := service.Start(r.Context(), chi.URLParam(r, "taskID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, toTaskResponse(task))
	}
}

func PauseTask(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		task, err := service.Pause(r.Context(), chi.URLParam(r, "taskID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, toTaskResponse(task))
	}
}

func StopTask(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		task, err := service.Stop(r.Context(), chi.URLParam(r, "taskID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, toTaskResponse(task))
	}
}

func toTaskResponse(task domain.Task) taskResponse {
	return taskResponse{
		ID:           task.ID,
		Name:         task.Name,
		ConnectionID: task.ConnectionID,
		LocalPath:    task.LocalPath,
		RemotePath:   task.RemotePath,
		Direction:    string(task.Direction),
		Status:       string(task.Status),
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
	}
}
