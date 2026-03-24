# Remaining Product Work Plan

> **Current status:** The backend control plane, baseline sync engine, runtime persistence, queue-backed action execution, retry APIs, and runtime aggregation APIs are implemented. This plan tracks the material gaps that still separate the codebase from the full design target.

## Goal

Close the remaining product gaps after the current backend milestone, focusing on execution depth, observability, and delivery completeness rather than reworking already-shipped control-plane foundations.

## What Is Already Done

- connection/task CRUD and connection test API
- upload/download/bidirectional baseline sync
- planner -> queue -> executor minimal loop
- runtime state, failure records, operation queue, file index
- scheduler startup recovery and local watcher trigger path
- batch retry and failure-level retry APIs
- task runtime aggregate API
- README and minimal OpenAPI file

## Remaining Milestones

### Milestone 1: Stronger Execution Model

- [ ] Add richer operation queue lifecycle states beyond `pending/executing/retry_wait`
- [ ] Persist executor-side action result metadata back into `file_index`
- [ ] Distinguish task-level fatal failure from file/action-level degraded execution
- [ ] Add per-action idempotency guards so repeated retries do not over-apply mutations

### Milestone 2: Remote Incremental Reconcile

- [ ] Introduce an explicit remote poller abstraction instead of only periodic full reconcile
- [ ] Track remote scan cursors/checkpoints in runtime state
- [ ] Reduce unnecessary full-tree remote scans for upload-only tasks
- [ ] Add tests for remote-side change discovery feeding planner input

### Milestone 3: Conflict And Delete Semantics

- [ ] Improve `keep_both` naming stability and conflict history retention
- [ ] Strengthen bidirectional delete semantics with clearer tombstone/version transitions
- [ ] Add more precise planner rules for rename/move detection
- [ ] Add regression tests for repeated conflict/delete cycles

### Milestone 4: Observability And Admin Surface

- [ ] Add metrics for task state, queue depth, retries, and failure counts
- [ ] Add structured task/event logging or a task event timeline table
- [ ] Expose runtime/failure/task detail APIs in OpenAPI more comprehensively
- [ ] Optionally serve static API docs or lightweight admin pages

### Milestone 5: Frontend / UX

- [ ] Build a minimal management UI for connections, tasks, runtime state, queue, and failures
- [ ] Add retry and task control actions in UI
- [ ] Add runtime summary and recent failure views

## Recommended Next Execution Order

1. Strengthen execution model and file-index writeback.
2. Add explicit remote incremental reconcile.
3. Improve conflict/delete semantics.
4. Add observability.
5. Build the frontend/admin UX.

## Exit Criteria

- queue/action execution is robust under retries and restart recovery
- remote change discovery is no longer just periodic full reconcile
- conflict/delete handling is regression-tested for repeated cycles
- runtime state and failures are observable without direct DB inspection
- a minimal user-facing admin surface exists
