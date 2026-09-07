# Claude tool accounting and SenseNova recovery

User approved implementation on 2026-09-07 after production investigation found generic-map Claude tools omitted from token estimates and tiny probes erasing TPM failure history.

## Requirements
- Count ordinary and web-search tools decoded from real client JSON, including names, descriptions, schemas and search location. Preserve wire payload and relaykit independence.
- Probe success must not overwrite actual request success timestamps or erase rate-limit failure streaks. A recovered rate-limited key enters pending real-traffic verification.
- Only one leased real request verifies recovery per account. Preserve cooldown deadlines, stale-result protections, administrative precedence, request cancellation and cleanup.
- Keep unknown-TPM pacing, configured quotas, complete client tools and output limits unchanged.
- No dependencies, schema changes or fabricated upstream quota values.

## Acceptance
JSON-shaped regression tests fail before repair and pass afterwards. Model/service regressions cover retained history, exclusive recovery ownership, failed verification backoff, successful verification, cancellation/renewal and stale owners. Run focused Go checks on forge and independent GOWORK=off relaykit build. Report remaining upstream capacity limits honestly.

