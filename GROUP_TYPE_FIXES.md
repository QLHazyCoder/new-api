# 分组类型一致性修复记录

## 项目整体分析

本次修复针对 `4da7a937b feat(groups): add group type routing and management`。类型用于令牌创建与编辑时的选组分类；请求路由、分组权限、实际计费仍使用原有计费分组与倍率配置。

已复现：类型定义与映射部分保存、残留映射重新生成已删除分组及倍率 `1`、`__auto__` 类型 ID 与自动路由入口冲突、未分类统计误计 `auto`。生产表单集成测试另复现了保存失败后草稿被设置刷新覆盖、失败 Promise 未处理的问题。

## 修复方案

- `PUT /api/option/group-types` 在原有 RootAuth 组内提供类型定义与映射的成对保存，沿用 `option:write` access-token scope。
- 请求体的 `group_types`、`group_type_map` 均为 JSON 字符串，必须同时提供。后端拒绝空对象以外的非对象结构、保留 ID、未修剪的标识、未知类型、无基础倍率的分组和对 `auto` 的类型分配。
- 两项配置在同一数据库事务中写入；提交成功后成对更新内存映射。数据库重载与更新串行执行，读选项接口按同一快照读取类型与映射。
- 单字段 option 更新和不完整 bulk 更新均拒绝写入这两个强关联字段。原有倍率等其他设置继续使用原接口。
- 无效的历史类型配置不替换最后有效的运行态快照；管理员仍可从 option 列表读到原始值并修正。缺失配置按空对象处理。
- 类型映射不能创建分组；保存时过滤已删除分组的残留映射。`auto` 保留系统入口，不进入未分类统计或普通类型分配。
- 设置刷新保留尚未保存的脏字段并更新其他字段；成对保存失败后保留草稿、显示现有错误提示并允许重试。

## 文件结构与职责

