# new-api 维护与更新指南

本文档面向后续维护、排障和版本更新。所有路径均以 `/opt/qlh-main/new-api` 为代码根目录；线上部署文件位于 `/opt/qlh-main/deploy`，公网入口位于 `/opt/qlh-main/caddy`。

## 1. 项目边界

- `controller/`：HTTP/API 入口和权限校验。
- `model/`：数据库模型、迁移、配置和事务逻辑。
- `service/`：跨模块业务服务、后台任务和运行时策略。
- `relay/`：OpenAI、Claude、Gemini 等协议解析、模型路由和上游调用。
- `web/src/`：当前唯一前端，不要恢复 `web/classic` 或 `web/default`。
- `.github/workflows/docker-build.yml`：推送 `main` 后构建并发布 amd64、arm64 和多架构 GHCR 镜像。

自研功能保护边界见 [custom-feature-preservation-checklist.md](./custom-feature-preservation-checklist.md)。合并上游正式标签前，必须先阅读该清单和对应的 `upstream-merge-*.md` 审计记录。

## 2. 敏感词与内容审计

### 2.1 代码入口

请求统一在 [controller/relay.go](../controller/relay.go) 的预扣费、计费和上游请求之前调用 `model.CheckSensitiveRequest`。命中后：

1. 从规范化提示词生成哈希、脱敏摘要和长度受限的完整提示词。
2. 匹配全局规则及当前分组的局部规则；旧 `setting.SensitiveWords` 仅作为迁移期回退。
3. 写入主数据库审计事件，并写入使用日志类型 8“关键词拦截”。
4. 白名单用户记录但继续请求，不增加违规次数。
5. 非白名单用户按用户行锁累计历史命中；达到配置阈值时封禁、清零主余额、递增 `auth_version` 并失效缓存。

### 2.2 数据表

| 表 | 作用 |
| --- | --- |
| `sensitive_word_rules` | 全局/局部敏感词规则 |
| `sensitive_word_rule_groups` | 局部规则与分组定价名称的绑定 |
| `sensitive_word_user_whitelists` | 按用户 ID 的白名单 |
| `sensitive_word_audit_events` | 误判复核的唯一证据来源 |
| `options` 的 `SensitiveWordConfig` | 开关、提示、封禁阈值和保留策略 |

审计列表接口不会返回 `full_prompt`；完整提示词只允许管理员详情接口读取。保留任务会清空过期提示词和摘要，但保留命中计数与审计元数据。

### 2.3 管理接口

统一前缀：`/api/sensitive-words`，全部要求管理员权限。

- `GET/PUT /config`：读取或保存策略配置。
- `GET /stats`：规则、白名单和今日命中统计。
- `GET/POST/PUT/DELETE /rules`：规则管理。
- `GET /groups`：只返回分组定价中的候选分组。
- `GET/POST/DELETE /whitelist`：用户白名单管理。
- `GET /audits`、`GET /audits/:id`：审计列表和管理员详情。
- `POST /users/:id/clear-violations`：清理违规记录，必须明确操作。
- `POST /users/:id/unban`：解封并刷新认证版本。

排查误判时，先用请求 ID或用户 ID筛选列表，再查看管理员详情中的完整提示词、命中词、规则版本、白名单状态和封禁结果。不要把完整提示词写入普通日志或聊天记录。

## 3. 常用排障路径

### 3.1 代码与工作区

```bash
git status --short --branch
git log -5 --oneline --decorate
git diff --check
```

确认没有未授权改动后，再进行构建、合并或发布。不要使用 `git reset --hard`、`git checkout --` 清理用户改动。

### 3.2 线上实例

```bash
docker ps --format '{{.Names}}\t{{.Image}}\t{{.Status}}'
grep -oE 'new-api-(blue|green):3000' /opt/qlh-main/caddy/Caddyfile | head -n1
curl -k -sS https://api.qlhazycoder.top/api/status
```

当前公网流量槽位必须从 `Caddyfile` 实时读取。`new-api-blue` 和 `new-api-green` 共用 MySQL、Redis 和 `/data/new-api`，不要同时进行不兼容的数据库迁移。

### 3.3 敏感词问题定位

按以下顺序查找：

1. 管理页面规则是否启用、范围是否正确、局部分组是否仍存在于分组定价。
2. `request_id` 是否同时出现在审计表和使用日志 `Other.audit_id`。
3. 审计事件的 `whitelist_bypassed`、`blocked`、`violation_count` 和 `rule_version`。
4. 用户状态、`quota`、`auth_version` 和认证缓存是否一致。
5. Relay 日志是否在预扣费前返回 `SensitiveWordsDetected`。

如果是误判，不要直接删除单条证据；由管理员审核后使用清理违规记录接口，并保留管理审计日志。

## 4. 标准更新流程

### 4.1 更新前检查

1. 阅读本文件、[custom-feature-preservation-checklist.md](./custom-feature-preservation-checklist.md) 和目标上游正式标签审计文档。
2. 只选择正式发布标签；不得直接合并上游 `main`、alpha、beta、preview 或 nightly。
3. 确认本地 `main` 工作区干净、目标提交已推送，且 GitHub Actions 的构建、镜像推送和多架构 manifest 全部成功。
4. 记录当前公网状态、在线槽位、镜像 revision 和回滚目标。

### 4.2 蓝绿发布

线上标准脚本位于 `/opt/qlh-main/deploy`：

```bash
bash /opt/qlh-main/deploy/update-new-api-standby.sh
bash /opt/qlh-main/deploy/switch-new-api.sh
```

脚本顺序不可颠倒：

1. 从 `Caddyfile` 判断在线槽位。
2. 只拉取和重建备用槽位，不停止在线槽位。
3. 等待备用容器健康，并检查其 `/api/status`。
4. 再次确认公网旧槽位健康。
5. 修改 Caddy 上游并验证、平滑 reload。
6. 立即检查公网 `/api/status` 和根路径 HTTP 状态。
7. 保留旧槽位作为回滚入口，观察稳定后再按运维决定停止。

备用实例启动可能执行共享数据库迁移。新增表或字段必须确认迁移幂等且向后兼容后才能继续；发现启动日志有迁移、数据库或健康检查错误时，不切流量。

### 4.3 失败回滚

切流后任一公网健康检查失败，立即把 `Caddyfile` 恢复到切流前槽位并 reload Caddy；不要先重建或删除旧槽位。回滚后再次检查公网 `/api/status`、根路径和两个容器健康状态，并保留失败日志。

## 5. 验收门禁

代码提交前至少执行：

```bash
gofmt -w <changed-go-files>
go build ./...
go test ./model -run 'Test(NormalizeSensitiveWord|TruncateSensitivePrompt)$'
git diff --check
```

前端变更执行：

```bash
cd web
npm run build
```

还要检查：

- 文本按 UTF-8 解码，无 Unicode replacement character 或乱码。
- 没有真实冲突标记、无意删除保护功能、无 `web/classic`/`web/default` 回流。
- 列表接口不返回完整提示词，白名单不泄露用户密码字段。
- 更新 `main` 后 GitHub Actions 的两个架构构建、manifest、签名和镜像标签全部成功。

全量测试若因已有 SQLite 迁移测试夹具并发建表失败，必须在提交文档中记录具体失败表和测试名，不得把它误报为敏感词逻辑通过。

## 6. 变更记录要求

每次新增敏感词字段、规则语义、管理接口、迁移、排障结论或发布流程，都要在同一提交更新本文件，并同步更新保护清单或对应的上游合并审计文档。提交信息应说明是功能、修复、文档还是上游合并，避免把部署操作和代码合并混在一个不可追踪的提交中。
