package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoxuesen/fn-cloudsync/internal/api"
	"github.com/xiaoxuesen/fn-cloudsync/internal/app"
	"github.com/xiaoxuesen/fn-cloudsync/internal/connector/webdav"
	"github.com/xiaoxuesen/fn-cloudsync/internal/crypto"
	sqlitestore "github.com/xiaoxuesen/fn-cloudsync/internal/store/sqlite"
	appsync "github.com/xiaoxuesen/fn-cloudsync/internal/sync"
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

func TestConnectionTestEndpointPersistsCapabilities(t *testing.T) {
	t.Parallel()

	davServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodOptions:
			w.Header().Set("Allow", "OPTIONS, PROPFIND, MOVE")
			w.WriteHeader(http.StatusOK)
		case "PROPFIND":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:propstat>
      <d:prop>
        <d:getetag>"etag"</d:getetag>
        <d:getlastmodified>Mon, 24 Mar 2026 03:00:00 GMT</d:getlastmodified>
        <d:getcontentlength>0</d:getcontentlength>
      </d:prop>
    </d:propstat>
  </d:response>
</d:multistatus>`))
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer davServer.Close()

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

	createConnection := httptest.NewRequest(http.MethodPost, "/api/v1/connections", bytes.NewBufferString(`{"id":"conn-1","name":"primary","endpoint":"`+davServer.URL+`","username":"alice","password":"top-secret","root_path":"","tls_mode":"strict","timeout_sec":30,"status":"active"}`))
	createConnection.Header.Set("Content-Type", "application/json")
	createRecorder := httptest.NewRecorder()
	router.ServeHTTP(createRecorder, createConnection)
	if got, want := createRecorder.Code, http.StatusCreated; got != want {
		t.Fatalf("create connection status = %d, want %d", got, want)
	}

	testRecorder := httptest.NewRecorder()
	router.ServeHTTP(testRecorder, httptest.NewRequest(http.MethodPost, "/api/v1/connections/conn-1/test", nil))
	if got, want := testRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("test connection status = %d, want %d", got, want)
	}

	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/connections/conn-1", nil))
	if got, want := getRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("get connection status = %d, want %d", got, want)
	}

	var payload map[string]any
	if err := json.Unmarshal(testRecorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal(test response) error = %v", err)
	}
	if payload["success"] != true {
		t.Fatalf("success = %v, want true", payload["success"])
	}

	connection, err := sqlitestore.NewConnectionRepository(db).GetByID(context.Background(), "conn-1")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if connection.CapabilitiesJSON == "" {
		t.Fatal("CapabilitiesJSON empty after test endpoint")
	}
}

func TestTaskLifecycleEndpointsUpdateStatus(t *testing.T) {
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

	createTask := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{"id":"task-1","name":"sync-home","connection_id":"conn-1","local_path":"/tmp/local","remote_path":"/remote","direction":"upload"}`))
	createTask.Header.Set("Content-Type", "application/json")
	createTaskRecorder := httptest.NewRecorder()
	router.ServeHTTP(createTaskRecorder, createTask)
	if got, want := createTaskRecorder.Code, http.StatusCreated; got != want {
		t.Fatalf("create task status = %d, want %d", got, want)
	}

	startRecorder := httptest.NewRecorder()
	router.ServeHTTP(startRecorder, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/start", nil))
	if got, want := startRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("start task status = %d, want %d", got, want)
	}

	pauseRecorder := httptest.NewRecorder()
	router.ServeHTTP(pauseRecorder, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/pause", nil))
	if got, want := pauseRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("pause task status = %d, want %d", got, want)
	}

	stopRecorder := httptest.NewRecorder()
	router.ServeHTTP(stopRecorder, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/stop", nil))
	if got, want := stopRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("stop task status = %d, want %d", got, want)
	}
}

