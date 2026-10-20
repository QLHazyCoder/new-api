# Responses 流断连与异常终态计费改造计划

> 当前状态（2026-10-20）：问题 1 的共享 drain 改造已在此前提交完成；本轮已完成问题 2 的明确失败估算修复。生产改动集中在 `service/responses_usage.go`；全量测试、定向 race、vet、构建、三数据库账务与 UTF-8 检查均已通过，尚未部署。具体方案及实施记录见本文末尾“2026-10-20：明确失败后的估算计费修复设计”。
> 历史基线：2026-10-17 的 drain 方案以 `d90827ab6` 为改造前基线，当时报告部署版本为 `5d48973b0`。本轮源码分析基线为 `0a758e1d7`，运行态核查与早期结论的更正以末尾新增章节为准；下方早期验证记录保留原日期和范围，不代表本次设计已实现或测试通过。

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

1. 改造前 `relay/helper/stream_scanner.go` 的主循环在 `c.Request.Context().Done()` 设置 `client_gone` 后直接执行 `cleanup()`；后者关闭 `resp.Body`、停 scanner 并等待 worker。因此后续 `response.completed` 的原生 usage 不可达。这是问题 1 在 new-api 的明确机制，但第三方在何种账户/协议路径继续计费仍须逐路径确认。
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

- 两个问题独立验收。问题 1 先统一共享 scanner 的生命周期；问题 2 仍按证据分支定位，避免无端改动全局计费代码。
- 所有调用 `StreamScannerHandler` 的流式路径统一支持客户端断开后的 drain，不判断渠道、客户端类型或端点。自定义 SSE/WebSocket 生命周期不经过该 scanner 的路径仍明确保持原行为。
- 计价合同采用“有上游权威 usage 时按该事实结算；缺失时保留并标记现有估算/争议流程”。`client_gone` 是下游交付事实，不等于上游成功；消费日志还须区分终态是否获得和计数来源。
- 保持已有 wallet、token、订阅/混合资金预扣与结算链路，不新建账务表、不改价格系数。更改下游取消后仍收实际生成费用需要运营方确认客户告知、申诉和欠费政策。

### 问题 1：共享 scanner 的限时 drain

1. 共享 `StreamScannerHandler` 读取 `CLIENT_GONE_DRAIN_TIMEOUT`，默认 60 秒，单位为秒；`0` 表示立即关闭上游，负值记录配置错误并回退默认 60 秒。配置值不再按端点/渠道分支，也不设置进程内并发上限或硬编码最大值。`STREAMING_TIMEOUT` 继续作为流式空闲超时；drain 另有不因每个 chunk 重置的单请求绝对时限。
2. 主循环观察到下游取消后记录 `end_reason=client_gone` 并停止 ping 和下游写入，但不调用 `stop()`、`cancel()` 或 `resp.Body.Close()`；scanner、data handler 和协议回调继续在当前请求流程内运行。`StreamStatus.TerminalReason()` 单独保留首个上游/协议终态，收到 `[DONE]`、EOF、读取错误或 handler stop 后只更新其内部终态分类，`end_reason` 仍保持 `client_gone`；随后 `drain_result` 记录 `ended`、`scanner_error` 或 `handler_stop`。只有 drain 被禁用/空闲超时/绝对超时且尚无 scanner 终态时，按相应结果结束并 cleanup。
3. 断开后 callback 仍调用 accumulator/协议解析逻辑（含终态、工具调用和 usage），公共 `ResponseChunkData`、`StringData` 以及共享路径上的 Gemini 写入在请求已断开时返回 nil/无操作，不通过 `sr.Stop` 提前截断 drain。非取消导致的真正写错误和上游解析失败仍按原协议处理。
4. `dataChan` 只由 scanner 关闭；handler 在 drain 期间继续消费，scanner 不会把 `gin.Context` 或 `RelayInfo` 的使用遗留到 handler 返回之后。当前请求的 `cleanup()` 负责关闭 body、cancel、停止计时器并 `wg.Wait`，完成后才回到 usage 结算。
5. `drain_result` 只记录传输结果，不声称 usage 一定权威。最终枚举为 `disabled`、`ended`、`timeout`、`idle_timeout`、`scanner_error`、`handler_stop`；删除中间态 `draining` 和不存在的 `limit_reached`。普通未断流请求不写该字段。`end_reason`、`response_status` 和 `drain_result` 独立记录，usage 是否估算继续由现有 `usage_billing_path` 判断。

