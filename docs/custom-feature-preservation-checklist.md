# 自研功能保护清单

本文档是 `/opt/qlh-main/new-api` 后续合并上游版本时的长期功能保护清单。

它记录的是本项目自行开发或按本项目业务规则改造后的行为契约，而不是一次性
的 Git 冲突处理记录。上游出现相似实现时，可以采用上游代码，但只有在逐项证明
它保留了本文件的行为、数据和配置契约后才可以替换。不能因为文件名变化、Git
没有冲突、或上游“功能看起来更完整”就删除本项目实现。

## 1. 覆盖范围与基线

- 当前盘点快照：本地 `main@25e5f87a4d5409086af69015626d9384fd7961b7`（本次复核日期
  2026-09-28）。作者正式标签仍为 `v1.0.0-rc.40@0aec08fee811ec6136828fda790551b49e410301`；
  当前滚动主线为 `upstream/main@c2b7a9a9e0b548c2051a949fceabb59029adcb49`，只作旁证，
  不替代固定正式标签，也未合入当前 `main`。
- 2026-09-23 的 RC-40 对照以 `main@6fb2358525cf762fa81a4c59ec3751275493bf71` 为历史快照，
  直接比较得到 541 个不同路径（114 个新增、422 个修改、5 个删除）；它们包含本项目差异、
  作者版本演进、测试/文档/工具文件，不能把路径数或提交数直接当成功能数，逐项分类仍保留在
  第 8、9 节。RC-40 已于 2026-09-27 通过 `ba886153f`、`3ffc9dd4f` 集成到当前 `main`。
- 本轮逐项复核 P-01 至 P-34，并把 RC-39 合并收尾提交 `4824d38fd` 之后的 26 个本地
  非合并提交逐一归属；新增 P-35。现行第 4 节列出 29 项仍含本地独有实现或本地业务契约的
  保护项。5 个旧条目中，P-09/P-10/P-25 的行为已由作者 RC-40 等价覆盖而整项移出；P-18/P-23
  仅保留仍独有的子契约，其上游等价子行为移入历史归档。另将 P-02/P-26 两项治理条目和
  已退休的 P-33 移出；历史编号及来源仍可追溯。另识别 1 组作者历史实现残留，不计入 QLH
  自研项。上述数字按保护项/行为组统计，不是原子用户功能数量。
- `v1.0.0-rc.40` 与当前 `main` 尚未合并；两者的提交图存在分叉。因此“已吸收”只表示
  作者 RC-40 中可见等价行为，不表示当前本地代码已包含 RC-40；不得将两侧不共享的提交对象
  误算成本地功能数量。

- 历史自研基线：从上一个上游基线 `v1.0.0-rc.21` 到原 `main@6a978443` 的
  93 个非合并提交。
- rc.22 合并完成后的自研补充：`441ea707`、`9773de9e`、`51cd6ec4`、
  `51e0b058`、`d81c294f`、`504e193f`。
- 历史基线共映射 106 个可追踪的本地非合并提交（截至 `main@4173af597`）。其中
  `d81c294f` 是对分组可见性回归测试的泛化修正，不单独增加产品功能；其余提交都
  对应下列可见行为、账务不变量、数据迁移或发布能力。
- 已批准退休的本地实现包括：`cd2f8814` 只服务于已退休的 Classic 前端构建，不得恢复
  `web/classic` 或 `web/default`；`bca83882` 的用户编辑分组扩展已按用户明确决定恢复上游
  原始行为；`66759ee72` 的旧定价编辑器辅助实现已删除。现行单前端必须全部位于 `web/src`。
- 清单创建后补录的功能提交为 `571b38f03`、`3707af0c4`、`b44e6971e`、
  `cc70f2c3c`、`e1765fd8e` 和 `4173af597`；
  `88ff1c7cd` 是执行已批准退休决定的用户管理维护提交，不新增独立保留功能，
  但仍在 P-21 中保留可追踪映射。rc.22/24 的合并提交和保留执行提交已逐项复核，
  不再笼统标为“文档维护”；具体归属见第 8 节及 RC-39 审计表第 11 节。
- 本次新增 P-30 敏感词与内容审计保护项，功能提交为 `21cc64f46`、`7f17b6307`、
  `ab37d8b51`、`384e4988c`；本轮实时查找功能提交为 `416fabe52`；
  `84daf8a43` 仅回填发布记录，不新增产品契约。
- P-30 的后续实现修复包括 `b24c6ad95`（跨数据库方言正确读取敏感词配置）和
  `52d68cb40`（长提示词存储、UTF-8 截断以及 observe/block 审计失败语义）；它们属于
  现行 P-30 契约，不能在合并时只保留最初的拦截逻辑。
- RC-40 集成后的 P-30 重实现提交为 `9e5e31632`、`6c72aebf5`、`3d9c70e8a` 和
  `a09489b80`，分别覆盖运行时/迁移、Relay 与认证失效、管理界面/文档以及审查修复；
  它们通过 `ba886153f`、`3ffc9dd4f` 进入本地 `main`，不是作者上游提交。
- `25e5f87a4` 修复 P-30 日志权限与结果状态：普通用户和 API Key 在查询及计数阶段排除
  类型 8，管理员列表只读取新的 `other.admin_info.keyword_filter` 三态结果，详情要求
  `AuditRead`，并移除旧顶层字段推断；该提交属于 P-30，不新增独立保护项。
- 2026-09-28 的 P-30 用户编辑修复要求接口对 `sensitive_word_violation_count` 和
  `sensitive_word_whitelist` 保持显式 patch 语义：常规资料/权限编辑不能携带安全字段，
  只有对应控件实际变更才提交，避免陈旧表单值覆盖并发违规计数或白名单状态。
- P-13/P-16 的后续 Playground 提交包括 `889436b78`、`734b60163` 和 `4cd9c9460`；
  前者保护图片历史清理的查询边界，后两者把图片能力配置外置并统一 GPT 图片模型能力。
- 清单上次更新后新增的独立自研功能提交为 `b2e905185`（CC Switch 多来源导入）和
  `afab95b53`（钱包额度 int64 迁移）。
- `66759ee72` 的分层定价表达式编辑器状态辅助实现已按用户明确决定退休；当前定价编辑器
  使用上游 AST 表达式编辑器和官方测试，旧辅助文件及其专属测试已删除。
- rc.39 合并前新增的独立自研修复为 `5e5c79c1b`（CC Switch 名称与作用域整理）和
  `f0a62f2c6`（按所选 API Key 查询模型）；二者共同归入 P-34。当前合并目标固定为
  上游正式标签 `v1.0.0-rc.39@9978ee1e25a647bfe004e96c8719a2cb62c24732`，具体决策
  与验证证据记录在 `upstream-merge-v1.0.0-rc.39.md`；这是历史合并基线，不是本次盘点基线。
- `955243896` 与 `d1529b6eb`、`22eeed171` 与 `4c9c5209f` 分别是已回滚的功能/回滚对，
  当前主分支不保留其注册提示或自动封禁后重试文案契约；`78ae12181`、`3dda71a37` 等
  仅记录该回滚过程的文档提交也不新增保护项。
- 本文档与 [upstream-merge-v1.0.0-rc.22.md](./upstream-merge-v1.0.0-rc.22.md)、
  `upstream-merge-v1.0.0-rc.23.md`、`upstream-merge-v1.0.0-rc.24.md` 配套使用：
  各审计文件保留对应上游标签的逐提交迁移证据，本文件是以后每次合并的验收入口。

## 2. 合并时的硬性规则

