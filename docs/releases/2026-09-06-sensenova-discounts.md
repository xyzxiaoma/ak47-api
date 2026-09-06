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
default behavior and continued original-price updates. Production verification
and backup evidence will be appended after deployment.
