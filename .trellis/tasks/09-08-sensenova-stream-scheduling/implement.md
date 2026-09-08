# SenseNova repair implementation plan

Goal: truthful stream completion and bounded conversation scheduling.
Spec: prd.md, design.md and docs/sensenova-limit-study-20260908.md.
Architecture: provider-scoped validation, existing billing/retry boundaries,
Redis atomic pacing and existing health generation checks. Go/Gin/GORM/Redis.

- [x] Classification: add observed error fixtures in service/sensenova_error_test.go;
  run focused tests RED, implement exact tuples in sensenova_error.go, version
  shape keys in sensenova_capacity.go, and verify ambiguous errors do not publish
  TPM penalties. Preserve generic 429 model cooling and safe outward errors.
- [x] Stream: add empty/malformed/embedded/truncated cases for OpenAI and Claude,
  valid text/thinking/tool terminal controls, and before/after commitment tests.
  Modify relay/channel/openai provider-specific handling and helper commitment
  only as necessary. Trace relay text/Claude settlement and controller retry.
  Require failed attempts not to report successful health or consume empty output.
- [x] Scheduling: tests for same-session preference, tenant separation, removal,
  unavailable-key escape and concurrent allowance ownership. Implement bounded
  optional policy in service/sensenova_{admission,budget,capacity_routing}.go and
  new affinity helper; ensure every dispatched attempt consumes request demand.
- [x] Integrate on forge, run affected SenseNova service/model/controller/relay
  tests and relevant billing/retry tests; focused race checks for new shared state.
  Build root module; if relaykit changes, GOWORK=off go build ./... there too.
- [x] Independent Trellis review, resolve findings, update specs and operational
  settings. Live synthetic canary only with available authorized credentials;
  distinguish technical regression success from upstream capacity acceptance.
- [x] Record final changes, validation and limitations; apply repository commit
  and release workflow only after concrete checks. Never claim guaranteed SLA.

Development remains in this isolated local worktree; forge workspace is
/root/codex-workspaces/ak47-api-capacity-20260908. Sync owned files without delete.
Use docker golang:1.25-bookworm, existing /root/go-cache/{pkg,build}, GOMAXPROCS=2
and go test -p 2. No local runtime installs or production key/config changes.
