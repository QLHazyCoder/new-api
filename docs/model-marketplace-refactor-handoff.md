# 模型广场与媒体能力重构交接说明

> 文档用途：给后续负责模型广场、图片 Playground、插件端点和动态定价的 AI 或工程师使用。
> 本文不是一次性需求描述，而是当前系统的事实、历史设计、已知问题、目标架构和验收入口。
> 任何改动都应先核对本文所列的实际调用链，再决定修改位置。

- 盘点日期：2026-09-29
- 代码目录：`/opt/qlh-main/new-api`
- 相关旧记录：`docs/model-catalog-endpoint-capabilities.md`
- 图片配置说明：`docs/image-capability-configuration.md`
- 插件契约：`docs/plugin-api/v1.md`、`docs/plugin-api/v1.d.ts`
- 自研功能保护：`docs/custom-feature-preservation-checklist.md`，尤其是 P-13
- 当前状态：模型广场端点/能力问题仍需要继续分析；本文不代表新-api 镜像已经部署到线上。

## 项目整体分析

### 1. 业务目标

模型广场不是单纯的模型名称列表。用户在模型详情页看到的内容至少包括：

1. 模型是否实际可用。
2. 模型属于哪些公开分组，以及当前用户能否看到这些分组。
3. 模型支持哪些公开 API 端点。
4. 每个端点的正确路径、HTTP 方法、请求示例和参数说明。
5. 图片模型的尺寸、比例、分辨率、质量、输出格式、编辑能力等能力。
6. 视频模型的创建、查询、内容下载流程，以及与模型绑定的定价用量字段。
7. 计费表达式、用量单位和不同插件供应商的定价变体。

这些信息必须和真实路由一致。模型名称、模型元数据、渠道类型、渠道模型映射、插件协议、图片能力文件和定价配置分别属于不同事实源，不能用其中一个事实源猜测另外一个。

### 2. 端到端数据流

```text
channels + abilities
  + channel.model_mapping
  + channel.setting / task plugin binding
  + enabled channel status
  + task-plugin routing generation
  + models metadata (description/icon/tags/endpoints/name_rule/status)
  + ratio/billing options
  + image-capabilities.json
        |
        v
model.GetPricing()
        |
        +--> supported_endpoint_types (per public model)
        +--> supported_endpoint (endpoint type -> default/custom path)
        +--> image_capabilities (mapped model + channel rules)
        +--> billing_usage_schema / billing_usage_examples
        +--> groups, vendor, price, metadata
        |
        v
GET /api/pricing
        |
        v
web/src/features/pricing
```

另外还有两条相近但不能直接混用的链路：

```text
user group -> enabled abilities in that group
           -> service.GetUserImageModelGroups()
           -> GET /api/user/image-models
           -> Playground image model options

user/token group -> concrete model names
                 -> controller.buildUserModelOptions()
                 -> GET /api/user/models?with_endpoint_types=true
                 -> Chat/Responses 等模型选择器
```

当前 `/api/user/models` 的端点字段通过全局 `model.GetModelSupportEndpointTypes(modelName)` 读取，而不是在用户分组范围内重新解析每个 ability；这是需要重点复核的潜在范围泄漏点。

### 3. 名称和事实源必须分层

同一个请求可能同时出现以下名称：

| 名称 | 含义 | 是否可以直接显示给用户 |
| --- | --- | --- |
| 公开模型名 | 客户端请求中的 `model`，也是模型广场主键 | 可以，必须保持原样 |
| ability 模型名 | `abilities.model` 中绑定渠道的公开名称 | 通常与公开模型名相同 |
| 上游模型名 | `channels.model_mapping` 解析后的厂商名称 | 不应替换客户端请求名 |
| 插件声明模型 | `meta.models` 中允许插件处理的模型 | 只用于插件绑定/路由/能力声明 |
| 模型元数据名 | `models.model_name`，可能是 exact/prefix/contains/suffix 规则 | 只能作为元数据覆盖规则 |
| 计费目标名 | 定价配置或 `usageProfiles` 选中的声明模型 | 只用于计费和参数校验 |

图片能力可以使用上游映射名查找规则，但返回给客户端的 `model` 仍然必须是公开模型名。插件可以把公开模型转换成厂商请求体，但不能让模型广场把厂商名当成公开模型名。

### 4. 公开端点与插件内部协议

公开端点名由 `relaykit/types/endpoint_type.go` 和 `constant` 体系定义。当前相关端点如下：

| 公开端点类型 | 默认路径 | 插件内部协议 | 典型用途 |
| --- | --- | --- | --- |
| `openai` | `POST /v1/chat/completions` | 无 | Chat Completions |
| `openai-response` | `POST /v1/responses` | `openai_responses` | Responses |
| `image-generation` | `POST /v1/images/generations` | `openai_image` | OpenAI 图片生成 |
| `openai-video` | `POST /v1/videos` | `openai_video` | OpenAI 兼容视频创建 |
| `embeddings` | `POST /v1/embeddings` | 无 | Embeddings |

硬性规则：

- `openai-video` 是模型广场、`/api/pricing`、模型元数据中对外使用的规范键。
- `openai_video` 是插件运行时协议名，必须继续保留在 `meta.protocols`、`protocols.openai_video`、路由注册和任务持久化内部逻辑中。
- 不能全局替换 `openai_video`。正确做法是边界转换：内部协议解析为公开端点，公开元数据拒绝写入内部协议名。
- `common/endpoint_defaults.go` 已为 `openai-video` 提供 `/v1/videos` 默认路径；标准端点路径属于主机契约，不应由普通模型元数据任意覆盖。

