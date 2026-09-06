# Evidence collected on 2026-09-06

## Scope and provenance

The user identified `https://github.com/xyzxiaoma/ak47-api.git` as the formal-site source. An SSH clone succeeded into `/Users/ma/Developer/personal/ak47-api`; the default branch tip was `7509625` (pricing fix). This is separate from the existing `ak47token` working tree, which contains unrelated uncommitted work.

Read-only production inspection found container `new-api` using `new-api:ak47token-2026-08-30-pricing-items.1` under `/opt/new-api`. SHA-256 values for `model/channel.go` and `controller/relay.go` match the local checkout and `/opt/new-api/releases/75096257-pricing-items`. This corroborates those files only; complete deployed-source provenance still needs verification before release.

The referenced Codex task `01a070a7-fd4c-7882-867d-fa849e2ae711` was read. Its context established that formal-site pricing/configuration is separate from the replacement app. No credentials from that task belong in this repository.

## Existing production domestic channels

| ID | Name | Group | Models |
| --- | --- | --- | --- |
| 4 | 国模-GLM | glm | glm-5.2-fast-preview, glm-5.2 |
| 5 | 国模-Kimi | kimi | kimi-k3, kimi-k2.7-code |
| 6 | 国模-DeepSeek | deepseek | deepseek-v4-flash, deepseek-v4-flash-0731, deepseek-v4-pro |

All three used `https://c-api.csid.cc` and were enabled at inspection. Other GPT/Grok/Claude channels exist and are not implicitly part of this replacement. No production configuration has been changed by this task.

## Provider evidence

- `GET https://token.sensenova.cn/v1/models` accepted the supplied credential and returned the selected four model IDs among eight advertised models.
- One tiny DeepSeek Flash completion returned a valid `chat.completion`; it exhausted its small output budget in reasoning. This validates only that request, not every model/stream/tool path.
- Public console code identifies `GET https://platform.sensenova.cn/lite/console/v1/tokenplan/pool-usage` as the quota source and uses the website access token.
- A request to that exact quota endpoint with the API key returned HTTP 401, `error_key=auth_type_disabled`, reason `Authentication type 'apikey' is not enabled`.
- The authenticated console showed shared general and Flash-Lite-exclusive pools. The user confirmed the remaining keys belong to independent accounts and that no login credentials are available.
- Public documentation at `https://platform.sensenova.cn/docs` describes rolling 5-hour/week limits and `429 quota_exceeded_error` for rate/quota excess. A generic documented code cannot distinguish those cases or establish a reset time.
- No actual exhaustion response has been captured. Do not exhaust an account merely to collect one.

## Source-backed reuse and gaps

- `model/channel.go:199` selects enabled keys using random/polling modes and rejects an all-disabled key list.
- `model/channel.go:647` locates the used credential and records its status/reason/time. It uses list-index status maps, so new persistent health records require stable identity or careful remapping.
- `controller/relay.go:239` supplies the attempt's key to `processChannelError`; `controller/relay.go:367` dispatches disabling asynchronously. A synchronous request-local exclusion is needed before the next attempt.
- `controller/channel.go:1462` and `:1473` define existing per-key actions and paginated states/counters.
- `controller/channel.go:1007` supports appending/replacing keys; deletion remaps index-based metadata later in the same file.
- `service/channel.go:45` uses generic status/keyword disabling. It is not a provider-specific temporary quota lifecycle.
- `controller/channel-test.go:857` and subsequent batch-test logic operate on channel test selection; due-key recovery cannot assume this selects a cooling key.
- `service/system_task_test.go:65` contains scheduler/dedup behavior that may be reusable for recovery jobs.
- `web/src/features/channels/components/dialogs/multi-key-manage-dialog.tsx` already supports status filtering, pagination, counts and per-key actions.

## Conventions

Root `AGENTS.md` is the substantive source for JSON wrappers, GORM compatibility, billing safety, tests, attribution and deployment-source obligations. Backend spec pages inspected are placeholders, not evidence of implemented conventions. Frontend additions must follow `web/AGENTS.md`, use existing components/i18n, and include meaningful interaction tests.

## Creation authorization

The user approved task creation with “可以，创建吧”. The task remains `planning`; no implementation activation, code changes, deployment, key ingestion, or active monitoring has occurred.
