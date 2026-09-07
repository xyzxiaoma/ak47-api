# SenseNova latency breakdown and quota-aware scheduling

## Approved scope

The user approved items 1 and 2 of the proposed optimization: measure gateway
queueing, retry waiting, upstream first meaningful output, first answer text,
and total response time; verify actual TPM/RPM/concurrency and shared-account
scope, then improve capacity selection using evidence. The existing public
SenseNova Codex/Claude integration is the baseline. This is a bounded extension
of the existing admission, transport and administrator log paths.

## Requirements

- Add request-correlated, millisecond latency breakdowns to administrator-only
  diagnostics for opted-in SenseNova traffic. Cover successful requests,
  failed attempts, final errors, cancellation and streaming termination.
- Distinguish response headers / empty SSE / usage / heartbeat from actual
  reasoning, answer text or tool arguments. Record first semantic output and
  first answer text independently; absent events must not become zero latency.
- Separate measured gateway capacity/health sleeps, failed upstream attempts,
  successful attempt time-to-first-content and full request duration. Document
  clock origins and which measurements overlap; do not double count them.
- Never log credentials, raw upstream headers, prompts, tool arguments, model
  reasoning or response text. Retain current non-administrator log filtering.
- Research quota scope using current official supplier materials, available
  authenticated console information and safe existing observations. Treat
  undocumented quotas and account relationships as unknown. No saturation test,
  no contacting third parties, no new accounts or purchases.
- Apply only evidence-backed quota configuration. Improve earliest-available
  selection / capacity wait computation where demonstrated by deterministic
  tests. Keep unknown-quota conservative pacing unless reliable quota evidence
  permits replacing it. Independent keys alone do not prove independent capacity.
- Preserve authorization, model/pricing identity, billing once, retry bounds,
  owner-checked Redis leases, cancellations and provider cooldown requirements.

## Non-goals

No tool/context reduction, output-ceiling clamp, thinking-mode change, alternate
provider routing, pricing change or general-purpose monitoring platform.

## Acceptance

1. Focused deterministic tests prove latency semantics, retry isolation,
   privacy, cancellation and relevant admission invariants.
2. Root module builds on forge. If relaykit changes, verify its independent
   build with GOWORK=off. No local development runtime is installed or used.
3. Record quota facts with dated evidence and explicitly unresolved values.
4. A bounded real client acceptance produces the new diagnostics without
   changing client payload semantics; compare measured waits and first output.
5. Review, commit, preserve release attribution and follow the established
   deployment/rollback workflow when publishing this approved follow-up.

## Work allocation

- Implementation worker: latency state and transport/stream/log instrumentation;
  coordinate admission hooks with the main session to avoid edit conflicts.
- Main session: quota research, scheduler/admission corrections, test environment,
  integration, release and operational acceptance.
- Independent reviewer: targeted cross-layer and accounting/privacy review.
