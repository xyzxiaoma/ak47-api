# SenseNova API-Key Pool

## 1. Scope / Trigger

Applies only to channels explicitly opting in with `sensenova_pool: true`.
The relationship between configured keys and supplier accounts is unverified;
do not assume separate keys imply independent quotas. No documented inference-key
contract for reading console balances is available. Health observations are not
remaining quota or reset-time measurements. Do not duplicate the inventory
across model pools.

## 2. Signatures

- `model.Channel.SenseNovaPool bool`: JSON/SQL column `sensenova_pool`.
- `model.SenseNovaKeyState`: unique `(channel_id, fingerprint, scope)`; blank
  scope means account-wide, otherwise one supported model. No credential column.
- `SenseNovaKeySnapshot(channelID, key, model)` selects generations.
- `SenseNovaRequestSnapshot(channelID, key, model, now)` also selects due
  rate-limited keys for real-traffic verification, without modifying health.
- `RecordSenseNovaFailure(snapshot, key, scope, reason, invalid, retryAfter, now)`
  and `RecordSenseNovaSuccess(snapshot, key, now)` use those generations.
- `ClaimSenseNovaProbe(channelID, key, scope, now, force)` grants a 60-second
  lease; `FinishSenseNovaProbe(...)` checks lease, identity and generations.
- `service.SenseNovaMaxAttempts = 4` is the shared total-attempt bound.
- `SenseNovaAttemptLogInfo(c)` returns allowlisted failed-attempt diagnostics;
  `SetSenseNovaRetryAfterHeader(c)` is called only on a final 429/503 response.
- Existing multi-key API accepts `action: "test_key"`, `channel_id`, `key_index`
  and `key_id`; `service.QueueSenseNovaProbe(channel, key)` schedules, not proves,
  recovery. No new public route is required.

## 3. Contracts

- Allowed models: `deepseek-v4-pro`, `deepseek-v4-flash`, `glm-5.2`, `kimi-k3`.
- Require OpenAI type 1 and base `https://token.sensenova.cn` (optional trailing
  slash). Reject credential, status, parameter and model overrides. The key list
  is 1–200 unique plain keys, newline-separated; create/edit forces polling.
- General edits preserve the opt-in field if absent. Changing it requires
  `ChannelSensitiveWrite`. Explicit `false` must persist despite GORM omitting
  zero values from struct updates.
- Per-key operations must match the fingerprint at the submitted index. A stale
  row is rejected instead of changing whichever key moved into its index.
- Status responses retain administrator status separately and add `key_id`,
  `health`, `health_counts`, and a `health_state` filter. Health exposes only
  `state`, safe `reason`, observation/check timestamps and model restrictions.
  Never serialize leases, generations, credentials or fabricated balances.
- Request failures are published synchronously before retry selection. Retain
  request-local failed-key exclusion even when persistence fails. Retry must
  obey a pool-specific maximum of four distinct-key total attempts, response-written and
  client-cancellation gates. Do not start a retry after 90 seconds. Pool retries
  remain inside the initially authorized channel and pricing group, independent
  of the legacy `RetryTimes=0` default. Keep classification separate from the
  sanitized outward error so code-only failures remain retryable.
- A 15-second master-only scheduler selects due rows independently of routing
  availability. Two local slots plus database leases bound probes. Probes use
  the exact credential, a 20-second deadline, no redirects, 8 output tokens and
  a 64-KiB response limit. They never enter customer billing/settlement. Initial
  manual probes claim a model scope (prefer configured `deepseek-v4-flash`), not
  the global scope; only already globally cooling keys use a global claim.
- GLM probes must pair `thinking.type: disabled` with `reasoning_effort: none`.
  Live acceptance on 2026-09-06 showed that disabled thinking alone returns 400;
  this is a probe payload compatibility error, not exhausted account credits.
- Backoff is `max(60 / 300 / 900 seconds, Retry-After)`. Capture this header on
  real relay failures as well as probes; parse unsigned seconds or an HTTP date,
  ceil future fractional seconds and bound to 86400. Ignore malformed/past
  values. Both automatic and manual probes respect future cooling deadlines.
  Invalid and administrator-disabled keys do not auto-probe.
- Reset attempt-local failure/header state before selection. A successful
  fallback must not inherit `Retry-After`. An all-cooling 503 may give the
  earliest known eligible-key delay, combining account/model restrictions by
  maximum within each key, not maximum across the entire pool. This is a retry
  hint, not a balance/reset measurement or a guarantee of upstream capacity.
