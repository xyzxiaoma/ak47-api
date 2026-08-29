# Quality Guidelines

> Code quality standards for backend development.

---

## Overview

<!--
Document your project's quality standards here.

Questions to answer:
- What patterns are forbidden?
- What linting rules do you enforce?
- What are your testing requirements?
- What code review standards apply?
-->

(To be filled by the team)

---

## Forbidden Patterns

<!-- Patterns that should never be used and why -->

(To be filled by the team)

---

## Required Patterns

<!-- Patterns that must always be used -->

(To be filled by the team)

---

## Testing Requirements

<!-- What level of testing is expected -->

(To be filled by the team)

---

## Code Review Checklist

<!-- What reviewers should check -->

(To be filled by the team)

## Scenario: Channel-Test Responses Routing and Deployment Source Metadata

### 1. Scope / Trigger

- Trigger: A channel test or deployment change crosses controller, relay policy, frontend build, and production container boundaries.
- Scope: Automatic OpenAI-compatible channel tests and the public corresponding-source URL shown by the frontend.

### 2. Signatures

- `normalizeChannelTestEndpoint(channel, modelName, endpointType) string`
- `GET /api/channel/test/:id?model=<model>`
- Docker build argument `VITE_DERIVATIVE_SOURCE_URL`.

### 3. Contracts

- An explicit `endpoint_type` wins over all automatic inference.
- Responses Compact and Codex channel precedence remains unchanged.
- Automatic Chat Completions-to-Responses conversion applies only when global and per-channel pass-through are disabled and `ShouldChatCompletionsUseResponsesGlobal` matches channel and model.
- The frontend accepts only absolute HTTP(S) source URLs and falls back to the public derivative repository.
- Build-time public variables used through dynamic access must also be statically defined in `rsbuild.config.ts`; otherwise production bundles may contain only the fallback.

### 4. Validation & Error Matrix

| Condition | Required behavior |
| --- | --- |
| Explicit endpoint present | Return it unchanged |
| Matching policy with pass-through disabled | Select Responses endpoint |
| Global or channel pass-through enabled | Preserve automatic endpoint behavior |
| Invalid or absent source URL | Use derivative repository fallback |
| Valid deployment source URL | Render the exact public tag URL |

### 5. Good / Base / Bad Cases

- Good: A matching policy test records `request_path=/v1/responses` and returns structured JSON.
- Base: A non-matching model continues through existing Chat Completions behavior.
- Bad: A channel test calls `/v1/chat/completions` against an upstream that only serves `/v1/responses`, producing an HTML `bad_response_body` parse failure.

### 6. Tests Required

- Controller table tests must cover explicit endpoint precedence, policy match, model/channel mismatch, both pass-through gates, Compact suffix, and Codex channel.
- Frontend typecheck and production build must pass with a unique tag URL; inspect the bundle or browser DOM for the exact URL, protected attribution, and original-project link.
- Production acceptance must keep container health, `/api/status`, Nginx syntax, and channel-test results as separate evidence.

### 7. Wrong vs Correct

#### Wrong

```typescript
const raw = import.meta.env?.[name]
```

Dynamic property access can survive bundling without the custom public value.

#### Correct

```typescript
source: {
  define: {
    'process.env.VITE_DERIVATIVE_SOURCE_URL': JSON.stringify(derivativeSourceUrl),
  },
}
```

Define the build-time value statically and keep URL validation in the shared accessor.

## Scenario: Scheduled Upstream Model Pricing

### Contracts

- Preserve upstream `original` numeric values without CNY/USD conversion.
- For time-of-day DeepSeek prices, use the top-level peak `original` and ignore `off_peak`.
- For GLM, Kimi, DeepSeek, and Qwen, resolve input, output, cache-read, and cache-write selling ratios independently as `min(platform / original + 0.1, 1)`.
- GPT synchronization updates original prices only and must preserve its existing selling discount configuration.
- Public/free models (including `free/ds-v4-flash`) and tiered models are excluded from the flat-price sync.
- Persist original-price and four model-level selling-ratio option maps in one database transaction. Missing item overrides fall back to the model input override and then to the selected group ratio.
- Text, realtime audio, channel-test settlement, consume logs, `/api/pricing`, and frontend original/discounted projections must share these ratio semantics.

### Tests Required

- Peak-vs-off-peak conversion and independent cache discounts.
- GPT discount preservation and public/free exclusion.
- Transaction rollback for bulk pricing-option persistence.
- Model/item/group fallback behavior and representative text/channel-test settlement.
- Frontend typecheck and production build, with base-price views verified to exclude all model-level selling discounts.