### 5. Task Plugin 的两个端点层

Task Plugin v1 有两种完全不同的注册方式：

1. `meta.routes`：插件拥有厂商原生路径，例如 `/vendor/v1/jobs`。插件声明 decode/render，主机负责鉴权、请求、重试、持久化和轮询。
2. `meta.protocols`：插件声明自己实现主机已有协议，不复制 URL。`openai_video` 的主机路径固定为：
   - `POST /v1/videos`
   - `GET /v1/videos/:task_id`
   - `GET|HEAD /v1/videos/:task_id/content`

标准视频插件的正确职责是：

- `protocols.openai_video.decodeRequest`：解析公开 OpenAI 视频请求。
- `buildSubmitRequest`：将公开请求转换成上游厂商请求；若上游也是兼容接口，可以使用上游 `/v1/videos`。
- `parseSubmitResponse`、`buildQueryRequest`、`parseTaskResult`：处理异步任务。
- `protocols.openai_video.render`：返回公开视频任务对象。
- `listArtifacts`、`buildContentRequest`：提供主机代理的视频内容。
- `extractUsage`、`extractUsageOnComplete`：只返回用量事实，不计算价格或配额。

插件不是“模型广场对外正式接口”本身。对外标准接口由主机路由注册；插件只是该接口的路由适配器和上游转换器。

### 6. 当前线上志奇视频插件事实

线上 `zhiqi-video` 当前活动版本为 `1.0.2`，绑定渠道 117（Task Plugin 类型），渠道当前使用四个模型：

- `seedance-2.0`
- `seedance-2.0-fast`
- `seedance-2.0-mini`
- `seedance-2.5`

活动插件的 `usageProfiles` 当前为：

- 前三个 Seedance 2.0 变体：`resolution = ["480p", "720p"]`
- `seedance-2.5`：`resolution = ["480p", "720p", "1080p"]`
- 公共用量字段还包括 `duration` 和 `image_count`

这次线上修复解决了一个具体问题：插件之前给所有模型共用 `[480p, 720p, 1080p, 768P, 2K]` 枚举，定价可视化编辑器会按 schema 自动生成 768P/2K 行，即使用户表达式只有 480p/720p/1080p。现在插件只声明四个模型，并按模型拆分 schema。

注意：`wan3.0-video` 若仍出现在模型广场，可能来自 Alibaba 插件。移除它只代表它不再由 `zhiqi-video` 实现，不代表从所有插件和所有渠道删除该模型。

### 7. 图片能力是独立的公共事实源

图片插件协议不能替代图片能力配置。图片请求能否路由和图片请求支持哪些参数是两个问题：

- 插件/渠道协议回答“请求由哪个适配器处理、进入哪个上游”。
- 图片能力配置回答“该公开模型在该渠道上允许哪些 size、aspect ratio、resolution、quality、output format、编辑和图片数量”。

图片能力持久化文件：

```text
/data/image-capabilities.json
```

生产宿主机的常见挂载路径：

```text
/opt/qlh-main/data/new-api/image-capabilities.json
```

也可以通过 `IMAGE_CAPABILITY_CONFIG_FILE` 指定绝对路径。`pkg/imagecapability/registry.go` 负责内置默认规则、首次生成、每秒最多一次热加载、最后一份有效配置回退和配置版本号。

配置规则特点：

- `rules` 按顺序匹配，精确模型应放在前缀/包含规则之前。
- 支持 `exact_models`、`model_prefixes`、`model_contains`，大小写不敏感。
- 支持 `channel_types` 白名单和 `excluded_channel_types` 黑名单。
- `size_mode` 为 `dimensions`、`aspect_ratio_resolution` 或 `none`。
- `resolution_suffixes` 只影响展示默认值，不改变上游请求模型名。
- 配置 JSON 无效时保留最后一份有效配置，不能让已有图片模型整体消失。

当前图片查找链路是：

```text
public model
  -> channel.model_mapping (如果存在，解析到 upstream model)
  -> imagecapability.Resolve(channel type, upstream model)
  -> ApplyModelAliasDefaults(public model)
  -> image capability DTO
```

`imagecapability.Intersect` 用于同一个公开模型有多个可用渠道时计算保守交集。这个策略能避免用户选择一个某些渠道不支持的参数，但会隐藏供应商之间的差异；后续如果需要显示供应商变体，应新增 provider/channel 维度，而不是放宽交集。

### 8. 定价与用量 schema

动态任务价格由 `billing_setting.billing_expr` 和插件用量 schema 共同决定。插件提供事实，主机执行表达式并负责计费。

插件可以使用：

```js
meta.usageSchema
meta.usageExamples
meta.usageProfiles // 按模型替换默认 schema，不会合并
```

`usageProfiles` 的规则：

- 每个 profile 包含 `models`、完整 `schema` 和可选 `examples`。
- 一个声明模型最多属于一个 profile。
- profile schema 会替换默认 schema，不会和默认字段合并。
- 表达式中引用了不在当前 schema 的 `u("...")` 键时，定价不兼容。
- schema 中的 enum 是定价编辑器组合矩阵的事实源之一。

模型定价编辑器的关键行为：

