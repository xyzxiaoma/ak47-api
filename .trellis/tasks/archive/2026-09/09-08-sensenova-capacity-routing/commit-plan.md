# Proposed commit and release

## One work commit

`fix: route SenseNova requests by observed workload capacity`

- `service/sensenova.go`
- `service/sensenova_admission.go`
- `service/sensenova_admission_test.go`
- `service/sensenova_budget.go`
- `service/sensenova_budget_test.go`
- `service/sensenova_budget_renewal_test.go`
- `service/sensenova_capacity.go`
- `service/sensenova_capacity_test.go`
- `service/sensenova_capacity_routing.go`
- `service/sensenova_capacity_routing_test.go`
- `service/sensenova_capacity_review_test.go`
- `service/sensenova_pending_budget_test.go`
- `model/sensenova_routing.go`
- `model/sensenova_routing_test.go`
- `VERSION`
- `.trellis/spec/backend/sensenova-pool.md`
- `docs/sensenova-429-investigation-20260908.md`
- `docs/releases/2026-09-08-sensenova-capacity.md`
- `.trellis/tasks/09-08-sensenova-capacity-routing/` (PRD, design, plan, review,
  retrospective and context manifests, including this commit plan)

Unrecognized dirty files in this isolated worktree: none.

## Publication and production step

After confirmation, publish the corresponding source and immutable tag
`ak47token-2026-09-08-sensenova-capacity.1`; verify public source reachability
before deploying its matching image. Back up production Compose and database,
record configuration hashes, then replace the application image and verify
health, public version and attribution. Retain the preceding image for rollback.
Database and Redis services remain running. Run the full-context canary without
changing output ceilings, tools, thinking or pricing, and report its complete
outcome, including all attempts and waiting. Do not claim a guaranteed capacity
or latency SLA from a single workflow.

Task archival and the session journal follow work/release evidence commits,
using Trellis after the accepted scope is complete.
