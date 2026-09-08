# SenseNova Token Plan isolated capacity study and proposed operating plan

Status: completed, 2026-09-08. The direct-inference window ran from 06:41:29 to
07:12:29 UTC (14:41:29 to 15:12:29 Asia/Shanghai), followed by an offline gateway
regression. This document does not claim additional production changes.

**Recommendation:** correct false success and error classification first, then
canary conversation-aware scheduling with a shared retry budget. Qualify
contracted per-model upstream capacity before adding accounts. Simply doubling
Token Plan keys or increasing fixed cooldowns is not supported by these results.
The earlier capacity-routing release is already deployed; this study tests the
upstream directly and identifies further work.

## Scope and method

The user supplied one dedicated SenseNova Token Plan API key and confirmed that
its account has no other callers during this study. All inference runs on forge
against `https://token.sensenova.cn`, bypassing AK47, with `deepseek-v4-pro`.
Only synthetic text and test tool work are sent. The credential is held in a
temporary restricted directory, excluded from source and evidence, and was
removed after the final direct probe. No existing production provider keys were
used. The credential was not installed in the production pool.

The small diagnostic uses a constant instruction to return `OK` followed by
synthetic numbered records. A successful 2,000-record request reports 28,025
input tokens. Maximum output, input size, concurrency and idle periods are
varied in separate phases. Most diagnostic requests explicitly disable thinking
and return one output token; these are admission experiments, not coding-quality
acceptance. The final tool workflow separately preserves the actual client
shape: 25 tools, 32,000 output ceiling, adaptive thinking and a full synthetic
context. Client and gateway retries must be counted separately from successful
user workflows.

Each request records UTC start, duration, exact body digest, status, returned
usage, request ID and a sanitized error. Failed requests have no reported token
usage; successful requests with identical input establish the workload size.
No numeric quota or window is inferred merely from an HTTP status or error-code
name. Waiting periods are measured from the end of the previous diagnostic.

The initial admission diagnostic contains 28 inference requests: 16 HTTP 200
and 12 HTTP 429 responses, with 79,096 total tokens reported by successful
responses. Read-only model and documentation requests are not included.
The successful-request count includes deliberately tiny controls; it is not a
coding-task success rate. Sanitized per-request evidence is in
[diagnostic.jsonl](evidence/sensenova-limit-study-20260908/diagnostic.jsonl), which
also includes the final empty-stream probe described below.

| Controlled observation | Result | Interpretation |
| --- | --- | --- |
| Tiny input, max 128 versus max 32,000 | Both succeeded | A high ceiling alone does not always cause rejection |
| 6,325 input, max 128 | Succeeded | Small success does not certify full coding capacity |
| 28,025 input, max 32,000, at 06:45:24 and 06:51:42 UTC | Both succeeded; one output token each | This workload is possible, but not consistently available |
| 14,025 input followed immediately by the identical request | Success, then 429 | The second request can be rejected despite a completed first request |
| Four simultaneous 25-input-token requests | Four succeeded | No hard concurrency-one limit was demonstrated |
| Seven requests in 21.5 seconds in the size ladder | Last response explicitly said `rpm exhausted` | Request-rate exhaustion exists; a numeric threshold is not established |
| Seven tiny requests spread across 40.9 seconds | All succeeded | Do not turn the preceding observation into a fixed six-RPM quota |
| Four cold large requests, 150 seconds idle before each; ceilings 128 / 32,000 / 32,000 / 128 | Four 429 responses | Increasing idle to 150 seconds and changing only the ceiling did not restore reliable admission |

The final cold sequence started at 06:54:15, 06:56:45, 06:59:16 and 07:01:47
UTC. No successful inference occurred during this sequence. None of these
requests overlapped, and the user confirmed account exclusivity. This excludes
our gateway's queue and concurrent callers on this account as explanations for
these particular failures. It does not identify hidden provider-side limit scope
or establish that rejected requests do not affect provider accounting.

## Complete coding-workflow results

Both runs used Claude Code 2.1.263, native SenseNova Anthropic Messages, 25 tools,
`max_tokens=32000`, adaptive thinking, and 700 synthetic reference records. They
ran in fresh isolated client directories on the same dedicated account. The
task was Write an exact file, Read it back, then return an exact answer. No
gateway scheduling, protocol conversion, provider-key rotation or AK47 billing
was involved. Input for the first successful turn was 27,231 tokens.