1. 合并开始前复制第 4 节的全部现行保护项到本次合并审计中，并逐项标记为“原样保留”、
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
3. 对第 5 节的提交映射做集合比对：现行提交须归属一个保护项；已吸收、治理及退休提交
   须归属第 5 节的历史归档，不能再次计入现行自研项。
4. 检查删除列表、迁移后的文件路径、数据库 `AutoMigrate`/快速迁移路径、配置键
   读取路径和缓存失效路径。
5. 运行与改动范围相称的自动检查。合并完成时至少记录 `git diff --check`、Go 测试、
   前端类型检查/测试/lint/build 检查，以及 GitHub Actions 的结果。测试需隔离共享数据，
   并恢复 Gin 等全局测试模式，避免污染后续用例（原 P-26 工程治理项，不计入自研功能）。
   `new-api` 不启动本地开发或预览服务器；发布仍直接推送 `main`，由 GitHub Actions 构建镜像。

## 4. 功能保护项

本节仅列当前仍需作为本地独有行为/契约保护的项目。作者 RC-40 已有等价实现、仅属工程治理或
已按决定退休的条目不列在此处；它们的稳定编号和来源提交保留在第 5 节历史归档中。

### P-01 Fork 镜像发布与多架构产物

- 必须保留：GitHub Actions 为 `QLHazyCoder/new-api` 发布 GHCR 镜像；`main` 推送生成
  amd64、arm64 和多架构 manifest，并更新 `latest`。构建摘要和 manifest 失败信息必须
  保持可读，不能退回到上游作者的镜像命名或只发布单架构镜像。
- 当前位置：`.github/workflows/docker-build.yml`。
- 合并检查：确认 owner、小写 GHCR 路径、`main-<short-sha>` 标签、`latest`、两个架构
  和 manifest Job 均仍存在；以 GitHub Actions 为构建验收，不在本机做 Docker 构建替代。
- 来源提交：`8c813336`、`7b887e44`、`0c315d4a`。

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

### P-06 性能指标的分组可见性、聚合与失败归类

- 必须保留：性能指标的可用率颜色、成功率聚合和统计口径正确，普通用户只看到其可用
  的真实分组数据；虚拟 `auto` 分组对所有用户可见；管理员/根用户按权限查看真实分组。
- 必须保留：图片 Relay 收到上游原始 HTTP 400 时不写入新的性能请求/失败样本；原始状态
  必须在渠道状态码映射前记录。图片上游 5xx/429、本地或映射产生的 400，以及非图片请求
  仍按原规则计入；该规则只影响发布后的新样本，不回算或修改历史聚合数据。
- 当前位置：`pkg/perf_metrics/**`、`controller/perf_metrics.go`、
  `web/src/features/performance-metrics/**`、仪表盘性能组件。
- 数据/配置：性能统计依赖现有日志/统计数据，不得因合并清空或改写历史聚合数据。
- 验证入口：`controller/perf_metrics_test.go`、`pkg/perf_metrics/metrics_test.go`。
- 维护记录：2026-08-10 修正图片上游拒绝的成功率统计口径，保留错误日志、计费/退款和重试行为。
- 维护记录：2026-09-21 明确性能查询中的 `auto` 是对所有用户可见的虚拟请求分组，真实
  分组仍按用户可用分组和管理员权限过滤。
- 维护记录：2026-09-21 按官方 24 小时窗口清理模型详情页自研的 1 小时成功率展示及其
  专用聚合工具，保留后端按小时分桶和 24 小时汇总逻辑。
- 来源提交：`2d8a8fde`、`7cf40dba`、`250bde67`、`cc70f2c3c`。

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
- 必须保留：图片能力规则从代码外置到 `/data/image-capabilities.json`，首次启动生成持久化
  配置，支持 `IMAGE_CAPABILITY_CONFIG_FILE`、精确/前缀/包含匹配、渠道白/黑名单、固定
  分辨率后缀和每秒检查一次的热加载；无效 JSON 必须继续使用最后一份有效配置和内置默认值，
  不能因配置错误让已有图片模型从页面消失。配置只影响能力展示，不能改写实际路由模型名。
- 必须保留：`gpt-image-2`、`gpt-image-2.5`、其变体和 `chatgpt-image-latest` 共享完整的
  GPT 图片分辨率能力，包括横竖 4K 规格；能力注册、前端模型选项和图片 relay 必须保持一致。
- 当前位置：`pkg/imagecapability/**`、`/data/image-capabilities.json`、`service/image_capability.go`、
  `controller/playground.go`、`controller/user.go`、`router/api-router.go`、
  `middleware/distributor.go`、`relay/channel/gemini/**`、`relay/channel/xai/**`、
  `web/src/features/playground/**`。
- 合并检查：模型能力注册、用户可见分组、渠道选择、实际 relay DTO 和前端模型筛选
  必须同时存在；只迁移其中一层会造成“看得到但不能生成”或“能路由但用户看不到”。
- 验证入口：`pkg/imagecapability/registry_test.go`、`service/image_capability_test.go`、
  `controller/playground_test.go`、Gemini/xAI 图片 relay 测试。
- 来源提交：`a7b870b0`、`f3018f4f`、`99f171da`、`fce66558`、`31dce377`、
  `dfe64ed2`、`cc64bf3b`、`4e40cd8a`、`3ea4788d`、`2209a200`、`734b60163`、
  `4cd9c9460`。

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
- 必须保留：成功图片的保留清理在每次事务中最多读取固定批次（当前为 500 条），并在
  保留上限之后使用显式 SQL `LIMIT`；不能恢复无界的 `OFFSET` 查询，避免用户图片历史
  增长后造成大查询或锁事务失控。
- 当前位置：`model/playground_image.go`、`controller/playground_image_task.go`、
  `service/playground_image_worker.go`、
  `web/src/features/playground/components/playground-image-task-grid.tsx`、
  `web/src/features/playground/hooks/use-image-generation-handler.ts`、
  `web/src/features/playground/index.tsx`。
- 验证入口：`model/playground_image_test.go`、`controller/playground_image_task_test.go`、
  前端 Playground 图片库测试。
- 来源提交：`72c401ed`、`f042ea22`、`fe04dbd2`、`0f00ee3a`、`1df5a78d`、
  `78364808`、`325726da`、`3df68604`、`4284ca06`、`089f7e9a`、`6347b97e`、
  `d39cc0bc`、`5c6cfd85`、`6a978443`、`51cd6ec4`、`889436b78`。

### P-17 邀请奖励账本与额度换算

- 必须保留：注册奖励、充值返利和邀请余额划转都在所属业务事务中完成；账本为追加式
  `affiliate_reward_events`，使用幂等键，不能由账户累计字段反推或补造历史事件。
- 必须保留：邀请额度与普通额度转换使用正确的 quota 单位；邀请人数从
  `users.inviter_id` 为事实来源，硬删除会原子维护 `aff_count`，周期核对可以收敛冗余
  计数但不能改写历史奖励。
- 必须保留：Epay 充值奖励使用订单创建时保存的 `credited_quota`、奖励基点和资格快照，
  回调不能按当前 QPU 或当前奖励比例重新计算；奖励、普通额度和追加式账本必须在同一
  事务中完成，`affiliate_reward_events.reward_rate_bps` 记录实际采用的比例。
- 必须保留：钱包及邀请额度的原始值通过 `*_raw` 十进制字符串传输；转移接口接受数字
  或字符串形式的 `int64`，前端不得将原始额度转换为 JavaScript `Number` 后再提交。
- 当前位置：`model/affiliate_reward.go`、`model/topup.go`、`model/user.go`、
  `controller/topup.go`、`web/src/features/wallet/components/affiliate-rewards-card.tsx`。
