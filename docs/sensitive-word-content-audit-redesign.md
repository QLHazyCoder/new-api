# 敏感词与内容审计重构说明

本文档说明 `new-api` 当前敏感词、内容审计、违规计数和自动封禁的实现边界。它是后续修改该功能时的入口文档；发布、回滚和日常排障还应同时阅读 [maintenance-and-update-guide.md](./maintenance-and-update-guide.md)。

## 1. 目标与硬约束

系统在请求抵达上游前检查文本提示词，并将命中结果写入管理员可见的使用日志详情。

- 全局规则对全部定价分组生效；局部规则只对绑定的定价分组生效。
- 局部规则的分组只能来自 `ratio_setting.GetGroupRatioCopy()`；`auto`、空分组和不存在的分组均拒绝保存。
- 一个规则可以包含多个词条。规则列表只显示词条数量，完整词条仅在新增/编辑弹窗和管理员详情接口中返回。
- 白名单是 `User.SensitiveWordWhitelist` 字段。白名单用户仍写审计事件和类型 8 使用日志，但不拦截、不增加违规次数、不自动封禁。
- 非白名单用户每个命中请求只增加一次 `User.SensitiveWordViolationCount`，即使同时命中多个规则或多个词条。
- 默认阈值为 5。第 5 次有效命中禁用账号、增加认证版本并撤销会话；**绝不修改 `quota`、余额、历史消费或充值记录**。
- 使用日志是唯一的审计入口。敏感词配置页不展示命中数据、白名单用户或审计列表。
- 完整规范化提示词只保存在主数据库的审计事件中，并且仅管理员通过日志详情按需读取；它绝不会写入普通使用日志、请求体、headers 或 API Key 字段。

默认客户端提示为：

```text
你的请求因命中敏感词已被拦截，已记录 1 次；累计超过 5 次将立即封号，余额不退，如果有攻击破解别人网站等情节严重的情况将会直接报警。请勿使用当前分组进行违规对话；如有误判，请联系群主审核并清理你的记录。
```

管理员可以修改提示文案。文案中的“余额不退”是处罚说明，不触发任何余额写操作。

## 2. 请求处理流程

```text
协议请求
  -> 请求解析和 RelayInfo 分组准备
  -> 提取规范化文本提示词
  -> 全局规则 + 当前定价分组规则匹配
  -> 写主库审计事件
  -> 写类型 8 使用日志
  -> 白名单 / 观察 / 拦截决策
  -> 未拦截才进入 token 估算、预扣费、选渠道、上游调用和重试
```

实现入口是 [controller/relay.go](../controller/relay.go)，在 `service.EstimateRequestToken`、`PreConsumeBilling`、渠道选择和任何上游调用之前执行检查。

### 2.1 分组与 `auto`

普通令牌直接检查 `RelayInfo.UsingGroup`。`auto` 令牌在预扣费前调用 `service.GetRequestAutoGroups` 得到其全部有权限的候选定价分组，并对这些候选逐个匹配。这样后续跨分组重试无法绕过局部规则。

如果局部规则命中，审计事件和类型 8 日志记录实际命中的候选分组，而不是字面量 `auto`。全局规则命中时会记录首个有效候选分组，表示该请求在任何候选路径上都不能继续。

### 2.2 决策矩阵

| 条件 | 审计事件 | 类型 8 日志 | 违规次数 | 封禁 | Relay 继续 |
| --- | --- | --- | --- | --- | --- |
| 未命中 | 不写 | 不写 | 不变 | 否 | 是 |
| `mode=off` | 不写 | 不写 | 不变 | 否 | 是 |
| `mode=observe` 命中 | 写 | 写 `observe` | 不变 | 否 | 是 |
| 白名单命中 | 写 | 写 `whitelist_bypass` | 不变 | 否 | 是 |
| 普通用户命中 | 写 | 写 `blocked` | 原子加一 | 第 5 次一次 | 否，返回 403 |
| 审计事务失败 | 不保留半条记录 | 不写或仅系统错误 | 回滚 | 否 | `observe` 放行并记录降级；`block` 返回 503 |

审计写入失败按策略模式区分处理：`mode=observe` 的目标是观察而不是阻断请求，因此仅当
命中结果已经明确、且失败类型是审计事件落库失败（`ErrSensitiveWordAuditPersistence`）时
允许 Relay 继续，并在服务日志中记录降级；这次请求不增加违规次数，也不伪造“已记录”的
使用日志。规则、用户、运行时快照或其他数据库读取/事务错误仍返回 503。`mode=block` 始终
对审计落库失败保持失败关闭，避免审计不可用时绕过拦截策略。

