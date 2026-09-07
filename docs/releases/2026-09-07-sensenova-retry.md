# SenseNova retry reliability — 2026-09-07

Planned derivative version: `ak47token-2026-09-07-sensenova-retry.1`.
New API AGPL licensing, notices and attribution remain intact; dependencies,
models, channel inventory and pricing are unchanged.

## Changes

- Allow a request to try up to four distinct eligible SenseNova account keys,
  preserving the existing channel/group, cancellation, response-written and
  90-second retry-admission gates.
- Carry bounded upstream Retry-After into per-key/model cooldown. Manual probes
  cannot clear a cooling restriction before its deadline. Final error hints
  consider the earliest available key; successful fallback has no stale hint.
- Add safe admin-only attempt number, key fingerprint, controlled limit class,
  retry duration and numeric request-size diagnostics. Unknown error codes and
  provider content are never persisted in these fields.

This repairs gateway failover and observability. It does not establish the
provider's historical 429 cause or guarantee that upstream limits disappear.

## Validation and deployment state

The fourth-key regression failed against the original three-attempt bound.
Bounded remote controller/service/model and relay-helper regressions passed,
including generic error compatibility, cancellation, billing and final review's
per-key maximum / pool-minimum cooldown correction. Middleware compiled.
The four-key test composes controller retry and selection; it is not a full
authenticated HTTP Relay integration test. Billing boundaries were also reviewed.
No real provider load test or customer-request replay is required.

Production remains on `ak47token-2026-09-06-pricing-flat.1`. Compose and PostgreSQL
were backed up under `/opt/new-api/backups/sensenova-429-20260907/`; the validated
dump SHA-256 is
`96bcb48a00bd5330a1631058a18bf5a541dbcd2196bd802847abbe7a8e225aff`.
The old image and Compose remain the rollback target. Release requires the
reviewed source commit/tag, bounded build on `forge`, image/source checks and
an image-only application rollout preserving all sixteen 0.1 selling ratios.
