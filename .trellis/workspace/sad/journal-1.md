# Journal - sad (Part 1)

> AI development session journal
> Started: 2026-08-09

---


## Session 1: RelaxyCode channel adapter deployment

**Date**: 2026-08-22
**Task**: RelaxyCode channel adapter deployment
**Branch**: `main`

### Summary

Adapted automatic channel tests to Chat Completions-to-Responses policy with pass-through gates; added exact AGPL corresponding-source links and static build metadata; deployed tag ak47token-2026-08-22-relaxycode-adapter.2 with backups, health checks, public browser acceptance, and production channel test success.

### Git Commits

| Hash | Message |
|------|---------|
| `5d8e3571` | (see git log) |
| `6fc0487d` | (see git log) |
| `f312cadd` | (see git log) |

### Status

[OK] **Completed**


## Session 2: 前端控制室视觉重设计并部署

**Date**: 2026-08-22
**Task**: 前端控制室视觉重设计并部署
**Branch**: `main`

### Summary

完成断层控制室与编辑式公开首页视觉重设计，加入本地 Iconify 与暖色材质 Token；提交并推送 main，部署标签 ak47token-2026-08-22-frontend-control-room.1 到 RainYun。现网 new-api、PostgreSQL、Redis 健康，Nginx 配置通过，公开首页与 About 页源码链接和 New API 归属验收通过。

### Git Commits

| Hash | Message |
|------|---------|
| `674d6dcc` | (see git log) |
| `c304ab95` | (see git log) |

### Status

[OK] **Completed**


## Session 3: SenseNova 密钥池实现与正式发布

**Date**: 2026-09-06
**Task**: SenseNova 密钥池实现与正式发布
**Branch**: `codex/sensenova-key-pool`

### Summary

完成独立账户 SenseNova 多密钥池、状态冷却与恢复探测、原生管理界面和定向回归验证。forge 恢复后用受限 BuildKit 构建精确公开标签，隔离恢复 PostgreSQL 并通过迁移及管理 API 验收，仅替换正式 new-api 应用容器。线上版本、健康、公开价格、源码署名及业务配置摘要通过；未导入真实 Key、修改价格或切换旧渠道。保留旧镜像和备份，移除临时 canary，停止等待提醒。详见发布记录。

### Git Commits

| Hash | Message |
|------|---------|
| `84759ef` | (see git log) |
| `b3f2637` | (see git log) |
| `612e3b7` | (see git log) |

### Status

[OK] **Completed**


## Session 4: SenseNova activation and GLM probe compatibility

**Date**: 2026-09-06
**Task**: SenseNova activation and GLM probe compatibility
**Branch**: `codex/sensenova-key-pool`

### Summary

Published and deployed pool.2; activated supplied single key in pool 15, disabled old channels 4/5/6, preserved prices and unrelated providers. GLM and both DeepSeek models passed real gateway; Kimi intermittent upstream rate limit handled by recovery. Backups and rollback retained; temporary acceptance resources cleaned.

### Git Commits

| Hash | Message |
|------|---------|
| `59353ed` | (see git log) |

### Status

[OK] **Completed**


## Session 5: Fixed SenseNova discounts and key-pool operation

**Date**: 2026-09-06
**Task**: Fixed SenseNova discounts and key-pool operation
**Branch**: `codex/sensenova-key-pool`

### Summary

Deployed pool.3 with per-model discount preservation in scheduled pricing sync. Set the four SenseNova models to 0.1 for input/output/cache-read/cache-write and verified public pricing after cache refresh. Preserved other prices, channels and keys. Documented append-key workflow and round-robin TPM boundaries.

### Git Commits

| Hash | Message |
|------|---------|
| `d7beb4c` | (see git log) |

### Status

[OK] **Completed**


## Session 6: Flat model marketplace deployment

**Date**: 2026-09-06
**Task**: Flat model marketplace deployment
**Branch**: `codex/sensenova-key-pool`

### Summary

