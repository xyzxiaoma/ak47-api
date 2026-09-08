# Verification

Status: code regression, independent review, production build and isolated image
smoke passed. Deployment and live capacity acceptance remain separate gates.

On forge using golang:1.25-bookworm, existing module/build caches, GOMAXPROCS=2:

```sh
go test -p 2 ./service ./model ./controller ./relay ./relay/channel/openai ./relay/helper -run 'SenseNova|StreamScannerHandler|TextQuota|TieredBilling' -count=1
go test -race -p 2 ./relay/channel/openai ./controller -run 'TestSenseNova(ChatStream|NonStream|StreamFailure|LatencyChat)' -count=1
go test -race -p 2 ./service -run 'TestSenseNova(Unsent|FollowupConcurrent)' -count=1
```

All passed. Partial-billing real wallet/token and existing settlement tests,
focused races and service vet also passed. No unrelated full suite was run.
Docker production build supplies the root-module build gate; relaykit unchanged.

RED cases reproduced empty/error/truncated success, malformed done markers,
nonstream empty replies, incomplete tools, parallel first-tool loss, mixed-choice
reordering, mixed Claude semantic loss and unsent follow-up pacing/metadata leaks.
Independent reviewers rechecked fixes; no remaining material findings in their
assigned stream/billing/scheduling scopes.

Static checks are not upstream capacity acceptance. Retain the prior isolated
study's failed workflow; key count and public context size cannot establish quota.

Release deployed at 08:11:21 UTC with matching image/source and unchanged runtime
configuration/business invariants. One full-context Write/Read/exact-answer
workflow passed in 378.03s, exit0/is_error=false: seven client requests,22 upstream
attempts,19 rate-limit failures. Three consume records totaling14151 quota;
failures uncharged, temporary identity/token/cache cleaned. Stable one-person
capacity remains unproven and expedited pacing remains disabled. See release
record for complete evidence and operational limits.
