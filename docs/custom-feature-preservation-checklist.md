# 自研功能保护清单

本文档是 `/opt/qlh-main/new-api` 后续合并上游版本时的长期功能保护清单。

它记录的是本项目自行开发或按本项目业务规则改造后的行为契约，而不是一次性
的 Git 冲突处理记录。上游出现相似实现时，可以采用上游代码，但只有在逐项证明
它保留了本文件的行为、数据和配置契约后才可以替换。不能因为文件名变化、Git
没有冲突、或上游“功能看起来更完整”就删除本项目实现。

## 1. 覆盖范围与基线

- 历史自研基线：从上一个上游基线 `v1.0.0-rc.21` 到原 `main@6a978443` 的
  93 个非合并提交。
- rc.22 合并完成后的自研补充：`441ea707`、`9773de9e`、`51cd6ec4`、
  `51e0b058`、`d81c294f`、`504e193f`。
- 本清单共映射 99 个自研提交。其中 `d81c294f` 是对分组可见性回归测试的泛化
  修正，不单独增加产品功能；其余提交都对应下列可见行为、账务不变量、数据迁移
  或发布能力。
- 已批准退休的本地提交有两个：`cd2f8814` 只服务于已退休的 Classic 前端构建，
  不得恢复 `web/classic` 或 `web/default`；`bca83882` 的用户编辑分组扩展已按用户
  明确决定恢复上游原始行为。现行单前端必须全部位于 `web/src`。
- 本文档与 [upstream-merge-v1.0.0-rc.22.md](./upstream-merge-v1.0.0-rc.22.md)
  配套使用：后者保留 rc.22 的逐提交迁移证据，本文件是以后每次合并的验收入口。

## 2. 合并时的硬性规则

1. 合并开始前复制第 4 节的全部保护项到本次合并审计中，并逐项标记为“原样保留”、
   “语义等价的上游替代”或“经用户明确批准退休”。不允许空白状态。
2. “语义等价的上游替代”必须同时给出新路径、自动测试或可重复验证证据、数据库
   迁移/配置兼容说明。没有这三项就仍按“未保留”处理。
3. 不允许用整文件 `ours` / `theirs` 解决冲突，也不允许把本地文件删除后只以“上游
   已重构”为理由结案。
4. 所有数据库表、现有 `options` 键、历史任务记录、账本记录、订阅余额和用户会话
   都必须保持向后兼容。除非用户明确授权，不直接修改生产 MySQL 数据来迁移行为。
5. 分组名称没有内置公开或私有语义。`default`、`vip` 或其他任何名称是否公开，仅由
   `UserUsableGroups`、`GroupSpecialUsableGroup`、用户自身分组以及现行配置决定。
6. 涉及 `web/default`、`web/classic` 的旧路径时，必须证明每个仍有效的自研实现已迁移
   到 `web/src`；旧目录本身不得重新引入。
7. 每次新增自研功能、修复或数据契约时，同一提交必须更新本文件：新增保护项或把
   提交加入既有保护项，不能只在聊天记录中保留背景。

## 3. 合并验收步骤

1. 获取正式上游标签，记录共同祖先、待合并提交和当前 `main` 的本地自研提交。
2. 对第 4 节每个编号记录最终代码位置、状态和验证结果；先处理高风险项，再处理
   普通前端展示项。
3. 对第 5 节的提交映射做集合比对：每一个提交都必须恰好归属一个保护项或经用户
   明确批准的退休项。
4. 检查删除列表、迁移后的文件路径、数据库 `AutoMigrate`/快速迁移路径、配置键
   读取路径和缓存失效路径。
5. 运行与改动范围相称的自动检查。合并完成时至少记录 `git diff --check`、Go 测试、
   前端类型检查/测试/lint/build 检查，以及 GitHub Actions 的结果。`new-api` 不启动
   本地开发或预览服务器；发布仍直接推送 `main`，由 GitHub Actions 构建镜像。

## 4. 功能保护项

