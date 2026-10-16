# Responses 流断连与缓存计费改造计划

> 状态：问题 1 已完成本地实现并待发布验证；问题 2 保留运行态证据定位，未做猜测性字段补丁；未改线上服务或价格配置。
> 编制日期：2026-10-16。代码基线：`/opt/qlh-main/new-api` 的 `main` (`3343b7c5d`)；报告中的部署版本为 `5d48973b0`（2026-10-14）。相关核心文件在两个提交之间经核对无差异。部署镜像修订、实际渠道路由和上游 SSE 尚未现场复核。其他 checkout（例如 `new-api-merge-v1.0.0-rc.39`）不是本方案的实施基线。

## 项目整体分析

### 范围和现象

请求路径：Codex CLI -> new-api -> 自建 sub2api（type 59）-> 第三方站点；相关模型 `gpt-5.6-sol`、`gpt-5.6-terra`，入口 `/v1/responses`。以下为运营方提供的个案数据，金额为该方记录，尚缺同一请求的三段原始事件和完整 request ID；时间沿用原始展示，时区未核实。为避免在仓库文档保存客户标识，省略用户名。

| 样本 | new-api 日志 (in/out, client_gone) | new-api 收费 | 上游记录 (非缓存输入/缓存读/输出) | 上游成本 |
| --- | --- | ---: | --- | ---: |
| A | 105245 / 1；3 s | $0.1110 | 1668 / 110335 / 232 | $0.0705 |
| B | 105245 / 1；57 s | $0.1110 | 0 / 0 / 0 | $0.0000 |
| C | 105245 / 1；38 s | $0.1110 | 95652 / 13325 / 709 | $0.5062 |
| D | 105246 / 83；71 s | $0.1110 | 1631 / 109311 / 1824 | $0.1175 |
| E | 105330 / 1；24 s | $0.1110 | 615 / 110335 / 2540 | $0.1344 |

这五条新网关共收费约 $0.555，上游记录合计 $0.8286；C 的单笔缺口约 $0.3952。该对比描述收支问题，不证明每一笔都在断开后完整生成，也不证明 B 的零费用对应上游 SSE 明确报告全零 usage。另一组 `gpt-5.6-terra` 的正常/异常请求中，新网关的 `cache_tokens=0`，而第三方账单出现 `cache_read=84736/85760/12032/3840`。其中第三方输出为 86、143 的请求仍需核验新网关的 `end_reason`、终态事件和同一 request ID；不能仅以第三方“成功”推出新网关收到终态。

第三方账单据提供样本推算为 `(非缓存输入 * 2 + 缓存读取 * 0.2 + 输出 * 12) / 1e6 * 0.14` 美元；这是运营方观察到的成本口径，**不是**新网关零售定价的正确性标准。零售组倍率 0.38 / 0.21 保持独立，本项目不拟以成本 0.14 覆盖零售价。计费行为和客服政策仍须按客户可见的定价规则验收。

### 当前调用链与可证实的原因

```text
/v1/responses -> ResponsesHandler -> type 59 sub2api.Adaptor
  -> newapi.Adaptor -> openai.Adaptor.DoResponse -> OaiResponsesStreamHandler
  -> StreamScannerHandler (读 SSE / 送数据回调)
  -> ResponsesUsageAccumulator.Observe/Finish
  -> ConsumeResponsesQuota -> PostTextConsumeQuota
  -> effectiveBillingUsage -> BuildTieredTokenParams -> SettleBilling -> 消费日志
```

