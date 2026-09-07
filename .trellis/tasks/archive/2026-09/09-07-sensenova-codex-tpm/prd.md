# SenseNova Codex protocol and TPM reliability

The user approved fixing the concrete failures observed by Codex 0.153.4 on forge against ak47token.com: Responses requests incorrectly reach an unsupported upstream path (404), while repeated large Chat requests exhaust TPM with provider code 429001.

Acceptance:
- Real Codex completes a shell write/read/final-answer round trip for deepseek-v4-pro.
- SenseNova code 429001 is classified as TPM without exposing provider content.
- The authorized channel/key/model pool admits requests using shared, atomic capacity reservations and bounded cancellation-aware waiting.
- Known TPM budgets are configurable; unknown provider limits are not invented. A conservative per-key/model pacing policy protects the default DeepSeek Pro route until an operator supplies a verified limit.
- All-cooling requests can wait within the bounded admission deadline; queue saturation and unavailable state return controlled errors.
- Protocol capability errors do not cool healthy account credentials.
- Existing authorization, pricing group, four-distinct-key retry bound, stream terminal behavior, and exactly-once billing remain intact.
- No customer-content replay, secret persistence, unrelated channel/key/pricing changes or local development runtime.
