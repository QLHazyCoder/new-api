# Payment Amount Discount Group Policy

## Objective

Limit exact-amount top-up discounts to an administrator-managed whitelist of
occupied user groups while preserving the existing top-up amount, group-ratio,
payment-return, affiliate-reward, and completion semantics.

## Baseline

- Source baseline: `main` at `cc70f2c3c726cf9f6a7fb2ec42854dbbdb0e9016`.
- Comparison baseline: official `v1.0.0-rc.24`.
- Existing formula: normalized requested amount multiplied by the gateway unit
  price, `TopupGroupRatio`, and the exact-amount discount rate.
- Existing local behavior to preserve: configurable default top-up amount,
  payment-return wallet refresh, transactional affiliate rewards, provider
  guards, and unified top-up completion.
- Production delivery is out of scope. This change is source-only and must not
  start a local preview server, restart containers, or deploy.

## Policy Contract

- `payment_setting.amount_discount` remains an exact positive-integer amount to
  finite rate map, with `0 < rate <= 1`.
- `payment_setting.amount_discount_eligible_groups` is a normalized string
  array. A missing or empty array means no group receives an amount discount.
- Candidate groups come from non-soft-deleted users. Enabled and disabled users
  both count; blank group values are excluded.
- New selections must be occupied at save time. Previously selected groups may
  remain selected after their user count reaches zero, and may be removed.
- The server reads the current database-authoritative user group for previews
  and order creation. Clients never submit the group used for pricing.
- Epay, Waffo, and Waffo Pancake share the policy. Stripe, Creem, subscriptions,
  and redemption codes are intentionally unchanged.
- Existing pending orders retain their stored `amount` and `money`; no order is
  repriced after creation.

## Interfaces

- `GET /api/option/payment/amount-discount-groups` (Root): returns occupied
  group names in deterministic order, without user counts.
- `PUT /api/option/payment/amount-discount-policy` (Root): atomically validates
  and stores the amount discount map and eligible group list.
- `GET /api/user/topup/info`: returns the discount map only when the current
  user group is eligible; otherwise returns an empty object.
- `GET /api/user/topup` (Admin): includes an optional parsed pricing snapshot.
  `GET /api/user/topup/self` never exposes that snapshot.

## Pricing Snapshot

Version 1 records the payment provider, user group, quota display type,
requested amount, normalized amount, quota-per-unit, gateway unit price,
top-up group ratio, discount eligibility, applied rate, and final pay money.
Legacy orders keep an empty snapshot and are displayed as legacy records.

## File Responsibilities

- `setting/operation_setting/payment_setting.go`: policy configuration,
  normalization, validation, and immutable runtime policy snapshot.
- `model/payment_group.go`: occupied user-group query.
- `model/topup.go` and `model/topup_pricing_snapshot.go`: snapshot persistence
  and safe serialization.
- `controller/payment_discount_policy.go`: Root policy APIs.
- `controller/topup_pricing.go`: shared Epay/Waffo/Pancake pricing calculation.
- `controller/topup*.go`: gateway integration and role-specific history DTOs.
- `web/src/features/system-settings/`: policy API, form state, and whitelist UI.
- `web/src/features/wallet/`: admin-only pricing snapshot presentation.
- `web/src/i18n/locales/`: all supported translations.

## Development Checklist

- [x] Stage 1: baseline and design record.
- [x] Stage 2: policy configuration, occupied groups, and atomic Root APIs.
- [x] Stage 3: shared pricing and order audit snapshot.
- [x] Stage 4: settings UI and admin audit UI.
- [x] Stage 5: full tests, cleanup, encoding checks, and reverse review.
- [x] Stage 6: commit and push to `origin/main`.

## Stage Review Log

### Stage 1 - Baseline and Documentation

- Status: passed.
- Findings: the working tree was clean; local `main` matched `origin/main`; the
  payment area contains local behavior beyond official `v1.0.0-rc.24` that
  must be preserved.
- Verification: repository and web instructions were read in full; direct
  `AmountDiscount` reads were inventoried; scope exclusions were recorded.

### Stage 2 - Policy and APIs

- Status: passed.
- Findings: the existing generic option loader silently tolerated malformed
  complex JSON, and separate writes could expose a partial policy. Both policy
  keys now have strict model validation, are blocked from the generic Root
  update endpoint, and are committed together through the dedicated endpoint.
- Verification: focused setting, model, and controller tests passed for empty
  policies, exact matching, invalid rates, disabled and soft-deleted users,
  deterministic group names, stale selections, and partial-write rejection.
