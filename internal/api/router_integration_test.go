package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaoxuesen/fn-cloudsync/internal/api"
	"github.com/xiaoxuesen/fn-cloudsync/internal/app"
	"github.com/xiaoxuesen/fn-cloudsync/internal/crypto"
	sqlitestore "github.com/xiaoxuesen/fn-cloudsync/internal/store/sqlite"
	"github.com/xiaoxuesen/fn-cloudsync/testutil/testdb"
)

func TestConnectionTaskLifecycleRoundTrip(t *testing.T) {
	t.Parallel()

	db, err := sqlitestore.Open(testdb.Path(t))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := sqlitestore.Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	secrets, err := crypto.NewSecretManager("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretManager() error = %v", err)
	}

	connectionService, err := app.NewConnectionService(sqlitestore.NewConnectionRepository(db), secrets)
	if err != nil {
		t.Fatalf("NewConnectionService() error = %v", err)
	}
	taskService := app.NewTaskService(sqlitestore.NewTaskRepository(db))
	router := api.NewRouter(connectionService, taskService)

	createConnection := httptest.NewRequest(http.MethodPost, "/api/v1/connections", bytes.NewBufferString(`{"id":"conn-1","name":"primary","endpoint":"https://dav.example.com/root","username":"alice","password":"top-secret","root_path":"/","tls_mode":"strict","timeout_sec":30,"status":"active"}`))
	createConnection.Header.Set("Content-Type", "application/json")
	createConnectionRecorder := httptest.NewRecorder()
	router.ServeHTTP(createConnectionRecorder, createConnection)
	if got, want := createConnectionRecorder.Code, http.StatusCreated; got != want {
		t.Fatalf("create connection status = %d, want %d", got, want)
	}

	listConnectionsRecorder := httptest.NewRecorder()
	router.ServeHTTP(listConnectionsRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/connections", nil))
	if got, want := listConnectionsRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("list connections status = %d, want %d", got, want)
	}

	createTask := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{"id":"task-1","name":"sync-home","connection_id":"conn-1","local_path":"/tmp/local","remote_path":"/remote","direction":"upload"}`))
	createTask.Header.Set("Content-Type", "application/json")
	createTaskRecorder := httptest.NewRecorder()
	router.ServeHTTP(createTaskRecorder, createTask)
	if got, want := createTaskRecorder.Code, http.StatusCreated; got != want {
		t.Fatalf("create task status = %d, want %d", got, want)
	}

	deleteConnectionRecorder := httptest.NewRecorder()
	router.ServeHTTP(deleteConnectionRecorder, httptest.NewRequest(http.MethodDelete, "/api/v1/connections/conn-1", nil))
	if got, want := deleteConnectionRecorder.Code, http.StatusConflict; got != want {
		t.Fatalf("delete referenced connection status = %d, want %d", got, want)
	}

	listTasksRecorder := httptest.NewRecorder()
	router.ServeHTTP(listTasksRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil))
	if got, want := listTasksRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("list tasks status = %d, want %d", got, want)
	}

	var tasks []map[string]any
	if err := json.Unmarshal(listTasksRecorder.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("json.Unmarshal(tasks) error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("len(tasks) = %d, want 1", len(tasks))
	}

	updateConnection := httptest.NewRequest(http.MethodPut, "/api/v1/connections/conn-1", bytes.NewBufferString(`{"name":"primary-updated","endpoint":"https://dav.example.com/root","username":"alice","root_path":"/","tls_mode":"strict","timeout_sec":60,"status":"active"}`))
	updateConnection.Header.Set("Content-Type", "application/json")
	updateConnectionRecorder := httptest.NewRecorder()
	router.ServeHTTP(updateConnectionRecorder, updateConnection)
	if got, want := updateConnectionRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("update connection status = %d, want %d", got, want)
	}

	getConnectionRecorder := httptest.NewRecorder()
	router.ServeHTTP(getConnectionRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/connections/conn-1", nil))
	if got, want := getConnectionRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("get connection status = %d, want %d", got, want)
	}

	deleteTaskRecorder := httptest.NewRecorder()
	router.ServeHTTP(deleteTaskRecorder, httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/task-1", nil))
	if got, want := deleteTaskRecorder.Code, http.StatusNoContent; got != want {
		t.Fatalf("delete task status = %d, want %d", got, want)
	}

	deleteConnectionOKRecorder := httptest.NewRecorder()
	router.ServeHTTP(deleteConnectionOKRecorder, httptest.NewRequest(http.MethodDelete, "/api/v1/connections/conn-1", nil))
	if got, want := deleteConnectionOKRecorder.Code, http.StatusNoContent; got != want {
		t.Fatalf("delete connection after task removal status = %d, want %d", got, want)
	}
}
