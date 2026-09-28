# 敏感词重构清单

## 基线与隔离

| 项目 | 值 |
| --- | --- |
| 官方实施基线 | `d04c118c8803f49e0c9bab74dcf5b5efeab9464a` |
| 实施分支 | `codex/sensitive-word-p30-d04c118c` |
| 工作目录 | `/opt/qlh-main/.worktrees/new-api-sensitive-word-d04c118c` |
| 原始工作树 | 未修改；其两份用户文档保持未暂存 |

提交 PR 前必须重新 `fetch upstream/main`。若官方基线推进，先重新变基、复查受影响调用链并重跑本清单中的完整验证。

## 文件职责

| 路径 | 职责 |
| --- | --- |
| `model/sensitive_word_types.go` | 表模型、DTO、常量、跨数据库完整提示词类型和旧结构 DTO |
| `model/sensitive_word_rules.go` | 策略/规则校验、CRUD、实际定价分组约束 |
| `model/sensitive_word_runtime.go` | 文本规范化、Aho-Corasick 快照、候选分组单次扫描 |
| `model/sensitive_word_audit.go` | 审计事务、幂等计数、阈值禁用、启用清零、留存和类型 8 日志 |
| `model/sensitive_word_migration.go` | 新表初始化后的单向旧数据导入和迁移标记；兼容本地主线历史独立白名单表，但不创建或运行时读取该表 |
| `model/sensitive_word_database_matrix_test.go` | SQLite/MySQL/PostgreSQL 迁移矩阵与 ClickHouse 日志投影矩阵 |
| `middleware/sensitive_word.go` | 有效分组解析后、渠道选择前的协议 DTO 预检和 422/503 响应封装 |
| `relay/request_billing.go` | 复用预检决定；未经过分发中间件时的计费前兜底和 422/503 错误边界 |
| `relaykit/dto/openai_request.go` | Responses `function_call_output.output` 的文本提取 |
| `controller/responses_websocket_test.go` | 非主节点 WebSocket 夹具的敏感词已迁移 schema，以及渠道选择前阻断回归测试 |
| `controller/sensitive_word.go`、`router/api-router.go` | AdminAuth 管理接口和审计详情接口 |
| `web/src/features/system-settings/request-policies/sensitive-words/` | 策略表单、规则表、弹窗、TXT 导入、草稿搜索和独立 API |
| `web/src/features/users/` | 违规次数/白名单维护与启用确认 |
| `model/log.go`、`model/log_other.go` | 普通用户与 API Key 的类型 8 查询边界及公共元数据投影 |
| `model/log_sensitive_visibility_test.go` | SQLite 角色、分页、总数、请求 ID 与 token 查询权限回归 |
| `router/api-router.go`、`controller/log_test.go`、`controller/sensitive_word_test.go` | 管理员详情路由边界与角色/审计权限回归 |
| `web/src/features/usage-logs/lib/sensitive-word-outcome.ts` | 管理员列表与详情共享的三态结果映射；无历史顶层兼容 |
| `web/src/features/usage-logs/components/columns/common-logs-columns.tsx` | 管理员桌面/移动列表的结果、计数、封禁及命中词摘要 |
| `web/src/features/usage-logs/components/dialogs/sensitive-word-audit-section.tsx` | 唯一管理员审计详情和受权限约束的证据读取 |
| `web/src/features/usage-logs/components/dialogs/details-dialog.tsx` | 提供新结构 `audit_id` 并只挂载一次详情组件 |
| `web/src/features/usage-logs/components/common-logs-filter-bar.tsx` | 根据账号真实角色提供类型 8 筛选项 |
| `web/src/features/usage-logs/components/usage-logs-provider.tsx`、`components/usage-logs-table.tsx`、`lib/columns.ts` | 区分全局查看模式和账号真实角色；管理员“仅自己”保留类型 8 审计详情入口 |
| `web/src/features/usage-logs/components/usage-logs-table.tsx`、`components/__tests__/billing-source-visibility.test.tsx` | 按用户 ID 和角色隔离日志查询缓存，角色降级时不得沿用管理员“仅自己”审计行 |
| `web/src/features/usage-logs/components/__tests__/sensitive-word-audit.test.tsx`、`detail-preview.test.tsx`、`log-type-filter.test.tsx` | 新元数据、三态、唯一详情、权限和筛选项的前端回归 |
| `model/log_other_test.go`、`model/sensitive_word_test.go` | 元数据和三种真实审计写入的回归 |
| `controller/user_manage_test.go` | 管理员编辑接口缺失/显式安全字段的 SQLite 回归，核对 quota 和 used_quota 不变 |
| `web/src/features/system-settings/request-policies/section-registry.tsx`、`features/users/types.ts`、`features/usage-logs/lib/format.ts` | 清除合并留下的无效导入/重复键，保持后出现的实际运行值不变 |
| `web/src/features/system-settings/models/upstream-price-cells.tsx` | 补齐现有可选比较价格类型声明，不改变调用和展示逻辑 |
| `web/src/features/users/lib/user-form.ts` | 基础用户表单 payload 不隐式包含敏感词违规次数/白名单，避免陈旧字段覆盖安全状态 |
| `web/src/features/users/components/users-mutate-drawer.tsx` | 仅在对应控件实际变更时追加安全字段，允许显式提交 `0` 或 `false`；本次无需修改已有 dirtyFields 分支 |
| `web/src/features/users/lib/__tests__/user-form.test.ts`、`features/users/components/__tests__/permissions.test.tsx` | 验证转换函数不附带陈旧安全字段、权限编辑不覆盖计数和白名单、显式清零/关闭仍可提交 |
| `docs/sensitive-word-content-audit-redesign.md` | 架构、数据流、迁移和排障说明 |

