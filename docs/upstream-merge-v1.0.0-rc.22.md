# Upstream Merge Audit: v1.0.0-rc.22

## Baseline

- Local branch baseline: `main` at `6a978443ec3a91c945b80c11d7be2487d314ff93`.
- Previous upstream release baseline: `v1.0.0-rc.21` at `bde9b2f44887d34ec54799ae191d50f97914359e`.
- Merge target: signed tag `v1.0.0-rc.22` at `bc14c18f6024e79cba1c08d02cd007796e12d668`.
- Unreleased commits after `v1.0.0-rc.22` are explicitly out of scope.
- Local-only history contains 93 non-merge commits and 115 commits including merges.
- Dry merge census: 63 conflict events: 23 content, 1 add/add, 2 directory rename split, 16 file location, and 21 modify/delete conflicts.

## Preservation Rules

1. A local-only file may be deleted only when it belongs exclusively to the retired Classic frontend or its behavior is mapped to a reviewed replacement.
2. Whole-file `ours` or `theirs` conflict resolution is prohibited. Resolution must preserve both upstream contracts and local business behavior.
3. Auto-merged files changed by both sides require the same semantic review as explicit conflicts.
4. Every protected feature must retain an implementation path and automated coverage before the merge is considered complete.
5. `web/default` local additions must move to `web`; no local additions may remain stranded under the retired directory.
6. The production deployment repository is outside this merge. Secure cookie and trusted-proxy settings remain deployment prerequisites.

## Stage Status

| Stage | Deliverable | Status |
| --- | --- | --- |
| 0 | Preservation inventory and audit baseline | Complete |
| 1 | Two-parent merge, frontend flattening, complete local-code migration | Complete |
| 2 | Backend semantic audit and regression coverage | Complete |
| 3 | Frontend semantic audit and regression coverage | Complete |
| 4 | Full validation, encoding audit, Docker build | Pending |
| Delivery | Fast-forward `main`, push, and successful multi-arch Actions run | Pending |

## Protected Feature Matrix

| Area | Local commits | Required final behavior | Expected destination | Disposition |
| --- | --- | --- | --- | --- |
| Fork Docker publishing | `8c813336`, `7b887e44`, `0c315d4a` | Preserve fork registry, multi-arch, manifest, and summary behavior | `.github/workflows/docker-build.yml` | Preserve |
| Wallet defaults and payment return | `711a0315`, `bf162983`, `50f25187`, `1a704ad1`, `ade4551d` | Configurable default amount and correct wallet refresh/redirect behavior | `controller/return_path.go`, `web/src/features/wallet` | Merge with upstream auth and payment fixes |
| Performance metrics | `ab1d7995`, `2d8a8fde`, `7cf40dba`, `250bde67` | Visibility switch, availability colors, correct aggregation, visible-group filtering | `pkg/perf_metrics`, `web/src/features/performance-metrics` | Move and preserve |
| Relay conversion and cache billing | `c54a1d55`, `4d972671`, `cb7ad647`, `4066e54f`, `fc08d7e5` | No public Chat-to-Responses auto conversion, stable tests, cache-write billing, audio ratio compatibility | `relay`, `service`, `setting` | Merge with upstream fixes |
| Channel and group UI state | `38d6e277`, `bbdeb35d`, `057f635e`, `b365401a`, `2d08cd1f` | Persist table options, keep group descriptions, correct selectors and auto-key retry | `web/src/features/channels`, `web/src/features/system-settings`, `web/src/features/keys` | Move and preserve |
| Log retention and visibility | `89c2d59a`, `45f4e67f`, `d565eea6` | Retention limits enforced on active async cleanup; user views hide error logs | `service/system_task.go`, `model/log.go` | Move validation and preserve filtering |
| Playground image flow | Image commits listed below | Multi-provider generation/editing, capabilities, async tasks, persistence, retry/delete/download, retention cap | `controller/playground_image_task.go`, `model/playground_image.go`, `service/playground_image_worker.go`, `pkg/imagecapability`, `web/src/features/playground` | Move and preserve completely |
| Model metadata and pricing display | `076da1ef`, `dd3b02c3`, `2dab0f32`, `cf386058` | Explicit model metadata, selected-group prices, pricing-backed usage-log groups | `model`, `controller`, `web/src/features/pricing`, `web/src/features/usage-logs` | Merge with upstream model fixes |
| Affiliate rewards | `b5fb25e1`, `4190de6e`, `ba3b9be6`, `6adf6ffb` | Transactional reward ledger, correct quota conversion, inviter accounting | `model/affiliate_reward.go`, `model/user.go`, `model/topup.go`, wallet UI | Merge with upstream auth cleanup |
| Quota notifications | `db10c428`, `71bfa129`, `a4a43c99` | IP logging default, balance-based thresholds, notify-limiter enforcement | `service/quota.go`, user settings UI | Merge with auth/cache changes |
| Subscription billing | `3c7df8f5`, `46423a16`, `3219bde1`, `460b36a8`, `3eb2c6ed`, `1779060b`, `525ecf72` | Applicable groups, mixed subscription/wallet settlement, strict fallback, full plan names | `model/subscription.go`, `service/task_billing.go`, subscription and wallet UI | Merge with upstream session cache changes |
| User administration and registration | `bca83882`, `693494a0`, `8aaede90`, `8ccd7a17`, `393aec79` | Override groups, ID search, last-used time, source-aware registration group policy | `controller/user.go`, `controller/oauth.go`, `model/user.go`, user UI | Merge into new authentication paths |
| Locale normalization | `5832779f`, `ea08ee94` | Standard interface locale codes and valid `Intl` locales | `web/src/i18n`, `web/src/lib` | Move and preserve |
| Rankings | `f3236ab4`, `705070a8`, `3642fd14` | Local calendar boundaries, administrator-only access, yesterday period | `service/rankings.go`, `web/src/features/rankings` | Move and preserve |
| Resizable tables | `e079ae13` | Resizable columns fill available width without breaking action sizing | `web/src/components/data-table` | Combine with upstream action-width fix |
| Classic build compatibility | `cd2f8814` | No runtime behavior; only supports retired Classic build | None | Retire with Classic |

