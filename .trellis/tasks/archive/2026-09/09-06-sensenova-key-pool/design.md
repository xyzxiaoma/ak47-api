# Technical Design Draft

## Architecture

Implement inside the production New API fork using Router -> Controller -> Service -> Model. Reuse existing multi-key channel creation, append/import, key status views, and relay selection. Do not add a separate proxy service.

Keep one SenseNova pool spanning the selected models and existing model groups. Selection must consult both the key's administrator state and provider-health state. Per-model transient restrictions must not incorrectly exhaust the whole account.

## Existing integration points

- `model/channel.go:63`: multi-key status, disabled reason/time, polling index.
- `model/channel.go:199`: enabled-key selection and polling lock.
- `model/channel.go:647`: update the exact key using the credential selected for the attempt.
- `controller/relay.go:193`: request retry loop; `controller/relay.go:363`: asynchronous channel disable path to integrate with carefully.
- `middleware/distributor.go:473`: key identity/index in request context.
- `controller/channel.go:1462`: multi-key administration request/status contracts.
- `controller/channel-test.go:857`: existing channel test entry point; recovery must explicitly address a cooling key rather than select a different enabled key.
- `service/channel.go:45`: generic auto-disable/enable policy; new provider-specific cooling must not depend on treating every 429 as permanent disable.
- `service/system_task.go`: reusable task scheduling/claim infrastructure; verify its lifecycle before choosing a recovery handler.
- `web/src/features/channels/components/dialogs/multi-key-manage-dialog.tsx`: existing status dialog to extend.

## State and identity

Use a stable non-secret identifier derived from channel/pool identity and the exact key, not only the editable list index. Add persistent health state with last-success/failure/probe times, safe reason code, consecutive failure count, next-probe time, version, and a bounded probe lease. Choose a dedicated GORM model if that is the cleanest way to provide transactional updates and cross-process claiming; do not store duplicate plaintext credentials in it.

Manual enabled/disabled state remains authoritative. Health eligibility is separate: untested/usable, cooling, invalid. Key-level quota cooldown applies across the four general-pool models. A model-specific capacity/permission error applies only to that model. A bare 429 is a transient/unknown limit, not proof of exhausted credits.

Import/reorder/removal must preserve identity for unchanged keys, initialize truly new keys without stale cooldowns, and invalidate in-flight probe versions for removed/replaced keys. Deduplicate repeated keys within the same pool.

## Request path

1. Select an eligible key, record its stable identity and generation on the attempt, and preserve the existing model/group pricing context.
2. On failure, classify the actual upstream error, not a gateway-local validation or quota failure.
3. Publish the appropriate exclusion synchronously before retry selection. Persist it without an asynchronous context/key mismatch.
4. Keep an attempted-key set for this client request so a failed key is not immediately reused even if persistence fails.
5. Retry only while the response is unwritten, the client context remains live, and retry/time budgets allow it. Preserve existing refund/settlement ownership.
6. Return a bounded error when no key is eligible. An all-cooling pool must remain discoverable by the recovery scheduler.

## Recovery path

Claim due cooling keys with one active lease per key across runners. Explicitly probe that key with a tiny completion request, no expensive tools or image input. Honor a trustworthy bounded upstream retry delay when available; otherwise use the proposed 1/5/15-minute backoff. Tick and probe concurrency must be bounded; probes must not create a retry fan-out across all keys.

Commit results only if the key identity, state generation, manual state, and lease still match. A stale success cannot override a newer failure or manual disable. An HTTP 200 must still have a valid success body; do not regard an embedded error or malformed response as recovery. A key blocked by a model-specific error should be probed in that model's scope.

Recovery traffic consumes upstream resources, but is an operator health check, not a billable customer request. Retain distinct probe counts/outcomes, never store full prompt/response bodies or credentials. Invalid credentials require operator attention rather than endless probes.

## Console contract

Extend existing authorized key-status responses with safe health fields and aggregate counters. Display last/next probe, reason, and transition state using existing components and translated text. Reuse append/import and manual disable controls, and add a permission-checked, rate-limited per-key test action. Give a clear empty/all-cooling view. Avoid secret persistence in browser storage.

## Compatibility and persistence

Follow root `AGENTS.md` for common JSON wrappers and SQLite/MySQL/PostgreSQL compatibility. Preserve legacy channel behavior by explicit opt-in to the SenseNova policy. Prefer additive schema changes. Existing generic channel recovery must not override manual disables or mark the entire SenseNova pool healthy because one unrelated channel test succeeded.

## Validation and rollout

Use fake upstreams and injected time for state/selection/probe tests. Cover concurrent failures, expired leases, key edits, all-cooling recovery, written streams, and accounting ownership. Run relevant backend and frontend checks on `forge` after inspecting its project workspace and sync method.

Production deployment remains separate from task creation: record image/source identity, verified backup and rollback image, build from an exact commit, validate the four models, then atomically change only the intended channels. Preserve source/attribution requirements in `AGENTS.md`. Do not mark a deployment ready from matching two source files alone.

## Known limits

Without account login, recovery detection is delayed until a successful probe. Neither the remaining-credit balance nor the actual weekly/5-hour reset can be inferred reliably from one failed request. The UI must expose detection timestamps rather than claim exact quota refresh.
