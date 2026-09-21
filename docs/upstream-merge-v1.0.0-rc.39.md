# 上游 v1.0.0-rc.39 合并审计

## 1. 冻结基线

| 项目 | 值 |
| --- | --- |
| 本地基线 | `main@f0a62f2c6fe50fd4caf64560c075157880b6342d` |
| 上游目标 | `v1.0.0-rc.39@9978ee1e25a647bfe004e96c8719a2cb62c24732` |
| 共同祖先 | `f116414284162ad15d8925f7bca494c109b83e93` (`v1.0.0-rc.25`) |
| 集成分支 | `codex/merge-v1.0.0-rc.39` |
| 预测冲突 | 135（129 内容、4 modify/delete、2 add/add） |
| 实际合并提交 | `fd74c42d99689bfda756ef1cdf3b4eec9ef07fcc` |
| 合并前审计提交 | `87bc5a7c4e441fb4b49c50edd7054565f97e50f4` |
| 本地提交数（祖先之后） | 177 |
| 上游提交数（祖先之后） | 191 |
| 生产操作 | 未授权；本轮不启动容器、不改数据库、不切换代理 |

本次只合并固定正式标签，不引入 `upstream/main` 的后续提交。冲突一律按语义三方比较
处理，禁止整文件 `ours`/`theirs`。

## 2. 决策准则

- **A：上游等价或更好**：采用上游结构、实现与测试，删除本地重复代码。
- **B：上游主体 + 本地窄适配**：保留上游 API/路径，在边界处注入本地业务契约。
- **C：本地语义不可替代**：保留本地实现并增加回归测试；不得复制第二套上游主流程。
- 删除、重命名和数据库迁移必须标明数据/配置兼容结论；无法证明等价即为阻断项。

## 3. 保护项初始映射

| 保护项 | 预定处理 | 初始依据 |
| --- | --- | --- |
| P-01 | C | 保留 fork GHCR 多架构工作流 |
| P-02 | A | 继续单一 `web/src` 前端 |
| P-03–P-05 | B | 采用上游钱包/支付结构，保留本地支付配置与回跳契约 |
| P-06 | B | 合并上游 terminal task metrics，保留可见性和图片 upstream 400 排除 |
| P-07 | B | 保留 relay 协议与 cache-write 计费语义 |
| P-08–P-12 | B | 以上游页面/服务为主体，保留本地分组、日志和隐私语义 |
| P-13–P-16 | B | 保留 Playground 队列/历史/能力，接入上游插件图片路径 |
| P-17–P-21 | B | 保留账务、订阅、用户数据与兼容迁移 |
| P-22–P-29 | B | 保留本地业务规则，收敛到上游接口与 UI |
| P-30 | B | 上游 Request Policies UI + 本地唯一敏感词执行引擎 |
| P-31 | B | 保留七类来源注册表，采用上游 Dialog 结构 |
| P-32 | B | 上游 billing session + 本地 signed BIGINT 边界层 |
| P-33 | B | 上游表达式 UI + raw expression 保留语义 |
| P-34 | B | 上游 Combobox/UI 测试 + 所选 Key `/v1/models` 数据源 |

## 4. 高风险模块决策

### CC Switch

采用 rc.39 `2d7aef741` 的 Dialog/Combobox 结构和测试位置，移除旧的 `pb-52` 布局补丁。
本地仅保留 `getApiKeyModels()`、`skipSessionAuthorization` 与多来源注册表。全局
`/api/user/models` 不能作为模型授权来源。

### Task Plugin 与 Playground

采用上游 JS Task Plugin 体系、内置插件、管理 UI 和计费。历史任务不重写；完成迁移验证
后删除上游已淘汰的旧 task adaptor。Playground 保留持久队列、租约、重试、结果/历史与
清理，通过统一的上游图片入口执行：插件认领模型走插件，未认领模型走普通 Relay。

### 钱包与定价

采用上游预扣和订阅计费流程；保留本地所有钱包余额字段的 signed BIGINT、checked
更新、精确 raw quota 表示以及 schema 校验。首次生产候选配置固定
`trust_quota_usd=0`、`pre_consume_multiplier=1`。已有 task/video 模型先做价格等价
映射，不自动重写生产价格。

分层表达式的最终预扣估算遵循 rc.39：预扣只使用已知输入量，未知 completion 不做本地
8192 token 猜测；最终结算仍使用完整原始表达式和实际 usage。这样本地不会与上游维护两套
预扣算法，同时保留表达式、分组价格和结算精度语义。

首次候选实例的兼容配置（只记录，不在本轮写入生产）为：

| 配置 | 值 | 目的 |
| --- | --- | --- |
| `TASK_PLUGIN_ENABLED` | `true` | 启用 rc.39 插件链路 |
| `TASK_POLL_MAX_FAILURES` | `20` | 保持上游轮询失败/退款边界 |
| `trust_quota_usd` | `0` | 首次发布禁用免预扣旁路 |
| `pre_consume_multiplier` | `1` | 保持原始预扣估算 |
| 现有模型价格 | 不自动修改 | 从生产有效配置生成插件映射；缺映射即阻断发布 |

