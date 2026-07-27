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
| 1 | Two-parent merge, frontend flattening, complete local-code migration | Pending |
| 2 | Backend semantic audit and regression coverage | Pending |
| 3 | Frontend semantic audit and regression coverage | Pending |
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

## Validation Record

This section is updated after each stage. A stage may not be marked complete until its commit and gate results are recorded.

### Stage 0

- Commit: this stage commit (`docs: establish rc22 merge preservation audit`)
- UTF-8 audit: passed with `iconv`.
- `git diff --check`: passed.
- Inventory count check: passed; all 93 Git commit IDs exactly match the 93 classified entries above.

### Stage 1

- Commit: pending
- Merge parents: pending
- Conflict-marker scan: pending
- Old-directory scan: pending
- Basic build/test: pending

### Stage 2

- Commit: pending
- Backend targeted tests: pending
- `go build ./...`: pending
- `go test ./...`: pending

### Stage 3

- Commit: pending
- Frontend install/typecheck/test/lint/build: pending
- Locale JSON audit: pending

### Stage 4

- Commit: pending
- Full automated validation: pending
- Encoding/mojibake scan: pending
- Docker build: pending

### Delivery

- Pushed commit: pending
- Actions run: pending
- Multi-arch result: pending