```text
usage schema enum
  -> createDefaultTaskMatrixConfig()
  -> 每个 enum 值生成一行
  -> tryParseTaskMatrixConfig(expression, schema)
  -> 编辑/预览/保存表达式
```

因此，表达式不能修复错误的 schema。示例：

```text
u("resolution") == "480p" ? tier("480p", u("duration") * 0.450089) :
u("resolution") == "720p" ? tier("720p", u("duration") * 0.750385) :
tier("1080p", u("duration") * 1.4003)
```

当 schema 错误地包含 `768P`、`2K` 时，编辑器会给这两个值套用最终 fallback tier；这就是额外行和错误价格预览的根因。修复顺序必须是先修插件 `usageProfiles`，再重新读取模型定价，不能让管理员靠表达式手动规避。

还要区分：

- `billing_usage_schema`：当前主要用于计费和定价展示。
- 视频请求能力 schema：描述视频接口实际接受的 `duration`、`resolution`、输入图片/音频等字段，当前没有完全独立的数据契约。

后续不应把计费 schema 直接当成完整的上游请求参数 schema；两者需要在模型广场展示层做明确映射。

## 当前实现与历史设计

### 1. 已经存在的后端实现

以下文件已经在当前源码树中存在，具体行为仍需以运行版本和测试结果为准：

| 文件 | 当前职责 | 重点风险 |
| --- | --- | --- |
| `model/pricing.go` | 汇总 enabled abilities、模型元数据、端点、图片能力、价格和插件用量 schema | 同时承载太多聚合策略，容易把不同供应商事实压成一个模型级结果 |
| `model/pricing_endpoint_resolver.go` | `ResolveAbilityEndpointTypes`，按具体 ability、映射、插件绑定和路由 generation 解析端点 | 返回仍是简单端点数组，缺少 provider/channel 证据 |
| `common/endpoint_defaults.go` | 公开端点到默认路径/方法的映射 | 全局 map 不能表达同一模型不同供应商的自定义路径 |
| `model/model_endpoint_migration.go` | 一次性把公开元数据旧键 `openai_video` 迁移为 `openai-video` | 只在启动迁移，不能自动修复每次渠道变化后的陈旧元数据 |
| `model/model_metadata_sync.go` | 元数据更新校验，拒绝公开写入 `openai_video` | 只约束写入，不等于运行时路由真实存在 |
| `common/model.go` | 通过映射解析图片能力，并保留旧模型名兼容判断 | 旧名称 fallback 可能继续给未知能力打图片标签 |
| `service/image_capability.go` | 为 Playground 按用户分组筛选真实图片能力 | 这条链路比全局模型广场更接近真实用户范围 |
| `pkg/imagecapability/registry.go` | 图片能力规则加载、热更新、匹配、版本缓存 | 配置文件和渠道数据库是两套事实源，缺一不可 |
| `controller/pricing.go` | `/api/pricing`，按用户可用分组过滤模型和倍率 | 端点和图片能力已经在 `model.GetPricing` 聚合后才过滤分组 |
| `controller/user.go` | `/api/user/models`、`/api/user/image-models` | 普通模型端点使用全局聚合；图片模型使用分组内能力链路 |
| `model/model_meta.go` | 模型元数据、匹配优先级、模型广场状态辅助 | 元数据行可能存在但没有实际 enabled ability |

### 2. 已经存在的前端实现

| 文件 | 当前职责 | 重点风险 |
| --- | --- | --- |
| `web/src/features/pricing/types.ts` | `/api/pricing` 的 `PricingModel`、图片能力和插件用量类型 | 当前 endpoint 证据仍只表现为字符串数组 |
| `web/src/features/pricing/hooks/use-pricing-data.ts` | 获取 `/api/pricing`，React Query `staleTime` 为 5 分钟 | 插件/渠道更新后页面可能继续显示旧数据 |
| `web/src/features/pricing/components/model-details-api.tsx` | 按 endpoint 选择代码示例和参数表 | 视频参数目前是静态通用列表，不是插件模型级能力 |
| `web/src/features/pricing/lib/media-samples.ts` | 图片生成示例、视频创建/轮询/内容下载示例 | 视频示例没有读取插件请求参数 schema |
| `web/src/features/pricing/lib/mock-stats.ts` | endpoint 参数展示、静态性能/RPM/TPM/RPD 展示 | `VIDEO_PARAMS` 只包含通用 model/prompt；不能作为真实上游参数事实源 |
| `web/src/features/pricing/lib/filters.ts` | 按 endpoint、分组、供应商、计费类型过滤 | 过滤使用聚合后的模型级端点，没有 channel/provider 解释 |
| `web/src/features/pricing/components/model-details.tsx` | 模型详情、端点标签、价格、参数、示例 | 端点切换后必须保证参数和示例同时切换 |
| `web/src/features/playground/**` | 图片 Playground 前端 | 依赖 `/api/user/image-models` 返回的能力 DTO |

### 3. 既有设计记录的完成度

`docs/model-catalog-endpoint-capabilities.md` 记录了上一轮设计，内容包括统一端点解析、图片映射、公开键迁移、视频示例和回归测试。它声称源码层面的阶段 1/2 已完成，但其中也明确写了“尚未部署，需上线后只读核对”。

因此后续 AI 必须区分：

- “文件存在/测试通过”不等于“线上镜像已经运行该代码”。
- “插件 API 返回某端点”不等于“所有模型元数据和所有用户分组都已同步”。
- “模型广场能显示”不等于“对应请求一定能由当前用户可用渠道处理”。