## Playground Image Commit Set

The following commits form one protected feature and must be evaluated as a unit:

`a7b870b0`, `72c401ed`, `f042ea22`, `fe04dbd2`, `0f00ee3a`, `1df5a78d`, `78364808`, `325726da`, `3df68604`, `f3018f4f`, `99f171da`, `fce66558`, `31dce377`, `dfe64ed2`, `cc64bf3b`, `30515cc6`, `99853f90`, `4284ca06`, `a934a149`, `2c9a61cb`, `f3a181fb`, `ae0dc6be`, `93aaa3c2`, `8ce14f79`, `2209a200`, `4e40cd8a`, `3ea4788d`, `089f7e9a`, `19d9bb7e`, `9b7fd1ca`, `6347b97e`, `d39cc0bc`, `5c6cfd85`, `6a978443`.

## Local Frontend Additions Requiring Relocation

All paths below move from `web/default` to the same relative location under `web`:

- `src/features/performance-metrics/hooks/use-performance-metrics-visibility.ts`
- `src/features/performance-metrics/lib/aggregate.ts`
- `src/features/performance-metrics/lib/availability.ts`
- `src/features/playground/components/playground-image-input.tsx`
- `src/features/playground/components/playground-image-task-grid.tsx`
- `src/features/playground/components/playground-mode-toggle.tsx`
- `src/features/playground/hooks/use-image-generation-handler.ts`
- `src/features/playground/hooks/use-playground-image-options.ts`
- `src/features/playground/lib/image-generation-capabilities.test.ts`
- `src/features/playground/lib/image-generation-capabilities.ts`
- `src/features/playground/lib/image-payload-builder.test.ts`
- `src/features/playground/lib/image-payload-builder.ts`
- `src/features/playground/lib/image-result-utils.ts`
- `src/features/playground/lib/storage/storage.test.ts`
- `src/features/rankings/access.test.ts`
- `src/features/rankings/access.ts`
- `src/features/wallet/lib/payment-return.ts`

## Complete Local Commit Inventory

Disposition labels are planning requirements and must be confirmed against the final tree.

### Preserve or Merge

