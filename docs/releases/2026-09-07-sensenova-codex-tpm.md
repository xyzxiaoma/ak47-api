# SenseNova Codex compatibility and admission — 2026-09-07

Deployed release: `ak47token-2026-09-07-sensenova-codex-tpm.1`.
Implementation, review, isolated acceptance and public production acceptance passed.

## Problem and behavior

Codex 0.153.4 sends Responses requests, while the configured SenseNova endpoint
supports Chat Completions. Its upstream Responses 404 was incorrectly treated
as key/model unavailability. Separately, a real large Chat request followed by
another request produced `429001` (`inference tpm exhausted`), which was not
recognized by the existing exact-code classifier.

Opted-in SenseNova channels now convert Responses to Chat, restore function and
custom tool results, and report malformed or truncated streams as failures.
Other OpenAI channels retain their existing routing. Unsupported stateful and
compact requests fail before dispatch. Exact string/numeric `429001` is TPM.
SenseNova does not accept the `developer` message role; this bridge maps it to
`system` while preserving text and message order. Namespaced tools use distinct
Chat-safe aliases and restore the original `namespace`/`name` pair. Reasoning
items expose the `summary` array required by Codex 0.153.4.

Set `web_search = "disabled"` in the Codex profile used with this provider.
SenseNova cannot execute OpenAI-hosted web search; the gateway rejects that
declaration explicitly instead of advertising a tool that cannot run. Local
shell, namespaced functions and custom tool calls remain supported.

Admission is enabled for `deepseek-v4-pro` by default. It uses shared Redis
reservations and one in-flight request per key/model, tries healthy keys with
available capacity, then waits at most 75 seconds across the request. The
shared waiting queue holds at most 32 requests per channel. Cancellation and
existing authorization, pricing and four-distinct-key attempt bounds remain
authoritative. No customer database migration or new dependency is required.

Unknown provider TPM uses at least 60 seconds between request starts on each
key/model. This is a conservative operator policy, not a provider guarantee.
Accounts shared with other clients and provider-specific accounting can still
cause upstream 429s. Confirm supplier limits before configuring token budgets.

## Configuration

Defaults require working Redis. Redis failure returns sanitized 503 rather than
dispatching unaccounted traffic. Explicit `SENSENOVA_ADMISSION_ENABLED=false`
disables admission and restores legacy selection. This retains protocol fixes.

| Variable | Default | Meaning |
| --- | --- | --- |
| `SENSENOVA_ADMISSION_MODELS` | `deepseek-v4-pro` | Comma-separated supported SenseNova models |
| `SENSENOVA_TPM_LIMITS` | unset | Model to local default/per-fingerprint TPM map |
| `SENSENOVA_OUTPUT_TOKEN_ALLOWANCE` | `4096` | Estimated output reserve when client omitted a ceiling |
| `SENSENOVA_UNKNOWN_TPM_INTERVAL_SECONDS` | `60` | Unknown-limit spacing, 60–300 seconds |
| `SENSENOVA_ADMISSION_WAIT_SECONDS` | `75` | Request-wide wait budget, 1–75 seconds |
| `SENSENOVA_ADMISSION_QUEUE_LIMIT` | `32` | Shared waiting slots, 1–32 |

TPM example shape: `{"deepseek-v4-pro":{"default":0,"keys":{}}}`. Zero means
unknown, not unlimited. Per-key entries use the 64-character public fingerprint,
never credentials. Positive limits must be operator-confirmed local budgets.

Predispatch errors release unused reservations. Uncertain dispatches retain
estimates; successful measured usage reconciles only its own reservation.
Health probes do not reset capacity. No new prompt, response or credential
logging is introduced by admission.

A stream that outlives reservation history but still owns its lease restores
a fresh 60-second debit on completion. Stale owners cannot recreate accounting
state. Lost admission leases cancel both upstream transport and response-body
reading; cleanup preserves that cancellation.

## Verification and release

