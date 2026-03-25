# Remaining Product Work Plan

> **Current status:** The backend control plane, baseline sync engine, runtime persistence, queue-backed action execution, retry APIs, runtime aggregation APIs, explicit remote poller, runtime checkpoints, first-pass conflict history retention, explicit tombstone/recovery sync-state transitions, comprehensive OpenAPI coverage for runtime/admin APIs, and a minimal built-in admin UI are implemented. This plan now reflects the completed delivery state for the originally tracked remaining gaps.

## Goal

Close the remaining product gaps after the current backend milestone, focusing on execution depth, observability, and delivery completeness rather than reworking already-shipped control-plane foundations.

## What Is Already Done

- connection/task CRUD and connection test API
- upload/download/bidirectional baseline sync
- planner -> queue -> executor minimal loop
- runtime state, failure records, operation queue, file index
- scheduler startup recovery and local watcher trigger path
- explicit remote poller for download/bidirectional tasks
- runtime checkpoint persistence and runtime API exposure
- batch retry and failure-level retry APIs
- task runtime aggregate API
- stable `keep_both` conflict naming and persisted `conflict_history`
- explicit `tombstoned` recovery semantics in `file_index` action writeback
- conflict recovery now clears `ConflictFlag` on successful post-conflict sync actions
- README and minimal OpenAPI file

## Remaining Milestones

### Milestone 1: Stronger Execution Model

- [x] Add richer operation queue lifecycle states beyond `pending/executing/retry_wait`
- [x] Persist executor-side action result metadata back into `file_index`
- [x] Distinguish task-level fatal failure from file/action-level degraded execution
- [x] Add per-action idempotency guards so repeated retries do not over-apply mutations

### Milestone 2: Remote Incremental Reconcile

- [x] Introduce an explicit remote poller abstraction instead of only periodic full reconcile
- [x] Track remote scan cursors/checkpoints in runtime state
- [x] Reduce unnecessary full-tree remote scans for upload-only tasks
- [x] Add tests for remote-side change discovery feeding planner input beyond poll-triggered full reconcile

### Milestone 3: Conflict And Delete Semantics

- [x] Improve `keep_both` naming stability and conflict history retention
- [x] Strengthen bidirectional delete semantics with clearer tombstone/version transitions and repeat-delete recovery rules
- [x] Add more precise planner rules for rename/move detection
- [x] Add regression tests for repeated conflict/delete cycles and post-conflict recovery

### Milestone 4: Observability And Admin Surface

- [x] Add metrics for task state, queue depth, retries, and failure counts
- [x] Add structured task/event logging or a task event timeline table
- [x] Expose runtime/failure/task detail APIs in OpenAPI more comprehensively
- [x] Optionally serve static API docs or lightweight admin pages

### Milestone 5: Frontend / UX

- [x] Build a minimal management UI for connections, tasks, runtime state, queue, and failures
- [x] Add retry and task control actions in UI
- [x] Add runtime summary and recent failure views

## Recommended Next Execution Order

1. Strengthen execution model and file-index writeback.
2. Deepen remote incremental reconcile beyond poll-triggered full reconcile.
3. Improve conflict/delete semantics and repeated-cycle regression coverage.
4. Add observability.
5. Build the frontend/admin UX.

## Exit Criteria

- queue/action execution is robust under retries and restart recovery
- remote change discovery is no longer just periodic full reconcile
- conflict/delete handling is regression-tested for repeated cycles
- runtime state and failures are observable without direct DB inspection
- a minimal user-facing admin surface exists
