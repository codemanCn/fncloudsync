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
	GetMetrics(context.Context) (domain.TaskMetrics, error)
	ListEvents(context.Context, string, int) ([]domain.TaskEvent, error)
	ListConflicts(context.Context, string) ([]domain.ConflictRecord, error)
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
	ID            string    `json:"id"`
	TaskID        string    `json:"task_id"`
	Path          string    `json:"path"`
	OpType        string    `json:"op_type"`
	ErrorCode     string    `json:"error_code"`
	ErrorMessage  string    `json:"error_message"`
	Retryable     bool      `json:"retryable"`
	FirstFailedAt time.Time `json:"first_failed_at"`
	LastFailedAt  time.Time `json:"last_failed_at"`
	AttemptCount  int       `json:"attempt_count"`
	ResolvedAt    time.Time `json:"resolved_at"`
}

type eventResponse struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	EventType   string    `json:"event_type"`
	Level       string    `json:"level"`
	Message     string    `json:"message"`
	DetailsJSON string    `json:"details_json"`
	CreatedAt   time.Time `json:"created_at"`
}

type taskRuntimeResponse struct {
	Task           taskResponse           `json:"task"`
	Runtime        runtimeResponse        `json:"runtime"`
	QueueSummary   queueSummaryResponse   `json:"queue_summary"`
	FailureSummary failureSummaryResponse `json:"failure_summary"`
	Queue          []queueItemResponse    `json:"queue"`
	Failures       []failureResponse      `json:"failures"`
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
	CheckpointJSON   string    `json:"checkpoint_json"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type queueSummaryResponse struct {
	Total     int `json:"total"`
	Queued    int `json:"queued"`
	Pending   int `json:"pending"`
	Executing int `json:"executing"`
	RetryWait int `json:"retry_wait"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
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

type metricsResponse struct {
	TaskStates map[string]int       `json:"task_states"`
	Queue      queueMetricsResponse `json:"queue"`
	Failures   failureMetricsResp   `json:"failures"`
}

type queueMetricsResponse struct {
	Total     int `json:"total"`
	Queued    int `json:"queued"`
	Executing int `json:"executing"`
	RetryWait int `json:"retry_wait"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

type failureMetricsResp struct {
	Total         int `json:"total"`
	Open          int `json:"open"`
	Resolved      int `json:"resolved"`
	RetryableOpen int `json:"retryable_open"`
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
				CheckpointJSON:   view.Runtime.CheckpointJSON,
				UpdatedAt:        view.Runtime.UpdatedAt,
			},
			QueueSummary: queueSummaryResponse{
				Total:     view.QueueSummary.Total,
				Queued:    view.QueueSummary.Queued,
				Pending:   view.QueueSummary.Pending,
				Executing: view.QueueSummary.Executing,
				RetryWait: view.QueueSummary.RetryWait,
				Succeeded: view.QueueSummary.Succeeded,
				Failed:    view.QueueSummary.Failed,
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

func ListTaskEvents(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := service.ListEvents(r.Context(), chi.URLParam(r, "taskID"), 100)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, domain.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		response := make([]eventResponse, 0, len(items))
		for _, item := range items {
			response = append(response, eventResponse{
				ID:          item.ID,
				TaskID:      item.TaskID,
				EventType:   item.EventType,
				Level:       item.Level,
				Message:     item.Message,
				DetailsJSON: item.DetailsJSON,
				CreatedAt:   item.CreatedAt,
			})
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func GetMetrics(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		metrics, err := service.GetMetrics(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, metricsResponse{
			TaskStates: metrics.TaskStates,
			Queue: queueMetricsResponse{
				Total:     metrics.Queue.Total,
				Queued:    metrics.Queue.Queued,
				Executing: metrics.Queue.Executing,
				RetryWait: metrics.Queue.RetryWait,
				Succeeded: metrics.Queue.Succeeded,
				Failed:    metrics.Queue.Failed,
			},
			Failures: failureMetricsResp{
				Total:         metrics.Failures.Total,
				Open:          metrics.Failures.Open,
				Resolved:      metrics.Failures.Resolved,
				RetryableOpen: metrics.Failures.RetryableOpen,
			},
		})
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

type conflictResponse struct {
	ID                 string    `json:"id"`
	TaskID             string    `json:"task_id"`
	RelativePath       string    `json:"relative_path"`
	LocalConflictPath  string    `json:"local_conflict_path"`
	RemoteConflictPath string    `json:"remote_conflict_path"`
	Policy             string    `json:"policy"`
	DetectedAt         time.Time `json:"detected_at"`
}

func ListTaskConflicts(service taskCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := service.ListConflicts(r.Context(), chi.URLParam(r, "taskID"))
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, domain.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}

		response := make([]conflictResponse, 0, len(items))
		for _, item := range items {
			response = append(response, conflictResponse{
				ID:                 item.ID,
				TaskID:             item.TaskID,
				RelativePath:       item.RelativePath,
				LocalConflictPath:  item.LocalConflictPath,
				RemoteConflictPath: item.RemoteConflictPath,
				Policy:             item.Policy,
				DetectedAt:         item.DetectedAt,
			})
		}
		writeJSON(w, http.StatusOK, response)
	}
}