### 敏感词与安全中心

采用上游安全中心、Passkey、2FA、访问令牌审计和 Telegram OAuth；本地高级敏感词规则、
分组、白名单、审计、observe/block 与自动封禁保留为唯一执行路径，避免与上游简单规则
产生双重拦截。

## 5. 本次实际决策账

本轮遵循“上游已有等价或更好实现即直接采用”的原则；保留项只限于上游没有覆盖、或
属于本地数据/业务契约的窄边界。不存在为了兼容而长期维护两套主流程。

| 模块 | 决策 | 实际结果与依据 |
| --- | --- | --- |
| CC Switch | A + B | 完整采用 rc.39 Dialog/Combobox/Portal、键盘交互和测试；仅保留所选 Key 的 `/v1/models` Bearer 查询、`skipSessionAuthorization` 与七类来源注册表。 |
| 图片工具加价 | A | 采用上游通用工具价格（默认/索引价格）；删除本地 GPT 图片质量/尺寸动态回退，避免重复定价逻辑。 |
| 钱包与额度 | B | 采用上游 billing session、预扣和结算；在统一边界保留 signed BIGINT、精确字符串/BigInt、溢出检查和完整 schema 审计。 |
| Task Plugin | A | 采用上游 JS 插件运行时、内置插件、管理界面和插件计费；旧 Ali/Doubao/Gemini/Hailuo/Jimeng/Kling/Sora/Suno/Vertex/Vidu 任务适配器不保留双栈。 |
| Playground 图片任务 | B/C | 保留本地队列、租约、重试、历史、结果存储和清理；执行器统一接入插件认领/普通图片 Relay，并恢复启动时的执行器注册和 runner。 |
| 敏感词 | B | 采用上游 Request Policies 页面框架；本地高级规则、分组、观察/阻断、白名单、审计和封禁作为唯一执行引擎；删除上游简单 `RequestChecksSection`，避免双重检查。 |
| 安全中心 | A/B | 采用上游 Security、2FA、Passkey、访问令牌审计和 Telegram OAuth；只恢复本地确有契约的窄接口。 |
| Gemini/GPT-image-2 能力 | C | rc.39 没有等价的 Gemini 原生图片/Imagen 与 GPT-image-2 请求校验，保留本地 capability/validation 实现。 |
| 注册分组/充值/用户模型 | B | 采用上游页面和接口结构，恢复注册分组策略、充值完成/邀请账本、管理员价格快照及 endpoint metadata。 |
| 性能指标 | B | 采用上游指标主体和响应隐私字段；控制器仅注入当前用户可见分组，保留图片上游 400 排除、本地/映射 400 以及 5xx/429 失败归类契约。 |
| 认证扩展表兼容 | B | 2FA/Passkey 表尚未完成 expand migration 时按不可用能力处理，旧版/备用库仍可完成密码或 OAuth 登录。 |
| 额度写入边界 | B | token JSON 同时接受 number/decimal string；上下文额度统一按 int64 读取，禁止退回 JS 安全整数或 Go int。 |

## 6. 数据库与发布门禁

生产数据库只允许在后续单独授权的发布阶段操作。发布前必须：

1. 对生产 MySQL 建立可恢复的一致性备份，并恢复至隔离库。
2. 候选镜像连接隔离 MySQL/Redis，禁止外部任务副作用，完成 migration 后比较 schema、
   行数、钱包极值、任务、审计和 Playground 数据。
3. 以旧版 `f0a62f2c` 对迁移后的隔离库做只读兼容检查；出现收缩、删表、余额重算或
   旧版不兼容时停止发布并改做 expand/dual-read/backfill/contract 迁移。
4. 本地检查、候选镜像、最终 `main` 的 amd64、arm64、manifest、签名与 OCI revision
   必须精确对应同一 SHA。

## 7. 文件删除、迁移与职责收敛

- 删除旧的 CC Switch 重复测试，统一到 `dialogs/__tests__`；删除本地 `pb-52` 布局补丁。
- 删除上游已替代的简单请求检查组件；高级敏感词组件接入 Request Policies 注册表。
- 删除旧任务适配器、Gemini/Vertex 视频专用代理和已被插件 artifact 协议覆盖的双路径；历史
  任务数据仍由上游 task/plugin 查询接口读取。
- 删除本地 GPT 图片质量/尺寸动态价格回退，使用上游统一工具价格实现。
- 恢复 `pkg/imagecapability` 外置配置初始化、master 邀请计数同步，以及 Playground 执行器
  注册；这些是本地运行时契约，rc.39 没有等价替代。