### P-01 Fork 镜像发布与多架构产物

- 必须保留：GitHub Actions 为 `QLHazyCoder/new-api` 发布 GHCR 镜像；`main` 推送生成
  amd64、arm64 和多架构 manifest，并更新 `latest`。构建摘要和 manifest 失败信息必须
  保持可读，不能退回到上游作者的镜像命名或只发布单架构镜像。
- 当前位置：`.github/workflows/docker-build.yml`。
- 合并检查：确认 owner、小写 GHCR 路径、`main-<short-sha>` 标签、`latest`、两个架构
  和 manifest Job 均仍存在；以 GitHub Actions 为构建验收，不在本机做 Docker 构建替代。
- 来源提交：`8c813336`、`7b887e44`、`0c315d4a`。

### P-02 单前端结构与 Classic 退休

- 必须保留：运行时和构建只使用 `web/`；本地自研前端功能都在 `web/src`。不允许把
  已退休的 `web/classic` 或 `web/default` 恢复为第二套前端。
- 当前位置：`web/rsbuild.config.ts`、`web/src/**`；原 Classic 功能映射见 rc.22 审计的
  “Classic Deletion Proof”。
- 合并检查：任何旧目录删除或重命名都要按功能迁移核对，而不是按目录整体接受删除。
- 来源提交：`cd2f8814`（唯一批准退休的 Classic 兼容提交）。

### P-03 钱包默认自定义充值金额

- 必须保留：管理员可配置默认自定义充值金额；前端在配置缺失、非法或低于当前最小
  充值额时仍给出安全可用的默认值，不把金额置零或造成不可提交状态。
- 当前位置：`setting/operation_setting/payment_setting.go`、`controller/topup.go`、
  `web/src/features/wallet/lib/payment.ts`、`web/src/features/wallet/index.tsx`。
- 数据/配置：现有支付设置中的 `default_topup_amount` 必须兼容保留。
- 来源提交：`711a0315`、`bf162983`、`1a704ad1`。

### P-04 支付回跳后的钱包刷新

- 必须保留：Stripe、Epay、Creem、Waffo 及订阅支付回跳后，钱包页通过统一返回标记
  刷新余额、订单或订阅状态；Safari 同页表单提交不能因导航竞态丢失该标记。
- 当前位置：`controller/return_path.go`、`service/return_path.go`、
  `web/src/features/wallet/lib/payment-return.ts`、`web/src/features/wallet/index.tsx`。
- 验证入口：`controller/return_path_test.go`、
  `web/src/features/wallet/lib/payment-return.test.ts`。
- 来源提交：`50f25187`、`ade4551d`。

### P-05 钱包与订阅面板的稳定布局

- 必须保留：钱包移动端/窄宽布局正常流式展示，订阅面板高度受约束但不遮挡内容，
  不因上游样式替换导致支付卡片、订阅列表或钱包操作不可见。
- 当前位置：`web/src/features/wallet/**`。
- 合并检查：在改动钱包布局时保留订阅卡、充值表单、邀请奖励卡和支付返回状态的
  可达性；不要以旧 Classic 样式覆盖当前单前端实现。
- 来源提交：`b415a3f1`、`9a3826d4`、`6be657a2`。

### P-06 性能指标的开关、可见性与聚合

- 必须保留：性能指标可被配置关闭；关闭时用户界面不显示。启用时可用率颜色、成功率
  聚合和统计口径正确，普通用户只看到其可用分组的数据，管理员/根用户按权限查看。
- 当前位置：`pkg/perf_metrics/**`、`controller/perf_metrics.go`、
  `web/src/features/performance-metrics/**`、仪表盘性能组件。
- 数据/配置：性能统计依赖现有日志/统计数据，不得因合并清空或改写历史聚合数据。
- 验证入口：`controller/perf_metrics_test.go`、`pkg/perf_metrics/metrics_test.go`。
- 来源提交：`ab1d7995`、`2d8a8fde`、`7cf40dba`、`250bde67`。

