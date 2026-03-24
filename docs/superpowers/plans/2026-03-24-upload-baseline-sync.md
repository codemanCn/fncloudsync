# Upload Baseline Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `upload` direction tasks perform a one-time local-to-WebDAV baseline sync when the task is started.

**Architecture:** Keep execution synchronous and narrow: `TaskService.Start` resolves the task, loads the referenced connection, decrypts credentials, then delegates to a baseline sync runner. The runner walks the local directory tree, creates remote directories, uploads files through the WebDAV connector, and only then persists task status and error state.

**Tech Stack:** Go, `filepath.WalkDir`, SQLite repositories, existing WebDAV connector, `httptest`

---

### Task 1: Add baseline sync runner

**Files:**
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/sync/baseline_runner.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/sync/baseline_runner_test.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/domain/remote_entry.go`

- [ ] **Step 1: Write failing runner tests**
- [ ] **Step 2: Verify RED with `go test ./internal/sync -v`**
- [ ] **Step 3: Implement local walk + `MkdirAll` + `Upload`**
- [ ] **Step 4: Verify GREEN**

### Task 2: Wire task start to execution

**Files:**
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/app/task_service.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/app/task_service_test.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/cmd/server/main.go`

- [ ] **Step 1: Write failing `TaskService.Start` execution tests**
- [ ] **Step 2: Verify RED**
- [ ] **Step 3: Inject connection lookup, secret manager, baseline runner**
- [ ] **Step 4: Verify GREEN**

### Task 3: Add end-to-end upload baseline verification

**Files:**
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/api/router_integration_test.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/README.md`

- [ ] **Step 1: Write failing API integration test for `POST /tasks/{id}/start`**
- [ ] **Step 2: Verify RED**
- [ ] **Step 3: Implement missing glue only**
- [ ] **Step 4: Run `go test ./...` and a manual smoke check**
