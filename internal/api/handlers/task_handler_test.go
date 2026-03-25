package handlers_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/api"
	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

func TestCreateTaskReturnsCreated(t *testing.T) {
	t.Parallel()

	service := &stubTaskService{
		createResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionUpload,
			Status:       domain.TaskStatusCreated,
			CreatedAt:    time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
			UpdatedAt:    time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
		},
	}

	router := api.NewRouter(nil, service)
	body := bytes.NewBufferString(`{"id":"task-1","name":"sync-home","connection_id":"conn-1","local_path":"/tmp/local","remote_path":"/remote","direction":"upload"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusCreated; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestCreateTaskRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(nil, &stubTaskService{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString("{"))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestListTasksReturnsItems(t *testing.T) {
	t.Parallel()

	service := &stubTaskService{
		listResult: []domain.Task{
			{
				ID:           "task-1",
				Name:         "sync-home",
				ConnectionID: "conn-1",
				LocalPath:    "/tmp/local",
				RemotePath:   "/remote",
				Direction:    domain.TaskDirectionUpload,
				Status:       domain.TaskStatusCreated,
				CreatedAt:    time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
				UpdatedAt:    time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	router := api.NewRouter(nil, service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestGetTaskReturnsNotFound(t *testing.T) {
	t.Parallel()

	service := &stubTaskService{getErr: domain.ErrNotFound}
	router := api.NewRouter(nil, service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/missing", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestUpdateTaskReturnsOK(t *testing.T) {
	t.Parallel()

	service := &stubTaskService{
		updateResult: domain.Task{
			ID:           "task-1",
			Name:         "sync-home-updated",
			ConnectionID: "conn-1",
			LocalPath:    "/tmp/local",
			RemotePath:   "/remote",
			Direction:    domain.TaskDirectionUpload,
			Status:       domain.TaskStatusCreated,
			CreatedAt:    time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
			UpdatedAt:    time.Date(2026, 3, 24, 11, 0, 0, 0, time.UTC),
		},
	}
	router := api.NewRouter(nil, service)
	body := bytes.NewBufferString(`{"name":"sync-home-updated","connection_id":"conn-1","local_path":"/tmp/local","remote_path":"/remote","direction":"upload"}`)
	request := httptest.NewRequest(http.MethodPut, "/api/v1/tasks/task-1", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestDeleteTaskReturnsNoContent(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(nil, &stubTaskService{})
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/task-1", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusNoContent; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestStartTaskReturnsOK(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(nil, &stubTaskService{
		startResult: domain.Task{ID: "task-1", Status: domain.TaskStatusRunning},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/start", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestPauseTaskReturnsOK(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(nil, &stubTaskService{
		pauseResult: domain.Task{ID: "task-1", Status: domain.TaskStatusPaused},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/pause", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestStopTaskReturnsOK(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(nil, &stubTaskService{
		stopResult: domain.Task{ID: "task-1", Status: domain.TaskStatusStopped},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/stop", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestListTaskFailuresReturnsOK(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(nil, &stubTaskService{
		failuresResult: []domain.FailureRecord{
			{ID: "fail-1", TaskID: "task-1", Path: "report.txt", OpType: "UploadFile"},
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-1/failures", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestRetryTaskFailuresReturnsOK(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(nil, &stubTaskService{retryCount: 2})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/retry", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestRetryTaskFailureReturnsOK(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(nil, &stubTaskService{retryByIDCount: 1})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/failures/fail-1/retry", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestGetTaskRuntimeReturnsOK(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(nil, &stubTaskService{
		runtimeViewResult: domain.TaskRuntimeView{
			Task:           domain.Task{ID: "task-1", Status: domain.TaskStatusRunning},
			Runtime:        domain.TaskRuntimeState{Phase: "idle", CheckpointJSON: `{"remote":{"cursor":"etag-1"}}`},
			QueueSummary:   domain.TaskQueueSummary{Total: 2, Queued: 1, Succeeded: 1},
			FailureSummary: domain.TaskFailureSummary{Total: 1, Open: 1},
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-1/runtime", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(recorder.Body.String(), `"checkpoint_json":"{\"remote\":{\"cursor\":\"etag-1\"}}"`) {
		t.Fatalf("body = %s, want checkpoint_json", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"queued":1`) || !strings.Contains(recorder.Body.String(), `"succeeded":1`) {
		t.Fatalf("body = %s, want richer queue summary counts", recorder.Body.String())
	}
}

