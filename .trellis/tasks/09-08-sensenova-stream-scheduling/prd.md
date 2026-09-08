# SenseNova completion integrity and conversation scheduling

User approved the operating plan in docs/sensenova-limit-study-20260908.md on
2026-09-08 and requested implementation plus a realistic key-count recommendation.

Requirements:
- Empty, malformed, truncated and embedded-error Chat/Claude streams must fail.
- Retry only before any semantic output; never replay a delivered tool call.
- Failed streams never establish successful health/capacity or bill a full answer.
- Preserve model, full context, tools, thinking, output ceiling and price.
- Distinguish explicit TPM/RPM, ambiguous limits, credit exhaustion and overload.
- Prefer eligible conversation keys with bounded configurable follow-up pacing;
  preserve conservative defaults, atomic reservations and all request deadlines.
- No invented provider quota, raw credential/content storage or capacity guarantee.
- Validate targeted failures, successful text/tool streams, billing and routing;
  document live-workflow acceptance limitations and rollback settings.
