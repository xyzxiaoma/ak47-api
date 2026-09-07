# SenseNova latency diagnostics and capacity wait correction

Deployed release: `ak47token-2026-09-07-sensenova-latency.1` (2026-09-07).

## Behavior

Opted-in SenseNova requests record administrator-only timing diagnostics.
They distinguish actual gateway capacity/health sleeps, failed upstream
attempts, response headers, first meaningful model output, first answer text,
and complete request duration. Client payloads and streams are unchanged.

Consume/error logs contain a snapshot under `other.admin_info.sensenova_latency`.
Because settlement and final cleanup can continue after those logs are created,
these snapshots have `complete: false`. A single final backend log, correlated
by the existing request ID, uses prefix `SenseNova request latency: ` and
contains the same object under `admin_info`, with `complete: true`.
This adds no billing record and does not rewrite settled consume logs.

| Field | Meaning |
| --- | --- |
| `total_ms` | Request start to snapshot/final cleanup |
| `total_wait_ms` | Sum of actual health and capacity sleeps |
| `health_wait_ms` | Sleeps waiting for the next eligible health check |
| `capacity_wait_ms` | Sleeps waiting for key budget, pacing or in-flight capacity |
| `retry_wait_ms` | Subset of total waits after a dispatched attempt finished |
| `attempts[].dispatch_ms` | Request start to this actual upstream dispatch |
| `attempts[].headers_ms` | This dispatch to response headers |
| `attempts[].first_semantic_ms` | This dispatch to first nonempty reasoning, answer text or tool arguments |
| `attempts[].first_answer_text_ms` | This dispatch to first nonempty answer text |
| `attempts[].finish_ms` | Request start to attempt finish |
| `attempts[].duration_ms` | This dispatch to attempt finish |

All values are milliseconds. Missing events are omitted, not reported as zero.
Role-only/empty chunks, tool names/IDs alone, usage, errors and heartbeats are
not model output. A tool-only response can have a first-semantic time without
any first-answer-text time. Non-streaming output is observed after body decode.

These measurements overlap: do not add retry wait to total wait, or attempt
duration/first-output offsets to total request time. For request-relative
first output, add that attempt's `dispatch_ms` and `first_semantic_ms`.
The end-to-end client time can additionally include network transit and client
SDK retry sleeps across separate HTTP requests, which server records cannot
measure as one request. Response headers are not time to meaningful output.

Only fixed timing/outcome fields and existing key fingerprints are logged.
Credentials, prompts, reasoning, answer text, tool arguments and raw upstream
headers are excluded. Existing non-administrator log filtering still removes
the entire `admin_info` object. No new public metrics endpoint is introduced.

## Capacity scheduling

Known local TPM policies now compute the earliest expiry that frees enough
tokens for the pending request. Previously the hint could name the oldest
debit even when that debit was too small. For a 100-token local budget with
20/60/20 tokens admitted at seconds 0/10/20, an 80-token request at second 20
must wait 50 seconds, not 40. Atomic reservations, lease ownership, cancellation,
retry limits, authorized channel/group and billing remain authoritative.

## Supplier quota findings