func TestGetMetricsReturnsOK(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(nil, &stubTaskService{
		metricsResult: domain.TaskMetrics{
			TaskStates: map[string]int{"running": 2, "degraded": 1},
			Queue:      domain.QueueMetrics{Total: 3, RetryWait: 1},
			Failures:   domain.FailureMetrics{Total: 2, RetryableOpen: 1},
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(recorder.Body.String(), `"running":2`) || !strings.Contains(recorder.Body.String(), `"retryable_open":1`) {
		t.Fatalf("body = %s, want aggregated metrics payload", recorder.Body.String())
	}
}

func TestListTaskEventsReturnsOK(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(nil, &stubTaskService{
		eventsResult: []domain.TaskEvent{
			{ID: "event-1", TaskID: "task-1", EventType: "task_started"},
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-1/events", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(recorder.Body.String(), `"event_type":"task_started"`) {
		t.Fatalf("body = %s, want task_started event", recorder.Body.String())
	}
}

func TestListTaskConflictsReturnsOK(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(nil, &stubTaskService{
		conflictsResult: []domain.ConflictRecord{
			{
				ID:                 "conflict-1",
				TaskID:             "task-1",
				RelativePath:       "docs/report.txt",
				LocalConflictPath:  "docs/report (local copy).txt",
				RemoteConflictPath: "docs/report (remote copy).txt",
				Policy:             "keep_both",
			},
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-1/conflicts", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(recorder.Body.String(), `"relative_path":"docs/report.txt"`) {
		t.Fatalf("body = %s, want relative_path payload", recorder.Body.String())
	}
}

type stubTaskService struct {
	createResult      domain.Task
	createErr         error
	listResult        []domain.Task
	listErr           error
	getResult         domain.Task
	getErr            error
	updateResult      domain.Task
	updateErr         error
	deleteErr         error
	startResult       domain.Task
	startErr          error
	pauseResult       domain.Task
	pauseErr          error
	stopResult        domain.Task
	stopErr           error
	failuresResult    []domain.FailureRecord
	failuresErr       error
	retryCount        int
	retryErr          error
	retryByIDCount    int
	retryByIDErr      error
	runtimeViewResult domain.TaskRuntimeView
	runtimeViewErr    error
	metricsResult     domain.TaskMetrics
	metricsErr        error
	eventsResult      []domain.TaskEvent
	eventsErr         error
	conflictsResult   []domain.ConflictRecord
	conflictsErr      error
}

func (s *stubTaskService) Create(_ context.Context, _ domain.Task) (domain.Task, error) {
	return s.createResult, s.createErr
}

func (s *stubTaskService) List(_ context.Context) ([]domain.Task, error) {
	return s.listResult, s.listErr
}

func (s *stubTaskService) GetByID(_ context.Context, _ string) (domain.Task, error) {
	return s.getResult, s.getErr
}

func (s *stubTaskService) Update(_ context.Context, task domain.Task) (domain.Task, error) {
	if s.updateResult.ID == "" {
		s.updateResult = task
	}
	return s.updateResult, s.updateErr
}

func (s *stubTaskService) Delete(_ context.Context, _ string) error {
	return s.deleteErr
}

func (s *stubTaskService) Start(_ context.Context, _ string) (domain.Task, error) {
	return s.startResult, s.startErr
}

func (s *stubTaskService) Pause(_ context.Context, _ string) (domain.Task, error) {
	return s.pauseResult, s.pauseErr
}

func (s *stubTaskService) Stop(_ context.Context, _ string) (domain.Task, error) {
	return s.stopResult, s.stopErr
}

func (s *stubTaskService) ListFailures(_ context.Context, _ string) ([]domain.FailureRecord, error) {
	return s.failuresResult, s.failuresErr
}

func (s *stubTaskService) RetryFailures(_ context.Context, _ string) (int, error) {
	return s.retryCount, s.retryErr
}

func (s *stubTaskService) RetryFailureByID(_ context.Context, _, _ string) (int, error) {
	return s.retryByIDCount, s.retryByIDErr
}

func (s *stubTaskService) GetRuntimeView(_ context.Context, _ string) (domain.TaskRuntimeView, error) {
	return s.runtimeViewResult, s.runtimeViewErr
}

func (s *stubTaskService) GetMetrics(_ context.Context) (domain.TaskMetrics, error) {
	return s.metricsResult, s.metricsErr
}

func (s *stubTaskService) ListEvents(_ context.Context, _ string, _ int) ([]domain.TaskEvent, error) {
	return s.eventsResult, s.eventsErr
}

func (s *stubTaskService) ListConflicts(_ context.Context, _ string) ([]domain.ConflictRecord, error) {
	return s.conflictsResult, s.conflictsErr
}