## 阶段记录

| 阶段 | 状态 | 自检与结果 |
| --- | --- | --- |
| 1. 模型、迁移和运行时 | 完成 | 模型拆分完成；规则去重、范围、模式优先级、热更新、白名单、幂等计数、并发、阈值、启用清零、审计失败、UTF-8 截断均有定向测试。 |
| 2. Relay 与认证安全 | 完成 | HTTP 分发和 Responses WebSocket 都在渠道选择、估算和预扣费前检查；预检决定由计费阶段复用；阻断为 422、审计失败为 503、禁用令牌为 403 `user_banned`；状态变更后执行认证缓存、会话和令牌失效。 |
| 3. 前端与多语言 | 完成 | 新页面、规则弹窗、左侧用户抽屉、管理员详情和七种 locale 已接入；旧安全页路由重定向保留。 |
| 4. 数据库和文档 | 完成 | 四类实际数据库矩阵、全量 Go/web 验证、定向质量检查和桌面/移动视口检查完成；全局质量脚本的既有失败已与官方基线逐项对照。 |
| 5. PR 复核与发布 | 等待官方审批 | 官方 `main` 已在发布前重新拉取并固定在 `d04c118c`；反向差异审查确认无旧入口和无超出本功能范围的差异；功能分支已推送并创建官方 Draft PR，workflow 等待维护者批准。 |

## 已修正的问题

| 发现 | 修复 |
| --- | --- |
| 旧实现按自动候选分组重复扫描长提示词 | 单个 Aho-Corasick 快照只扫描一次，再按候选分组解析元数据。 |
| Responses `function_call_output.output` 未被检查 | 支持字符串、内容数组和 JSON 对象，并保持请求序列化不变。 |
| 观察模式的审计写入失败可能放行 | 所有审计持久化失败均为不可重试 `503`。 |
| 停用/启用认证失效某一步失败会跳过后续步骤 | 提交后依次尝试三种失效操作并记录单项失败。 |
| 预检和计费兜底可能对同一请求重复审计 | 在请求上下文缓存预检决定，计费只复用；新增回归测试确认违规和审计均只写一次。 |
| 多节点规则修改只能使本机快照失效 | 规则修改与策略版本递增置于同一事务；所有节点以持久化版本重新构建快照。 |
| 已完成迁移后的策略读取错误可能被当作停用 | 区分迁移未完成的 fail-open 与数据库读取错误；Relay 对后者返回不可重试 `503`。 |
| MySQL 矩阵断言使用保留字 `key` 的原始条件 | 新测试使用结构化 `Option` 条件，跨方言通过。 |
| 旧请求策略测试错误模拟了新规则接口 | 独立 mock `/api/sensitive-words/*`，并在前端 API 层校验对象/数组响应。 |
| 审计详情 API 的 `success:false` 会被当作空数据 | 复用统一业务响应校验，详情改为明确的加载失败状态。 |
| Responses WebSocket 未经过 `Distribute`，可能在选择渠道后才被计费兜底检查 | 解析 `response.create` 后调用与 HTTP 共用的 DTO 预检；新增测试确认 `function_call_output.output` 命中时没有渠道握手、预扣费或额度变化。 |
| 非主节点 WebSocket 测试夹具没有迁移表和标记 | 夹具显式建立敏感词 schema 并运行单向迁移，保留生产运行时对“已迁移后策略读取失败”返回 503 的安全边界。 |