完整提示词的存储类型按数据库方言选择：MySQL 使用 `MEDIUMTEXT`，PostgreSQL 和 SQLite
使用 `TEXT`。`max_prompt_runes` 是逻辑字符上限，写入前还会执行 UTF-8 字节上限保护
（`SensitiveWordMaxPromptBytes`），保证截断结果是合法 UTF-8 并保留截断标记；因此不能只
通过提高字符上限来替代数据库迁移。

第 5 次之后的请求仍会记录和递增违规次数，但不会重复执行封禁或重复增加认证版本。

### 2.3 启用/解封边界

普通用户管理接口 `POST /api/user/manage` 的 `action=enable` 和专用
`POST /api/sensitive-words/users/:id/unban` 共享同一个行锁事务服务。两条入口的结果完全一致：

| 启用前状态 | 启用前次数 | 状态/次数结果 | 认证与会话 | 余额与证据 |
| --- | ---: | --- | --- | --- |
| 已禁用 | 任意值 | `status=enabled`，次数清零 | 递增一次 `auth_version`，刷新认证缓存并撤销旧会话 | `quota`、`used_quota`、历史审计和使用日志不变 |
| 已启用 | 大于 0 | 保持启用，次数清零 | 不递增认证版本，不撤销新会话 | 同上 |
| 已启用 | 0 | 保持原样 | 不产生认证状态刷新 | 同上 |

清零使用显式 `Updates(map)`，避免 GORM 结构体更新忽略零值。启用操作本身会写一条
`sensitive_word.enable_reset` 管理审计，记录目标用户、状态/次数前后值、入口来源以及
`balance_changed=false`。它只清零当前计数，不删除任何历史证据；下一次有效命中从第 1 次重新计数。
用户列表在次数大于 0 时必须先显示确认文案，避免管理员误以为启用会删除历史记录或余额。

### 2.4 协议与错误响应

`request.GetTokenCountMeta().CombineText` 是统一的提示词快照来源，覆盖 OpenAI Chat、Responses、Claude Messages、Gemini GenerateContent 和图片生成的文本 prompt。

拦截错误为 HTTP `403`，错误码和类型均为 `sensitive_words_detected`，同时设置不可重试和不写普通 Relay 错误日志。OpenAI、Claude 和 Gemini 路径都保持相同的中文提示原文，不附加请求 ID，避免客户端看到被篡改的策略文案。

自动封禁后的客户端重试有一个额外边界：认证中间件早于 Relay 执行，因此第五次命中
完成封禁后，后续重试不会再次进入提示词检查。`middleware.TokenAuth` 会通过当前用户的
违规次数和最近一条 `auto_banned=true` 审计事件识别这种情况，并继续返回当前配置的
敏感词提示及 `sensitive_words_detected`（仍为 HTTP 403）；它不会解除封禁、增加次数、
写入余额或产生上游请求。已清零次数、普通人工禁用、审计查询失败或策略模式为 `off`
仍返回原来的通用封禁提示。这样可以覆盖会在收到 403 后自动重试的第三方客户端，同时
不把普通账号禁用误报为敏感词命中。

## 3. 数据模型与存储位置

### 3.1 主数据库

| 实体 | 责任 |
| --- | --- |
| `users.sensitive_word_violation_count` | 当前有效违规次数，是封禁判断的唯一计数来源，可由管理员在用户抽屉中修正或清零。 |
| `users.sensitive_word_whitelist` | 白名单开关，是运行时唯一权威状态。 |
| `sensitive_word_rules` | 规则名称、范围、启用状态、版本和旧单词兼容列。 |
| `sensitive_word_rule_words` | 一条规则的多词条内容与标准化哈希。新代码的词条权威来源。 |
| `sensitive_word_rule_groups` | 局部规则与分组定价名称的多对多绑定。 |
| `sensitive_word_audit_events` | 完整提示词、哈希、脱敏摘要、命中规则、用户状态、分组、协议和处理结果。 |
| `options.SensitiveWordConfig` | 开关、模式、阈值、提示文案、审计证据保留设置和策略版本。 |

`sensitive_word_user_whitelists` 保留为旧管理接口的兼容元数据表（备注、创建人），不参与运行时放行判断。迁移时会把其启用行单向同步到 `users.sensitive_word_whitelist`；之后用户抽屉的开关为唯一编辑入口。

### 3.2 使用日志数据库

使用日志新增 `LogTypeSensitiveWordBlock = 8`。该日志仍写入 `LOG_DB`，因此支持独立 ClickHouse 日志库；完整审计事件始终写入主数据库，不能写入 ClickHouse。

