# Design

## Tool accounting
Normalize map[string]any ordinary and web-search definitions in relaykit/dto/claude.go ProcessTools using relaykit kitutil JSON wrappers, preserving existing typed inputs. Test token metadata from JSON-decoded ClaudeRequest, not just manually typed structs; no mutation of the client tool payload.

## Recovery
Keep the existing public state vocabulary. After a rate_limited probe succeeds, set state=untested while retaining reason=rate_limited, Failures and LastFailureAt; only real traffic updates LastSuccessAt and clears failure history. Non-rate probe recovery keeps existing routing behavior but does not manufacture real success timestamps.

Add request-owned recovery leases to model snapshots using the existing version and lease columns, without migration. Request selection also accepts an expired rate_limited cooldown for real verification, eliminating dependence on the 15-second probe scan; quota, authentication and transport restrictions still require their existing recovery path. Selection reads eligibility; claim occurs after capacity reservation (or in admission-disabled dispatch path) so abandoned candidates cannot hold a lease. Pending recovery with an active lease is not eligible. Claim locks account then scoped rows, claims both generations, and renews while upstream runs. Real success requires ownership to clear pending recovery. Failure or cancellation releases only matching generations; lease loss cancels the upstream. A stale owner must not release a new lease or overwrite a newer result. Existing probe cooldowns and bounded four distinct-key attempts remain. The console retains untested/rate_limited model restrictions until real recovery succeeds and renders their existing Untested label.

## Constraints
No assumed TPM quota, shorter unknown pacing, silent tool removal, client output clamp, new provider or schema migration. Tiny probes remain small and retain their existing scheduler/cooldown rules. This correction removes false recovery evidence; stable full-context latency still depends on provider capacity.