- 数据/迁移：`affiliate_reward_events` 是生产账本表，禁止删除、重建或用非事务 SQL
  批量重算；详细语义见 [affiliate-reward-ledger.md](./affiliate-reward-ledger.md)。
- 验证入口：`model/affiliate_reward_test.go`、`model/payment_method_guard_test.go`。
- 来源提交：`b5fb25e1`、`4190de6e`、`ba3b9be6`、`6adf6ffb`。

### P-18 缺省开启请求与错误日志 IP 记录

- 必须保留：用户设置中没有 `record_ip_log` 键时，普通请求和错误日志默认记录客户端 IP；
  用户显式设置 `false` 时停止记录。缺省值与显式关闭必须可区分，不能改成上游 RC-40
  的非指针布尔值语义。
- 当前位置：`relaykit/dto/user_settings.go`、`controller/user.go`、`model/log.go`。
- 合并检查：分别验证设置缺失、显式 `true` 和显式 `false`；错误日志与消费日志使用一致规则。
- 来源提交：`db10c428`。

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
- 来源提交：`693494a0`、`8aaede90`、`9773de9e`、`3707af0c4`；`bca83882` 为
  经用户批准退休项，`88ff1c7cd` 为该退休决定的实施提交。

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

### P-23 本地化同步的英文基准与翻译键保留

- 必须保留：`sync-i18n` 固定以英文作为稳定源语言，不根据哪个 locale 当前键数最多而切换；
  同步前合并所有 locale 已有键，缺少英文源值时以 key 本身补齐，并将旧根命名空间键迁入
  `translation` 而不覆盖现有值，避免同步丢弃本地翻译或键。
- 当前位置：`web/scripts/sync-i18n.mjs`。
- 合并检查：对比同步前后的各 locale 键和值；标准 BCP-47 语言码和 `Intl` 归一化属于
  RC-40 已有的上游行为，不是本项的保护内容。
- 来源/保留提交：`ec15c8e23`。

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

### P-27 个人数据看板最长查询窗口

- 必须保留：普通用户的 `/api/data/self` 与 `/api/data/flow/self` 使用同一 31 天上限，
  覆盖最长自然月；不得恢复 30 天硬编码，也不得使两个个人接口出现不同上限。
- 必须保留：时间戳格式、现有数据和管理员查询范围不变；该规则不要求数据库迁移，
  仅在个人接口参数校验层生效。
- 当前位置：`controller/usedata.go`。
- 验证入口：`controller/usedata_flow_test.go`。
- 来源提交：`571b38f03` (`fix: allow 31-day user dashboard ranges`)。

### P-28 文本请求最终结果与成功率统一口径

- 必须保留：Chat Completions/Completions、Responses/Compact、Claude Messages 和
  Gemini 文本生成使用同一最终结果判定；最终结算额度大于 0 即成功，即使流有异常；
  结算为 0 时只有明确免费配置且正常完成才成功，其余为 `failed / no_billable_result`。
- 必须保留：非文本接口不改变现有统计；渠道重试只按外层用户请求的最终结果计一个
  性能样本；成功后退款不改原消费日志或成功率。新 `type=2` 消费日志在 `other` 写入
  `request_outcome`，历史日志、历史性能桶和账务/退款行为不回填或迁移。
- 当前位置：`service/text_quota.go`、`pkg/perf_metrics/metrics.go`、
  `web/src/features/usage-logs/types.ts`、`web/src/features/usage-logs/lib/format.ts`、
  `web/src/features/usage-logs/components/columns/common-logs-columns.tsx`、
  `web/src/features/usage-logs/components/usage-logs-mobile-card.tsx`、
  `web/src/features/usage-logs/components/dialogs/details-dialog.tsx`。
- 验证入口：`service/text_quota_test.go`、`pkg/perf_metrics/metrics_test.go`、
  `web/src/features/usage-logs/lib/__tests__/request-outcome.test.ts`；已通过
  `go build ./service ./pkg/perf_metrics`、性能指标测试、前端 typecheck/build/lint/format
  检查。完整 `go test ./service` 仍受既有 `service/task_billing_test.go` 中
  `dto.UserSetting` 编译错误阻断。
- 来源提交：`b44e6971e` (`fix(logs): align text request success rate`)。

### P-29 按用户分组的支付金额折扣与充值定价快照

- 必须保留：精确充值金额折扣仍使用正整数金额到 `0 < rate <= 1` 的映射；只有管理员
  选择的可用分组才能获得折扣。可用分组来自未软删除用户（启用和禁用用户都计入），
  新增选择必须在保存时有用户，历史已选分组即使后来无人仍可保留并移除；空分组数组
  表示不向任何分组发放折扣，并且 API/运行时序列化为 `[]` 而不是 `null`。
- 必须保留：定价时服务端读取数据库中的当前用户分组，客户端不能提交用于计价的分组；
  Epay 使用金额/分组倍率/折扣公式和版本化充值定价快照，Epay 待支付订单使用创建时
  保存的金额和快照，不因策略变更重新计价。Waffo、Waffo Pancake、Stripe、Creem、
  订阅和兑换码不纳入本次 Epay 固定结算扩展，继续使用各自现有流程。
- 当前位置：`setting/operation_setting/payment_setting.go`、`model/payment_group.go`、
  `model/topup_pricing_snapshot.go`、`controller/payment_discount_policy.go`、
  `controller/topup_pricing.go`、`controller/topup*.go`、
  `web/src/features/system-settings/integrations/amount-discount-visual-editor.tsx`、
  `web/src/features/wallet/components/dialogs/billing-history-dialog.tsx`。
- 数据/配置：保留 `payment_setting.amount_discount`、
  `payment_setting.amount_discount_eligible_groups`、既有 `top_up` 订单金额/状态和
  定价快照字段；Epay 新增 `credited_quota`、`quoted_money_minor`、奖励基点和结算版本，
  只做扩展式迁移，不回写或重算历史订单，不直接修改生产 MySQL 数据。
- 验证入口：`setting/operation_setting/payment_setting_test.go`、
  `model/payment_group_test.go`、`model/topup_pricing_snapshot_test.go`、
  `controller/payment_discount_policy_test.go`、`controller/topup_pricing_test.go`、
  `web/src/features/system-settings/integrations/__tests__/amount-discount-visual-editor.test.tsx`、
  `web/src/features/wallet/components/dialogs/__tests__/billing-history-pricing-audit.test.tsx`。
- 设计与阶段记录：[payment-amount-discount-group-policy.md](./payment-amount-discount-group-policy.md)。
- 来源提交：`e1765fd8e`、`4173af597`。

### P-30 敏感词规则、内容审计与用户违规控制

- 必须保留：策略支持全局规则和绑定定价分组的局部规则，规则以统一表格管理；局部
  分组只能来自 `ratio_setting.GetGroupRatioCopy()`，不得接受 `auto` 或不存在的分组。
  规则词条支持批量文本/TXT 导入、去重、编辑、`block/observe/off` 模式和确认删除，
  运行时快照更新无需重启；动作由规则自身决定，不恢复全局处理模式。
- 必须保留：Relay 在 token 估算、预扣费、计费、渠道选择、上游调用和重试之前检查
  规范化提示词；自动分组检查全部候选分组。同一请求无论命中多少词只计一次，命中
  返回不可重试的 HTTP 422 `sensitive_words_detected`；已封禁账号后续请求返回 HTTP
  403 `user_banned`，并使用协议对应的错误封装。