### P-07 Relay 协议兼容与缓存写入计费

- 必须保留：公开 Chat Completions 请求不会被自动改写为 Responses；Responses 的
  cache creation/cache write 用量会进入账务和日志；旧音频完成倍率配置继续有效。
- 当前位置：`relay/chat_completions_via_responses.go`、
  `service/openai_chat_responses_mode.go`、`service/openai_chat_responses_compat.go`、
  `relaykit/relayconvert/**`、`relay/channel/openai/relay_responses.go`、
  `service/billing.go`、`service/log_info_generate.go`、相关 `setting` 配置。
- 合并检查：区分协议转换、上游请求显示用量和实际扣费用量，不能只因上游返回字段
  更少就丢弃 cache-write 计费；不要重新打开公共 Chat-to-Responses 自动转换。
- 验证入口：`relay/chat_completions_via_responses_test.go`、
  `relay/channel/openai/relay_responses_billing_test.go`、相关 relay/service 测试。
- 来源提交：`c54a1d55`、`cb7ad647`、`4066e54f`、`fc08d7e5`。

### P-08 分组描述与配置编辑的保留

- 必须保留：分组倍率/可用分组的可视化编辑器在切换、保存和重新加载后不丢失
  `GroupDescriptions`；分组特殊可见规则继续可配置。
- 当前位置：`service/group.go`、`setting/ratio_setting/group_ratio.go`、
  `web/src/features/system-settings/models/group-ratio-form.tsx`、
  `web/src/features/system-settings/models/group-ratio-visual-editor.tsx`。
- 数据/配置：`GroupRatio`、`GroupGroupRatio`、`UserUsableGroups`、
  `group_ratio_setting.group_descriptions`、
  `group_ratio_setting.group_special_usable_group` 都是已有生产配置，不能在迁移中归零。
- 来源提交：`bbdeb35d`、`057f635e`。

### P-09 渠道界面状态、CC 切换与自动分组重试

- 必须保留：渠道列表视图选项持久化；CC 切换模型下拉选择正确；自动分组令牌在当前
  分组无可用渠道时按配置顺序继续尝试下一个可用分组，不能退回随机或静默失败。
- 当前位置：`web/src/features/channels/**`、
  `web/src/features/keys/components/dialogs/cc-switch-dialog.tsx`、`setting/auto_group.go`、
  `service/channel_select.go`、`middleware/distributor.go`。
- 合并检查：保留分组访问过滤、优先渠道亲和性和跨分组重试之间的既有边界。
- 来源提交：`38d6e277`、`b365401a`、`2d08cd1f`。

### P-10 模型元数据与按所选分组显示价格

- 必须保留：模型元数据可同步并在模型广场/定价页明确展示；用户选择具体分组时，
  卡片和表格显示该分组的价格，而不是所有分组中的最低价格。
- 当前位置：`model/model_meta.go`、`controller/model_meta.go`、`model/pricing.go`、
  `web/src/features/pricing/components/**`、`web/src/features/pricing/lib/price.ts`。
- 合并检查：选中分组必须一路传递到价格格式化逻辑；模型详情中的元数据和主列表价格
  是不同展示面，不能用其中一方覆盖另一方。
- 来源提交：`076da1ef`、`dd3b02c3`、`2dab0f32`。

### P-11 分组可见性、定价接口脱敏与使用日志分组选择

- 必须保留：`UserUsableGroups` 定义公开分组；用户自身同名分组始终可用；
  `GroupSpecialUsableGroup` 的加减规则生效。任何分组名称，包括 `default`，都不具有
  硬编码的公开/私有语义。
- 必须保留：`/api/pricing` 只返回至少有一个可见分组的模型，并且每个返回模型的
  `enable_groups` 只包含该用户可见的分组，不能向普通用户泄露同一模型上的其他私有
  分组；过滤必须使用副本，不能污染共享的定价缓存。