- 保留价格/指标/认证的窄适配：价格组列表只返回用户可见分组，性能指标查询按角色过滤，
  认证投影在 2FA/Passkey expand migration 前使用零能力列；这些不是第二套主流程。
- 未删除公开 REST 路径、钱包历史字段、Playground 历史数据表或敏感词审计数据；本轮没有
  数据库写入、删除或生产配置变更。

## 8. 变更与验证记录

| 阶段 | 状态 | 证据 / 待办 |
| --- | --- | --- |
| 基线冻结 | 完成 | 本文第 1 节 |
| P-34 / 审计文档 | 完成 | `87bc5a7c4`；本文件和保护清单已更新 |
| 祖先合并 | 完成 | `fd74c42d9`；固定标签一次性合并，当前无未解决冲突标记 |
| 语义收敛 | 完成 | 后端、CC Switch、Task Plugin、Playground、钱包、敏感词、安全、定价已按本节决策收敛；`main.go` 运行时 wiring 已恢复；上游更优的旧任务适配器、简单 RequestChecks、GPT 动态加价和 completion 预扣猜测均未保留 |
| 数据库克隆验证 | 未完成/阻断 | 尚未执行生产 MySQL 克隆与旧版 `f0a62f2c` 兼容矩阵；仅完成代码级 SQLite migration/schema、钱包和任务测试 |
| 本地 Go 验证 | 完成 | `go test ./...`、`go build ./...`、`go test ./controller -short`、model/service/perf/relay 专项及 RelayKit `GOWORK=off go test ./...` 全部通过；最后一次修复提交 `daf01437e` |
| 本地 Web 验证 | 完成 | `npm test -- --run --reporter=dot`：178 个文件、2099 个测试全部通过；`typecheck`、`lint`（仅既有 warning）、`format:check`、`build:check` 通过。`copyright:check` 仍会列出仓库基线中 1348 个缺少头部的文件，本轮不批量改写无关文件 |
| CI 与主线交付 | 待推送 | 两个指定开发会话已提交到本地 `main`；最终代码和本次精度/测试修复待最终审查后推送，之后必须等待精确 SHA 的 Actions、镜像签名、manifest 与 OCI revision |
| 生产发布 | 未授权 | 需单独执行蓝绿流程 |

## 9. 当前阻断项与风险

1. 未进行真实 MySQL/Redis 克隆迁移和旧版读兼容验证，不能据此批准蓝绿切流。
2. 未执行候选镜像 CI、GHCR 签名、manifest 和 OCI revision 校验；本次只推送代码到 `main`，
   不自动触发生产发布。
3. `copyright:check` 是仓库既有基线问题，当前会要求给 1348 个现有文件补头部；本轮保持范围收敛，未将其扩散为全仓库格式重写。
4. 尚未在生产 MySQL/Redis 克隆库执行候选版本迁移并用旧版 `f0a62f2c` 回读；代码级 SQLite
   migration/schema 和全量 Go 测试通过，不替代真实数据库兼容证据。

以上阻断项只影响交付门禁，不改变已经完成的代码合并；未经用户另行授权，不修改生产容器、
数据库、Caddy 或流量。

## 10. 135 个冲突路径的机械核对清单

冲突路径由 `git merge-tree --write-tree f0a62f2c6fe50fd4caf64560c075157880b6342d 9978ee1e25a647bfe004e96c8719a2cb62c24732` 重新计算，结果为 135 个。当前工作树逐路径与 rc.39 树比较：79 个直接采用上游（A），48 个上游主体窄适配（B），5 个同时承载本地运行时/业务契约（B/C），3 个本地不可替代能力（C）。下表是可复查的路径账，不允许用整文件 ours/theirs 覆盖。

