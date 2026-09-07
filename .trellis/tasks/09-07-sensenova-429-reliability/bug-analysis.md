# Bug Analysis: SenseNova pool 429 reliability

## 1. Root Cause Category

- B, cross-layer contract: real relay errors discarded Retry-After while probes
  understood it; decoding also discarded structured errors with no message.
- D/E, coverage gap and assumption: selection rotated four accounts over time,
  but each request was capped at three attempts. Four configured keys did not
  imply that a request could reach the fourth.
- These are demonstrated gateway weaknesses, not proof of the upstream reason
  for historical 429s. The operator confirmed independent account ownership.

## 2. Why Fixes Failed

The original test covered two-key fallback, not four-key exhaustion. Adding
keys alone left the three-attempt cap and missing cooldown metadata unchanged.
The new four-key regression failed on the original code before implementation.

## 3. Prevention Mechanisms

| Priority | Mechanism | Action | Status |
| --- | --- | --- | --- |
| P0 | Shared bound | Four distinct attempts, existing replay gates | Implemented |
| P0 | Cross-layer metadata | Bounded attempt-local cooldown and safe category | Implemented |
| P0 | Regression | Fourth-key success and four-rejection stop | First pass passed |
| P1 | Privacy | Admin-only allowlist, no original body/code/secret | First pass passed |
| P1 | Probe consistency | Manual probes cannot bypass future cooling | Remote regression passed |

## 4. Systematic Expansion

Reviewed initial middleware selection, controller retry admission, service
error decoding, model persistence, probe claims and token-log redaction.
Preserve pricing group, billing session, client cancellation and written-stream
gates. Do not invent upstream TPM budgets, sleep queues or account balances.

## 5. Knowledge Capture

Updated `.trellis/spec/backend/sensenova-pool.md` in this task. This repository
does not ship a `src/templates/markdown/spec` tree; no generated spec template
needs synchronization. Changes belong in the same reviewed work commit.