- 必须保留：使用日志的分组筛选来自定价接口的可见分组；历史 URL 中仍存在的分组值
  仍可显示，不应因下拉框改造丢失筛选状态。
- 当前位置：`service/group.go`、`controller/pricing.go`、
  `controller/pricing_test.go`、`web/src/features/pricing/**`、
  `web/src/features/usage-logs/components/common-logs-filter-bar.tsx`。
- 验证入口：`controller/pricing_test.go`；未来合并应追加普通用户、同名私有分组和
  管理员三类接口断言。
- 来源提交：`cf386058`、`51e0b058`、`d81c294f`。

### P-12 日志保留保护、错误日志权限与工具附加费标记

- 必须保留：日志异步清理不能删除保留期内的数据；`LogRetentionDays` 的上限校验同时
  在选项更新和实际系统任务中生效，不能只保留前端或管理接口校验。
- 必须保留：普通用户日志视图不显示错误日志，管理员仍可以查看；这不是删除错误日志
  数据，而是访问层过滤。
- 必须保留：工具调用附加费仍计入实际账务和日志数据，但当前用户界面不展示附加费
  标记。
- 当前位置：`controller/option.go`、`model/option.go`、`model/log.go`、
  `service/system_task.go`、`web/src/features/usage-logs/components/log-cost-display.tsx`。
- 验证入口：`controller/log_test.go`、`service/system_task_test.go`、
  `model/log_format_test.go`、
  `web/src/features/usage-logs/components/__tests__/cost-display.test.tsx`。
- 来源提交：`89c2d59a`、`45f4e67f`、`d565eea6`、`504e193f`。

### P-13 Playground 多供应商图片能力与路由

- 必须保留：Playground 图片模式支持本项目已接入的多供应商能力；模型可见性、
  OpenAI/Gemini/xAI 的图片路由和模型名保持一致，不能因上游模型识别变化错误地把
  图片请求送到文本渠道或重新开放已明确移除的 Grok 图片路径。
- 当前位置：`pkg/imagecapability/**`、`service/image_capability.go`、
  `controller/playground.go`、`relay/channel/gemini/**`、`relay/channel/xai/**`、
  `web/src/features/playground/**`。
- 合并检查：模型能力注册、用户可见分组、渠道选择、实际 relay DTO 和前端模型筛选
  必须同时存在；只迁移其中一层会造成“看得到但不能生成”或“能路由但用户看不到”。
- 验证入口：`pkg/imagecapability/registry_test.go`、`service/image_capability_test.go`、
  `controller/playground_test.go`、Gemini/xAI 图片 relay 测试。
- 来源提交：`a7b870b0`、`f3018f4f`、`99f171da`、`fce66558`、`31dce377`、
  `dfe64ed2`、`cc64bf3b`、`4e40cd8a`、`3ea4788d`、`2209a200`。

### P-14 Playground 图片请求构造、编辑与规格选项

- 必须保留：GPT 图片请求按所需规格拆分；参考图编辑、直接预览/灯箱、质量/尺寸
  选项和 `auto` 尺寸都能正确落到请求载荷。`auto` 是新增可选值，默认尺寸仍为
  `1024x1024`，不得被意外改写。
- 当前位置：`relaykit/dto/openai_image.go`、`dto/image_capability.go`、
  `web/src/features/playground/components/playground-image-input.tsx`、
  `web/src/features/playground/hooks/use-playground-image-options.ts`、
  `web/src/features/playground/lib/image-payload-builder.ts`。
- 合并检查：不要为了特殊 `auto` 值提前返回而跳过其他输入校验；参考图、尺寸、质量、
  数量和供应商特有参数都要保留。
- 验证入口：`web/src/features/playground/lib/image-payload-builder.test.ts`、
  `web/src/features/playground/lib/image-generation-capabilities.test.ts`、
  `relay/helper/valid_image_request_test.go`。
- 来源提交：`30515cc6`、`99853f90`、`a934a149`、`2c9a61cb`、`f3a181fb`、`93aaa3c2`。

