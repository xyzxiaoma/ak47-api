# Evidence and prevention

On 2026-09-07, real Claude Code 2.1.263 completed the same Write/Read task through
the public gateway with these observed totals: 25 tools/max_tokens=32000,
203.78 seconds; 25 tools/max_tokens=4096, 158.56 seconds; 6 tools/max_tokens=4096,
16.05 seconds; a repeat of the 6-tool profile, 84.74 seconds. All tasks completed
correctly. Client 429 appeared in every trial except the first 6-tool trial.
Production load was not isolated, so these are small observations, not a measured
speedup or long-term reliability claim. Lowering the output ceiling alone did
not eliminate the failures; no client defaults are modified by this task.

One repeat request took 53.63 seconds to return 429, with 50.512 seconds spent
waiting for gateway capacity. Four attempted keys all returned TPM errors.
The client then honored Retry-After: 7 and retried successfully. Fast successful
attempt output does not represent end-to-end user latency across queueing and
client retries.

The same 25-tool captured synthetic request produced ToolsCount=0 and an estimate
of 3733 when decoded from wire JSON, versus ToolsCount=25 and 18378 after typed
conversion. The repair gives 25 and 18378 directly from the wire request.

Root causes crossed two boundaries: JSON decoder output versus typed accounting,
and tiny health probes versus real request capacity. Prevention: test decoded
wire shapes and unchanged serialization; preserve actual traffic history; require
leased real verification after rate cooldown; assert stale-owner, cancellation,
deadline and model-scope behavior. Do not use a tiny success as proof that large
payloads fit, and do not infer independent provider capacity from key count.
