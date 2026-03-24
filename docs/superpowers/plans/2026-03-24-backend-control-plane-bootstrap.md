# Backend Control Plane Bootstrap Implementation Plan

## Status

Completed on 2026-03-24.

Delivered scope:
- runnable Go service skeleton with `cmd/server`, config, logger, and SQLite bootstrap
- connection/task CRUD API
- connection password encryption
- connection test endpoint
- control-plane integration tests

Follow-up work moved beyond this plan:
- runtime execution
- queue, watcher, scheduler, and failure handling
- runtime/failure/retry HTTP APIs

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first runnable Go backend skeleton for the WebDAV cloud sync product with health check, connection CRUD, task CRUD, SQLite persistence, and validation-ready service boundaries.

**Architecture:** The server starts as a single Go binary with explicit layers: `cmd/server` for wiring, `internal/api` for `chi` handlers and JSON contracts, `internal/app` for business services, `internal/domain` for core models and enums, `internal/store/sqlite` for migrations and repositories, and `internal/crypto` for password-at-rest encryption. Runtime task execution is intentionally deferred; task status is stored as control-plane metadata only.

**Tech Stack:** Go, `chi`, SQLite, `database/sql`, `github.com/mattn/go-sqlite3`, `httptest`, table-driven tests

---

## File Structure

### New directories and files

- `go.mod`
- `go.sum`
- `cmd/server/main.go`
- `internal/config/config.go`
- `internal/domain/connection.go`
- `internal/domain/task.go`
- `internal/domain/errors.go`
- `internal/app/connection_service.go`
- `internal/app/task_service.go`
- `internal/api/router.go`
- `internal/api/handlers/health_handler.go`
- `internal/api/handlers/connection_handler.go`
- `internal/api/handlers/task_handler.go`
- `internal/api/handlers/helpers.go`
- `internal/api/dto/connection.go`
- `internal/api/dto/task.go`
- `internal/store/sqlite/db.go`
- `internal/store/sqlite/migrations.go`
- `internal/store/sqlite/connection_repository.go`
- `internal/store/sqlite/task_repository.go`
- `internal/crypto/secrets.go`
- `internal/obs/logger.go`
- `testutil/testdb/testdb.go`
- `internal/app/connection_service_test.go`
- `internal/app/task_service_test.go`
- `internal/store/sqlite/connection_repository_test.go`
- `internal/store/sqlite/task_repository_test.go`
- `internal/api/handlers/health_handler_test.go`
- `internal/api/handlers/connection_handler_test.go`
- `internal/api/handlers/task_handler_test.go`
- `.gitignore`
- `Makefile`
- `README.md`

### Responsibilities

- `internal/domain`: source of truth for entities, enum values, and typed business errors.
- `internal/app`: orchestration, validation, referential integrity checks, and repository-facing use cases.
- `internal/store/sqlite`: schema migration, `SQLite + WAL` initialization, and persistence adapters.
- `internal/api`: transport-only concerns, request decoding, response encoding, and HTTP error mapping.
- `internal/crypto`: symmetric encryption wrapper for `password_ciphertext` so plaintext never hits disk.
- `testutil/testdb`: isolated temporary SQLite helpers reused by repository and service tests.

## Task 1: Initialize the Go service skeleton

**Files:**
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/go.mod`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/.gitignore`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/Makefile`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/README.md`

- [ ] **Step 1: Write the failing smoke test command expectation**

Document the expected initial failure in `README.md`: running `go test ./...` should fail before any package exists because the module is not initialized.

- [ ] **Step 2: Verify the workspace has no Go module yet**

Run: `go test ./...`
Expected: FAIL with a module/package setup error.

- [ ] **Step 3: Create the minimal module and workspace defaults**

Implement:
- `go mod init` using a stable module path for this repository.
- `.gitignore` entries for binaries, `.db`, `.db-shm`, `.db-wal`, coverage, and local env files.
- `Makefile` targets: `test`, `test-short`, `run`, `fmt`.
- `README.md` with bootstrap commands and environment variables.

- [ ] **Step 4: Verify the empty module baseline**

Run: `go test ./...`
Expected: PASS or report no packages to test, without module errors.

- [ ] **Step 5: Commit**

```bash
git add go.mod .gitignore Makefile README.md
git commit -m "chore: initialize golang service workspace"
```

## Task 2: Define domain models and business errors

