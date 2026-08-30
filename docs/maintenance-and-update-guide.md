# new-api 维护与更新指南

本文档面向后续维护、排障和版本更新。代码根目录为 /opt/qlh-main/new-api；线上发布脚本位于 /opt/qlh-main/deploy；公网入口配置位于 /opt/qlh-main/caddy。

敏感词子系统的完整架构、字段和接口见 [sensitive-word-content-audit-redesign.md](./sensitive-word-content-audit-redesign.md)。任何修改该功能的提交都必须同步更新那份文档和 [sensitive-word-refactor-checklist.md](./sensitive-word-refactor-checklist.md)。

## 1. 项目边界

- controller/：HTTP/API 入口、权限校验和协议响应。
- model/：数据库模型、迁移、事务、Option 和运行时数据。
- service/：跨模块业务服务、后台任务和运行时策略。
- relay/ 与 relaykit/：协议解析、提示词提取、上游调用和错误契约；relaykit 是独立 Go 模块。
- web/src/：当前前端。不要恢复 web/classic 或 web/default。
- .github/workflows/docker-build.yml：main 推送后的 amd64、arm64 和多架构镜像构建。
- docs/：架构、兼容、排障、发布和保护清单。文档属于同一交付，不在发布后补写。

自研功能保护边界见 [custom-feature-preservation-checklist.md](./custom-feature-preservation-checklist.md)。合并上游时只选正式标签，并先完成对应上游审计记录；不得直接合并上游 main、alpha、beta、preview 或 nightly。

## 2. 敏感词与内容审计维护

### 2.1 正确的管理入口

敏感词策略页只管理策略和规则。它不显示命中记录、白名单清单、用户违规历史或完整提示词。

- 规则：系统设置中的敏感词策略页。
- 违规次数和白名单：用户管理的用户编辑右抽屉，内容安全区。
- 命中记录和完整提示词：使用日志，类型 8“关键词拦截”的详情抽屉。

不要创建第二个白名单页面或审计页面，否则同一状态会出现多个编辑入口并引入不一致。

### 2.2 运行时边界

Relay 在预扣费、计费、选渠道、上游调用和自动重试之前检查提示词。正常用户命中后返回 403；这类请求不能产生预扣费、消费日志或上游请求。

白名单命中和观察模式命中仍写一条类型 8 日志与主库审计事件，但不会增加用户违规次数。第五次有效拦截禁用用户并撤销会话。任何敏感词路径都不得修改 users.quota、充值记录、历史消费或内部余额。

规则变更必须通过敏感词管理 API 或页面进行。规则操作会立即刷新当前进程的匹配器；策略配置有五秒本地缓存，但通过页面/API 保存会立即失效。不要用直接 SQL 修改规则、配置、用户白名单或违规次数。

### 2.3 误判排查步骤

1. 在使用日志按关键词拦截类型和 request_id 查找事件。
2. 管理员打开详情，核对实际分组、规则 ID/名称、命中词、匹配片段、白名单、观察/拦截状态和当前次数。
3. 需要复核语义时，查看完整规范化提示词；普通用户日志不会也不应看到它。
4. 确认规则或绑定分组有误后先修正规则，再到用户编辑抽屉修改或清零违规次数。
5. 不要删除单条使用日志或审计事件来处理误判。历史证据和操作审计必须保留。
6. 用户第五次后仍可调用时，检查 users.status、auth_version、user_sessions 撤销状态和认证缓存。

审计详情没有完整提示词通常有两种原因：管理员关闭了“保存完整审计证据”，或证据已超过保留期被清理任务清空。清理只清空 full_prompt 和 redacted_preview，不删除审计元数据。

### 2.4 旧配置与迁移

旧 SensitiveWords Option 在第一次启动时导入独立规则表。成功后写 SensitiveWordRulesMigrationVersion=1，新的规则表成为权威来源，删除导入规则不会重新回退旧词库。

若旧 Option 含有超过新限制的词条，迁移会保留兼容匹配、不写完成标记并在系统日志提示；这不会阻断服务，但管理员必须在规则编辑器中修正旧数据。不要手工删除迁移标记或旧 Option，除非已按架构文档完成迁移复核。

### 2.5 数据库和日志库

审计事件表始终位于主数据库，支持 SQLite、MySQL 和 PostgreSQL。类型 8 使用日志仍位于 LOG_DB；当 LOG_DB 是 ClickHouse 时，日志表不需要额外列迁移，因为类型是整数。

上线前重点检查：

