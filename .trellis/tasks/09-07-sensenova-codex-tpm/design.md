# Design

## Protocol
Scope Responses-to-Chat conversion to opted-in SenseNova OpenAI channels. Reuse relaykit request conversion and existing Chat-to-Responses streaming/nonstreaming handlers. Preserve tools and custom-tool replay. Keep native OpenAI behavior unchanged. Reject unsupported compact/stateful fields before upstream dispatch.

## Classification
Recognize exact SenseNova string/numeric 429001 as TPM. Preserve privacy and existing classification behavior for unrelated channels/codes. Do not persist raw provider messages.

## Capacity
Use Redis atomic rolling 60-second reservations scoped by channel, key fingerprint and model. No credentials in keys/values. Configured TPM is a local admission budget, not a claimed provider measurement. Reserve conservative estimated input plus explicit output ceiling or a configurable absent-output allowance. A per-key in-flight lease prevents overlapping admissions. Successful usage reconciles the reservation; uncertain failures conservatively retain it until expiry.

Unknown TPM defaults to one in-flight admission and at least 60 seconds between starts per key/model. This is an operational pacing policy, not a claim that the provider resets at 60 seconds. Initially enable admission for deepseek-v4-pro only. Other SenseNova models can opt in by configuration.

Bound waiting across each request to 75 seconds and at most 32 waiting requests per channel, using Redis leases. Honor client cancellation and writer state. Try currently budget-eligible, healthy authorized keys before waiting. Redis failure while enabled fails closed with a sanitized 503; disabling the feature explicitly retains legacy behavior. Never retry more than four distinct dispatched keys; waiting does not increment upstream attempt counters.

Health recovery and capacity are separate: tiny probes cannot erase admission reservations or pacing. Real provider cooldown remains authoritative when longer than a local budget wait. Request validation/billing errors before dispatch release an unused reservation; unknown execution outcomes retain conservative accounting.

## Verification/release
Focused fail-first tests for protocol, real 429001 envelope, atomic reservation/concurrency, cancellation and queue caps. Integrate then run affected controller/service/model/openai regressions and independent relaykit build if touched. Rehearse the patched gateway on forge with isolated credentials and synthetic prompts; real Codex must complete tool use. Prepare exact-commit artifacts, backups and rollback before any deployment.
