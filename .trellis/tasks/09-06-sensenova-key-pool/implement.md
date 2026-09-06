# Implementation and Verification Plan

## Planning gates

- [x] User authorized creation of this Trellis task.
- [x] Confirmed source repository and existing multi-key integration points.
- [x] Recorded independent-account/API-key-only constraint and selected models.
- [x] Drafted PRD, design, and source-backed research.
- [ ] Review final implementation scope, especially migration of old-only model IDs before production cutover.
- [x] User explicitly approved implementation on 2026-09-06 (开始实现吧). The local task CLI remains parser-incompatible; task status was updated without altering the CLI.

## Implementation order

1. Establish an isolated branch/worktree in `ak47-api`; preserve other work. Load `trellis-before-dev` and applicable root/frontend conventions.
2. Inspect `forge` for an existing checkout, available Go/Bun tooling and resources. Establish a reviewed non-destructive synchronization method before builds/tests. Do not run production workloads for development.
3. Add deterministic failing tests for exact-key failure attribution, exclusion before retry, manual-disable precedence, and recovery of an all-cooling pool.
4. Implement stable key identity and persistent health state, transactional mutation and probe leases; validate key edit/removal behavior.
5. Add SenseNova-specific error classification and safe retry integration. Preserve unchanged provider behavior, stream boundaries, and customer settlement invariants.
6. Integrate scheduled small recovery probes and authorized manual probes, avoiding duplicate work across runners.
7. Extend the existing channel form/status dialog, API types, counters, filters and i18n. Add the corresponding user-visible behavior tests.
8. Record operator instructions and derivative modification notes. Keep API keys out of fixtures, commits, logs, and generated evidence.

## Relevant checks

Run these on the configured development host, starting with new focused tests. Exact test names will be set when implementation exists.

- `go test ./model ./service ./controller ./middleware -run 'SenseNova|MultiKey' -count=1`
- Run narrower individual packages first, then affected existing channel/retry/accounting tests if the change crosses those boundaries.
- Verify GORM persistence/migration behavior for SQLite, MySQL and PostgreSQL; expand beyond unit tests only as necessary for the shared state design.
- If `relaykit/` is changed: `cd relaykit && GOWORK=off go build ./...`.
- In `web/`: `bun run typecheck`, affected-file lint, and the repository's available feature test runner after verifying its configuration. Do not assume a test script exists in package.json.
- Before an actual release: frontend production build and required source/license checks.
- `git diff --check` for all task changes.

## Behavior matrix

| Scenario | Required outcome |
| --- | --- |
| Explicit quota failure on A, B available | A excluded before retry; B may serve the same unwritten request |
| Generic 429 | Temporary/unknown limit; no invented weekly or 5-hour label |
| Model-specific failure | No incorrect global account exhaustion |
| Every key cooling | Prompt response; recovery scheduling remains active |
| Successful due probe | Key becomes eligible; status timestamps update |
| Manual disable during probe | Probe success cannot re-enable key |
| Delete/reorder/replace during request | State follows the actual key, not the reused list index |
| Restart/multiple runners | Durable schedule and single claimed probe per key |
| Stream already written | No transparent replay on another key |
| Multiple failed attempts then success | No duplicate customer billing |
| Status/API/log response | Masked identifiers and coarse safe reasons only |

## Production handoff checklist

- Resolve old-only model IDs without silently remapping them.
- Inspect current running source/image and channel state again; previous observations may have changed.
- Verify backup restoration metadata and source-pinned rollback image.
- Confirm deployment prerequisites from `AGENTS.md`, including matching published source and dependency/license requirements.
- Enter the key inventory through the restricted console; no credentials in source.
- Validate one small request per selected model and targeted streamed failover tests.
- Preserve and disable old GLM/Kimi/DeepSeek channels only at cutover; preserve unrelated groups and pricing.
- Record exactly what changed and tested; roll back routing/image on acceptance failures.

## Implementation result — 2026-09-06

- Added opt-in SenseNova channel policy, persisted fingerprint/model-scope health,
  atomic generation invalidation, manual-state precedence and bounded probe leases.
- Native form/import/key manager supports the four models, health filters and
  safe timestamps/reasons. Pool balance is N/A because API keys cannot read it.
- Pool failover has three total attempts, stays inside the selected channel and
  pricing group, and stops after client cancellation, written output or a
  90-second retry window. First relay reuses the distributor's exact selection.
- Recovery uses two slots, a 15-second master scheduler, 60-second DB leases,
  20-second tiny non-billable probes and 1/5/15-minute cooldown. Manual initial
  probes are model-scoped; quota/authentication failures escalate account-wide.
- Removed pool response bodies from generic error/debug logging. Preserved
  legacy provider behavior and single preconsume/refund/settlement ownership.
- Code review found and fixed default-zero legacy retry behavior, loss of retry
  classification during sanitization, ordinary success clearing a probe lease,
  malformed probe success acceptance, initial probe scoping and double polling.

### Verification evidence

Development ran in bounded containers on `forge`, at
`/srv/codex-workspaces/ak47-api-sensenova`, with non-destructive source sync.
The local checkout remains the source of truth. No local runtimes were installed.

- Passed: `go test ./model ./service ./controller ./middleware ./relay/channel/openai ./relay/helper -run 'SenseNova|ChannelHasSensitiveChanges|MultiKeyAction|Retry|Billing|Stream' -count=1`.
- Passed subsequent narrow regressions for preserving concurrent probe leases,
  initial model probe account health, reusing initial polling selection,
  cancelled client requests and pre-controller raw-error log redaction.
- Passed: frontend typecheck; 15 tests across SenseNova form/control and existing
  New API form tests; affected-file oxlint (zero warnings/errors); protected-header
  formatting; frontend production build (25.1 seconds).
- Backend executable build initially required the frontend `web/dist` embed;
  after building the actual frontend, `go build -o /tmp/sensenova-new-api .`
  passed in the bounded Go container. This is build verification, not a release
  image or deployment.
- Passed: `git diff --check`; no raw credentials in task/spec artifacts.
- Intentionally not run: unrelated full suites, benchmarks, live quota-exhaustion
  traffic, production acceptance/cutover, MySQL/PostgreSQL integration matrix.
  `relaykit` source/public APIs were not changed; independent module rebuild is
  not required by the project gate for this change.

### Remaining workflow / release gates

Confirm the work-commit plan before committing. Do not push. Archive/journal
bookkeeping follows the work commit, not before it. Production release remains
separate and requires the cutover checks above and a full operator key inventory.

## Tooling note

The normal `task.py create` invocation failed on the local Python parser in `common/task_context.py:240` before it created files. These artifacts were created with the canonical schema from `common/task_store.py`. Before using task activation/validation, use a compatible existing Python interpreter (prefer the remote development environment) or resolve the tooling issue in a separately scoped change. This task does not alter Trellis scripts.
