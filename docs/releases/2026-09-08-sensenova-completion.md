# SenseNova completion integrity and conversation scheduling

Modified 2026-09-08. Candidate release:
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