### P-15 Playground 异步任务、持久队列与全局并发

- 必须保留：图片生成是可持久化的异步任务；任务状态、租约、重试、中断恢复和工作器
  处理跨实例有效，不能退回浏览器本地历史或内存队列。
- 必须保留：`PlaygroundImageMaxConcurrency` 是数据库中必须存在的 option。启动迁移只
  在缺失时写入默认 `0`，绝不覆盖旧值；`0` 表示不限并发。队列为空时不查询该 option，
  有待处理任务但 option 缺失/非法时必须明确失败，不能持续 `record not found` 高频查询
  或悄悄使用旧进程缓存回退。
- 当前位置：`model/playground_image.go`、`model/required_option.go`、`model/main.go`、
  `service/playground_image_worker.go`、`controller/playground_image_task.go`。
- 数据/迁移：保留 `playground_image_tasks`、相关批次/文件记录和 `options` 中既有并发值；
  标准/快速迁移均必须 seed 并校验 required option。
- 验证入口：`model/playground_image_test.go`、`model/required_option_test.go`、
  `model/main_migration_test.go`、`service/playground_image_worker_test.go`。
- 来源提交：`19d9bb7e`、`ae0dc6be`、`8ce14f79`、`9b7fd1ca`、`441ea707`。

### P-16 Playground 图片历史、删除与任务卡交互

- 必须保留：图片结果按用户保留上限 50 条；超额时同时清理文件和数据库行。删除是硬删除，
  不允许旧任务历史在页面刷新后复活。
- 必须保留：前端快速重复点击同一删除按钮时先同步锁定任务、禁用重复动作并保留成功
  tombstone；后端 DELETE 幂等，重复删除返回成功而不是 `image task not found`。这不影响
  不同任务的独立删除。
- 必须保留：任务卡的重试、刷新、下载、完整预览、生成中占位、控件可用性和布局保持
  可操作，不因上游 UI 改动丢失。
- 当前位置：`model/playground_image.go`、`controller/playground_image_task.go`、
  `service/playground_image_worker.go`、
  `web/src/features/playground/components/playground-image-task-grid.tsx`、
  `web/src/features/playground/hooks/use-image-generation-handler.ts`、
  `web/src/features/playground/index.tsx`。
- 验证入口：`model/playground_image_test.go`、`controller/playground_image_task_test.go`、
  前端 Playground 图片库测试。
- 来源提交：`72c401ed`、`f042ea22`、`fe04dbd2`、`0f00ee3a`、`1df5a78d`、
  `78364808`、`325726da`、`3df68604`、`4284ca06`、`089f7e9a`、`6347b97e`、
  `d39cc0bc`、`5c6cfd85`、`6a978443`、`51cd6ec4`。

### P-17 邀请奖励账本与额度换算

- 必须保留：注册奖励、充值返利和邀请余额划转都在所属业务事务中完成；账本为追加式
  `affiliate_reward_events`，使用幂等键，不能由账户累计字段反推或补造历史事件。
- 必须保留：邀请额度与普通额度转换使用正确的 quota 单位；邀请人数从
  `users.inviter_id` 为事实来源，硬删除会原子维护 `aff_count`，周期核对可以收敛冗余
  计数但不能改写历史奖励。
- 当前位置：`model/affiliate_reward.go`、`model/topup.go`、`model/user.go`、
  `controller/topup.go`、`web/src/features/wallet/components/affiliate-rewards-card.tsx`。
- 数据/迁移：`affiliate_reward_events` 是生产账本表，禁止删除、重建或用非事务 SQL
  批量重算；详细语义见 [affiliate-reward-ledger.md](./affiliate-reward-ledger.md)。
- 验证入口：`model/affiliate_reward_test.go`、`model/payment_method_guard_test.go`。
- 来源提交：`b5fb25e1`、`4190de6e`、`ba3b9be6`、`6adf6ffb`。

### P-18 额度预警与 IP 日志默认值