**Files:**
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/domain/connection.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/domain/task.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/domain/errors.go`
- Test: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/app/connection_service_test.go`
- Test: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/app/task_service_test.go`

- [ ] **Step 1: Write failing tests for validation-facing domain behavior**

Cover:
- connection requires `name`, `endpoint`, `username`
- task requires `name`, `connection_id`, `local_path`, `remote_path`
- task `direction` only accepts `upload`, `download`, `bidirectional`
- task `status` defaults to `created`

- [ ] **Step 2: Run the targeted tests and confirm RED**

Run: `go test ./internal/app -run 'TestConnectionService|TestTaskService' -v`
Expected: FAIL because domain types and services do not exist yet.

- [ ] **Step 3: Implement minimal domain types and typed errors**

Implement:
- `Connection`, `Task`
- `TLSMode`, `TaskDirection`, `TaskStatus`
- typed sentinel errors such as `ErrInvalidArgument`, `ErrNotFound`, `ErrConflict`, `ErrReferencedResource`

- [ ] **Step 4: Re-run tests to keep focus on domain/service compilation**

Run: `go test ./internal/app -run 'TestConnectionService|TestTaskService' -v`
Expected: FAIL now only on missing service logic, not missing domain types.

- [ ] **Step 5: Commit**

```bash
git add internal/domain internal/app/*_test.go
git commit -m "feat: define core domain models"
```

## Task 3: Build SQLite bootstrap and migrations

**Files:**
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/store/sqlite/db.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/store/sqlite/migrations.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/testutil/testdb/testdb.go`
- Test: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/store/sqlite/connection_repository_test.go`
- Test: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/store/sqlite/task_repository_test.go`

- [ ] **Step 1: Write failing repository bootstrap tests**

Cover:
- opening DB enables WAL mode
- migrations create `schema_migrations`, `connections`, `tasks`
- deleting a referenced connection is blocked by FK or service-level check

- [ ] **Step 2: Run the repository tests and confirm RED**

Run: `go test ./internal/store/sqlite -run 'TestOpen|TestMigrate' -v`
Expected: FAIL because bootstrap code does not exist.

- [ ] **Step 3: Implement DB open and migration code**

Implement:
- `Open(path string) (*sql.DB, error)`
- PRAGMA setup for foreign keys and WAL
- migration runner with ordered SQL statements
- `connections` and `tasks` schema aligned to the approved first-slice fields

- [ ] **Step 4: Re-run bootstrap tests**

Run: `go test ./internal/store/sqlite -run 'TestOpen|TestMigrate' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/sqlite testutil/testdb
git commit -m "feat: add sqlite bootstrap and migrations"
```

## Task 4: Implement repositories for connections and tasks

**Files:**
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/store/sqlite/connection_repository_test.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/store/sqlite/task_repository_test.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/store/sqlite/connection_repository.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/store/sqlite/task_repository.go`

- [ ] **Step 1: Write failing CRUD tests**

Cover:
- create/list/get/update/delete connection
- create/list/get/update/delete task
- list ordering is deterministic by `created_at desc, id desc` or equivalent documented order
- task creation fails when `connection_id` does not exist

- [ ] **Step 2: Run the repository CRUD tests**

Run: `go test ./internal/store/sqlite -run 'TestConnectionRepository|TestTaskRepository' -v`
Expected: FAIL with missing repository implementations.

- [ ] **Step 3: Implement minimal repositories**

Implement:
- repository interfaces close to CRUD use cases only
- UTC timestamp handling
- password stored only as ciphertext bytes/string
- `updated_at` refresh on updates

- [ ] **Step 4: Re-run CRUD tests**

Run: `go test ./internal/store/sqlite -run 'TestConnectionRepository|TestTaskRepository' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/sqlite
git commit -m "feat: add sqlite repositories for connections and tasks"
```

## Task 5: Add secrets encryption wrapper

**Files:**
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/crypto/secrets.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/app/connection_service_test.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/store/sqlite/connection_repository_test.go`

- [ ] **Step 1: Write failing tests for password-at-rest behavior**

Cover:
- service encrypts password before persisting
- reading a connection returns decrypted password only when explicitly requested by service logic, not in API DTO
- empty encryption key fails fast at startup or service construction

- [ ] **Step 2: Run the targeted tests**

Run: `go test ./internal/app ./internal/store/sqlite -run 'TestConnectionService|TestConnectionRepository' -v`
Expected: FAIL because crypto wrapper is missing.

- [ ] **Step 3: Implement minimal symmetric secret manager**

Implement:
- AES-GCM or XChaCha20-Poly1305 wrapper
- methods `EncryptString` and `DecryptString`
- base64-encoded ciphertext payload with nonce included

- [ ] **Step 4: Re-run targeted tests**

Run: `go test ./internal/app ./internal/store/sqlite -run 'TestConnectionService|TestConnectionRepository' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/crypto internal/app internal/store/sqlite
git commit -m "feat: encrypt stored connection passwords"
```

## Task 6: Implement application services

**Files:**
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/app/connection_service.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/app/task_service.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/app/connection_service_test.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/app/task_service_test.go`

- [ ] **Step 1: Write failing service tests for use-case rules**

Cover:
- connection create/update validates required fields and normalizes timestamps
- connection delete rejects when tasks reference it
- task create validates enums and defaults status to `created`
- task update preserves immutable fields that should not change in this slice

- [ ] **Step 2: Run service tests and confirm RED**

Run: `go test ./internal/app -v`
Expected: FAIL on missing service implementations.

- [ ] **Step 3: Implement minimal services**

Implement:
- constructor injection for repositories and secret manager
- create/list/get/update/delete methods
- error translation from repository layer to domain-level typed errors

- [ ] **Step 4: Re-run service tests**

Run: `go test ./internal/app -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app
git commit -m "feat: add connection and task services"
```

## Task 7: Add HTTP router and handlers

**Files:**
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/api/router.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/api/handlers/helpers.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/api/handlers/health_handler.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/api/handlers/connection_handler.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/api/handlers/task_handler.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/api/dto/connection.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/api/dto/task.go`
- Test: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/api/handlers/health_handler_test.go`
- Test: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/api/handlers/connection_handler_test.go`
- Test: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/api/handlers/task_handler_test.go`

- [ ] **Step 1: Write failing handler tests**

Cover:
- `GET /healthz` returns `200` and a simple JSON status
- connection CRUD endpoints return expected status codes and redact password from responses
- task CRUD endpoints validate payloads and return `404` for missing resources
- malformed JSON returns `400`

- [ ] **Step 2: Run handler tests and confirm RED**

Run: `go test ./internal/api/handlers -v`
Expected: FAIL because router and handlers do not exist.

- [ ] **Step 3: Implement router and handlers**

Implement:
- `chi` router with `/healthz` and `/api/v1` grouping
- JSON helpers and error mapping
- DTOs that keep internal fields separate from transport contracts

- [ ] **Step 4: Re-run handler tests**

Run: `go test ./internal/api/handlers -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api
git commit -m "feat: expose control plane http api"
```

## Task 8: Wire configuration, startup, and graceful shutdown

**Files:**
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/config/config.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/obs/logger.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/cmd/server/main.go`

- [ ] **Step 1: Write the failing startup test or manual smoke criteria**

Define expected boot behavior:
- reads env config for `APP_ADDR`, `APP_DB_PATH`, `APP_SECRET_KEY`
- opens DB and runs migrations on startup
- starts HTTP server and handles SIGINT/SIGTERM gracefully

- [ ] **Step 2: Verify startup is not available yet**

Run: `go run ./cmd/server`
Expected: FAIL because startup wiring is missing.

- [ ] **Step 3: Implement startup wiring**

Implement:
- config loader with defaults
- structured logger wrapper
- dependency graph assembly
- graceful shutdown with context timeout

- [ ] **Step 4: Run the server smoke check**

Run: `APP_SECRET_KEY=0123456789abcdef0123456789abcdef go run ./cmd/server`
Expected: server starts, logs listen address, and `curl http://127.0.0.1:8080/healthz` returns `200`.

- [ ] **Step 5: Commit**

```bash
git add cmd/server internal/config internal/obs
git commit -m "feat: wire backend server bootstrap"
```

## Task 9: Run full verification and document known gaps

**Files:**
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/README.md`

- [ ] **Step 1: Update README with actual run and test commands**

Document:
- bootstrap env vars
- `make test`
- `make run`
- current non-goals: no connection test endpoint, no task runner, no WebDAV sync engine

- [ ] **Step 2: Run formatting**

Run: `gofmt -w $(find cmd internal testutil -name '*.go')`
Expected: no errors

- [ ] **Step 3: Run the full test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 4: Run a final HTTP smoke check**

Run: `APP_SECRET_KEY=0123456789abcdef0123456789abcdef go run ./cmd/server`
Expected: server starts successfully and serves the documented endpoints.

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: finalize backend bootstrap usage"
```

## Review Notes

- This workspace is currently not a git repository. If that remains true during execution, the commit steps become checkpoints rather than literal commands.
- The plan intentionally defers:
  - WebDAV connection test endpoint
  - task lifecycle execution (`start`, `pause`, `resume`, `stop`)
  - sync engine, watchers, scheduler, metrics, and frontend
- If `github.com/mattn/go-sqlite3` causes CGO friction in the target environment, replace it early with `modernc.org/sqlite` and update bootstrap tests accordingly.
