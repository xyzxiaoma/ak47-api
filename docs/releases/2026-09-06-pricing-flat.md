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
Production build and release verification are recorded below.

## Production verification

- Deployed at 2026-09-06 08:53 UTC from public source tag
  `ak47token-2026-09-06-pricing-flat.1`, commit
  `65e92ad7eb11889c7d2654cdb14cfbd5c273fbfd`.
- Image manifest:
  `sha256:03ff5e2137de4caa6c6f1dbb012783ad424c573cbb00ea46f15ebc0b024a6615`.
- Direct SSH to `forge` was unavailable. An existing SSH jump route through
  production restored access; the build still ran on `forge`, with no private
  key copying or agent forwarding. The isolated builder was limited to 3 GiB
  and two CPUs and stopped after successful frontend/backend production builds.
- Image version, source revision and bundled licenses passed before transfer.
  The loaded production image matched the build image. Compose changed only the
  application image; PostgreSQL and Redis retained their existing start times.
- Backups: `/opt/new-api/backups/pricing-flat-20260906/`, including Compose and
  a validated PostgreSQL dump with SHA-256
  `c36eca7ed1fc09ccb17f86b1940af142ca62dd45845a5653981904d2bc853285`.
- Public version, pricing, exact-tag source link, upstream attribution and
  application health passed. All sixteen SenseNova model/item selling ratios
  remained 0.1; the preserve-discounts environment setting remained intact.
- Browser verification used an existing bounded Playwright container on
  `forge` because its host browser lacked a shared library. Desktop (1440 px)
  and mobile (390 px) each exposed all 17 separate model cards, with no hidden
  cards, overlapping card boxes, horizontal page overflow or page errors.

Rollback retains the preceding `sensenova-pool.3` image and backed-up Compose.
No key, channel, schema or pricing mutations were part of this UI deployment.