## 已知问题与复现方向

以下问题按照当前证据分为已确认、强烈怀疑和需要验证三类。新 AI 不要把“需要验证”直接改成结论。

### P0：模型广场端点可能残留 `openai` 或旧视频端点

**现象**：渠道或插件已经移除某视频模型，但模型广场仍显示 OpenAI Chat 或视频端点。

**已确认的结构性原因**：

1. `models.endpoints` 是持久化元数据，不会随着渠道模型列表自动清空。
2. `model.GetPricing()` 会将 enabled abilities 推导的端点与元数据端点合并。
3. 同一公开模型可能有多个渠道，模型级结果天然是聚合结果；只要任一可见 enabled ability 仍提供某端点，该端点就可能合理存在。
4. `/api/pricing` 的前端 React Query 默认缓存 5 分钟，后端定价缓存约 1 分钟；插件激活或渠道修改后短时间看到旧数据是预期缓存行为，但长期不消失就是 bug。
5. 旧版本元数据可能保存 `openai_video`、`openai-video`、`openai` 或自定义路径；一次性迁移只在启动时处理已存在记录。

**必须区分的情况**：

- 还有另一个 enabled 渠道/插件支持 Chat：这是真实的聚合端点，不应直接删除。
- 只有 Task Plugin 渠道，却因元数据留下 `openai`：这是错误的默认聊天声明，应清理或在解析层拒绝。
- 用户所在分组没有 Chat 渠道，但其他分组有：全局 `/api/pricing` 可以显示 Chat，用户模型选项却不应把它当成该分组可用端点。

**复现检查顺序**：

1. 查 `abilities`：公开模型、`channel_id`、`enabled`、group。
2. 查 `channels`：status、type、models、model_mapping、setting/task plugin binding。
3. 查看当前节点的 task-plugin runtime generation 和插件错误。
4. 查 `models`：`name_rule`、`status`、`endpoints`。
5. 请求 `/api/pricing`、`/api/user/models?with_endpoint_types=true`，分别使用管理员和普通用户/目标分组。
6. 清理或等待缓存后再次读回，确认是否只是缓存。

### P1：端点解析仍然缺少证据维度

`ResolveAbilityEndpointTypes` 当前返回 `[]constant.EndpointType`，调用方不知道端点来自哪个 channel、哪个 plugin、哪个 upstream model，也不知道候选为什么被拒绝。这样的问题很难解释：

- plugin claim 没有绑定当前 channel setting；
- 公开模型经过 mapping 后，插件声明的是另一个名字；
- 同一个模型有两个插件，其中一个只声明 image，另一个只声明 video；
- type-60 New API channel 使用 `task_plugin_key`/`task_extend_plugin_keys`，type-61 Task Plugin channel 使用 `task_plugin_key`；
- endpoint metadata 有声明但没有真实 route。

后续需要把“解析结果”和“用户展示结果”分开，至少保留内部诊断证据，不要继续只传字符串数组。

### P1：普通用户模型选项的端点范围可能不按分组

`controller.GetUserModels` 先根据用户/token 分组得到模型名，再调用 `buildUserModelOptions`；后者通过全局 `GetModelSupportEndpointTypes(modelName)` 取端点。模型名属于该分组并不代表该分组内的每个渠道都具备全局聚合得到的端点。

需要验证的案例：

```text
group-a: model-x 只有 openai-video
group-b: model-x 只有 openai
用户属于 group-a
```

预期是用户模型选项只显示 `openai-video`，而不是 `[openai, openai-video]`。如果产品决定模型广场展示“全局供应商能力”，则必须和用户可选模型选项使用不同 DTO/文案，不能悄悄复用同一个聚合结果。

### P1：图片能力配置与图片端点可能不一致

当前 `ResolveAbilityEndpointTypes` 可以判定一个插件声明 `openai_image`，但 `imagecapability.Resolve` 找不到规则时，图片参数为空；Playground 服务会过滤掉没有能力规则的模型，而 `/api/pricing` 仍可能把它标为 `image-generation`。

这会产生两种用户困惑：

- 模型广场说支持图片，但 Playground 没有该模型。
- Playground 能选模型，但模型详情没有尺寸/比例参数，示例也不能安全构造。

预期策略应明确为：

- 已知且被配置排除的模型：不可显示图片能力。
- 图片端点存在但能力规则缺失：显示“能力未配置”或从图片 Playground 隐藏，不能假定默认尺寸。
- 旧名称 fallback 只作为兼容过渡，并且必须有显式监控/迁移计划。

### P1：多渠道图片能力交集可能过度收窄

`model.GetPricing` 和 `service.GetUserImageModelGroups` 对同一公开模型的多个能力做交集。交集保证最保守，但可能把供应商 A 支持而供应商 B 不支持的参数全部隐藏，即使渠道选择策略能稳定命中 A。

这是产品策略问题，不应通过随意删除交集逻辑解决。候选方案：

1. 保持交集，文案明确“所有可用渠道共同支持的参数”。
2. 根据用户分组和渠道优先级选择一个 provider profile。
3. 返回 provider/channel 变体，让用户选择供应商。

### P1：视频参数展示和视频请求能力没有统一事实源

`model-details-api.tsx` 使用 `buildSupportedParameters`；`mock-stats.ts` 对 `openai-video` 返回静态 `VIDEO_PARAMS`，目前主要展示 model/prompt。它没有使用插件 `usageProfiles`，也不能说明：