| Run | Duration | HTTP requests | HTTP 429 | Completed model responses | Complete workflow |
| --- | --- | --- | --- | --- | --- |
| 1, starting 07:03:07 UTC | 41.77s | 6 | 2 | 3 | Passed: Write, Read, exact answer; exit 0, `is_error=false` |
| 2, starting 07:06:14 UTC | 286.11s | 23 | 18 | 2 | Failed: Write and Read completed, final answer did not; exit 1, `is_error=true` |

In run 1, the final three successful requests started at 07:03:30, 07:03:36 and
07:03:41 UTC. Read and final answer reported 27,136 and 27,392 cached input
tokens respectively. The next complete run still failed after many retries,
despite eventually reporting a cache hit on Read. Cache reuse is a useful
scheduling signal, not proof of TPM exemption or guaranteed admission.

Run 1 had one HTTP-200 stream without a captured completion; run 2 had three.
The client subsequently retried non-streaming. These are not counted as
successful model responses. The CLI result subtype `success` also occurred in
the failed run and must not override exit status, `is_error`, and task assertions.

Evidence: [workflow summary](evidence/sensenova-limit-study-20260908/workflow-summary.json),
[run 1 HTTP records](evidence/sensenova-limit-study-20260908/workflow-1-http-evidence.json),
and [run 2 HTTP records](evidence/sensenova-limit-study-20260908/workflow-2-http-evidence.json).
This is one pass and one failure in two trials, not an estimated long-run SLA.

## Empty-stream reproduction and confirmed gateway defect

A final direct Anthropic streaming request used the 2,000-record synthetic
input, adaptive thinking and 32,000 maximum output. SenseNova returned HTTP 200
and ended after 0.741s with **zero response-body bytes**. It contained no message
or terminal event. This independent stdlib HTTP probe establishes that the
condition does not require Claude Code or our local forwarding proxy.

A targeted offline test then fed that zero-byte HTTP-200 event stream into the
current OpenAI adaptor's Chat streaming mode, for both OpenAI and Claude client
formats. Both returned a nil API error. The OpenAI client body was only
`data: [DONE]`; the Claude client body was empty. The assertion that an empty
stream must return an error failed in both cases (0.050s package run).

`OaiStreamHandler` currently returns usage and nil after finalization;
`ClaudeHelper` then follows the normal consume path, and `controller/relay.go`
calls `RecordSenseNovaRelaySuccess` when the API error is nil. That success
function marks admission successful and updates key health without requiring
semantic output. This creates false health/capacity evidence for the reproduced
condition. The unit fixture had zero input estimate and does not establish any
historical customer charge or charge amount; billing behavior with real estimates
must be covered in the repair.

Evidence: [failing regression output](evidence/sensenova-limit-study-20260908/empty-stream-regression.log)
and [isolated fixture](evidence/sensenova-limit-study-20260908/sensenova_empty_study_test.go.txt).
The fixture was removed from the remote build workspace after capture. No fix
was silently installed. Existing Responses-mode guards do not cover these Chat
and Claude paths; existing latency tests already note their legacy nil-error
contract for embedded stream failures.

## Official documentation checked