`http.NewRequest` 仍未绑定下游 context，本轮没有改成 `http.NewRequestWithContext(c.Request.Context())`，因此客户端断开不会在 scanner 层之外提前杀死上游。drain 超时或其他终态仍通过当前请求流程的 `resp.Body.Close()` 和 `wg.Wait()` 完成清理；未引入脱离 handler 生命周期的后台请求。

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
| 经过共享 scanner 的 HTTP SSE（包括原生 Responses、Chat、Claude、Gemini、Dify、xAI、Baidu 等调用方） | 各协议回调 + 对应 usage/状态归集 | 本期统一启用 drain；逐协议回归下游断开后的解析和结算 |
| 非流式 Responses | `OaiResponsesHandler` 直接读 `response.usage` | 仅核对缓存解析回归，不实施 drain |
| 自定义 SSE、长轮询或其他不走共享 scanner 的路径 | 独立响应生命周期 | 保持原行为，不宣称本期覆盖 |
| `/v1/responses` WebSocket | `relay/responses_websocket.go` 独立会话生命周期 | 不受 HTTP scanner 方案自动覆盖；另行评估 |

主要风险：不设并发准入上限后，多个断连请求会同时占用上游连接、scanner、队列和内存；因此每个请求仍必须受绝对 drain 时限、既有 `STREAMING_TIMEOUT` 以及 `RELAY_TIMEOUT` 影响，运行态要监控连接数、goroutine 和内存。其他风险包括超时仍拿不到权威 usage、下游已断但用户仍被扣款的公平性与争议、上游明确失败/全零与估算混淆、预扣不足后的实际差额，以及诊断信息泄露 prompt/token。`RELAY_TIMEOUT` 保持现有全局 HTTP client 总超时语义：若早于 drain 到期，上游会先被它终止。特定 sub2api 路径可能在对 new-api 写失败时提前结束 SSE，即便它本身已累计/支付费用；那时本方案不能宣称 drain 必然解决所有损失。

## 文件结构设计

**只规划局部文件职责，不重排现有目录、不引入新的服务和数据库 schema。** 下表路径相对于 `/opt/qlh-main/new-api`；`保留` 表示无证据不修改，`条件` 表示仅在阶段 0 证据成立后修改。

| 路径 | 职责与拟调整原因 | 状态 |
| --- | --- | --- |
| `docs/responses-stream-usage-cache-billing-plan.md` | 本方案、证据分级、阶段清单和最终自检记录的唯一文档 | 本轮更新 |
| `relay/helper/stream_scanner.go` | 所有共享 SSE 调用方的断连后限时 drain、终态优先、worker/队列退出与资源清理；删除 options/并发准入 | 已修改 |
| `relay/helper/stream_scanner_test.go` | 覆盖立即关闭、[DONE]、EOF、绝对超时、空闲超时、scanner error、handler stop 与下游写无操作 | 已扩展 |
| `relay/common/stream_status_test.go` | 验证 `client_gone` 与首个 scanner terminal reason 分离 | 已扩展 |
| `relay/channel/openai/relay_responses.go` | 直接使用共享 scanner；断连后继续 `Observe`，不再判断 Sub2API/type 59 | 已修改 |
| `relay/channel/openai/relay_responses_billing_test.go` | 用真实 Responses SSE 覆盖任意渠道 handler 断开后终态 usage 回传 | 已扩展 |
| `relay/channel/openai/relay_image.go`、`relay/channel/openai/image_stream_test.go` | 以独立 scanner 终态确认 drain 后图像计数完整；未终止 drain 仍保留请求数量保护 | 已修改/扩展 |
| `relay/helper/common.go` | Responses/String 公共下游写入在 client gone 后无操作，避免取消错误触发 scanner stop | 已修改 |
| `relay/channel/openai/helper.go`、`relay/channel/openai/chat_via_responses.go`、`relay/channel/claude/relay-claude.go` | 把共享 scanner 调用链内 Gemini SSE 直写改为公共写 helper，确保断连不截断解析 | 已修改 |
| `service/log_info_generate.go` | 已有 `drain_result` 输出，本轮保留；不改变日志 schema | 保留并验证 |
| `service/log_info_generate_test.go` | 验证 `end_reason`、`response_status`、`drain_result` 独立记录且正常请求省略 drain 字段 | 已新增 |
| `service/responses_usage.go` | 根据抓包确认权威终态 usage 的有无，显式零与估算分离 | 条件修改 |
| `service/responses_usage_test.go` | 有 usage、缺 usage、全零、失败/incomplete 和 tool 用量回归 | 条件扩展 |
| `constant/env.go`、`common/init.go`、`.env.example` | 声明/解析 `CLIENT_GONE_DRAIN_TIMEOUT`，默认 60，0 关闭；无并发上限和硬上限 | 已修改 |
| `relay/common/stream_status.go` | 保留 `drain_result` 审计字段；独立保存首次 scanner 终态，公开日志分别输出 end reason、response outcome 和 drain result | 已修改 |
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
| 0-C | 复现不同 sub2api 账户平台在下游断开后的继续生成/推送行为；量测断开后至终态的 P95/P99、取消比例、连接数与最大内存 | 待授权 | 验证 60 秒默认值是否适合运行态；若上游不能返回 usage，转上游修复/对账方案 |
| 1-A | 在共享 scanner 实现全调用方可用的限时 drain；明确 cleanup、`dataChan` 关闭、scanner/handler/ping 退出先后 | 已完成（本地） | 默认值 60 秒；0 立即关闭；无终态等待绝对时限；客户端不再接收断开后的事件 |
| 1-B | 给断连/终态两个维度增加可审计结果；核对资金结算仅发生一次，预扣在延迟期间保留 | 已完成（代码/单测） | `client_gone`、`response_status`、`drain_result` 分离；实际预扣/结算仍需部署后日志对账 |
| 2-A | 仅对抓包确定的缓存丢失层修改字段映射，必要时在 sub2api 相应路径补出站终态 | 待阶段 0 证据 | 标准字段从 SSE 到消费日志均为同一正值；缺字段绝不假称命中 |
| 2-B | 确认并修复“明确全零 usage”被本地 prompt 回填；保持“无权威 usage”的现有估算/审计策略 | 待阶段 0 证据 | 显式全零应为零 token 计价（额外固定价/工具费按既有契约）；缺失 usage 有清楚来源标记 |
| 3-A | 测试：断开后 `[DONE]`/EOF/绝对超时/空闲超时/read error/handler stop、0 关闭、同时就绪终态优先、Responses 终态 usage、日志字段独立 | 已完成（本地） | 共享路径定向用例和 race 通过；全量共享包 race 存在 logger 全局状态竞争，见验证记录 |
| 3-B | 回归所有共享 scanner 调用方；明确自定义 SSE/WebSocket 未覆盖；确认缓存定价无改动 | 已完成（源码/全量测试） | 所有共享调用方均走同一 scanner；逐协议业务集成仍需部署日志核实；价格表达式和 WebSocket 未改 |
| 4-A | 本地验收、审查文件清单与本方案逐项状态；不自动部署 | 已完成（本地） | 全量串行测试、定向 race、vet、build、diff/UTF-8 检查通过；未做线上 usage/账单对账 |
| 4-B | **另行确认**灰度和生产发布：低流量单组试运行、核心指标观察、异常回退到 0、运行镜像/配置回读 | 需单独上线授权 | 经授权后按实际生产窗口与回滚预案执行；若有 DB 修改必须跑 SQLite/MySQL/PostgreSQL 实例矩阵 |

