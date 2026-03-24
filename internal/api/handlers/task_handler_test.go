package handlers_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
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

type stubTaskService struct {
	createResult domain.Task
	createErr    error
	listResult   []domain.Task
	listErr      error
	getResult    domain.Task
	getErr       error
	updateResult domain.Task
	updateErr    error
	deleteErr    error
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
