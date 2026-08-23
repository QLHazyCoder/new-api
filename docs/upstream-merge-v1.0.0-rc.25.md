# 上游合并审计：v1.0.0-rc.25

## 范围与基线

- 上游来源：`https://github.com/QuantumNous/new-api.git`。
- 合并对象：正式标签 `v1.0.0-rc.25`，解析后的提交为
  `f116414284162ad15d8925f7bca494c109b83e93`。
- 本地基线：保护清单更新后的 `main@eb9c4324452050ac84e4a03c16b9c8ed7db9fb3d`，
  在隔离分支 `codex/merge-v1.0.0-rc.25` 上开始审计。
- 共同祖先：正式标签 `v1.0.0-rc.24@5c3abffe8572aa8a49f15c3916707d2019d66af4`。
- 上游增量：rc.24 至 rc.25 共 39 个正式标签提交；不纳入上游 `main`、alpha、beta、
  preview、dev 或 nightly 提交。
- 本轮开始前本地相对 `origin/main` 仅多出保护清单提交 `eb9c43244`；它不修改运行时代码。
- 上一轮 rc.24 合并交付后新增的自研提交已在保护清单归类：`cc70f2c3c` 属于 P-06，
  `e1765fd8e` 与 `4173af597` 属于 P-29；没有未归类产品提交。
- 本阶段及后续阶段不直接修改生产 MySQL、PostgreSQL 或 SQLite 数据。既有用户、订单、
  充值定价快照、options、账本、订阅、图片任务和认证记录必须由代码保持兼容。

## 合并硬性规则

1. `docs/custom-feature-preservation-checklist.md` 是 P-01 至 P-29 的验收入口；每项必须
   在最终审计中标记为原样保留、语义等价上游替代或经用户批准退休。
2. 禁止使用整文件 `ours`、`theirs` 或等价覆盖冲突；双方修改文件必须按业务语义融合。
3. 自动合并且双方都修改过的文件也要审查，不能只处理 Git 标记的冲突文件。
4. 不因上游目录变化删除本地实现；删除前必须证明 Classic 已退休或功能已完整迁移。
5. 按当前用户要求不执行本地 Go、Bun、Docker 构建或测试；完整编译与测试由推送后的
   GitHub Actions 完成。本地只做格式、编码、JSON、差异、路径和提交结构静态检查。
6. 每个阶段单独提交；全部阶段完成后以 `--ff-only` 快进本地 `main`，一次推送
   `origin/main`。不 squash、不重写历史、不直接部署。

## 上游增量重点

| 领域 | rc.25 变更 | 保护重点 |
| --- | --- | --- |
| Responses/Relay | cached token 结算、参数覆盖、Claude 工具和 Responses 转换修复 | P-07、P-28 的 cache-write 计费、协议选择、最终结果口径 |
| 充值与额度 | 充值原子结算、不可充值订单拒绝、钱包额度保护、并发额度更新 | P-03、P-04、P-17、P-20、P-29；不覆盖本地分组折扣和快照 |
| 渠道测试 | 新的渠道测试模式、状态更新、自动禁用与多适配器支持 | P-09、P-13；保留自研真实用户、状态和计费边界 |
| 模型/路由 | advanced custom 路由编辑器、模型映射、参数透传 | P-07、P-09、P-10；保留本地路由和图片能力注册 |
| 用户/认证 | OAuth 绑定字段、Turnstile、访问令牌旋转确认、用户状态保护 | P-21、P-22；保留自助字段白名单和注册分组策略 |
| Playground/前端 | 流式文字淡入、图片编辑器加固、模型选择同步、Vitest 规范化 | P-13、P-14、P-16、P-23；不删除图片任务交互和本地翻译 |
| 依赖/工作流 | web 测试基础设施和 electron/web 依赖更新、CI 细节调整 | P-01、P-02、P-23；仅接受不影响 fork 发布的上游 CI 变更 |

## Dry-run 冲突清单

`git merge-tree --write-tree main v1.0.0-rc.25` 预测 16 个内容冲突；命令未写入工作区或
索引。冲突处理必须保留下面的本地契约并吸收不冲突的上游修复：