| 路径 | 调整与职责 |
| --- | --- |
| `setting/ratio_setting/group_ratio.go` | 类型及映射校验、保留 ID、成对运行态发布与读取。 |
| `model/option.go` | 事务持久化、禁止部分写入、数据库重载协调。 |
| `controller/option.go` | 专用请求结构及处理器，通用接口拒绝部分写入。 |
| `router/api-router.go` | RootAuth 内注册专用路由。 |
| `middleware/access_token_routes.go` | 登记新路由的 `option:write` scope。 |
| `service/group.go` | 用户可选分组从同一类型快照组装，不使用保留类型 ID。 |
| `web/src/lib/group-types.ts` | 新增前端共享保留类型 ID。 |
| `web/src/features/keys/lib/api-key-group-types.ts` | 过滤旧数据中的保留类型 ID。 |
| `web/src/features/keys/components/api-keys-mutate-drawer.tsx` | 选组界面处理保留类型 ID。 |
| `web/src/features/system-settings/api.ts` | 专用成对保存 API 客户端。 |
| `web/src/features/system-settings/types.ts` | 成对保存请求契约。 |
| `web/src/features/system-settings/hooks/use-update-option.ts` | 成对保存 mutation 及缓存刷新。 |
| `web/src/features/system-settings/models/ratio-settings-card.tsx` | 类型 JSON 校验、单请求保存、残留映射清理和失败草稿保留。 |
| `web/src/features/system-settings/models/group-ratio-form.tsx` | 复用现有 `FieldSet`，保存期间阻止新编辑被提交后重置覆盖。 |
| `web/src/features/system-settings/models/group-type.ts` | 类型解析与映射序列化，不接纳自动路由分配或失效引用。 |
| `web/src/features/system-settings/models/group-ratio-visual-editor.tsx` | 目录不再从映射生成分组；自动路由不参与类型分配与未分类提示。 |
| `web/src/components/data-table/static/static-data-table.tsx` | 为静态表格提供可选列组，让宽屏表格固定窄列并把剩余宽度交给内容列。 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json` | 同步后台校验提示的七种语言，参数使用结构化 message。 |
| `model/option_test.go` | 持久化、重载、历史错误配置恢复、单字段拒绝及 SQL 写入故障回滚测试。 |
| `controller/billing_option_test.go` | API 拒绝路径及真实 SQLite/MySQL/PostgreSQL 事务矩阵。 |
| `service/group_test.go` | 类型元数据与输入边界测试。 |
| `router/access_token_scope_test.go` | 新路由 scope 契约及路由覆盖检查。 |
| `web/src/features/system-settings/models/__tests__/group-settings.test.tsx` | 残留组不复活、自动路由不误报等可视化回归。 |
| `web/src/features/system-settings/models/__tests__/group-type.test.tsx` | 类型辅助逻辑及生产设置表单的失败、刷新、重试集成测试。 |
| `web/src/features/keys/components/__tests__/api-keys-mutate-drawer.test.tsx` | 保留类型 ID 的令牌选组兼容回归。 |
| `GROUP_TYPE_FIXES.md` | 本次方案、范围、验证与审查记录。 |

## 开发清单与阶段自检

| 阶段 | 状态 | 发现与处理 |
| --- | --- | --- |
| 全链路分析与复现 | 完成 | 先运行失败用例，确认残留分组、自动路由统计和保留 ID 行为。 |
| 数据库与运行态一致性 | 完成 | 成对事务、错误返回、运行态快照及同步互斥；禁止部分写入。 |
| 前端与兼容边界 | 完成 | 单请求、保留 ID、残留引用清理、失败草稿保留。 |
| 分组定价表布局 | 完成 | 复现宽屏下类型列吸收剩余宽度、与倍率列之间出现大空白；改为固定列组，组名和说明列吸收剩余空间。 |
| 权限与翻译 | 完成 | 补齐新路由 scope 登记，复用 RootAuth 和审计中间件；新增校验消息使用 `common.NewMessage`。 |
| 最终验证 | 完成 | 下列测试、真实三数据库事务、全量 Go 测试、类型检查、lint 与生产构建均通过。 |

## 验证记录

| 验证 | 命令或范围 | 结果 |
| --- | --- | --- |
| 后端全量测试 | `go test -p 2 ./...` | 通过，包含新增路由覆盖与大小写分组兼容用例。 |
| 后端静态检查 | `go vet -p 1 ./...` | 通过。 |
| 三数据库事务矩阵 | 设置隔离实例的 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN` 后执行 `go test -p 1 ./controller -run '^TestGroupTypeOptionsDatabaseMatrix$' -count=1 -v` | SQLite 3.50.4、MySQL 8.2.0、PostgreSQL 18.4 均通过；创建、替换、第二项写入失败回滚和未知映射拒绝均已覆盖。 |
| 持久化与重载 | `go test -p 1 ./model -run 'TestGroupTypeOptionsPersistAndReload\|TestUpdateGroupTypeOptionsRollsBackBothOptionsOnDatabaseFailure' -count=1` | 通过；包含真实 SQL 故障回滚、重复重载、保留最后有效运行态配置和原始错误值可修复。 |
| 前端回归 | `bun run test --maxWorkers=1` 指定分组设置、类型设置、分组倍率精度、自动路由上限和令牌抽屉五个测试文件 | 31 项通过；末轮保存期间编辑保护调整后，关联两个表单测试文件的 19 项再次通过。 |
| 分组定价布局回归 | `bun run test --maxWorkers=1 src/features/system-settings/models/__tests__/group-settings.test.tsx` | 15 项通过；新增固定列组契约，先验证旧实现失败，再验证修复通过。 |
| TypeScript | `bun run typecheck` | 通过。 |
| 前端静态检查 | `bunx oxlint -c .oxlintrc.json` 指定本次 TypeScript/TSX 变更文件 | 通过。 |
| 前端格式与构建 | `bunx oxfmt -c .oxfmtrc.json --check` 相关文件；`RAYON_NUM_THREADS=2 bun run build --dist-path /tmp/newapi-group-layout-build.XXXX` | 格式检查通过，生产构建通过。 |
| 生产构建 | `RAYON_NUM_THREADS=2 bun run build --dist-path /tmp/newapi-group-type-final-build.upr4T7` | 通过；构建输出与当前业务使用的资源目录隔离。 |
| 文本与差异 | `git diff --check`、UTF-8/控制字符扫描、七种语言的消息键与插值参数核对 | 通过。 |