| 路径 | 决策 | 证据/理由 |
| --- | --- | --- |
| common/constants.go | B | 上游主体 + 本地窄适配 |
| common/model.go | B | 上游主体 + 本地窄适配 |
| common/quota_math.go | B | 上游主体 + 本地窄适配 |
| common/quota_math_test.go | B | 上游主体 + 本地窄适配 |
| controller/group.go | A | rc.39 等价/更好实现，采用上游原文件 |
| controller/misc.go | A | rc.39 等价/更好实现，采用上游原文件 |
| controller/model_sync.go | A | rc.39 等价/更好实现，采用上游原文件 |
| controller/oauth.go | B | 上游主体 + 本地窄适配 |
| controller/perf_metrics.go | B | 上游查询主体 + 当前用户可见分组窄适配 |
| controller/pricing.go | B | 上游价格主体 + 返回 payload 的可见分组裁剪 |
| controller/redemption.go | B | 上游主体 + 本地窄适配 |
| controller/relay.go | B/C | 上游主体 + 本地运行时/业务契约 |
| controller/token.go | B | 上游主体 + 本地窄适配 |
| controller/topup.go | B | 上游主体 + 本地窄适配 |
| controller/topup_quota_limit_test.go | B | 上游主体 + 本地窄适配 |
| controller/topup_waffo.go | A | rc.39 等价/更好实现，采用上游原文件 |
| controller/user.go | B | 上游主体 + 本地窄适配 |
| controller/user_manage_test.go | B | 上游主体 + 本地窄适配 |
| main.go | B/C | 上游主体 + 本地运行时/业务契约 |
| middleware/distributor.go | B | 上游主体 + 本地窄适配 |
| model/ability.go | B | 上游主体 + 本地窄适配 |
| model/channel_cache.go | A | rc.39 等价/更好实现，采用上游原文件 |
| model/checkin.go | B | 上游主体 + 本地窄适配 |
| model/log.go | B | 上游主体 + 本地窄适配 |
| model/log_format_test.go | A | rc.39 等价/更好实现，采用上游原文件 |
| model/main.go | B/C | 上游主体 + 本地运行时/业务契约 |
| model/model_meta.go | A | rc.39 等价/更好实现，采用上游原文件 |
| model/option.go | B | 上游主体 + 本地窄适配 |
| model/payment_method_guard_test.go | B | 上游主体 + 本地窄适配 |
| model/pricing.go | A | rc.39 等价/更好实现，采用上游原文件 |
| model/quota_reserve.go | B | 上游主体 + 本地窄适配 |
| model/redemption.go | B | 上游主体 + 本地窄适配 |
| model/subscription.go | B | 上游主体 + 本地窄适配 |
| model/task.go | B/C | 上游主体 + 本地运行时/业务契约 |
| model/token.go | B | 上游主体 + 本地窄适配 |
| model/topup.go | B | 上游主体 + 本地窄适配 |
| model/usedata.go | B | 上游主体 + 本地窄适配 |
| model/user.go | B | 上游主体 + 本地窄适配 |
| model/utils.go | B | 上游主体 + 本地窄适配 |
| pkg/perf_metrics/metrics.go | A | rc.39 等价/更好实现，采用上游原文件 |
| pkg/perf_metrics/metrics_test.go | B | 上游主体 + 本地窄适配 |
| pkg/perf_metrics/types.go | A | rc.39 等价/更好实现，采用上游原文件 |
| relay/channel/gemini/adaptor.go | C | 上游无等价能力，保留本地契约 |
| relay/channel/openai/relay_responses.go | A | rc.39 等价/更好实现，采用上游原文件 |
| relay/chat_completions_via_responses.go | A | rc.39 等价/更好实现，采用上游原文件 |
| relay/chat_completions_via_responses_test.go | A | rc.39 等价/更好实现，采用上游原文件 |
| relay/claude_handler.go | A | rc.39 等价/更好实现，采用上游原文件 |
| relay/common/relay_info.go | B | 上游主体 + 本地窄适配 |
| relay/compatible_handler.go | A | rc.39 等价/更好实现，采用上游原文件 |
| relay/helper/model_mapped.go | A | rc.39 等价/更好实现，采用上游原文件 |
| relay/helper/valid_request.go | C | 上游无等价能力，保留本地契约 |
| router/api-router.go | A | rc.39 等价/更好实现，采用上游原文件 |
| service/billing_session.go | B | 上游主体 + 本地窄适配 |
| service/channel_select.go | A | rc.39 等价/更好实现，采用上游原文件 |
| service/group.go | A | rc.39 等价/更好实现，采用上游原文件 |
| service/log_info_generate.go | B | 上游主体 + 本地窄适配 |
| service/quota.go | B | 上游主体 + 本地窄适配 |
| service/task_billing.go | B/C | 上游主体 + 本地运行时/业务契约 |
| service/task_billing_test.go | B | 上游主体 + 本地窄适配 |
| service/text_quota.go | A | rc.39 等价/更好实现，采用上游原文件 |
| service/text_quota_test.go | B | 上游主体 + 本地窄适配 |
| setting/model_setting/gemini.go | C | 上游无等价能力，保留本地契约 |
| setting/model_setting/global.go | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/components/data-table/toolbar/bulk-actions.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/components/layout/components/app-header.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/auth/forgot-password/components/forgot-password-form.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/auth/hooks/use-email-verification.ts | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/channels/components/dialogs/edit-tag-dialog.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/channels/components/dialogs/param-override-editor-dialog.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/channels/components/dialogs/upstream-update-dialog.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/channels/components/model-mapping-editor.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/channels/hooks/use-channel-upstream-updates.ts | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/dashboard/components/models/performance-overview.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/dashboard/components/overview/overview-dashboard.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/dashboard/components/overview/performance-health-panel.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/keys/components/api-key-timestamp-cell.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/keys/components/api-keys-cells.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/keys/components/api-keys-columns.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/keys/components/api-keys-multi-delete-dialog.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/keys/components/api-keys-table.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/keys/components/dialogs/cc-switch-dialog.tsx | B | 上游主体 + 本地窄适配 |
| web/src/features/models/components/data-table-bulk-actions.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/models/components/dialogs/create-deployment-drawer.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/models/components/dialogs/upstream-conflict-dialog.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/models/components/dialogs/vendor-mutate-dialog.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/models/components/drawers/model-mutate-drawer.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/models/types.ts | B | 上游主体 + 本地窄适配 |
| web/src/features/performance-metrics/types.ts | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/pricing/components/model-details-performance.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/pricing/components/model-details.tsx | B | 上游主体 + 本地窄适配 |
| web/src/features/pricing/components/model-perf-badge.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/pricing/components/pricing-toolbar.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/profile/components/dialogs/change-password-dialog.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/profile/components/dialogs/email-bind-dialog.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/profile/components/dialogs/two-fa-backup-dialog.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/profile/components/dialogs/two-fa-disable-dialog.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/profile/components/tabs/notification-tab.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/security/components/dialogs/two-fa-setup-dialog.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/system-settings/api.ts | B | 上游主体 + 本地窄适配 |
| web/src/features/system-settings/hooks/use-update-option.ts | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/system-settings/models/group-ratio-form.tsx | B | 上游主体 + 本地窄适配 |
| web/src/features/system-settings/models/group-ratio-visual-editor.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/system-settings/models/model-pricing-sheet.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/system-settings/models/model-ratio-visual-editor.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/system-settings/models/tiered-pricing-editor.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/system-settings/request-policies/request-checks-section.tsx | B | 上游主体 + 本地窄适配 |
| web/src/features/system-settings/types.ts | B | 上游主体 + 本地窄适配 |
| web/src/features/usage-logs/api.ts | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/usage-logs/components/__tests__/cost-display.test.tsx | B | 上游主体 + 本地窄适配 |
| web/src/features/usage-logs/components/columns/common-logs-columns.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/usage-logs/components/common-logs-filter-bar.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/usage-logs/components/dialogs/details-dialog.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/usage-logs/components/log-cost-display.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/usage-logs/components/usage-logs-mobile-card.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/usage-logs/components/usage-logs-table.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/usage-logs/lib/query-params.ts | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/users/components/user-quota-cell.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/users/components/user-quota-dialog.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/users/components/users-columns.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/users/components/users-mutate-drawer.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/users/components/users-table.tsx | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/wallet/hooks/use-affiliate.ts | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/wallet/hooks/use-creem-payment.ts | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/wallet/hooks/use-redemption.ts | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/features/wallet/hooks/use-waffo-pancake-payment.ts | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/hooks/use-notifications.ts | A | rc.39 等价/更好实现，采用上游原文件 |
| web/src/i18n/locales/en.json | B | 上游主体 + 本地窄适配 |
| web/src/i18n/locales/fr.json | B | 上游主体 + 本地窄适配 |
| web/src/i18n/locales/ja.json | B | 上游主体 + 本地窄适配 |
| web/src/i18n/locales/ru.json | B | 上游主体 + 本地窄适配 |
| web/src/i18n/locales/vi.json | B | 上游主体 + 本地窄适配 |
| web/src/i18n/locales/zh-TW.json | B | 上游主体 + 本地窄适配 |
| web/src/i18n/locales/zh.json | B | 上游主体 + 本地窄适配 |
| web/src/routes/rankings/index.tsx | B | 上游主体 + 本地窄适配 |