- 必须保留：使用日志类型 8 `敏感词审计` 通过 request ID 关联主库审计事件；普通用户
  `/api/log/self` 和 API Key 的 `/api/log/token` 在数据库查询及计数阶段完全排除该类型，
  管理员/超级管理员的全局和本人视图才可查看。管理员列表明确区分拦截、白名单放行与
  观察；审计详情要求 `AuditRead` 才能查看完整规范化提示词、命中规则、片段和规则版本。
  新日志只写 `other.admin_info.keyword_filter`，不写公开结果；界面不解读历史顶层结构。
  审计证据不写入原始请求体、API Key 或普通日志字段。
- 必须保留：用户字段保存敏感词违规次数和白名单开关。白名单命中仍记录审计和日志但
  不拦截、不计数；观察模式只记录。普通用户按行锁事务原子递增，达到可配置阈值（新环境
  默认 50）时禁用
  账户、刷新认证版本并撤销会话；封禁、清零次数、解封和白名单操作均不得清空或修改
  `quota`、余额、历史账务或历史审计证据。
- 必须保留：常规用户资料/权限编辑的前端 payload 不含违规次数和白名单字段，后端只在请求
  显式携带时更新；对应控件变更时必须能提交 `0` 和 `false`。不要将表单默认值或陈旧快照
  当作显式变更写回，以免覆盖并发产生的命中或白名单更新。
- 必须保留：默认客户端警示文案明确说明“余额不退”和严重情形报警，但这是提示文本，
  不是余额处理指令；管理员可在用户编辑左抽屉维护违规次数、清零次数和个人白名单，
  敏感词页面不承载白名单名单或审计列表，审计复核入口统一在使用日志。
- 必须保留：规则编辑弹窗在 TXT 导入按钮左侧提供实时查找。查找只针对当前未保存的
  `draft.wordsText` 做不区分大小写的普通包含匹配，首个命中自动选中并滚动，`Enter`/
  `Shift+Enter` 循环定位；搜索不支持正则、不重建或过滤文本，草稿编辑时不得抢占
  文本框光标，关闭弹窗必须清空搜索状态，保存请求仍只提交原始解析后的词条。
- 必须保留：管理员从用户列表启用账户或调用专用解封接口时，必须在同一行锁事务内恢复
  `status` 并将当前违规次数清零；历史审计、使用日志、`quota`、`used_quota` 和白名单状态
  不得改变。禁用状态转启用才递增 `auth_version`、刷新认证缓存并撤销旧会话；重复启用
  不得重复执行这些认证动作。操作审计使用 `sensitive_word.enable_reset`，并记录入口来源
  与 `balance_changed:false`。
- 必须保留：审计完整提示词在 MySQL 使用 `MEDIUMTEXT`、在 PostgreSQL/SQLite 使用 `TEXT`，
  写入前执行合法 UTF-8 的字符/字节双重截断。`observe` 模式仅对明确的审计落库失败放行并记录
  降级；规则、用户或其他事务错误以及 `block` 模式仍失败关闭返回 503，不能借故绕过策略。
- 当前位置：`model/sensitive_word_{types,rules,runtime,audit,migration}.go`、
  `model/user.go`、`model/main.go`、`model/log.go`、`model/log_other.go`、
  `relay/request_billing.go`、`controller/relay.go`、`controller/sensitive_word.go`、
  `controller/user.go`、`middleware/auth.go`、`middleware/sensitive_word.go`、
  `router/api-router.go`、`web/src/features/system-settings/request-policies/sensitive-words/`、
  `web/src/features/users/lib/user-form.ts`、
  `web/src/features/users/components/users-columns.tsx`、
  `web/src/features/users/components/users-mutate-drawer.tsx`、
  `web/src/features/usage-logs/**`。
- 数据/配置：保留旧 `SensitiveWords` Option 的一次性迁移，不保留运行时兼容回退；新增规则、词条、分组、
  审计表及用户字段必须同时兼容标准/快速迁移。审计事件写主库，日志库只保存结构化
  摘要；配置关闭证据留存时不得写入完整提示词或片段。
- 验证入口：`model/sensitive_word_test.go`、`controller/sensitive_word_test.go`、
  `controller/relay_test.go`、`controller/user_manage_test.go`、
  `web/src/features/users/lib/__tests__/user-form.test.ts`、
  `web/src/features/users/components/__tests__/permissions.test.tsx`、
  `relaykit/**`，以及敏感词页面、用户抽屉和使用日志的前端 typecheck/build/lint。
- 来源提交：`21cc64f46`、`7f17b6307`、`ab37d8b51`、`384e4988c`、`416fabe52`、
  `b24c6ad95`、`52d68cb40`。
- 长提示词/观察模式故障修复涉及 `model/sensitive_word.go`、`controller/relay.go` 及其
  模型/控制器回归测试；完整提示词字段、UTF-8 截断和审计失败类型必须继续保持。

### P-31 CC Switch 多来源导入

- 必须保留：密钥页面的 CC Switch 导入支持由集中来源注册表驱动；当前支持 Claude、Codex、
  Gemini、Grok Build、OpenCode、OpenClaw、Hermes 七个来源，并保留 Claude Desktop 的
  不支持说明，不生成错误的深链。
- 必须保留：来源专属 endpoint、模型字段和默认名称保持独立；服务地址规范化会去除尾部
  `/v1` 和斜杠，深链参数必须进行 URL 编码，缺少必填模型时不能生成导入链接。
- 当前位置：`web/src/features/keys/lib/cc-switch-sources.ts`、
  `web/src/features/keys/components/dialogs/cc-switch-dialog.tsx`、相关 locale 和测试。
- 数据/配置：无数据库迁移；新增来源必须同步更新注册表、界面选择、国际化静态键和来源
  构造测试，不能把来源配置重新散落回弹窗组件。
- 验证入口：`web/src/features/keys/lib/__tests__/cc-switch-sources.test.ts`、
  `web/src/features/keys/components/__tests__/cc-switch-dialog.test.tsx`。
- 来源提交：`b2e905185`。

### P-32 钱包额度 int64 与计费额度边界分离

- 必须保留：用户钱包、Token 剩余额度/已用额度、充值、兑换码、签到、邀请奖励、返利、
  审计累计字段使用有符号 `int64`/`BIGINT`；`MaxWalletQuota` 和 `MinWalletQuota` 只是
  防止技术溢出的边界，不重新设置 int32 产品余额上限。现有额度单位和 `QuotaPerUnit` 不变。
- 必须保留：单次请求费用、任务费用和历史消费日志仍在 int32 charge domain 中，异常价格
  或请求参数经过饱和/严格错误处理，不能为了放开钱包而取消单次计费溢出保护。
- 必须保留：钱包增减、覆盖、预扣、退款回补和批量更新经过统一的 checked SQL/CAS 入口；
  Redis 额度使用十进制字符串比较与原子 `HINCRBY`，接近 int64 边界时拒绝操作，Token
  双字段更新失败必须回补。缓存失效和数据库降级路径不能绕过边界检查。
- 必须保留：旧数据库值不缩放、不重算；标准/快速迁移后校验所有钱包字段为有符号 BIGINT，
  并记录超出旧 int32 范围但被保留的历史值。API 写入兼容 JSON number/string，查询提供
  `quota_raw`、`used_quota_raw`、`remain_quota_raw` 等精确字符串字段，前端使用 BigInt。
- 必须保留：用户余额提醒的阈值运算继续处于钱包 `int64` 域，不能通过 `int` 窄化或
  未检查的浮点转整数溢出；极大阈值必须按钱包上限安全限制。
- 当前位置：`common/wallet_quota.go`、`common/quota_math.go`、`model/wallet_quota.go`、
  `model/wallet_quota_schema.go`、`model/quota_reserve.go`、`model/user.go`、
  `model/token.go`、`controller/misc.go`、`controller/user.go`、`service/quota.go`、
  `relaykit/dto/user_settings.go`、`web/src/lib/format.ts`。