- 必须保留：IP 日志默认值符合本项目设定；额度预警阈值根据实际余额而不是错误的显示
  口径判断；通知限流器是最终频率控制，不能被上游重复判断替代后造成漏发或刷屏。
- 当前位置：`service/quota.go`、`service/user_notify.go`、`relaykit/dto/user_settings.go`、
  用户通知设置界面。
- 合并检查：通知逻辑涉及余额、缓存和限流时，必须完整追踪从扣费到通知的链路。
- 来源提交：`db10c428`、`71bfa129`、`a4a43c99`。

### P-19 订阅适用分组与管理界面

- 必须保留：订阅计划和用户订阅都支持 `ApplicableGroup`；空值表示全分组，非空值仅在
  使用分组匹配时扣订阅额度。管理员创建/更新计划会校验分组，用户界面显示完整计划名
  并提供“适用分组”选择及翻译。
- 当前位置：`model/subscription.go`、`controller/subscription.go`、
  `web/src/features/subscriptions/components/subscriptions-mutate-drawer.tsx`、
  `web/src/features/subscriptions/**`。
- 数据/迁移：保留订阅计划和用户订阅中的 `applicable_group` 字段，不能在上游迁移中
  置空或丢列。
- 验证入口：`model/subscription_applicable_group_test.go`、订阅 controller/前端测试。
- 来源提交：`3c7df8f5`、`46423a16`、`3219bde1`、`3eb2c6ed`。

### P-20 订阅优先的混合扣费

- 必须保留：`subscription_first` 在订阅余额不足时可以按规则使用钱包补足；严格计划
  的钱包回退语义正确。混合扣费必须保存订阅/钱包分摊，结算负差额先退钱包，正差额
  继续按正确来源追扣，任务退款同样按分摊回滚。
- 当前位置：`service/funding_source.go`、`service/billing_session.go`、
  `service/task_billing.go`、`model/subscription.go`、钱包订阅偏好界面。
- 数据/迁移：既有任务 `private_data` 中的 billing allocations 是账务事实，不能把
  mixed 任务当成单一订阅或单一钱包任务处理。
- 验证入口：`service/task_billing_test.go`、`service/billing_session_test.go`（如存在）。
- 来源提交：`460b36a8`、`1779060b`、`525ecf72`。

### P-21 用户管理、缓存一致性与硬删除边界

- 必须保留：用户管理支持按 ID 搜索、显示最后使用时间、识别覆盖分组；管理员配额
  覆盖会同时更新数据库、缓存和认证版本，不留下旧额度或旧会话可见状态。
- 必须保留：管理操作不作用于软删除用户；专用永久删除路径仍可读取目标并完整清理
  认证相关数据、缓存、邀请计数及相关业务数据。不能把软删除与永久删除重新混为一谈。
- 必须保留：`PUT /api/user/self` 的资料和密码修改只能更新自助字段白名单；
  `quota`、`used_quota`、`request_count` 等账务字段不得从请求或旧快照写回，侧边栏与
  语言偏好也只能更新 `setting` 列。
- 已批准退休：`bca83882` 曾使用户编辑抽屉将 `GroupRatio` 的分组定价和
  `GroupGroupRatio` 的用户分组覆盖键合并为可分配分组。该展示/接口扩展已按用户
  明确决定恢复上游原始行为：抽屉只从 `/api/group/` 读取 `GroupRatio` 分组；
  `GroupGroupRatio` 仍保留为真实计费覆盖配置，但不再作为可分配用户分组来源。
- 当前位置：`controller/user.go`、`model/user.go`、`model/user_auth_cache.go`、
  `web/src/features/users/**`。
- 验证入口：`controller/user_manage_test.go`、`controller/user_self_update_test.go`、
  `model/user_update_test.go`、`model/user_authentication_test.go`、
  `model/user_cache_auth_version_test.go`。
- 来源提交：`693494a0`、`8aaede90`、`9773de9e`；`bca83882` 为经用户批准退休项。

### P-22 注册来源分组策略

