# AK47 Token SenseNova Key Pool Release

Modified derivative release: `ak47token-2026-09-06-sensenova-pool.1`.
Modification date: 2026-09-06. Based on New API; this is not an official upstream release.

## Material changes

- Native opt-in SenseNova channel preset and independent-account key inventory.
- Shared account and model-specific health, safe failover, durable cooldown and
  bounded recovery probes; administrator disable remains authoritative.
- Per-key health/reasons/check times and permission-checked manual tests.
- Unknown provider balance is shown as N/A; no guessed credit/reset values.
- Preserve ordinary providers, customer billing ownership and stream boundaries.

This application upgrade does not itself import credentials, change prices,
disable existing channels or remap old-only model IDs. Operators configure the
new pool in channel management and review the separate upstream cutover first.

## Source and notices

Complete corresponding source:
https://github.com/xyzxiaoma/ak47-api/tree/ak47token-2026-09-06-sensenova-pool.1

The derivative remains under AGPLv3. Preserve `LICENSE`, `NOTICE` and
`THIRD-PARTY-LICENSES.md`, including the unchanged upstream go-epay inventory
entry. The operator explicitly chose the upstream release requirements and
removed the additional local permission-document gate on 2026-09-06; no new
dependency license or separate permission is asserted by this release record.

Frontend design and development by New API contributors.
Original project: https://github.com/QuantumNous/new-api

## Build and rollback

Build the exact tag using the repository Dockerfile, passing
`VITE_DERIVATIVE_SOURCE_URL` equal to the source URL above. The Dockerfile uses
pinned Bun, Go and runtime images and includes license files in `/licenses`.

Before replacement, save the live Compose configuration, consistent PostgreSQL
dump, image identity and a channel/pricing checksum. Restore-test the dump using
an isolated database; never point a canary at production services. Update only
the `new-api` service after canary checks; do not recreate PostgreSQL or Redis.
Retain the previous image and Compose configuration for immediate application
rollback. The new database table and channel flag are additive; rolling back
the application does not require overwriting live customer data with a backup.