- 数据/迁移：钱包字段迁移必须保留默认值、索引、空值行为和历史 raw quota；写入大额度后不能
  回滚到仍带有旧钱包 int32 校验的版本。
- 验证入口：`common/wallet_quota_test.go`、`model/wallet_quota_test.go`、
  `model/quota_reserve_test.go`、`controller/user_manage_test.go`、
  `controller/topup_quota_limit_test.go`、相关前端额度格式化测试。
- 来源提交：`afab95b53`。

### P-34 CC Switch 所选密钥授权模型与可访问下拉框

- 必须保留：CC Switch 的模型列表必须由当前所选 API Key 授权的 `/v1/models` 决定，
  不得改用登录用户的全局 `/api/user/models`。请求显式携带所选 Key 的 Bearer token，
  并使用 `skipSessionAuthorization` 防止共享客户端把 session JWT 覆盖到该请求上。
- 必须保留：切换 Key、刷新中或请求失败时隐藏旧模型、禁用模型选择及导入；不得将已
  缓存但不再授权的模型用于深链导入。
- 必须保留：模型 Combobox 采用可脱离 Dialog 裁剪的 Portal 结构，支持过滤、键盘选择、
  Escape 关闭及异步模型到达；名称输入不应意外打开模型下拉框。上游具备等价或更完整
  UI 行为时优先采用上游组件布局和测试，保留本项的模型数据提供者与来源注册表边界。
- 当前位置：`web/src/features/keys/api.ts`、
  `web/src/features/keys/lib/cc-switch-sources.ts`、
  `web/src/features/keys/components/dialogs/cc-switch-dialog.tsx`、
  `web/src/features/keys/components/dialogs/__tests__/cc-switch-dialog.test.tsx`。
- 验证入口：所选 Key 的 `/v1/models` 授权、加载/失败隐藏、Portal、键盘、异步更新及
  七类来源的深链构造测试。
- 来源提交：`5e5c79c1b`、`f0a62f2c6`。

### P-35 使用日志模型诊断的管理员可见性与历史迁移

- 必须保留：`is_model_mapped`、`upstream_model_name`、`response_model` 等诊断字段
  存入 `other.admin_info`。普通用户的 `/api/log/self`、`/api/log/token` 和任务 DTO
  不得暴露这些字段；管理员/根用户可在使用日志详情中查看。不能只隐藏前端徽标而仍从
  普通用户 API 返回原始模型名。
- 必须保留：`cmd/migrate-log-model-visibility` 提供显式 dry-run/apply，将历史
  `logs.other` 顶层诊断字段迁入 `admin_info`，保留无关字段并按批次事务处理；服务启动时
  不得自动改写日志库，也不得改写任务表 `properties`。apply 前先发布新读写逻辑、备份并
  核对候选数；旧写入器仍在线时不得迁移。
- 当前位置：`model/log.go`、`model/log_other.go`、`model/log_model_visibility_migration.go`、
  `controller/log.go`、`controller/task.go`、`service/log_info_generate.go`、
  `service/task_billing.go`、`cmd/migrate-log-model-visibility/main.go`、
  `web/src/features/usage-logs/**`。
- 数据/迁移：仅重写日志 JSON，不删日志、不改账务；`admin_info` 同名新值优先，非法 JSON
  或非法对象结构必须报错。精确回滚依赖迁移前备份，不能声称可逐字节无损反向迁移。
- 验证入口：`controller/log_test.go`、`controller/task_generic_test.go`、
  `model/log_format_test.go`、`model/log_other_test.go` 和普通用户/管理员接口响应断言。
- 运维步骤：[log-model-visibility.md](./maintenance/log-model-visibility.md)。本清单只证明
  代码和迁移工具存在，不代表生产日志已执行迁移。
- 来源提交：`9b0238b83`。

## 5. 提交映射完整性

以下现行保护项映射用于机械核对。未来合并前，应从历史基线列出本地非合并提交并与本节
及下方历史归档比较。归档行保留原 P 编号；标为“子契约”的只剥离该上游等价行为，编号对应的
本地独有部分仍以第 4 节为准，不重复计数。

| 保护项 | 已覆盖提交 |
| --- | --- |
| P-01 | `8c813336`, `7b887e44`, `0c315d4a` |
| P-03 | `711a0315`, `bf162983`, `1a704ad1` |
| P-04 | `50f25187`, `ade4551d` |
| P-05 | `b415a3f1`, `9a3826d4`, `6be657a2` |
| P-06 | `2d8a8fde`, `7cf40dba`, `250bde67`, `cc70f2c3c` |
| P-07 | `c54a1d55`, `cb7ad647`, `4066e54f`, `fc08d7e5` |
| P-08 | `bbdeb35d`, `057f635e` |
| P-11 | `cf386058`, `51e0b058`, `d81c294f` |
| P-12 | `89c2d59a`, `45f4e67f`, `d565eea6`, `504e193f` |
| P-13 | `a7b870b0`, `f3018f4f`, `99f171da`, `fce66558`, `31dce377`, `dfe64ed2`, `cc64bf3b`, `4e40cd8a`, `3ea4788d`, `2209a200`, `734b60163`, `4cd9c9460` |
| P-14 | `30515cc6`, `99853f90`, `a934a149`, `2c9a61cb`, `f3a181fb`, `93aaa3c2` |
| P-15 | `19d9bb7e`, `ae0dc6be`, `8ce14f79`, `9b7fd1ca`, `441ea707` |
| P-16 | `72c401ed`, `f042ea22`, `fe04dbd2`, `0f00ee3a`, `1df5a78d`, `78364808`, `325726da`, `3df68604`, `4284ca06`, `089f7e9a`, `6347b97e`, `d39cc0bc`, `5c6cfd85`, `6a978443`, `51cd6ec4`, `889436b78` |
| P-17 | `b5fb25e1`, `4190de6e`, `ba3b9be6`, `6adf6ffb` |
| P-18 | `db10c428` |
| P-19 | `3c7df8f5`, `46423a16`, `3219bde1`, `3eb2c6ed` |
| P-20 | `460b36a8`, `1779060b`, `525ecf72` |
| P-21 | `693494a0`, `8aaede90`, `9773de9e`, `3707af0c4`; 已批准退休：`bca83882`; 退休实施：`88ff1c7cd` |
| P-22 | `8ccd7a17`, `393aec79` |
| P-23 | `ec15c8e23` |
| P-24 | `f3236ab4`, `705070a8`, `3642fd14` |
| P-27 | `571b38f03` |
| P-28 | `b44e6971e` |
| P-29 | `e1765fd8e`, `4173af597`, `964c8dbe6`, `0059f64bd` |
| P-30 | `21cc64f46`, `7f17b6307`, `ab37d8b51`, `384e4988c`, `416fabe52`, `b24c6ad95`, `52d68cb40`, `9e5e31632`, `6c72aebf5`, `3d9c70e8a`, `a09489b80`, `25e5f87a4` |
| P-31 | `b2e905185` |
| P-32 | `afab95b53` |
| P-34 | `5e5c79c1`, `f0a62f2c` |
| P-35 | `9b0238b83` |

### 已移出或剥离的历史映射（不计入现行保护项）

