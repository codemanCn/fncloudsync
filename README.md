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
- `POST/GET/GET{id}/PUT/DELETE /api/v1/tasks`
- SQLite bootstrap and migrations for `connections` and `tasks`
- Password-at-rest encryption before connection persistence
- Connection delete protection when tasks still reference the connection
- Router-to-SQLite integration tests for the control plane

Not implemented yet:
- connection test endpoint
- task lifecycle execution
- WebDAV connector
- sync engine, watcher, scheduler, metrics, frontend