验证命令：`go test ./... -count=1 -p=1`、定向 `go test -race`、`go vet ./...`、`go build ./...`、`git diff --check`；文档另做 UTF-8 解码和 U+FFFD 检查。触及 `relaykit/` 时另在该目录运行 `GOWORK=off go build ./...` 及相应测试。全量 race 如受既有全局 logger 测试竞争影响，必须报告失败原因，不得将其标记为通过。

## 阶段自检记录

| 阶段 | 逻辑/依赖/遗漏/文档自检 | 已发现问题与拟处理动作 | 状态 |
| --- | --- | --- | --- |
| 方案前 | 对照部署提交 `5d48973b0` 和当前主仓库，读取 billing/expr 规范，核实 scanner、type 59、Responses、settlement 与 sub2api 多路径源码 | 纠正“type 59 不走 accumulator”“已合并 #4500”“表达式还要手减一次 cr”；分离运营日志与上游线缆事实 | 已完成，仅源码/提供案例 |
| 阶段 0 | 回查部署、对应请求和脱敏 SSE，核验计费字段一致性及超时预算 | 当前没有线上抓包/运行态对账，不提前认定缓存字段具体丢失点、sub2api 路径或全零 SSE | 未开始 |
| 阶段 1 | 检查 scanner 的 stop/body/dataChan、writer/ping、Gin context 与一次结算，更新文件映射 | 删除 options、渠道/端点分支和并发准入；默认 60 秒，0 立即关闭；完成/超时均等待后 cleanup；加入最终 `drain_result` | 已完成（本地） |
| 阶段 2 | 检查标准缓存字段与显式零，回归协议/DTO/表达式三个口径 | 任何额外字段兼容/上游改动均需阶段 0 原始形态证据 | 未开始 |
| 阶段 3-4 | 核验测试结果、变更文件与本计划逐项覆盖，补充验收证据与残余风险 | 全量串行测试、定向 race、vet、build、diff/UTF-8 检查均通过；全量 race 记录既有 logger 竞争；未宣称线上 usage 或账务已修复 | 已完成（本地） |

