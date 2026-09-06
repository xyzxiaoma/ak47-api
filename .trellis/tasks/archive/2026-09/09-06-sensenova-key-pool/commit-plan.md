# Proposed Work Commit

Awaiting one-shot user confirmation. No commit, push or production deployment
has been performed. All paths below are task-owned; unrecognized dirty paths: none.

## 1. `feat: 添加 SenseNova 多密钥冷却轮转与恢复管理`

One coherent cross-layer feature commit, including its tests and executable spec.

```text
main.go
model/channel.go
model/main.go
model/sensenova.go
model/sensenova_test.go
service/channel.go
service/error.go
service/sensenova.go
service/sensenova_error.go
service/sensenova_error_test.go
service/sensenova_probe.go
service/sensenova_test.go
middleware/distributor.go
controller/channel.go
controller/channel-test.go
controller/channel_authz.go
controller/relay.go
controller/sensenova.go
controller/sensenova_config.go
controller/sensenova_console_test.go
controller/sensenova_relay_test.go
relay/channel/openai/relay-openai.go
relay/helper/stream_scanner.go
web/src/features/channels/api.ts
web/src/features/channels/types.ts
web/src/features/channels/hooks/use-channel-mutate-form.ts
web/src/features/channels/lib/channel-form.ts
web/src/features/channels/lib/multi-key-utils.ts
web/src/features/channels/lib/sensenova-pool.ts
web/src/features/channels/lib/__tests__/sensenova-pool.test.ts
web/src/features/channels/components/channels-columns.tsx
web/src/features/channels/components/__tests__/sensenova-controls.test.tsx
web/src/features/channels/components/dialogs/multi-key-manage-dialog.tsx
web/src/features/channels/components/dialogs/multi-key-table-row-actions.tsx
web/src/features/channels/components/dialogs/sensenova-key-health.tsx
web/src/features/channels/components/drawers/channel-mutate-drawer.tsx
web/src/features/channels/components/drawers/sections/sensenova-pool-field.tsx
web/src/i18n/static-keys.ts
web/src/i18n/locales/en.json
web/src/i18n/locales/fr.json
web/src/i18n/locales/ja.json
web/src/i18n/locales/ru.json
web/src/i18n/locales/vi.json
web/src/i18n/locales/zh-TW.json
web/src/i18n/locales/zh.json
.trellis/spec/backend/index.md
.trellis/spec/backend/sensenova-pool.md
.trellis/tasks/09-06-sensenova-key-pool/prd.md
.trellis/tasks/09-06-sensenova-key-pool/design.md
.trellis/tasks/09-06-sensenova-key-pool/implement.md
.trellis/tasks/09-06-sensenova-key-pool/task.json
.trellis/tasks/09-06-sensenova-key-pool/implement.jsonl
.trellis/tasks/09-06-sensenova-key-pool/check.jsonl
.trellis/tasks/09-06-sensenova-key-pool/research/findings.md
.trellis/tasks/09-06-sensenova-key-pool/commit-plan.md
```

After confirmation, stage only these paths and make the work commit. Trellis
task archival/session bookkeeping may follow separately. Never amend or push
as part of this confirmation.
