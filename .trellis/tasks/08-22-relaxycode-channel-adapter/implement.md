# RelaxyCode channel compatibility and production deployment implementation plan

## 1. Activate and load implementation context

- Run the Phase 1.4 review gate and `task.py start` only after the user explicitly approves the final planning summary.
- Load `trellis-before-dev` for backend and frontend conventions.
- Re-read `web/AGENTS.md`; load `shadcn-ui` only if the existing footer composition requires project UI guidance.
- Preserve the user-owned `AGENTS.md` modification throughout.

## 2. Backend policy-aligned channel test

- Update `controller/channel-test.go` to extend automatic endpoint normalization with the existing Chat Completions-to-Responses policy and both pass-through gates.
- Keep explicit endpoint, Responses Compact, and Codex precedence unchanged.
- Add controller regression cases using `testify/require` and `testify/assert`; restore singleton settings with `t.Cleanup` and avoid parallel execution.
- Format the touched Go files.

Focused validation:

```powershell
go test ./controller -run 'TestNormalizeChannelTestEndpoint' -count=1
go test ./service -run 'Test.*Chat.*Responses' -count=1
```

## 3. Exact corresponding-source link

- Add a validated derivative source URL accessor to the existing build-metadata module.
- Update the shared project-attribution footer and default About state to render the link without changing the protected New API notice or original-project URL.
- Add the Docker build argument/public variable and retain a derivative repository-root fallback.
- Add the `Source Code` key for `en`, `zh`, `zh-TW`, `fr`, `ja`, `ru`, and `vi` only via `web/scripts/add-missing-keys.mjs`; run the script and `bun run i18n:sync`, then remove temporary scripts.

Frontend validation from `web/`:

```powershell
bun run i18n:sync
bun run typecheck
bun run lint
bun run format:check
bun run copyright:check
$env:VITE_DERIVATIVE_SOURCE_URL='https://github.com/xyzxiaoma/ak47-api/tree/test-source-ref'; bun run build
```

Inspect the built output to confirm the configured source URL, exact attribution text, and original-project URL are present.

## 4. Integrated quality gate

- Run `git diff --check`, inspect every changed file, and verify no provider key, credential, `.env`, production dump, or unrelated `AGENTS.md` change is staged.
- Run the focused tests again after formatting, then a proportionate package/full backend gate:

```powershell
go test ./controller ./service -count=1
go test ./... -count=1
go build ./...
```

- Run the `trellis-check` skill and resolve verified findings.
- Confirm `relaykit/` was not changed; if its public API becomes affected unexpectedly, additionally run `cd relaykit; $env:GOWORK='off'; go build ./...`.

## 5. Commit, push, and public revision

- Compare git identity with repository history as required by project governance; do not change git config.
- Stage only the intended controller test/fix, source-link frontend/Docker/i18n files, durable modification record, and this Trellis task. Exclude `AGENTS.md`.
- Add or update `MODIFICATIONS.md` with a dated description of this material derivative change and its deployment tag; do not alter or remove upstream license, notice, or attribution files.
- Commit the focused change and push `main` to `origin`.
- Verify the public commit URL is reachable, create a unique deployment tag at that commit, push the tag, and verify the exact tag tree is public.
- Treat local checks, remote CI, and public source reachability as separate evidence. If CI fails, fix and repeat before deployment.

## 6. Production preflight and backup

- Reconfirm `rainyun-rcs` identity, host resources, running containers, ports, Nginx upstream, Compose services, image IDs, mounts, and relevant free space.
- Create a timestamped directory inside `/opt/new-api/backups/` after resolving and verifying that exact path.
- Copy the current Compose file and sanitized Compose rendering there; make a PostgreSQL custom-format dump without printing credentials.
- Apply a unique local rollback tag to the current `new-api` image and record its image ID.
- Verify the backup file exists and is non-empty before changing Compose.

## 7. Build and deploy the exact tag

- Check out the public deployment tag into `/opt/new-api/releases/<tag>`.
- Build an amd64 image with a commit-derived local tag and `VITE_DERIVATIVE_SOURCE_URL` set to the exact public tag URL. Keep the current container running during the build.
- Validate the resulting image labels/files/licenses and run a bounded container smoke check if practical.
- Back up again immediately before the edit, change only the `new-api` image reference, and run `docker compose config`.
- Recreate only `new-api` with `docker compose up -d --no-deps new-api`; wait for the Docker health check within a bounded window.

## 8. Production acceptance

- Verify `docker ps`, new-api startup logs, PostgreSQL, Redis, port `127.0.0.1:3000`, Nginx config test, and unchanged public TLS routing.
- Verify `/api/status`, homepage, footer, and default About through public HTTPS. Confirm the source link resolves to the exact tag and the protected attribution/original-project link remain visible.
- Use the existing authenticated admin session or production API to run the automatic channel test for channel `1` and each configured GPT model. Confirm the prior HTML `bad_response_body` failure is absent.
- Make one bounded real gateway Responses request using an authorized credential, validate a structured response and usage/log record, and do not disclose credentials or response content.
- Record automated checks separately from browser/visual acceptance; perform an actual browser inspection of the footer/about placement.

## 9. Rollback on failure

- Restore the backed-up Compose file or change the service image to the local rollback tag.
- Validate Compose and recreate only `new-api`; do not stop or replace PostgreSQL/Redis.
- Re-run container, dependency, Nginx, public HTTPS, source-link, and channel availability checks.
- Restore the database only if a concrete data/schema regression is proven; this code change has no expected migration.

## 10. Finish

- Update task acceptance evidence and project spec only if the implementation reveals a durable convention not already captured.
- Report commit SHA, tag, public source URL, image tag/ID, backup location, local checks, CI result, deployment health, channel/model tests, browser acceptance, and any remaining key-rotation action.
- Run `trellis-finish-work` after the quality gate, commit/push, and deployment are genuinely complete.
