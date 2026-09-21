# 敏感词与内容审计重构开发清单

本文档是自研功能清单和阶段审查记录，必须与 sensitive-word-content-audit-redesign.md 同步。当前工作分支为 main；用户编辑面板固定为左抽屉。

## 0. 项目整体分析

- [x] 梳理 Relay 请求解析、实际分组、token 估算、预扣费、渠道重试和上游调用顺序。
- [x] 确认局部规则分组只能来自 ratio_setting.GetGroupRatioCopy()，auto 只作为候选路由输入，不作为可绑定分组。
- [x] 确认审计详情在主库，使用日志可独立写入 ClickHouse，列表不返回完整提示词。
- [x] 确认余额、钱包、used_quota、历史账务不是敏感词处罚对象。
- [x] 对照 sub2api 的结构化动作、脱敏摘要、规则快照和管理员详情思路，但不引入外部审核供应商。

## 1. 数据模型和迁移

- [x] 新增策略表、规则表、规则词条表、规则分组表和主库审计事件表。
- [x] User 增加 sensitive_word_violation_count 和 sensitive_word_whitelist。
- [x] 正常迁移和快速迁移均纳入新表和用户字段。
- [x] 旧 SensitiveWords/SensitiveWordConfig Option 只做一次性导入，运行时不再读取旧 Option。
- [x] 旧独立白名单表只在迁移时同步用户字段，运行时唯一来源为 User 字段。
- [x] 迁移异常只停用敏感词功能并记录日志，不阻断主服务启动。
- [x] MySQL full_prompt 使用 MEDIUMTEXT，PostgreSQL/SQLite 使用 TEXT，并执行 UTF-8 字节安全截断。
- [x] 迁移使用版本标记，重复执行不会重复导入规则。

阶段自检结果：新建表、用户字段、旧数据导入、失败开放和审计主库边界已覆盖模型测试；不再保留旧敏感词运行时 fallback。

## 2. 规则和运行时快照

- [x] 规则动作收敛为 block、observe、off，移除全局处理模式。
- [x] 全局与局部规则使用同一张规则表展示和管理。
- [x] 局部规则校验真实定价分组，拒绝 auto、空分组和未知分组。
- [x] Aho-Corasick 快照按规则版本构建，保存、编辑、模式切换和删除后立即失效。
- [x] 同一请求命中多规则/多词条只计一次。
- [x] 同一请求同时命中 block 和 observe 时 block 优先。
- [x] auto 候选分组在计费前一次性检查，重试不能绕过局部规则。
- [x] 规则弹窗保留批量文本/TXT 导入、去重、空行和超长统计。
- [x] 规则弹窗搜索仅定位当前未保存草稿，不改写保存 payload。

阶段自检结果：全局、局部、模式优先、候选分组、运行时刷新、搜索定位均有定向测试或代码路径验证。

## 3. 用户违规、白名单和封禁

- [x] 白名单用户写类型 8 日志和审计，不拦截、不计数、不封禁。
- [x] observe 命中只记录，不拦截、不计数、不封禁。
- [x] block 命中在用户行锁事务内原子增加次数。
- [x] 阈值默认 50，达到 count >= threshold 时自动禁用普通用户。
- [x] 第一次达到阈值的请求仍返回配置提示；后续请求才返回 user_banned 403。
- [x] 封禁只修改状态、auth_version、缓存和会话，不修改 quota、used_quota、钱包、订阅余额或历史消费。
- [x] 用户编辑支持直接修改/清零违规次数和切换个人白名单。
- [x] 管理员启用账号在同一事务内清零当前次数，历史证据、余额和白名单不变。
- [x] 禁用转启用才刷新 auth_version/缓存/会话；重复启用保持幂等。
- [x] 启用操作记录 sensitive_word.enable_reset 和 balance_changed=false。
- [x] 用户列表启用前在次数大于 0 时显示确认框。

阶段自检结果：第 1、4、5、6 次、阈值后启用、并发启用/命中、余额不变、白名单保持和重复启用测试已覆盖。

## 4. 审计和使用日志

- [x] 固定日志类型 8：关键词拦截。
- [x] action 区分 blocked、observe、whitelist_bypass。
- [x] 日志摘要通过 request_id 和 audit_id 关联主库审计。
- [x] 管理员详情可读取完整规范化提示词、命中规则、词条、片段、哈希、状态变化和证据保留状态。
- [x] 列表和普通用户视图移除 audit_id、规则、命中词、哈希和完整提示词。
- [x] retain_full_prompt 关闭时不保存正文、预览和片段。
- [x] 保留期清理只清空正文/预览，不删除事件元数据。
- [x] observe 审计落库失败只记录降级并放行；block 或其他数据库故障返回不可重试 503。
- [x] 审计详情路由统一为 /api/log/sensitive-word-audit/:id，不新增独立敏感词审计列表页。