- duration/seconds 是否必需；
- resolution 的合法枚举；
- 图生视频需要公开 URL 还是允许 multipart；
- image_count、输入音频、比例等字段是否支持；
- 不同视频模型的能力差异。

`usageProfiles` 只能解决计费 schema，不能直接当完整视频请求 schema。需要新增请求能力描述或由插件 manifest 提供可验证的 request schema，再由模型广场和请求示例共同使用。

### P2：模型元数据和实际路由的生命周期不同步

元数据同步接口只校验新写入的 endpoint 键，启动迁移只处理历史数据；渠道禁用、模型删除、插件停用、插件版本切换不会自动把 `models.endpoints` 重新计算为真实路由。

模型元数据应该是描述/展示覆盖，而不是标准端点的权威来源。标准端点的权威来源应是当前 enabled ability + channel mapping + binding + routing generation。元数据可以补充非标准自定义端点，但不能凭空创造标准端点。

### P2：多个插件共享模型名时容易误导

一个模型可能同时被 Alibaba、ZeeQi 或其他插件声明。定价后端已有 `billing_plugin_variants`，但公开端点字段仍是模型级数组，不能告诉用户：

- 哪个供应商提供视频；
- 哪个供应商提供图片；
- 当前用户分组是否能使用该供应商；
- 每个供应商是否有不同请求参数和价格。

例如，`wan3.0-video` 从 `zhiqi-video` 删除，不等于 Alibaba 插件的同名模型消失。清理插件声明时必须按 plugin/channel 维度验证，不要根据模型名全局删除。

## 重构方案

### 1. 建立模型级、渠道级端点解析器

新增或扩展一个纯解析层，输入至少包括：

```text
channel type
public model name
resolved upstream model name
channel model_mapping
channel setting / task plugin bindings
enabled channel/ability state
active plugin routing generation
plugin protocol claims and model scopes
metadata endpoint declaration
```

输出不应只有 `[]EndpointType`，建议内部结构如下：

```go
type ModelEndpointEvidence struct {
    PublicModel       string
    UpstreamModel     string
    ChannelID         int
    ChannelType       int
    Group             string
    PluginKey         string
    InternalProtocol  string // openai_video/openai_image/openai_responses
    PublicEndpoint    string // openai-video/image-generation/openai-response
    Method            string
    Path              string
    Source            string // channel, plugin, metadata, advanced_custom
    Eligible           bool
    RejectionReason    string
}
```

实现要求：

- 先解析公开名到 upstream 名，再按当前 plugin generation 查询候选。
- type-61、type-60 和普通 adaptor 的 binding 规则分别处理，不要用一个 channel type 猜全部。
- plugin protocol 的内部名只在解析层和运行层使用。
- 标准端点路径来自 `common.GetDefaultEndpointInfo`；只有非标准自定义端点才允许 metadata path 覆盖。
- 对每个 ability 输出证据；最后再按全局模型、用户分组、插件供应商分别聚合。
- 当 plugin-only channel 没有实际 Chat claim 时，不能因为模型名或历史 metadata 自动添加 `openai`。
- 同名模型多个 provider 不能丢失 provider 维度；至少管理员诊断接口要能列出来源。

### 2. 明确三种聚合视图

不要让一个 `supported_endpoint_types` 同时承担三种语义：

1. **公共模型广场视图**：在当前公开目录范围内，模型至少有一个 enabled、可路由渠道支持该端点。可展示聚合端点，但详情页必须能解释来源。
2. **用户/分组视图**：只按该用户实际可用 group 的 abilities 计算；不能复用全局端点数组。
3. **管理员诊断视图**：返回每个 channel/plugin/upstream 的 endpoint evidence、失败原因和 metadata 冲突。

建议 API 增量字段：

```json
{
  "supported_endpoint_types": ["openai-video"],
  "endpoint_evidence": [
    {
      "public_endpoint": "openai-video",
      "channel_id": 117,
      "channel_type": 61,
      "plugin_key": "zhiqi-video",
      "internal_protocol": "openai_video",
      "path": "/v1/videos",
      "method": "POST",
      "upstream_model": "seedance-2.5"
    }
  ]
}
```

普通用户可以只收到经过权限过滤的 evidence，或只收到来源摘要；不能泄露渠道 key、API key、上游私有地址和内部错误详情。

### 3. 端点元数据迁移和写入策略

保留当前迁移策略的原则，但补齐生命周期：

- 启动一次性迁移 `openai_video` -> `openai-video`。
- 发生规范键和旧键冲突时保留 `openai-video`，记录可检索日志。
- 对 plugin-only exact model 删除历史 `openai` chat metadata，但只在确认没有其他可用 Chat channel 时执行。
- 新写入的公开元数据拒绝 `openai_video`。
- metadata 的标准端点不能覆盖实际 channel/plugin resolver 的结果。
- 渠道/插件/模型映射变化时使模型广场缓存失效，不能等固定 TTL 自然过期。
- 迁移前备份 `models.endpoints`；迁移后按记录数、冲突数、跳过数输出统计。

### 4. 图片能力统一方案

保持 `/data/image-capabilities.json` 为图片参数能力事实源，插件只负责协议路由和上游转换。

推荐链路：

```text
ability (group/channel/public model/mapping)
  -> endpoint resolver must include image-generation
  -> ResolveChannelImageCapability(public, mapping, channel type)
  -> capability rule result
  -> user-group/provider aggregation
  -> /api/pricing image_capabilities
  -> /api/user/image-models capabilities
  -> Playground controls + image samples
```