| 原编号 | 归档原因 | 来源提交 |
| --- | --- | --- |
| P-02 | Classic/Default 第二套前端已退休；禁止恢复的约束由第 2 节规则 6 维护。 | `cd2f8814` |
| P-09 | RC-40 已有等价行为；实现仍在本地树中，但不再是本地独有自研契约。 | `38d6e277`, `b365401a`, `2d08cd1f` |
| P-10 | RC-40 已有等价行为；实现仍在本地树中，但不再是本地独有自研契约。 | `076da1ef`, `dd3b02c3`, `2dab0f32` |
| P-18（上游等价子契约） | 额度预警按余额比较及通知限流已由 RC-40 覆盖；缺省开启 IP 日志仍列在现行 P-18。 | `71bfa129`, `a4a43c99` |
| P-23（上游等价子契约） | 标准 BCP-47 语言码及 `Intl` 归一化已由 RC-40 覆盖；本地 `sync-i18n` 规则仍列在现行 P-23。 | `5832779f`, `ea08ee94` |
| P-25 | RC-40 通用数据表已覆盖该行为；不再作为本地独有保护项。 | `e079ae13` |
| P-26 | 测试隔离属于工程治理，不是产品自研功能；保留测试本身。 | `271be484`, `4d972671` |
| P-33 | 旧分层定价编辑器实现已按用户决定退休并删除。 | `66759ee72` |

## 6. 本次及以后维护记录模板

每次上游合并，在合并审计文件中为每个保护项追加一行，至少包含：

| 编号 | 状态 | 最终路径 | 数据/配置检查 | 验证命令或测试 | 替代依据 |
| --- | --- | --- | --- | --- | --- |
| P-XX | 原样保留 / 等价替代 / 经批准退休 | 路径 | 无 / 已验证迁移 | 命令或测试名 | 仅等价替代时必填 |

合并结束前必须满足：没有未填写的 P 编号；没有未解释的本地文件删除；没有丢失的
`options` 键或业务表；所有当前变更文本可按 UTF-8 解析；工作区干净；推送 `main` 后
GitHub Actions 的 amd64、arm64 与 manifest 均成功。

## 7. 维护记录

- 2026-08-10：以 `dd95ab677..main@b44e6971e` 的第一父提交链复核历史；确认
  `88ff1c7cd`、`571b38f03`、`3707af0c4` 已有行为或审计文档但映射不完整，已补齐；
  新增 P-28 记录文本请求成功率与日志结果契约。合并/审计维护提交已在第 1 节明确
  标注，不与产品功能提交混计。
- 2026-08-23：以 `main@4173af597` 对比保护清单；补齐 P-06 的图片上游 400 统计修复
  来源 `cc70f2c3c`，新增 P-29 记录按用户分组的支付金额折扣、空数组序列化、统一
  充值定价和订单快照契约，并将映射计数从 103 更新为 106。
- 2026-08-23：在 rc.25 合并前再次按第一父提交链与本文件反向核对；确认 106 个产品
  提交引用均可解析且已归属 P-01 至 P-29，补录 rc.24 的五个阶段/审计维护提交和
  本轮 `eb9c43244` 的清单维护归属。发现并修复 rc.25 合并后 Epay 成功回调遗漏
  `success` 响应的问题；该修复属于阶段 2 后端审查，不改变历史订单或数据库数据。
- 2026-08-30：新增 P-30，登记敏感词与内容审计重构的规则、分组、审计、白名单、
  违规次数、第五次封禁、HTTP 403 和余额不变契约；确认功能提交已推送并按蓝绿流程
  上线，`84daf8a43` 仅为发布记录文档维护提交。
- 2026-08-30：补充 P-30 的启用边界：所有管理员启用/解封入口清零当前次数、保留历史
  证据和余额；状态变化才刷新认证并撤销会话，重复启用保持幂等。实现已由
  `384e4988c` 发布，Actions `33313133592` 成功，green standby-first 切流完成，公网版本
  `main-384e498`；该不可变版本标签的 GHCR manifest digest 为 `sha256:314616ab408bb92bdf579d814e043881dc953b5c510e6ddfe354979bc0ed8ebc`。
- 2026-08-30：在 P-30 增加规则弹窗实时查找契约及其辅助函数/组件测试；搜索仅作用于
  未保存草稿，不改变保存 payload 或后端数据结构。功能提交为 `416fabe52`。
- 2026-09-18：以 `main@66759ee72` 反向核对清单；补录 P-13 的外置图片能力与 GPT
  能力统一、P-16 的图片历史清理查询边界、P-30 的 SQL/审计存储修复，并新增 P-31
  CC Switch 多来源导入、P-32 钱包 int64 额度域和 P-33 分层定价表达式编辑器保护项。
  已明确登记 `955243896`/`d1529b6eb` 与 `22eeed171`/`4c9c5209f` 为已回滚对，
  不把它们误计入当前产品契约。
- 2026-09-20：在集成分支 `codex/merge-v1.0.0-rc.39` 完成固定标签
  `v1.0.0-rc.39@9978ee1e` 的一次性祖先合并（合并提交 `fd74c42d9`）。按“上游等价或
  更好则直接采用”原则，P-31 采用上游 Portal/Combobox，P-30 采用上游 Request Policies
  外壳并保留本地高级引擎，P-32 采用上游 billing session 并保留 signed BIGINT 精度。
  旧任务适配器、简单 RequestChecksSection、GPT 图片动态加价等重复实现已退休；保留项
  均收敛为窄适配或本地唯一契约。专项 Go/前端测试、构建和格式检查已执行；真实 MySQL
  克隆迁移、旧版兼容、候选镜像 CI 和生产发布仍未执行；本次仅完成代码合并并推送 `main`，
  不得视为生产发布完成。
- 2026-09-20：完成 `daf01437e` 兼容收尾。修复 token quota 的 number/decimal-string
  JSON 边界、int64 上下文读取、性能指标可见分组、价格分组裁剪、2FA/Passkey expand
  migration 前登录兼容、敏感词旧表回退及 BIGINT 测试断言；遵循 rc.39 删除未知
  completion 的本地预扣猜测。`go test ./...`、`go build ./...`、RelayKit 独立测试和
  controller short 全部通过；数据库克隆、候选镜像和发布仍为未授权门禁。
- 2026-09-23：以本地 `main@6fb235852` 对照作者正式 `v1.0.0-rc.40@0aec08fee`，逐项
  复核 P-01 至 P-34 的当前源文件、历史来源和 RC-40 等价实现；标出 P-09/P-10/P-18/P-23/P-25
  已吸收，P-02/P-26 为结构或测试治理，P-33 已退休。补录 P-35（使用日志模型诊断的管理员
  可见性与历史迁移），并为 RC-39 合并后 26 个本地非合并提交建立完整归属。同步清除
  RC-39 审计表内所有“待按文件反查”状态。只改审计文档，未合并 RC-40、未运行代码测试、
  未执行数据库迁移或发布。
- 2026-09-23：完成 RC-40 与本地提交树的双向差异分类，纠正“三点比较 501 路径即完整树差异”
  的口径，记录直接比较的 541 个不同路径；新增 L-01 作者 Compact 后缀历史残留和
  A-01 Task Plugin 作者版本差异，并明确二者均不计作 QLH 自研功能。
- 2026-09-23：复核时确认旧 P-18/P-23 条目各包含一项仍独有契约：缺省开启 IP 日志及
  英文为固定基准的本地化同步；已将条目收窄后保留。P-09/P-10/P-25 整项移出，P-18/P-23
  已被 RC-40 覆盖的子契约、P-02/P-26 治理项及退休的 P-33 转入第 5 节历史归档。现行保护项
  为 29 项；历史来源保留但不再混入自研数量。

## 8. RC-40 对照与 RC-39 后续提交闭环（2026-09-23 历史快照）

本节记录 RC-40 集成前的逐项源码审计证据。当前 `main` 已在 2026-09-27 集成 RC-40
及 P-30 重实现；本节中的“未合入”只描述当时的审计时点，不代表当前分支状态。

### 对照快照

