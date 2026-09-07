# Claude token accounting and SenseNova real-request recovery

Release candidate: `ak47token-2026-09-07-sensenova-recovery.2`.
Modified on 2026-09-07. Deployment and public acceptance are recorded below only
after they have been verified.

## Changes

Claude JSON tool definitions now participate in token estimation. Previously,
generic maps decoded from client JSON were ignored by a typed-only switch.
Ordinary tool descriptions and nested schemas, and web-search location, are
counted without modifying the request sent upstream. Typed tool inputs remain
supported and nil pointers are skipped safely. The independent relaykit module
continues using its own JSON wrappers.

SenseNova recovery now distinguishes tiny probes from real traffic. A probe
does not update the real-traffic success timestamp. After rate-limit failures,
a successful probe preserves failure history and leaves the key pending real
verification (`untested`, reason `rate_limited`). Probe transport/model failures
cannot erase that preceding rate-limit history.

After a rate cooldown expires, a real request can select the key for verification
without waiting for the 15-second probe scan. Selection does not mark it healthy.
Following capacity admission, one request claims a 120-second account/model
lease, renewed every 30 seconds while it runs. Only a successful owner can clear
the rate failure history. Failed rate verification retains a minimum 60-second
cooldown and any longer provider Retry-After without escalating to health-failure
backoff merely because the failure count increases. Cancellation and pre-dispatch
failures release owned leases; expired or superseded owners cannot renew,
publish results, or release a replacement owner's lease. Lease loss cancels
the upstream context. Renewal and release SQL have 2-second deadlines, preventing
blocked database connections from hanging request cancellation or cleanup.

This path also applies when Redis capacity admission is disabled. Quota,
authentication and transport failures retain their existing health recovery
requirements. The console keeps pending model restrictions visible without
exposing leases or generations.

## Scope and limits

No dependencies, database schema, customer tool declarations, output limits,
pricing settings, provider credentials or numeric provider quotas are changed.
Unknown-quota pacing remains one in-flight request and at least 60 seconds
between starts per key/model. Four keys are not assumed to have four independent
quotas. Full-context requests can still receive upstream TPM errors.

This repair removes omitted token accounting and false recovery decisions. It
does not establish a stable first-output SLA or promise zero 429s. A persistent
real-traffic failure streak remains visible until real recovery succeeds.
Non-rate health failures retain progressive 60/300/900-second backoff.

## Validation

- Tests reproduced the original wire-tool omission, false probe timestamp and
  streak reset, unowned recovery, hidden pending model restriction, and mandatory
  probe-scan delay before the corresponding fixes.
- The same captured synthetic 25-tool Claude request now counts all 25 tools and
  estimates 18,378 tokens, matching its equivalently typed form. The original
  code estimated 3,733 tokens and counted zero tools; real input was about 17,500.
- Focused SenseNova model/service/controller tests passed on forge.
- Claude DTO regressions and independent `GOWORK=off go build ./...` in relaykit
  passed on forge.
- Frontend typecheck and changed-file lint/format passed on forge.
- Focused lifecycle tests with `-race` passed after the review corrections
  (model 5.786s, service 6.885s). Deterministic blocked-connection tests protect
  normal shutdown, renewal deadline failure and bounded cleanup.
- Independent full-diff review found and fixed unbounded renewal/release SQL;
  no remaining concrete code findings were reported.
- Image build and deployment records are pending at this draft stage.

## Supplemental correction after first deployment

Release `.1` was deployed at 2026-09-07T15:14:55.775800793Z. Public Claude Code
acceptance with all 25 tools, 32,000 output tokens and adaptive thinking wrote the
correct file, but the Read turn received 429 and then 503 with Retry-After 201.
The complete task failed after 193.79 seconds. Tool accounting was verified live
(18,371 estimated input versus 17,462 actual input).

Preserving rate-limit failure history exposed generic health backoff: repeated
TPM failures escalated to five and fifteen minutes. Release `.2` keeps those
counts but uses max(60 seconds, provider Retry-After) for rate limits; other
health failures retain progressive backoff. A regression reproduced the second
failure deadline 1360 instead of 1120 before repair and covers repeated owned
recovery failures plus a longer 450-second provider instruction.

The first acceptance window also contained transient slow database queries
across several tables. Their cause was not established; subsequent inspection
showed idle connections and no continuing slow-query events.
