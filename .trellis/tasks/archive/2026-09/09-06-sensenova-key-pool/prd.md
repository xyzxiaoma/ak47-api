# SenseNova Key Pool and Recovery Management

## Goal

Let the operator manage dozens of independent SenseNova account API keys inside the existing production New API console. When a real upstream request shows a key is temporarily unusable, stop selecting it, fail over safely, and restore it after a successful scheduled probe.

## Confirmed background

- Production is the `xyzxiaoma/ak47-api` fork, not the separate `ak47token` replacement application. The local baseline is commit `7509625`.
- The user confirmed that the keys belong to independent accounts and that only API keys are available, without account login credentials.
- SenseNova's console quota endpoint rejects API-key authentication with `auth_type_disabled`. Quota is shared by all keys of an account.
- The selected models are `deepseek-v4-pro`, `deepseek-v4-flash`, `glm-5.2`, and `kimi-k3`. All four use the general credit pool; DeepSeek Flash does not use the SenseNova Flash-Lite exclusive pool.
- Published general-pool limits currently include 60,000 credits per 5-hour window and 600,000 credits per week. These are provider limits, not measurable remaining balances through the available credential.
- The user accepted request-driven status detection and small scheduled recovery probes instead of authenticated balance synchronization.
- Existing production GLM, Kimi, and DeepSeek channels use an older upstream. Preserve their configuration for rollback and disable them only during the eventual cutover.

## Requirements

### R1. Native administrator workflow

Extend the existing channel/multi-key controls. Allow the operator to add one key or import several, view individual states, manually disable/enable a key, and request a bounded manual health check. Reuse existing sensitive-edit authorization. Secret values must not appear in status responses, logs, audit metadata, or committed artifacts.

### R2. Request-driven key state

Associate every attempt with the exact selected key. An explicit upstream quota failure moves that key into temporary cooldown. An ambiguous rate-limit response must not be presented as confirmed weekly or 5-hour exhaustion. Invalid credentials, model-specific failures, transient transport errors, and administrator-disabled keys need distinct handling.

### R3. Safe failover

Exclude the failed key before retry selection. Retry another eligible key within bounded attempt and request-time budgets. Never replay a request after a streaming response has begun. Do not multiply customer charges across failed attempts. If no key can serve the request, return an appropriate temporary-unavailability response rather than loop or invoke the disabled old upstream.

### R4. Recovery

Normal successful traffic provides health evidence without extra probes. Probe eligible cooling keys using an extremely small model request. Proposed default backoff is 1 minute, then 5 minutes, then at most one probe every 15 minutes, with bounded concurrency and scheduling jitter. Successful probes restore eligibility. Never automatically revive manually disabled or invalid keys. Persist recovery state across restarts and protect against stale probe results.

### R5. Honest status display

Show a masked key identifier, usable/cooling/invalid/manually-disabled state, safe reason, last success, last failure/check, next scheduled check, and recovery history or latest transition. Show model-specific restrictions where relevant. New untested keys must not be described as verified healthy. Do not display fabricated remaining credits, claimed reset times, or a guessed distinction between weekly and 5-hour exhaustion.

### R6. Provider and model scope

Apply the new policy explicitly to the SenseNova pool. One credential-state source must govern all four selected models and all applicable New API groups. Do not duplicate the same keys into independently managed pools for GLM, Kimi, and DeepSeek. Preserve existing unrelated providers and prices.

### R7. Operational delivery

Validate the behavior with deterministic simulated upstream errors and time. Before a production cutover, prepare a source-pinned image, database/configuration backup, rollback plan, and small end-to-end checks. Full key inventory is to be entered through the administrator workflow, not committed into source.

## Acceptance criteria

- A1: An authorized operator imports independent keys and sees distinct masked rows and statuses; unauthorized reads/writes are rejected. (R1, R5)
- A2: A quota response from key A excludes A before the current request safely succeeds with B; later eligible requests do not select A while cooling. (R2, R3)
- A3: A generic 429 or model failure is not falsely labeled as weekly exhaustion or a permanently invalid credential. (R2)
- A4: Fake-time tests exercise 1/5/15-minute recovery, persisted restart state, and successful restoration. Manual disable wins over an in-flight success result. (R4)
- A5: Editing, reordering, replacing, or removing keys does not attach an old key's cooldown to a different secret or resurrect a deleted key. (R1, R4)
- A6: A streaming response already sent to the client is never restarted on another key, and failed attempts do not cause duplicate customer settlement. (R3)
- A7: The administrator sees last/next check and reason changes; status output contains neither raw keys nor invented balances. (R5)
- A8: Empty/all-cooling pools return promptly and scheduled recovery still operates even when every key is cooling. (R3, R4)
- A9: The new upstream serves only the four selected model IDs, with shared key state across their groups. Old channels remain recoverable and unrelated routes remain unaffected. (R6, R7)

## Out of scope

- Account login/session harvesting, authenticated balance scraping, or accurate remaining-credit accounting.
- Using SenseNova Flash-Lite/image models or generating artificial Flash-Lite traffic for promotional credits.
- Automatic fallback to the deliberately disabled old upstream.
- Replacing the formal site with the separate `ak47token` application or building an external key-pool dashboard.
- Automatic credential creation or rotation, pricing changes, and unrelated model migrations.

## Review items

- Implementation was explicitly approved on 2026-09-06. Remaining legacy-model decisions apply only to production cutover, not building the key-pool feature.
- Before cutover, resolve how existing old-upstream-only IDs (`glm-5.2-fast-preview`, `kimi-k2.7-code`, `deepseek-v4-flash-0731`) should appear to current customers. Do not silently map them to a different model.
- The new provider error classifier has documented but not real exhaustion-response evidence. Keep ambiguous cases conservative and verify with redacted real failures when they occur; do not burn a quota to manufacture one.
