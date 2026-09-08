# SenseNova recurring TPM failures: evidence and proposed solution

Investigated 2026-09-08. The diagnosis below was collected read-only, with no
additional inference calls or customer content collection. The user subsequently
approved implementation; the refinement below preserves four recovery attempts.
Implementation and release evidence are tracked separately in the capacity
routing task and release record.

## Confirmed incident

The screenshot corresponds to production channel 15 in this repository, not
the adjacent `ak47token` application. Production is running
`ak47token-2026-09-07-sensenova-recovery.2`, so the previous repair is deployed.

The following times are Asia/Shanghai. Final structured request-latency records
were joined to the administrator allowlisted attempt metadata in log rows
6213–6225 using request IDs.

| Request start | End | Upstream attempts | Final outcome | Gateway wait |
| --- | --- | --- | --- | --- |
| 10:44:14 | 10:44:30 | 1 | Success, 835 input / 546 output | 0s |
| 10:44:15 | 10:45:15 | 4 | Failure, all four classified TPM | 56.453s |
| 10:45:31 | 10:46:16 | 4 | Failure, all four classified TPM | 41.029s |
| 10:46:33 | 10:47:59 | 4 | Success after three TPM failures | 41.037s |

All eleven failed attempts have `limit_kind=tpm`, `estimated_prompt_tokens=28935`
and `requested_max_tokens=32000`. No positive upstream retry delay was retained
in these attempt records. The successful large request used 27,354 input and
2,403 output tokens. Its gateway duration was 86.027s; the successful upstream
attempt took 41.245s, with first semantic content after 3.566s from dispatch.

Thus the thirteen rows are eleven failed internal attempts and two successful
consume records, belonging to four gateway requests. Two of the three large
requests failed. Whether those three requests were automatic client retries or
manual resubmissions is not established by numeric request metadata.

The three repeatedly rejected keys previously had real success observations;
they are not proven invalid or permanently incapable. The fourth key also
rejected this size twice before succeeding. This is evidence against assuming
that every idle key can serve the request, but not proof of a fixed per-request
limit, a numeric TPM threshold, or shared account quotas.

## Why the existing repairs do not prevent this

1. **Production has pacing, not a configured token budget.** Exact allowlisted
   environment inspection found no SenseNova admission overrides. Defaults are
   admission enabled for `deepseek-v4-pro`, one in-flight request and at least
   60 seconds between starts per key/model, a 75-second request-wide admission
   deadline, and at most 32 queued requests per channel.
   `service/sensenova_budget.go:175` only calculates an estimated debit and
   checks request size when `TokensPerMinute > 0`. Unknown budgets reserve a
   zero token debit and enforce start spacing. Better token estimation alone
   cannot change that production decision.
2. **Eligibility does not represent capacity for a particular request size.**
   `service/sensenova.go:158` rotates the starting key. Expired rate cooldowns
   may participate as leased real verifiers. `AdmitSenseNovaAttempt` checks
   candidates in rotation order without preferring verified real capacity over
   a key repeatedly rejecting comparable requests. Retained failure history
   does not influence this ordering.
3. **Fixed cooldowns cannot discover provider capacity.** Repeated TPM failures
   retain a minimum 60-second cooldown. The prior change correctly avoided
   blocking an entire key/model for 5–15 minutes merely because rate failures
   accumulated, but now similarly sized requests can revisit the same failing
   candidates once per minute. Simply restoring that global escalation would
   repeat the earlier availability regression.
4. **Past acceptance demonstrated recovery, not reliability.** The previous
   three-turn Claude acceptance succeeded, but its Read turn already needed
   four attempts and 43.412s of waiting. It did not establish sustained capacity.

The operator previously confirmed different supplier accounts for the keys.
Keep that context; do not ask for account independence again or infer shared
accounts from correlated failures. Provider-level shared capacity, external
traffic, window mechanics, cache accounting and output reservation rules remain
unknown. The official FAQ mentions limiting and peak-load queueing but does not
establish this model's applicable numeric TPM contract:
https://github.com/OpenSenseNova/SenseNova-Skills/blob/main/docs/faq_CN.md
Its older request-count figures must not be substituted for TPM. The current
platform documentation could not be fetched during this investigation.

## Options and recommendation

| Option | Benefit | Limitation | Recommendation |
| --- | --- | --- | --- |
| Increase retries or use a longer fixed sleep | Occasionally reaches a later successful attempt | More latency and repeated rejection; no capacity information | Do not use as the main repair |
| Capacity-aware selection and bounded recovery | Reduces reuse of recent failures and unnecessary provider calls | Cannot create missing supplier capacity | Implement as the gateway improvement |
| Verified quota policy, plus an explicitly authorized independent fallback | Makes admission quantitative; fallback can improve availability | Requires applicable provider facts and possibly different costs | Required for a defensible stable-service commitment |

