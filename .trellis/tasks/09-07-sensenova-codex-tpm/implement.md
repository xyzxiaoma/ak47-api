# Implementation plan

Approved source: prior diagnostic findings and user approval in this conversation.

- [x] Add failing SenseNova Responses adaptor tests (URL, request conversion, custom tools, streaming and unsupported state).
- [x] Implement conversion using the existing relaykit and OpenAI handlers; run focused tests on forge.
- [x] Add real 429001 string/numeric error regression; implement provider-scoped classification.
- [x] Implement/test Redis rolling reservations, in-flight leases, usage correction, unknown-limit spacing and bounded shared queue.
- [x] Integrate admission after validation, before dispatch; handle initial all-cooling requests, reselection and cancellation without changing authorized channel/group or retry accounting.
- [x] Verify pre-dispatch release, uncertain-failure retention, failed-attempt logging, exactly-once settlement and queue budget across retries.
- [x] Run bounded affected Go regressions on forge and required independent module checks.
- [x] Review the full diff and resolve findings.
- [x] Run isolated patched-gateway and real Codex tool-loop acceptance with synthetic content.
- [ ] Record verified source/version and rollback artifacts for deployment; document final measured behavior and remaining provider-capacity limits.
