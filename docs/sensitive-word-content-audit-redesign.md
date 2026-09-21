# 敏感词与内容审计重构说明

本文档是 new-api 敏感词功能的当前实现说明。敏感词页面只负责策略和规则，命中证据统一从使用日志的“关键词拦截”详情查看；用户违规次数和白名单统一在用户编辑左抽屉的“内容安全”区域维护。修改代码时同步更新本文档和 sensitive-word-refactor-checklist.md。

## 1. 产品边界

- 全局规则对所有真实定价分组生效；局部规则只对绑定分组生效。
- 局部分组候选来自 ratio_setting.GetGroupRatioCopy()，不接受 auto、空值或不存在的分组。
- 规则动作属于规则本身：block 拦截、observe 只观察、off 关闭。策略表不再保存全局处理模式。
- 同一请求命中多个规则或多个词条只计一次；存在任意 block 规则时，block 优先于 observe。
- 白名单用户仍写审计事件和日志，但不拦截、不增加违规次数、不封禁。
- 非白名单用户的有效拦截按用户字段 sensitive_word_violation_count 累计。达到策略阈值（新环境默认 50）时禁用账号；封禁只修改账号状态、认证版本和会话，不修改任何余额或账务。
- 阈值触发的这一次请求仍返回敏感词提示；后续请求才由令牌鉴权返回账号封禁 403。
- 提示文案中的“余额不退”和严重情形报警只是警示文字，不是余额处理指令。

默认提示文案（支持 {{threshold}} 占位符）：

~~~
你的请求因命中敏感词已被拦截，已记录 1 次；累计达到 {{threshold}} 次将立即封号，余额不退，如果有攻击破解别人网站等情节严重的情况将会直接报警。请勿使用当前分组进行违规对话；如有误判，请联系群主审核并清理你的记录。
~~~

## 2. 请求流程和错误契约

敏感词检查在 token 估算、预扣费、计费、渠道选择、上游调用和重试之前执行：

~~~
请求解析 -> 分组确定 -> 提取规范化提示词 -> 全局/局部规则匹配
       -> 审计事件与类型 8 日志 -> 白名单/观察/拦截决策
       -> 未拦截才进入 token、预扣费、上游和重试
~~~

Request.GetTokenCountMeta().CombineText 是 OpenAI Chat、Responses、Claude Messages、Gemini GenerateContent 和图片文本请求的统一提示词快照来源。auto 分组在检查时使用已经解析的候选定价分组集合，不能通过跨分组重试绕过局部规则。

| 情况 | 审计/日志 | 违规次数 | 请求结果 |
| --- | --- | --- | --- |
| 未命中或 off | 不写 | 不变 | 继续 |
| observe 命中 | 写 action=observe | 不变 | 继续 |
| 白名单命中 | 写 action=whitelist_bypass | 不变 | 继续 |
| 普通用户 block 命中 | 写 action=blocked | 原子加一 | HTTP 422，sensitive_words_detected |
| 达到阈值 | 同上并标记 auto_banned=true | 保留本次次数 | 本次仍返回敏感词提示 |
| 已封禁用户后续请求 | 不进入敏感词计数流程 | 不变 | HTTP 403，user_banned |

敏感词 422 和账号封禁 403 均设置不可重试。敏感词错误不附加破坏配置文案的请求 ID；请求 ID 保留在结构化日志和响应上下文中。拦截请求不产生预扣费、正常消费日志或上游访问。

审计事务失败时，block 失败关闭并返回不可重试 503；observe 仅在已确认命中且明确是审计事件落库失败时放行并记录服务降级，规则读取、用户读取、运行时快照和其他事务错误仍返回 503。请求 ID 加行锁复核保证同一逻辑请求不会重复计数或重复写日志。

## 3. 数据模型

主数据库由 model/sensitive_word.go 和 model/main.go 管理：