- 必须保留：新建用户可按密码、微信、GitHub/其他 OAuth 来源应用
  `RegistrationGroupPolicy`；策略从数据库实时读取，非法/缺失配置安全回退默认分组。
  已存在用户、OAuth 绑定/登录和管理员直接创建用户不得被重新分组。
- 当前位置：`model/registration_group_policy.go`、`controller/user.go`、
  `controller/oauth.go`、`controller/wechat.go`。
- 数据/配置：`options.RegistrationGroupPolicy` 必须保留；详细 JSON 契约见
  [registration-group-policy.md](./registration-group-policy.md)。
- 验证入口：`model/registration_group_policy_test.go`、
  `controller/registration_group_policy_test.go`。
- 来源提交：`8ccd7a17`、`393aec79`。

### P-23 接口语言代码与本地化稳定性

- 必须保留：前端使用标准语言/区域代码，供 `Intl` 格式化安全使用；i18n 同步以英文为
  稳定基准，不能用“最丰富的任意语言”覆盖其他 locale 的本地翻译。
- 当前位置：`web/src/i18n/config.ts`、`web/src/i18n/languages.ts`、
  `web/src/lib/format.ts`、`web/scripts/sync-i18n.mjs`、`web/src/i18n/locales/*.json`。
- 合并检查：全部 locale JSON 必须可解析、键集合一致，不能引入乱码、根命名空间漂移
  或把中文值批量复制到其他语言。
- 来源提交：`5832779f`、`ea08ee94`。

### P-24 排行榜的本地自然日与管理员访问边界

- 必须保留：排行榜按应用时区的本地自然日计算 today/yesterday/week/month/year，查询和
  分桶使用半开区间，缓存跨本地零点失效；“昨天”是完整上一个本地自然日。
- 必须保留：排行榜界面入口只给管理员/根用户，未登录跳登录，普通用户跳 403；这不
  擅自改变后端模块开关和接口认证语义。
- 当前位置：`service/rankings.go`、`model/usedata_rankings.go`、
  `web/src/features/rankings/**`、`web/src/routes/rankings/index.tsx`。
- 数据/配置：数据库实现必须同时兼容 SQLite、MySQL、PostgreSQL；详细时间契约见
  [rankings-time-boundaries.md](./rankings-time-boundaries.md)。
- 验证入口：`service/rankings_test.go`、`model/usedata_rankings_test.go`、
  `web/src/features/rankings/access.test.ts`。
- 来源提交：`f3236ab4`、`705070a8`、`3642fd14`。

### P-25 可伸缩数据表列宽

- 必须保留：可调整列宽的数据表会填满可用宽度，同时不挤压固定操作列、选择列或造成
  表头/行内容错位。
- 当前位置：`web/src/components/data-table/core/data-table-colgroup.tsx`、
  `web/src/components/data-table/core/table-sizing.ts` 及数据表基础组件。
- 合并检查：上游对 action column 的宽度优化必须与本项目“剩余空间拉伸”行为融合。
- 来源提交：`e079ae13`。

### P-26 自研回归隔离与关键测试前置条件

- 必须保留：渠道亲和性统计测试独立初始化/清理，Gin 测试模式在 relay 测试后恢复；
  不得为了合并或提速删除这类保护真实行为的测试。
- 当前位置：`service/channel_affinity_usage_cache_test.go`、相关 relay 测试初始化。
- 合并检查：若上游重构测试基础设施，保留相同的隔离契约，不允许测试污染后续运行。
- 来源提交：`271be484`、`4d972671`。

### P-27 个人数据看板最长查询窗口

- 必须保留：普通用户的 `/api/data/self` 与 `/api/data/flow/self` 使用同一 31 天上限，
  覆盖最长自然月；不得恢复 30 天硬编码，也不得使两个个人接口出现不同上限。
- 必须保留：时间戳格式、现有数据和管理员查询范围不变；该规则不要求数据库迁移，
  仅在个人接口参数校验层生效。