func TestStartUploadTaskPerformsBaselineSync(t *testing.T) {
	t.Parallel()

	localDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(localDir, "docs"), 0o755); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "docs", "readme.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(readme) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "report.txt"), []byte("payload"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(report) error = %v", err)
	}

	uploads := make(map[string]string)
	davServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "MKCOL":
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			uploads[r.URL.Path] = string(body)
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer davServer.Close()

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
	taskService.SetConnectionRepository(sqlitestore.NewConnectionRepository(db))
	taskService.SetSecrets(secrets)
	taskService.SetBaselineRunner(appsync.NewBaselineRunner(webdav.NewClient()))
	router := api.NewRouter(connectionService, taskService)

	createConnection := httptest.NewRequest(http.MethodPost, "/api/v1/connections", bytes.NewBufferString(`{"id":"conn-1","name":"primary","endpoint":"`+davServer.URL+`","username":"alice","password":"top-secret","root_path":"","tls_mode":"strict","timeout_sec":30,"status":"active"}`))
	createConnection.Header.Set("Content-Type", "application/json")
	createConnectionRecorder := httptest.NewRecorder()
	router.ServeHTTP(createConnectionRecorder, createConnection)
	if got, want := createConnectionRecorder.Code, http.StatusCreated; got != want {
		t.Fatalf("create connection status = %d, want %d", got, want)
	}

	createTask := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{"id":"task-1","name":"sync-home","connection_id":"conn-1","local_path":"`+localDir+`","remote_path":"/remote","direction":"upload"}`))
	createTask.Header.Set("Content-Type", "application/json")
	createTaskRecorder := httptest.NewRecorder()
	router.ServeHTTP(createTaskRecorder, createTask)
	if got, want := createTaskRecorder.Code, http.StatusCreated; got != want {
		t.Fatalf("create task status = %d, want %d", got, want)
	}

	startRecorder := httptest.NewRecorder()
	router.ServeHTTP(startRecorder, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/start", nil))
	if got, want := startRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("start task status = %d, want %d", got, want)
	}

	if uploads["/remote/report.txt"] != "payload" {
		t.Fatalf("uploaded /remote/report.txt = %q, want %q", uploads["/remote/report.txt"], "payload")
	}
	if uploads["/remote/docs/readme.txt"] != "hello" {
		t.Fatalf("uploaded /remote/docs/readme.txt = %q, want %q", uploads["/remote/docs/readme.txt"], "hello")
	}
}

func TestStartDownloadTaskPerformsBaselineSync(t *testing.T) {
	t.Parallel()

	localDir := t.TempDir()
	davServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			switch r.URL.Path {
			case "/remote":
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response><d:href>/remote</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
  <d:response><d:href>/remote/docs</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
  <d:response><d:href>/remote/report.txt</d:href><d:propstat><d:prop><d:resourcetype></d:resourcetype></d:prop></d:propstat></d:response>
</d:multistatus>`))
			case "/remote/docs":
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response><d:href>/remote/docs</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
  <d:response><d:href>/remote/docs/readme.txt</d:href><d:propstat><d:prop><d:resourcetype></d:resourcetype></d:prop></d:propstat></d:response>