- `[Preserve]` `8c813336` ci: keep fork docker workflow
- `[Move/Merge]` `711a0315` feat: default custom topup amount to 100
- `[Move/Merge]` `bf162983` fix: guard wallet default topup amount
- `[Move/Merge]` `50f25187` fix payment return wallet refresh
- `[Move]` `ab1d7995` feat: hide performance metrics when disabled
- `[Move]` `2d8a8fde` fix performance availability colors
- `[Preserve]` `7b887e44` harden docker manifest summary
- `[Move/Merge]` `1a704ad1` feat: make default top-up amount configurable
- `[Merge]` `c54a1d55` fix: remove public chat-to-responses auto conversion
- `[Move/Merge]` `38d6e277` fix(web): persist channels table view options
- `[Preserve]` `4d972671` test: restore gin mode after relay test
- `[Merge]` `89c2d59a` feat: add usage log retention protection
- `[Merge]` `45f4e67f` fix: tighten usage log retention validation
- `[Move/Merge]` `a7b870b0` feat: add playground image generation
- `[Move]` `72c401ed` feat: improve playground image task actions
- `[Move]` `f042ea22` fix: align playground image task result slots
- `[Move]` `fe04dbd2` fix: simplify playground image result slot styles
- `[Move]` `0f00ee3a` fix: keep playground image format control inline
- `[Move]` `1df5a78d` fix: keep playground image controls active without prompt
- `[Move]` `78364808` fix: prevent disabled submit from dimming image controls
- `[Move]` `325726da` fix: unify playground image control styles
- `[Move]` `3df68604` fix: unify playground chat input controls
- `[Merge]` `f3018f4f` fix: recognize grok image models in playground
- `[Move/Merge]` `99f171da` fix: refine playground image generation routing
- `[Merge]` `fce66558` fix xai image generation channel selection
- `[Merge]` `31dce377` fix playground image model visibility
- `[Merge]` `dfe64ed2` fix grok image generation proxy routing
- `[Merge]` `076da1ef` feat: sync and display model metadata
- `[Merge]` `cc64bf3b` fix: remove grok image generation support
- `[Move/Merge]` `30515cc6` fix: split playground gpt image requests
- `[Move/Merge]` `99853f90` fix: clean up playground image generation flow
- `[Preserve]` `271be484` test: isolate channel affinity usage cache stats
- `[Move]` `4284ca06` fix: show full playground image previews
- `[Move/Merge]` `a934a149` feat: add playground reference image edits
- `[Move]` `2c9a61cb` fix: move playground reference previews to lightbox
- `[Move/Merge]` `bbdeb35d` fix: preserve pricing group descriptions
- `[Move/Merge]` `057f635e` fix: keep group descriptions during toggle
- `[Move/Merge]` `b365401a` fix cc switch model dropdown
- `[Move/Merge]` `f3a181fb` feat: support gpt image 2 playground options
- `[Merge]` `b5fb25e1` add topup referral rewards
- `[Merge]` `4190de6e` harden topup referral reward option
- `[Move/Merge]` `dd3b02c3` fix(web): require explicit model spec metadata
- `[Move/Merge]` `db10c428` fix: default enable IP log setting
- `[Move/Merge]` `71bfa129` fix: use balance amount for quota warning threshold
- `[Move/Merge]` `ae0dc6be` Fix playground image interruption persistence
- `[Move/Merge]` `2d08cd1f` Default cross-group retry for auto keys
- `[Merge]` `a4a43c99` fix: rely on notify limiter for quota warnings
- `[Move/Merge]` `7cf40dba` fix perf metrics success rate aggregation
- `[Move/Merge]` `93aaa3c2` add playground image auto size option
- `[Move/Merge]` `8ce14f79` persist playground image task updates
- `[Move/Merge]` `bca83882` fix: include override user groups in user management
- `[Move/Merge]` `ba3b9be6` fix: convert affiliate transfer amount to quota
- `[Merge]` `3c7df8f5` feat: restrict subscription quota by group
- `[Move/Merge]` `46423a16` fix: add subscription applicable group translations
- `[Move/Merge]` `3219bde1` fix: show full subscription plan names
- `[Move/Merge]` `2209a200` Fix playground image model loading
- `[Move/Merge]` `693494a0` feat: support user search by id
- `[Move/Merge]` `8aaede90` feat: show user last used time
- `[Move/Merge]` `250bde67` fix: filter performance metrics by visible groups
- `[Move/Merge]` `2dab0f32` fix: show selected group prices in pricing
- `[Move/Merge]` `5832779f` fix: normalize interface locales for Intl formatting
- `[Move/Merge]` `ea08ee94` fix: use standard interface locale codes
- `[Move/Merge]` `b415a3f1` fix: constrain wallet subscription panel height
- `[Preserve]` `0c315d4a` fix: repair docker publish workflow
- `[Merge]` `cb7ad647` fix: preserve responses cache creation usage
- `[Merge]` `4066e54f` fix: map OpenAI cache write usage
- `[Merge]` `460b36a8` fix: support mixed subscription wallet billing
- `[Move/Merge]` `9a3826d4` fix: constrain wallet subscription layout
- `[Move/Merge]` `6be657a2` fix: restore wallet mobile layout flow
- `[Move/Merge]` `e079ae13` fix: stretch resizable data table columns
- `[Merge]` `d565eea6` fix: hide error logs from user views
- `[Move/Merge]` `f3236ab4` fix: align rankings with local calendar days
- `[Merge]` `6adf6ffb` feat: add transactional affiliate reward ledger
- `[Move/Merge]` `705070a8` fix: restrict rankings page to administrators
- `[Merge]` `8ccd7a17` feat(users): apply registration group policy
- `[Merge]` `393aec79` fix(users): harden registration group policy fallback
- `[Move/Merge]` `4e40cd8a` feat(playground): support multi-provider image generation
- `[Move/Merge]` `3eb2c6ed` fix(subscription): restore applicable group selector
- `[Merge]` `3ea4788d` fix(playground): preserve Gemini image model names
- `[Merge]` `1779060b` fix: fallback to wallet for strict subscriptions
- `[Merge]` `525ecf72` fix: allow mixed billing for subscription first
- `[Move/Merge]` `089f7e9a` fix(playground): download generated images directly
- `[Merge]` `fc08d7e5` fix: repair legacy audio completion ratio option
- `[Move/Merge]` `ade4551d` fix: restore topup success redirect
- `[Move/Merge]` `3642fd14` feat(rankings): add yesterday period
- `[Move/Merge]` `cf386058` feat: select usage log groups from pricing
- `[Move/Merge]` `19d9bb7e` feat(playground): add async image generation tasks
- `[Merge]` `9b7fd1ca` fix(playground): escape mysql option key query
- `[Move/Merge]` `6347b97e` fix(playground): restore local image history controls
- `[Move/Merge]` `d39cc0bc` refactor(playground): remove local image task history
- `[Merge]` `5c6cfd85` feat(playground): retain fifty image results per user
- `[Move/Merge]` `6a978443` feat(playground): cap and hard-delete image history