日志的 `Other.keyword_filter` 是结构化摘要，包含动作、审计 ID、规则 ID/名称、命中词、分组、模型、协议、当前次数、白名单状态、封禁状态、规则版本和 `balance_changed:false`。日志通过 `request_id` 和 `audit_id` 关联审计事件。

普通用户读取自己的日志时，后端会移除 `audit_id`、规则、命中词、提示词哈希和其他管理员证据，只保留处理结果、当前次数、是否白名单放行、是否观察和是否自动封禁。

## 4. 规则、快照与迁移

### 4.1 运行时快照

规则使用 Aho-Corasick 匹配器。规则新增、编辑、启用/停用、删除、策略保存和旧 Option 更新都会调用 `invalidateSensitiveWordRuntime()`，因此同一进程的下一次请求会重新加载规则，不需要重启服务。

策略配置有五秒本地缓存以避免每个 Relay 请求重复查询配置和规则版本；通过本系统 API 保存会立即失效缓存。不要用直接 SQL 修改规则或配置；多实例场景应始终通过管理 API 变更，并在发布流程中逐实例确认新规则已加载。

规则列表使用 SQL 聚合只读取词条数量；编辑某条规则时才读取该规则的完整词条。

### 4.2 旧 `SensitiveWords` 迁移

启动迁移在 [model/main.go](../model/main.go) 的正常和快速迁移路径中执行：

1. 新增用户字段、规则表、词条表、分组表和审计表。
2. 从持久化 `options.SensitiveWords` 导入一个名为“旧配置导入”的全局规则。
3. 旧白名单表的启用行同步到用户字段。
4. 成功完成时写入 `SensitiveWordRulesMigrationVersion=1`。

新编辑器限制单词最多 200 个字符、单条规则最多 10,000 个词条。若历史 Option 中存在超长或超量词条，迁移不会失败、不会静默丢失这些词，也不会写完成标记；运行时继续以旧 Option 兼容匹配它们，并在系统日志写出迁移提示。管理员应在规则弹窗中将这些旧值拆分或修正后，再完成迁移。

一旦完成标记存在，删除导入规则不会重新回退到旧 `SensitiveWords`，避免管理员删除规则后意外继续拦截。

### 4.3 回滚

关闭策略开关或将模式设为“关闭”即可立即停止新拦截。回滚应用代码时不要删除以下数据：规则、审计事件、用户违规次数、白名单字段和日志类型 8 记录。它们均为向后兼容的附加数据。

## 5. 管理界面

### 5.1 敏感词策略页

文件：[web/src/features/system-settings/request-limits/sensitive-words-section.tsx](../web/src/features/system-settings/request-limits/sensitive-words-section.tsx)

页面只包含两类内容：

1. 策略开关：功能启用、提示词检查、完整审计证据保存、拦截/观察/关闭模式、封禁阈值、提示词保存天数和客户端提示。
2. 统一规则表：规则名称、全局/指定分组、使用分组、词条数、状态、更新时间和右侧操作列。

新增和编辑都使用同一个弹窗，支持逐行粘贴和 TXT 上传、范围选择、定价分组多选、去重/空行/超长预览以及启用状态。规则表不展示词条原文、审计记录、白名单列表或统计卡片。日志入口仅跳转到使用日志 `type=8`。

规则弹窗的“敏感词条”编辑区提供实时查找框，搜索框位于“导入 TXT”按钮左侧。查找只对当前未保存的 `draft.wordsText` 做不区分大小写的普通包含匹配；输入后会选中并滚动到首个命中，`Enter`/`Shift+Enter` 可循环前后命中，搜索不会过滤、删除或重建原始文本。草稿文本变化只重算匹配计数，不抢占正在编辑的文本框光标；再次输入搜索词或导航时才重新定位。关闭或重新打开弹窗会清空搜索状态，保存请求仍只提交原有解析后的词条内容。

### 5.2 用户页

用户列表增加“敏感词违规”列，显示次数和白名单状态。用户编辑右抽屉在备注/分组之后、管理员权限之前提供“内容安全”区：

- `敏感词违规次数` 数字输入和“清零”按钮；
- `敏感词白名单` 开关，文案明确说明“命中后继续请求，不增加违规次数，但仍记录日志”。

保存、清零和白名单变更均产生管理员操作审计日志。用户更新接口忽略任何试图通过该表单提交的 `quota` 值，内容安全字段不能改变余额。

### 5.3 使用日志

使用日志类型筛选支持“关键词拦截”。列表显示动作、次数和管理员可见的词条摘要；完整详情在既有日志详情抽屉中按 `audit_id` 延迟获取，包含请求 ID、分组、模型、协议、端点、规则范围/版本、命中词和片段、白名单/观察/拦截状态、封禁结果、提示词哈希、脱敏摘要和完整规范化提示词。

