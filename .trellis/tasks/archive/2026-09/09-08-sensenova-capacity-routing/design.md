# Design

Read `docs/sensenova-429-investigation-20260908.md`. Existing pacing and health
remain authoritative. Add a service-layer observation mechanism without SQL
schema or frontend changes.

## Observation store

Use the same hashed Redis slot as each budget scope (channel, fingerprint,
exact model). Add a hash per numeric request-size class with a 15-minute TTL.
Shape uses power-of-two input buckets starting at 8192 and output buckets
starting at 1024, distinguishing absent and explicit zero output limits.
These are routing classes, not provider quotas or maximum admissible sizes.

Expose `senseNovaCapacityObservation{Verified bool, Failures int64, RetryAfter time.Duration}`.
Read with Redis TIME without refreshing TTL. Real success sets verified and
clears only that shape's penalty. Exact TPM rejection clears verified and
increments that shape's count; penalty is 60,120,240,300 seconds, capped at 300
but never shorter than captured Retry-After (bounded 86400). Other errors do
not fabricate TPM evidence. Failed observations expire after at least their
penalty and 15 minutes. Key identity is its fingerprint, not list index.

Publishing atomically verifies the reservation still owns the budget lease.
Publish before `finishSenseNovaBudget` releases it. Unsent/canceled work does
not publish. Health-only/tiny probes never call this mechanism. Redis failures
fail admission closed through existing sanitized errors; cleanup has independent
2-second deadlines.

## Selection and recovery

`AdmitSenseNovaAttempt` owns reordering because validated metrics are available
there. Middleware can nominate pending capacity without claiming it; admission
must replace that nomination with an authoritative snapshot before dispatch.
Read candidate eligibility in one batched SQL query, in rotation order, then
stable tiers: (0) recent verified same-class success, (1) fresh keys
without rejection evidence, (2) keys needing health recovery or with same-class
failures. Try tiers 0/1 before tier 2. If ordinary capacity is busy but its wait
fits the shared deadline, wait instead of testing known-rejected tier 2. Fresh
keys remain admissible when verified keys are busy, making added capacity useful.

A shape penalty blocks that workload only; health still governs all requests.
After expiry, allow recovery within the existing four-distinct-key request bound,
with one concurrent pool/model lease across instances. If no capacity can become eligible within the existing shared
deadline, return sanitized 503 with bounded Retry-After instead of pointless
sleeping. Keep bounded exploration and expiring evidence to avoid starvation.

The pool lease uses a hashed channel/model key and attempt owner. Acquire after
key budget reservation, before dispatch; release unsent budget if acquisition
or subsequent health claim fails. Renew alongside budget renewal; cancel on loss.
Release before clearing state. Do not introduce a one-recovery-per-request cap:
it would prevent success on the fourth key after three failed recoveries.

Combine health and same-shape penalties before waiting. Budget reservation
returns an atomic fixed minimum alongside the overall wait: finished pacing
cannot release early; active ownership can. A fixed minimum beyond the deadline
must not suppress another key's recoverable capacity or consume the entire wait.

## Interfaces for independent store implementation

- `senseNovaCapacityShape(request senseNovaBudgetRequest) string`
- `readSenseNovaCapacity(ctx context.Context, requests []senseNovaBudgetRequest) ([]senseNovaCapacityObservation, error)`
- `recordSenseNovaCapacity(ctx context.Context, reservation *senseNovaBudgetReservation, request senseNovaBudgetRequest, success bool, tpm bool, retryAfter int64) error`
- `claimSenseNovaCapacityRecovery(ctx context.Context, channelID int, name, owner string, lease time.Duration) (bool, time.Duration, error)`
- `renewSenseNovaCapacityRecovery(ctx context.Context, channelID int, name, owner string, lease time.Duration) (bool, error)`
- `releaseSenseNovaCapacityRecovery(ctx context.Context, channelID int, name, owner string) error`

Extract a shared budget base-key helper if necessary so hashing matches exactly.
Do not change accounting or exported interfaces. Integration remains in admission.

## Validation and rollout

Use real Gin selection, miniredis and SQLite fixtures with synthetic 28,935 input
and 32,000 output metadata. Cover fresh-key growth, three-failing/all-failing
layouts, deadline, owner replacement, expiry, cancellation and size isolation.
Run changed service tests and existing related model/controller/billing gates;
focused race checks for shared-state lifecycle. Build on forge. Deployment must
use exact public source, attribution and rollback evidence. Do not change keys,
pricing or environment to make acceptance pass. Provider quotas remain unknown.
