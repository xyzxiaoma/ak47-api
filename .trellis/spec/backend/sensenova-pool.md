# SenseNova API-Key Pool

## 1. Scope / Trigger

Applies only to channels explicitly opting in with `sensenova_pool: true`.
The operator supplies keys belonging to independent accounts. API keys cannot
read the console credit balance, so health observations are not remaining quota
or reset-time measurements. Do not duplicate the inventory across model pools.

## 2. Signatures

- `model.Channel.SenseNovaPool bool`: JSON/SQL column `sensenova_pool`.
- `model.SenseNovaKeyState`: unique `(channel_id, fingerprint, scope)`; blank
  scope means account-wide, otherwise one supported model. No credential column.
- `SenseNovaKeySnapshot(channelID, key, model)` selects generations.
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
| All keys cooling | Prompt 503; scheduler still discovers due keys |
| Only fourth key works | Reach it once, settle once; do not stop at three |
| All four reject | Stop after four distinct attempts; bounded final retry hint |
| Upstream Retry-After exceeds fallback | Preserve longer cooldown, including manual probes |
| Known code without message | Classify safely; never leak unknown metadata |
| HTTP 200 with embedded error, malformed/empty completion | Not recovered |
| Old success after newer failure/manual edit | Ignore stale result |
| Probe result after its lease expires | Ignore stale result |

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
