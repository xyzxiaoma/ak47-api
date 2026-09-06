# Flat model marketplace cards — 2026-09-06

This derivative release retains New API licensing and attribution. Version
`ak47token-2026-09-06-pricing-flat.1` replaces group-based overlapping model
stacks with separate cards in a responsive grid (one, two or four columns).
Every model on the current page is directly visible. Model details, copying,
filters, pagination and displayed price calculations remain unchanged.

Removed stack rotation, hidden/inert back layers and the click-to-cycle overlay.
Cards share a consistent shape and stretch to their grid row height. Existing
SenseNova routing and the four-model manual-discount preservation setting are
outside this UI-only change and must remain unchanged during deployment.

The regression test failed against the original stack (the fourth same-group
model was absent) and passed after the change. Both targeted tests, frontend
typecheck, affected-file lint and protected-header formatting passed on `forge`.
Production build and release verification are recorded after deployment.