### 本轮验证记录

| 命令/检查 | 结果 | 说明 |
| --- | --- | --- |
| `go test ./relay/helper ./relay/channel/openai ./relay/common ./service -count=1` | 通过 | 覆盖共享 scanner、Responses accumulator、drain 日志元数据 |
| `go test -race ./relay/helper ./relay/channel/openai -run 'TestClientGoneDrainDuration|TestStreamScannerHandler_(ClientCancel|Drain)|TestStreamDataWritersIgnoreClientGone|TestOaiResponsesStreamHandlerDrainsTerminalUsageAfterClientGone|TestOpenaiImageStreamHandlerDrainsCompletedCountAfterClientDisconnect|TestOpenaiImageStreamHandlerClientDisconnectKeepsRequestedCount' -count=1` | 通过 | 新增 drain、timeout 边界、Responses、图像计数及公共 writer 断连路径无 race |
| `go test -race ./relay/helper ./relay/channel/openai ./relay/common -count=1` | 未通过（既有竞争） | Race detector 报告并行测试的 `logger.logHelper` 全局状态竞争（`logger/logger.go:114-115`）；定向 drain 用例单独 race 通过 |
| `go test ./... -count=1 -p=1` | 通过 | 全量串行测试 |
| `go vet ./...`、`go build ./...` | 通过 | 根模块 |
| `git diff --check`、UTF-8 解码及 U+FFFD 检查 | 通过 | 无空白错误，文档 UTF-8 有效且无替换字符 |

一次较早的全量串行测试曾报告 `controller` 包失败，但当次输出未保留具体失败用例；随后 `go test ./controller -count=1` 与再次运行的完整测试套件均通过。未确认初次失败原因，不据此归因。

本轮未更改数据库、价格表达式或生产服务，因此未运行数据库方言矩阵，也未作线上 usage/账单对账。提交推送后仍须等用户部署；部署侧不由本次提交自动执行。

## 最终审查结论

**2026-10-17 的审查结论：问题 1 已完成本地代码改造。** 所有共享 `StreamScannerHandler` 流式路径使用 `CLIENT_GONE_DRAIN_TIMEOUT`（默认 60 秒，0 立即关闭），无渠道/端点白名单、无并发上限、无硬编码最大值。成功读到终态时 usage 在同一 handler 返回并进入既有结算链路；`drain_result` 只说明传输最终如何结束，不代表 usage 一定权威。该结论及前面的命令结果属于此前 drain 实施阶段，本次不重复修改或验收 scanner。

**问题 2 的旧结论由下面的新设计取代。** 标准缓存字段解析和表达式拆分已存在；本轮确认需要优先修复的是明确失败流仍生成本地 token 估算。历史请求的跨站对应关系必须区分已核实 token/渠道对应与仅时间接近的候选，不能据早期对账推断每一条失败流都在上游成功出账。

## 2026-10-20：明确失败后的估算计费修复设计

### 项目整体分析与证据边界

本节对应当前未处理的问题：上游明确返回失败且没有 usage，new-api 仍把输出片段估成非零 token，继而补入整段本地输入估算。`cache_tokens=0` 是这条本地估算分支缺少缓存事实的结果，不能据此断言缓存解析器缺失。此前查到的成功流已证明同类渠道可以正确记录缓存与输出。

源码调用链已经完整核对：

```text
type 59 sub2api.Adaptor -> newapi.Adaptor -> openai.Adaptor
  -> OaiResponsesStreamHandler -> ResponsesUsageAccumulator.Observe
  -> ResponsesUsageAccumulator.Finish
  -> ConsumeResponsesQuota -> PostTextConsumeQuota
  -> effectiveBillingUsage -> BuildTieredTokenParams -> SettleBilling
  -> BillingSession.Settle/settle -> 资金与令牌差额、消费日志

Responses WebSocket 每轮请求也使用同一个 ResponsesUsageAccumulator。
```

资金处理沿用已有一次性结算，不需要另建退款路径。HTTP SSE 已发出失败终态时，handler 返回 `usage, nil` 是避免重复发送错误、重试已输出流的既有契约，不等于协议成功。日志失败状态来自 `StreamStatus`。

当前缺陷及相邻边界：

