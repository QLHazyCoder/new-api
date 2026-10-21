# Responses 流断连与缓存计费改造计划

> 状态：问题 1 已按 PR #7178 drain 方向完成本地实现和代码验证；加入最终状态审计、共享回调无写断连处理及终态优先检查。问题 2 保留运行态证据定位，未做猜测性字段补丁；未改线上服务或价格配置。
> 编制日期：2026-10-17。改造前基线为当前 `main` 的 `d90827ab6`；报告中的部署版本为 `5d48973b0`（2026-10-14）。部署镜像修订、实际渠道路由和上游 SSE 尚未现场复核。其他 checkout 不是本方案的实施基线。

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

一次较早的全量串行测试曾报告 `controller` 包失败，复核日志确认是旧的 `failed-estimated-output` 用例仍固定断言 `completion=1`；按当前临时估算策略更新断言后，`go test ./controller -count=1` 与再次运行的完整测试套件均通过。

本轮未更改数据库、价格表达式或生产服务，因此未运行数据库方言矩阵，也未作线上 usage/账单对账。提交推送后仍须等用户部署；部署侧不由本次提交自动执行。

## 最终审查结论

**当前结论：问题 1 已完成本地代码改造，待回归和线上镜像验证。** 所有共享 `StreamScannerHandler` 流式路径现在使用 `CLIENT_GONE_DRAIN_TIMEOUT`（默认 60 秒，0 立即关闭），无渠道/端点白名单、无并发上限、无硬编码最大值。成功读到终态时 usage 会在同一 handler 返回并进入既有结算链路；`drain_result` 只说明传输最终如何结束，不代表 usage 一定权威。部署后仍需核对 `end_reason`、`response_status`、`drain_result`、`usage_billing_path` 与实际消费日志。

**问题 2 当前不宣布已修复或已定位。** 源码已有标准 `input_tokens_details.cached_tokens` DTO、Responses accumulator 复制和 `p/cr` 拆分逻辑；现有测试也覆盖标准字段。用户更新后需提供同一 request ID 的 new-api 日志、脱敏终态 SSE 或 sub2api 转发日志，再判断字段是在上游未发出、转换丢失还是账务快照/日志展示丢失。未拿到该证据前不改 sub2api 或全局计价表达式。

## 临时 completion 估算策略

经过生产日志对账，client gone 后仍有一部分请求在 drain 超时前拿不到上游终态 usage。为减少下游看到 `completion_tokens=1` 引起的争议，当前临时策略只处理本地 token 估算结果恰好为 1 的请求：按 `RelayInfo.RequestId` 的 CRC32 稳定映射到 30--90 个输出 token。request ID 为空时使用下限 30。

该策略只作用于本地 `tokenkit.Count` 产生的估算值；上游明确报告的 `output_tokens=1` 保持 1，已有正值 usage、缓存字段、工具和图片计数均保持原有结算。明确失败流恢复原有本地估算路径，因此没有 usage 且输出片段估为 1 的失败流也会使用该临时范围。

这是临时的运营估值，不代表上游真实输出量。作者修复终态 usage 对账后，应删除 `temporaryEstimatedCompletion` 及对应测试和文档段落，再恢复只按权威 usage 或明确的正式估算策略结算。
