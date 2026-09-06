# SenseNova activation and GLM probe correction — 2026-09-06

This derivative release preserves New API attribution and licensing. Version
`ak47token-2026-09-06-sensenova-pool.2` adds `reasoning_effort: none` to GLM
recovery probes when thinking is disabled. Real provider acceptance identified
the otherwise rejected combination; the exact outgoing payload is covered by
the focused probe regression test. Other model probe payloads are unchanged.

The operator authorized importing the previously supplied single account key
into one pool and disabling old GLM, Kimi and DeepSeek channels (IDs 4, 5, 6).
Only the four exact supported IDs are activated; legacy-only IDs are not
silently aliased. Prices and unrelated providers are preserved.

Before activation, tiny direct requests succeeded for both DeepSeek models and
GLM. Kimi returned `ModelAccountTpmRateLimitExceeded` (HTTP 429). The operator
explicitly accepted disabling old Kimi while the new pool awaits recovery.
Represent this as model-scoped rate limiting, not measured credit exhaustion.

The credential is never included in source or release evidence.

## Production acceptance

- Source: `59353ed24bf5d2b12898342923f30c537caeabf0`, public tag
  `ak47token-2026-09-06-sensenova-pool.2` (reachable HTTP 200).
- Image: `new-api:ak47token-2026-09-06-sensenova-pool.2`, manifest
  `sha256:c187df7e2b8a58ec7330d390d0d115e514193ed5db60284be5fd51c386080fbf`.
- Built on `forge` with the repository Dockerfile and the isolated existing
  BuildKit builder limited to 3 GiB and 2 CPUs. Focused service regression,
  frontend production build, backend build and bundled license checks passed.
  The builder was stopped afterwards; no local runtime was installed.
- Backup: `/opt/new-api/backups/sensenova-activation-20260906/`;
  `postgres-pre-activation.dump` SHA-256
  `c6cf04344440ee485021be35a99fce1fc600c49964f6ef4b84915596cd8eec52`.
  Restore and the exact activation transaction passed against a temporary
  no-network PostgreSQL clone with a fake key. That clone was then removed.
- At 06:52 UTC, one transaction created pool **15** with the supplied single
  key, the four exact models and groups `glm,kimi,deepseek`, disabled channels
  **4, 5, 6** and their abilities, and seeded Kimi's observed model-only cooldown.
  Configuration digest comparison proved that existing channel changes were
  limited to those statuses, with all unrelated configuration/prices unchanged.
- Only the application Compose image line changed. The application restarted;
  PostgreSQL and Redis were not recreated. Container health, loopback/public
  status, public pricing, exact-tag source link and upstream attribution passed.
- Through the actual gateway, `glm-5.2`, `deepseek-v4-pro` and
  `deepseek-v4-flash` each returned HTTP 200 with one output token. Settlement
  logs attributed all three to channel 15 (13, 12 and 1 quota units). The small
  operator-owned acceptance tokens were revoked and their Redis cache removed.
- Kimi's first scheduled probe recovered successfully, but the subsequent
  gateway request met upstream rate limiting again and returned bounded 503.
  Persisted state showed `cooling / rate_limited`, with the next probe scheduled.
  This demonstrates intermittent availability, not a measured credit reset.

## Rollback

Retain old channels and their credentials/configuration. Routing rollback, if
authorized, disables pool 15 and its abilities and restores the saved statuses
of channels 4, 5, 6 with their abilities in one transaction; then refresh channel
cache or restart only the application. Do not restore a whole database over
new customer traffic. The preceding `.1` image and backed-up Compose remain
available for application rollback; `.2` only changes probe payload compatibility.

## Bug Analysis: GLM disabled-thinking probe

### 1. Root Cause Category

E (implicit assumption) and D (test gap): generic disabled-thinking input was
assumed sufficient for all four models. Real GLM validation requires the paired
`reasoning_effort: none` value.

### 2. Why the Initial Request Failed

The provider returned 400 before inference; adding the explicit effort made the
same tiny GLM completion succeed. Kimi's separate 429 was not this payload bug.

### 3. Prevention Mechanisms

The fake-transport regression now asserts both outgoing GLM fields, and the
backend pool spec records the executable contract. Both are committed.

### 4. Systematic Expansion

The correction is GLM-only: the already-tested DeepSeek and Kimi payloads remain
unchanged. Customer request parameters are not silently overridden. Keep tiny
per-model live acceptance alongside deterministic state-machine tests.

### 5. Knowledge Capture

Updated `.trellis/spec/backend/sensenova-pool.md`; this repository has no bundled
`src/templates/markdown/spec` tree to synchronize. No unrelated specs were added.
