# SenseNova 四 Key 频繁 429 排障与修复

## Goal

Identify why a single customer's DeepSeek V4 Pro requests frequently fail with
429 despite four configured SenseNova keys, then address verified gateway
defects without weakening upstream limits or changing customer model semantics.

## Confirmed evidence

- Read-only production inspection on 2026-09-07 14:30 China time: channel 15 is
  enabled, has four distinct keys, polling mode, and no manually disabled keys.
- The user confirmed that the three added keys belong to three different
  SenseNova accounts, making four independent customer accounts in this pool.
  Shared provider-side model capacity is still possible; account independence
  alone does not identify the historical rejection subtype.
- For channel 15 today, 18 failed upstream attempts belong to nine distinct
  request IDs; three customer requests succeeded. Error rows are attempts, not
  one-to-one customer failures (`controller/relay.go:241`, `:434`). Requests
  rejected when the pool is already unavailable are not included in this count.
- All four key indices appear in failed attempts. At 13:18:04–08 all four keys
  received 429; all passed tiny model recovery probes at 13:19:12–27.
- Successful requests included 632/346, 33716/1563 and 602/79 input/output tokens.
  Failed request token sizes and requested output ceilings are not retained.
- Actual relay failures are reduced to `SenseNova: rate_limited`, discarding
  the provider error code (`service/sensenova.go:176`); response body logging is
  suppressed intentionally to prevent credential exposure (`service/error.go:98`).
  Today's application log also contains no retained model/account limit codes.
- Pool requests allow only three total attempts (`controller/relay.go:355`),
  even when four keys are eligible. This is a verified limitation, but not proof
  it caused every observed failure: the remaining fourth key also failed in the
  next request at 13:18:08.
- Relay failure persistence currently passes zero Retry-After
  (`service/sensenova.go:165`); recovery probes can read it.
- No production code, configuration, keys or prices changed during diagnosis;
  no extra live inference requests were sent.

## Requirements

- Distinguish verified observations from TPM/RPM, concurrency, shared-account
  quota and upstream capacity hypotheses. Do not infer independent quotas from
  key count or claim historical upstream codes that were not retained.
- Preserve safe diagnostic metadata sufficient to identify the next rejection,
  including per-attempt key identity and provider-controlled limit category.
  Never expose credentials, prompts, raw provider bodies or arbitrary messages.
- Allow up to four distinct eligible keys per request, with no repeat of a
  failed key; stop earlier when none is eligible. Preserve the existing
  90-second retry-admission, cancellation, response-written, billing and group
  gates. Do not replay an emitted stream or retry the same key in a loop.
- Honor a valid upstream Retry-After when cooling the affected key/model and
  provide a safe retry hint on the final failed response. Keep existing fallback
  cooldowns when upstream metadata is absent or invalid. Do not add an
  in-request sleeping queue or change user output/token limits.
- Keep user-selected models, thinking/output semantics, prices and other
  providers unchanged. No automatic switch to a cheaper model or paid upstream.

## Acceptance Criteria

- [x] Evidence establishes whether keys were actually used and separates retries
  from unique customer requests.
- [x] User approved the reviewed repair scope, testing and deployment.
- [x] A four-key test proves first three rejections can reach a successful
  fourth key; all-rejected and all-cooling cases terminate without repeats.
- [x] Deterministic tests cover Retry-After seconds/dates, invalid or excessive
  headers, per-attempt reset, model scope and safe final response hints.
- [x] Known provider limit codes map to controlled categories; unknown or
  malicious metadata cannot expose arbitrary values or customer content.
- [x] Existing cancellation, response-written, group and billing tests pass.
- [x] No secret or user-content exposure and no duplicate billing/replay
  (privacy regressions and composition tests; preconsume-outside-loop review).
- [x] Production changes wait for plan review and deployment authorization.

## Out of scope and deferred evidence

- Actual provider rejection subtype and applicable limit remain unconfirmed
  because historical structured upstream codes were discarded. This is a
  deferred operational observation, not a reason to invent a TPM threshold.
- No model/provider switching, paid fallback, new credentials, price changes,
  content truncation, thinking changes, global limiter changes or quota bypass.
- No promise to eliminate provider-side 429s. This scope fixes verified gateway
  limitations and makes the next rejection diagnosable without sensitive data.
- Final scope and commit/deployment were approved on 2026-09-07. The tested
  release was deployed and passed production verification; see release notes.
