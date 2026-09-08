# SenseNova completion integrity and conversation scheduling

Modified and deployed 2026-09-08. Release:
`ak47token-2026-09-08-sensenova-completion.1`.

The isolated Token Plan study reproduced HTTP 200 with zero response bytes.
The former Chat/Claude path accepted this as success and could settle estimated
input usage. This release requires a valid completed stream before reporting
success. Empty, malformed, truncated and embedded-error responses fail visibly.
Before output, the existing bounded retry may select another key. Once text,
reasoning or a tool call is delivered, the gateway reports a terminal error and
never transparently replays the call. Final Claude events and OpenAI usage keep
their protocol metadata. Valid parallel tools and mixed-choice ordering are
covered by regressions.

Partial delivered work uses existing pricing and settlement, with an explicit
`incomplete` consume marker and no success sample. Empty failures remain fully
refundable. HTTP status alone cannot establish healthy-key or workload success.
Neither payload parameters nor model, context, tools, thinking, output ceiling,
prices, account keys or numeric provider quotas are changed.

Code `429001` alone and the mixed `inference exceeds tpm/rpm limit` tuple now
classify as ambiguous rate limiting. Explicit TPM and RPM tuples remain distinct,
including code `8` / `quota_exceeded_error` / `rpm exhausted`. Ambiguous errors
cannot create workload-specific TPM penalties. The capacity evidence namespace
is versioned; old observations expire naturally while active budget, health and
recovery leases are retained.

## Optional canary configuration

Conservative production defaults remain unchanged. These options are disabled:

| Setting | Default | Contract |
| --- | --- | --- |
| `SENSENOVA_CONVERSATION_AFFINITY_ENABLED` | `false` | Prefer an eligible recently successful conversation key |
| `SENSENOVA_CANARY_FOLLOWUPS` | `0` | 0–2 expedited follow-ups; requires affinity |
| `SENSENOVA_CANARY_FOLLOWUP_INTERVAL_SECONDS` | `5` | 5–60 seconds between expedited starts |

Clients opt in with `X-AK47-Conversation-ID`, a distinct stable value per
conversation. It permits 1–128 characters from `[A-Za-z0-9_.-]` and is scoped by
authenticated user/token, channel and exact model. Missing or invalid identity
uses ordinary routing. Raw identities, prompts and credentials are not stored;
Redis keeps bounded hashes/fingerprints with a 15-minute TTL. Busy, cooling,
disabled and removed preferred keys are bypassed.

For a single Claude Code conversation, retain other existing headers and add
`X-AK47-Conversation-ID: my-session-001` to `ANTHROPIC_CUSTOM_HEADERS`. Reuse that
value on resume and change it for another conversation. Do not set one global
conversation value for unrelated users or tasks. Existing generic affinity rules
and arbitrary user metadata are not assumed to be trustworthy conversation IDs.

Enabled unknown-capacity canaries retain one in-flight request per key/model,
at most `1 + followups` starts per rolling minute, and same-conversation grants
only after verified complete responses. Completion does not refill request
demand; failures withdraw grants, and canceled unsent work restores the original
pacing deadline. These are bounded experimental operator policies, not provider
TPM/RPM claims or evidence that cached input is exempt from limits.

## Validation and operational limits

Targeted tests reproduced the original failures before implementation. Automated
coverage includes completion/error framing, no replay after tool delivery,
no false health, partial wallet/token settlement and refund idempotency, error
classification, evidence migration, affinity identity/escape, rolling request
budgets, stale ownership and cancellation cleanup. Builds and tests run on forge.

- Focused SenseNova service/model/controller/OpenAI and scanner checks passed:
  6.150s / 1.575s / 0.284s / 0.430s / 2.648s. The relay package compiled.
- Final stream/partial-replay race tests passed for OpenAI/controller:
  3.448s / 1.509s, including mixed reasoning/text/tool preservation.
- Partial billing and existing text/tiered settlement tests passed (0.060s);
  partial wallet/token race checks passed (1.487s).
- Final unsent/follow-up/budget regressions passed (0.302s); targeted cleanup
  and concurrent follow-up race checks passed (1.663s).
- Independent stream and scheduling review findings were fixed and rechecked.
  Formatting and diff checks passed. No relaykit module code or dependencies
  changed, so its separate build requirement is not triggered.

Production build, deployment and live acceptance are separate gates below.

The production Docker build passed with pinned Bun/Go toolchains. Image:
`sha256:a54bbd5d6253d1095f59aace5bf0556f7eabd28845b5db2e8351bdf21515c25e`.
An isolated SQLite startup passed `/api/status`, exact version, web entry,
corresponding-source tag URL, upstream link, contributor attribution and packaged
licenses. The temporary validation container was removed.

The prior upstream study remains one successful and one failed complete workflow,
not a stability estimate. Four independent accounts are available in production;
this release does not prove four, eight or any other fixed number can supply one
person reliably. Every selected account must admit the whole request. Qualify
the existing four with the full workload and obtain actual per-model capacity
before purchasing more subscriptions. No additional paid capacity is purchased.