| 项目 | 证据 | 结论 |
| --- | --- | --- |
| 本地代码 | `main@6fb2358525cf762fa81a4c59ec3751275493bf71` | 2026-09-23 历史盘点对象；工作区开始时干净。 |
| 作者正式版本 | `upstream` tag `v1.0.0-rc.40@0aec08fee811ec6136828fda790551b49e410301` | 本次唯一逐项版本基线。 |
| 作者滚动主线 | `upstream/main@d04c118c8803f49e0c9bab74dcf5b5efeab9464a` | 已 fetch，仅记录版本位置；未用未发布主线替代正式 tag。 |
| 集成边界 | `v1.0.0-rc.39@9978ee1e25a647bfe004e96c8719a2cb62c24732` | 历史盘点时本地功能分支以 RC-39 合并结果为上游起点；RC-40 后续已通过 `ba886153f`、`3ffc9dd4f` 集成。 |
| 新增/保留项 | 第 4 节 29 项现行保护项及第 5 节历史归档 | 无未归属的 RC-39 后本地非合并提交；RC-39 历史表中原“待按文件反查”也已逐项定性。 |

作者 RC-40 相对 RC-39 有 14 个提交；抓取时 `upstream/main` 又在 RC-40 之后。它们会影响后续
上游迁移评估，尤其是 Task Plugin、RelayKit 转换和任务提交流程，但不属于当前本地已经交付的
代码。本文只判定“本地有哪些仍独有的行为”和“作者 RC-40 是否已有等价功能”，不判定 RC-40
的合并冲突解决方案，也不把提交对象数量当成功能数量。

### RC-39 合并后本地提交归属

下表覆盖 `4824d38fd..6fb235852` 的 26 个非合并提交。`文档/验证维护` 表示不增加运行时功能，
但仍保留可追踪归属；多项编号表示该提交确实跨越多个既有契约。

| 提交 | 归属 | 复核结论 |
| --- | --- | --- |
| `6fb235852` | P-30 | 恢复敏感词审计详情呈现和回归测试。 |
| `3c998a7c5` | P-14 | 修复 Playground 图片编辑模型解析。 |
| `634807fb0` | P-13 | 恢复 Playground 图片相关 API 路由。 |
| `612d478dd` | P-30 | 敏感词选择值本地化，不改变规则数据契约。 |
| `964c8dbe6` | P-29 | 恢复支付折扣策略 API 路由并增加路由测试。 |
| `0059f64bd` | P-29/P-30/P-32 | 收尾 Epay 固定结算、精确钱包金额及敏感词默认值；没有新增独立产品项。 |
| `6230adff7` | P-21 | 恢复专用用户删除边界和额度缓存一致性。 |
| `38d33931b` | P-19/P-20 | 恢复订阅分组与混合扣费/资金来源语义。 |
| `d0835263c` | P-30 | 收敛敏感词内容审计执行、存储和管理端行为。 |
| `8b2a32f8b` | P-15/P-16 | 加固图片历史保留、结果清理与任务生命周期。 |
| `6dbb8c681` | P-06 | 删除已由上游 24 小时窗口取代的本地 1 小时成功率展示；属于批准收敛，不再保留旧行为。 |
| `9b0238b83` | P-35 | 新增管理员专属模型诊断投影和显式历史日志迁移工具。 |
| `084d663e4` | P-06 | 确保虚拟 `auto` 分组在性能视图中保留。 |
| `471b866d9` | 历史归档 P-33 | 删除已批准退休的旧分层定价状态辅助实现和专属测试。 |
| `441453dcc` | P-06 | 对齐上游性能可见性 Hook，同时保留服务端本地分组授权边界。 |
| `a19ff933d` | P-13 | 恢复 Playground 图片模型发现端点。 |
| `d81bfe556` | P-12 | 隐藏普通使用日志视图中的工具附加费标记，不改实际扣费。 |
| `99e8662c8` | 工程测试治理（历史 P-26） | 稳定前端测试运行器及重型测试套件；不新增产品行为。 |
| `69b2b99ed` | 文档/验证维护 | 记录 main 推送后的固定 SHA。 |
| `fbf0c13fc` | 文档/验证维护 | 记录 RC-39 main 交付状态。 |
| `44e18e738` | 文档/验证维护 | 记录前端聚焦测试数量。 |
| `6d6c4619f` | 文档/验证维护 | 记录 RC-39 最终验证门禁。 |
| `daf01437e` | P-06/P-11/P-19/P-20/P-30/P-32 | 跨模块兼容收尾：可见分组、账务、认证迁移边界、敏感词存储及 int64 精度；不新增独立功能编号。 |
| `1de5558e3` | 文档/验证维护 | 刷新 RC-39 验证证据。 |
| `f60b0cd9f` | P-30 | 稳定敏感词策略默认值和界面回退。 |
| `a4d93620b` | 文档/验证维护 | 记录 RC-39 兼容门禁。 |

### 完整性与限制

- RC-39 审计文件第 11 节已将原 `待按文件反查` 行改为明确的上游同步/合并、P 编号归属、
  后续撤销或纯审计维护结论；敏感词自动封禁后重试文案和注册成功提示均属于已回滚功能，
  不计入当前功能。
- 当前自研清单为 29 项本地独有实现/契约（含部分行为已被上游吸收、但仍留有本地独有
  契约的条目）。另有 8 条历史映射：6 个条目整体移出（3 项作者等价、2 项治理/测试、1 项退休），
  以及从 P-18/P-23 剥离的 2 个上游等价子契约；其提交来源仍在第 5 节归档。所有数字按保护项
  或子契约映射统计，不按提交或文件计数。
- 本轮对文档执行 `git diff --check` 和“无待归类标记”检查；没有执行 Go/前端测试，因为
  生产代码未改。RC-40 尚未合并，所有上游数据迁移兼容、CI 和部署仍须在未来获准的合并/发布
  审计中重新验证。

## 9. RC-40 全量源码差异闭环（2026-09-23 历史快照）

本节的 541 路径分类和作者提交核销均冻结于 2026-09-23。当前分支的集成结果、P-30
权限修复及验证记录见第 10 节。

### 固定比较与路径清点

共同祖先为 `v1.0.0-rc.39@9978ee1e25a647bfe004e96c8719a2cb62c24732`。本节用两棵实际提交树
直接比较，不把三点比较的单侧变更数当成树间差异数。

| 路径类别 | 不同路径数 | 处理规则 |
| --- | ---: | --- |
| Go 后端生产代码 | 129 | Go 源码中排除 `_test.go` 后的生产代码；按控制器、模型、服务、Relay、设置及迁移职责归入 P 项或下方显式差异项。 |
| 前端生产代码与资源 | 253 | `web/` 下排除 44 个前端测试文件、2 个测试基础设施文件及 2 个工程工具文件后；按钱包、Playground、定价、日志、用户、设置、共享组件和本地化行为归入 P 项。纯呈现/类型整理不单独计功能。 |
| 自动化测试及测试基础设施 | 133 | 87 个 Go 测试、44 个前端测试及 `web/src/test-setup.ts`、`web/vitest.config.ts`；作为行为回归证据或上游测试变化，不单独计运行时功能。 |
| 文档 | 17 | 作为功能约束、维护/合并记录，不单独计运行时功能。 |
| 内置插件源码 | 5 | 对照作者插件版本变化；不是新增的 QLH 自研插件功能。 |
| CI 工作流 | 1 | 归入 P-01。 |
| 配置/工具资源 | 3 | `web/.oxlintrc.json` 属工程治理；`web/scripts/sync-i18n.mjs` 的本地化同步契约归入 P-23；`pkg/imagecapability/image-capabilities.default.json` 归入 P-13。 |
| 合计 | 541 | 114 新增、422 修改、5 删除。 |