## 2026-09-28 本地主线日志权限与状态修复

| 阶段 | 状态 | 发现与处理 | 自检/验证 |
| --- | --- | --- | --- |
| 查询边界 | 完成 | 普通用户与 API Key 在计数和取数前均排除类型 8；新类型 8 日志只写管理员元数据，管理员/超级管理员仍可查看本人和全局审计。 | Go `./...`、SQLite 角色/分页/筛选/路由测试通过；ClickHouse 独立库因未配置 DSN 未执行。 |
| 管理界面 | 完成 | 合并丢失三态摘要、详情组件重复；按新 `admin_info.keyword_filter.action` 显示拦截/白名单/观察，移除旧顶层字段读取；账号/角色进入查询和草稿缓存键。 | 定向 Vitest 6 个文件 57 项通过，角色降级专项 7 项通过；`bun run typecheck`、目标格式检查和 build:check 通过。目标 lint 无错误，仅一条未改动上游定价列表 key 警告。 |
| 文档与交付 | 完成 | 更新本清单、架构说明、P-30 保护清单和维护指南；保留其他自开发功能及既有数据迁移。 | `bun run i18n:sync` 和 `git diff --check` 通过；日志权限修复已于 `25e5f87a4` 提交推送。 |

本次不更改数据库 schema、不回填或删除历史日志。旧顶层日志仍可由管理员查看日志行，但处理结果不会从旧字段推断；缺失新结果数据时界面显示“未知”。

## 2026-09-28 用户编辑安全字段修复

本地主线代码和回归测试提交为 `54da177f7`，与 P-30 保护清单同属本次交付。

| 阶段 | 状态 | 发现与处理 | 自检/验证 |
| --- | --- | --- | --- |
| 前端提交边界 | 完成 | `transformFormDataToPayload` 在普通更新中无条件发送安全字段，使抽屉的 `dirtyFields` 判断失效；移除转换函数里的两个默认赋值，保留控件实际修改时的显式添加逻辑。 | 原权限用例 4 项中 2 项失败复现；修复后与新增转换单测合计 7 项通过（含非零旧计数、白名单开启、显式 `0` 和 `false`）。 |
| 后端与余额边界 | 完成（生产代码不变） | `controller.UpdateUser` 仅按字段是否存在调用 `UpdateSensitiveWordUserFieldsWithTx`；`User.EditWithTx` 已不隐式保存安全字段。增加实际编辑接口用例，检查缺失/显式 `0` 与 `false`、quota 和 used_quota。 | SQLite 3.50.4 控制器专项通过，`go test -p 1 ./controller -count=1` 通过（52 秒）；不修改 schema 或余额相关代码。 |
| 文档同步 | 完成 | 在 P-30 保护清单、架构文档及本清单补充端到端 patch 约束、文件职责和测试入口。 | 前端 typecheck、目标 oxlint/oxfmt 检查、`git diff --check` 通过；未重启或部署服务。 |

## 已执行验证