1. `relay/helper/stream_scanner.go` 的主循环在 `c.Request.Context().Done()` 设置 `client_gone` 后直接执行 `cleanup()`；后者关闭 `resp.Body`、停 scanner 并等待 worker。因此后续 `response.completed` 的原生 usage 不可达。这是问题 1 在 new-api 的明确机制，但第三方在何种账户/协议路径继续计费仍须逐路径确认。
2. `relay/channel/openai/relay_responses.go` 将 SSE 解码成 `ResponsesStreamResponse` 并交给 `service/responses_usage.go`；只有终态事件内的 `response.usage` 才在这条原生路径上进入 `ApplyResponsesUsage`。丢失终态时 `Finish()` 用已观察到的输出文本与本地 prompt 估算补账；输出为 1 不能简单解释为“客户端收到了 1 token”。
3. type 59 **没有独立的 Responses 用量适配器**：`relay/channel/sub2api/adaptor.go` 嵌入 newapi 适配器，最终调用相同的 `OaiResponsesStreamHandler`。因此“sub2api 渠道完全不经过 ResponsesUsageAccumulator”不成立。
4. 标准 `response.usage.input_tokens_details.cached_tokens` 在 `relaykit/dto/openai_response.go` 有字段，经 `NormalizeResponsesUsage` 映射为 `PromptTokensDetails.CachedTokens`，被 `service/text_quota.go` 读取并提供给 `service/tiered_settle.go` 的 `cr`；源代码**不能支持**“该字段根本未实现”的定论。`cache_tokens=0` 需先判定线缆数据、终态、字段别名、`BillingUsage` 覆盖与实际生效的定价快照。
5. `service/responses_usage.go` 目前通过“值是否为零”触发估算：如果确实收到 `response.usage`，但其中输入与输出**明确为 0**，后续 `a.started` 仍可导致本地 prompt 回填。需把“没有 usage”与“收到权威的全零 usage”分开；仅靠 drain 不一定能修复样本 B。
6. 已有的 `#7427` 处理截断流的本地估算，不能凭空恢复上游未到达的缓存/输出事实；同时主路径 `DoApiRequest` 用 `http.NewRequest`，现行关闭方式主要是 scanner 关闭 `resp.Body`，不能将未合并的 `#4500` 描述为当前源码已实现的按下游 context 取消上游请求。