### Retire with Classic

- `[Retire]` `cd2f8814` fix: make classic date-fns alias portable

Inventory count: 92 preserve/move/merge entries plus 1 Classic-only retirement entry, totaling 93 local non-merge commits.

## Classic Deletion Proof

The final index contains no path under `web/classic` or `web/default`. The intersection between locally modified files and files deleted by this merge contains only the following Classic paths; their active behavior is mapped below.

| Deleted Classic path(s) | Active single-frontend destination or retirement reason |
| --- | --- |
| `web/classic/rsbuild.config.ts` | Retired with the Classic build; `web/rsbuild.config.ts` is the only build configuration. |
| `web/classic/src/components/settings/OperationSetting.jsx` | `web/src/features/system-settings/operations` and the section registry. |
| `web/classic/src/components/settings/PaymentSetting.jsx` | `web/src/features/system-settings/integrations/payment-settings-section.tsx`. |
| `web/classic/src/components/settings/PersonalSetting.jsx` | `web/src/features/profile`. |
| `web/classic/src/components/settings/personal/cards/NotificationSettings.jsx` | `web/src/features/profile/components/tabs/notification-tab.tsx`. |
| `web/classic/src/components/table/tokens/modals/EditTokenModal.jsx` | `web/src/features/keys/components/api-keys-mutate-drawer.tsx`. |
| `web/classic/src/components/topup/InvitationCard.jsx` | `web/src/features/wallet/components/affiliate-rewards-card.tsx`. |
| `web/classic/src/components/topup/RechargeCard.jsx` | `web/src/features/wallet/components/recharge-form-card.tsx`. |
| `web/classic/src/components/topup/SubscriptionPlansCard.jsx` | `web/src/features/wallet/components/subscription-plans-card.tsx`. |
| `web/classic/src/components/topup/index.jsx` | `web/src/features/wallet/index.tsx`. |
| `web/classic/src/helpers/paymentReturn.js` | `web/src/features/wallet/lib/payment-return.ts`. |
| `web/classic/src/helpers/index.js` | Retired Classic-only barrel; payment-return behavior was migrated explicitly. |
| `web/classic/src/i18n/locales/{en,fr,ja,ru,vi,zh-CN,zh-TW,zh}.json` | `web/src/i18n/locales`; standard `zh-CN` and `zh-TW` runtime codes remain normalized by `web/src/i18n/languages.ts`. |
| `web/classic/src/pages/Setting/Model/SettingGlobalModel.jsx` | `web/src/features/system-settings/models/global-settings-card.tsx`. |
| `web/classic/src/pages/Setting/Operation/SettingsCreditLimit.jsx` | `web/src/features/system-settings/general/quota-settings-section.tsx`. |
| `web/classic/src/pages/Setting/Payment/SettingsGeneralPayment.jsx` | `web/src/features/system-settings/integrations/payment-settings-section.tsx`. |