规则：

- capability match 必须使用映射后的 upstream model，但返回对象保留 public model。
- 已知模型规则被 channel blacklist 排除时，不能再由名称列表 fallback 恢复。
- capability 缺失时不能生成默认尺寸/质量/比例；应标为未配置并从需要参数的 Playground 流程中排除。
- `image-generation` 端点和 `image_capabilities` 必须同时经过一致的权限/分组过滤。
- 多渠道能力交集策略要有文档和测试；如果改成 provider profile，计费和请求示例也要按 provider 选择。
- 图片配置热加载只刷新能力相关缓存，不应改写模型名、channel mapping 或计费值。

### 5. 视频能力与计费 schema 分离

保留 `usageProfiles` 作为计费事实源，同时考虑新增插件 manifest 字段，例如：

```js
requestProfiles: [{
  models: ["seedance-2.5"],
  schema: {
    prompt: {type: "string", required: true},
    duration: {type: "number", unit: "second", minimum: 1, maximum: 30},
    resolution: {enum: ["480p", "720p", "1080p"]},
    input_reference: {type: "url", public: true}
  },
  examples: [{...}]
}]
```

如果暂时不扩展插件 API，至少不要从模型名猜参数；模型广场的视频参数区应显示“插件未声明请求参数”而不是伪造完整列表。

视频示例应从公开 endpoint/path 和 request profile 生成：

- `POST /v1/videos` 创建任务。
- `GET /v1/videos/{id}` 轮询状态。
- `GET /v1/videos/{id}/content` 下载内容。
- 公开示例使用公开模型名和标准字段。
- 图生视频示例必须遵循插件声明的图片来源约束；如果上游要求公共图床 URL，不应生成本地文件上传示例。
- 示例生成器不能回退到 `/v1/chat/completions`。

### 6. 缓存和失效

以下变化必须触发与影响范围相匹配的失效：

| 变化 | 至少需要失效 |
| --- | --- |
| channel status/type/models/model_mapping/setting | pricing、model endpoint map、user model options、相关 Playground options |
| ability enabled/group | pricing、分组模型列表、用户模型选项 |
| plugin upload/activate/deactivate | routing generation、pricing schema/endpoint、task plugin options |
| model metadata endpoint/status | pricing metadata、模型广场列表 |
| image-capabilities.json 有效版本变化 | pricing image capabilities、Playground image models |
| billing expression/usage profile | model pricing snapshot、模型详情价格和定价编辑器 |

React Query 的 5 分钟 stale time 不能成为后台配置变更后唯一的同步策略。后台保存/激活成功后应主动 invalidate；公共缓存若存在，应有版本号或 ETag。

## 文件结构设计

### 1. 现有文件职责

| 路径 | 作用 | 维护要求 |
| --- | --- | --- |
| `model/pricing.go` | 公共模型广场定价聚合 | 只编排解析结果，不继续堆叠新的渠道判断 |
| `model/pricing_endpoint_resolver.go` | ability 级端点解析 | 作为统一 resolver 的过渡入口，补充 evidence 输出 |
| `model/ability.go` | enabled ability 与 channel join 查询 | 保持 channel status、mapping、setting 一起读取 |
| `model/model_meta.go` | 模型 metadata 和匹配规则 | exact > prefix > suffix > contains 的优先级不能被破坏 |
| `model/model_endpoint_migration.go` | 历史公开端点键迁移 | 迁移只改公开 metadata，不改插件内部协议 |
| `model/model_metadata_sync.go` | metadata 写入校验和事务 | 持续拒绝 `openai_video`，保留版本冲突保护 |
| `common/endpoint_defaults.go` | 公开标准端点默认 path/method | 标准路径由主机拥有 |
| `common/endpoint_type.go` | channel/adaptor 到公开 endpoint 的旧兼容映射 | 只作为无插件渠道 fallback |
| `common/model.go` | 图片模型旧列表与映射能力解析 | 新能力规则优先，旧列表只能兼容兜底 |
| `pkg/imagecapability/types.go` | 图片能力 DTO 内部类型 | 变更时同步 DTO、配置校验和前端类型 |
| `pkg/imagecapability/registry.go` | 图片配置持久化、热加载和匹配 | 无效文件回退最后有效配置 |
| `pkg/imagecapability/image-capabilities.default.json` | 内置图片默认能力 | 生产文件生成前必须保持兼容 |
| `service/image_capability.go` | 用户分组图片模型选项 | 必须按用户 group 和实际 image endpoint 过滤 |
| `controller/pricing.go` | 公共 `/api/pricing` | 只做用户分组脱敏，不重新猜端点 |
| `controller/user.go` | `/api/user/models` 和 `/api/user/image-models` | 两条链路的权限语义不能混淆 |
| `pkg/jsplugin/routing.go` | host protocol 和 `/v1/videos`、图片路径定义 | 内部协议名不能公开化 |
| `pkg/jsplugin/registry.go` | plugin meta、profiles、routing generation | profile 模型不能重复或脱离 `meta.models` |
| `router/task-plugin-protocol-router.go` | 标准插件协议路由 | 主机负责鉴权、调度和任务生命周期 |
| `web/src/features/pricing/types.ts` | 广场 API 类型 | 后续增加 endpoint evidence/request profile 时同步 |
| `web/src/features/pricing/lib/media-samples.ts` | 图片/视频请求示例 | 只使用有能力来源的字段 |
| `web/src/features/pricing/lib/mock-stats.ts` | 参数与静态性能展示 | 不应继续承载真实能力事实 |
| `web/src/features/pricing/components/model-details-api.tsx` | endpoint 示例和参数 UI | endpoint、参数、示例必须联动 |
| `web/src/features/playground/**` | 图片 Playground | 使用 `/api/user/image-models` 能力 DTO |