审计详情加载失败时，抽屉仍显示日志摘要，不会影响使用日志的其他功能。

## 6. API 与权限

所有 `/api/sensitive-words/*` 路由由 `middleware.AdminAuth()` 保护。

| 接口 | 用途 |
| --- | --- |
| `GET/PUT /api/sensitive-words/config` | 读取/保存策略。 |
| `GET /api/sensitive-words/groups` | 仅返回分组定价中的候选分组。 |
| `GET /api/sensitive-words/rules` | 返回不带词条内容的规则表数据。 |
| `GET /api/sensitive-words/rules/:id` | 编辑弹窗读取完整词条。 |
| `POST/PUT /api/sensitive-words/rules` | 创建/更新规则。 |
| `PATCH /api/sensitive-words/rules/:id/status` | 启用或停用规则。 |
| `DELETE /api/sensitive-words/rules/:id` | 永久删除规则。 |
| `GET /api/sensitive-words/audits/:id` | 管理员日志详情读取完整审计证据。 |
| `PUT /api/user/` | 管理员同时更新用户信息、违规次数和白名单。 |
| `POST /api/user/manage` (`action=enable`) | 启用用户并按统一事务清零当前违规次数。 |
| `POST /api/sensitive-words/users/:id/unban` | 与普通启用完全一致，兼容旧管理入口。 |

`/stats`、`/whitelist`、`/audits`、`/users/:id/clear-violations` 和 `/users/:id/unban` 保留给已有管理员集成，不在新页面提供入口。使用它们仍会产生管理审计；新页面应优先通过用户编辑抽屉维护计数和白名单。

## 7. 排障顺序

1. 从使用日志 `type=8` 的请求 ID 定位事件，而不是先从词库猜测。
2. 管理员打开日志详情，核对命中词、规则 ID/名称、分组、白名单状态、模式、次数和自动封禁结果。
3. 核对 `sensitive_word_audit_events` 是否有相同 `request_id`，以及 `audit_id` 是否与日志 `Other` 一致。
4. 误判时先修正规则或分组绑定，再在用户抽屉清零违规次数；不要删除历史日志或审计事件。
5. 用户被封禁后，使用用户列表“启用”或专用解封入口会清零当前次数，但不会删除历史证据、余额或消费记录；下一次命中应显示第 1 次。
6. 若用户在第 5 次后仍可访问，检查 `users.status`、`auth_version`、`user_sessions` 的撤销状态和用户认证缓存。
   若客户端显示“正在重新连接”后最终出现 `User has been banned`，先按 request ID 对照时间线：
   第五次请求应有类型 8 审计事件和配置提示，后续重试会在 `TokenAuth` 阶段结束。新版本会
   对审计确认的自动封禁重试返回同一配置提示；若仍是通用文案，核对违规次数是否已被
   管理员清零、最新自动封禁事件是否存在，或当前运行槽位是否已加载包含该修复的版本。
7. 若规则未生效，确认策略已启用、模式不是关闭、规则已启用、局部规则分组存在于分组定价，并确认请求实际使用的分组。
8. 若审计详情为空，检查“保存完整审计证据”是否关闭，或是否已经超过提示词保留天数。清理任务只清空完整提示词和摘要，不删除命中元数据。
9. 若 Relay 返回“敏感词审计暂时不可用，请稍后重试”，先查看同一 request ID 的服务日志和数据库错误。重点确认 MySQL 的 `sensitive_word_audit_events.full_prompt` 为 `MEDIUMTEXT`；旧版本的 `TEXT` 在中文、多字节或长 Responses 提示词下会触发 1406。`observe` 模式遇到明确的审计落库错误应记录“continuing request”并放行，若仍返回 503，继续检查是否为规则/用户读取错误或未运行新版本迁移。
10. 修复数据库类型或配置后，用一条受控测试请求确认：观察模式继续请求且不计数，拦截模式仍返回 403；两种模式都不能写入余额、预扣费或上游调用。

## 8. 文件职责映射