Focused protocol, classification, admission, queue and existing controller,
service, model and cancellation/billing regressions passed on `forge`.
Independent `relaykit` converter tests and `GOWORK=off go build ./...` passed.
Final review fixed long-stream ledger expiry, cancellation propagation, local
usage estimates refunding capacity, embedded TPM health classification and
SenseNova request-body debug logging. Focused regressions passed after each fix.

On `forge`, the isolated patched gateway and Codex 0.153.4 completed the original
synthetic shell task: write `DEEPSEEK_CODEX_OK` to a file, read it, then return
the exact marker. Codex exited 0; both file and answer matched. Default local
tools and the multi-agent namespace were declared; only hosted web search was
disabled. No customer prompts or database were replayed.

| Codex request | Bytes | Input / output tokens | Client status | Elapsed |
| --- | --- | --- | --- | --- |
| Initial instructions and tools | 37,621 | 8,493 / 198 | 200, completed | 9.87 s |
| First tool result replay | 38,590 | 8,686 / 133 | 200, completed | 55.33 s |
| Second tool result and final answer | 39,170 | 8,873 / 9 | 200, completed | 18.52 s |

The complete run took 84.6 seconds. Three underlying TPM rejections during the
second request were recovered through bounded waiting and another eligible
attempt. This proves this synthetic task completes despite observed upstream
limits; it does not prove that provider 429s are eliminated for every workload.

A separate real-provider custom-tool test returned two HTTP 200 responses in
5.88 seconds. Streamed `custom_tool_call_input` and final freeform patch text
matched exactly; replaying `custom_tool_call_output` produced the exact final
`CUSTOM_OK` marker. Deterministic tests additionally cover namespaced identity,
forced tool choice, incomplete streams, lease ownership and budget accounting.

## Production deployment and rollback

- Deployed at 2026-09-07 09:56:38 UTC from source commit
  `14152e0f5588ebe20d26a9ef6a5b4cc5b217ee6a` and the public release tag above.
- Image identity on both `forge` and production:
  `sha256:1025e7c4975ce5840f766b7c8b7421b5da52d8a7059dae4f5e2f23cb186094c7`.
- The exact source link, required New API attribution, original-project link,
  version and three license/notice files passed artifact verification. Public
  status and all three source/attribution strings in initial homepage assets
  passed after deployment. Frontend build: 27.3 s; backend build: 176.5 s.
- Backups are under `/opt/new-api/backups/sensenova-codex-tpm-20260907/`.
  The PostgreSQL custom-format dump passed `pg_restore --list`; SHA-256:
  `4908ddd2c1b2f18bf73a75bb3188444c58aca137281fc636810f81f410b465ba`.
  Compose and environment backups remain root-only. The previous
  `new-api:ak47token-2026-09-07-sensenova-retry.1` image remains available.
- Compose changed only the application image. The application is healthy.
  PostgreSQL and Redis retained their 2026-08-07 start times. Channel
  configuration checksum `52d05bc282c817eb98072ca595c9cd68` and pricing-option
  checksum `9cddd1a0895f7f525536201e0a3097b0` matched before and after deployment.
  No channel/key, pricing, database or Redis configuration edits were made.
- Public Codex 0.153.4 acceptance through `https://ak47token.com` exited 0 in
  108.58 seconds. The written file and final marker matched exactly. Three
  Responses requests returned HTTP 200 with completed events (19.89, 47.60,
  38.38 seconds); input/output tokens were 8509/428, 8806/237, and 9100/9.
  Only operator-owned test credentials and synthetic content were used.
- Temporary canary credentials/database, copied production test credential and
  isolated application/Redis containers were removed. Build/test containers
  were stopped; reusable dependency caches and sanitized evidence remain.

Rollback: restore the backed-up Compose and run
`docker compose --project-directory /opt/new-api -f /opt/new-api/docker-compose.yml up -d --no-deps --wait new-api`.
This release adds no schema migration; ordinary application rollback does not
require restoring the database dump or touching PostgreSQL/Redis.
