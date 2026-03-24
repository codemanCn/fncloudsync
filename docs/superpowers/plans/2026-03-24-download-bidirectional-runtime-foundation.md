# Download And Runtime Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend task execution from one-time upload baseline to download and bidirectional baseline sync, while laying the first runtime-state persistence foundation for later continuous scheduling and recovery.

**Architecture:** Reuse the current synchronous `TaskService.Start` entrypoint. The sync runner becomes direction-aware: upload baseline walks local files, download baseline traverses remote entries recursively, and bidirectional baseline composes the two in sequence with conservative “no overwrite existing local file” behavior for now. Runtime state is persisted separately from task definition so later watcher/poller/scheduler work has a durable cursor anchor.

**Tech Stack:** Go, SQLite, existing WebDAV connector, `filepath.WalkDir`, recursive `PROPFIND`, `httptest`

---

### Task 1: Direction-aware baseline runner

**Files:**
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/sync/baseline_runner.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/sync/baseline_runner_test.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/domain/remote_entry.go`

- [ ] **Step 1: Write failing tests for download and bidirectional baseline**
- [ ] **Step 2: Verify RED**
- [ ] **Step 3: Implement recursive remote traversal and local file materialization**
- [ ] **Step 4: Verify GREEN**

### Task 2: Runtime state persistence foundation

**Files:**
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/store/sqlite/migrations.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/store/sqlite/task_runtime_repository.go`
- Create: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/store/sqlite/task_runtime_repository_test.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/domain/task.go`

- [ ] **Step 1: Write failing repository tests for `task_runtime_state`**
- [ ] **Step 2: Verify RED**
- [ ] **Step 3: Implement runtime state upsert/get**
- [ ] **Step 4: Verify GREEN**

### Task 3: Wire start execution and runtime tracking

**Files:**
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/app/task_service.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/app/task_service_test.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/internal/api/router_integration_test.go`
- Modify: `/Users/xiaoxuesen/LLM/fn-cloudsync/cmd/server/main.go`

- [ ] **Step 1: Write failing tests for runtime phase/status updates across upload/download/bidirectional**
- [ ] **Step 2: Verify RED**
- [ ] **Step 3: Persist runtime state around baseline execution**
- [ ] **Step 4: Verify GREEN**