- MySQL：旧 sensitive_word_rules.word 列保持 VARCHAR(200) 及索引，新多词条存于子表。
- PostgreSQL：保留 group 和 key 等保留字的现有引号策略。
- SQLite：快速迁移顺序执行，避免内存 SQLite 并发建表。
- ClickHouse：确认 logs 表已有 type、request_id 和 other；类型 8 不改变表结构。

## 3. 代码与工作区检查

执行任何变更、合并或发布前：

    cd /opt/qlh-main/new-api
    git status --short --branch
    git log -5 --oneline --decorate
    git diff --check

不要用 git reset --hard 或 git checkout -- 清理不属于当前任务的改动。发现非本次变更时，先保留并判断是否影响当前工作。

敏感词改动至少执行：

    gofmt -w <changed-go-files>
    go test ./model -run 'Test(MigrateSensitiveWordData|SaveSensitiveWordConfig|SensitiveWord)' -count=1
    go test ./controller -run 'Test(SensitiveWordAuditListRedactsFullPrompt|RelaySensitiveWordBlockPrecedesBillingAndRedactsEndpointQuery|UpdateUserPersistsSensitiveWordControlsWithoutChangingQuota)' -count=1
    go test ./...
    go build ./...

    cd relaykit
    go test ./... -count=1
    go build ./...

    cd ../web
    npm run typecheck
    npm run build:check
    npm run lint
    npm test

还要检查：没有乱码或 replacement character；列表没有完整提示词；普通用户日志没有 audit_id/命中词；白名单不影响计数以外的用户属性；任何自动封禁路径不写 quota。

## 4. 标准更新流程

### 4.1 更新前检查

1. 阅读本文件、自研功能保护清单和目标正式标签的上游审计文档。
2. 确认本地 main 工作区干净，目标提交已推送。
3. 等待 GitHub Actions 的构建、镜像推送和多架构 manifest 全部成功。
4. 记录当前在线槽位、运行镜像 revision、容器状态和回滚目标。
5. 对包含数据库迁移的版本，先确认迁移幂等且旧在线槽位可继续读取新字段。

### 4.2 蓝绿发布

标准脚本位于 /opt/qlh-main/deploy：

    bash /opt/qlh-main/deploy/update-new-api-standby.sh
    bash /opt/qlh-main/deploy/switch-new-api.sh

脚本顺序不能颠倒：

1. 从 Caddyfile 读取当前在线槽位。
2. 只拉取和重建备用槽位，不能停止在线槽位。
3. 等待备用容器健康并检查其 /api/status。
4. 再次确认公网旧槽位健康。
5. 修改 Caddy 上游并验证、平滑 reload。
6. 检查公网 /api/status 和根路径 HTTP 状态。
7. 保留旧槽位作为回滚入口，观察稳定后再由运维决定停止。

备用实例可能执行共享数据库迁移。任何迁移、数据库连接或健康检查错误都意味着不能切流量。

### 4.3 敏感词功能上线后核对

切流后使用管理员 API 或页面做最小验证，不向真实用户发送违规请求：

1. 打开敏感词策略页，确认规则数量、分组多选和默认提示显示正确。
2. 创建临时本地测试用户和临时规则，在非生产流量环境验证一次观察模式日志。
3. 确认类型 8 日志详情能读取审计事件，普通用户日志不显示完整词条。
4. 确认用户抽屉能编辑违规次数和白名单，且修改后 quota 不变。
5. 删除临时规则和测试用户时保留必要操作审计。

生产环境若无法安全创建临时测试数据，只做只读检查：页面加载、路由鉴权、数据库表存在、日志详情权限和容器健康。

### 4.4 失败回滚

切流后任何公网健康检查失败，立即将 Caddyfile 恢复到切流前槽位并 reload Caddy。不要先删除、重建或停止旧槽位。

回滚后检查：

    docker ps --format '{{.Names}}	{{.Image}}	{{.Status}}'
    grep -oE 'new-api-(blue|green):3000' /opt/qlh-main/caddy/Caddyfile | head -n1
    curl -k -sS https://api.qlhazycoder.top/api/status

数据库新增字段、规则和审计数据不应在代码回滚时删除。关闭敏感词策略或改为观察模式是可逆的业务回退，优先于删除数据。

## 5. 变更记录要求

每次修改敏感词字段、规则语义、管理接口、迁移、权限、日志结构、错误文案、测试或发布流程时：

1. 同一提交更新敏感词架构文档和开发清单。
2. 记录实际验证命令和失败原因；不要把未运行的验证写成通过。
3. 代码提交、镜像发布和线上部署分别留下可追踪记录。
4. 涉及余额的代码审查必须显式确认：敏感词路径没有 quota 写操作。
