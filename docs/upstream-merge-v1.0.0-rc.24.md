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

### 阶段 1 至交付：待更新

- 阶段 1：正式双父合并提交和冲突决议。
- 阶段 2：后端/Relay/数据兼容性复核与修复提交。
- 阶段 3：前端/翻译/表单复核与修复提交。
- 阶段 4：最终反向审计与验证记录提交。
- 交付：快进本地 `main`、推送 `origin/main`，确认 GitHub Actions 的 amd64、arm64 和
  manifest Job 成功；本次不包含部署授权。