| 路径 | 本地受保护行为 | 上游方向 | 预定处理 |
| --- | --- | --- | --- |
| `controller/channel-test.go` | 渠道测试真实用户、计费/状态边界、图片能力 | 新渠道测试模式与状态检查 | 三方融合，保留用户和计费边界，采用安全的测试模式扩展 |
| `controller/topup.go` | 分组金额折扣、服务端分组、充值定价快照、回跳语义 | 充值额度保护与结算调整 | 保留 P-29 定价函数/快照和 P-04 回跳，逐段接入额度保护 |
| `model/option.go` | 折扣策略两个 option 的严格校验和专用写入 | 新充值/系统 option | 保留专用策略校验，合并新增 option，不允许通用接口绕过校验 |
| `model/payment_method_guard_test.go` | 支付方式/账务测试前置条件 | 充值订单保护测试 | 合并测试覆盖，保持现有支付方法边界 |
| `model/topup.go` | 订单金额、快照、完成和奖励账务不变量 | 原子结算与额度保护 | 保留历史订单和快照兼容，吸收原子更新/CAS，不重算既有订单 |
| `relay/helper/model_mapped.go` | 本地模型映射、图片能力和分组价格 | 映射后协议判断修复 | 保留映射后模型语义，采用上游协议判断修复 |
| `service/funding_source.go` | subscription-first 混合扣费、钱包回退、分摊退款 | 并发额度/状态保护 | 保留资金来源和 allocations，逐段合并原子保护 |
| `web/src/features/usage-logs/components/__tests__/cost-display.test.tsx` | 工具附加费 UI 隐藏契约 | 测试框架迁移 | 迁移测试语法但保留“不展示附加费标记”断言 |
| `web/src/features/usage-logs/lib/format.ts` | request outcome、成功率和费用展示 | 账务/日志格式调整 | 保留 P-28 字段与结果口径，采用上游兼容格式 |
| `web/src/i18n/locales/{en,fr,ja,ru,vi,zh-TW,zh}.json` | 标准 locale、P-29 翻译和本地文案 | 新增/重排翻译键 | 使用 JSON 结构合并，保留本地值、键集合和 locale 标准化 |

## 阶段记录

### 阶段 0：保护清单与 rc.25 基线

- 已完成保护清单与当前 `main` 的先行对比；新增 P-29，补齐 P-06 的 `cc70f2c3c`，
  将映射计数更新为 106，并提交 `eb9c43244`。
- 已获取正式 `v1.0.0-rc.25` 标签，确认其未包含在当前 `main`；记录共同祖先、39 个
  上游提交、16 个预测冲突、P-01 至 P-29 和本轮 4 个本地新增提交归属。
- 阶段静态门禁：工作区干净、`git diff --check` 通过、保护清单映射 106 个提交引用
  均可解析；未修改生产数据，未执行本地构建/测试。

### 阶段 1：正式合并与初步迁移

- 已按业务语义处理 16 个冲突；未使用整文件 ours/theirs 覆盖。
- 渠道测试保留图片模型、Responses Compact 自动识别，并采用 rc.25 并发测试与
  `SupportsResponsesCompact` 校验。
- 充值统一保留本地 `CompleteTopUp`、邀请奖励账本、价格快照和缓存失效，并接入 rc.25
  的严格额度转换与原子额度上限保护；Epay 保留验证错误分类但不绕过统一事务。
- 模型映射采用公共链式解析器，保留 Compact 后缀还原；日志状态、工具加价隐藏策略和
  7 个 locale 的本地键值均保留并合并上游新增键。
- 阶段 1 静态门禁已通过：16 个冲突路径均已解决，无文件删除、无旧前端目录、无真实
  冲突标记；变更 Go 文件已格式化，locale JSON 可解析，`git diff --check` 通过。

### 阶段 2：后端二次审查与修复

- 用户与认证：`hardDeleteUserTx` 继续在同一事务中清理外部身份、2FA、Session、AuthFlow、
  Passkey、Token 和 OAuth 绑定；认证版本栅栏、用户/Token 缓存失效、邀请人数原子维护
  和追加式奖励账本均保留。注册分组策略仍只作用于新建用户，用户自身分组和特殊可见
  规则仍由 `service/group.go` 实时计算，不对 `default` 等名称硬编码公开性。
- 额度与账务：`TryReserveUserQuota`/`TryReserveTokenQuota` 的缓存 Lua 原子预扣、数据库
  条件落库和失败补偿保留；钱包、订阅优先和混合扣费的 allocation/退款路径未被上游
  覆盖。充值完成继续使用 `CompleteTopUp`，保留历史订单金额、分组定价快照、邀请奖励
  事务和 Redis 额度同步；rc.25 的严格额度换算及 int32 上限 CAS 已接入，旧订单不重算。
- 图片与配置：`PlaygroundImageMaxConcurrency` 通过标准/快速迁移的幂等 seed 保留已有值；
  空队列不读取该 option，有任务时缺失/非法值明确报错，持久队列、租约、重试和硬删除
  路径仍存在。日志保留期限在 option 校验和异步清理服务两层执行，性能指标原始图片
  上游 400 排除规则及文本最终结果/cache-write 计费字段仍存在。
- Relay 与路由：公共 Chat-to-Responses 自动转换仍关闭；Responses/Claude/Gemini 的
  用量语义、缓存写入结算、图片能力注册、模型映射链解析和自动分组过滤均保留。没有
  发现 `web/default`、`web/classic` 或受保护后端文件的未解释删除。
- 本阶段发现并修复 1 个合并回归：`controller/topup.go` 的 Epay 验签和
  `CompleteTopUp` 成功后提前 `return`，遗漏向支付网关写回 `success`，会导致重复回调。
  现在成功结算、重复幂等结算和非成功交易都统一返回 `success`，失败路径仍返回 `fail`；
  不涉及数据库数据迁移或旧订单改写。
