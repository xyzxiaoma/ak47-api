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
  obey a pool-specific maximum of three total attempts, response-written and
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
- Backoff is 60 / 300 / 900 seconds, with bounded `Retry-After` when observed by
  probes. Invalid and administrator-disabled keys do not auto-probe.

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
| HTTP 200 with embedded error, malformed/empty completion | Not recovered |
| Old success after newer failure/manual edit | Ignore stale result |
| Probe result after its lease expires | Ignore stale result |

## 5. Good / Base / Bad Cases

- Good: A fails with explicit credits exhausted, A is excluded before B is
  selected, and the original request settles once on success.
- Base: A non-opted-in channel follows the existing multi-key behavior.
- Bad: Reordering secrets moves cooldown or administrator state by list index.
- Bad: Disabling then enabling a key clears its upstream cooldown. Administrative
  state edits bump generations but preserve health. Replacement/reset is distinct.

## 6. Tests Required

- `model/sensenova_test.go`: shared/model scope, 1/5/15 recovery, stale results,
  manual precedence, identity changes, lease expiry, scheduling past paused keys,
  successful traffic concurrent with a failure, probe auth escalation.
- `service/sensenova*_test.go`: classification, exact-key fake HTTP probes,
  malformed success bodies, failure persistence outage, A→B and all-cooling.
- `controller/sensenova*_test.go`: opaque status contract, permissions, stale
  indexes, import/reorder and no retry after written/cancelled responses.
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