- `other.admin_info.sensenova` contains only `key_id` (fingerprint), `attempt`,
  `max_attempts`, controlled `limit_kind`, `retry_after_seconds`, and optional
  numeric `estimated_prompt_tokens` / explicit output ceilings. Missing output
  ceilings remain absent; explicit zero remains zero. Never log raw upstream
  codes/messages, headers, prompts, response bodies or credentials. Nonadmin
  token-log access must strip the entire `admin_info` object.
- Known exact upstream codes classify TPM/RPM, capacity, quota or authentication;
  unknown codes (including code-only error objects) remain sanitized `unknown`.
  Keep existing health reasons stable and do not infer quota exhaustion from 429.

## 4. Validation & Error Matrix

| Observation | Required state / behavior |
| --- | --- |
| Explicit insufficient credits / quota | Account cooling, `quota_exhausted` |
| Bare 429 | Model cooling, `rate_limited`; quota remains unknown |
| Upstream 401 / invalid-key code | Account invalid, operator action required |
| Upstream 403 / 404 without quota evidence | Model cooling, `model_unavailable` |
| Upstream 5xx / transport failure | Model cooling, `upstream_unavailable` |
| Gateway billing, validation or database error | Do not classify as key quota |
| All keys cooling | Admission-enabled requests wait within their shared deadline; due rate cooldowns permit one leased real verifier; other classes retain probe recovery |
| Only fourth key works | Reach it once, settle once; do not stop at three |
| All four reject | Stop after four distinct attempts; bounded final retry hint |
| Upstream Retry-After exceeds fallback | Preserve longer cooldown, including manual probes |
| Known code without message | Classify safely; never leak unknown metadata |
| HTTP 200 with embedded error, malformed/empty completion | Not recovered |
| Old success after newer failure/manual edit | Ignore stale result |
| Probe result after its lease expires | Ignore stale result |

## Real-traffic capacity recovery and Claude accounting (2026-09-07)

### 1. Scope / Trigger

Tiny probes can succeed when a full Claude request still exceeds upstream TPM.
Wire tool definitions are generic JSON maps, not preconstructed Go structs.

### 2. Signatures

- `ClaimSenseNovaRecovery(snapshot, key, now) (*SenseNovaSnapshot, error)`.
- `RenewSenseNovaRecovery(ctx, snapshot, key, now) (bool, error)` and
  `ReleaseSenseNovaRecovery(ctx, snapshot, key) error`.
- `SenseNovaSnapshot.NeedsRecovery` and `.RecoveryLease` are internal only.
- `dto.ProcessTools([]any)` accepts typed tools and `map[string]any` through
  relaykit's JSON wrapper; no root-module dependency is permitted.

### 3. Contracts

- A successful probe never writes `LastSuccessAt`; only actual traffic does.
  Rate-limit probe success retains `Failures`, `LastFailureAt` and the
  `rate_limited` reason, with `State=untested` pending real verification.
  Probe transport/model failures cannot erase that pending rate history.
- At an expired rate cooldown, request selection may nominate a verifier
  without waiting for the scheduler. It does not reset state or lease a key.
  Claim rechecks eligibility/deadline and takes a 120-second account/model
  lease after budget reservation; legacy admission-disabled paths also claim.
- Only one real verifier owns an account at once. Renew every 30 seconds and
  cancel the upstream if ownership is lost. Stop renewal before publishing an
  outcome, and release each still-owned generation after success, failure,
  cancellation or a pre-dispatch billing/validation failure.
- Renewal SQL uses a 2-second child context; normal renewal shutdown does not
  cancel a valid retry. Lease-release SQL uses an independent 2-second cleanup
  context so a canceled client can still release ownership without hanging on
  an exhausted connection pool. If cleanup fails, the lease expires naturally.
- Success may clear rate history only with valid recovery ownership. Preserve
  cancellation through cleanup; never refund uncertain dispatched capacity.
  Newer failures, administrator edits and expired/replaced owners take priority.
- Keep pending model restrictions in console `model_states`; render `untested`
  as Untested, not Cooling. Do not serialize generations or leases.
- Tool accounting includes ordinary names/descriptions/nested input schemas
  and web-search location. Identify web search by its versioned type, since a
  custom tool can share its name. Do not mutate, strip or clamp client tools.

### 4. Validation & Error Matrix