## Rollout and rollback

Publish exact corresponding source before deployment, retain licenses and public
attribution, and replace only the application image after a fresh backup.
Keep affinity/follow-up canaries disabled initially. The preceding
`new-api:ak47token-2026-09-08-sensenova-capacity.1` image is the rollback target.
No schema migration or configuration reset is required. Preserve budget/health
state across rollback; old capacity observations are allowed to expire naturally.

## Verified deployment

- Source commit `0161e3e90ce09ef1b08de5031d8f8bd81f96d732`, published as
  [the exact release tag](https://github.com/xyzxiaoma/ak47-api/tree/ak47token-2026-09-08-sensenova-completion.1).
- Container started at `2026-09-08T08:11:21.077551515Z` (16:11:21 Asia/Shanghai).
  Running image digest matches the tested candidate. Private container health
  is healthy and public `/api/status` reports success and the exact version.
- Public initial JavaScript contains the exact source tag, upstream link and
  required attribution. The tag's VERSION was reachable before deployment.
- Compose changed only the application image. Environment file/effective runtime
  environment, options and channel configuration hashes match their backup;
  PostgreSQL and Redis retain their original start times. Channel 15 still has
  four configured keys. Affinity and follow-up canaries remain disabled.
- Backup `/opt/new-api/backups/sensenova-completion-20260908` contains prior
  Compose/environment and a fresh 534,098-byte PostgreSQL custom dump; its
  `pg_restore --list` catalog validated. Previous production image is retained.
- After validation, 6.429 GB of reclaimable cache was removed from the dedicated
  AK47 buildx builder, restoring 8.7 GB free disk on forge. The builder was stopped;
  production and rollback images remain available. Other builders/services were
  not changed.

## Full-context acceptance: completed, slow and still heavily limited

One Claude Code 2.1.263 workflow called the public gateway with all 25 tools,
`max_tokens=32000`, adaptive thinking and 700 synthetic reference records.
Initial estimated input was 28,867 tokens. No context, tool, model or output
ceiling was reduced. The task required Write an exact file, Read it back and
return an exact answer.

The workflow **passed**: exit 0, `is_error=false`, exact file and exact answer,
one Write and one Read. Total duration was **378.03 seconds**. Seven client HTTP
requests produced three completed responses and four HTTP 429 responses. Server
records show 22 upstream attempts: three completed responses and 19 failures
classified `rate_limit`. No empty or malformed successful stream was observed
in this live canary; those paths are covered by the targeted regressions.

| Start UTC | Result | Client duration | Upstream attempts | Gateway wait |
| --- | --- | --- | --- | --- |
| 08:14:25 | 429 before Write | 3.14s | 4 | 0s |
| 08:15:27 | 429 before Write | 3.98s | 4 | 0s |
| 08:16:28 | Write completed | 8.43s | 4 | 1.00s |
| 08:16:37 | Read rejected, 429 | 55.92s | 4 | 53.127s |
| 08:18:32 | Read rejected, 429 | 2.59s | 4 | 0s |
| 08:19:33 | Read completed | 5.25s | 1 | 0s |
| 08:19:39 | Exact final answer | 60.22s | 1 | 54.619s |

These durations distinguish gateway waits from client/SDK retry waits. The
final answer's first text arrived at 59.829s within its request. Retry-After
values 57–59 seconds are gateway pacing/health hints, not a measured supplier
quota reset. The new classification avoids reporting mixed limits as proven TPM.
This single completion is not a long-run success rate, a stable one-user SLA or
a causal performance comparison against the earlier upstream study.

Billing produced exactly three consume records, totaling 14,151 internal quota
units (12,459 / 862 / 830). Failed attempts had zero quota. The temporary test
user/token were removed; authentication caches and temporary token files were
verified absent. No canary process remains. Model/key configuration and all
business invariants were rechecked after cleanup.
The channel hash changed only because the existing model updater advanced
`upstream_model_update_last_check_time` on channels 7 and 8. A private canonical
comparison against the database backup verified every other channel field,
including provider credentials and business settings, unchanged.

Evidence: [client assertions and requests](../evidence/sensenova-completion-20260908/acceptance-summary.json),
[server totals](../evidence/sensenova-completion-20260908/canary-server-summary.json),
and [request-level timing/accounting](../evidence/sensenova-completion-20260908/canary-request-summary.json).
Private numeric source records remain in the deployment backup. No second run
was selected to replace failures, and the optional faster pacing was not enabled
globally to make this test pass.

The practical recommendation remains: retain the existing four independent
accounts while qualifying per-model capacity. Do not buy eight or sixteen
Token Plan keys on the assumption that count guarantees stable coding. A pool
cannot split one full-context request across independent accounts. Obtain actual
throughput/admission terms and validate consecutive complete tasks before sizing
or purchasing additional capacity. Optional conversation pacing still needs a
separate controlled live canary; its automated safety checks do not prove upstream
acceptance of expedited requests.