| 文件 | 职责 |
| --- | --- |
| [model/sensitive_word.go](../model/sensitive_word.go) | 规则、快照、匹配、审计事件、违规计数、封禁、旧配置迁移和日志摘要。 |
| [model/user.go](../model/user.go) | 用户违规次数和白名单字段的持久化。 |
| [model/main.go](../model/main.go) | 主库迁移和旧数据导入。 |
| [model/log.go](../model/log.go) | 类型 8、普通用户日志脱敏。 |
| [model/option.go](../model/option.go) | 旧 Option 更新时的运行时快照失效。 |
| [controller/relay.go](../controller/relay.go) | 计费前的检查、403 和禁止重试。 |
| [middleware/auth.go](../middleware/auth.go) | 令牌认证阶段识别自动封禁重试，并保持普通人工禁用的通用响应。 |
| [middleware/utils.go](../middleware/utils.go) | OpenAI 错误响应封装；敏感词响应保留配置文案，不追加传输 request ID。 |
| [controller/sensitive_word.go](../controller/sensitive_word.go) | 管理 API 与操作审计。 |
| [controller/user.go](../controller/user.go) | 管理员编辑次数/白名单和操作审计。 |
| [router/api-router.go](../router/api-router.go) | 管理路由与权限边界。 |
| [web/src/features/system-settings/request-limits/sensitive-words-section.tsx](../web/src/features/system-settings/request-limits/sensitive-words-section.tsx) | 策略与统一规则管理页面。 |
| [web/src/features/users/components/users-mutate-drawer.tsx](../web/src/features/users/components/users-mutate-drawer.tsx) | 用户抽屉的内容安全区。 |
| [web/src/features/usage-logs/components/dialogs/details-dialog.tsx](../web/src/features/usage-logs/components/dialogs/details-dialog.tsx) | 使用日志中的管理员审计详情。 |

## 9. 验证命令

后端根模块：

```bash
go test ./model -run 'Test(MigrateSensitiveWordData|SaveSensitiveWordConfig|SensitiveWord)' -count=1
go test ./controller -run 'Test(SensitiveWordAuditListRedactsFullPrompt|RelaySensitiveWordBlockPrecedesBillingAndRedactsEndpointQuery|UpdateUserPersistsSensitiveWordControlsWithoutChangingQuota|ManageUserEnableResets|SensitiveWordUnbanEndpoint)' -count=1
go test ./model -run 'Test(SensitiveWordAuditPrompt(GormTypeForSupportedDialects|UsesDialectSizedTypes)|TruncateSensitivePromptIsByteSafeForLargeUnicodeInput|SensitiveWordObserveAuditPersistenceErrorIsTyped)' -count=1
go test ./model -run 'TestSensitiveWordAutoBanMessageRequiresCurrentAutomaticBanState' -count=1
go test ./controller -run 'TestShouldAllowSensitiveAuditFailureOnlyInObserveMode' -count=1
go test ./middleware -run 'TestTokenAuthReturnsSensitivePolicyMessageForAutomaticBanRetry' -count=1
go test ./...
go build ./...
```

`relaykit` 是独立 Go 模块：

```bash
cd relaykit
go test ./... -count=1
go build ./...
```

前端：

```bash
cd web
npm run typecheck
npm run build:check
npm run lint
npm test
```

详细阶段状态见 [sensitive-word-refactor-checklist.md](./sensitive-word-refactor-checklist.md)。

## 10. 本轮边界修复发布记录

2026-08-30，提交 `384e4988c5a453b0c000cc4f85d7866e2729f3e6` 已推送到 `main`，GitHub
Actions `33313133592` 的 amd64、arm64、manifest 和 cosign 均成功。不可变镜像标签
`main-384e498` 的 GHCR manifest digest 为 `sha256:314616ab408bb92bdf579d814e043881dc953b5c510e6ddfe354979bc0ed8ebc`。

发布按 standby-first 蓝绿流程完成：先更新 `new-api-green` 并确认健康，再将 Caddy 三处
上游从 blue 平滑切换到 green；两槽均为 healthy，公网 `/api/status` 连续返回 HTTP 200 和
版本 `main-384e498`。本次上线只改变启用/解封的状态与违规次数边界，不清理余额、消费、
历史审计、使用日志或白名单状态。

## 11. 自动封禁重试响应修复（2026-08-31）

运行日志复核确认：第五次敏感词命中在 Relay 中已经按配置文案返回并完成自动封禁；
客户端随后自动重试，重试请求在 `TokenAuth` 阶段发现账号已禁用，于是显示了通用
`User has been banned`。本轮修复在认证阶段查询当前违规次数和最近自动封禁审计事件，
仅对该类重试返回配置的敏感词文案和 `sensitive_words_detected`，不改变封禁状态、
计数、余额或历史证据。观察模式仍只影响后续命中，不能追溯解除已经存在的封禁；管理员
启用/解封并清零次数后，认证恢复通用的账号状态判断。代码提交为
`22eeed17108a2d6ad5bf1078c4ef55f4923f6ed4`，已推送 `main`；本次未执行线上部署。
