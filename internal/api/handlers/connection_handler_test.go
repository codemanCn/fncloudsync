package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/api"
	"github.com/xiaoxuesen/fn-cloudsync/internal/domain"
)

func TestCreateConnectionReturnsCreatedAndRedactsPassword(t *testing.T) {
	t.Parallel()

	service := &stubConnectionService{
		createResult: domain.Connection{
			ID:                 "conn-1",
			Name:               "primary",
			Endpoint:           "https://dav.example.com/root",
			Username:           "alice",
			PasswordCiphertext: "encrypted",
			RootPath:           "/",
			TLSMode:            domain.TLSModeStrict,
			TimeoutSec:         30,
			Status:             "active",
			CreatedAt:          time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
			UpdatedAt:          time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
		},
	}

	router := api.NewRouter(service, nil)
	body := bytes.NewBufferString(`{"id":"conn-1","name":"primary","endpoint":"https://dav.example.com/root","username":"alice","password":"top-secret","root_path":"/","tls_mode":"strict","timeout_sec":30,"status":"active"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/connections", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusCreated; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if _, ok := response["password"]; ok {
		t.Fatal("response includes password field, want redacted response")
	}
}

func TestCreateConnectionRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(&stubConnectionService{}, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/connections", bytes.NewBufferString("{"))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestListConnectionsReturnsItems(t *testing.T) {
	t.Parallel()

	service := &stubConnectionService{
		listResult: []domain.Connection{
			{
				ID:        "conn-1",
				Name:      "primary",
				Endpoint:  "https://dav.example.com/root",
				Username:  "alice",
				RootPath:  "/",
				TLSMode:   domain.TLSModeStrict,
				CreatedAt: time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	router := api.NewRouter(service, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/connections", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestGetConnectionReturnsNotFound(t *testing.T) {
	t.Parallel()

	service := &stubConnectionService{getErr: domain.ErrNotFound}
	router := api.NewRouter(service, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/connections/missing", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestUpdateConnectionReturnsOK(t *testing.T) {
	t.Parallel()

	service := &stubConnectionService{
		updateResult: domain.Connection{
			ID:        "conn-1",
			Name:      "updated",
			Endpoint:  "https://dav.example.com/root",
			Username:  "alice",
			RootPath:  "/",
			TLSMode:   domain.TLSModeStrict,
			CreatedAt: time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 3, 24, 11, 0, 0, 0, time.UTC),
		},
	}
	router := api.NewRouter(service, nil)
	body := bytes.NewBufferString(`{"name":"updated","endpoint":"https://dav.example.com/root","username":"alice","password":"top-secret","root_path":"/","tls_mode":"strict","timeout_sec":30,"status":"active"}`)
	request := httptest.NewRequest(http.MethodPut, "/api/v1/connections/conn-1", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestDeleteConnectionReturnsNoContent(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(&stubConnectionService{}, nil)
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/connections/conn-1", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusNoContent; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestTestConnectionReturnsCapabilities(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(&stubConnectionService{
		testResult: domain.ConnectionTestResult{
			Success: true,
			Capabilities: domain.ConnectionCapabilities{
				SupportsETag: true,
				SupportsMove: true,
			},
		},
	}, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/connections/conn-1/test", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

type stubConnectionService struct {
	createResult domain.Connection
	createErr    error
	listResult   []domain.Connection
	listErr      error
	getResult    domain.Connection
	getErr       error
	updateResult domain.Connection
	updateErr    error
	deleteErr    error
	testResult   domain.ConnectionTestResult
	testErr      error
}

func (s *stubConnectionService) Create(_ context.Context, _ domain.Connection, _ string) (domain.Connection, error) {
	return s.createResult, s.createErr
}

func (s *stubConnectionService) List(_ context.Context) ([]domain.Connection, error) {
	return s.listResult, s.listErr
}

func (s *stubConnectionService) GetByID(_ context.Context, _ string) (domain.Connection, error) {
	return s.getResult, s.getErr
}

func (s *stubConnectionService) Update(_ context.Context, connection domain.Connection, _ string) (domain.Connection, error) {
	if s.updateResult.ID == "" {
		s.updateResult = connection
	}
	return s.updateResult, s.updateErr
}

func (s *stubConnectionService) Delete(_ context.Context, _ string) error {
	return s.deleteErr
}

func (s *stubConnectionService) TestConnection(_ context.Context, _ string) (domain.ConnectionTestResult, error) {
	return s.testResult, s.testErr
}