| Case | Result |
| --- | --- |
| Future rate cooldown, invalid key or manual disable | No request claim |
| Due rate cooldown | One real verifier; no mandatory tiny probe |
| Due quota/transport/model restriction | Existing health recovery required |
| Tiny success after TPM failure | Retain streak and real success timestamp |
| Verification fails again | Continue 60/300/900 backoff, honoring longer Retry-After |
| Recovery lease lost/expired | No renewal or outcome publication by stale owner |
| Owner canceled before dispatch | Release owned lease and unsent reservation |
| JSON tool map or typed tool | Same relevant metadata; wire payload unchanged |

### 5. Good / Base / Bad Cases

Good: real 17k-token traffic, not an 8-token probe, confirms capacity recovery.
Base: ordinary healthy-key routing and unknown-TPM pacing remain unchanged.
Bad: classify a successful tiny probe as full recovery, or interpret four keys
as four independently known provider quotas.

### 6. Tests Required

Use JSON-decoded Claude requests to assert tool count/text and unchanged wire
payload. Model/service tests cover exclusive claims, cooldown boundaries,
failure classes, renewal, cancellation, stale cleanup and successful recovery.
Console tests retain pending model state while excluding private lease fields.
Run the independent relaykit build and focused race-sensitive recovery checks.

### 7. Wrong vs Correct

Wrong: `FinishSenseNovaProbe(success)` sets `Failures=0` and `LastSuccessAt=now`.
Correct: preserve traffic history; use `ClaimSenseNovaRecovery` and a successful
real request to reset rate state, then release only its owned generations.

## Responses compatibility and capacity admission

- Opted-in OpenAI channels translate `/v1/responses` to upstream
  `/v1/chat/completions`, including streaming, function calls and Codex custom
  tool input/output replay. Preserve original tool definitions to restore
  `custom_tool_call` items and input events. Conversion is required even when
  global body pass-through is enabled. Unsupported stateful/compact requests
  fail before dispatch and do not alter key health.
- Map Responses `developer` messages to SenseNova-supported `system` messages,
  preserving order and content. Flatten namespace tool declarations using
  distinct deterministic aliases, restoring original namespace/name identities
  on responses and replay. Reasoning output must carry `summary: []` even at
  item start; do not emit `summary_text` as reasoning `content`.
- Hosted web search is unsupported and must return an explicit client error.
  Configure Codex `web_search = "disabled"`; do not silently drop its tools.
- Treat exact string or numeric provider code `429001` as TPM exhaustion.
  Unknown codes remain unknown; never classify by a broad numeric prefix.
- Admission defaults to `deepseek-v4-pro` only. Redis state is scoped by
  channel, key fingerprint and model, separate from health/probe state. Use
  Redis time and atomic reservations, an owner-checked renewable in-flight
  lease, bounded shared queue leases and a per-request wait deadline.
- Unknown TPM defaults to one in-flight request and 60 seconds between starts
  per key/model. This is local pacing, not a measured provider reset window.
  Known local budgets reserve estimated prompt tokens plus an explicit output
  ceiling, or a 4096-token allowance when absent. Reject a request exceeding
  its configured budget without cooling an otherwise healthy key.
- Initially admit after validated request metrics and price estimation, before
  billing reservation. Budget-based key reselection stays in the authorized
  channel/group and does not count as another upstream attempt. Recheck health
  before dispatch. Share the 75-second wait deadline across retries; at most
  32 requests per channel wait by default. Cancelled requests stop promptly.
- For known budgets, the wake hint must name the earliest ledger expiry that
  frees enough tokens for the pending estimate, not simply the oldest debit.
  Provider points, subscription request allowances and independent API keys do
  not establish TPM/RPM values or independent account capacity.
- Undispatched reservations are released; uncertain upstream outcomes retain
  conservative debits. Successful measured usage reconciles only the owning
  reservation. A probe, stale completion or another request cannot clear it.
  Redis failures fail closed with sanitized 503 while admission is enabled.
- Local fallback token estimates must not refund conservative reservations.
  A long stream finishing after ledger expiry restores a fresh rolling-window
  debit only while it still owns its lease. Lease loss cancels both dispatch
  and response-body reads, and cleanup must preserve the cancellation.
