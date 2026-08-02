# 上游合并审计：v1.0.0-rc.23

## 范围与基线

- 上游来源：`https://github.com/QuantumNous/new-api.git`。
- 合并对象：正式标签 `v1.0.0-rc.23`，目标提交
  `0ab02020603d22e5613bc4cf46bfab06f8567769`。
- 本地第一父提交：`88ff1c7cd9768d12de4be84e0c5a994cd7029277`。
- `v1.0.0-rc.22` 是该标签的祖先；本次纳入其后的 32 个正式上游提交，未合并任意
  上游 `main` 的额外提交。
- 本次没有直接执行生产 MySQL、PostgreSQL 或 SQLite 数据修改。现有数据兼容由
  `model/main.go` 的迁移、`seedRequiredOptions` 的冲突忽略写入和既有表结构负责；
  对已存在的 `PlaygroundImageMaxConcurrency`、账本、订阅和用户记录不覆盖写入。

## 上游结构迁移

| 原路径 | 最终路径 | 处理结论 |
| --- | --- | --- |
| `dto` 的协议对象 | `relaykit/dto` | 上游模块化迁移；根 `dto` 仅保留任务、Midjourney、视频及图片能力展示 DTO。 |
| `types` 的 relay 对象 | `relaykit/types` | 上游模块化迁移；宿主计费/定价类型继续留在根 `types`。 |
| `service/relayconvert` | `relaykit/relayconvert` | 上游模块化迁移；宿主级 Chat-to-Responses 策略改由 `service/openai_chat_responses_mode.go` 持有。 |
| `relay/reasonmap` | `relaykit/reasonmap` | 上游模块化迁移，无本地行为删除。 |

所有上述删除均有对应迁移路径；未以整文件 `ours` 或 `theirs` 解决冲突。

## 冲突决议

| 路径 | 上游增量 | 本地保留或融合内容 |
| --- | --- | --- |
| `relay/channel/gemini/adaptor.go` | RelayKit DTO 与转换入口 | 保留多供应商图片能力、Gemini 原生图片/Imagen 分流、分辨率别名和规格校验，并迁移为 RelayKit DTO。 |
| `relay/chat_completions_via_responses.go` | 重新加入 Chat 转 Responses 执行路径 | 保留公共 Chat 不自动改写协议；开关在根服务层固定禁用，遗留函数仅作为受保护回归边界。 |
| `relay/helper/model_mapped.go` | DTO 路径迁移 | 保留共享模型映射解析、链式映射、自映射和循环检测语义。 |
| `service/group.go` | 上游自动分组 | 同时保留分组描述、特殊可见分组规则、用户自身分组可见性，并接入上游自动分组过滤与去重。 |
| `setting/model_setting/gemini.go` | Gemini 安全设置校验 | 保留图片模型大小后缀和 Flash Lite 图片模型识别，并加入上游安全设置验证。 |
| `setting/model_setting/gemini_test.go` | 安全设置测试 | 保留图片模型回归断言，合并上游安全设置测试。 |
| `web/src/features/keys/components/api-keys-mutate-drawer.tsx` | API Key 自动分组编辑器 | 保留表单状态完整更新；切入 `auto` 时启用跨分组重试，离开时同步关闭。 |
| `web/src/features/keys/lib/api-key-form.ts` | 类型导入排序 | 仅合并等价导入，无行为变化。 |
| `web/src/features/system-settings/hooks/use-update-option.ts` | OIDC 显示名状态刷新 | 保留性能指标开关的即时状态缓存更新，并加入 OIDC 状态失效。 |
| `service/relayconvert/request_compat.go` | 上游删除旧宿主转换包 | 删除已迁移的旧实现；禁用策略迁移到根服务并增加对应测试。 |
| `relaykit/relayconvert/policy_test.go` | 独立 RelayKit 模块 | 删除错误依赖宿主设置的迁移中测试，改在根 `service` 模块验证策略。 |

## 自研功能保护矩阵

