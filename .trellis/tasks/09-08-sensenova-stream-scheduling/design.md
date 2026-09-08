# Approved design

Implement the P0/P1 contracts in docs/sensenova-limit-study-20260908.md. Scope
stream changes to opted-in SenseNova Chat conversion; reuse existing Responses
guards, conversion primitives, deadline and distinct-key retry ownership.

Validate stream events before forwarding. Hold role-only/empty prefixes until
semantic data; terminal success requires valid finish semantics and transport
completion. Fail before commitment through ordinary retry; after commitment emit
the client's terminal error and forbid replay. Retain partial usage for explicit
settlement and never mark partial results successful. Other providers keep their
existing behavior. No new relaykit dependency on the root module.

Use exact provider tuples; 429001 alone or mixed tpm/rpm is ambiguous, code 8 plus
quota_exceeded_error plus rpm exhausted is RPM. Version shape evidence to discard
old false TPM penalties without deleting live budget, health or ownership state.

Conversation affinity is a preference, scoped by authenticated user/token/channel
and hashed trusted-or-explicit session identity; absent identity uses ordinary
routing. Never hash/store raw full prompts as identity. Escape disabled, removed,
cooling or budget-ineligible keys. Canary follow-up allowance only after verified
completion, bounded per-key with atomic local request demand, expiry, and
withdrawal on failure. Defaults remain conservative. Account mapping remains
unknown unless explicitly configured; separate keys do not manufacture quota.