### First implementation: selection and recovery policy

- Reuse existing numeric request metrics, health generations and Redis leases.
  Separate admission capacity observations from credential/model health.
- Prefer currently admissible keys with recent successful real traffic of a
  comparable request size. Within equivalent candidates retain fair rotation.
  Treat observations as expiring evidence, never as measured quota or a promise
  that the next request will succeed.
- After repeated TPM failures for comparable large requests, reduce that key's
  participation in that workload using a bounded, expiring scheduling penalty.
  Do not globally disable small requests or other models. A tiny success must
  not erase the large-request penalty. Small-request admission must still obey
  existing account/model health restrictions.
- Bound concurrent recovery across the pool as well as per key: allow one
  verifier concurrently per pool/model. Preserve up to four distinct-key
  attempts, including recoveries. Review rejected the original one-recovery
  request cap because it could miss a successful fourth key. If verified candidates are
  temporarily busy, use their earliest admissible time within the existing
  shared deadline; otherwise return a bounded capacity-unavailable response.
  Measure unique-request success and latency, not merely fewer failed attempts.
- Retain a bounded exploration path and expire penalties so a formerly limited
  key can recover; do not permanently concentrate traffic on one successful key.
- Honor provider Retry-After as a lower bound. Keep cancellation, no retries
  after response commitment, same model/group, at-most-once settlement and
  generation ownership invariants. Do not conceal internal errors in reports.

### Quantitative admission and capacity boundary

- Obtain TPM/RPM/concurrency and accounting rules applicable to these Token
  Plan accounts and this exact model. A metered API quota table may describe a
  different product. Confirm output-ceiling reservation, cached input charges,
  window/reset behavior and any account/model shared constraints.
- Configure the existing per-key/model budget only from applicable evidence,
  with a documented local safety margin. If credentials share an established
  quota scope, admission needs an additional atomic reservation at that scope;
  do not assume that scope exists merely because keys fail together.
- Under the current conservative known-budget formula, this request would
  reserve 28,935 + 32,000 = 60,935 tokens. That is a proposed local reservation,
  not a claim that SenseNova actually debits 60,935 tokens. Actual output of
  2,403 is unavailable at dispatch and cannot justify reserving that amount.
- A request larger than a verified capacity must receive a clear, actionable
  pre-dispatch error; waiting a minute cannot make a fixed oversized request
  smaller. Choose error semantics that do not encourage endless automatic
  retries of a permanently inadmissible request.
- If official quotas remain unavailable, expose policy mode as unknown/pacing.
  Use expiring observed-capacity preferences, bounded waiting and explicit
  unavailability; do not fabricate a TPM value or advertise guaranteed capacity.
- Context compression or lowering the 32,000 output ceiling is a separate,
  explicit client choice. It can change reasoning/output behavior. Do not trim
  tools, reduce thinking, truncate context or clamp output inside the gateway.
- If uninterrupted large-request service is required when this pool rejects
  traffic, an independently provisioned same-model upstream is the operational
  fallback. It needs verified identity/capacity and explicit routing/cost
  authorization before activation; a different model is a separate decision.

## Verification required before calling the repair successful

Use synthetic requests with the observed numeric sizes, without copying user
content. Run focused tests on forge before any production change.

1. Three keys repeatedly reject the large class while a fourth has recent
   success: choose admissible verified capacity without first burning three
   attempts on the rejected keys; wait for it only within the shared deadline.
2. All keys lack reliable capacity: bounded exploration, pool-wide concurrency
   ownership, expiring penalties, clear final status and no infinite wait.
3. Small requests and unaffected models retain service; one tiny success does
   not erase a large-request failure history. Large success does not establish
   a permanent size threshold.
4. Multiple gateway instances, lease loss, cancellation, stale results and
   unsent reservations preserve existing ownership and billing guarantees.
5. Known-budget request-too-large and sufficient-capacity wakeups remain
   correct; unknown-budget mode is reported honestly.
6. After an authorized canary, evaluate unique gateway request success,
   end-to-end first-content latency including waiting, attempts per request and
   final 429/503 rate by request-size class. Include pre-dispatch rejections.
   Report a consecutive multi-turn workload and natural-traffic observation,
   with denominator and time window; one successful workflow is insufficient.

No source code, runtime configuration, keys, prices, deployments or provider
quotas were changed during this investigation. This document records a proposal,
not verified improvement in production reliability.