### 2. 建议新增或拆分的文件

| 建议路径 | 责任 |
| --- | --- |
| `model/model_endpoint_evidence.go` | 定义 `ModelEndpointEvidence`、来源、拒绝原因和权限过滤类型 |
| `model/model_endpoint_resolver.go` | 统一处理 channel/mapping/plugin/metadata 输入，输出 ability 级证据 |
| `model/model_endpoint_aggregate.go` | 分别构造公共、用户分组、管理员诊断三种视图 |
| `model/model_endpoint_cache.go` | 将 channel/plugin/metadata/image config 版本纳入缓存 key 或失效机制 |
| `dto/model_endpoint.go` | 对外安全 DTO，过滤 channel key、上游凭证和内部异常 |
| `pkg/jsplugin/request_schema.go` | 如果扩展插件契约，定义视频/图片请求参数 schema |
| `web/src/features/pricing/lib/endpoint-evidence.ts` | 前端安全地解释 provider/endpoint 来源 |
| `web/src/features/pricing/lib/request-samples.ts` | 根据 endpoint 和 request schema 生成示例 |
| `web/src/features/pricing/lib/media-capabilities.ts` | 图片/视频能力到参数表的统一转换 |
| `web/src/features/pricing/__tests__/endpoint-evidence.test.ts` | 多渠道、多插件、分组过滤和 metadata 冲突测试 |
| `model/model_endpoint_resolver_test.go` | resolver 输入/输出和拒绝原因回归 |
| `controller/model_endpoint_test.go` | 公共/用户/管理员 API 脱敏和范围回归 |

旧的 `docs/model-catalog-endpoint-capabilities.md` 应继续保留为历史实现记录；本文件是后续改造的交接入口，不要让两份文档对“线上是否部署”产生歧义。

## 开发清单

| 阶段 | 工作项 | 当前状态 | 验收标准 |
| --- | --- | --- | --- |
| 0 | 保存当前线上/数据库/源码快照，确认 active new-api 节点和插件 generation | 部分完成 | 能复现问题并区分线上版本、数据库和本地代码 |
| 1 | 为一个错误端点模型建立完整证据链 | 待继续 | 输出 ability、channel、mapping、binding、metadata、cache 的逐层结论 |
| 2 | 实现 ability 级统一 endpoint resolver/evidence | 设计完成 | 普通 adaptor、type-60、type-61、插件多协议测试通过 |
| 3 | 分离公共模型、用户分组、管理员诊断三种聚合 | 设计完成 | 同一模型在不同分组的端点显示符合实际可用渠道 |
| 4 | 完成旧 `openai_video` 迁移和新写入拒绝 | 源码已有，需线上验证 | 迁移幂等、规范键胜出、插件内部协议仍可运行 |
| 5 | 统一图片能力链路 | 源码已有，需检查线上 | 映射、channel exclusion、配置缺失、交集和 Playground 一致 |
| 6 | 分离视频请求能力和计费 schema | 待实现 | 不能再用 billing enum 推断全部请求字段；示例只生成已声明字段 |
| 7 | 修复前端 endpoint/参数/示例联动 | 部分完成 | 切换 endpoint 后参数、示例、路径全部变化；无聊天回退 |
| 8 | 完善缓存失效和版本观测 | 待实现 | 插件/渠道/配置更新后无需等待 5 分钟即可读到新目录 |
| 9 | 回归测试、部署和线上只读核对 | 待执行 | Go、前端、插件、数据库迁移和公共/用户 API 验收完成 |

### 重点测试矩阵

至少覆盖以下组合：

| 场景 | 预期 |
| --- | --- |
| 一个普通 OpenAI channel | 只显示 adaptor 实际支持的 endpoint |
| 一个 type-61 Task Plugin channel | 只显示绑定插件声明的 host protocol，不自动加 Chat |
| 一个 type-60 New API channel + `task_plugin_key` | 只显示实际绑定且支持 New API upstream 的插件端点 |
| 同一公开模型同时有 Chat 和 Video channel | 公共目录可显示两者，但 evidence 能区分来源 |
| 同一模型 Chat 在 group-b、Video 在 group-a | group-a 用户不能得到 Chat endpoint |
| 旧 metadata 只有 `openai_video` | 启动迁移为 `openai-video`，插件内部仍使用 `openai_video` |
| 旧/新视频键同时存在 | 保留 `openai-video`，记录冲突，删除旧键 |
| plugin-only metadata 含 `openai` | 无真实 Chat ability 时不显示 Chat |
| image plugin 有 endpoint、无 capability rule | 不虚构尺寸；Playground 按策略隐藏或标记未配置 |
| image public model 通过 mapping 到 upstream image model | 能力按 upstream 匹配，示例仍使用 public model |
| image channel 被 capability blacklist 排除 | 不能被旧名称列表 fallback 重新加入 |
| 同一 image model 多渠道能力不同 | 按明确的交集或 provider profile 策略展示 |
| `seedance-2.5` 只有 480/720/1080 schema | 定价矩阵不出现 768P/2K |
| `seedance-2.0` 系列只有 480/720 schema | 不能套用 seedance-2.5 的 1080 fallback |
| 旧插件版本和新插件版本并存 | active generation 唯一，新任务用新版本，旧任务可按 pinned producer 继续解析 |