- 当前位置：`controller/usedata.go`。
- 验证入口：`controller/usedata_flow_test.go`。
- 来源提交：本项创建提交 `fix: allow 31-day user dashboard ranges`。

## 5. 提交映射完整性

以下映射用于机械核对。未来合并前，应从历史基线列出本地非合并提交并与本节比较；
除 `P-02` 的 Classic 退休项和 P-21 中经用户批准的 `bca83882` 外，每一个提交均须
保留其对应的功能契约。

| 保护项 | 已覆盖提交 |
| --- | --- |
| P-01 | `8c813336`, `7b887e44`, `0c315d4a` |
| P-02 | `cd2f8814` |
| P-03 | `711a0315`, `bf162983`, `1a704ad1` |
| P-04 | `50f25187`, `ade4551d` |
| P-05 | `b415a3f1`, `9a3826d4`, `6be657a2` |
| P-06 | `ab1d7995`, `2d8a8fde`, `7cf40dba`, `250bde67` |
| P-07 | `c54a1d55`, `cb7ad647`, `4066e54f`, `fc08d7e5` |
| P-08 | `bbdeb35d`, `057f635e` |
| P-09 | `38d6e277`, `b365401a`, `2d08cd1f` |
| P-10 | `076da1ef`, `dd3b02c3`, `2dab0f32` |
| P-11 | `cf386058`, `51e0b058`, `d81c294f` |
| P-12 | `89c2d59a`, `45f4e67f`, `d565eea6`, `504e193f` |
| P-13 | `a7b870b0`, `f3018f4f`, `99f171da`, `fce66558`, `31dce377`, `dfe64ed2`, `cc64bf3b`, `4e40cd8a`, `3ea4788d`, `2209a200` |
| P-14 | `30515cc6`, `99853f90`, `a934a149`, `2c9a61cb`, `f3a181fb`, `93aaa3c2` |
| P-15 | `19d9bb7e`, `ae0dc6be`, `8ce14f79`, `9b7fd1ca`, `441ea707` |
| P-16 | `72c401ed`, `f042ea22`, `fe04dbd2`, `0f00ee3a`, `1df5a78d`, `78364808`, `325726da`, `3df68604`, `4284ca06`, `089f7e9a`, `6347b97e`, `d39cc0bc`, `5c6cfd85`, `6a978443`, `51cd6ec4` |
| P-17 | `b5fb25e1`, `4190de6e`, `ba3b9be6`, `6adf6ffb` |
| P-18 | `db10c428`, `71bfa129`, `a4a43c99` |
| P-19 | `3c7df8f5`, `46423a16`, `3219bde1`, `3eb2c6ed` |
| P-20 | `460b36a8`, `1779060b`, `525ecf72` |
| P-21 | `693494a0`, `8aaede90`, `9773de9e`; 已批准退休：`bca83882` |
| P-22 | `8ccd7a17`, `393aec79` |
| P-23 | `5832779f`, `ea08ee94` |
| P-24 | `f3236ab4`, `705070a8`, `3642fd14` |
| P-25 | `e079ae13` |
| P-26 | `271be484`, `4d972671` |
| P-27 | 本项创建提交 `fix: allow 31-day user dashboard ranges` |

## 6. 本次及以后维护记录模板

每次上游合并，在合并审计文件中为每个保护项追加一行，至少包含：

| 编号 | 状态 | 最终路径 | 数据/配置检查 | 验证命令或测试 | 替代依据 |
| --- | --- | --- | --- | --- | --- |
| P-XX | 原样保留 / 等价替代 / 经批准退休 | 路径 | 无 / 已验证迁移 | 命令或测试名 | 仅等价替代时必填 |

合并结束前必须满足：没有未填写的 P 编号；没有未解释的本地文件删除；没有丢失的
`options` 键或业务表；所有当前变更文本可按 UTF-8 解析；工作区干净；推送 `main` 后
GitHub Actions 的 amd64、arm64 与 manifest 均成功。
