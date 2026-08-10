# 上游合并审计：v1.0.0-rc.24

## 范围与基线

- 上游来源：`https://github.com/QuantumNous/new-api.git`。
- 合并对象：正式标签 `v1.0.0-rc.24`，解析后的提交为
  `5c3abffe8572aa8a49f15c3916707d2019d66af4`。
- 本地基线：`main@9f0ad7c0883721b677c541e5d0a0986a058f3b24`，在隔离分支
  `codex/merge-v1.0.0-rc.24` 上开始合并。
- 共同祖先：`v1.0.0-rc.23` 的目标提交
  `0ab02020603d22e5613bc4cf46bfab06f8567769`。
- 范围仅包含 rc.23 至 rc.24 的 8 个正式上游提交；不纳入上游 `main`、alpha、beta、
  preview、dev 或 nightly 提交。
- 本阶段不直接修改生产 MySQL、PostgreSQL 或 SQLite 数据。既有 `options` 键、用户、
  订阅、邀请账本、认证记录和 Playground 图片记录必须由代码保持兼容，不通过 SQL
  覆盖或重建。

## 合并规则

1. [自研功能保护清单](./custom-feature-preservation-checklist.md) 是本次合并的验收入口；
   P-01 至 P-28 均默认必须保留，只有用户明确批准时才能退休。
2. 禁止使用整文件 `ours`、`theirs` 或等价覆盖处理冲突。每个双方修改文件都必须融合
   上游修复和本地行为。
3. 所有自动合并且双方都改过的文件也要审查，尤其是认证、账务、路由、Relay 和发布
   工作流。
4. 只有确认新路径、验证证据和数据/配置兼容性后，才可把保护项标记为“语义等价的
   上游替代”。
5. 按当前用户要求，不在本机执行 Go、Bun 或 Docker 构建/测试；仅执行格式、编码、
   差异和结构静态检查。完整构建验收由推送 `main` 后的 GitHub Actions 执行。

## 上游增量

| 提交 | 上游内容 | 合并关注点 |
| --- | --- | --- |
| `d6b5ce99d` | HTTP/2 流重置时设置 `Request.GetBody` 以支持透明重试 | 保留 P-07 的协议选择和 P-13/P-14 的图片请求体语义。 |
| `ea4f02101` | 将 replay 元数据迁移到请求体 | 与 Relay 请求体缓存、重放和签名逻辑融合。 |
| `0cd9dc85e` | fork 汇入提交 | 逐文件审查其中的渠道、兑换码和前端变更。 |
| `c9bc03864` | 优化渠道抓取模型分类 | 融合 P-09/P-10 的渠道状态与模型展示行为。 |
| `b941253ae` | Claude/Gemini 连通性测试使用原生请求格式 | 保留多供应商图片与原有 Relay 测试语义。 |
| `1da23d6b3` | 为 access token 与 affiliate transfer 增加临界限流 | 与 P-17 的邀请额度转移和 P-21 用户路径一并复核。 |
| `e926e5cac` | 修复兑换码额度精度损失 | 保留项目的货币/额度转换语义并接受精度修复。 |
| `5c3abffe8` | Release 同步工作流支持可选文件同步 | 不影响 P-01 的 fork GHCR 多架构发布；审查同步工作流权限和发布边界。 |

## Dry-run 冲突与预定处理

`git merge-tree --write-tree` 预测 4 个内容冲突；该命令未写入工作区或索引。

| 路径 | 本地受保护行为 | 上游行为 | 预定融合结论 |
| --- | --- | --- | --- |
| `model/user.go` | P-17 邀请奖励账本，P-21 硬删除/缓存/自更新隔离，P-22 注册分组 | 兑换码、认证与用户更新相关基础调整 | 保留事务边界、账本、缓存失效、注册策略和自更新白名单；仅采纳不改变这些契约的上游修复。 |
| `relay/chat_completions_via_responses.go` | P-07 公共 Chat 不自动改写为 Responses | replay 元数据与 HTTP 请求可重试性 | 保留显式禁用策略及 cache-write 计费，接入安全的请求体重放元数据。 |
| `web/src/features/channels/components/dialogs/fetch-models-dialog.tsx` | P-09 渠道界面状态，P-10 模型展示 | 上游模型分类 UI | 保留当前对话框状态和可访问性，采用上游分类数据模型与展示。 |
| `web/src/features/redemption-codes/components/redemptions-mutate-drawer.tsx` | 当前单前端表单迁移和额度显示约定 | 上游兑换码精度与异步加载保护 | 保留本地表单/货币约定，采用精度换算和加载竞态修复。 |