- Operator configuration: `SENSENOVA_ADMISSION_ENABLED`,
  `SENSENOVA_ADMISSION_MODELS`, `SENSENOVA_TPM_LIMITS`,
  `SENSENOVA_OUTPUT_TOKEN_ALLOWANCE`,
  `SENSENOVA_UNKNOWN_TPM_INTERVAL_SECONDS`,
  `SENSENOVA_ADMISSION_WAIT_SECONDS`, `SENSENOVA_ADMISSION_QUEUE_LIMIT`.
  TPM JSON maps a model to `{"default": N, "keys": {"<fingerprint>": N}}`;
  zero means unknown. Do not put raw credentials in configuration or logs.
- Validate budget/queue ownership, long-stream accounting, usage correction,
  cancellation, oversized admission, Redis failure and health independence in
  `service/sensenova_{budget,admission}_test.go`; validate the Responses bridge
  and terminal errors in `relay/channel/openai/sensenova_responses_test.go`.

### Request latency diagnostics

- Record only opted-in traffic under `admin_info.sensenova_latency`. Keep
  credentials, raw headers, prompts and all output content out of diagnostics.
  Existing user/token log filtering must remove the whole administrator object.
- Record actual dispatch, headers, first nonempty reasoning/text/tool arguments,
  first answer text and attempt finish separately. Empty/role-only events,
  usage, heartbeats, errors and tool names/IDs alone do not count as content.
- Bind stream observations to their original attempt, using a synchronized
  monotonic clock. Attempt durations start at dispatch; dispatch and finish
  offsets start at the request. Missing events are omitted, never zero-filled.
- Actual capacity and health sleeps are disjoint. Retry wait is their subset
  after a finished attempt; never add it again to total wait or total duration.
- Consume/error records contain incomplete snapshots. Emit one final structured
  `SenseNova request latency: ` backend record after request cleanup, including
  failures before dispatch and cancellations. Do not add or rewrite billing
  records to collect timing. Separate client HTTP retry sleeps remain outside
  a server request's clock. See `docs/releases/2026-09-07-sensenova-latency.md`.

## 5. Good / Base / Bad Cases

- Good: A fails with explicit credits exhausted, A is excluded before B is
  selected, and the original request settles once on success.
- Base: A non-opted-in channel follows the existing multi-key behavior.
- Good: First three keys return 429, fourth succeeds without a stale failure
  header, extra billing reservation, model change or pricing-group change.
- Bad: Reordering secrets moves cooldown or administrator state by list index.
- Bad: Disabling then enabling a key clears its upstream cooldown. Administrative
  state edits bump generations but preserve health. Replacement/reset is distinct.

## 6. Tests Required

- `model/sensenova_test.go`: shared/model scope, 1/5/15 recovery, stale results,
  manual precedence, identity changes, lease expiry, scheduling past paused keys,
  successful traffic concurrent with a failure, probe auth escalation.
- `service/sensenova*_test.go`: classification, exact-key fake HTTP probes,
  malformed success bodies, failure persistence outage, A→B and all-cooling;
  seconds/date/malformed/oversized retry headers, safe code-only errors,
  per-key cooldown isolation, successful-fallback reset and manual deadline.
- `controller/sensenova*_test.go`: opaque status contract, permissions, stale
  indexes, import/reorder, fourth-key success/four-failure stop, admin-only log
  privacy and no retry after written/cancelled responses or 90-second budget.
- Frontend feature tests: opt-in form payload/validation, health display,
  permission gates and manual probe fingerprint. Run typecheck and affected lint.
- Existing billing/retry checks remain required because relay safety is shared.

## 7. Wrong vs Correct

Wrong: asynchronously disable `c.GetString("channel_key")` after a retry changed
the context, or bump the generation for every healthy response (which can discard
a concurrent genuine failure).

Correct:

```go
snapshot, err := model.SenseNovaKeySnapshot(channelID, exactKey, modelName)
// Keep snapshot + exactKey on this attempt, then publish before selecting again.
_, err = model.RecordSenseNovaFailure(snapshot, exactKey, "", "quota_exhausted", false, 0, now)
```

Healthy observations preserve the generation; failures and administrative edits
advance it. Generic automatic channel enable/disable and generic channel tests
must not mutate the opted-in pool's state.

Wrong: assume four configured accounts means four attempts when the loop still
uses a three-attempt cap; or pass a hardcoded zero retry delay after receiving
an upstream cooldown header.

Correct: derive the retry budget from `SenseNovaMaxAttempts - 1`, capture bounded
metadata on the exact request attempt, and persist it before selecting another
key. Independent accounts do not prove absence of shared upstream capacity or
per-request size limits; only safe diagnostic evidence can distinguish those.
