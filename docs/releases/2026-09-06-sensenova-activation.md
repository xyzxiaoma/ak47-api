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

The credential is never included in source or release evidence. Production
backup and cutover acceptance will be recorded after execution.