All 59 files added locally since rc.21 were checked against the merge result. Non-frontend files remain at their original paths, the 17 `web/default` additions exist under `web`, and the Classic payment-return helper maps to the typed wallet implementation.

## Validation Record

This section is updated after each stage. A stage may not be marked complete until its commit and gate results are recorded.

### Stage 0

- Commit: this stage commit (`docs: establish rc22 merge preservation audit`)
- UTF-8 audit: passed with `iconv`.
- `git diff --check`: passed.
- Inventory count check: passed; all 93 Git commit IDs exactly match the 93 classified entries above.

### Stage 1

- Commit: this stage's `merge: sync upstream v1.0.0-rc.22` merge commit.
- Merge parents: local preservation commit `4b1276d7` and tagged upstream commit `bc14c18f`.
- Conflict-marker scan: passed; the index has no unresolved entries or conflict markers.
- Old-directory scan: passed; neither old frontend directory is tracked and no source or build configuration refers to either path.
- Local-added-file audit: passed; all 59 local additions remain or have an explicit migration mapping.
- `git diff --cached --check`: passed.
- `bun install --frozen-lockfile`: passed.
- `bun run typecheck`: passed.
- `bun test`: passed, 115 tests.
- `bun run build:check`: passed and produced `web/dist`.
- `go build ./...`: passed after the frontend asset build.
- `go test ./...`: passed.

### Stage 2

- Commit: this stage's `fix: preserve backend customizations after rc22 merge` commit.
- Registration audit: password, OAuth-provider, and WeChat registration all apply the source-aware group policy; existing OAuth users keep their current group. The tests now use rc.22 database Sessions instead of the retired `gin-contrib/sessions` middleware.
- User deletion audit: hard deletion fences authentication, removes Sessions/AuthFlow/Passkey/OAuth/token data, invalidates caches, decrements the inviter count transactionally, and preserves the append-only affiliate reward ledger.
- Subscription audit: applicable groups, subscription-first partial consumption, strict-plan wallet fallback, mixed allocations, wallet-first refunds, and cache refresh behavior remain covered and passed.
- Upstream behavior adopted: ordered auto-group model deduplication, task status CAS before refunds, Sub2API adaptor tests, Responses tool-call deduplication, and model discovery tests all passed.
- Image tool billing: an explicit administrator model-prefix/default price has priority, including explicit zero; absent configuration uses local quality/size pricing and never the rc.22 implicit `$0.15` charge.
- Relay billing audit: public Chat Completions automatic Responses rerouting remains disabled; OpenAI cache-write usage, image completion counting, multi-provider image routing, and tool surcharge logging tests passed.
- Migration audit: standard and fast migration paths contain local business tables plus rc.22 authentication tables. Fast migration was corrected to include `CasbinRule` and `AuthzRole`, with a dedicated parity test.
- Added file: `model/main_migration_test.go` verifies that fast migration creates the authentication, affiliate, performance, system-task, Playground image, Casbin, and authorization-role tables.
- Database compatibility: full empty-database startup migration passed on SQLite, MySQL 8.2, and PostgreSQL 18. The configured MySQL/PostgreSQL Session migration tests also passed, and all key custom/authentication tables were confirmed present.
- `go mod tidy -diff`: passed with no module-file changes.
- `go build ./...`: passed.
- `go test ./...`: passed.

