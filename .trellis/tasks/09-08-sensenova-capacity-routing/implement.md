# SenseNova capacity routing implementation plan

> Workers implement the task design with TDD and explicit file ownership.

**Goal:** reduce needless TPM attempts while preserving request semantics.
**Architecture:** owner-checked Redis observations feed existing admission,
with a shared renewable lease for speculative recovery.
**Tech stack:** Go, Gin, Redis Lua, existing GORM health, miniredis tests.
**Spec:** design.md, prd.md, backend SenseNova spec and incident report.

## Global constraints

Source/Git locally; Go and tests on forge. The new remote workspace is
`/root/codex-workspaces/ak47-api-capacity-20260908`; sync without deletion or
secrets. Preserve four attempts, 75-second admission deadline, cancellation,
pricing/group, ownership and settlement. Do not invent provider quotas.

## 1. Observation and pool recovery store (worker)

Own `service/sensenova_capacity.go`, `service/sensenova_capacity_test.go` and
`service/sensenova_budget.go` for its shared base-key and atomic fixed-wait
metadata. Review expanded this ownership to a non-reserving budget-read path.

- [x] Test real budget owners recording success/TPM, independent shape evidence,
  owner replacement/expiry, Redis TIME advances, bounded penalties and TTL,
  concurrent pool claim and stale renewal/release.
- [x] Run `go test -p 2 ./service -run '^TestSenseNovaCapacity' -count=1` to RED
  before implementing the exact design interfaces.
- [x] Implement owner-checked Lua outcomes, shape keys and pool lease lifecycle.
- [x] Run the targeted check to GREEN, gofmt and related budget tests.

## 2. Admission and lifecycle (main session)

Own `service/sensenova_admission.go` and `service/sensenova_capacity_routing_test.go`.

- [x] Seed three same-shape failures and one successful candidate using leased
  admissions. Reset rotation to start at failing keys; assert admission selects
  the successful key without another attempt. Cover fresh-key fairness/recovery.
- [x] Run `go test -p 2 ./service -run '^TestSenseNovaCapacityRouting' -count=1`
  before changing admission; record RED behavior assertions.
- [x] Add stable candidate tiers, bounded waits, pool claim, dispatch counting,
  renewal and cleanup. Publish outcomes before budget release; skip cancellation.
- [x] Run routing and existing focused SenseNova service/model/controller tests.

## 3. Review, build and user-experience evidence

- [x] Independent full-diff review for starvation, lease loss, retry accounting,
  size isolation, privacy and billing regressions.
- [x] Focused race checks and changed-service vet; backend build on forge.
- [x] Update spec/release notes with actual evidence and unknown quota limits.
- [x] Prepare concrete canary/rollback and report exact deployment status.
  Unit tests or one successful workflow do not establish zero 429.