## 11. 本地 177 个提交映射

以下表格由 `git log --reverse f116414284162ad15d8925f7bca494c109b83e93..f0a62f2c6fe50fd4caf64560c075157880b6342d` 生成，共 177 条。P-01 至 P-34 直接引用保护清单；合并/审计提交标为维护项，不产生新的产品语义。未被历史清单单独标注的产品提交标为“待按文件反查”，其代码已在本轮冲突路径账和专项测试中复核，不能据此跳过后续审计。

<details>
<summary>展开 177 条提交</summary>

| 提交 | subject | 映射 |
| --- | --- | --- |
| 8c813336ee68 | ci: keep fork docker workflow | P-01 |
| 711a03151ab9 | feat: default custom topup amount to 100 | P-03 |
| 045068237690 | Merge branch 'codex/topup-default-amount-100' | 合并/审计维护 |
| bf1629833473 | fix: guard wallet default topup amount | P-03 |
| 6890ef262abc | Merge branch 'codex/topup-default-amount-100' | 合并/审计维护 |
| 50f251870572 | fix payment return wallet refresh | P-04 |
| ab1d7995a80d | feat: hide performance metrics when disabled | P-06 |
| 9b5231ac8835 | Merge pull request #3 from QLHazyCoder/codex/perf-metrics-visibility | 合并/审计维护 |
| 2d8a8fde93f1 | fix performance availability colors | P-06 |
| e24aac6edcf3 | Merge pull request #4 from QLHazyCoder/codex/perf-status-availability-colors | 合并/审计维护 |
| 7b887e441c09 | harden docker manifest summary | P-01 |
| 8d10bbefdc49 | Merge pull request #5 from QLHazyCoder/codex/docker-manifest-summary-resilience | 合并/审计维护 |
| 1a704ad198f9 | feat: make default top-up amount configurable | P-03 |
| ee46a49104f3 | Merge branch 'codex/topup-default-amount-100' | 合并/审计维护 |
| c54a1d5510b8 | fix: remove public chat-to-responses auto conversion | P-07 |
| 38d6e277fa83 | fix(web): persist channels table view options | P-09 |
| 4fda3b4bcbc0 | Merge branch 'codex/persist-channels-view-options' | 合并/审计维护 |
| 4d9726717814 | test: restore gin mode after relay test | P-26 |
| 89c2d59a8b79 | feat: add usage log retention protection | P-12 |
| 45f4e67f6901 | fix: tighten usage log retention validation | P-12 |
| 73915854771f | Merge remote-tracking branch 'upstream/main' | 合并/审计维护 |
| a7b870b0a597 | feat: add playground image generation | P-13 |
| 72c401ed27db | feat: improve playground image task actions | P-16 |
| f042ea229785 | fix: align playground image task result slots | P-16 |
| fe04dbd2e8dc | fix: simplify playground image result slot styles | P-16 |
| 0f00ee3a2eb3 | fix: keep playground image format control inline | P-16 |
| 1df5a78d8e11 | fix: keep playground image controls active without prompt | P-16 |
| 7836480865c8 | fix: prevent disabled submit from dimming image controls | P-16 |
| 325726dae7b9 | fix: unify playground image control styles | P-16 |
| 3df68604535d | fix: unify playground chat input controls | P-16 |
| f3018f4f107f | fix: recognize grok image models in playground | P-13 |
| 99f171daeb9e | fix: refine playground image generation routing | P-13 |
| fce665584091 | fix xai image generation channel selection | P-13 |
| 31dce377b89e | fix playground image model visibility | P-13 |
| dfe64ed2b46c | fix grok image generation proxy routing | P-13 |
| 98ec43ee2dc4 | merge dev into main | 合并/审计维护 |
| 076da1ef95be | feat: sync and display model metadata | P-10 |
| aa207079190f | merge dev into main | 合并/审计维护 |
| cc64bf3b675b | fix: remove grok image generation support | P-13 |
| 30515cc67f18 | fix: split playground gpt image requests | P-14 |
| 99853f90d986 | fix: clean up playground image generation flow | P-14 |
| 271be484502a | test: isolate channel affinity usage cache stats | P-26 |
| a69cebcdba32 | Merge pull request #6 from QLHazyCoder/codex/playground-gpt-image-clean | 合并/审计维护 |
| 4284ca06b4f1 | fix: show full playground image previews | P-16 |
| a934a149d562 | feat: add playground reference image edits | P-14 |
| 2c9a61cbd129 | fix: move playground reference previews to lightbox | P-14 |
| 28ccd31cbdf7 | Merge pull request #7 from QLHazyCoder/codex/playground-image-preview-fit | 合并/审计维护 |
| bbdeb35d6981 | fix: preserve pricing group descriptions | P-08 |
| d5ebd8624aca | Merge pull request #8 from QLHazyCoder/codex/preserve-group-descriptions | 合并/审计维护 |
| 057f635ef2cc | fix: keep group descriptions during toggle | P-08 |
| f9dca1b91574 | Merge pull request #9 from QLHazyCoder/codex/fix-group-description-toggle | 合并/审计维护 |
| b365401a7629 | fix cc switch model dropdown | P-09 |
| 8d555522a856 | Merge pull request #10 from QLHazyCoder/codex/fix-cc-switch-model-combobox | 合并/审计维护 |
| f3a181fb8a25 | feat: support gpt image 2 playground options | P-14 |
| b5fb25e11c6d | add topup referral rewards | P-17 |
| 5f2b0d6f1917 | merge topup referral rewards | 合并/审计维护 |
| 4190de6e0c49 | harden topup referral reward option | P-17 |
| 0aa894dd9b8f | Merge branch 'codex/topup-referral-reward' | 合并/审计维护 |
| dd3b02c35297 | fix(web): require explicit model spec metadata | P-10 |
| db10c4283e89 | fix: default enable IP log setting | P-18 |
| 71bfa1297b21 | fix: use balance amount for quota warning threshold | P-18 |
| ae0dc6bee77a | Fix playground image interruption persistence | P-15 |
| 2d08cd1f0ca2 | Default cross-group retry for auto keys | P-09 |
| a4a43c99d348 | fix: rely on notify limiter for quota warnings | P-18 |
| fc0101ca035f | merge: sync upstream main through 79396745 | 待按文件反查 |
| 7cf40dbaaf64 | fix perf metrics success rate aggregation | P-06 |
| 93aaa3c24520 | add playground image auto size option | P-14 |
| 8ce14f790db8 | persist playground image task updates | P-15 |
| bca83882d8e2 | fix: include override user groups in user management | P-21 |
| ba3b9be6b005 | fix: convert affiliate transfer amount to quota | P-17 |
| 14ca440b4406 | Merge remote-tracking branch 'upstream/main' into merge-upstream-20260702 | 合并/审计维护 |
| 3c7df8f50a4e | feat: restrict subscription quota by group | P-19 |
| 46423a166a42 | fix: add subscription applicable group translations | P-19 |
| 3219bde1b5f2 | fix: show full subscription plan names | P-19 |
| 2209a200dcc4 | Fix playground image model loading | P-13 |
| 693494a0c095 | feat: support user search by id | P-21 |
| 8aaede907f09 | feat: show user last used time | P-21 |
| 250bde67ffd2 | fix: filter performance metrics by visible groups | P-06 |
| 2dab0f326f7a | fix: show selected group prices in pricing | P-10 |
| 8148b336a376 | Merge upstream v1.0.0-rc.19 | 合并/审计维护 |
| 5832779f58d8 | fix: normalize interface locales for Intl formatting | P-23 |
| ea08ee94877b | fix: use standard interface locale codes | P-23 |
| b415a3f14090 | fix: constrain wallet subscription panel height | P-05 |
| e2a10750dd45 | Merge upstream v1.0.0-rc.20 | 合并/审计维护 |
| 0c315d4a6032 | fix: repair docker publish workflow | P-01 |
| cd2f8814dcaa | fix: make classic date-fns alias portable | P-02 |
| cb7ad647f9f0 | fix: preserve responses cache creation usage | P-07 |
| 4066e54fea79 | fix: map OpenAI cache write usage | P-07 |
| 460b36a893ea | fix: support mixed subscription wallet billing | P-20 |
| 9a3826d4e004 | fix: constrain wallet subscription layout | P-05 |
| 6be657a2e1b2 | fix: restore wallet mobile layout flow | P-05 |
| fe56f6a0b0de | Merge upstream v1.0.0-rc.21 | 合并/审计维护 |
| e079ae13514f | fix: stretch resizable data table columns | P-25 |
| d565eea69e2d | fix: hide error logs from user views | P-12 |
| f3236ab4c24a | fix: align rankings with local calendar days | P-24 |
| 6adf6ffbc8c5 | feat: add transactional affiliate reward ledger | P-17 |
| 705070a8f7e1 | fix: restrict rankings page to administrators | P-24 |
| 8ccd7a17d895 | feat(users): apply registration group policy | P-22 |
| 393aec79604a | fix(users): harden registration group policy fallback | P-22 |
| 4e40cd8ab020 | feat(playground): support multi-provider image generation | P-13 |
| 3eb2c6ede5df | fix(subscription): restore applicable group selector | P-19 |
| 3ea4788de81d | fix(playground): preserve Gemini image model names | P-13 |
| 1779060bd285 | fix: fallback to wallet for strict subscriptions | P-20 |
| 525ecf7216b5 | fix: allow mixed billing for subscription first | P-20 |
| 089f7e9af958 | fix(playground): download generated images directly | P-16 |
| fc08d7e5c289 | fix: repair legacy audio completion ratio option | P-07 |
| ade4551df8e8 | fix: restore topup success redirect | P-04 |
| 3642fd14f8bf | feat(rankings): add yesterday period | P-24 |
| cf386058c7eb | feat: select usage log groups from pricing | P-11 |
| 19d9bb7e43a7 | feat(playground): add async image generation tasks | P-15 |
| 9b7fd1ca87a8 | fix(playground): escape mysql option key query | P-15 |
| 6347b97edd45 | fix(playground): restore local image history controls | P-16 |
| d39cc0bc8204 | refactor(playground): remove local image task history | P-16 |
| 5c6cfd8512c0 | feat(playground): retain fifty image results per user | P-16 |
| 6a978443ec3a | feat(playground): cap and hard-delete image history | P-16 |
| 4b1276d7dbb3 | docs: establish rc22 merge preservation audit | 合并/审计维护 |
| 0256310dcab2 | merge: sync upstream v1.0.0-rc.22 | 待按文件反查 |
| f8c06ddc5edc | fix: preserve backend customizations after rc22 merge | 待按文件反查 |
| ec15c8e23873 | fix: preserve frontend customizations after rc22 merge | 待按文件反查 |
| 45d39a658297 | docs: record rc22 merge validation | 合并/审计维护 |
| 441ea707b82b | fix: require persisted playground image concurrency | P-15 |
| 9773de9e3c39 | fix(user): keep admin mutations cache-consistent | P-21 |
| 51cd6ec47bf4 | fix: make playground image deletion idempotent | P-16 |
| 51e0b058e7da | fix(pricing): hide inaccessible model groups | P-11 |
| d81c294f6db6 | test(pricing): use a generic private user group | P-11 |
| 504e193f2ace | fix(usage-logs): hide tool surcharge marker | P-12 |
| dd95ab67762a | docs: add custom feature preservation checklist | 合并/审计维护 |
| 88ff1c7cd976 | fix(users): restore upstream group assignment options | P-21 |
| 7d8eeb44d9e8 | merge: sync upstream v1.0.0-rc.23 | 待按文件反查 |
| 571b38f0370a | fix: allow 31-day user dashboard ranges | P-27 |
| 3707af0c4740 | fix(user): isolate self updates from billing fields | P-21 |
| b44e6971e06e | fix(logs): align text request success rate | P-28 |
| 9f0ad7c08837 | docs: complete custom feature preservation map | 合并/审计维护 |
| d67f133be31d | docs: establish rc24 merge preservation audit | 合并/审计维护 |
| 4372401890f4 | merge: sync upstream v1.0.0-rc.24 | 待按文件反查 |
| 34de1e75aef0 | fix: preserve backend customizations after rc24 merge | 待按文件反查 |
| 754a7de43b74 | fix: preserve frontend customizations after rc24 merge | 待按文件反查 |
| 6d5b3e5543ed | docs: record rc24 merge validation | 合并/审计维护 |
| cc70f2c3c726 | fix(perf): exclude upstream image 400 rejections | P-06 |
| e1765fd8e3ce | feat(payment): scope amount discounts by user group | P-29 |
| 4173af597a33 | fix(payment): preserve empty discount group arrays | P-29 |
| eb9c43244520 | docs: update custom feature preservation checklist | 合并/审计维护 |
| 7770d42f83e6 | docs: establish rc25 merge preservation audit | 合并/审计维护 |
| 4ef332374dea | merge: sync upstream v1.0.0-rc.25 | 待按文件反查 |
| fc10fde916e3 | fix: preserve backend customizations after rc25 merge | 待按文件反查 |
| cb565a84b179 | fix: preserve frontend customizations after rc25 merge | 待按文件反查 |
| aec217f1a27f | docs: record rc25 merge validation | 合并/审计维护 |
| 432b4ee68a76 | docs: record rc25 actions success | 合并/审计维护 |
| 21cc64f46737 | feat: rebuild sensitive word auditing | P-30 |
| 1c28b3df4a88 | docs: add new-api maintenance and update guide | 合并/审计维护 |
| b24c6ad9571b | fix: quote sensitive word option key | P-30 |
| 7f17b6307c55 | fix: preserve balance when banning sensitive word users | P-30 |
| ab37d8b51498 | feat: redesign sensitive word content audit | P-30 |
| 84daf8a43d46 | docs: record sensitive word rollout verification | 合并/审计维护 |
| def35c88b21f | docs: add sensitive word feature preservation entry | 合并/审计维护 |
| 384e4988c5a4 | fix: reset sensitive word violations when enabling users | P-30 |
| 2cde82f8acb0 | docs: record sensitive word enable reset rollout | 合并/审计维护 |
| a0bde8284641 | docs: pin release record to immutable image tag | 合并/审计维护 |
| b5cfc6460850 | docs: record frontend test baseline | 合并/审计维护 |
| 416fabe52ad1 | feat: add sensitive word draft search | P-30 |
| 5073dfa33586 | docs: record sensitive word search delivery | 合并/审计维护 |
| 52d68cb40b4d | fix: make sensitive audit storage resilient | P-30 |
| 22eeed17108a | fix: preserve sensitive word message after auto-ban | 待按文件反查 |
| 78ae121811cb | docs: record sensitive auto-ban retry fix | 合并/审计维护 |
| 3dda71a37f0e | Revert "docs: record sensitive auto-ban retry fix" | 待按文件反查 |
| 4c9c5209fd05 | Revert "fix: preserve sensitive word message after auto-ban" | 待按文件反查 |
| 955243896ae4 | feat(auth): add spam folder hint after verification email | 待按文件反查 |
| d1529b6ebf52 | Revert "feat(auth): add spam folder hint after verification email" | 待按文件反查 |
| 889436b7821f | fix(playground): bound retention cleanup query | P-16 |
| 734b6016342a | feat(playground): externalize image capabilities | P-13 |
| 4cd9c9460876 | fix(playground): unify GPT image capabilities | P-13 |
| b2e905185f25 | feat(web): extend CC Switch import sources | P-31 |
| afab95b533e1 | feat(quota): migrate wallet balances to int64 | P-32 |
| 66759ee721f7 | fix(web): preserve tiered pricing expressions | P-33 |
| 5e5c79c1b97d | fix(web): use coder name and scoped CC Switch models | P-34 |
| 21fa3084e0a8 | fix(docs): 更新维护与更新指南，修正发布脚本信息和发布顺序描述 | 待按文件反查 |
| f0a62f2c6fe5 | fix(web): load CC Switch models from selected API key | P-34 |

</details>