| 编号 | 状态 | 最终位置和检查结论 |
| --- | --- | --- |
| P-01 | 原样保留 | `.github/workflows/docker-build.yml` 仍为 fork 的 GHCR 多架构发布工作流。 |
| P-02 | 原样保留 | 运行时前端继续唯一位于 `web/src`；未恢复 Classic 目录。 |
| P-03 | 原样保留 | 钱包默认自定义充值金额配置与前端安全回退路径未被本次上游变更覆盖。 |
| P-04 | 原样保留 | 支付回跳统一标记和钱包刷新路径仍在。 |
| P-05 | 原样保留 | 钱包和订阅当前单前端布局未被旧 Classic 文件替换。 |
| P-06 | 原样保留 | 性能指标开关、可见分组过滤及前端状态缓存补丁仍在。 |
| P-07 | 迁移保留 | 协议 DTO/转换迁至 `relaykit`；公共 Chat-to-Responses 在 `service/openai_chat_responses_mode.go` 明确禁用，缓存写入计费路径保留。 |
| P-08 | 原样保留并融合 | `service/group.go` 保留 `GroupDescriptions` 与特殊规则；上游自动分组使用同一可见性语义。 |
| P-09 | 原样保留并融合 | 自动分组、令牌顺序和跨分组重试使用上游实现，同时保留本地访问边界。 |
| P-10 | 原样保留 | 模型元数据与按选定分组的价格展示路径仍在。 |
| P-11 | 原样保留 | `GetUserUsableGroups` 和 `controller/pricing.go` 保持配置驱动的可见性与副本脱敏；未特判 `default`。 |
| P-12 | 原样保留 | `LogRetentionDays` 的更新校验和异步清理保护仍在。 |
| P-13 | 迁移保留 | 图片能力注册、可见分组、Gemini/xAI 路由及图片 Relay DTO 全部迁至或接入 `relaykit/dto`。 |
| P-14 | 迁移保留 | 图片请求和 Playground 载荷 DTO 迁至 `relaykit/dto`；图片能力展示 DTO 保留在根 `dto`。 |
| P-15 | 原样保留 | 必需 option 通过 `seedRequiredOptions` 仅在缺失时写入默认值；队列有任务才读取并发值，缺失或非法值显式报错。 |
| P-16 | 原样保留 | 任务硬删除、幂等 DELETE、客户端 tombstone 与图片历史控制路径仍在。 |
| P-17 | 原样保留 | `affiliate_reward_events`、事务账本、硬删除邀请计数维护仍在；未做数据重建。 |
| P-18 | 迁移保留 | 额度预警和通知逻辑保留，用户设置 DTO 迁至 `relaykit/dto`。 |
| P-19 | 原样保留 | `ApplicableGroup` 字段、迁移、校验和订阅扣费选择均保留。 |
| P-20 | 原样保留并融合 | `subscription_first` 混合扣费保留，并接受上游最终重试分组结算修复。 |
| P-21 | 原样保留 | 用户缓存、认证清理和硬删除事务边界保留；`bca83882` 的用户分组编辑扩展继续按用户决定退休。 |
| P-22 | 原样保留 | 注册来源分组策略和数据库 option 读取路径仍在。 |
| P-23 | 原样保留 | locale 标准化与前端 JSON locale 文件保留。 |
| P-24 | 原样保留 | 排行榜本地自然日、权限边界和多数据库实现未被覆盖。 |
| P-25 | 原样保留 | 表格可伸缩列宽实现仍位于单前端基础组件。 |
| P-26 | 原样保留 | 亲和性与 relay 测试隔离文件未被删除。 |

## 静态验收

- 已检查所有 11 个 Git 冲突的最终语义，没有残留冲突标记。
- 已核对协议 DTO 的旧导入：聊天、图片、Gemini、Playground 协议对象均使用
  `relaykit/dto`；根 `dto` 剩余引用仅用于任务或本地展示数据模型。
- 已检查旧 `service/relayconvert` 导入：不存在运行时代码引用；删除路径均有 RelayKit
  替代实现。
- 已执行 `git diff --check`；最终提交前还会复查 UTF-8、翻译 JSON、删除列表和 Git 状态。
- 按用户当前要求，不在本机运行 Go/Bun/Docker 构建或测试；本次推送由 GitHub Actions
  执行自动构建与测试。推送本身不包含部署、容器更新或流量切换授权。