阶段自检结果：管理员详情脱敏、类型 8、审计失败语义和跨数据库字段测试已覆盖。

## 5. 请求和错误边界

- [x] 敏感词检查位于 token 估算、预扣费、计费、渠道、上游和重试之前。
- [x] 拦截返回 HTTP 422、错误码 sensitive_words_detected、不可重试且不写普通错误日志。
- [x] 后续已封禁账号返回 HTTP 403、错误码 user_banned。
- [x] OpenAI、Responses、Claude、Gemini、图片文本请求共用提示词快照入口。
- [x] Relay 错误包装不覆盖管理员配置的敏感词文案。
- [x] 请求级审计幂等检查避免 WebSocket/重试重复计数和重复日志。
- [x] 拦截请求不产生预扣费、正常消费日志或上游访问。

阶段自检结果：relay、controller、relaykit 错误契约和计费前检查已通过定向测试；通用 TokenAuth 封禁响应补充 user_banned。

## 6. 管理页面

- [x] 敏感词页只保留策略设置、统一规则表和规则弹窗。
- [x] 规则表右侧操作列支持编辑、删除和 block/observe/off 模式切换。
- [x] 规则弹窗固定四个区域：名称/范围、分组、词条、处理模式。
- [x] 词条搜索框位于 TXT 导入按钮左侧，支持实时普通包含匹配、首个定位、Enter/Shift+Enter 循环和清空。
- [x] 敏感词页不显示审计列表、白名单列表或命中统计。
- [x] 用户列表显示违规次数和白名单状态。
- [x] 用户编辑面板使用左抽屉 side="left"，内容安全区包含次数输入、清零和白名单开关。
- [x] 使用日志提供关键词拦截类型和管理员详情延迟加载。
- [x] 新增中简、繁中、英文文案，检查无乱码和 replacement character。

阶段自检结果：敏感词定向 Vitest、前端类型检查、lint 和构建均需在本轮最终门禁再次执行。

## 7. 接口和旧逻辑收缩

- [x] 注册 policy、groups、rules、rule detail、rule mode 接口并加 AdminAuth。
- [x] 注册日志审计详情接口并加 AdminAuth。
- [x] 删除旧 config、stats、whitelist、audits、clear-violations、unban 运行接口。
- [x] 删除旧 service/sensitive.go 和 setting.SensitiveWords 运行时读取。
- [x] 删除前端旧全局敏感词字段和旧 Request Policies 入口。
- [x] 规则 mode 成为唯一规则动作来源，保留旧数据库列只用于迁移窗口。

阶段自检结果：代码检索不应再出现旧接口调用、旧全局 matcher、旧抽屉位置说明或旧敏感词页面数据区。

## 8. 文档和发布记录

- [x] 更新架构、接口、数据迁移、排障和文件职责文档。
- [x] 更新本清单，记录左抽屉、阈值 50、422/403 分离、余额不变和规则级 mode。
- [x] 更新 maintenance-and-update-guide.md 的运行时排障和发布门禁。
- [x] 更新 custom-feature-preservation-checklist.md 的 P-30，避免上游合并恢复旧逻辑。
- [x] 本轮最终测试结果写入本节并提交到 main。
- [x] 提交前确认只暂存本次敏感词重构相关文件，不覆盖用户已有无关修改。

## 9. 最终门禁

- [x] gofmt 和 git diff --check。
- [x] go test ./model -run 'SensitiveWord' -count=1。
- [x] go test ./controller -run 'SensitiveWord|RelaySensitive' -count=1。
- [x] go test ./router -count=1。
- [x] go test ./... 和 go build ./...。
- [x] relaykit 独立测试和构建。
- [x] web typecheck、定向 Vitest、lint、生产构建。
- [x] 检查新增文案、字段、接口、文件和文档无乱码、无旧接口引用。
- [x] 提交到 main 并核对远端 main 指向。

## 10. 阶段审查记录

- 2026-09-21：按合并后的最新代码核对敏感词路由、旧 fallback、规则 mode、观察模式 503、422/403 边界和日志详情入口。
- 2026-09-21：确认用户编辑面板为左抽屉；补充通用 TokenAuth 的 user_banned 错误码；修正用户字段白名单测试。
- 2026-09-21：重写架构说明和本清单，删除阈值 5、旧 config/audits/unban、全局 mode、独立白名单列表和旧抽屉方向等过期描述。
- 2026-09-21：补充一次性迁移标记短路，防止删除已导入规则后旧 Option 在后续启动重新复活；空请求 ID 统一生成后再写审计和使用日志，保证两者可关联。
- 2026-09-21：完成 Go 全量测试/构建、relaykit 测试/构建、前端类型检查、11 项敏感词定向测试、lint 和生产构建；lint 仅有仓库既有 warning。
