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

## Production verification — 2026-09-06

- Deployed to `https://ak47token.com` at approximately 06:17 UTC.
- Exact source revision: `b3f2637a3fdac9b49462db6287531e8f7488e66f`.
- Image: `new-api:ak47token-2026-09-06-sensenova-pool.1`, manifest
  `sha256:e754d20fbccf936192dbbb3365bbea672f6879973d33c73b1aa9fe44e27efe20`.
- Built the exact published tag on `forge` using the repository Dockerfile and
  an isolated Buildx `docker-container` builder capped at 3 GiB and two CPUs.
  The legacy builder attempt failed during layer export; no failed artifact
  was deployed. Frontend and backend builds completed with BuildKit.
- Restored the production dump into a temporary internal-network PostgreSQL
  instance. Candidate migration, unauthorized-access rejection, native pool
  creation/deduplication, opaque key identities, stale-list rejection and
  manual key disable/enable checks passed using only fake fixture keys.
- Candidate and production public status, pricing, exact-version source link
  and upstream attribution checks passed. External version verification and
  the production container health check passed after replacement.
- Channel, model and option configuration digests matched before and after
  deployment. Digest normalization excludes request accounting, probe timing,
  polling position and `upstream_model_update_last_check_time`; the isolated
  comparison showed only that expected background-check timestamp changing.
- Only the application's Compose image line changed. PostgreSQL and Redis
  were not recreated. Production has zero opted-in pools and zero pool health
  records: no real credentials, channel cutover or pricing changes were made.
- Backup directory: `/opt/new-api/backups/sensenova-20260906-before-b3f2637`.
  The immediately pre-switch dump is `postgres-pre-switch.dump`, SHA-256
  `4f6097ed10d87aea23cacbffd3e82f60adc793434656534284e3365b6d30552b`.
  Keep this directory and the previous image for rollback. Temporary canary
  application, restored database and network were removed after acceptance.

No real-provider quota, upstream failover or billing calls were made during
deployment. Those require a separately configured key inventory and explicit
provider-cutover acceptance; deterministic failover/billing regressions were
already covered by the implementation's targeted tests.
