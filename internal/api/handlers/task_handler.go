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
	ListFailures(context.Context, string) ([]domain.FailureRecord, error)
	RetryFailures(context.Context, string) (int, error)
	RetryFailureByID(context.Context, string, string) (int, error)
	GetRuntimeView(context.Context, string) (domain.TaskRuntimeView, error)
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

type failureResponse struct {
	ID           string    `json:"id"`
	TaskID       string    `json:"task_id"`
	Path         string    `json:"path"`
	OpType       string    `json:"op_type"`
	ErrorCode    string    `json:"error_code"`
	ErrorMessage string    `json:"error_message"`
	Retryable    bool      `json:"retryable"`
	FirstFailedAt time.Time `json:"first_failed_at"`
	LastFailedAt time.Time `json:"last_failed_at"`
	AttemptCount int       `json:"attempt_count"`
	ResolvedAt   time.Time `json:"resolved_at"`
}

type taskRuntimeResponse struct {
	Task           taskResponse       `json:"task"`
	Runtime        runtimeResponse    `json:"runtime"`
	QueueSummary   queueSummaryResponse `json:"queue_summary"`
	FailureSummary failureSummaryResponse `json:"failure_summary"`
	Queue          []queueItemResponse `json:"queue"`
	Failures       []failureResponse   `json:"failures"`
}

type runtimeResponse struct {
	Phase            string    `json:"phase"`
	LastLocalScanAt  time.Time `json:"last_local_scan_at"`
	LastRemoteScanAt time.Time `json:"last_remote_scan_at"`
	LastReconcileAt  time.Time `json:"last_reconcile_at"`
	LastSuccessAt    time.Time `json:"last_success_at"`
	BackoffUntil     time.Time `json:"backoff_until"`
	RetryStreak      int       `json:"retry_streak"`
	LastError        string    `json:"last_error"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type queueSummaryResponse struct {
	Total     int `json:"total"`
	Pending   int `json:"pending"`
	Executing int `json:"executing"`
	RetryWait int `json:"retry_wait"`
}

type failureSummaryResponse struct {
	Total    int `json:"total"`
	Resolved int `json:"resolved"`
	Open     int `json:"open"`
}

type queueItemResponse struct {
	ID            string    `json:"id"`
	OpType        string    `json:"op_type"`
	TargetPath    string    `json:"target_path"`
	Status        string    `json:"status"`
	AttemptCount  int       `json:"attempt_count"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	LastError     string    `json:"last_error"`
	UpdatedAt     time.Time `json:"updated_at"`
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

func GetTaskRuntime(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view, err := service.GetRuntimeView(r.Context(), chi.URLParam(r, "taskID"))
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, domain.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}

		response := taskRuntimeResponse{
			Task: toTaskResponse(view.Task),
			Runtime: runtimeResponse{
				Phase:            view.Runtime.Phase,
				LastLocalScanAt:  view.Runtime.LastLocalScanAt,
				LastRemoteScanAt: view.Runtime.LastRemoteScanAt,
				LastReconcileAt:  view.Runtime.LastReconcileAt,
				LastSuccessAt:    view.Runtime.LastSuccessAt,
				BackoffUntil:     view.Runtime.BackoffUntil,
				RetryStreak:      view.Runtime.RetryStreak,
				LastError:        view.Runtime.LastError,
				UpdatedAt:        view.Runtime.UpdatedAt,
			},
			QueueSummary: queueSummaryResponse{
				Total:     view.QueueSummary.Total,
				Pending:   view.QueueSummary.Pending,
				Executing: view.QueueSummary.Executing,
				RetryWait: view.QueueSummary.RetryWait,
			},
			FailureSummary: failureSummaryResponse{
				Total:    view.FailureSummary.Total,
				Resolved: view.FailureSummary.Resolved,
				Open:     view.FailureSummary.Open,
			},
		}
		for _, item := range view.Queue {
			response.Queue = append(response.Queue, queueItemResponse{
				ID:            item.ID,
				OpType:        item.OpType,
				TargetPath:    item.TargetPath,
				Status:        item.Status,
				AttemptCount:  item.AttemptCount,
				NextAttemptAt: item.NextAttemptAt,
				LastError:     item.LastError,
				UpdatedAt:     item.UpdatedAt,
			})
		}
		for _, item := range view.Failures {
			response.Failures = append(response.Failures, failureResponse{
				ID:            item.ID,
				TaskID:        item.TaskID,
				Path:          item.Path,
				OpType:        item.OpType,
				ErrorCode:     item.ErrorCode,
				ErrorMessage:  item.ErrorMessage,
				Retryable:     item.Retryable,
				FirstFailedAt: item.FirstFailedAt,
				LastFailedAt:  item.LastFailedAt,
				AttemptCount:  item.AttemptCount,
				ResolvedAt:    item.ResolvedAt,
			})
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func ListTaskFailures(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := service.ListFailures(r.Context(), chi.URLParam(r, "taskID"))
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, domain.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}

		response := make([]failureResponse, 0, len(items))
		for _, item := range items {
			response = append(response, failureResponse{
				ID:            item.ID,
				TaskID:        item.TaskID,
				Path:          item.Path,
				OpType:        item.OpType,
				ErrorCode:     item.ErrorCode,
				ErrorMessage:  item.ErrorMessage,
				Retryable:     item.Retryable,
				FirstFailedAt: item.FirstFailedAt,
				LastFailedAt:  item.LastFailedAt,
				AttemptCount:  item.AttemptCount,
				ResolvedAt:    item.ResolvedAt,
			})
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func RetryTaskFailures(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		count, err := service.RetryFailures(r.Context(), chi.URLParam(r, "taskID"))
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, domain.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"retried": count})
	}
}

func RetryTaskFailure(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		count, err := service.RetryFailureByID(r.Context(), chi.URLParam(r, "taskID"), chi.URLParam(r, "failureID"))
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, domain.ErrNotFound) {
				status = http.StatusNotFound
			}
			if errors.Is(err, domain.ErrConflict) {
				status = http.StatusConflict
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"retried": count})
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