- Additional repair: corrected a pre-existing four-argument
  `httptest.NewRequest` call that prevented the controller test package from
  compiling with the current Go API.

### Stage 3 - Pricing and Audit Snapshot

- Status: passed.
- Findings: Epay, Waffo, and Waffo Pancake previously repeated the same
  amount, group-ratio, and exact-discount formula. They now use one decimal
  pricing function and capture the same inputs used for the order price.
  Stripe retains its existing independent calculation as an explicit scope
  exclusion.
- Verification: controller, model, and operation-setting package tests passed
  for USD, CNY, and token display modes; exact hits and misses; eligible and
  ineligible groups; group ratios; gateway unit prices; snapshot round trips;
  legacy and invalid snapshots; and user/admin response isolation. A direct
  read audit found no remaining scoped gateway access to the mutable discount
  map.
- Completion safety: all three gateway callbacks still use the persisted
  `TopUp.Amount` and `TopUp.Money` through the existing completion functions.
  Existing pending orders are not repriced or backfilled.

### Stage 4 - User Interface

- Status: passed.
- Findings: the settings form previously sent amount discounts through the
  generic option endpoint. The form now keeps the discount map and eligible
  group list in one draft and sends both through the dedicated atomic policy
  endpoint on Save all settings. The visual editor shows `Available groups:`
  with `Not set` for an empty whitelist, and its edit control sits beside Add
  discount tier. The existing MultiSelect now opens on click/focus and accepts
  a preferred upward Portal position with bounded scrolling and chip folding.
- Verification: component tests passed for empty, selected, stale, and load
  failure states; selecting a group writes back to the form draft and the
  portal reports `data-side=top`. The admin audit component tests passed for a
  complete formula, applied discount status, legacy orders, and excluded
  gateways. Frontend typecheck, changed-file format check, and locale JSON
  parsing passed. The JSON editor mode keeps a read-only available-group
  summary and no longer renders the retired discount-map description key.
- Residual baseline warnings: the repository-wide lint output still contains
  pre-existing warnings in the billing history list; no new warning from the
  whitelist editor remains. Repository-wide format/copyright checks also
  report unrelated baseline files, recorded for the final verification step.

### Stage 5 - Final Verification

- Status: passed for the feature scope; repository baseline exceptions recorded.
- Verification: `go test ./...`, `go vet ./...`, and `go build ./...` passed;
  `go test -race ./setting/operation_setting ./model ./controller` also passed.
  The two new frontend component test files passed together (5/5), frontend
  typecheck, lint, production build, changed-file formatting, locale JSON
  parsing, `git diff --check`, UTF-8 replacement-character scanning, and
  retired-text/old-logic searches passed. The scoped production code has no
  remaining direct amount-discount read outside the explicitly excluded
  Stripe implementation; test fixtures may configure the policy directly.
- Baseline exceptions: repository-wide `bun test` still reports existing
  `node:test`/Bun nested-describe errors and three pre-existing API-key-cell
  failures; `bun run format:check` reports five unmodified files;
  `bun run copyright:check` reports two unmodified files; and `bun run knip`
  reports the repository's existing unused exports/configuration hints and
  treats test files, including the new focused tests, as unused under its
  current entry configuration. The format/copyright paths were not changed by
  this feature, and all changed-file checks are clean.
- Additional repair required for the full Go gate: updated the stale test-only
  import in `service/task_billing_test.go` to the current `relaykit/dto`
  package, then reran the service and full Go checks successfully. This does
  not change production behavior.
- Reverse review: the Root route, database-authoritative group lookup,
  disabled-user inclusion, soft-delete exclusion, stale-selection rule,
  empty-list behavior, atomic policy save, immutable order snapshot, admin-only
  response, gateway scope, and no-deployment boundary were checked against
  the original requirements. No replacement characters or retired discount
  description remain in changed files.

### Stage 6 - Source Delivery

- Status: completed by the feature commit containing this record.
- Delivery record: the feature commit
  `feat(payment): scope amount discounts by user group` was pushed directly to
  `origin/main`. Production readback then exposed that an empty cloned group
  slice serialized as `null`; a narrow follow-up fix keeps the public contract
  at `[]` and adds a regression assertion. The follow-up was used instead of a
  destructive force-push of the already published feature commit.
- Runtime rollout and production policy selection are operational follow-up
  work and do not add source changes to this commit.
