# Implementation and verification plan

## Approval gate

The user approved task creation, account ownership, the final scope, testing and
deployment on 2026-09-07. The task is now `in_progress`; release remains gated by
the targeted checks and source/backup verification below.

## Ordered work

1. Activate the reviewed task; load current root AGENTS and applicable backend
   specs through `trellis-before-dev`. Preserve all unrelated edits.
2. Add the targeted four-key regression in
   `controller/sensenova_relay_test.go`, extending real selection/failure flow.
   Verify the fourth-key success case fails with the current three-attempt cap.
3. Extend bounded retry admission to four total attempts for opted-in pools;
   preserve first-selection reuse, exclusions, group and writer/cancel gates.
4. Add exact-code classification tests in `service/sensenova_error_test.go`
   and malicious metadata/redaction cases in existing error/service tests.
5. Capture safe attempt-local Retry-After metadata at `RelayErrorHandler`,
   carry it to failure persistence and final response hints. Add seconds/date,
   malformed/oversized, stale-header and independent-key scope regression tests.
6. Attach controlled admin diagnostic fields to failed-attempt logs. Reuse
   existing estimated-token data only; never retain request bodies or raw
   provider metadata. Preserve nonadmin log stripping.
7. Run the narrow controller/service/model regressions on `forge`, expanding
   only if shared-interface behavior changes or targeted checks fail. No local
   runtime installation, provider load tests or unrelated full-suite run.
8. Apply `trellis-check`, review cross-layer retry/error/billing flow and update
   the SenseNova spec to match the new tested retry and privacy contracts.
9. Report test results and remaining upstream uncertainty; obtain commit and
   deployment authority as required. Publish exact source/tag and bounded build.
10. If deployment is approved, back up, change only the image, verify health,
    source/attribution, all sixteen discount ratios and retained channel/key
    configuration. Preserve rollback image and stop the builder.

## Relevant files

- `controller/relay.go`, `controller/sensenova_relay_test.go`
- `service/sensenova.go`, `service/sensenova_error.go`, `service/error.go`
- `service/sensenova_probe.go` (reuse parser), associated service/error tests
- `model/sensenova.go` only if needed for a safe cold-pool retry hint; preserve
  schema and CAS/generation contracts, adding narrow model tests if changed
- `.trellis/spec/backend/sensenova-pool.md`, release documentation and VERSION

Avoid changes to relaykit unless truly required. If its public interfaces are
changed, independently build `relaykit` with `GOWORK=off` on `forge`.

## Acceptance evidence

- First three keys reject, fourth succeeds with one settlement and unchanged
  model/group/body; four rejections stop after four different keys.
- Empty eligibility, cancelled clients, already-written streams, and elapsed
  retry-admission windows do not launch extra upstream work.
- Retry-After affects only appropriate key/model cooldown and final errors;
  successful fallback has no stale failure hint.
- Known limits are diagnosable; unknown/credential-like values and request
  content never enter logs, client errors, or health state.

## Verification evidence — 2026-09-07

- Four-key controller regression failed against the old three-attempt code,
  then passed with the shared four-attempt bound.
- Final snapshot passed on `forge` in pinned Go container (3 GiB, 2 CPUs):
  `go test ./controller ./service ./model ./middleware ./relay/helper -run
  'TestSenseNova|TestRelayErrorHandler|TestPreConsumeBillingRejects|TestTryTieredSettle_PreConsumeMatchesPostConsume|TestBillingSessionReserveWalletTopUpDecrementsBalance|TestStreamScannerHandler_ClientCancelAbortsUpstreamAndReturns'
  -count=1`. Middleware compiled with no matching tests; other packages passed.
- Included final manual-probe cooling guard, code-only provider errors, per-key
  maximum / pool-minimum Retry-After, generic error compatibility and cancellation.
- The four-key test composes real middleware/selection/controller retry logic;
  it is not a complete authenticated HTTP Relay integration test. Billing safety
  has dedicated tests plus inspection that reservation is outside the retry
  loop, success returns once and original body storage is reused.
- No relaykit public API or frontend behavior changed. No unrelated full suite,
  database matrix, real provider load test or customer-content replay was run.
- User confirmed the single work-commit plan and continued deployment on
  2026-09-07. Publish/build exact corresponding source after that commit.