1. `ObserveResponsesOutcome()` 已识别 `response.failed`、`response.error`、`error` 和 `response.status=failed`，但只写 `StreamStatus`；没有 `StreamStatus` 的调用方无法通过它保留失败事实。
2. `Finish()` 在判断失败之前，先对任何非空输出字符串调用 `tokenkit.Count()`。一个空格和一小段真实文本都可能得到 1；`completion_tokens=1` 本身不能用来判断失败或免单。
3. `billsPrompt := completion != 0 || (started && !failed)` 的第一个条件绕过失败判断。估算输出只要为正，即使明确失败，也补入完整预估输入。
4. `CloneBillingUsageWithEstimatedCompletion()` 还可能把真实 usage 中的零输出补成估算值。失败分支必须在它之前结束，而非只改 prompt 条件。
5. `ApplyResponsesUsage()` 使用 `MergeUsageNonZero()`；其语义是累计非零上游快照，零值不覆盖更早的正值。共享 `BillingUsage` 构造和 canonical 层也故意拒绝全零 sidecar。不能为一个失败流补丁全局改变这些约定。
6. `usage_billing_path` 位于 `other.admin_info`。没有 sidecar 且没有设置本地估算 context 标记时，它会显示 `upstream`；因此这个值目前不是“已获得真实 usage”的充分证据。
7. 上次用来对照的若干 Seoyh token 数，在本地对应其他成功请求，不能继续作为该失败请求真实出账的证明。跨服务缺少共同 request ID 时，同秒、模型和 key 的唯一候选只记作关联证据，不升级为严格同一请求证明。

### 官方 issue / PR 核对

截至本轮查询，GitHub 元数据、diff 和当前官方 `main` 的相关文件提交历史已核对：