| 表/字段 | 作用 |
| --- | --- |
| sensitive_word_policy | 单例策略：启用、检查提示词、完整证据开关、阈值、保留天数、最大提示词字符数、提示文案和版本。 |
| sensitive_word_rules | 规则名称、global/group 范围、block/observe/off 动作、版本和创建人。旧 word/enabled 列只在迁移窗口保留，不作为新 API 的运行时入口。 |
| sensitive_word_rule_words | 规则词条子表，保存规范化词条和哈希。 |
| sensitive_word_rule_groups | 局部规则与真实定价分组的多对多绑定。 |
| sensitive_word_audit_events | 主库审计证据：请求上下文、哈希、脱敏摘要、完整规范化提示词、命中规则/词条/片段、用户状态、次数和余额前后快照。 |
| users.sensitive_word_violation_count | 当前有效违规次数，管理员可编辑或清零。 |
| users.sensitive_word_whitelist | 用户 ID 级白名单唯一权威状态。 |
| 使用日志 type=8 | LogTypeSensitiveWordBlock，保存结构化摘要并通过 audit_id、request_id 关联主库证据。 |

原始请求体、headers、API Key 不保存。完整提示词只保存规范化文本，并受 max_prompt_runes 和 UTF-8 字节上限保护；MySQL 使用 MEDIUMTEXT，PostgreSQL/SQLite 使用 TEXT。关闭完整证据后仍保留哈希、规则、命中词和处理结果，但不保存正文、预览和片段。保留期清理只清空正文和预览，不删除审计元数据。

## 4. 迁移和运行时

启动迁移会创建策略、规则、词条、分组和审计表，并增加用户字段。旧 SensitiveWords Option 和旧 SensitiveWordConfig 只在一次性迁移时读取，导入新表后不再运行时读取；删除新规则不会回退旧词库。旧独立白名单表只在迁移时把启用用户同步到 users.sensitive_word_whitelist，运行时不再查询该表。

迁移遇到异常时只停用敏感词功能并记录系统日志，不拖垮主服务；修复数据库后可重复执行迁移。规则新增、编辑、模式切换、删除和策略保存都会失效 Aho-Corasick 运行时快照，下一次请求即时加载，不需要重启。

用户启用入口（POST /api/user/manage 的 action=enable）在同一行锁事务内恢复状态并清零当前违规次数：

- 从禁用到启用才递增 auth_version、刷新认证缓存和撤销旧会话；已启用用户重复启用只清零次数，不重复刷新认证。
- 不修改 quota、used_quota、钱包/订阅余额、历史消费、审计事件、使用日志或白名单状态。
- 写管理审计动作 sensitive_word.enable_reset，记录状态/次数前后值、入口和 balance_changed=false。
- 下一次有效命中从第 1 次计数。历史证据始终保留。

## 5. 管理接口

所有敏感词管理接口由 AdminAuth 保护：

| 方法和路径 | 作用 |
| --- | --- |
| GET/PUT /api/sensitive-words/policy | 读取/保存全局策略。 |
| GET /api/sensitive-words/groups | 返回分组定价中的可用分组，不含 auto。 |
| GET/POST /api/sensitive-words/rules | 读取规则摘要、创建规则。列表不返回词条正文。 |
| GET/PUT/DELETE /api/sensitive-words/rules/:id | 读取详情、编辑或删除规则。 |
| PATCH /api/sensitive-words/rules/:id/mode | 切换 block/observe/off。 |
| GET /api/log/sensitive-word-audit/:id | 管理员从使用日志详情读取主库完整审计事件。 |
| POST /api/user/manage (action=enable) | 启用用户并清零当前违规次数。 |

旧的 /api/sensitive-words/config、/stats、/whitelist、/audits 和专用 unban 路由不再注册，避免维护多套状态入口。白名单和违规次数统一走用户管理接口。

## 6. 管理界面

### 6.1 敏感词策略页

文件：web/src/features/system-settings/request-limits/sensitive-words-section.tsx。

页面只保留全局策略和一个统一规则表，不显示白名单名单、命中统计卡片或审计列表。规则表同时展示全局/局部规则、分组、词条数量、更新时间和处理模式；右侧操作列支持编辑、删除和模式切换。