## 自动合并高风险路径

- `.github/workflows/sync-release-to-gitcode.yml`：P-01 发布边界；不得覆盖
  `.github/workflows/docker-build.yml` 的 fork GHCR 多架构产物。
- `controller/user.go`、`middleware/rate-limit.go`、`router/api-router.go`：P-17、P-21、
  P-22；复核 affiliate transfer、用户自更新、认证清理和注册分组。
- `common/body_storage.go`、`relay/common/outbound_body.go`、`relay/channel/api_request.go`
  及各协议 handler：P-07、P-13、P-14；复核请求体可重读性、图片编辑 multipart、
  cache-write 计费和协议选择。
- `web/src/lib/currency.ts`、`web/src/lib/format.ts`、兑换码表单：P-03、P-17；复核显示
  金额与原始 quota 的双向换算，禁止引入浮点精度回退。

## 自研功能保护矩阵

所有项目在合并开始时均已归类为“必须保留，待最终树验证”；没有未归类的本地功能。

| 编号 | 必须保留的行为 | 当前主要位置 | 初始状态 |
| --- | --- | --- | --- |
| P-01 | fork GHCR amd64/arm64/manifest 发布 | `.github/workflows/docker-build.yml` | 必须保留 |
| P-02 | 单前端结构，Classic 已退休 | `web/src/**` | 必须保留 |
| P-03 | 钱包默认自定义充值金额 | `setting/operation_setting`、`web/src/features/wallet` | 必须保留 |
| P-04 | 支付回跳后钱包刷新 | `service/return_path.go`、钱包前端 | 必须保留 |
| P-05 | 钱包/订阅稳定布局 | `web/src/features/wallet/**` | 必须保留 |
| P-06 | 性能指标开关、可见性与聚合 | `pkg/perf_metrics`、性能前端 | 必须保留 |
| P-07 | Relay 协议兼容与 cache-write 计费 | `relay`、`service`、`relaykit` | 必须保留并融合 |
| P-08 | 分组描述与特殊可见规则 | `service/group.go`、分组设置前端 | 必须保留 |
| P-09 | 渠道状态、CC 切换、自动分组重试 | `web/src/features/channels`、`service/channel_select.go` | 必须保留并融合 |
| P-10 | 模型元数据与选定分组价格 | `model/model_meta.go`、定价前端 | 必须保留并融合 |
| P-11 | 分组可见性、定价脱敏、日志分组 | `service/group.go`、`controller/pricing.go` | 必须保留 |
| P-12 | 日志保留与错误日志权限 | `service/system_task.go`、`model/log.go` | 必须保留 |
| P-13 | Playground 多供应商图片能力/路由 | `pkg/imagecapability`、Relay、Playground | 必须保留 |
| P-14 | Playground 图片请求、编辑、规格 | 图片 DTO、Playground 选项 | 必须保留 |
| P-15 | 异步图片任务、队列、全局并发 | `model/playground_image.go`、worker | 必须保留 |
| P-16 | 图片历史、删除、任务卡交互 | Playground 历史/控制组件 | 必须保留 |
| P-17 | 邀请奖励账本与额度换算 | `model/affiliate_reward.go`、`model/user.go` | 必须保留并融合 |
| P-18 | 额度预警与 IP 日志默认值 | `service/quota.go`、用户设置 | 必须保留 |
| P-19 | 订阅适用分组和管理界面 | `model/subscription.go`、订阅前端 | 必须保留 |
| P-20 | 订阅优先混合扣费 | `service/task_billing.go`、订阅服务 | 必须保留 |
| P-21 | 用户管理、缓存一致性、硬删除 | `controller/user.go`、`model/user.go` | 必须保留并融合 |
| P-22 | 注册来源分组策略 | 注册控制器/策略模型 | 必须保留 |
| P-23 | 标准 locale 与本地化稳定性 | `web/src/i18n`、`web/src/lib` | 必须保留 |
| P-24 | 排行榜本地自然日和管理员边界 | `service/rankings.go`、排行榜前端 | 必须保留 |
| P-25 | 可伸缩数据表列宽 | `web/src/components/data-table` | 必须保留 |
| P-26 | 自研测试隔离与前置条件 | 亲和性/Relay 测试初始化 | 必须保留 |
| P-27 | 个人数据看板 31 天查询窗口 | `controller/usedata.go` | 必须保留 |
| P-28 | 文本请求最终结果与成功率 | `service/text_quota.go`、性能/日志前端 | 必须保留 |

