# RelaxyCode channel compatibility and production deployment

## Goal

Make the production RelaxyCode-backed GPT channel pass the same connectivity test path that its real traffic uses, without changing unrelated channels, then deploy the verified fix to `ak47token.com` with a recoverable rollback.

## Background and confirmed facts

- Production channel `1` (`GPT供应`) is an OpenAI-compatible channel with base URL `https://www.relaxycode.com`, models `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`, and `gpt-5.5`, and test model `gpt-5.5`.
- Production enables `global.chat_completions_to_responses_policy` only for channel `1` and those four GPT models. Real `/v1/chat/completions` traffic is therefore converted to upstream `/v1/responses` by `relay/compatible_handler.go`.
- `controller/channel-test.go` bypasses that compatibility policy when no endpoint is selected. It defaults `gpt-5.5` to `/v1/chat/completions`, causing the row action and scheduled test to probe a different endpoint from real traffic.
- Direct, authorized upstream probes on 2026-08-22 returned HTTP 200 HTML for `/v1/models` and `/v1/chat/completions`, while `/v1/responses` returned an OpenAI Responses JSON payload. This reproduces the screenshot's `invalid character '<' ... (bad_response_body)` failure without exposing or persisting the supplied key.
- Production is the shared RainYun host reached as `rainyun-rcs` (`154.9.24.195`). `/opt/new-api/docker-compose.yml` currently runs `calciumion/new-api:latest` with separate healthy PostgreSQL and Redis containers; Nginx proxies `ak47token.com` to `127.0.0.1:3000`.
- The default frontend preserves the required New API attribution and original-project link, but neither the repository nor production database config currently provides a visible link to the derivative source repository. A new commercial deployment cannot proceed until users can reach the complete corresponding source for the deployed revision.
- The working tree already contains a user-owned `AGENTS.md` modification. It must remain untouched and excluded from this task's commit.

## Requirements

- R1: When a channel test has no explicit endpoint and the configured Chat Completions-to-Responses policy matches the channel and model, select the OpenAI Responses test endpoint.
- R2: Preserve an explicitly selected test endpoint even when the compatibility policy matches.
- R3: Preserve existing Codex and Responses Compact automatic endpoint behavior.
- R4: Mirror the production pass-through gates: do not auto-select Responses when global request pass-through or the channel's body pass-through is enabled.
- R5: Add deterministic backend regression tests around endpoint selection; do not add provider credentials or network-dependent tests to the repository.
- R6: Keep the change provider-neutral. RelaxyCode is the reproducer, but the behavior should follow the existing compatibility policy instead of hard-coding a domain, channel ID, or model name.
- R7: Before production mutation, back up the affected Compose configuration and database, record the current image/container/health state, and preserve PostgreSQL, Redis, Nginx, volumes, `.env`, and unrelated host services.
- R8: Deploy a reproducible build of the reviewed fork revision, verify container health, public HTTPS, the channel test, and a real Responses request, and retain a rollback path to the previous image/configuration.
- R9: Never persist, print, commit, or copy the supplied RelaxyCode key into task artifacts, source, deployment files, logs, or tests. The user should rotate it after validation because it was shared in chat.
- R10: Before deployment, add a clear derivative `Source Code` link to the public footer/about surface that resolves to the exact deployed tag or commit, while retaining the required notice `Frontend design and development by New API contributors.` and the visible original-project link.

## Acceptance Criteria

- [ ] AC1: A policy-matched OpenAI channel/model with automatic endpoint selection builds and sends an OpenAI Responses test request.
- [ ] AC2: The same channel/model with an explicit OpenAI Chat Completions endpoint remains on that explicit endpoint.
- [ ] AC3: A non-matching model/channel remains on the current automatic Chat Completions path.
- [ ] AC4: Global or channel pass-through prevents implicit Chat-to-Responses test conversion.
- [ ] AC5: Existing Codex and Responses Compact endpoint tests continue to pass.
- [ ] AC6: Focused controller/service tests and the proportionate backend quality gate pass locally; no secret appears in `git diff` or tracked files.
- [ ] AC7: On production, the GPT channel quick test succeeds without `bad_response_body`, and the four configured GPT models can be checked through the Responses endpoint.
- [ ] AC8: The deployed `new-api` container is healthy, `https://ak47token.com` and its API health surface remain reachable, PostgreSQL/Redis stay healthy, and Nginx routing is unchanged.
- [ ] AC9: The exact deployed source revision is available from the public derivative repository and the live UI's required source/original-project attribution remains intact.

## Key decisions

- The user approved creating a focused commit, pushing it to `origin/main`, and deploying that exact public revision.
- The channel behavior fix remains policy-driven and provider-neutral; no RelaxyCode channel type or domain special case will be added.
- The user approved the minimum commercial-deployment compliance change: add a visible derivative `Source Code` link while retaining the exact New API attribution and original-project link, and publish an exact deployment tag for that link.

## Out of scope

- Adding a hard-coded `RelaxyCode` channel type, provider logo, or domain-specific model registry.
- Making RelaxyCode's unsupported `/v1/models` or `/v1/chat/completions` routes work upstream.
- Changing billing, prices, model mappings, user balances, the disabled Claude channel, or unrelated server services.
- Storing the supplied key as a new production credential; production channel `1` already has its own configured key.
