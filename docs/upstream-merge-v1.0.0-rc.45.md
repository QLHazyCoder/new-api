# QuantumNous/new-api v1.0.0-rc.45 合并审查

## 范围与基线

- 合并前本地 `main`：`0c0d1c8a05834bccab6a5f78b7eddb38c00eda27`，工作区干净。
- 最近一次官方基线：v1.0.0-rc.44，官方标签 SHA `633b4c5b190dc1af99a2814391354d86948e7943`；本地合并记录为 `5d48973b04d054bc8b11768c25a6a5d55ac2fe4c`。
- 本次官方目标：v1.0.0-rc.45，标签 SHA `6cf0d90b5ff27f901697dddd61e0c28aba8efefc`。目标通过 `upstream` 标签核对，不使用未发布提交替代。
- 合并涉及 235 个路径；冲突解决后无未合并路径、冲突标记或暂存 diff 空白错误。官方 rc.44 至 rc.45 没有修改 `.github/workflows/docker-build.yml`；本地 GHCR / Docker Hub 发布方式和 main 分支触发条件沿用本地实现。

## 项目结构与职责

- `controller/`：HTTP 路由处理、认证后请求编排与 API 响应。
- `service/`：计费、通知、任务生命周期和跨模型业务流程。
- `model/`：GORM 持久化、数据库迁移、缓存、并发任务领取及原子状态转换。
- `relay/`：上游协议适配、请求转发、费用结算和任务插件执行。
- `relaykit/`：跨 relay 使用的 DTO、协议转换器、类型和错误契约。
- `middleware/`、`common/`：认证/请求上下文、共享配置、日志、系统运行时能力。
- `pkg/perf_metrics/`、`setting/`、`dto/`：性能指标、站点配置和对外数据结构。
- `web/src/`：React 管理界面、用户设置、渠道/模型工作区和国际化资源。
- `plugins/`：内置及任务插件；本次未改动 `plugins/tasks/xai/plugin.js`。

## 新增路径职责

- `common/container_usage.go`：容器运行时资源用量采集。
- `common/system_monitor_test.go`：系统监控采集与输出测试。
- `controller/channel_billing_sub2api_test.go`：Sub2API 渠道计费控制器测试。
- `controller/channel_priority.go`：渠道优先级 API 处理。
- `model/channel_priority.go`：渠道顺序持久化和排序操作。
- `model/channel_priority_test.go`：渠道优先级持久化测试。
- `relay/channel/aws/mantle.go`：AWS Mantle 上游协议适配。
- `relay/channel/deepseek/web_search.go`：DeepSeek Search/Fetch 上游请求构造。
- `relay/channel/deepseek/adaptor_test.go`：DeepSeek 适配器测试。
- `relay/channel/vertex/adaptor_api_key_url_test.go`：Vertex API key 多区域 URL 选择测试。
- `relay/channel/vertex/url_builder_test.go`：Vertex URL 构造测试。
- `relay/deepseek_tools_handler.go`：DeepSeek 工具调用处理。
- `relaykit/dto/deepseek_tools_request.go`：DeepSeek 工具请求 DTO。
- `relaykit/relayconvert/kitutil/mask_test.go`：协议转换数据脱敏测试。
- `service/user_notify_test.go`：用户通知类型及设置测试。
- `web/src/components/__tests__/command-menu.test.tsx`：命令菜单导航过滤测试。
- `web/src/context/__tests__/theme-boot-script.test.ts`：主题启动脚本测试。
- `web/src/features/channels/components/__tests__/priority-guide.test.tsx`：渠道优先级引导测试。
- `web/src/features/channels/components/__tests__/priority-order.test.tsx`：渠道优先级排序交互测试。
- `web/src/features/channels/components/channel-priority-controls.tsx`：渠道优先级控制组件。
- `web/src/features/channels/components/channel-priority-demo.tsx`：优先级排序演示组件。
- `web/src/features/channels/components/channel-priority-drop-hint.tsx`：拖放目标提示组件。
- `web/src/features/channels/components/channel-priority-guide.tsx`：优先级交互引导组件。
- `web/src/features/channels/components/channel-priority-context.ts`：渠道优先级页面上下文。Git 相似度检测将其标记为从旧搜索上下文重命名；搜索能力实际改由 `web/src/context/search-provider.tsx` 提供。
- `web/src/features/channels/hooks/use-channel-priority-drag.ts`：渠道拖放状态及交互逻辑。
- `web/src/features/channels/hooks/use-channel-priority-order.ts`：渠道顺序变更状态。
- `web/src/features/channels/lib/__tests__/aws-channel.test.ts`：AWS 渠道配置测试。
- `web/src/features/channels/lib/__tests__/priority-preview.test.ts`：渠道优先级预览测试。
- `web/src/features/channels/lib/__tests__/vertex-channel.test.ts`：Vertex 渠道配置测试。
- `web/src/features/channels/lib/channel-priority-order.ts`：优先级排序和变更辅助逻辑。
- `web/src/features/playground/lib/message/__tests__/message-streaming.test.ts`：Playground 消息流式合并测试。
- `web/src/features/pricing/__tests__/sorting.test.ts`：模型定价排序测试。
- `web/src/features/system-settings/integrations/__tests__/email-notifications.test.tsx`：邮件通知设置测试。
- `web/src/lib/__tests__/model-provider.test.ts`：模型供应商分类测试。
- `web/src/lib/__tests__/theme-color.test.tsx`：主题颜色测试。
- `web/src/lib/theme-color.ts`：主题颜色解析和转换逻辑。