- [Current platform API documentation](https://platform.sensenova.cn/docs)
  was fetched directly on forge, including its published documentation asset.
  It documents the model, OpenAI and Anthropic endpoints, tool support, and
  a 1,048,576-token context window. Model context capacity is not a TPM quota.
- Its current Token Plan points section describes rolling five-hour and weekly
  point limits. These are separate from token throughput and request rate.
  The older GitHub FAQ's 1,500-calls-per-five-hours description must not be used
  as this account's current numeric contract.
- The public console implements a separate **metered API** usage-limits page
  with per-model TPM/RPM/RPS columns. Its data API requires account login;
  the supplied Token Plan API key is explicitly not an accepted authentication
  type. No login session or numeric account quota was obtained.
- `GET /v1/models` succeeds and reports this model supports both `tokenplan`
  and `metered` businesses. Returned zero pricing fields do not prove unlimited
  service or establish the account's point conversion or throughput quota.

## Findings that already change the diagnosis

1. A small request succeeds with either a 128 or a 32,000 maximum output value.
   A 6,325-input-token request also succeeds.
2. An idle 28,025-input-token request with a 32,000 output ceiling succeeds,
   using one actual output token. Subsequent larger requests can fail while
   tiny requests still succeed. Reducing the output ceiling to 128 is not a
   demonstrated remedy.
3. The exact same large request can fail even after 150 seconds without any
   requests from this account. A simple fixed 60-second refill model is not
   established. The observations do not prove the scope or cause of any
   additional provider-side capacity constraint.
4. Four simultaneous tiny requests all succeed. A hard upstream concurrency
   limit of one is therefore not established. Our current one-in-flight gate
   is an operator policy for unknown capacity.
5. Two distinct raw errors have been observed:
   - HTTP 429, code `429001`, type `rate_limit_error`, message
     `inference exceeds tpm/rpm limit`.
   - HTTP 429, code `8`, type `quota_exceeded_error`, message `rpm exhausted`.
   The first code cannot unconditionally be labeled TPM. The second establishes
   a request-rate rejection in this test, but not its numeric threshold or
   whether rejected requests count toward that threshold.
6. No tested response supplies a numeric rate-limit header or Retry-After.
   Locally synthesized Retry-After and cooldowns must remain distinguishable
   from provider reset evidence.
7. A zero-byte HTTP-200 stream is an upstream failure mode, and the current
   Chat/Claude adaptor does not propagate that failure. This is a confirmed
   gateway issue in addition to upstream rate limiting.

## Proposed changes in priority order

### P0: require a valid completion before recording success

Reject empty, malformed, truncated and embedded-error streams on the SenseNova
Chat and Claude paths. Do not update key health or capacity as successful for
those outcomes. Before any semantic response has been delivered, retry another
eligible key within the original request budget. Once semantic output or a tool
call has been delivered, do not transparently replay it on another key: surface
the terminal error using the client's protocol, preserving partial-output and
billing rules. Explicitly verify no duplicate tool execution, duplicate charge,
or failed-response success entry.

Hold client response commitment until valid initial data or an error is known,
within existing timeouts. Do not report an empty response as a completed answer
merely because HTTP headers say 200. Cover the error/no-content path and valid
text, thinking and tool-call completions in the regression tests.

### P0: correct classification and observability

`service/sensenova_error.go` currently maps every `429001` to `tpm`, and does not
recognize the observed code/type/message combination for RPM. Use narrowly
matched provider error tuples to distinguish explicit TPM, explicit RPM,
ambiguous rate limiting, account credit exhaustion and overload. Preserve safe
sanitized customer errors. Do not publish large-workload TPM evidence solely
from the ambiguous `429001` response; the same code can represent other rate
limits. Existing observations expire and require a migration/rollout policy so
stale misclassified evidence does not continue penalizing keys unnecessarily.

Expose account/credential, exact model, original-error category, input estimate,
actual usage, provider hints, gateway wait and all attempt counts separately.
No raw keys or customer content should enter these observations.

### P1: schedule against available capacity, not only rotation order

Keep health checks, owner-protected reservations, cancellation cleanup, distinct
key retries and bounded queues. Add an explicit request-rate budget alongside
the token ledger once its scope and contract are known. Track all dispatched
attempts as request demand locally, including failures, conservatively; this is
a scheduling policy until the provider confirms failed-request accounting.

Choose a currently eligible account by its earliest feasible admission time,
recent comparable-workload outcomes and available budgets. Use rotation to break
ties. Keep same-account keys in a common budget once their account relationship
is known. Do not represent additional credentials for one account as additional
quota. Separate exact-model limits from confirmed account-wide limits.

Prefer the same eligible key for subsequent turns of a conversation to encourage
prefix/cache reuse, with an explicit unavailable-key escape path. Keep stable
system/tool serialization. Account-scoped cache isolation and cached-token TPM
accounting have not been proven, so evaluate this preference with actual cache
usage and complete-task outcomes.

For unknown capacity, retain bounded cautious pacing by default. The successful
workflow demonstrates that a mandatory 60-second gap can delay valid follow-up
turns; the failed repeat demonstrates that unrestricted fast retries are unsafe
for availability. Canary a limited follow-up allowance after a verified complete
response, with a per-key in-flight bound, request-rate budget, and immediate
withdrawal when rate limiting recurs. Tune its size and interval from measured
workflow results rather than inventing a provider quota. Do not globally remove
the guard based on four tiny concurrent successes.

Use one shared internal retry/deadline budget per client request. Count all
attempts across keys and protocol fallback, add jitter to eligible recovery,
and provide a truthful Retry-After when available. Client/SDK retries are not
automatically the same gateway request: any cross-request coordination needs
an explicit trusted identity, without replaying responses or tools. Explain and
configure client retry behavior where supported rather than claiming the
gateway can unconditionally control it. Avoid synchronized retries and repeated
attempts when no key can recover before the deadline. Preserve the client's
context, tools, thinking, model and output ceiling.

### P1: resolve token accounting before enabling numeric TPM configuration

The current known-TPM admission mode reserves input estimate plus the caller's
maximum output. A 28,935 estimated input plus 32,000 output ceiling becomes a
60,935 reservation. Setting an assumed 30,000 TPM limit would reject such a
request locally even though upstream has accepted a comparable request.

Obtain the provider's reservation and settlement rules before enabling this
configuration. If the provider uses actual tokens, implement a documented
reservation policy for input and in-flight output, reconcile actual usage, and
retain uncertainty on canceled/failed requests. Do not silently lower maximum
output or equate maximum output with actual consumption. Cached-token accounting
also requires verification; cache discounts are not established by billing data.

### P2: qualify additional capacity before purchasing or adding accounts

The test cannot calculate a required key count from a single account with an
unknown or unstable admission contract. The earlier suggestion to move from
four to eight accounts was an experiment, not a measured capacity requirement.

For a reliable coding service, prefer capacity with an explicit per-model
TPM/RPM/concurrency contract and permission for the expected traffic. Ask
SenseNova whether metered service or a quota increase provides this for
`deepseek-v4-pro`; the Token Plan subscription and metered API are separate
products. Do not assume purchasing a plan automatically raises this model's
usable throughput.

Before adding an account to the primary pool, accept it with the full tool
workflow at the required cadence, including several consecutive turns and
independent repeated runs. If qualified accounts scale throughput, expand based
on observed demand and headroom. If the same large workload still fails after
idle, adding more unqualified accounts only adds retry candidates. A fallback
upstream must preserve the advertised model and be introduced with reviewed
cost/billing behavior.

Required capacity can be calculated only after those contracts are verified:
for request rate R and per-turn demand T, pool demand is approximately R × T,
plus retries, output and safety headroom. Every individual eligible account must
also admit a whole request. Increasing the number of accounts cannot divide one
request across their independent budgets.

## Acceptance criteria for the next implementation

- Provider errors classify correctly using fixtures from this study; ambiguous
  rate-limit errors do not create false TPM evidence.
- Empty and error streams fail visibly, do not become success evidence, and
  have verified partial-response and billing behavior.
- Request and token reservations are atomic under concurrent routing, with
  correct release, failed-attempt accounting and bounded owner leases.
- No semantic payload or billing changes are hidden inside the limiter.
- Full-context coding workflows complete at the target cadence; report unique
  workflow success, complete-task latency, first semantic output, attempts,
  waits and cost. Fewer internal attempts alone is not acceptance.
- Compare configuration or routing changes against the same workload and
  documented account capacity; retain failed samples and rollback evidence.

## Study completion and operational state

There were **58 direct inference HTTP requests**: 29 diagnostic requests and 29
requests across the two workflows. Status counts were 26 HTTP 200 and 32 HTTP
429. Five of the HTTP-200 responses did not yield a valid completion (four in
the workflows and the direct empty-stream probe); they must not be counted as
successful inference. The initial diagnostic's 16 small/large completed
responses and five completed workflow turns give 21 completed model responses.

No numeric TPM/RPM/window, cached-token admission rule, failed-request debit
rule, or common capacity pool across independent accounts was established.
No model change, quota purchase, new production key, runtime configuration edit,
code deployment, database mutation or customer inference occurred in this study.
Temporary key removal and absence from selected evidence files were verified.
No inference process or temporary regression test remains running/installed.
The report and sanitized evidence are retained with the subsequent completion
integrity repair. See the separate release record for implementation and rollout.

See [study totals](evidence/sensenova-limit-study-20260908/study-summary.json).