作者 RC-40 相对 RC-39 有 14 个提交；`upstream/main` 相对 RC-40 又有 8 个提交，直接树差异为
13 个路径（297 行新增、79 行删除）。这些作者侧提交也逐项检查过，没有发现其功能应计为本地
自研；它们属于未合入作者代码的升级差异，不改变本表按 RC-40 固定版本得到的自研统计。

### 非自研差异项

| 编号 | 当前代码与作者代码的差异 | 来源及分类 | 清单处理 |
| --- | --- | --- | --- |
| L-01 | 当前仍有 `-openai-compact` 的价格/倍率通配回退，以及渠道测试按后缀识别 Compact；但当前已没有旧版请求分发追加后缀、模型映射去后缀和 Codex 模型列表扩展的完整链路。 | 原始实现来自作者 `cf114ca7d` / `57746fc97`；作者在 `bb234ff41` 明确移除了模型后缀机制。当前残留是合并后保留的局部旧实现，不是 QLH 原创，也不能据此宣称端到端 Compact 后缀功能仍然完整。 | 记录为历史残留，不新增自研保护项、不计入 29 项；未来合并时需判断清理还是恢复完整契约。相关位置：`setting/ratio_setting/compact_suffix.go`、`setting/ratio_setting/model_ratio.go`、`controller/channel-test.go`。 |
| A-01 | 本地 Task Plugin 仍是旧版实现：上传上限 1 MiB，模型将 source 声明为普通 `text`，同步快照读取完整 source；RC-40 改为 8 MiB、跨数据库 `LongText`，并按哈希变化再读取/编译 source。RC-40 还包含上传后激活交互和安全解析静态常量预览。 | 这是作者旧版到作者 RC-40 的实现差异，不是 QLH 新造的功能。当前 MySQL `TEXT` 的容量小于 API 接受的 1 MiB；对新建或实际仍为 `TEXT` 的表，超过约 64 KiB 的 source 可能无法保存。生产表当前列类型未在本次读取。 | 不计入自研清单。列为版本/数据兼容风险，未来合并前必须核实实际表结构并运行 SQLite、MySQL、PostgreSQL 迁移验证；本次不改生产代码或数据库。 |

RC-40 相对 RC-39 的 14 个作者提交逐组核销如下；全部排除在 QLH 自研数量之外：

| 作者提交 | 行为/处置 |
| --- | --- |
| `47713bcb1`、`9a0be8750`、`474ed66fb`、`e537dc380`、`c0cff23a3`、`2c175190c`、`b6809a52d` | 内置插件更新、插件价格枚举编辑、Alibaba 图片元数据、上传后激活、8 MiB/跨数据库 source 存储、哈希增量同步、安全常量元数据预览。属于作者 Task Plugin 演进；当前本地代码尚未具备全部行为，见 A-01。 |
| `d61d6be75` | 任务提交接受任意 2xx 状态，属作者 Relay 行为更新。 |
| `6e9de44a7`、`4eb3b9160` | Responses/Claude 工具输出媒体和 reasoning 转换修复，属作者协议转换更新。 |
| `54eee488b` | 隔离主题偏好到本地存储，属作者前端状态持久化更新。 |
| `00e8a00cb` | Windows 构建包含 provider 图标，属作者构建更新。 |
| `9c293e8c0` | WebSocket 预扣拒绝退款测试修复，测试变化。 |
| `0aec08fee` | 作者署名维护，不增加运行时行为。 |

`upstream/main@d04c118c8` 比 RC-40 多出的 8 个提交为 `d04c118c8`、`c76452d22`、`6c14c0762`、
`a46f045d6`、`5401874c6`、`996adffe5`、`8cb88ebd1`、`9310231b3`；涉及 Responses WebSocket
设置保存、Playground 文本换行、定价表达式来源差异展示/列宽/编辑焦点、插件强制操作确认、
日志模型标记及 release ldflags。它们没有被误归为本地自研，也没有执行合并。

### 归类结论

- 第 4 节现行自研保护项为 29 项，包含部分被上游吸收但仍有本地独有子契约的条目；另有
  8 条历史映射已移入第 5 节（6 项整体移出、P-18/P-23 的 2 个等价子契约），不参与现行条目计数。
- 541 个路径差异经目录职责、现行 P 项、历史归档、测试/文档/工程资源及 L-01/A-01 分类核销；
  本轮未发现第二组可证实的 QLH 原创运行时行为或数据/工程契约落在现行 29 项之外。
- L-01 是作者旧功能残留，A-01 是作者版本差异；它们都被明确排除在 QLH 自研数量之外。A-01 的实际生产数据库列类型和 L-01 是否仍需保留，属于尚未由本次只读源码审计证明的运行状态/产品决策，不以猜测补成结论。
- 在 2026-09-23 这个历史审计时点，本轮只修改审计文档；未合并作者代码、未读取或写入生产数据库、未运行代码测试。`git diff --check` 和文档中的计数/标记复核是当时的验收门禁；当前集成后的验证结果见第 10 节。

## 10. 2026-09-28 当前主线集成与 P-30 维护记录

| 项目 | 状态 | 证据与边界 |
| --- | --- | --- |
| RC-40 祖先关系 | 已完成 | `main` 包含 `v1.0.0-rc.40@0aec08fee`；集成提交为 `ba886153f`，主线合并提交为 `3ffc9dd4f`。 |
| P-30 重实现保留 | 已完成 | `9e5e31632`、`6c72aebf5`、`3d9c70e8a`、`a09489b80` 均保留，规则运行时、Relay 预检、认证失效、管理界面和迁移链路未被 RC-40 合并覆盖。 |
| 普通用户日志隔离 | 已完成 | `model.GetUserLogsForRole` 和 `GetLogByTokenId` 在计数/分页前排除类型 8；管理员仍可按本人或全局范围查看。 |
| 管理员结果状态 | 已完成 | 类型 8 只使用 `other.admin_info.keyword_filter.action`，列表明确显示拦截、白名单放行、观察；缺失新结果显示未知，不从旧顶层字段推断。详情路由要求 `AuditRead`。 |
| 用户状态写入边界 | 已完成 | `User.EditWithTx` 不再隐式提交违规次数或白名单字段，只有敏感词专用显式事务更新路径可写；不改变 quota、余额、订阅或历史账务。 |
| 用户编辑 payload 边界 | 已修复 | `user-form.ts` 不再默认附加两个安全字段；用户抽屉仅按 `dirtyFields` 提交实际变更，包括明确清零 `0` 或关闭白名单 `false`。权限编辑、陈旧表单默认值及两个控件的独立提交均由前端回归测试覆盖；不改后端事务/数据库。 |
| 用户编辑修复验证 | 已完成 | 前端专项 2 文件 7 项、全量 181 文件 2165 项、typecheck/目标 lint/format 通过；SQLite 3.50.4 控制器专项和 `controller` 包全套通过，未执行部署或生产数据库操作。 |
| 验证 | 已完成（受限项已记录） | `go test ./...`、`go vet ./...`、`go build ./...`、relaykit 全套检查、目标 Vitest、typecheck、build:check、i18n sync 和 `git diff --check` 通过；copyright 仍有 24 个官方基线失败项，外部数据库/ClickHouse 因当前未配置 DSN 未执行。 |
| 当前上游差异 | 待后续批准 | `upstream/main@c2b7a9a9e` 已领先 RC-40，尚未合入；下一次上游更新必须重新执行第 2、3 节保护项核对。 |

本次维护提交为 `25e5f87a4`，已推送 `origin/main`；未部署、未重启服务，也未修改生产数据库。
