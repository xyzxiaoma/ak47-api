# SenseNova 429 reliability design

## Boundaries

Only channels with `sensenova_pool=true` participate. Keep the selected channel,
model, pricing group and original request body across retries. Reuse current
fingerprint exclusion and CAS health persistence; no schema migration or new
infrastructure is required. Four independently owned customer accounts do not
prove independent provider-side capacity.

## Request and error flow

1. The existing selector chooses an eligible key and creates attempt-local state.
2. For a rejected HTTP response, capture only a validated Retry-After duration
   and a controlled classification derived from the structured error code.
3. Before sanitizing the outward error, preserve this safe metadata on the
   current attempt and persist its cooldown using the existing CAS snapshot.
4. Keep existing global-versus-model failure scoping. TPM/RPM/capacity diagnostic
   categories do not imply shared credit exhaustion.
5. Retry another eligible, untried key. Admit no more than four total attempts
   and never admit a retry after the existing 90-second boundary, cancellation
   or response-written state. Stop when eligibility is exhausted.
6. On final failure only, attach a safe Retry-After hint when it is known. No
   stale failed-attempt header may leak onto a successful fallback response.

## Retry-After contract

- Reuse the existing `senseNovaRetryAfter` parser and test it deterministically.
  Accept delta seconds and HTTP dates; reject invalid/nonfuture values and cap
  excessive delays with the existing maximum of 86400 seconds.
- Persist `max(existing fallback delay, valid upstream delay)`. Preserve
  60/300/900-second fallback semantics; do not change all 429s to quota failure.
- A cold-pool hint must not advertise an earlier time than relevant known
  account/model cooldowns. Probe scheduling and a successful probe are not
  guarantees that a larger future customer request will fit provider limits.
- Do not sleep inside a request, retry a failed key early, or weaken a valid
  upstream restriction merely because another independent key exists.

## Safe diagnostics

Retain the existing outward redaction and admin-only log boundary. Add bounded
structured metadata under `other.admin_info.sensenova` for failed attempts:

- Controlled limit category: `tpm`, `rpm`, `concurrency`, `capacity`, `quota`,
  `authentication`, or `unknown`, only when supported by an exact known code.
- Attempt number / maximum and a nonsecret key fingerprint reference. Existing
  request IDs continue to correlate attempt rows with one customer request.
- Validated retry-after seconds and, if already available without body reads,
  numeric estimated prompt tokens and explicit requested output-token ceilings.
  Distinguish an estimate from provider-reported token usage and absence from 0.

Never persist an arbitrary provider code, message, raw response body, header
set, prompt or secret. Map known exact codes to application-owned enums; unknown
codes remain `unknown`, including strings resembling credentials. Do not derive
TPM from the number of human users or assert a provider limit from a prior
successful request's token size. Preserve top-level health reason codes for
compatibility; user-visible summaries may include controlled subcategories.

Keep response metadata in root-service request/attempt context rather than
coupling the independent `relaykit` module to gateway state. Reset it on each
selection. No frontend overhaul or log-row deduplication is part of this change.

## Verification

Use fake transports and explicit DB fixtures to prove the actual four-key
failover, final-failure, cooldown and privacy contracts. Existing legacy
non-SenseNova behavior and billing/stream guards must remain intact. Avoid live
provider load tests or replaying customer content to reproduce a rate limit.

## Rollout and rollback

After approval, build the exact tagged source on `forge` using the existing
bounded builder. Back up production Compose and the database; deploy only the
application image, preserving keys, channels and the four-model discount env.
Verify image/source attribution, public health, pricing and safe diagnostic
shape. Retain the current `pricing-flat.1` image and Compose for rollback.
Production deployment still requires explicit authorization. Any subsequent
provider-side finding is reported separately; it is not silently worked around.
