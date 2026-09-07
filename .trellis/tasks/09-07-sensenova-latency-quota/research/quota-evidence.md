# SenseNova quota evidence — 2026-09-07

## Scope and decision

Target: the existing opted-in channel 15 at `https://token.sensenova.cn`,
model `deepseek-v4-pro`. The operator confirmed in this session that no supplier
quota statement is currently available and approved proceeding with existing
evidence. Do not infer numeric quotas or shared-account relationships from key
count, healthy probes or successful requests.

## Evidence

1. User-supplied official platform documentation, retrieved 2026-09-07:
   https://platform.sensenova.cn/docs#points . The current published document
   describes new points rules effective 2026-08-28. General points can be used
   across open models; a separate Flash-Lite pool is restricted to that family.
   Each points pool has rolling five-hour and weekly allowances (60,000 and
   600,000 points during the public beta). Points are not tokens and are not
   TPM/RPM. The account page provides actual balance/use/deductions. This newer
   document takes precedence over the older request-count FAQ below.
2. Official console has a usage-limit table:
   https://platform.sensenova.cn/console/usage-limits . Its published page code
   renders model ID/name, modality, TPM and RPM; values are fetched dynamically
   from `GET /lite/console/v1/metered/models` on the same official origin.
   An unauthenticated read returned HTTP 401 (`auth_header_missing`). This is
   an authenticated console endpoint, not a documented inference-key quota API.
   The navigation associates this page with metered usage/API keys, so its
   eventual numbers must also be checked for applicability to Token Plan keys.
   No authentication boundary was bypassed and no inference key was sent there.
   Current public docs do not document numeric target-model TPM/RPM/concurrency,
   rate-limit response headers or an inference-key balance endpoint.
3. Older official SenseNova Skills FAQ, retrieved 2026-09-07:
   https://github.com/OpenSenseNova/SenseNova-Skills/blob/main/docs/faq_CN.md
   Its rate-limit section still describes a free trial request allowance of 1,500
   requests per five-hour rolling window and peak-load queueing. It does not
   specify the target model's TPM, RPM, concurrency or the mapping of these four
   credentials to quota accounts. A five-hour request allowance is not a TPM
   budget and must not be divided into a fabricated provider RPM limit. A
   similarly worded older plan string in the current public translation bundle
   says same-account keys share the per-model request allowance, but it is not
   proof that this older allowance remains active after the points update.
4. Official public landing page, retrieved 2026-09-07:
   https://www.sensenova.cn/ . The retrieved public content provides no target
   account/model quota table. The API base URL root returns HTTP 404; it is not
   evidence of invalid inference credentials. No authenticated supplier quota
   view was available through the connected browser.
5. Previous deployment evidence in
   `docs/releases/2026-09-07-sensenova-codex-tpm.md` records exact upstream
   `429001` (`inference tpm exhausted`) on a large request. This identifies a
   TPM rejection but supplies neither a numeric threshold nor reset time.
   Older Kimi observations (`ModelAccountTpmRateLimitExceeded`) concern a
   different model and cannot establish DeepSeek quota scope.
6. Current production container environment was inspected using an exact
   non-secret allowlist: no `SENSENOVA_TPM_LIMITS`, admission or unknown-TPM
   overrides were configured. Code defaults therefore apply: unknown budget,
   one in-flight request and at least 60 seconds between starts per key/model,
   75-second request-wide wait deadline, 32 waiting requests per channel.
7. The prior real Claude Code 2.1.263 test completed three streaming model turns
   after an initial 429, in 162.41 seconds. Inputs were 17,459 / 17,635 / 17,748
   tokens and requested output ceiling 32,000; later turns reported 16,384 cache
   hits. These observations do not reveal whether reserved output ceilings,
   cached inputs or traffic outside this gateway count toward provider TPM.
8. A read-only production audit during this task confirmed four configured keys
   and an enabled pool. In the preceding two hours, DeepSeek Pro had 24 internal
   error logs classified `tpm`, eight `unknown`, and nine consume logs. This is
   a count of attempts/logs, not an end-user failure rate or capacity estimate.

The official documentation was retrieved from its public HTML and referenced
JavaScript content bundle because browser navigation and search extraction
timed out. Static page source identifies the usage-limit data endpoint; only
its unauthenticated response was checked. No private console data was obtained.

## Unresolved provider facts

| Fact | Status |
| --- | --- |
| DeepSeek V4 Pro TPM | Unknown |
| DeepSeek V4 Pro RPM / short burst limits | Unknown |
| Maximum simultaneous requests | Unknown |
| Four credentials' account/quota relationship | Not independently verified |
| Account points across non-Flash-Lite models | Shared general points per current docs |
| Cross-model/shared inference capacity | Unknown |
| Cached input / output-ceiling accounting | Unknown |
| Provider window/reset mechanics | Unknown |

No quota was increased, no same-account keys were treated as new capacity, and
no saturation test or supplier contact was performed. Conservative pacing
remains until the operator can supply account/model quota evidence.

## Safe scheduler correction

For already configured known local TPM policies, the previous Redis wait hint
used the oldest debit expiry even if that expiry would free too few tokens.
For a 100-token local budget with debits 20 at t=0, 60 at t=10 and 20 at t=20,
an 80-token request at t=20 needs to wait 50 seconds, not 40. Compute the first
expiry that frees enough capacity, retaining atomic reservation and lease
checks. This improves wait-hint correctness; it does not justify reducing the
unknown-TPM safety interval used by the production channel.