| 来源 | 状态 | 与本次设计的关系 |
| --- | --- | --- |
| [PR #7440](https://github.com/QuantumNous/new-api/pull/7440)，提交 `c00de89f8812866d56dec230949e97c6b9cd6c55` | 2026-09-18 关闭，`mergedAt=null`；无解释关闭原因的人类评论 | 直接描述本问题，提出明确失败后不再估算输入/输出，保留已累计的 provider usage，包括零值；不是已发布的官方修复 |
| [PR #7427](https://github.com/QuantumNous/new-api/pull/7427)，合并提交 `55a6cd2a44eaf189bbd565dd2cdfa591d19b06d6` | 2026-09-18 已合并，本地已经包含 | 是当前截断流估算路径的来源；保留普通断连、incomplete 等没有明确失败时的既有估算 |
| [PR #7419](https://github.com/QuantumNous/new-api/pull/7419) | 仍开放，未合并 | 描述 Responses 本地估算被标成 upstream 的可观测性缺口；可独立参考，不整体引入其 scanner/WebSocket/前端重写 |
| [Issue #6594](https://github.com/QuantumNous/new-api/issues/6594) | 仍开放 | 讨论失败终态、EOF 与状态分类；维护者说明流已输出后的重试需由客户端执行，不能据此添加服务端透明重放 |
| [PR #7242](https://github.com/QuantumNous/new-api/pull/7242) | 已关闭，未合并 | 强调 failed/incomplete/cancelled 终态也可能携带真实 usage；失败不能一律清空已收到的用量 |

#7440 的作者不是维护者；关闭未合并不能解读为官方认可、官方已修复或已知被其他提交替代。本轮查到的官方 `service/responses_usage.go` 提交历史未出现该修复，当前本地代码也没有相应的失败估算短路。

### 推荐修复方案

采用 #7440 的明确失败规则，取代之前“仅过滤空白、真实部分文本仍估算”的初步建议。是否推算费用由明确失败事实决定，不引入“文本够不够有意义”的内容判断：

| 请求事实 | 本次 token 处理 | 对当前场景的影响 |
| --- | --- | --- |
| 明确失败，没有 usage，输出为空、空格、文字、推理或函数参数 | 返回已累计的上游 token；没有累计值即 0/0，不新增估算 | 不再由一个空格触发整段输入收费；有部分真实文本也适用同一规则 |
| 明确失败，usage 全零且此前没有正值上游快照 | 保留 0/0，跳过估算 | 不把明确零输出或零输入改成估算值 |
| 明确失败，usage 仅输入或仅输出为正 | 只保留已报告部分，不估算另一侧 | input=20/output=0 保持 20/0；input=0/output=5 保持 0/5 |
| 明确失败，完整 usage 含缓存 | 保留其输入、输出、缓存与原有 BillingUsage | 按已有价格与自动缓存拆分结算 |
| 明确失败前已经累计正值上游 usage，末次 usage 缺失或为零 | 保留现有累计非零快照，不新增估算 | 延续共享 merge 契约，不用晚到的缺项抹除已报告用量 |
| 普通 client gone、EOF、timeout，没有明确失败事件 | 沿用既有 drain 和估算规则 | 不将已修复的问题 1 改回提前取消或全额免单 |
| completed/incomplete/cancelled，没有明确失败事件 | 本次维持已有规则 | 成功全零 usage 与缺字段区分属于后续独立事项，不能宣称本补丁一起解决 |
| 请求失败，但已完成图片或已发生独立计价工具调用 | token 适用上面的失败规则，工具/图片计数按既有规则处理 | 零 token 不等于工具费用为零 |

上表的零费用结论仅适用于本次核查的纯 token 定价且无独立工具费用的请求。`fixed()`/旧按次价格属于独立合同，尤其 `PostTextConsumeQuota()` 可对零 token 计算按次费用；本补丁不把所有定价的失败请求一律免单，也不更改表达式出错时保留预扣的既有约定。

具体实现顺序：

1. 在 `ResponsesUsageAccumulator` 增加单请求 `failed bool`。复用现有事件分类，`Observe` 每次以 OR 累计这个事实，不依赖 `StreamStatus` 是否已初始化；不使用 HTTP 状态、completion 是否为 1、渠道 type 或客户端类型作判定。
2. 将现有失败判定提取成有稳定协议语义的内部函数/返回值，`ObserveResponsesOutcome` 的既有对外接口保留；日志与计费必须使用同一判定，避免两套条件漂移。
3. `Observe` 继续解析 usage、工具和图片；不能在第一次看到失败时停止 scanner 或直接退出整个 handler。对识别为失败且 `event.Response.Usage != nil` 的事件，应确保 usage 进入累计：现有 switch 没有单独涵盖的 `response.error`/`error` 携带 response.usage、或仅 `status=failed` 的形态也要回归，不能判失败却漏掉其中的真实 usage。不猜测支持未核实的平铺 usage 别名。
4. `Finish()` 先执行原有图片/工具提交，再在 token 估算之前检查 `failed`：失败时返回已累计 usage，跳过 `tokenkit.Count`、本地 prompt 回填和 `CloneBillingUsageWithEstimatedCompletion`。保持重复调用 `Finish` 的幂等性。
5. HTTP Responses handler 保持失败帧单次透传和 `usage, nil` 返回契约，不追加 JSON 错误、不补成功终态、不触发透明重试。消费日志继续有 `response_status=failed`，`eof` 等传输原因独立保留。
6. 对当前纯 token 表达式验证：零 usage 得到 quota=0，经现有 `SettleBilling(0)` 结算与预扣相减并返还；不另调用一次 `Refund()`。已有真实 usage 则照常计价，不能清零后再退款。价格、`cr` 自动拆分和资金事务代码保持原实现。

实现示意（展示执行顺序，不是可直接替换的补丁）：

```go
// Observe: derive explicitFailure using the shared protocol classifier.
a.failed = a.failed || explicitFailure
// Continue accumulating provider usage, completed images and tool facts.

// Finish: retain the existing finished guard and image/tool commit first.
if a.failed {
    return a.usage
}
// The existing missing-token estimation follows only for non-failed streams.
```

### 文件结构与覆盖范围

| 文件 | 计划动作/职责 |
| --- | --- |
| `service/responses_usage.go` | 唯一改动的生产文件：累计明确失败事实，收齐失败事件已有 usage，在 Finish 跳过 token 推算 |
| `service/responses_usage_test.go` | 在已有文件补表格回归；修改“failed/input-only usage 应补估输出”的旧断言 |
| `relay/channel/openai/relay_responses_billing_test.go` | 在已有 HTTP SSE fixture 验证真实协议事件、失败帧单次透传、数值与 BillingUsage 保留 |
| `relay/responses_websocket.go`、`relay/responses_websocket_test.go` | 已共用 accumulator；核对每轮重建状态和控制帧隔离，优先运行已有测试，不改变 WebSocket 断连生命周期 |
| `controller/responses_websocket_test.go` | 更新已有 HTTP/WebSocket 端到端失败计费断言，补空白与显式零场景；独立初始化 i18n，保留连接复用、后续成功请求与限流回归 |
| `service/text_quota.go`、`service/tiered_settle.go`、`service/text_quota_test.go` | 审查零 token 如何结算/保留工具费/固定价；复用已有计费 fixture，必要时在原测试文件追加少量端到端断言，不重构结算层 |
| `relay/helper/stream_scanner.go`、`relay/common/stream_status.go` | 已完成的 drain 与协议分类能力，本次无实现调整 |
| `relaykit/dto/usage_merge.go`、`relaykit/dto/billing_usage.go` | 维持已有累计/sidecar 规则，本次不全局改变显式零合并语义 |
| `docs/responses-stream-usage-cache-billing-plan.md` | 保存最新设计、实施清单、复现证据与实际验证状态 |

直接覆盖 type 59 原生 Responses 与其他共用原生 Responses handler 的渠道，以及使用同一 accumulator 的 Responses WebSocket 每轮 token 结算。Chat↔Responses 转换使用独立的 `chat_via_responses.go`/`responses_via_chat.go`，目前也有部分输出估算分支，但不因修改 accumulator 自动获得修复；若要统一它们的失败计费政策，应单独扩展范围和对应协议回归。Claude/Gemini 原生协议、自定义 SSE 同样不能宣称已经覆盖。

### 开发清单与验收

| 阶段 | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| F0 | 阅读 AGENTS/billing/表达式规范；核对失败分类、sidecar、结算、预扣退款与上游 PR diff | 已完成（源码审查） | 有完整调用链，PR 的合并状态真实可查 |
| F1 | 实现单请求失败事实及 Finish 估算短路，补齐有 response.usage 的失败形态 | 已完成 | 明确失败时不生成新 token 估算；有 StreamStatus 和无 StreamStatus 的用例结果一致 |
| F2 | 扩展已有 accumulator/HTTP SSE 回归 | 已完成 | 缺失、null、全零、部分、完整 usage；空白、文字、reasoning、工具参数均覆盖；失败帧只发一次 |
| F3 | 核对现有账务、WebSocket、drain、工具/图像计费回归 | 已通过 | 三种真实数据库覆盖预扣差额、零费用、真实缓存用量、工具费和固定价；service/OpenAI/relay 包及 controller 的 HTTP/WebSocket 入口用例已通过 |
| F4 | 完整检查、更新文档、选择性提交 | 已完成，随本提交交付 | 全量测试、定向 race、vet、构建、diff/UTF-8 均通过，实际命令见下表；生产部署尚未执行 |

关键回归向量：

- `created -> delta(" ") -> failed(no usage)`：预估输入为 80,505 时返回 0/0。
- `created -> delta("hello") -> failed(no usage)`：仍返回 0/0，防止修复退化为只 trim 空格。
- failed 明确 `usage={input_tokens:0,output_tokens:0}`：0/0；input=20/output=0 不补输出，input=0/output=5 不补输入。
- failed usage 含 `input_tokens_details.cached_tokens` 与正值 BillingUsage：计费层得到原缓存和数值，Estimated 不会被本地补算置真。
- `response.failed`、`response.error`、平铺 `error`、`response.done` 携带 `status=failed`；有 response.usage 时均不遗漏。
- 普通断连后 `completed` 带真实 usage：仍按 drain 收到的 usage；普通无终态且无明确失败：原估算保留。
- failed 前已完成 image_generation/计价工具：计数保留且只提交一次；未完成图片不新计费。
- 同一 accumulator 的二次 Finish 不重复提交；下一轮 WebSocket 创建新 accumulator，前一轮失败不污染下一轮。
- 真实输出确实为 1 且上游 usage 正常的成功请求保持原计费，不能按 completion=1 批量退款或重标状态。

验收执行 `go test ./service ./relay/channel/openai ./relay -count=1 -p=1`、相关用例定向 `go test -race`、`go test ./... -count=1 -p=1`、`go vet ./...`、`go build ./...`、`git diff --check` 及文档 UTF-8 检查，实际进展见下方实施记录。测试扩展集中到已有适合文件；若实际需要改事务/SQL/DB 配置，必须追加真实 SQLite/MySQL/PostgreSQL 矩阵，不能以 mock 代替。

### 阶段自检与最终设计结论

本轮已确认：失败分类已存在，缺口在估算准入；只改 prompt 条件或过滤空白不足；只在 handler 抛错误可能造成重复错误/重试；直接返回 nil usage 会触发结算层的输入回填，也不合适。应该保留非 nil 的已累计 usage，经原结算通道处理。

本轮未加入新数据库字段、定价配置、渠道白名单、估算阈值或上游重试。#7440 需要按当前 `Observe(event, raw)`、tokenkit 和工具/图片去重接口手工适配，不直接套用旧 diff。

需要单列的后续事项：

1. 成功/incomplete/cancelled 的显式零 usage，以及终态零是否应覆盖更早正值快照，需要字段存在性与快照语义设计；本次只停止明确失败后的新估算，不重写共享 DTO 的零值语义。
2. 原生 Responses 缺 usage 的普通估算当前可能被 `usage_billing_path` 标成 upstream。可参考 #7419 单独补准确来源标记；不能将它的多文件传输改造整体并入本次修复，也不能只靠当前该字段判定是否有权威 usage。
3. 自建 sub2api 的失败终态脱敏会删除 `response.usage`，而其内部可能已解析该字段；即便 new-api 不再估算，也无法恢复被删数据。若同一请求已证明上游报告了 usage，应在 sub2api 失败终态中保留经过校验的数值 usage 明细，不恢复 prompt/output 内容；当前已查失败终态原本就无 usage 的样本不能靠这项改动凭空恢复数据。
4. 不估算明确失败流，可能少收失败前已有部分生成而上游没有报告 usage 的成本。这是推荐方案的实际取舍；可靠计费应由上游返回 usage 支持，不用文本是否非空替代出账证据。
5. 历史 completion=1 包含多种原因，不批量重算/退款；如需追溯账务，另按请求证据制定范围。

### 本轮实施与自检记录

| 检查 | 当前结果 | 证据/范围 |
| --- | --- | --- |
| 修改前复现 | 已复现，预期失败 | `GOMAXPROCS=2 go test ./service -run 'TestResponsesUsageAccumulatorExplicitFailureDoesNotEstimate/failed_without_usage/with_stream_status' -count=1 -p=1`：期望 0/0，旧代码实际 80505/1 |
| 核心修复 | 已实现 | `failed` 独立于 StreamStatus；在 token 推算前返回上游 usage；error/status=failed 中已有 response.usage 也进入累计 |
| 工具/图像审查 | 已通过 | 裸 `error` 可以先于 `response.failed`；保留原有图像终态提交时机，避免提前提交空计数后遗漏失败终态内已完成图片；新增回归覆盖该序列及二次 Finish |
| 定向 usage/HTTP/账务回归 | 已通过 | 现有测试文件补充 34 个 failure/status 向量、9 个 HTTP SSE 场景及 7 个实际账务场景，另有工具/图像终态序列、已有正值上游快照不被缺失/全零终态抹除的回归 |
| `GOMAXPROCS=2 go test ./service ./relay/channel/openai ./relay -count=1 -p=1` | 已通过 | service、OpenAI relay、Responses WebSocket 所在 relay 包 |
| `GOMAXPROCS=2 go test ./controller -run '^TestResponsesStreamOutcomesPreserveAccounting$' -count=1 -p=1` | 已通过 | 20 个 HTTP/WebSocket 入口场景；失败后同连接的下一次成功请求仍结算，随后按成功请求限流 |
| `GOMAXPROCS=2 go test ./service -run '^TestFixedPriceBillingDatabaseMatrix$' -count=1 -p=1 -v`，配置独立测试 DSN | 已通过 | SQLite 3.50.4、MySQL 8.2.0、PostgreSQL 18.4；临时库 `newapi_failed_usage_cbbccf31ec97` 分别在 MySQL/PostgreSQL 新建，测试完成后均已删除，未连接业务库跑测试 |
| 首轮全量检查的失败处理 | 已定位并修正测试合同 | 原 controller `failed-estimated-output` 用例明确期望失败流仍按估算输出收费；按批准方案更新为零，并保留所有后续请求/资金/限流断言。单独运行另发现该用例依赖其他测试先初始化 i18n，已显式初始化，未改 controller 生产逻辑 |
| `GOMAXPROCS=2 go test ./... -count=1 -p=1`（使用 `-json` 收集结果） | 已通过 | 48 个有测试的包通过，无失败；包含更新后的 controller 端到端用例 |
| `GOMAXPROCS=2 go test -race ./service ./relay/channel/openai ./relay/helper ./controller -run 'TestResponsesUsageAccumulator\|TestObserveResponsesOutcome\|TestApplyResponsesUsage\|TestOaiResponsesStreamHandlerExplicitFailureUsage\|TestOaiResponsesStreamHandlerDrainsTerminalUsageAfterClientGone\|TestStreamScannerHandler_(ClientCancel\|Drain)\|TestStreamDataWritersIgnoreClientGone\|TestResponsesStreamOutcomesPreserveAccounting' -count=1 -p=1` | 已通过 | 四个包全部通过，无 race 报告；涵盖 usage、原有 drain 与 HTTP/WebSocket 入口 |
| `GOMAXPROCS=2 go vet -p=1 ./...`、`GOMAXPROCS=2 go build -p=1 ./...` | 已通过 | 根模块静态检查和构建 |
| relaykit 内 `GOWORK=off go build ./...`、`GOWORK=off go test ./... -count=1 -p=1` | 已通过 | 独立协议模块未作修改，独立构建和测试通过 |
| `git diff --check`、所有六个修改文件严格 UTF-8 解码及 U+FFFD 检查 | 已通过 | 无空白错误、无非法 UTF-8、无替换字符 |

**最终代码审查结论：明确失败后停止新增 token 估算，保留已报告 usage、缓存及已完成图片/工具计数；不修改失败帧透传、重试、drain 和价格合同。修改前 80505/1 的回归样本在本方案下为 0/0，三数据库已验证零 token 结算返还预扣且重复结算不重复扣费。完整本地验收通过，尚未部署；线上仅在运行该提交后的新请求生效，历史日志和账务不自动改写。**
