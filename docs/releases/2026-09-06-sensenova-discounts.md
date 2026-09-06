# SenseNova fixed discounts — 2026-09-06

This New API derivative release preserves licensing and attribution. Version
`ak47token-2026-09-06-sensenova-pool.3` supports the optional comma-separated
environment setting `UPSTREAM_PRICING_SYNC_PRESERVE_DISCOUNT_MODELS`.

Listed exact model IDs retain their current input, output, cache-read and
cache-write selling discounts during scheduled upstream price synchronization.
Original prices continue to synchronize. Unlisted models keep the existing
pricing policy; an unset setting preserves previous behavior. An absent item
override remains absent, preserving the normal input/group fallback.

The operator requested a 0.1 selling ratio (one-tenth of original price) for
`deepseek-v4-pro`, `deepseek-v4-flash`, `glm-5.2` and `kimi-k3`, and explicitly
approved preserving these manual discounts during future syncs and deployment.
No quota limit, provider key or model routing change is included in this release.

Regression coverage checks selected/absent discounts, unaffected other models,
default behavior and continued original-price updates.

## Production verification

- Deployed at 2026-09-06 08:24 UTC from public source tag
  `ak47token-2026-09-06-sensenova-pool.3`, commit
  `d7beb4c24a0a0fa9bc94d3176c658c1c89b3a76a`.
- Image manifest:
  `sha256:e925ddd63ae31e6e7345855410e1420923ae50452775a9dd636dfb38bbba0289`.
- Focused controller pricing conversion/preservation tests passed on `forge`.
  The bounded 3-GiB/2-CPU builder completed frontend and backend production
  builds; bundled license checks passed. The builder was then stopped.
- Backups: `/opt/new-api/backups/sensenova-discounts-20260906/`, including
  Compose and before/after discount maps. Database dump SHA-256:
  `5873e9604b21c4472f305a1dde971b13adf9eb881b3c4e4669f062c2358a4add`;
  `pg_restore -l` validation passed.
- An initial SSH image transfer failed; retry loaded the expected image. Only
  the application image and the four-model preserve-discounts environment value
  changed. PostgreSQL and Redis were not recreated; no channel/key changes.
- The old version's scheduled sync overwrote the initial manual discounts while
  deployment was pending. After the new version was running with the verified
  environment setting, the four maps were reapplied transactionally. All other
  entries were preserved. After normal cache refresh, public `/api/pricing`
  exposed 0.1 for all sixteen model/item combinations.
- Container health, public status/version, pricing, exact-tag source link and
  upstream attribution passed. Old channels 4/5/6 remain disabled; pool 15 stays
  enabled. No live model inference or forced unrelated-price synchronization was
  needed for this pricing-only release.

Rollback retains the preceding `.2` image and backed-up Compose/options. Note
that `.2` does not protect these manual discounts from the scheduled sync.

## Adding pool keys

Open Channels, edit `SenseNova 通用 Key 池` (15), select the key update mode
`Append to existing keys`, enter only the new keys one per line, and save.
Do not select replace unless intentionally replacing the entire inventory.
The existing multi-key manager exposes per-key health and manual testing.

The pool already round-robins eligible keys, cools a rate-limited key in the
observed scope, and performs bounded safe failover. Adding credentials does not
raise any provider-enforced account/model limit. The observed
`ModelAccountTpmRateLimitExceeded` error suggests account/model scope, but no
confirmed public per-key TPM threshold was found. Separate independent quotas
may distribute load; same-account keys must not be assumed independent. Current
scheduling balances requests, not estimated tokens; a single oversized request
or shared upstream capacity limit may still fail across all keys.