### Stage 3

- Commit: this stage's `fix: preserve frontend customizations after rc22 merge` commit.
- Authentication client audit: no frontend source uses `New-Api-User`, the retired frontend directories, or a legacy Session client. Protected direct streaming requests obtain headers through `getFreshAuthHeaders`; normal requests use the rc.22 `api` client and refresh-cookie flow.
- Feature-path audit: Playground image generation/editing and async history, performance metrics, administrator-only rankings, wallet and subscription flows, payment returns, user management, pricing, and usage-log filtering all remain connected under `web/src`. All 17 relocated local additions exist; every non-test implementation has an import or export reference, and all relocated tests are discovered by `bun test`.
- Payment return repair: generic Stripe and Epay flows now write the same return marker used by Creem, Waffo, and subscriptions. Safari is classified as a same-tab form flow before submission so navigation cannot race marker persistence. Pure return-state, marker-age, quota-change, and browser-target tests were added.
- Locale audit: `sync-i18n.mjs` now always uses English as the stable base, folds legacy root-level keys into the active `translation` namespace, and uses the union of locale keys without overwriting local additions. Chinese image strings accidentally copied into other locales by the old richest-locale algorithm were replaced with reviewed English, French, Japanese, Russian, Vietnamese, Traditional Chinese, and Simplified Chinese values.
- Locale result: all 7 locale files parse, contain only the `translation` root, and expose the same 5,276 keys. The sync report records zero missing, extra, or suspected-untranslated values for every locale; two consecutive sync runs were byte-for-byte idempotent. The only CJK text in English is the intentional WeChat reply keyword `验证码` inside its English instruction.
- Lint baseline audit: the untouched rc.22 frontend produced 386 errors with the locked oxlint version. Safe automatic fixes removed type-import side effects, missing braces, redundant spreads, obsolete catch bindings, and equivalent string/index operations. The 55 inherited nested-ternary findings and 48 inherited array-index-key findings were retained as warnings to avoid an unrelated 103-site rendering rewrite; every other lint error was repaired, leaving 0 errors and 124 visible warnings.
- Structural repair: query-string construction moved to `web/src/features/usage-logs/lib/query-params.ts`, breaking the API/utility import cycle while retaining the barrel export. Search state moved to `web/src/context/search-context.ts`, breaking the Provider/command-menu cycle. `web/src/features/wallet/lib/payment-return.test.ts` owns payment-return regression coverage.
- Mechanical-change audit: the final stage contains 196 modified frontend files, 3 new frontend files, this audit document, and no deletion. Large diffs were checked as brace, type-import, spread, catch-binding, formatting, or the explicit semantic repairs above; typecheck, tests, lint, and the production build passed after formatting.
- Encoding and residue audit: all 200 staged files decoded as strict UTF-8, no replacement characters or common mojibake sequences were found, and the Vietnamese phrase `Âm thanh` was confirmed as legitimate text. Conflict markers, tracked `web/default` or `web/classic` files, old-directory references outside this audit, and `New-Api-User` references are absent.
- `bun install --frozen-lockfile`: passed with no lockfile or installation changes.
- `bun run typecheck`: passed.
- `bun test`: passed, 119 tests.
- `bun run lint`: passed with 0 errors; the 124 reviewed warnings remain visible.
- `bun run format:check`: passed.
- `bun run build:check`: passed and produced the production frontend bundle.

### Stage 4

- Commit: pending
- Full automated validation: pending
- Encoding/mojibake scan: pending
- Docker build: pending

### Delivery

- Pushed commit: pending
- Actions run: pending
- Multi-arch result: pending