## 阶段记录

### 阶段 0：保护审计

- 基线、标签、8 个上游提交、4 个预测冲突和 28 项保护功能均已登记。
- 检查计划：每个冲突做三方语义比对；每个自动合并高风险文件做本地/上游差异复核；
  合并后反向检查保护矩阵、删除列表、导入、UTF-8、翻译 JSON、冲突标记和 Git 差异。
- 本阶段不修改运行时代码、数据库、部署配置或镜像。
- 验收：提交前执行 `git diff --check`、UTF-8 解码检查、审计矩阵完整性检查和 staged
  diff 复核。

### 阶段 1：正式合并与冲突决议

- 已以 `git merge --no-commit --no-ff v1.0.0-rc.24` 进行正式标签合并；预期的四个
  内容冲突均按语义处理，未使用整文件 `ours` 或 `theirs` 覆盖。
- `model/user.go`：保留本地 `hardDeleteUserTx`，因此硬删除仍会在同一事务中清理认证
  数据、维护邀请人数、发布认证版本并失效缓存；保留邀请余额转用户额度的行锁、原子
  更新和账本事件。上游旧式 `inviteUser` 辅助函数未并入，因为本地 `InsertWithTx`
  已覆盖其职责且额外保证注册奖励账本的事务一致性。
- `relay/chat_completions_via_responses.go`：保留公共 Chat/Claude 不自动改写到
  Responses 的显式拒绝策略；本次请求体可重读及 HTTP/2 重试改进由共享 Relay 请求体
  基础设施承接，不改变该协议选择。
- `fetch-models-dialog.tsx`：保留本地重定向源别名排除、已移除模型和选择状态；采用
  上游独立的模型厂商分类规则与 `dialogBody` 结构，避免分类逻辑重复。
- `redemptions-mutate-drawer.tsx`：保留本地货币/额度接口，采用上游的异步加载竞态
  保护、未编辑额度的原值保留和可编辑精度步长。
- 合并结果没有文件删除；自动合并的认证、Relay、渠道测试、货币格式化和工作流路径
  留待阶段 2/3 逐项复核。

### 阶段 2：后端二次审查与修复

- P-07：`relay/chat_completions_via_responses.go` 仍是显式拒绝入口；同时删除
  `relay/claude_handler.go` 中残留的自动 Chat-to-Responses 分支，避免未来策略配置
  误开时重新改变 Claude 公共协议。Responses cache creation/cache write 的解析、计费
  和日志字段仍位于现有 `relay/channel/openai`、`service/text_quota.go` 与
  `service/log_info_generate.go` 路径，本次上游文件未覆盖这些实现。
- P-17/P-21/P-22：复核 `InsertWithTx` 注册奖励事务、`hardDeleteUserTx` 鉴权清理与
  邀请人数维护、`TransferAffQuotaToQuota` 行锁/原子更新/账本事件、`UpdateWithTx`
  账务字段白名单、`UpdateUserAccessToken` 单列更新，以及密码/OAuth/微信注册分组入口。
  现有生产数据和 `affiliate_reward_events` 未执行 SQL 改写。
- P-12/P-15/P-16/P-18/P-19/P-20/P-24/P-27/P-28：逐路径确认日志期限下沉到异步清理、
  `PlaygroundImageMaxConcurrency` 缺失时仅在有队列任务时明确报错且迁移只补缺失值、
  图片删除重复请求幂等、额度预警/订阅混合扣费/排行榜边界/31 天窗口/最终结果统计均
  仍在原路径；本次 rc.24 没有触及这些模块。
