# SenseNova workload capacity routing

Modified and deployed 2026-09-08. Release:
`ak47token-2026-09-08-sensenova-capacity.1`.
Deployment checks passed; full-context tool-workflow acceptance failed because
upstream TPM rejection and subsequent capacity unavailability persisted.

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
  The temporary application container was removed. The source tag was
  subsequently published and verified publicly reachable before deployment.
- Checksummed dry-run synchronization found no differences in the changed Go
  source between the local worktree and the final forge build workspace.

Real production outcomes, including unique-request success, wait, first content
and internal attempts, must be assessed separately. This is not a zero-429 or
first-output SLA claim. The actual production outcome is recorded below.

## Rollout and rollback

Before publication, commit the complete corresponding source and publish an
immutable tag matching the candidate version. Build with the exact public tag
source link and preserve all packaged licenses and contributor attribution.
Back up Compose and verify a fresh database dump. Replace only the application
image, keeping the previous image and Compose for rollback; keep database and
Redis running. Verify version, health, source link, configuration digests, and
business settings before a full-context real-request canary. Do not change
request semantics or credentials to make acceptance pass.

## Verified deployment

- Source commit: `ff636ee6208f6c81bc2c430c4be481f43fc9ac95`.
- Public corresponding source:
  <https://github.com/xyzxiaoma/ak47-api/tree/ak47token-2026-09-08-sensenova-capacity.1>.
- The application started at `2026-09-08T06:07:47.257321048Z` (14:07:47
  Asia/Shanghai). Its running digest matches the tested image above. Private
  health and public `/api/status` report the exact release and success.
- Public initial JavaScript contains the exact source tag, original upstream
  link and contributor attribution. The public tag's VERSION was fetched
  successfully before replacement.
- Compose changed only the application image. Environment-file and effective
  runtime-environment hashes matched. PostgreSQL and Redis retained their
  original 2026-08-07 start times. Options and channel configuration matched.
  Post-canary channel changes were only its usage counter and the existing
  model updater's check timestamps on unrelated channels 7/8; comparison with
  the backup confirmed no business setting or key changes.
- Backup and rollback material:
  `/opt/new-api/backups/sensenova-capacity-20260908` contains preceding Compose,
  environment, image/configuration evidence and a fresh 532,871-byte custom
  PostgreSQL dump with a valid `pg_restore --list` catalog. The preceding
  `new-api:ak47token-2026-09-07-sensenova-recovery.2` image remains available.

## Full-context acceptance: failed

Claude Code 2.1.263 called the public gateway using `deepseek-v4-pro`, all 25
tools, `max_tokens=32000`, adaptive thinking and 700 synthetic reference records.
Estimated input was 28,867 on Write and 28,974 on Read, closely matching the
original incident's 28,935 estimate. No tools, context, output ceiling, thinking
or pricing were reduced to obtain a passing result.

The complete Write → Read → exact-answer workflow **failed after 320.86 seconds**
(CLI exit 1, `is_error=true`). Write produced the exact requested file, but Read
and the final answer did not complete. The CLI's result subtype was `success`,
which must not override its exit code, error flag and failed task assertions.

Times below are UTC on 2026-09-08. Durations include each client HTTP request;
SDK sleeps between requests are additional and included in the workflow total.

| Start | Intended turn | HTTP | Upstream attempts | Gateway wait | Client duration |
| --- | --- | --- | --- | --- | --- |
| 06:11:52 | Write | 429 | 4 | 0s | 5.00s |
| 06:12:53 | Write retry | 200 | 4 | 3.169s | 13.10s |
| 06:13:06 | Read | 503 | 1 | 49.415s | 51.90s |
| 06:14:54 | Read retry | 429 | 4 | 0s | 9.60s |
| 06:16:02 | Read retry | 503 | 1 | 60.900s | 67.31s |

This window contained **5 gateway requests: 1 successful, 4 failed**, with 14
upstream attempts: 1 success and 13 TPM rejections. The successful Write used
27,270 actual input and 149 output tokens; first semantic output arrived
10.496 seconds after that HTTP retry started. Gateway usage was charged once
(12,473 quota units), and failed-attempt log rows carried zero quota.

The next Read initially selected the same recently successful key, showing that
the capacity preference was active, but that upstream rejected the next large
request. Later requests stopped when remaining capacity could not recover
within their shared deadline. These facts verify scheduling and bounded waits;
they do **not** demonstrate stable availability or improved task success.
Verified upstream TPM limits or additional genuinely available capacity remain
necessary operational work. No quota value or fallback route was fabricated.

Temporary user/token records and their Redis caches were removed, with absence
verified independently. Credential files were removed on production and forge.
Numeric request evidence, final gateway latency records and configuration checks
remain in the restricted backup directory. No second canary was run merely to
replace this failed result with a successful sample.
