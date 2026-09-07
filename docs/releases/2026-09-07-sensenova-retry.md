# SenseNova retry reliability — 2026-09-07

Deployed derivative version: `ak47token-2026-09-07-sensenova-retry.1`.
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

Compose and PostgreSQL were backed up before deployment under
`/opt/new-api/backups/sensenova-429-20260907/`; the validated
dump SHA-256 is
`96bcb48a00bd5330a1631058a18bf5a541dbcd2196bd802847abbe7a8e225aff`.
The previous `pricing-flat.1` image and Compose remain the rollback target.

## Production verification

- Deployed at 2026-09-07 07:24:15 UTC from public tag
  `ak47token-2026-09-07-sensenova-retry.1`, source commit
  `3cef8f4ca0b79fb066d0eca5bd134246002ec0d3`.
- Image manifest/loaded image identity:
  `sha256:a92658f8149cd459ebacb4196ee352a6d5deec10a1b8a564b96324a82772ea40`.
- The isolated `forge` builder was limited to 3 GiB and two CPUs, then stopped.
  Frontend production build passed (24 seconds); backend build passed
  (184.5 seconds). Version, source revision and three bundled license/notice
  files passed before transfer; production loaded the identical image.
- Compose diff changed only the application image. Application is healthy;
  PostgreSQL and Redis retained their August 7 start times.
- Loopback and public HTTPS checks passed for status/version, exact-tag source
  link, upstream attribution and all sixteen SenseNova selling ratios (0.1).
  The four-model preserve-discounts environment variable remained unchanged.
- Read-only channel configuration checksum before/after matched
  `68cb1c29bc9ce9789a21afef2abeab56`. It covers credentials, enabled status,
  models, groups and overrides without printing their contents; volatile
  counters/polling position are excluded. No key or channel edits were made.
- No synthetic production inference or customer-content replay was sent. The
  new diagnostics are verified by fake upstream and admin-log privacy tests;
  the actual subtype of future provider rejections remains an observation.