- 上游 Relay 请求体改造已覆盖 JSON、透传、multipart 和任务请求的独立重放 reader，
  `Request.GetBody` 与 `ContentLength` 由原始 body 元数据恢复；保留本地缓存写入计费和
  图片请求语义。自动合并文件未发现删除。
- 阶段门禁（静态）：`gofmt -d` 无输出、`git diff --check`、UTF-8 解码、冲突标记和
  删除列表检查均通过。按用户要求未在本机执行 `go build`/`go test`，待推送后的
  GitHub Actions 进行完整编译与测试验收。

### 阶段 3：前端、翻译与表单二次审查

- 用户管理抽屉仍使用 `/api/group/` 返回的用户可分配分组；`GroupGroupRatio`/分组定价
  配置没有被误当作用户分组来源。该行为与保护清单中已确认的“恢复 new-api 原生用户
  分组编辑”决策一致，默认分组不增加额外的隐式可见性规则。
- P-09/P-10：渠道模型抓取弹窗采用 rc.24 的独立厂商分类库，同时保留本地重定向源模型
  别名排除、已移除模型页签、已选模型状态和保存行为；`model-categories` 的导出与调用
  路径已检查，无孤立实现或旧导入。
- P-03/P-04/P-05：钱包默认充值金额、支付回跳状态恢复和稳定布局仍位于现有 wallet、
  payment-return 与 operation-setting 路径；rc.24 未改动这些文件。
- P-13/P-14/P-15/P-16：Playground 多供应商图片、规格/编辑、异步历史与删除竞态保护
  仍位于原有图片组件和 hook；前端重复删除由任务 ID 锁和删除墓碑阻断，不依赖服务器
  返回 `image task not found` 来决定界面状态。
- P-06/P-19/P-20/P-24/P-25/P-27/P-28：性能指标、订阅/混合扣费、排行榜、表格列宽、
  31 天看板窗口和文本成功率路径均未出现在 rc.24 的前端变更列表中，逐项反查后仍存在。
- 鉴权：`web/src` 未发现 `New-Api-User` 或旧 Session 前端请求头；请求继续通过上游
  鉴权客户端。未发现旧 `web/default` 或已跟踪的 Classic 源文件。
- 翻译：全部 `web/src/i18n/locales/*.json` 可由 JSON 解析器读取，所有 locale 的标量键
  集与 `en.json` 一致；未发现 Unicode replacement character、冲突标记或明显乱码。
- 阶段门禁（静态）：前端路径差异、导入/导出、删除列表、UTF-8、翻译 JSON 和冲突标记
  检查通过。按用户要求未执行 `bun install`、typecheck、测试、lint 或 build；这些检查
  交由推送后的 GitHub Actions 执行。

本阶段没有需要修改的前端业务代码，审计结果以本记录提交固化。

### 阶段 4：最终反向审计与验证记录

- 从 `custom-feature-preservation-checklist.md` 反查 P-01 至 P-28；每项均有最终实现路径、
  语义等价的上游替代，或在保护清单中已有用户批准的退休说明，不存在未归类项。
- `git diff --name-status` 的最终变更未删除受保护源码；未跟踪的本地
  `web/classic/dist` 是被 `web/.gitignore` 忽略的旧构建产物，不进入提交或 GitHub 干净
  checkout，本次不删除用户本地回滚资料。
- 变更文本、Go 格式、差异空白、冲突标记、空文件和 locale JSON 均执行静态检查；没有
  发现 UTF-8 解码错误、replacement character、无效翻译 JSON 或异常文件删除。
- 本机不执行 Go/Bun/Docker 构建和测试。交付后以新提交 SHA 为准检查 GitHub Actions，
  必须确认 `Publish Docker image (Multi-arch)` 的 amd64、arm64 和 manifest Job 全部
  成功；失败时只提交有实际修复的修复提交，瞬时 Runner/网络失败优先重跑 Job。
- 本次只发布代码到 `origin/main`，不执行蓝绿部署；安全 Cookie、可信 HTTPS Origin 和
  `TRUSTED_PROXIES` 仍是正式部署前置条件。

阶段 4 完成后，保留阶段提交和双父合并提交，不 squash、不重写历史。