- 阶段静态门禁：`git diff --check`、变更 Go 文件 `gofmt -d`、保护路径删除扫描和
  变更 locale JSON 解析均通过；按用户要求未执行本地 Go/Bun/Docker 构建或测试。
- 阶段提交：`fc10fde91 fix: preserve backend customizations after rc25 merge`。

### 阶段 3：前端二次审查与修复

- 单前端路径完整保留在 `web/src`；Git 跟踪树中没有 `web/default` 或 `web/classic`，阶段 1
  的前端变更没有产生未解释删除。工作区中若存在同名 ignored 构建产物，不属于提交内容，
  不作为旧前端残留。Playground 图片能力、异步历史、任务重试/下载/预览、
  快速重复删除锁和成功 tombstone 均仍有对应组件与 hook。
- 用户管理编辑抽屉继续通过 `/api/group/` 使用 `GroupRatio` 分组，保持已批准的上游
  原始行为；`GroupGroupRatio` 仍只在系统倍率设置中作为真实计费覆盖配置，不被误当作
  用户可分配分组。钱包充值表单、按用户分组金额折扣、订单定价快照、支付回跳刷新和
  订阅/邀请奖励面板均仍位于现行组件路径。
- 性能指标开关和分组过滤、排行榜管理员访问控制、日志分组筛选/错误日志权限、搜索
  防抖、表格宽度和充值默认金额逻辑均保留。所有业务 API 使用 `@/lib/http-client`
  的新鉴权客户端；未发现 `New-Api-User`、旧 dashboard session 或旧 token 依赖。仅有
  外部延迟探测、GitHub 发布查询和图片下载使用原生 `fetch`，不属于面板鉴权请求。
- 7 个 `web/src/i18n/locales/*.json` 文件均可由 JSON 解析器读取，扁平键数量一致（每个
  5393 个）；本地 locale 标准化和既有中文/区域翻译值保留，上游新增键已同步。
- 阶段静态门禁：前端保护路径删除扫描、旧鉴权依赖扫描、locale JSON 解析/键数检查和
  `git diff --check` 通过；按用户要求未执行 `bun install`、typecheck、test、lint 或
  build。阶段提交：`cb565a84b fix: preserve frontend customizations after rc25 merge`。

### 阶段 4：最终审查与交付

- 保护清单反向核对完成：P-01 至 P-29 均有最终路径、数据/配置说明和验证证据；清单中
  的 118 个提交引用全部可解析，106 个产品提交均已归属保护项，rc.24 维护提交和本轮
  文档/阶段提交均有明确的审计归属。没有发现未解释的本地产品提交或受保护功能删除。
- 变更集静态检查完成：`git diff --check` 通过；所有变更 Go 文件 `gofmt -d` 无输出；
  变更文本均可按 UTF-8 解析；跟踪文件无真正的 `<<<<<<<`/`=======`/`>>>>>>>` 冲突标记，
  无 Unicode replacement character；7 个 locale JSON 均可解析且与英文基准各有 5393 个
  标量键，键集合一致。跟踪树没有 `web/default`、`web/classic` 或阶段 1 未解释删除。
- 跟踪树中的空文件仅有上游长期保留的 `VERSION` 占位文件（提交 `f4450040b` 明确说明
  其由构建/发布流程写入版本）；没有因本次合并产生的空文件。未删除工作区中可能存在的
  ignored `web/classic`、`web/dist` 或 `web/node_modules` 构建产物，它们不在 Git 跟踪树内。
- 按用户要求，本地未执行 Go、Bun、Docker 构建或测试，也未启动开发/预览服务器；这些
  自动检查以推送后 GitHub Actions 为准。密码登录、GitHub OAuth、Passkey、支付回跳和
  Playground 图片操作不做人工/线上回归，仓库内自动测试仍随工作流执行。
- 当前阶段提交序列为：`eb9c43244`（清单先行更新）、`7770d42f8`（阶段 0 审计）、
  `4ef332374`（保留上游 ancestry 的双父合并）、`fc10fde91`（后端复核修复）、
  `cb565a84b`（前端复核修复）；本节记录提交为 `docs: record rc25 merge validation`。
- 代码交付推送 SHA 为 `aec217f1a27f3664845b19f6c2919a5f11c3ea72`。GitHub Actions Run
  [`32661826254`](https://github.com/QLHazyCoder/new-api/actions/runs/32661826254)
  于 2026-08-23 19:43:51 UTC 结束并为 `success`：`Build & push (amd64)`、
  `Build & push (arm64)`、`Create multi-arch manifests` 三个 Job 全部成功，GHCR 版本和
  `latest` manifest 均创建并签名；从 19:37:30 UTC 推送起观察至 19:47:41 UTC 未出现延迟失败。
- 本轮不执行蓝绿部署；正式部署前仍需配置安全 Cookie、可信 HTTPS Origin 和
  `TRUSTED_PROXIES`。后续仅文档提交不会改变已验证的代码合并内容。
