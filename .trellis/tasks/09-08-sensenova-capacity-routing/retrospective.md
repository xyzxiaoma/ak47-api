# Recurring 429 retrospective

The earlier repairs addressed retry count, token estimates, pacing, diagnostics,
and false health recovery. They did not establish per-account numeric TPM or
teach selection which keys recently handled comparable workloads. A successful
three-turn canary with internal retries was insufficient evidence of sustained
reliability; the next large workflow again tried all keys repeatedly.

Root category: conflating credential/model health with workload capacity, plus
measuring internal attempt counts instead of unique client outcomes. A healthy
credential and a tiny successful probe do not prove capacity for a 28,935-input,
32,000-output-ceiling request. Unknown budgets still enforce pacing only.

Prevention now lives in the backend SenseNova spec and targeted regressions:

- Numeric workload evidence expires independently of health, never records
  content or raw credentials, and only live dispatched owners can update it.
- Fresh capacity remains usable, with four distinct recovery attempts retained;
  a hard one-recovery request limit would conceal fourth-key success.
- Compare fixed wait lower bounds against the shared deadline before queueing.
  Health leases, shape penalties and budget pacing must be combined per key.
- Normal cancellation of a renewal worker must not cancel a valid request;
  actual lost ownership must cancel upstream dispatch and response reads.
- Release acceptance must report client-level success and first-content latency,
  including pre-dispatch rejections and internal waiting. Synthetic regressions
  do not prove the upstream can meet a capacity or latency SLA.

Provider quota verification and additional independent capacity remain separate
operational work. No payload truncation, tool removal, output reduction or
pricing changes are acceptable substitutes for capacity.
