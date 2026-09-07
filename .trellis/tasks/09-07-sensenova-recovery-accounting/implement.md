# Claude accounting and recovery Implementation Plan

**Goal:** Repair two confirmed causes of inaccurate capacity accounting and false recovery.
**Architecture:** Decode tools at the DTO boundary; retain rate failure history and require an owned real-request recovery lease.
**Tech Stack:** Go, GORM, existing Redis admission. Runtimes/tests on forge only.
**Spec:** prd.md, design.md and .trellis/spec/backend/sensenova-pool.md.

## 1. Tool accounting (relaykit/dto only)
- [x] Add a JSON-decoded Claude request with ordinary and web-search tools; assert ToolsCount and text includes description, nested schema and location, and serialized tools remain unchanged.
- [x] Run `GOWORK=off go test ./dto -run 'Claude.*Tool|ProcessTools' -count=1`, confirm original implementation fails.
- [x] Normalize generic-map tools with kitutil wrappers, retaining typed compatibility and avoiding nil-pointer panics.
- [x] Run the regression and `GOWORK=off go build ./...`.

## 2. Recovery (model and service)
- [x] Reproduce tiny-probe failure-history reset and synthetic LastSuccessAt in model tests.
- [x] Preserve real traffic history; pending rate recovery uses untested with rate_limited reason.
- [x] Add claim/renew/release using snapshot generations and existing lease columns; test exclusive ownership, expiration, cancellation and stale claims.
- [x] Integrate claims after budget reservation, and in admission-disabled path; renew with cancellation and release on all exits.
- [x] Allow due rate recovery selection without a probe scan; recheck cooldown at claim. Preserve other failure classes' recovery requirements.
- [x] Retain pending model state in the console, render its existing Untested label, and pass frontend typecheck/file lint/format.
- [x] Run targeted SenseNova model/service/controller regressions; assess affected interfaces and avoid unrelated matrices.
- [x] Address independent review finding: context-bound renewal/release SQL, with deterministic blocked-connection red/green regression.
- [x] Correct live-discovered rate-limit escalation: retain failures but use max(60 seconds, provider Retry-After); preserve other health backoff. Regression failed before the repair; model/service/controller SenseNova tests passed afterwards (0.989s/5.653s/0.154s). Supplemental independent review found no concrete regression.

## 3. Review and finish
- [x] Review all affected packages and race-sensitive lifecycle, document new invariants in SenseNova spec. Independent review corrections passed focused normal and race tests.
- [x] Record validation and remaining capacity limits; source commits 190029f and 23f55b3 published, recovery.2 deployed, full public Claude Write/Read/final acceptance passed in 79.42s (three 200s), temporary token removed. Release notes retain failed recovery.1 evidence and 43.412s capacity wait in successful acceptance.
