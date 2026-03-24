# fn-cloudsync

Go backend for a single-node WebDAV cloud sync controller.

## Environment

- Go `1.24.2`
- Default server address: `:8080`
- Runtime env vars:
  - `APP_ADDR`
  - `APP_DB_PATH`
  - `APP_SECRET_KEY` (required, 32 bytes for AES-GCM)

## Commands

```bash
make test
make test-short
make fmt
make run
```

## Run

```bash
APP_SECRET_KEY=0123456789abcdef0123456789abcdef make run
```

Optional:

```bash
APP_ADDR=:8080
APP_DB_PATH=/tmp/fn-cloudsync.db
```

Health check:

```bash
curl http://127.0.0.1:8080/healthz
```

Expected response:

```json
{"status":"ok"}
```

## Current scope

Implemented:
- `GET /healthz`
- `POST/GET/GET{id}/PUT/DELETE /api/v1/connections`
- `POST /api/v1/connections/{id}/test`
- `POST/GET/GET{id}/PUT/DELETE /api/v1/tasks`
- `POST /api/v1/tasks/{id}/start`
- `POST /api/v1/tasks/{id}/pause`
- `POST /api/v1/tasks/{id}/stop`
- `GET /api/v1/tasks/{id}/runtime`
- `GET /api/v1/tasks/{id}/failures`
- `POST /api/v1/tasks/{id}/retry`
- `POST /api/v1/tasks/{id}/failures/{failure_id}/retry`
- SQLite bootstrap and migrations for `connections` and `tasks`
- Password-at-rest encryption before connection persistence
- Connection delete protection when tasks still reference the connection
- Router-to-SQLite integration tests for the control plane
- Minimal WebDAV capability probe via `OPTIONS` + `PROPFIND`
- Upload/download/bidirectional baseline sync execution
- Background scheduler with immediate recovery scan on startup
- Local fsnotify watcher for running upload/bidirectional tasks, with periodic reconcile as fallback
- Persistent `task_runtime_state`, `operation_queue`, and `failure_records`
- Persistent `file_index` metadata for synced objects
- Queue retry consumption with backoff rescheduling
- Single-direction mirror delete propagation
- Bidirectional conflict handling for `prefer_local`, `prefer_remote`, and `keep_both`
- Conservative bidirectional mirror delete propagation gated by prior `file_index` evidence

Not implemented yet:
- advanced conflict history, richer planner/executor states, and resumable chunked transfer
- remote incremental poll model beyond periodic reconcile
- metrics, frontend, and admin UX

## Runtime API

Task runtime view:

```bash
curl http://127.0.0.1:8080/api/v1/tasks/task-1/runtime
```

Failure list:

```bash
curl http://127.0.0.1:8080/api/v1/tasks/task-1/failures
```

Batch retry:

```bash
curl -X POST http://127.0.0.1:8080/api/v1/tasks/task-1/retry
```

Retry a specific failure:

```bash
curl -X POST http://127.0.0.1:8080/api/v1/tasks/task-1/failures/fail-1/retry
```

OpenAPI spec:
- [openapi.yaml](/Users/xiaoxuesen/LLM/fn-cloudsync/docs/openapi/openapi.yaml)
