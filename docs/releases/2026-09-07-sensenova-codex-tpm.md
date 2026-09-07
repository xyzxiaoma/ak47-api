# SenseNova Codex compatibility and admission — 2026-09-07

Release candidate: `ak47token-2026-09-07-sensenova-codex-tpm.1`.
Implementation, review and isolated acceptance passed; deployment pending.

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

Before deployment, publish the exact source revision/tag, embed its source link,
build the corresponding image on `forge`, verify license/attribution and version,
back up production Compose and database, and retain the current
`new-api:ak47token-2026-09-07-sensenova-retry.1` image for rollback. Change only
the application image/configuration; preserve production keys, channels,
pricing, databases and Redis. Validate public status, source link and a bounded
synthetic Codex request after deployment.