## 阶段自检记录

### 已完成的事实核对

- 已确认插件内部 `openai_video` 和公开端点 `openai-video` 是两层名称，不能全局替换。
- 已确认 `openai-video` 默认路径应为 `/v1/videos`，不是 `/grok/v1/videos`；厂商原生路径只在插件内部上游请求中使用。
- 已确认图片能力文件仍然是有效公共事实源，插件不能取代它。
- 已确认定价编辑器依据 `usageProfiles` 的 enum 自动生成矩阵行；schema 错误会造成额外分辨率行。
- 已确认线上 `zhiqi-video@1.0.2` 只声明渠道 117 当前四个 Seedance 模型，定价 API 读回的枚举符合预期。
- 已确认本地插件 lint/fixture 通过；这只能证明插件源和主机契约兼容，不能证明整个线上模型广场已修复。

### 上一轮源码实现记录

旧文档记录的后端和前端端点/图片能力改造包括：

- `pricing_endpoint_resolver.go` 按具体 ability、mapping、plugin binding 和 routing generation 解析。
- `model_endpoint_migration.go` 迁移历史公开视频键。
- `model_metadata_sync.go` 拒绝新写入 `openai_video`。
- `endpoint_defaults.go` 增加 `openai-video` `/v1/videos`。
- 图片配置热加载、映射解析和交集能力链路。
- 视频创建/轮询/内容下载示例和端点切换测试。

这些修改的线上部署状态必须通过活动容器镜像、公共 API 和 runtime status 重新确认，不能只看工作树文件。

### 当前仍未闭环的检查

- 模型广场端点是否严格按用户 group 计算。
- 渠道/插件停用后陈旧 `models.endpoints` 是否仍会被公共 API 合并出来。
- `supported_endpoint` 全局路径 map 在多供应商自定义路径下是否会误导详情页。
- 图片端点存在但 capability 缺失时，`/api/pricing`、`/api/user/image-models` 和 Playground 是否一致。
- 视频 request schema 是否需要扩展 Task Plugin v1，还是先采用受限的公共 capability DTO。
- 插件切换、渠道修改和图片配置热加载后的缓存是否及时失效。
- 旧 metadata 迁移是否已经在线上所有节点执行，是否存在不同节点 generation/数据库版本不一致。

### 推荐的线上只读采集

不要在文档、日志或聊天中输出密钥。管理员诊断时可只读取并脱敏：

```text
GET /api/pricing
GET /api/option/model_pricing?model=<name>
GET /api/user/models?with_endpoint_types=true
GET /api/user/image-models
GET /api/plugin/task
GET /api/plugin/task/<key>
GET /api/plugin/task/runtime/status
GET /api/task_plugin_options
```

数据库只读核对重点：

```text
channels: id, type, status, models, model_mapping, setting, other/settings
abilities: group, model, channel_id, enabled
models: model_name, status, name_rule, endpoints, sync_official
options: billing_setting.billing_expr, billing_setting.plugin_billing_expr
```

线上分析必须记录：活动节点/镜像 revision、插件 active version、数据库读回、API 响应和缓存刷新时间。不要把旧数据库 dump、静态前端 bundle 或某个 inactive plugin version 当成当前事实。

## 最终审查结论

模型广场当前的问题不是“给模型补一个端点字符串”这么简单，而是以下事实源没有完全合并成一个可解释的模型级视图：

```text
真实 enabled ability
  + 用户/分组范围
  + public -> upstream mapping
  + plugin binding and routing claims
  + metadata endpoint declarations
  + image capability configuration
  + billing/request schemas
  + cache/runtime generation
```

最优先的改造顺序是：

1. 先用具体问题建立完整证据链，确认是陈旧 metadata、全局聚合、错误 binding 还是缓存。
2. 把 endpoint resolver 从“返回字符串数组”升级为“返回可追踪证据”，再分别构造公共、用户和管理员视图。
3. 保留图片能力文件，并让映射、渠道过滤、端点判断和 Playground 共用同一链路。
4. 保留插件内部 `openai_video`，公开元数据统一 `openai-video`，继续使用标准 `/v1/videos`。
5. 修复计费 schema 造成的额外矩阵行，并单独设计视频请求能力 schema。
6. 最后再修改前端展示和缓存失效；不要通过前端隐藏来掩盖后端端点事实错误。

禁止以下低风险外观修复冒充架构修复：

- 在前端过滤掉 `openai`，但后端 `/api/pricing` 仍返回它。
- 全局把 `openai_video` 替换为 `openai-video`，破坏插件运行时。
- 删除 `image-capabilities.json`，让插件协议“猜”图片参数。
- 用模型名字正则推断视频分辨率、图片尺寸或供应商能力。
- 只改当前一个模型的定价表达式，不修复共用 schema。
- 看到同名模型就全局删除另一个插件的实现。
- 只验证 HTTP 200，不验证实际 route binding、插件 generation、上游请求、任务轮询和计费事实。

完成改造后，必须同时提交：源码变更、测试变更、迁移/缓存说明、线上读回证据和本文件的阶段更新。
