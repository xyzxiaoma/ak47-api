# SenseNova workload capacity routing

Modified 2026-09-08. Candidate release:
`ak47token-2026-09-08-sensenova-capacity.1`. Not deployed yet.

## Behavior

Requests prefer keys with a recent successful request in the same numeric input
and output size class. Fresh keys still participate fairly, including when a
verified key is occupied. Same-class TPM failures produce expiring scheduling
penalties of 60/120/240/300 seconds, honoring longer provider Retry-After.
Smaller successes cannot erase a larger request's penalty. Evidence is scoped
by channel, credential fingerprint and exact model, normally expires after 15
minutes (or after a longer active Retry-After), and contains no prompts, outputs
or raw credentials.

One renewable pool/model lease limits concurrent recovery. Each customer
request retains up to four distinct upstream keys, including recoveries, so
three failures do not prevent a fourth key from completing the request.
Only a live dispatched owner can publish evidence, once; cancellations, unsent
requests and stale owners cannot overwrite a replacement owner's observations.

Fixed waits beyond the shared admission deadline return a sanitized capacity
error with Retry-After promptly. Initial nomination defers combined health and
workload checks until numeric metrics exist; actual dispatch still requires
authoritative health validation and lease ownership. Budget wait metadata
distinguishes fixed pacing/settled history from capacity a live request may
release early. Normal renewal shutdown preserves the request context.

No payload fields, tool definitions, thinking, models, pricing, quota settlement,
provider keys or runtime configuration are changed. No schema or dependency
changes. Existing unknown-quota pacing remains in force. Adding keys helps when
they supply independent available upstream capacity; key count alone cannot
establish a numeric TPM quota or guarantee availability.

## Validation and remaining limits

Targeted failures reproduced wrong ordering, impossible fixed waits, and renewal
shutdown behavior before their repairs. Tests use synthetic workloads and keys.

- Focused SenseNova service/model/controller/OpenAI adapter and related billing
  regressions passed on forge: 6.054s / 0.975s / 0.187s / 0.186s.
- Lifecycle, capacity, budget, admission, recovery and routing race checks passed
  on forge for service/model: 10.910s / 11.920s. Both packages passed `go vet`.
- The final pending-health pacing regression reproduced a 100ms client timeout
  and incorrect 120s hint despite fixed pacing of 300s. After the correction,
  affected budget/pending/admission/review tests passed with `-race` in 3.301s;
  service `go vet` passed. The read path does not acquire a temporary reservation.
- Independent final review found no remaining concrete findings. A mutation
  removing the renewal shutdown guard made its deterministic regression fail.
- The final production Docker build passed on forge, including frontend build
  (29.5s) and backend compilation with the repository's pinned Go toolchain.
  Image: `new-api:ak47token-2026-09-08-sensenova-capacity.1`,
  SHA-256 `a4d4023c118023e010a2d523a6c6e1326076680676fda6f81ea0761ecb4925f0`.
- The final image passed an isolated, network-disabled SQLite startup check:
  status success, exact candidate version, web entry, exact candidate source
  reference, upstream link, contributor attribution and all three license files.
  The temporary application container was removed. The source tag is a prepared
  reference and has not yet been published or verified publicly reachable.
- Checksummed dry-run synchronization found no differences in the changed Go
  source between the local worktree and the final forge build workspace.

Real production outcomes, including unique-request success, wait, first content
and internal attempts, must be assessed separately. This is not a zero-429 or
first-output SLA claim. No live provider inference or production deployment has
been performed for this candidate.

## Rollout and rollback

Before publication, commit the complete corresponding source and publish an
immutable tag matching the candidate version. Build with the exact public tag
source link and preserve all packaged licenses and contributor attribution.
Back up Compose and verify a fresh database dump. Replace only the application
image, keeping the previous image and Compose for rollback; keep database and
Redis running. Verify version, health, source link, configuration digests, and
business settings before a full-context real-request canary. Do not change
request semantics or credentials to make acceptance pass.