MySQL 与 PostgreSQL 运行在本次临时创建的 loopback 容器内，矩阵使用独立测试数据库；数据库及临时容器均已清理。测试密码不是业务凭据，不写入项目配置。本次修复涉及的代码已按文件指纹核对，最终测试期间未发生变动。

## 分组定价表布局根因与修复

截图中的大间距来自浏览器的自动表格布局：类型列只有 `min-w-44`，但没有列宽上限；在宽表格中，`table-layout: auto` 会把剩余宽度分配给这类未明确指定宽度的列。类型选择器本身仍是 `w-44`，所以控件右侧留下了列内空白，倍率列被推远。

`StaticDataTable` 现在支持可选 `colgroup`。分组定价表启用 `table-fixed`，把类型、倍率、充值倍率、用户可选和操作列固定为控件需要的宽度，只让组名与说明列平分剩余空间。这样列边界和输入控件宽度保持一致，宽屏不会再出现类型列到倍率列之间的异常空白；小屏仍由表格容器横向滚动承载。

## 最终审查结论

已修复四项已复现缺陷，以及深查中复现的保存失败草稿丢失与未处理 Promise 问题；本次又修复了分组定价表在宽屏下的列间距缺陷。范围内未发现尚未处理的明确回归；旧令牌、自动路由和原有分组权限保持现有契约。`auto` 是独立系统入口，其他分组名继续按大小写区分。

类型配置的外部写入方需要使用 `/api/option/group-types` 成套提交，旧单字段写入会明确失败。此事务只覆盖类型定义与映射，原有倍率等独立设置仍使用既有 API。

此前分组修复已提交为 `0bab0308b`。以下为首次候选发布时补充的真实旧配置兼容审查；旧阶段的本机构建/测试不能替代该项启动验证。

## 2026-10-24 候选启动：旧 auto 分类绑定兼容

候选 `5a6799792` 启动后健康检查正常，但日志每分钟报告 `Automatic routing cannot be assigned a group type`。只读核对确认生产旧 `group_type_map` 保存了 `auto` 分类绑定；这是独立系统路由项的旧冗余数据。原冷启动会因此拒绝整对类型配置，使其余有效类型也未进入运行态。

新增读取兼容只在 `loadOptionsFromDatabase` 中移除这条旧 `auto` 绑定，然后用既有严格校验成对发布真实分组映射。其他定义、分组、类型 ID、倍率与自动路由顺序保持原值；生产数据库不改写，旧版本仍可读取原始配置。新写入继续明确拒绝 `auto` 类型分配；其他无效历史引用仍保留上个有效快照并暴露错误，不能被该兼容隐藏。

文件职责：`model/option.go` 增加旧映射读取规范化；`model/option_test.go` 复用真实三数据库测试基础设施，覆盖冷启动、两次重载、中文名称、保留真实分类、数据库原值不变和新写入严格拒绝。数据库表结构没有变化。

失败用例先在旧实现运行：SQLite 冷启动报同一错误，预期类型名“标准类型”实际为空；保持旧蓝槽服务，候选绿槽正常停止，未切换公网流量。

修复后 `TestGroupTypeOptionsReloadsLegacyAutomaticRoutingAssignment` 在 SQLite 3.50.4、MySQL 8.2.0、PostgreSQL 18.4 的隔离测试库均通过。两次冷态重载保留“标准类型”和两个真实组映射，原数据库 JSON 不变，新写入旧 auto 绑定仍被拒绝；既有缺失类型错误保留最后有效快照、成对保存和数据库故障回滚用例也通过。临时库已清理，未修改生产分组配置。补丁需重新经 Actions 与候选启动/周期重载检查后才切流。
