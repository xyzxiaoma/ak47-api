# RelaxyCode channel compatibility and production deployment design

## Problem boundary

This task fixes a probe-path mismatch, not an upstream protocol implementation. Production traffic for the configured GPT models already enters as Chat Completions and is upgraded to OpenAI Responses by the existing global compatibility policy. The channel-test controller currently bypasses that decision and probes Chat Completions directly.

The minimal correct behavior is to make automatic channel endpoint selection honor the same policy and pass-through gates. No RelaxyCode domain, channel ID, credential, or model name belongs in production code.

## Backend design

### Endpoint decision order

`normalizeChannelTestEndpoint` in `controller/channel-test.go` will keep one authoritative precedence order:

1. An explicit `endpoint_type` wins unchanged.
2. A Responses Compact model suffix selects `/v1/responses/compact`.
3. A Codex channel selects `/v1/responses` as today.
4. Otherwise, if global request pass-through is disabled, channel body pass-through is disabled, and `service.ShouldChatCompletionsUseResponsesGlobal(channel.ID, channel.Type, model)` matches, select `/v1/responses`.
5. Otherwise preserve the existing automatic Chat Completions path.

This makes quick tests, model-dialog automatic tests, and scheduled tests probe the endpoint selected by the configured compatibility policy. Manual endpoint testing remains able to deliberately probe unsupported endpoints.

### Request and response flow

For a policy match, the existing controller flow will:

`normalize endpoint` -> `/v1/responses` -> `RelayFormatOpenAIResponses` -> `buildTestRequest(OpenAIResponsesRequest)` -> existing OpenAI adaptor -> existing Responses response handler -> usage validation.

No new relay adaptor or response schema is needed. The change reuses the already-supported Responses path and therefore leaves billing, model mapping, header/parameter overrides, and request authentication in their established code paths.

### Regression coverage

Add deterministic table tests beside existing channel-controller tests. Tests will replace and restore the global model settings without running in parallel, and cover:

- matching policy selects Responses;
- explicit Chat Completions remains explicit;
- unmatched model/channel stays automatic;
- global pass-through blocks implicit conversion;
- channel pass-through blocks implicit conversion;
- Responses Compact and Codex precedence remain unchanged.

No network call, real provider key, production channel record, or provider-specific fixture is used.

## Corresponding-source UI design

### Source URL contract

Add a build-time public variable `VITE_DERIVATIVE_SOURCE_URL`. The Docker build receives the exact public deployment tag URL. The Dockerfile keeps the derivative repository root as a safe fallback, while the production build explicitly overrides it with the exact tag URL.

The existing build-metadata module will expose a small read-only accessor that accepts only an absolute HTTP(S) URL and falls back safely when the variable is absent or malformed. This is stable build metadata reused by the footer and default About surface.

### Visible placement and attribution

- Add a translated `Source Code` link beside the existing legal/project attribution in `ProjectAttribution`, so it remains visible for both fallback and custom-footer layouts.
- Add the same link to the default About empty state. If administrators later replace About content, the footer still preserves the source link.
- Keep the exact notice `Frontend design and development by New API contributors.` and the visible `https://github.com/QuantumNous/new-api` link unchanged.
- Add the `Source Code` key to all seven supported locales through the mandated translation script, then run `bun run i18n:sync`.

No new dependency or bundled asset is introduced, so `THIRD-PARTY-LICENSES.md` does not change.

The repository has no existing changelog or derivative modification ledger. Add a focused `MODIFICATIONS.md` entry dated 2026-08-22 that records this material derivative change and its deployment tag without altering upstream license or attribution files.

## Deployment design

### Artifact identity

1. Commit only task-owned code/tests/artifacts; exclude the pre-existing `AGENTS.md` modification.
2. Push the focused commit to `origin/main`.
3. Create and push a unique deployment tag pointing at that commit.
4. On the server, check out the exact tag under `/opt/new-api/releases/<tag>` and build a local amd64 image tagged with the commit SHA. Pass the exact GitHub tag URL as `VITE_DERIVATIVE_SOURCE_URL`.

The repository workflows publish to the upstream `calciumion/new-api` Docker Hub namespace, so they are not used for this derivative deployment.

### Safe production change

Before mutation:

- capture container/image/port/Nginx/health state;
- back up `/opt/new-api/docker-compose.yml` and the PostgreSQL database into a timestamped backup directory;
- tag the currently running image with a local rollback tag;
- record the current Compose rendering and ensure PostgreSQL/Redis are healthy.

After the new image builds while the old service remains online:

- change only the `new-api` service image reference in the Compose file;
- validate `docker compose config`;
- recreate only `new-api` with `docker compose up -d --no-deps new-api`;
- do not run Compose `down`, remove volumes, prune Docker globally, change Nginx, or restart PostgreSQL/Redis.

### Verification and rollback

Verify separately:

- container health and logs;
- PostgreSQL/Redis health and unchanged Nginx upstream;
- public `https://ak47token.com/api/status` and homepage;
- visible exact-tag `Source Code` link plus preserved upstream attribution;
- production channel `1` automatic test for all four GPT models;
- one bounded real Responses request through the deployed gateway and expected usage/log behavior.

If any required gate fails, restore the backed-up Compose file, point it at the local rollback image tag, recreate only `new-api`, and re-run the public and dependency health checks. The database backup is retained but not restored unless a verified schema/data regression requires it.

## Compatibility and risk notes

- SQLite, MySQL, and PostgreSQL behavior is unchanged; no schema or query changes are made.
- `relaykit/` is unchanged and remains independently buildable.
- The largest code risk is endpoint-precedence drift. The table test makes the order observable.
- The largest operational risk is replacing an unpinned upstream image with the derivative image. Capturing the current image ID and local rollback tag avoids relying on a mutable `latest` tag.
- The supplied provider key was exposed in chat. It is used only for bounded validation and should be rotated after acceptance.
