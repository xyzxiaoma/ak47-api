# SenseNova capacity-aware routing

## Goal and authorization

The user approved the solution in `docs/sensenova-429-investigation-20260908.md`
and requested implementation with good user experience. Reduce needless failed
attempts and bounded waiting for large requests without changing content, model,
tools, output ceilings, thinking, pricing or billing semantics.

## Deliverable

Use expiring numeric workload observations to prefer recent successful capacity
over repeatedly rejected capacity. Retain fair admission for fresh keys. Limit
real recovery of rejected workloads to one concurrent verifier per channel/model
across instances. Retain the four-distinct-key bound for all capacity, including
recoveries, so a fourth working key can still complete the request. Keep existing
request deadlines.

No provider quota is invented. Existing configured TPM budgets remain intact;
unknown quotas continue conservative pacing. Paid/new-provider fallback is not
activated. Do not promise zero upstream 429 or an unsupported latency SLA.

## Acceptance

- Three repeatedly failing large-request candidates do not precede admissible
  recent successful capacity; a busy good key can be awaited within the deadline.
- Added fresh keys participate fairly; stale evidence cannot permanently pin
  requests to one key or disable another.
- Evidence is scoped by channel, key, model and numeric request-size class;
  tiny success cannot erase another class. Health and Retry-After still apply.
- Pool recovery leases are renewable, owner-checked, released on unsent failure,
  cancellation and completion, and cancel requests on loss.
- Only dispatched owners publish outcomes; stale or canceled work cannot
  overwrite evidence. No credentials/content stored.
- No replay after stream commitment, cross-model rerouting, double settlement,
  growing wait deadline, uncertain-debit refunds, or admission-disabled regression.
- Focused RED/GREEN, lifecycle/concurrency and integration tests on forge;
  independent review before completion. Live canary preserves full request
  semantics and records request outcomes including internal waits.