| 命令或场景 | 结果 |
| --- | --- |
| `go test ./model -run 'TestSensitiveWord\|TestLogOther' -count=1` | 通过 |
| `go test ./relay -run TestPrepareRequestBilling -count=1` | 通过 |
| `(cd relaykit && go test ./dto -run sensitive -count=1)` | 通过 |
| `(cd relaykit && go vet ./dto)` | 通过 |
| `go test ./...` | 通过；包含完整 `controller` 套件 55 秒回归。 |
| `go test -race ./model -run '^TestSensitiveWordConcurrentSameRequestCountsOnce$' -count=1 -v` | 通过。 |
| `go vet ./...`、`go build ./...`、`make test` | 全部通过。 |
| `(cd relaykit && go test ./... && go vet ./... && go build ./...)` | 全部通过。 |
| `bun run i18n:sync`、`bun run typecheck`、`bun run build:check` | 全部通过。 |
| 定向 Vitest（策略、草稿搜索、用户安全、日志详情） | 5 个文件、72 项通过。 |
| `bun run test -- --maxWorkers=2` | 167 个文件、2113 项通过。 |
| 变更文件 `oxlint`、`oxfmt --check` | 全部通过；新增 8 个策略页源码文件均有版权头。 |
| 全局 `lint`、`format:check`、`copyright:check` | 失败项与 `d04c118c` 基线一致，且未包含本功能变更文件；保留为上游既有质量债务。 |
| 桌面与移动视口手工检查 | 策略页、规则弹窗/搜索、左侧用户抽屉与审计详情均无横向溢出、遮挡或缺失翻译。 |
| `bun run test -- src/features/system-settings/request-policies/__tests__/settings.test.tsx src/features/system-settings/request-policies/sensitive-words/draft-search.test.ts` | 通过，19 项 |
| 本地主线日志权限/状态定向 Vitest | 通过，6 个文件、57 项；角色降级专项包含在内 |
| 用户编辑安全字段定向 Vitest（单 worker） | 通过，2 个文件、7 项；修复前权限测试 4 项中 2 项失败 |
| 用户编辑修复后的全量 Vitest（`GOMAXPROCS=2`、单 worker、低优先级） | 通过，181 个文件、2165 项 |
| 用户编辑修复后的 `bun run typecheck`、目标 oxlint/oxfmt | 全部通过 |
| `go test -p 1 ./controller -run '^TestUpdateUserSensitiveWordFieldsRequireExplicitPayload$' -count=1 -v` | SQLite 3.50.4 通过 |
| `go test -p 1 ./controller -count=1`（`GOMAXPROCS=2`、低优先级） | 通过，52 秒 |
| 本地主线 `go test ./...`（`GOMAXPROCS=2 -p 1`） | 通过 |
| `go vet ./...`、`go build ./...`（单进程） | 通过 |
| `(cd relaykit && go test ./... && go vet ./... && go build ./...)` | 通过 |
| `bun run build:check` | 通过 |
| `bun run i18n:sync` | 通过；未保留生成器产生的无关 locale 重排/补键 |
| `bun run copyright:check` | 基线既有失败 24 个文件；本次新增 helper 已通过，未执行全仓库自动改头 |
| SQLite 新库、RC40 升级、旧版敏感词结构升级，各连续迁移两次 | 通过 |
| 本地主线历史 `sensitive_word_whitelists` 表单向导入，删除兼容表后不重建 | 待本次集成分支复验 |
| MySQL `8.2` 同三类迁移矩阵 | 通过，隔离临时数据库 |
| PostgreSQL `15` 同三类迁移矩阵 | 通过，隔离临时数据库 |
| ClickHouse `24.8.14.39` 类型 8 写入、管理员投影、普通用户脱敏 | 通过，隔离临时数据库 |
| 官方 Draft PR workflow | 等待维护者批准 fork PR 的 workflow；尚未执行。功能分支在 fork CI 中已通过后端 vet/build/test 与前端 typecheck/test。 |

实际数据库命令：

```bash
TEST_SENSITIVE_WORD_MYSQL_DSN='root:***@tcp(127.0.0.1:13306)/mysql?parseTime=true' \
TEST_SENSITIVE_WORD_POSTGRES_DSN='postgres://postgres:***@127.0.0.1:15432/postgres?sslmode=disable' \
TEST_SENSITIVE_WORD_CLICKHOUSE_DSN='clickhouse://default:***@127.0.0.1:19000/default' \
go test ./model -run 'TestSensitiveWord(DatabaseMigrationMatrix|ClickHouseLogMatrix)' -count=1 -v
```

## 最终复核结果

- [x] 执行最终 `git diff --check` 与逐项反向审查，确认无旧入口和无超出本功能范围的差异。
- [x] 重新拉取官方 `main`；发布时仍为 `d04c118c`，无需变基。
- [x] 按“模型与迁移、Relay/安全集成、前端与文档”提交，推送功能分支并创建官方 Draft PR；官方 workflow 等待维护者批准。
- [x] 2026-09-27 本地主线集成：`ba886153f` 将 RC-40 和 P-30 重实现集成到本地版本，
  `3ffc9dd4f` 合入 `main`；2026-09-28 的日志权限及用户编辑修复已补充回归测试。外部数据库、
  ClickHouse、线上部署与健康检查不因本地 Go/Web 测试通过而自动视为完成。