Replaced stacked model cards with a responsive flat grid and deployed pricing-flat.1. Targeted regression tests, typecheck, lint and production builds passed on forge. Production status, pricing and attribution passed; desktop/mobile browser checks displayed all 17 separate cards without overlap or overflow. Preserved SenseNova keys, channels and one-tenth pricing; backups and preceding image retained.

### Git Commits

| Hash | Message |
|------|---------|
| `65e92ad` | (see git log) |

### Status

[OK] **Completed**


## Session 7: SenseNova 429 reliability deployment

**Date**: 2026-09-07
**Task**: SenseNova 429 reliability deployment
**Branch**: `codex/sensenova-key-pool`

### Summary

Deployed four-key bounded failover, bounded Retry-After and safe admin diagnostics. Targeted regressions and remote builds passed; production healthy with unchanged channel configuration and all 16 discounts. Historical provider rejection subtype remains unproven.

### Git Commits

| Hash | Message |
|------|---------|
| `3cef8f4` | (see git log) |
| `f95a6bb` | (see git log) |

### Status

[OK] **Completed**


## Session 8: SenseNova Codex compatibility and TPM deployment

**Date**: 2026-09-07
**Task**: SenseNova Codex compatibility and TPM deployment
**Branch**: `codex/sensenova-key-pool`

### Summary

Deployed Responses-to-Chat compatibility, namespace/custom tool replay, developer-role mapping and bounded Redis TPM admission. Exact 429001 classification and review fixes preserve health, cancellation and measured usage accounting. Public Codex completed three HTTP200 requests and exact file/answer checks in 108.58 seconds; upstream TPM limits remain and cause bounded waiting.

### Git Commits

| Hash | Message |
|------|---------|
| `14152e0` | (see git log) |
| `ad7ead4` | (see git log) |

### Testing

- [OK] Focused controller/service/model/middleware/relay protocol, budget, cancellation and billing regressions passed on forge; relaykit independently built.
- [OK] Isolated real Codex 84.6s and custom-tool replay 5.88s passed; public production Codex 108.58s passed.
- [OK] Exact source/version/license/attribution and unchanged channel/pricing checksums verified; PostgreSQL and Redis unchanged.

### Status

[OK] **Completed**

### Next Steps

- Confirm supplier account TPM before configuring positive token limits; keep Codex web_search disabled for this provider.


## Session 9: SenseNova latency diagnostics and verified Claude acceptance

**Date**: 2026-09-07
**Task**: SenseNova latency diagnostics and verified Claude acceptance
**Branch**: `codex/sensenova-key-pool`

### Summary

Deployed latency diagnostics and sufficient-capacity wake correction. Official quota values remain unverified; conservative pacing retained. Targeted tests and release build passed. Public Claude completed exact Write/Read and final answer in 223.26 seconds after two 429 responses; cleanup verified after forge connectivity recovered.

### Git Commits

| Hash | Message |
|------|---------|
| `6f9f0bf` | (see git log) |
| `427477b` | (see git log) |
| `3d1545b` | (see git log) |

### Status

[OK] **Completed**


## Session 10: Claude accounting and real-request recovery release

**Date**: 2026-09-07
**Task**: Claude accounting and real-request recovery release

### Summary

Fixed JSON-decoded Claude tool estimates and leased SenseNova real-traffic recovery; bounded renewal/cleanup SQL. Preserved failure history while separating transient rate cooldown from progressive health backoff. Focused tests, race checks, independent reviews and production build passed on forge. Deployed recovery.2 from public source tag; full Claude Code 25-tool/32000/adaptive Write-Read-final workflow passed with three HTTP 200 responses in 79.42s. Retained failed recovery.1 evidence and documented 43.412s capacity wait; no stable latency SLA claimed. Verified config, data services, attribution, licenses and temporary credential cleanup.

### Git Commits

| Hash | Message |
|------|---------|
| `190029f` | (see git log) |
| `23f55b3` | (see git log) |
| `6bb3140` | (see git log) |

### Status

[OK] **Completed**