The [official platform documentation](https://platform.sensenova.cn/docs#points)
describes points rules effective August 28, 2026: separate general and Flash-Lite
pools, each with rolling five-hour and weekly allowances. General points are
shared across open models. Points are not TPM, and the older Skills FAQ's
request-count allowance cannot establish today's account/model capacity.

The [usage-limit page](https://platform.sensenova.cn/console/usage-limits)
loads model-specific TPM/RPM from an authenticated console endpoint. Its
unauthenticated response is 401; its metered-usage placement must also be checked
before applying any displayed number to Token Plan keys. No verified numeric
TPM/RPM/concurrency or four-key account mapping is currently available. No
provider quota or inference-key quota-query contract is invented.

Therefore the production unknown-quota policy remains conservative: one
in-flight request and at least 60 seconds between starts per key/model, within
the existing request-wide 75-second wait and 32-slot queue bounds. The wait
calculation fix improves known-budget correctness; it is not evidence that the
unknown production capacity can be increased. These changes do not promise
zero upstream 429s or a particular first-output latency.

## Verification

Focused service, OpenAI relay and controller tests passed on forge, including
semantic/tool-only output, retries, cancellations, administrator filtering,
Chat/Claude error envelopes before and after answer text, and wait rounding.
The deterministic known-budget regression failed with the original 40-second
hint and passed with the required 50-second hint. Independent review findings
were corrected and re-reviewed; no outstanding issue remains. Relaykit and
its public APIs are unchanged. Root build passed for the initial canary;
the release image rebuild validates the exact corrected source.

An initial isolated Claude Code 2.1.263 run completed Write, Read and the exact
final marker in 72.05 seconds, with three HTTP 200 completed streams. It kept
all 25 default tools, adaptive thinking and max_tokens=32000. The third request
took 59.87 seconds: 54.318 seconds were measured gateway capacity sleeps,
three failed upstream attempts took 0.561/0.393/0.664 seconds, and the successful
attempt reached first answer text in 3.398 seconds after dispatch. The first
two requests needed no gateway sleep and reached semantic output in
3.801/2.892 seconds. These are observations, not a throughput or latency SLA.
This initial canary preceded two telemetry-only review corrections (rounding
and late SSE error classification); final deployment acceptance is recorded
separately below. A discarded fixture run failed before upstream dispatch
because its SQLite JSON field was seeded as text instead of a blob.

## Deployment and rollback

- Deployed at 2026-09-07 12:33:07 UTC from exact source commit
  `6f9f0bfd385fa98d8ffb3df4fc5e65f92455d69c` and the public release tag above.
- Production and forge image identity:
  `sha256:c859e1f97505204832f98a5f56aa4b68a3b1513c74f4c51cab37c42f2cbdc396`.
  The full frontend/backend Docker build passed. Public version and exact source
  link, required New API attribution and original-project link passed checks
  in the release artifact and initial homepage assets. All three license/notice
  files remain packaged. No dependencies or database schemas changed.
- Backups: `/opt/new-api/backups/sensenova-latency-20260907/`. Root-only Compose
  and environment backups are retained. The PostgreSQL dump passed
  `pg_restore --list`; SHA-256:
  `7f88d8e24e06de32a10e18ad4f530d780ec49e83a79f8e1c469ccfb1155d1c16`.
- Compose changed only the application image; the application is healthy.
  PostgreSQL and Redis start times remained unchanged. Channel checksum
  `e5fd571ef6f89a0b0c04ed94677a9c82` and pricing checksum
  `9cddd1a0895f7f525536201e0a3097b0` matched immediately before/after deployment.

Rollback: restore `docker-compose.yml` from that backup, then run
`docker compose --project-directory /opt/new-api -f /opt/new-api/docker-compose.yml up -d --no-deps --wait new-api`.
The previous `new-api:ak47token-2026-09-07-sensenova-codex-tpm.1` image remains
available. No database restore or Redis reset is needed for application rollback.

## Public acceptance observations

The unchanged Claude Code 2.1.263 profile sent all 25 tools with adaptive
thinking and max_tokens=32000 through `https://ak47token.com`. Two HTTP 429
responses were followed by three successful, complete server-side responses.
The proxy independently observed successful Write and Read tool rounds.

| Request | Server total | Gateway wait | Upstream attempts | Successful first semantic output |
| --- | --- | --- | --- | --- |
| Initial 429 | 1.598 s | 0 | 4 failed | absent |
| Retried 429 | 28.482 s | 14.007 s | 4 failed | absent |
| Write | 38.667 s | 29.010 s | 2 failed, 1 successful | 3.714 s |
| Read | 10.955 s | 5.002 s | 1 successful | 3.693 s |
| Final response | 45.622 s | 40.107 s | 1 failed, 1 successful | 2.951 s |

First-semantic values start at the successful attempt's dispatch, not the
client's initial request. The final response's first answer-text time was also
2.951 seconds. The initial 429s instructed the client to wait 59 and 36 seconds;
those separate client sleeps are outside the server totals. Most gateway sleep
in this production sample was waiting for healthy eligible keys after cooldown.
The five request IDs had exactly three consume records (52,895 input / 191 output
tokens) and eleven internal TPM error records. Those errors count attempts,
not eleven customer-visible failures. The measurements do not show that 429s
were eliminated or establish any numeric provider quota.

After forge connectivity recovered, the saved client report was retrieved and
verified: exit code 0, successful Write/Read calls, exact file contents and exact
final answer, in 223.26 seconds. Client statuses were 429, 429, 200, 200, 200.
The temporary copied token was independently confirmed absent. Earlier isolated
canary credentials/database/services were independently confirmed removed, and
the dedicated build/test containers were stopped. A fresh forge check also
wrote/read 10,000 integers, verified their sum as 50,005,000, and removed its
temporary files. Production remained healthy during the forge connection
interruption; no application rollback was indicated by these observations.