</d:multistatus>`))
			default:
				http.Error(w, "unexpected path", http.StatusNotFound)
			}
		case http.MethodGet:
			switch r.URL.Path {
			case "/remote/report.txt":
				_, _ = w.Write([]byte("payload"))
			case "/remote/docs/readme.txt":
				_, _ = w.Write([]byte("hello"))
			default:
				http.Error(w, "unexpected path", http.StatusNotFound)
			}
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer davServer.Close()

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
	taskService.SetConnectionRepository(sqlitestore.NewConnectionRepository(db))
	taskService.SetSecrets(secrets)
	taskService.SetBaselineRunner(appsync.NewBaselineRunner(webdav.NewClient()))
	router := api.NewRouter(connectionService, taskService)

	createConnection := httptest.NewRequest(http.MethodPost, "/api/v1/connections", bytes.NewBufferString(`{"id":"conn-1","name":"primary","endpoint":"`+davServer.URL+`","username":"alice","password":"top-secret","root_path":"","tls_mode":"strict","timeout_sec":30,"status":"active"}`))
	createConnection.Header.Set("Content-Type", "application/json")
	createConnectionRecorder := httptest.NewRecorder()
	router.ServeHTTP(createConnectionRecorder, createConnection)
	if got, want := createConnectionRecorder.Code, http.StatusCreated; got != want {
		t.Fatalf("create connection status = %d, want %d", got, want)
	}

	createTask := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{"id":"task-1","name":"sync-home","connection_id":"conn-1","local_path":"`+localDir+`","remote_path":"/remote","direction":"download"}`))
	createTask.Header.Set("Content-Type", "application/json")
	createTaskRecorder := httptest.NewRecorder()
	router.ServeHTTP(createTaskRecorder, createTask)
	if got, want := createTaskRecorder.Code, http.StatusCreated; got != want {
		t.Fatalf("create task status = %d, want %d", got, want)
	}

	startRecorder := httptest.NewRecorder()
	router.ServeHTTP(startRecorder, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/start", nil))
	if got, want := startRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("start task status = %d, want %d", got, want)
	}

	report, err := os.ReadFile(filepath.Join(localDir, "report.txt"))
	if err != nil {
		t.Fatalf("os.ReadFile(report) error = %v", err)
	}
	if string(report) != "payload" {
		t.Fatalf("report.txt = %q, want %q", string(report), "payload")
	}
}

func TestStartBidirectionalTaskPerformsUploadAndDownload(t *testing.T) {
	t.Parallel()

	localDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(localDir, "local.txt"), []byte("local"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(local) error = %v", err)
	}
	uploads := make(map[string]string)
	davServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response><d:href>/remote</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
  <d:response><d:href>/remote/remote.txt</d:href><d:propstat><d:prop><d:resourcetype></d:resourcetype></d:prop></d:propstat></d:response>
</d:multistatus>`))
		case http.MethodGet:
			_, _ = w.Write([]byte("remote"))
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			uploads[r.URL.Path] = string(body)
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer davServer.Close()

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
	taskService.SetConnectionRepository(sqlitestore.NewConnectionRepository(db))
	taskService.SetSecrets(secrets)
	taskService.SetBaselineRunner(appsync.NewBaselineRunner(webdav.NewClient()))
	router := api.NewRouter(connectionService, taskService)

	createConnection := httptest.NewRequest(http.MethodPost, "/api/v1/connections", bytes.NewBufferString(`{"id":"conn-1","name":"primary","endpoint":"`+davServer.URL+`","username":"alice","password":"top-secret","root_path":"","tls_mode":"strict","timeout_sec":30,"status":"active"}`))
	createConnection.Header.Set("Content-Type", "application/json")
	createConnectionRecorder := httptest.NewRecorder()
	router.ServeHTTP(createConnectionRecorder, createConnection)
	if got, want := createConnectionRecorder.Code, http.StatusCreated; got != want {
		t.Fatalf("create connection status = %d, want %d", got, want)
	}

	createTask := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{"id":"task-1","name":"sync-home","connection_id":"conn-1","local_path":"`+localDir+`","remote_path":"/remote","direction":"bidirectional"}`))
	createTask.Header.Set("Content-Type", "application/json")
	createTaskRecorder := httptest.NewRecorder()
	router.ServeHTTP(createTaskRecorder, createTask)
	if got, want := createTaskRecorder.Code, http.StatusCreated; got != want {
		t.Fatalf("create task status = %d, want %d", got, want)
	}

	startRecorder := httptest.NewRecorder()
	router.ServeHTTP(startRecorder, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/start", nil))
	if got, want := startRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("start task status = %d, want %d", got, want)
	}

	if uploads["/remote/local.txt"] != "local" {
		t.Fatalf("uploaded /remote/local.txt = %q, want %q", uploads["/remote/local.txt"], "local")
	}
	remote, err := os.ReadFile(filepath.Join(localDir, "remote.txt"))
	if err != nil {
		t.Fatalf("os.ReadFile(remote) error = %v", err)
	}
	if string(remote) != "remote" {
		t.Fatalf("remote.txt = %q, want %q", string(remote), "remote")
	}
}
