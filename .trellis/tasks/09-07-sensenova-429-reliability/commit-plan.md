# Proposed commit plan

One coherent work commit, after final checks and operator confirmation:

`fix(sensenova): retry four keys and honor upstream cooldowns`

## Product and regression files

- `controller/relay.go`
- `controller/sensenova_relay_test.go`
- `controller/sensenova_request_metrics.go`
- `controller/sensenova_request_metrics_test.go`
- `middleware/distributor.go`
- `model/sensenova.go`
- `model/sensenova_test.go`
- `service/error.go`
- `service/sensenova.go`
- `service/sensenova_error.go`
- `service/sensenova_probe.go`
- `service/sensenova_retry_test.go`

## Specification, release and task evidence

- `.trellis/spec/backend/sensenova-pool.md`
- `VERSION`
- `docs/releases/2026-09-07-sensenova-retry.md`
- `.trellis/tasks/09-07-sensenova-429-reliability/prd.md`
- `.trellis/tasks/09-07-sensenova-429-reliability/design.md`
- `.trellis/tasks/09-07-sensenova-429-reliability/implement.md`
- `.trellis/tasks/09-07-sensenova-429-reliability/implement.jsonl`
- `.trellis/tasks/09-07-sensenova-429-reliability/check.jsonl`
- `.trellis/tasks/09-07-sensenova-429-reliability/task.json`
- `.trellis/tasks/09-07-sensenova-429-reliability/bug-analysis.md`
- `.trellis/tasks/09-07-sensenova-429-reliability/commit-plan.md`

All listed paths were edited for this task. No unrecognized dirty files exist
in this repository at the plan snapshot. The adjacent `ak47token` working tree
is outside this commit and remains untouched.

No amend or push in the Phase 3.4 commit step. The already approved deployment
has a separate corresponding-source publication/tag gate; it must use the exact
tested commit and preserve all production channel/key/discount configuration.