## 自开发功能审查结论

- Playground 图片任务：保留本地数据库任务/批次状态、并发领取、租约心跳、失败/中断恢复、结果保留和引用文件清理调用链（`controller/playground_image_task.go`、`service/playground_image_worker.go`、`model/playground_image.go`、`model/system_task.go`）。rc.45 对普通 Playground 图片请求的路由处理采用官方更新；官方普通请求路径没有取代本地持久化队列。相关控制器、模型、服务测试由 Go 全量测试执行。
- 计费与额度：保留本地 legacy billing reservation/settlement 事务和 request/phase 幂等模型（`service/quota.go`、`model/billing_operation.go` 等），将 rc.45 的结算入口接入既有交易路径。SQLite billing 数据库矩阵通过；MySQL/Postgres 矩阵因缺少隔离 DSN 未执行。长时间 reservation 仍只表示账务待核对，不在此执行清理或退款。
- 性能指标：采用 rc.45 在指标时间窗和模型历史取数方面的实现，保留其余当前指标能力；测试覆盖更新后的历史窗口行为。
- 用户/分组设置：官方新增通知选项与更新语义优于重复维护的旧路径，采用官方实现；本地 `record_ip_log` 默认行为通过可选字段保留。渠道模型排序/优先级采用官方新增实现及相应 Web 组件。
- 推广额度换算：保留本地 int64 quota、原始精度字段及 affiliate quota 转换；请求额度经既有 `common.Int64Value` 表达，不引入超出官方规则的额外校验。
- 额度告警：采用 rc.45 的全局开关、用户开关、阈值触发及通知目的地更新语义；本地 int64 额度存储/展示精度继续保留。阈值负值处理仅使用官方已有约束。
- 敏感词审计：继续使用本地 `middleware/sensitive_word.go` 等审核链路；不恢复已移除的 `setting/sensitive.go` 旧入口，也不恢复旧保护清单。
- 部署工作流：rc.45 相对 rc.44 未改工作流，因此保留当前仓库按 main 提交构建 GHCR 镜像的本地契约。

## 验证记录

- `GOWORK=off go test ./...`：通过，包括 controller、model、service、relay 与 router。
- `relaykit/`、`tokenkit/`：各自 `GOWORK=off go test ./...` 通过。
- `GOWORK=off go build ./...`：通过。
- `web/ bun run typecheck`：通过。修复了一处合并重复声明的 `quota_remind_threshold` 类型字段，保留原字段并增加官方开关字段。
- `web/ bun run lint`：退出码 0；有仓库已有的静态警告。
- `web/ bun run test`：217 个测试文件、2436 个测试通过。
- `web/ bun run build`：通过，生产构建约 27 秒。
- `web/ bun run format:check`：通过；仅格式化本次更新的 group-settings 测试文件。
- `GOWORK=off go test ./model -run TestBillingOperationDatabaseMatrix -count=1 -v`：SQLite、MySQL 8.2、PostgreSQL 15 均通过。MySQL 与 PostgreSQL 分别用 loopback 临时容器执行，覆盖升级、重复迁移、重复 reservation 幂等、退款及结算回滚；测试库和容器已清理。
- `controller` token migration compatibility：SQLite、MySQL 8.2、PostgreSQL 15 的旧表升级均通过；每个方言重复执行迁移，确认新列、既有数据和长 token key 仍可读写。MySQL/PostgreSQL 使用独立临时容器，容器已在测试后删除。
- `web/ bun run copyright:check`：全仓检查失败，列出 28 个需要补齐/更新版权头的文件，其中包括与本次差异无关的现存文件；未扩大提交范围。
- `web/ bun run lint:plugins` 与 `web/ bun run format:plugins:check`：因未修改的 `plugins/tasks/xai/plugin.js` 失败（catch 参数 `_error` 未使用、文件格式不符）；该文件相对合并前 `main` 无差异，未作顺手修改。

## 阶段状态

- 基线与官方标签：完成。
- 自开发功能调用链核对及冲突适配：完成。
- Go/Web 测试、构建和差异检查：完成；上文列出的数据库与全仓辅助检查仍有跳过/失败项。
- GitHub Actions、镜像 revision、实例部署及部署后持续检查：由本次发布交付记录提供最终证据；本文件不将未执行的运行检查记为通过。