上游参考：[new-api PR #7178](https://github.com/QuantumNous/new-api/pull/7178)（截至编制日仍开放，提议限时 drain）、[PR #4500](https://github.com/QuantumNous/new-api/pull/4500)（仍开放，非已合并）、[PR #7427](https://github.com/QuantumNous/new-api/pull/7427)（已合并），以及类似情形的 [issue #7062](https://github.com/QuantumNous/new-api/issues/7062)。这些 PR 是设计参考，不作为部署状态或本地适配正确性的证据。

### 尚待验证的关键事实

| 证据点 | 采集和判定 | 决定的后续动作 |
| --- | --- | --- |
| 部署版本/转发路径 | 核对运行镜像 revision、实际请求命中的渠道 ID/type、上游 URL/账户平台、new-api 与 sub2api 两边 request ID | 若与本文基线不一致，先更新文件清单和测试基线 |
| 正常成功请求的 SSE | 在 new-api **入站上游流**边界记录脱敏后的事件类型、是否有 `response.usage`、`input_tokens`、`output_tokens`、`input_tokens_details.cached_tokens`，以及 `[DONE]`/EOF；与 sub2api **出站流**对应比较 | 缺字段修上游协议形态；字段存在再沿 DTO/计费排查 |
| 断流 SSE | 记录客户端断开时刻，sub2api 后续是否仍生成/累计真实 usage，是否还能向已断开的 new-api 连接发送终态 | 判断 drain 能否拿到账单；若上游不发终态，需要 sub2api 协议修复或按 request ID 的事后对账设计 |
| 计费内部口径 | 对比解码后 `InputTokensDetails`、归一化 `PromptTokensDetails`、`effectiveBillingUsage()` 的最终值、`summary.CacheTokens`、`TokenParams{P,Len,CR,CC,C}` 与冻结价格快照 | 明确缓存丢在哪一层；排除历史配置/附加倍率偏差 |
| 全零 usage | 同一请求分别核对第三方后台“金额 0”、sub2api 实际回包中的 `usage` 是否存在且值全零、new-api 是否收到了该终态 | 只有明确收到权威全零 usage 才可避免本地估算；不允许凭后台零金额猜测 |

采集优先复用已有请求和可控测试流；如需在生产增加临时埋点，仅记录计数字段、路径/渠道和脱敏关联 ID，限定时间并停用。不记录 Authorization、原始 SSE、prompt、工具参数或用户标识；不为诊断主动制造大额线上调用。真实现场/第三方关联证据经授权后再采集，本轮不抓生产流量。

## 重构方案

### 原则与范围

- 两个问题独立验收。先查清 usage 实际形态，再实施问题 1 的有界 drain；缓存链路修复按证据分支实施，避免无端改动全局计费代码。
- 第一批仅覆盖 **type 59 渠道的 HTTP SSE `/v1/responses` 原生透传**，默认在客户端断开后继续读取。其他渠道/协议保留旧行为，完成协议矩阵后再按明确依据扩展；WebSocket 是另一条会话生命周期，本阶段不承诺自动覆盖。
- 计价合同采用“有上游权威 usage 时按该事实结算；缺失时保留并标记现有估算/争议流程”。`client_gone` 是下游交付事实，不等于上游成功；消费日志还须区分终态是否获得和计数来源。
- 保持已有 wallet、token、订阅/混合资金预扣与结算链路，不新建账务表、不改价格系数。更改下游取消后仍收实际生成费用需要运营方确认客户告知、申诉和欠费政策。

### 问题 1：有限时、可关闭的 drain

1. 已为共享 `StreamScannerHandler` 增加可选的 **单请求** drain 参数入口；旧入口仍立即关闭。仅 `OaiResponsesStreamHandler` 在 type 59 且 HTTP SSE 时选择新入口。默认 `RESPONSES_CLIENT_GONE_DRAIN_TIMEOUT_SECONDS=120`，0 关闭；默认 `RESPONSES_CLIENT_GONE_DRAIN_MAX_CONCURRENCY=32`，0 不接受 drain，环境值分别限制为 300 秒和 256 条。`STREAMING_TIMEOUT` 原有流式空闲超时仍生效；drain 另有**不因每个 chunk 重置**的绝对时限。
2. main select 观察到下游取消时，记录 `client_gone` 并停止 ping / 下游写入，**不调用** `stop()`、`cancel()` 或 `resp.Body.Close()`；转入等待 scanner 正常结束、空闲超时或绝对 drain 超时的受控状态。成功时沿用 scanner -> `dataChan` -> handler 的现有顺序，终态先由回调解析完，再返回到 `accumulator.Finish()`。超时、空闲超时或并发额度耗尽时回到原有 cleanup，关闭上游 body、解除阻塞，等全部 worker 退出后按现有缺 usage 估算策略结算。
3. 断开后 callback 仍要调用 `accumulator.Observe`（含终态、工具调用和 usage），只跳过 `sendResponsesStreamData` 及 `ExtendWriteDeadline` 等下游写操作。取消与某次写同时发生的竞态仍按现有写超时约束处理；非取消导致的真正写错误和上游解析失败不得伪装成可继续的正常 drain。
4. `dataChan` **只由 scanner 关闭**；handler 在 drain 期间持续消费，scanner 写队列不会因为下游 writer 已死而堵住。不可从主 goroutine 提前关闭 dataChan，不可将使用 `gin.Context` / `RelayInfo` 的 goroutine 扔到 handler 返回之后。现有 `cleanup()` 仍作为最终路径负责关闭 body、cancel、停计时器并 `wg.Wait`，完成后才允许进入额度结算及 Gin context 复用。
5. 已把下游结束原因和上游终态分开记录：`end_reason=client_gone`、`response_status=completed/failed/incomplete/unknown`、`drain_result=ended/timeout/idle_timeout/limit_reached/disabled`。现有 `StreamStatus.SetEndReason` 为 first-wins；取消与 scanner 已结束同时就绪时先检查 `stopChan`，避免把已完成流误标为断流。`[DONE]` 仍不等同于已解析终态 usage；usage 是否权威仍由 accumulator 的具体字段决定。
6. 已用进程内互斥计数限制同时 drain 的连接数；额度耗尽不等待，记录 `limit_reached` 后走原有立即 cleanup。当前日志暴露 drain 结果但尚未接入独立指标，线上需按 `stream_status.drain_result` 统计超时、额度回退和终态比例，再决定是否调整默认值。

`http.NewRequest` 当前未绑定下游 context；实施时不能直接改成 `http.NewRequestWithContext(c.Request.Context())`，否则客户端断开会在 scanner 层之外杀死上游。若测试显示 body.Close 不能可靠解阻，需让上游请求持有与下游**解耦且受绝对期限约束**的私有 context，同时覆盖建连失败、响应关闭与取消清理；不得引入永不结束的后台请求。

### 问题 2：定位后修正缓存事实，而非先改价格

按证据分支执行：

1. sub2api 发给 new-api 的 SSE **没有** `response.usage.input_tokens_details.cached_tokens`：核对是否只是第三方后台存有缓存读数。若上游可取回数据，在 sub2api 对应 Responses 输出转换处补齐标准终态字段，并保证 `input_tokens` 是含缓存总数、`cached_tokens` 是其子集，不能同时把缓存另加进输入再次计价。自建 sub2api 的 Anthropic -> Responses 转换源码已能生成此字段，但不同账户可能走原生 Responses 透传、兼容桥或其他路径，须以命中路径为准。
2. SSE **带标准字段**但 new-api 解码/归一化后为零：针对已复现的 payload 修 `ResponsesStreamResponse` / `ApplyResponsesUsage` / `MergeUsageNonZero` 的准确丢失点，不给 sub2api 另做重复适配；再查 `BillingUsage.CanonicalUsage()` 是否优先选了缺缓存的 sidecar。保留未知、显式零和正值的差别；异常负数、`cached_tokens > input_tokens` 等不合法数据不静默记成正常缓存优惠。
3. SSE 只有 `usage.prompt_tokens_details.cached_tokens`、平铺 `usage.cached_tokens` 或独立用量事件：先证明是当前账户真实协议及其总量语义，才实现**明确作用域**的兼容解析，写协议回归样本。不要全局把任意名为 `cached_tokens` 的字段作为 Responses 缓存读数。
4. 缺终态但 sub2api 后台有缓存读数：优先由问题 1 使终态可达；若独立的非断流请求仍未收到终态，定位上游何处提前 EOF/超时、SSE 格式和 `[DONE]` 顺序。若 sub2api 自身不返回 usage，仅改 new-api 无法得到真实缓存读数，应先补上游输出/可靠对账渠道，而非从价格或第三方金额反算 tokens。
5. 原有表达式**已引用 `cr`** 时，`BuildTieredTokenParams` 对非 Claude 格式自动计算 `p = prompt_tokens - cached_tokens`、`cr = cached_tokens`，而 `len` 保留总输入；继续使用概念上的 `(p*2 + cr*0.2 + cc*2.5 + c*12) / 1e6 * 0.38`（仅描述运营方提供的短上下文零售口径）。**不在表达式中再次写 `p-cr`**，否则双重扣减。长上下文档位、cache-write 重叠、分组及工具附加价继续按生效快照单独验证。示例：若总输入 85884、其中缓存 84736、输出 86，则计价 `p=1148, cr=84736, c=86`；非缓存输入不再按 85884 全价计。

`response.usage` 明确报告全零时还须修 `ResponsesUsageAccumulator.Finish` 的“无 usage 才估算”门槛：记录是否收到权威终态 usage（包括显式零），只对**缺失**的值按已定义政策估算。不要把 `output_tokens=0` 的真实上游事实补成 1，也不要用第三方后台“收费 0”自动判定 new-api 收到了全零事件。若全零 usage 与大量实际生成内容冲突，留下可审计的异常而不悄然改写供应商账单。

价格系数本轮不变。若证据证明线上生效表达式本身缺少 `cr`、档位错误或与零售合同不符，另列运营定价变更：读取版本化配置快照、基于核实的定价证据用仓库定价生成工具构造候选、运行表达式向量与只读预览，且须取得**单独授权**才可通过现有版本化 API 修改并回读；不手写整份配置 map，不直接改数据库。

### 协议边界与风险控制

| 范围 | 当前路径/限制 | 本期处理 |
| --- | --- | --- |
| type 59 原生 HTTP Responses SSE | openai 适配器 + ResponsesUsageAccumulator | 目标改造与端到端结算验收 |
| 非流式 Responses | `OaiResponsesHandler` 直接读 `response.usage` | 仅核对缓存解析回归，不实施 drain |
| Chat Completions SSE / 经 Responses 转换到 Chat | 分别有独立最终 usage 提取/格式转换 | 不自动开启 drain；检查现有取消语义、必要时另立方案 |
| 原生 Claude / Gemini 及转换到 Responses | 共用 scanner，但 callback 的写失败常会 `sr.Stop`，并可能在 scanner 后再合成输出 | 本期仅回归旧行为；扩展时逐协议修终态及结算，不复用尚未验证的全局无操作写法 |
| 图片、音频、Dify、Baidu、xAI 等 | 有共享 scanner 或自定义响应生命周期 | 避免全局改变；分别覆盖其取消/金额/图像数量规则 |
| `/v1/responses` WebSocket | `relay/responses_websocket.go` 独立会话生命周期 | 不受 HTTP scanner 方案自动覆盖；另行评估 |

主要风险：持续上游生成的增量成本与占用（有界时间 + 并发门槛）；长 SSE 的临时内存、队列、goroutine 与 HTTP 连接；超时仍拿不到权威 usage；下游已断但用户仍被扣款的公平性与争议；上游明确失败/全零与估算混淆；预扣不足后的实际差额、订阅/混合资金及限额；新增诊断信息泄露 prompt/token。交付条件不是“费用更接近上游”一项，还需客户协议、账务零值/欠费策略和审计记录同时达标。特定 sub2api 路径可能在对 new-api 写失败时提前结束 SSE，即便它本身已累计/支付费用；那时本方案必须进入上游补终态或 request ID 对账的条件分支，不能宣称 drain 必然解决所有损失。

## 文件结构设计

**只规划局部文件职责，不重排现有目录、不引入新的服务和数据库 schema。** 下表路径相对于 `/opt/qlh-main/new-api`；`保留` 表示无证据不修改，`条件` 表示仅在阶段 0 证据成立后修改。

| 路径 | 职责与拟调整原因 | 状态 |
| --- | --- | --- |
| `docs/responses-stream-usage-cache-billing-plan.md` | 本方案、证据分级、阶段清单和最终自检记录的唯一文档 | 本轮新增 |
| `relay/helper/stream_scanner.go` | 共享 SSE 生命周期、断连后限时 drain、并发额度、worker/队列退出与资源清理；旧入口行为不变 | 已修改 |
| `relay/helper/stream_scanner_test.go` | 保留现有立即取消测试，增加时限、并发额度、终态先后和泄漏回归 | 已扩展 |
| `relay/channel/openai/relay_responses.go` | type 59 HTTP Responses 启用可选 drain；断连后继续 `Observe`、停止下游写 | 已修改 |
| `relay/channel/openai/relay_responses_billing_test.go` | 用真实 SSE 形态覆盖 Sub2API 断开后终态 usage 回传 | 已扩展 |
| `service/responses_usage.go` | 根据抓包确认权威终态 usage 的有无，显式零与估算分离 | 条件修改 |
| `service/responses_usage_test.go` | 有 usage、缺 usage、全零、失败/incomplete 和 tool 用量回归 | 条件扩展 |
| `constant/env.go`、`common/init.go`、`.env.example` | 声明/解析/限制 drain 超时、并发上限和关闭值 0；无需新增数据库配置 | 已修改 |
| `relay/common/stream_status.go`、`service/log_info_generate.go` | 增加不含敏感内容的 `drain_result` 审计字段 | 已修改 |
| `relaykit/dto/openai_response.go`、`relaykit/dto/usage_merge.go` | 标准缓存字段已存在；只有确认 DTO / 合并丢字段后针对具体形态修复 | 条件修改 |
| `relaykit/relayconvert/internal/oai_responses/to_oai_chat_resp.go`、`service/billing_usage.go` | 归一化和原生 sidecar 来源；只有有证据显示未保留缓存计数时修改 | 条件修改 |
| `service/text_quota.go`、`service/tiered_settle.go`、对应现有 `*_test.go` | 校对 summary、`p/len/cr` 和结算；已有自动扣缓存机制，默认保留 | 保留/条件 |
| `relay/channel/sub2api/adaptor.go`、`relay/channel/newapi/adaptor.go`、`relay/channel/openai/adaptor.go` | 渠道派发边界；已复用 Responses handler，不另起一套解析 | 保留 |
| `relay/channel/api_request.go`、`relay/responses_handler.go`、`service/billing_session.go` | 上游请求生命周期、归并结算/预扣；如需修改必须单独证明不会破坏其他端点及资金路径 | 保留/条件 |
| `/opt/qlh-main/sub2api/backend/internal/service/gateway_forward_as_responses.go`、`openai_gateway_response_handling.go` | 外仓库不同 Responses 转发路径；仅新网关无法取回 usage 时另行实施并验收 | 条件，非本轮改动 |

已有 `pkg/billingexpr/expr.md` 和 `.agents/rules/billing.md` 是必须保持的计费约束文档，不拟更改；若实际修改 `relaykit/`，它须保持可独立构建。新增业务文件、迁移文件和新的测试目录均不在当前预案中；确有必要时先在本表写清职责和验收再申请调整。

## 开发清单

| 阶段 | 可追踪任务 | 当前状态 | 阶段验收/转向条件 |
| --- | --- | --- | --- |
| 0-A | 核对镜像 revision、命中账户平台/转发链路、分组与生效价格快照；整理 4 类脱敏案例（正常缓存命中/不命中、断开有用量、全零或缺终态） | 待授权 | 一条请求有贯穿三层的关联 ID、时间戳和 usage 事件/金额对应关系；不能对应则停止定因 |
| 0-B | 确认生效 `cr` 是否已在表达式中及 cache-write、长上下文档位；检测是否为有终态的非断流 `cache_tokens=0` | 待授权 | 写出“缓存数据在哪里消失”的最小证据；如标准字段完整则不得实施无意义 DTO 补丁 |
| 0-C | 复现不同 sub2api 账户平台在下游断开后的继续生成/推送行为；量测断开后至终态的 P95/P99、取消比例、连接数与最大内存 | 待授权 | 定稿正值 drain 时限、并发上限及告警阈值；若上游不能返回 usage，转上游修复/对账方案 |
| 1-A | 在本地实现 type 59 HTTP Responses 的可关闭有限时 drain；明确 cleanup、`dataChan` 关闭、scanner/handler/ping 退出先后 | 已完成（本地） | 默认入口旧行为回归通过；断开后终态进入 accumulator；无终态等待绝对时限终止；客户端不再接收断开后的事件 |
| 1-B | 给断连/终态两个维度增加可审计结果；核对资金结算仅发生一次，预扣在延迟期间保留 | 已完成（代码/单测） | `client_gone`、`response_status`、`drain_result` 分离；实际预扣/结算仍需部署后日志对账 |
| 2-A | 仅对抓包确定的缓存丢失层修改字段映射，必要时在 sub2api 相应路径补出站终态 | 待阶段 0 证据 | 标准字段从 SSE 到消费日志均为同一正值；缺字段绝不假称命中 |
| 2-B | 确认并修复“明确全零 usage”被本地 prompt 回填；保持“无权威 usage”的现有估算/审计策略 | 待阶段 0 证据 | 显式全零应为零 token 计价（额外固定价/工具费按既有契约）；缺失 usage 有清楚来源标记 |
| 3-A | 测试：完整/断开/正常 EOF 缺终态/绝对超时/上游 read error/失败终态/全零/有缓存/缓存超总数/同时断连及到达 `[DONE]`；并发高压只做有限可重复的资源回归 | 部分完成 | 新增定向测试和新增路径 race 已通过；全量 race 仍受既有 logger 全局测试竞争影响，不能据此宣称全量 race 通过 |
| 3-B | 回归其他 scanner 用户、WebSocket 未覆盖声明；验算零售倍率/档位与 sub2api 真实账单账务分类（不要求两者单价相等） | 已完成（源码范围） | 共享 scanner 默认入口未变，WebSocket/其他协议未开启 drain；真实账务对账待部署 |
| 4-A | 本地验收、整理脱敏前后对照、审查文件清单与本方案逐项状态；不自动提交或部署 | 已完成（本地） | 全量串行测试、vet、根模块 build、relaykit 独立 build、diff/UTF-8 检查通过；新增路径 race 通过；全量 race 仅受既有 logger 全局测试竞争影响 |
| 4-B | **另行确认**灰度和生产发布：低流量单组试运行、核心指标观察、异常回退到 0、运行镜像/配置回读 | 需单独上线授权 | 经授权后按实际生产窗口与回滚预案执行；若有 DB 修改必须跑 SQLite/MySQL/PostgreSQL 实例矩阵 |

建议验证命令仅用于后续代码阶段，执行后须记录真实结果：`go test ./relay/helper ./relay/channel/openai ./service -count=1`、`go test -race ./relay/helper ./relay/channel/openai ./service -count=1`、`go test ./... -count=1 -p=1`、`go vet ./...`、`go build ./...`、`git diff --check`；触及 `relaykit/` 时另在该目录运行 `GOWORK=off go build ./...` 及相应测试。是否需要更广的数据库矩阵、跨模块集成/反向代理测试，以实际修改的路径确定，不把计划项视为已经完成。

## 阶段自检记录

| 阶段 | 逻辑/依赖/遗漏/文档自检 | 已发现问题与拟处理动作 | 状态 |
| --- | --- | --- | --- |
| 方案前 | 对照部署提交 `5d48973b0` 和当前主仓库，读取 billing/expr 规范，核实 scanner、type 59、Responses、settlement 与 sub2api 多路径源码 | 纠正“type 59 不走 accumulator”“已合并 #4500”“表达式还要手减一次 cr”；分离运营日志与上游线缆事实 | 已完成，仅源码/提供案例 |
| 阶段 0 | 回查部署、对应请求和脱敏 SSE，核验计费字段一致性及超时预算 | 当前没有线上抓包/运行态对账，不提前认定缓存字段具体丢失点、sub2api 路径或全零 SSE | 未开始 |
| 阶段 1 | 检查 scanner 的 stop/body/dataChan、writer/ping、Gin context 与一次结算，更新文件映射 | 新增 options 入口；type 59 才开启；保留默认立即取消合同；完成/超时/并发满均等待后 cleanup；加入 `drain_result` | 已完成（本地） |
| 阶段 2 | 检查标准缓存字段与显式零，回归协议/DTO/表达式三个口径 | 任何额外字段兼容/上游改动均需阶段 0 原始形态证据 | 未开始 |
| 阶段 3-4 | 核验测试结果、变更文件与本计划逐项覆盖，补充验收证据与残余风险 | 定向普通测试、定向 race、全量串行测试、vet、根模块 build、relaykit 独立 build、diff/UTF-8 检查均通过；全量 race 受既有 logger 全局测试竞争影响 | 已完成（本地） |

### 本轮验证记录

| 命令/检查 | 结果 | 说明 |
| --- | --- | --- |
| `go test ./relay/helper ./relay/channel/openai ./service ./relay/common -count=1` | 通过 | 覆盖 drain、Responses accumulator、计费与状态日志相关包 |
| 新增 drain/Responses 用例的 `go test -race` | 通过 | 覆盖 drain 完成、超时、并发满回退、Sub2API 终态 usage 与 cache 字段 |
| `go test ./... -count=1 -p=1` | 通过 | 全量串行测试 |
| `go vet ./...`、`go build ./...` | 通过 | 根模块 |
| `GOWORK=off go build ./...` | 通过 | `relaykit/` 独立模块；本轮未修改该模块 |
| `git diff --check`、UTF-8 解码及 U+FFFD 检查 | 通过 | 文档为有效 UTF-8，无替换字符 |
| `go test -race ./relay/helper ./relay/channel/openai ./service ./relay/common -count=1` | 未通过 | 全包 race 检查发现并行测试间 logger 全局输出状态竞争（堆栈涉及 `logger/logger.go:114`）；新增路径的定向 race 用例单独通过，不能将全量 race 标记为通过 |

本轮未更改数据库、价格表达式或生产服务，因此未运行数据库方言矩阵，也未作线上 usage/账单对账。提交后须等用户部署；部署侧不由本次提交自动执行。

## 最终审查结论

**当前结论：问题 1 已完成本地代码改造，但尚未由线上镜像验证。** type 59 的 HTTP Responses 客户端断开后现在默认最多 drain 120 秒、每进程最多 32 条；成功读到终态时 usage 会在同一 handler 返回并进入既有结算链路，超时/并发满仍有界回退到原估算。部署后仍需核对 `stream_status.end_reason=client_gone` 与 `response_status`/`drain_result` 以及实际消费日志，才能确认运行态收益。

**问题 2 当前不宣布已修复或已定位。** 源码已有标准 `input_tokens_details.cached_tokens` DTO、Responses accumulator 复制和 `p/cr` 拆分逻辑；现有测试也覆盖标准字段。用户更新后需提供同一 request ID 的 new-api 日志、脱敏终态 SSE 或 sub2api 转发日志，再判断字段是在上游未发出、转换丢失还是账务快照/日志展示丢失。未拿到该证据前不改 sub2api 或全局计价表达式。