规则弹窗固定四个区域：规则名称与范围、使用分组、敏感词条、处理模式。敏感词条支持逐行粘贴和 TXT 导入；搜索框位于“导入 TXT”按钮左侧，只定位当前未保存的 draft.wordsText，不调用后端、不过滤或重建文本。输入后选中并滚动到首个匹配，Enter/Shift+Enter 循环导航，无结果显示 0/0；关闭弹窗清空搜索状态。

### 6.2 用户页

用户列表增加敏感词违规次数列。用户编辑面板使用左抽屉（SheetContent side="left"），在内容安全区提供违规次数数字输入、清零按钮和白名单开关。白名单命中继续请求、不增加次数但仍记录日志。已有违规次数时点击启用必须先确认“会清零当前次数，但历史审计、余额和消费记录不会删除”。

### 6.3 使用日志

使用日志筛选提供类型 8“关键词拦截”。列表只展示脱敏摘要和处理结果；管理员打开既有日志详情抽屉后，前端按 audit_id 请求 /api/log/sensitive-word-audit/:id，显示规则、命中词/片段、请求上下文、用户状态变化、当前次数、自动封禁、证据保留状态和完整规范化提示词。敏感词页面不另建审计列表。

## 7. 文件职责

| 路径 | 职责 |
| --- | --- |
| model/sensitive_word.go | 策略、规则、运行时快照、匹配、审计、计数、封禁和一次性迁移。 |
| model/user.go | 用户字段、显式零值更新和启用重置事务。 |
| model/main.go | AutoMigrate、快速迁移和可失败开放的敏感词迁移调用。 |
| model/log.go / model/log_other.go | 类型 8 和日志结构化摘要/普通用户脱敏。 |
| relay/request_billing.go | 计费前统一敏感词检查和 422/503 错误边界。 |
| controller/sensitive_word.go | 策略、规则、分组和审计详情 API。 |
| controller/user.go | 用户启用重置和管理审计。 |
| controller/relay.go / middleware/auth.go | 敏感词错误封装和后续 user_banned 403。 |
| router/api-router.go | 敏感词与日志详情路由及权限。 |
| web/src/features/system-settings/request-limits/sensitive-words-section.tsx | 策略页、统一规则表、规则弹窗和实时查找。 |
| web/src/features/users/components/users-mutate-drawer.tsx | 用户左抽屉内容安全区。 |
| web/src/features/usage-logs/components/dialogs/details-dialog.tsx | 类型 8 管理员审计详情。 |
| docs/sensitive-word-refactor-checklist.md | 阶段状态和验收记录。 |

## 8. 排障顺序

1. 在使用日志筛选类型 8 并用 request_id 找到事件。
2. 管理员打开详情，核对实际分组、规则模式、命中词、白名单、观察/拦截状态、次数和封禁结果。
3. 根据 audit_id 检查主库审计事件；若没有正文，核对完整证据开关或保留期。
4. 误判先修正规则/分组，再在用户左抽屉调整或清零当前次数，不删除历史证据。
5. 观察模式返回 503 时，区分审计落库失败（应放行并记服务日志）和规则/用户/事务故障（应失败关闭）。
6. 敏感词命中应是 422 sensitive_words_detected；只有后续已禁用账号请求才应是 403 user_banned。
7. 核对敏感词请求没有预扣费、消费日志或上游调用；余额和账务字段应保持不变。
8. 若规则不生效，确认策略启用、提示词检查开启、规则模式不是 off、局部规则绑定真实定价分组，并等待一次请求触发快照刷新。

## 9. 验证命令

~~~bash
go test ./model -run 'SensitiveWord' -count=1
go test ./controller -run 'SensitiveWord|RelaySensitive' -count=1
go test ./router -count=1
go test ./...
go build ./...
cd relaykit && go test ./... -count=1 && go build ./...
cd ../web && bun run typecheck
bunx vitest run src/features/system-settings/request-limits/sensitive-words-section.test.tsx src/features/system-settings/request-limits/sensitive-word-search.test.ts
bun run lint
bun run build:check
~~~

最终提交前还要执行 git diff --check，确认中文文案、英文键名和 UTF-8 文件均无乱码，并核对 main 上只提交本次功能相关文件。
