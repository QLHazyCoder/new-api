# Playground 生图能力开发方案

> 状态说明（2026-07-15）：本文前半部分保留首次实现 Playground 生图时的历史分析。当前代码已经具备 GPT 生图、图片编辑和本地任务恢复能力。本次重构的现行设计与开发清单以文末“多厂商生图重构”章节为准；该章节覆盖并替代历史方案中“所有供应商共用一套参数”的结论。

## 背景与目标

当前 Playground 页面只能调用聊天补全链路，无法以登录用户身份直接调用生图模型。用户即使在模型下拉中看到 `gpt-image-1`、`dall-e-3`、`imagen-*`、`flux-*` 等模型，现有前端仍会把请求发到 `/pg/chat/completions`，后端也会按 `RelayModeChatCompletions` 处理，最终走 `relay.TextHelper`，不是图片生成链路。

本方案目标是在 Playground 中增加“生图”模式，使登录用户可以直接选择可用分组和支持图片生成端点的模型，提交文生图请求并查看结果。首期聚焦文生图，优先保证权限、计费、日志、渠道分发和 UI 行为正确；图生图、遮罩编辑、流式局部预览作为后续阶段扩展。

文档放置在现有 `docs/` 目录下；仓库当前没有 `doc/` 目录，不额外创建平行目录，避免文档结构分裂。

## 当前实现分析

### 前端 Playground

- 页面入口：`web/default/src/features/playground/index.tsx`
- API 常量：`web/default/src/features/playground/constants.ts`
- 请求构造：`web/default/src/features/playground/lib/payload-builder.ts`
- 请求发送：`web/default/src/features/playground/api.ts`
- 流式处理：`web/default/src/features/playground/hooks/use-stream-request.ts`
- 状态持久化：`web/default/src/features/playground/hooks/use-playground-state.ts` 与 `web/default/src/features/playground/lib/storage.ts`
- 输入组件：`web/default/src/features/playground/components/playground-input.tsx`
- 聊天展示：`web/default/src/features/playground/components/playground-chat.tsx`

现有前端固定使用：

```ts
CHAT_COMPLETIONS: '/pg/chat/completions'
USER_MODELS: '/api/user/models'
USER_GROUPS: '/api/user/self/groups'
```

`buildChatCompletionPayload` 只构造 `model/group/messages/stream` 以及聊天参数。`formatMessageForAPI` 只把当前消息文本作为 `content` 发给上游。`buildMessageContent` 虽然保留了图片输入拼装工具，但当前输入组件还没有真正上传图片，且这属于视觉理解或图生图输入，不等于图片生成端点。

`/api/user/models` 当前只返回字符串数组，不带 `supported_endpoint_types`。因此当前 Playground 无法可靠区分聊天模型和生图模型。

### 后端 Playground 与 Relay

- `/pg/chat/completions` 路由：`router/relay-router.go`
- Playground controller：`controller/playground.go`
- 路径转 relay mode：`relay/constant/relay_mode.go`
- 分发器：`middleware/distributor.go`
- relay 分发：`controller/relay.go`
- 图片处理器：`relay/image_handler.go`
- 图片 DTO：`dto/openai_image.go`
- RelayInfo `/pg` 路径重写：`relay/common/relay_info.go`

现有 `/pg/chat/completions` 使用 `UserAuth + Distribute`，在 `controller.Playground` 中为登录用户构造临时 token context，再调用 `Relay(c, types.RelayFormatOpenAI)`。它的优点是用户无需手动创建 API token，也能走正常的渠道选择、计费、日志和额度扣减。

真正的图片生成入口是 `/v1/images/generations`，它注册在 token 态 `/v1` relay 下，并在 `Path2RelayMode` 中识别为 `RelayModeImagesGenerations`。`controller.relayHandler` 对 `RelayModeImagesGenerations` 调用 `relay.ImageHelper`。`ImageHelper` 会走 `dto.ImageRequest`、模型映射、渠道适配、上游请求、用量和扣费。

`relay/common/relay_info.go` 对 `/pg` 有通用路径重写逻辑：

```go
if strings.HasPrefix(c.Request.URL.Path, "/pg") {
    info.IsPlayground = true
    info.RequestURLPath = strings.TrimPrefix(info.RequestURLPath, "/pg")
    info.RequestURLPath = "/v1" + info.RequestURLPath
}
```

因此如果新增 `/pg/images/generations`，RelayInfo 可以自然重写为 `/v1/images/generations`。

### 模型能力与端点

- 默认端点定义：`common/endpoint_defaults.go`
- 模型端点推断：`common/endpoint_type.go`
- 生图模型名规则：`common/model.go`
- 定价与端点缓存：`model/pricing.go`
- OpenAI models list：`controller/model.go`
- Pricing 前端能力推断：`web/default/src/features/pricing/lib/model-metadata.ts`

后端已经会把图片生成模型优先标记为 `EndpointTypeImageGeneration`。`model.GetPricing()` 也会生成 `SupportedEndpointTypes`，其中 `image-generation` 是图片输出端点。`/v1/models` 的 `ListModels` 返回 `dto.OpenAIModels` 时也会填充 `SupportedEndpointTypes`。

但 Playground 当前使用的 `/api/user/models` 不返回端点能力。如果前端只靠模型名规则过滤，会漏掉自定义端点和模型元数据覆盖，也可能误判多端点模型。更稳的设计是为 Playground 增加一个带端点能力的模型列表接口，或复用 `/api/pricing` 的已过滤数据。

### 图片 DTO 与参数

`dto.ImageRequest` 已支持：

- `model`
- `prompt`
- `n`
- `size`
- `quality`
- `response_format`
- `style`
- `background`
- `moderation`
- `output_format`
- `output_compression`
- `partial_images`
- `images`
- `mask`
- `input_fidelity`
- `watermark`
- `extra_fields`

字段中部分使用 `json.RawMessage`，实际序列化仍走 `common.Marshal` / `common.Unmarshal`，符合项目 JSON 包装规则。

`stream` 字段当前被注释，`ImageRequest.IsStream` 固定返回 `false`。因此首期不应承诺图片流式生成；可以先做非流式结果展示。AIPic 对流式 partial image 的处理可以作为后续参考，但不应在首期强依赖。

### AIPic 可参考点

已分析 `simplaj/AIPic` 的实现，核心参考如下：

- `src/types.ts` 把任务抽象为 `TaskRecord`，包含 prompt、params、status、outputImages、error、elapsed、revisedPrompt、rawImageUrls。
- `src/types.ts` 的 `TaskParams` 覆盖 `size/quality/output_format/output_compression/moderation/n`，适合借鉴为 Playground ImageConfig。
- `src/lib/openaiCompatibleImageApi.ts` 同时解析 `data[].url` 和 `data[].b64_json`，并记录 `rawImageUrls`。
- `src/lib/imageApiShared.ts` 对 base64 正规化、图片 URL 跨域失败提示、API 错误解析做了完整处理。
- 它的 API Key、API 代理、自定义 provider、fal.ai 适配不适合直接搬到本项目，因为本项目已有后端统一代理、渠道管理、额度和日志体系。

结论：AIPic 适合借鉴任务模型、结果解析、参数设计和失败提示；不适合照搬 API 配置体系和完整修图工作台。

## 需求符合性分析

### 功能需求

| 需求 | 方案是否覆盖 | 说明 |
| --- | --- | --- |
| Playground 能生图 | 覆盖 | 新增 `/pg/images/generations` 与前端 Image 模式 |
| 用户无需手动 API Key | 覆盖 | 复用登录态 `UserAuth` 和临时 token context |
| 能选择分组 | 覆盖 | 请求保留 `group` 字段用于分发器选择，不透传上游 |
| 只显示生图模型 | 覆盖 | 使用 `supported_endpoint_types` 过滤 `image-generation` |
| 支持 OpenAI 兼容图片响应 | 覆盖 | 解析 `data[].url` 与 `data[].b64_json` |
| 支持计费与日志 | 覆盖 | 复用 `relay.ImageHelper`、`PostTextConsumeQuota` 与 PriceData |
| 支持 i18n | 覆盖 | 新 UI 文案必须同步 en/zh/fr/ja/ru/vi |
| 保持现有聊天功能 | 覆盖 | 双模式隔离状态，不改聊天 payload 行为 |

### 完整性判断

首期方案完整覆盖文生图的主链路：模型选择、参数输入、提交、分发、上游调用、结果展示、错误展示、历史记录和验收测试。未覆盖图生图和遮罩编辑是刻意裁剪，因为它们需要 multipart 上传、图片体积限制、mask 编辑器和更复杂存储策略，风险明显高于文生图。

### 正确性判断

正确性关键在于不要把图片生成伪装成聊天补全。新增 `/pg/images/generations` 后，后端必须在 `Path2RelayMode` 中识别为 `RelayModeImagesGenerations`，并调用 `RelayFormatOpenAIImage`。如果只在前端改 endpoint 到 `/v1/images/generations`，会进入 TokenAuth 链路，破坏 Playground 登录态体验，这是错误设计。

### 规范一致性判断

- Go JSON 实际 marshal/unmarshal 必须继续使用 `common.*`，不能新增 `encoding/json` 调用。
- 路由、controller、middleware 应复用现有 Playground 和 relay patterns。
- 前端包管理和脚本使用 Bun。
- UI 使用现有 shadcn/Base UI 组件、`ModelGroupSelector`、`PromptInput` 或同风格表单控件。
- 新文案使用 `t('English key')`，并同步所有 locale。
- 不改动项目受保护品牌、作者、许可证和元数据。

## 总体方案

### 用户体验

Playground 页面新增一个紧凑的模式切换：

```text
[ Chat ] [ Image ]
```

Chat 模式保持当前界面和行为。Image 模式首屏直接是可用的生图工作台，不做营销页或说明页：

```text
上方/输入区：
  分组选择  模型选择  尺寸  质量  数量  返回格式

中间：
  图片任务流或网格
  - running: loading 状态和耗时
  - done: 图片预览、下载、复制链接、复用 prompt
  - error: 错误详情、重试

底部：
  prompt 输入框 + 生成按钮
```

首期不做复杂三栏工作台，避免破坏现有 Playground 的轻量聊天体验。图片结果区域可以使用网格而不是聊天气泡，因为图片天然适合卡片展示。

### 后端接口设计

新增路由：

```go
playgroundRouter.POST("/images/generations", controller.PlaygroundImageGeneration)
```

新增 controller：

```go
func PlaygroundImageGeneration(c *gin.Context) {
    // 与 Playground 类似：
    // 1. 禁止 use_access_token
    // 2. GenRelayInfo(c, types.RelayFormatOpenAIImage, nil, nil)
    // 3. 读取用户缓存并 WriteContext
    // 4. SetupContextForToken 临时 token
    // 5. Relay(c, types.RelayFormatOpenAIImage)
}
```

需要避免复制粘贴导致两套逻辑漂移。建议抽一个小 helper：

```go
func setupPlaygroundRelayContext(c *gin.Context, relayFormat types.RelayFormat) *types.NewAPIError
```

但首期也可以先保持两个 controller 结构相似，后续再抽象。若抽象，必须保证错误响应结构与现有 `Playground` 一致。

### relay mode 与路径重写

`relay/constant/relay_mode.go` 增加：

```go
if strings.HasPrefix(path, "/v1/chat/completions") ||
   strings.HasPrefix(path, "/pg/chat/completions") {
    relayMode = RelayModeChatCompletions
} else if strings.HasPrefix(path, "/v1/images/generations") ||
          strings.HasPrefix(path, "/pg/images/generations") {
    relayMode = RelayModeImagesGenerations
}
```

`relay/common/relay_info.go` 已有 `/pg` 到 `/v1` 的通用重写，新增路径后可以复用，不需要额外分支。

### 分组选择与请求字段

前端请求可以包含：

```json
{
  "model": "gpt-image-1",
  "group": "default",
  "prompt": "A product photo of ...",
  "size": "1024x1024",
  "quality": "auto",
  "n": 1,
  "response_format": "url"
}
```

`group` 只用于 Playground 分发，不应转发上游。当前 `dto.ImageRequest` 没有 `group` 字段，`UnmarshalJSON` 会把未知字段放进 `Extra`，但 `MarshalJSON` 不合并 `Extra`，因此默认不会透传 `group`，这符合预期。

还需要额外覆盖渠道 pass-through 场景：当全局或渠道启用原始请求体透传时，relay 可能直接使用客户端 JSON body，而不是重新 marshal `dto.ImageRequest`。因此 Playground controller 在进入 `Relay()` 前必须从 JSON body 中剥离 `group` 等 Playground 内部字段，确保该字段只参与本地分发和权限校验，不会出现在上游请求体中。

`middleware/distributor.go` 目前只对 `/pg/chat/completions` 特判 group。需要扩展为：

```go
if strings.HasPrefix(c.Request.URL.Path, "/pg/chat/completions") ||
   strings.HasPrefix(c.Request.URL.Path, "/pg/images/generations") {
    req, err := getModelFromRequest(c)
    modelRequest.Model = req.Model
    modelRequest.Group = req.Group
    common.SetContextKey(c, constant.ContextKeyTokenGroup, modelRequest.Group)
    // 同样校验用户可用分组并覆盖 usingGroup
}
```

建议抽函数：

```go
func applyPlaygroundGroupOverride(c *gin.Context, usingGroup string) (string, error)
```

避免未来 `/pg/images/edits`、`/pg/responses` 继续复制。

实现时必须注意请求体可重复读取。`Distribute()` 会先用 `getModelFromRequest` 或 `common.UnmarshalBodyReusable` 读取 `model/group`，随后 `Relay()` 还要在 `helper.GetAndValidateRequest` 中再次读取完整 `dto.ImageRequest`。因此新增生图分支不能直接 `io.ReadAll(c.Request.Body)` 后丢弃 body，也不能用一次性 decoder 读取请求体；必须继续使用现有 `BodyStorage` / `common.UnmarshalBodyReusable` / `gjson` 读取方式，确保下游还能解析 `prompt/size/quality/n` 等字段。

首期建议由前端强校验 `prompt` 非空，并在图片提交前 trim。后端 `dto.ImageRequest` 虽有 `binding:"required"`，但当前 `GetAndValidOpenAIImageRequest` 中 prompt required 校验被注释；如果要恢复后端 prompt 校验，必须评估对现有 `/v1/images/generations` 兼容性的影响，不能为了 Playground 直接改变全部 token API 行为。更稳妥做法是先在 `PlaygroundImageGeneration` 或 `/pg` 专用校验中限制空 prompt。

### 模型列表设计

优先方案：新增 Playground 模型接口，返回带端点能力的数据。

```http
GET /api/user/models?with_endpoint_types=true
```

兼容返回：

```json
{
  "success": true,
  "data": [
    {
      "label": "gpt-image-1",
      "value": "gpt-image-1",
      "supported_endpoint_types": ["image-generation", "openai"]
    }
  ]
}
```

兼容性要求：

- 不带 query 时保持原字符串数组，避免影响其他调用。
- 带 `with_endpoint_types=true` 时返回对象数组。
- 返回的模型仍需按用户可用分组过滤。
- 若某模型缺少端点缓存，但模型名命中 `common.IsImageGenerationModel`，后端应至少返回 `image-generation`；更准确的来源仍是 `model.GetModelSupportEndpointTypes`。

替代方案：前端调用 `/api/pricing`，过滤 `supported_endpoint_types`。该接口已按用户可用组过滤，信息完整，但受 HeaderNavModuleAuth("pricing") 控制。如果管理员关闭 pricing 模块，Playground 不应因此失去模型列表，所以不作为首选。

### 前端状态设计

新增类型：

```ts
export type PlaygroundMode = 'chat' | 'image'

export interface ImageGenerationConfig {
  model: string
  group: string
  size: string
  quality: 'auto' | 'standard' | 'hd' | 'low' | 'medium' | 'high'
  n: number
  response_format: 'url' | 'b64_json'
  output_format?: 'png' | 'jpeg' | 'webp'
  output_compression?: number | null
  moderation?: 'auto' | 'low'
}

export type ImageTaskStatus = 'running' | 'done' | 'error'

export interface ImageResult {
  url?: string
  b64_json?: string
  revised_prompt?: string
}

export interface ImageTask {
  id: string
  prompt: string
  config: ImageGenerationConfig
  status: ImageTaskStatus
  images: ImageResult[]
  rawImageUrls?: string[]
  error?: string
  errorCode?: string
  createdAt: number
  finishedAt?: number
}
```

存储 key 建议：

```ts
PLAYGROUND_MODE: 'playground_mode'
IMAGE_CONFIG: 'playground_image_config'
IMAGE_TASKS: 'playground_image_tasks'
```

存储边界：

- localStorage 只保存最近 20 条任务。
- 如果 `response_format=b64_json`，只保存最近少量 base64 或只保存当前会话内结果，避免 localStorage 配额溢出。
- `url` 结果可以保存链接和 revised_prompt。
- 保存失败不能影响生成主流程，只打 console error。

### 前端 API 设计

新增：

```ts
export async function sendImageGeneration(
  payload: ImageGenerationRequest
): Promise<ImageGenerationResponse>
```

endpoint：

```ts
IMAGE_GENERATIONS: '/pg/images/generations'
```

响应类型兼容：

```ts
export interface ImageGenerationResponse {
  created?: number
  data: Array<{
    url?: string
    b64_json?: string
    revised_prompt?: string
  }>
  metadata?: unknown
}
```

错误解析沿用当前 axios `skipErrorHandler: true`，由图片任务 hook 自己展示错误，并保留后端 OpenAI error code。

### 图片结果解析

首期前端展示规则：

- `url`：直接作为 `<img src>` 展示，并提供复制链接。
- `b64_json`：规范化为 `data:image/{format};base64,...` 展示，并提供下载。
- `revised_prompt`：若存在，在任务详情或图片卡片二级信息中展示。
- `data` 为空：任务标记 error，提示“API did not return image data”，并可显示原始响应摘要。
- 图片加载失败：保留原始链接，提供复制按钮，不强制 fetch 转 data URL。

不要首期在浏览器端主动 fetch 所有图片 URL 转 base64，因为跨域、过期链接和大图体积会造成不稳定。AIPic 的 `fetchImageUrlAsDataUrl` 可作为后续“保存图片到本地历史”的参考。

### UI 组件设计

建议新增：

- `components/playground-mode-toggle.tsx`
- `components/playground-image-panel.tsx`
- `components/playground-image-input.tsx`
- `components/playground-image-task-grid.tsx`
- `components/playground-image-task-card.tsx`
- `hooks/use-image-generation-handler.ts`
- `lib/image-payload-builder.ts`
- `lib/image-result-utils.ts`

组件风格：

- 使用现有 `Button`、`Tabs` 或 `ToggleGroup`、`Select`、`Input`、`Textarea`、`Slider`、`Dialog`、`Skeleton`。
- 按钮使用 lucide icon，例如 `ImageIcon`、`DownloadIcon`、`CopyIcon`、`RefreshCwIcon`。
- 不嵌套卡片；图片结果每个任务卡片是单层 card。
- 桌面端图片网格保持紧凑，移动端单列。
- 固定图片容器比例，避免加载状态和图片完成后布局跳动。

### 默认参数

建议首期默认：

```ts
const DEFAULT_IMAGE_CONFIG = {
  model: 'gpt-image-1',
  group: DEFAULT_GROUP,
  size: '1024x1024',
  quality: 'auto',
  n: 1,
  response_format: 'url',
  output_format: 'png',
  output_compression: null,
  moderation: 'auto',
}
```

默认模型需要在模型列表加载后校正：如果 `gpt-image-1` 不可用，则选择第一个支持 `image-generation` 的模型。

尺寸候选：

- `auto`
- `1024x1024`
- `1024x1536`
- `1536x1024`
- `1024x1792`
- `1792x1024`
- `1:1`
- `16:9`
- `9:16`

其中比例值主要服务 Gemini Imagen、部分兼容供应商。UI 可以将尺寸和比例放在一个 Select 中，不同供应商不强制动态约束，失败由后端和上游返回。

质量候选：

- `auto`
- `standard`
- `hd`
- `low`
- `medium`
- `high`

兼容 OpenAI DALL-E 与 gpt-image 系列，同时覆盖 Gemini adaptor 中的映射。若某供应商不支持，允许上游报错或由渠道参数覆盖。

### i18n 设计

所有新增 UI 文案必须使用英文 key：

```ts
t('Image Generation')
t('Generate image')
t('Image model')
t('Output size')
t('Output quality')
t('Response format')
t('Generated images')
t('Copy image link')
t('Download image')
t('Reuse prompt')
t('API did not return image data')
```

开发后必须运行：

```bash
cd web/default
bun run i18n:sync
```

并补齐：

- `web/default/src/i18n/locales/en.json`
- `web/default/src/i18n/locales/zh.json`
- `web/default/src/i18n/locales/fr.json`
- `web/default/src/i18n/locales/ja.json`
- `web/default/src/i18n/locales/ru.json`
- `web/default/src/i18n/locales/vi.json`

### 权限、计费与日志

必须复用现有 relay，而不是在前端直连上游：

- 权限：`UserAuth` 与用户可用分组校验。
- 渠道选择：`middleware.Distribute` 与 `CacheGetRandomSatisfiedChannel`。
- 模型映射：`helper.ModelMappedHelper`。
- 计费：`relay.ImageHelper` 中的 `ImagePriceRatio` 与 `OtherRatio("n")`。
- 日志：`service.PostTextConsumeQuota`。
- 失败退款：`controller.Relay` 中已有 billing refund 路径。

注意 `n` 不得重复计费。`ImageRequest.GetTokenCountMeta` 已说明 `n` 不在 token meta 中计入，`ImageHelper` 或 adaptor 通过 OtherRatio 处理。新增代码不要再自行乘以 `n`。

### 与现有功能的关联影响

| 关联点 | 风险 | 设计处理 |
| --- | --- | --- |
| `/api/user/models` 返回格式 | 破坏旧调用 | query 参数开启对象返回，默认保持字符串数组 |
| `/pg` 路径识别 | 误走 TextHelper | `Path2RelayMode` 必须识别 `/pg/images/generations` |
| 分组字段 `group` | 透传上游或未生效 | 分发器读取 group；DTO 不 marshal Extra，默认不透传 |
| localStorage | base64 图片过大 | 限制历史数量，必要时不持久化 base64 |
| 图片 URL 跨域 | 下载失败 | 首期直接展示和复制链接，不强制 fetch 转存 |
| pricing 模块关闭 | 模型过滤失效 | 不依赖 `/api/pricing` 作为唯一模型源 |
| 生图流式 | DTO 不支持 stream | 首期不做流式，后续再扩展 |
| 图生图/edits | multipart 复杂 | 后续阶段单独设计 |

## 开发阶段清单

### 阶段 1：后端 Playground 生图入口

开发项：

- 在 `router/relay-router.go` 为 `/pg` 增加 `POST /images/generations`。
- 在 `controller/playground.go` 增加 `PlaygroundImageGeneration`。
- 在 `relay/constant/relay_mode.go` 支持 `/pg/images/generations`。
- 在 `middleware/distributor.go` 让 `/pg/images/generations` 读取 `model/group` 并校验 group；读取方式必须可复用 body。
- 保证 `relay/common/relay_info.go` 的 `/pg` 重写仍将上游路径变成 `/v1/images/generations`。
- 评估是否给 `/pg/images/generations` 增加 `ModelRequestRateLimit()` 或等价限流，避免 Playground 生图绕开 `/v1` 的模型请求限流。

自检清单：

- `POST /pg/images/generations` 不要求 API token，只要求登录态。
- 请求缺少 model 时返回清晰错误。
- 请求缺少 prompt 或 prompt trim 后为空时，前端必须阻止提交；如做后端专用校验，必须不破坏 `/v1/images/generations` 兼容性。
- 请求带不可用 group 时返回 forbidden。
- 请求带 group 时实际按该 group 选渠道。
- `RelayMode` 是 `RelayModeImagesGenerations`，不是 `RelayModeChatCompletions` 或 unknown。
- `RelayFormat` 是 `OpenAIImage`。
- 上游路径是 `/v1/images/generations`。
- `group` 不出现在上游 JSON body。
- 即使渠道启用原始 body pass-through，`group` 也必须在 Playground controller 阶段被剥离，不能依赖 DTO marshal 才过滤。
- `Distribute()` 读取 `model/group` 后，`Relay()` 仍能完整解析同一个 body 中的 `prompt/size/quality/n/response_format`。
- `/pg/chat/completions` 原 group override 行为不变。
- 没有新增直接 `encoding/json` marshal/unmarshal 调用。
- `go test ./controller ./middleware ./relay/...` 或相关子集通过。

### 阶段 2：模型列表能力增强

开发项：

- 扩展 `controller.GetUserModels` 支持 `with_endpoint_types=true`。
- 返回对象时填充 `supported_endpoint_types`。
- 保持默认无 query 的字符串数组响应不变。
- 前端 `ModelOption` 增加 `supportedEndpointTypes?: string[]`。
- Playground 加载模型时使用增强接口。

自检清单：

- 旧调用 `/api/user/models` 仍返回 `string[]`。
- 新调用 `/api/user/models?with_endpoint_types=true` 返回对象数组。
- 普通用户只看到自己可用分组的模型。
- auto group 用户不丢模型。
- 生图模型能被过滤出来。
- 没有 `supported_endpoint_types` 的模型不会误显示为生图模型，除非后端明确按规则补充。
- 模型为空时 UI 有空状态。
- TypeScript 类型不使用 `any` 扩散。

### 阶段 3：前端 Image 模式状态与 API

开发项：

- 增加 `PlaygroundMode`、`ImageGenerationConfig`、`ImageTask`、`ImageGenerationRequest/Response` 类型。
- 增加 image storage helpers。
- 增加 `sendImageGeneration` API。
- 增加 `useImageGenerationHandler`。
- Chat 和 Image 模式状态隔离。

自检清单：

- prompt 为空、全空格或超出合理长度时不能发起请求，并给出清晰错误。
- 切回 Chat 后原聊天记录、stream 状态不受影响。
- Image 模式生成中不能重复提交同一任务，或重复提交有明确并发行为。
- 失败任务不会卡在 running。
- localStorage 存储失败不影响生成。
- base64 历史数量和体积受限。
- 刷新页面后不会恢复一个永久 running 的旧任务，应标记 interrupted 或 error。
- axios 错误能展示 OpenAI error message/code。

### 阶段 4：前端 Image UI

开发项：

- 增加模式切换控件。
- 新建 Image 输入区，包含 prompt、模型/分组、尺寸、质量、数量、返回格式。
- 新建任务结果网格和图片卡片。
- 支持预览、复制链接、下载 base64/url、复用 prompt、重试。
- 移动端适配。

自检清单：

- 首屏就是可用工作台，不是介绍页。
- 控件不溢出、不重叠。
- 图片容器有稳定宽高或 aspect-ratio。
- 文本在移动端按钮里不挤爆。
- icon 按钮有 tooltip 或 aria-label。
- loading/error/done/empty 四种状态齐全。
- 多图 `n > 1` 显示数量正确。
- `b64_json` 和 `url` 都能展示。
- 图片加载失败时仍能复制原始链接。
- UI 色彩不变成单一紫/蓝/灰主题，尽量贴合现有应用。

### 阶段 5：i18n 与文案

开发项：

- 所有新增文本用 `t('English key')`。
- 补全 en、zh、fr、ja、ru、vi。
- 运行 `bun run i18n:sync`。

自检清单：

- 没有裸中文或裸英文 UI 文案遗漏。
- `_sync-report.json` 无新增 missing key。
- 占位符如 `{{count}}` 在所有语言中一致。
- 技术词如 model、API、OpenAI、Base64 保持合理译法。
- locale JSON 没有乱码。

### 阶段 6：测试与验收

开发项：

- 后端单测覆盖 relay mode、Playground image controller、`GetUserModels` 兼容返回。
- 前端类型检查和构建。
- 至少用 mock 或真实可用渠道验证一次文生图。
- Playwright 或浏览器验证桌面/移动布局。

自检清单：

- `go test ./...` 或受影响包测试通过。
- `cd web/default && bun run typecheck` 通过。
- `cd web/default && bun run build` 通过。
- `/pg/chat/completions` 仍可正常聊天。
- `/pg/images/generations` 可正常生图。
- `/pg/images/generations` 与 `/v1/images/generations` 的 relay mode、上游路径、计费入口一致，差异只在登录态鉴权和 Playground 分组选择。
- 无生图模型时 UI 清楚提示。
- 渠道不可用、余额不足、模型无权限、上游 400/500 均有合理错误显示。
- 计费日志里生图数量和尺寸/质量合理。
- 浏览器控制台无关键报错。
- 文档、locale JSON、TS/TSX 文件均以 UTF-8 读取无乱码。

### 阶段 7：后续扩展预留

后续可选项：

- `/pg/images/edits` 图生图入口。
- 图片上传与引用图列表。
- mask 编辑器与局部重绘。
- 图片任务 IndexedDB 存储，避免 localStorage 限制。
- 生图流式 partial images。
- 更多 provider-specific 参数模板。
- 任务历史搜索、收藏、批量下载。

自检清单：

- 图生图必须使用 multipart 或兼容 JSON，不得破坏文生图接口。
- 上传文件大小限制和后端 BodyStorage 限制一致。
- mask 与 image 字段命名兼容 OpenAI、Ali、Replicate 等 adaptor。
- IndexedDB 数据迁移有版本号。
- 流式图片必须先确认 `ImageRequest.IsStream` 和后端 handler 支持。

## 方案自身缺陷与冲突检查

### 潜在缺陷 1：模型能力接口增加复杂度

如果扩展 `/api/user/models` 返回对象，前端和后端都要维护兼容逻辑。缺点是接口有双形态。优点是不会依赖 pricing 模块，也不会让 Playground 使用模型名猜测能力。

结论：可接受，但必须保持默认响应不变，并为新 query 增加测试。

### 潜在缺陷 2：首期不做图生图

用户可能期望“生图模型”包括图生图和编辑。首期只做文生图会显得功能不完整，但能明显降低 multipart、mask、上传限制、图片持久化的风险。

结论：首期范围合理；文档和 UI 不应宣称支持图生图。后续阶段单独实现 `/pg/images/edits`。

### 潜在缺陷 3：localStorage 不适合图片历史

如果保存 base64，容易超过浏览器配额。首期若默认 `response_format=url`，历史保存链接即可；`b64_json` 只应保存少量或当前会话结果。

结论：首期必须限制历史数量和 base64 持久化策略；长期应迁移到 IndexedDB。

### 潜在缺陷 4：不同供应商参数不完全一致

OpenAI、Gemini Imagen、Ali、Replicate 等对 `size`、`quality`、`output_format` 的语义不同。强行做 provider-specific UI 会复杂化首期。

结论：首期使用通用 OpenAI 兼容参数，并允许后端 adaptor 和渠道参数覆盖处理。错误由上游返回，UI 展示即可。

### 潜在缺陷 5：与聊天 Playground 视觉结构冲突

聊天是消息流，图片是任务网格。如果强行把图片塞进聊天气泡，会导致操作和预览都别扭。

结论：采用双模式隔离展示；共享模型/分组选择器和输入风格，但结果区域按图片任务网格设计。

### 潜在缺陷 6：`/pg` rate limit 与 `/v1` 不完全一致

`/v1` relay 路由有 `ModelRequestRateLimit()`，现有 `/pg` 没有该 middleware。新增生图可能比聊天更重，如果不加限制，Playground 可能成为高成本入口。

结论：需要评估是否给 `/pg` 统一增加或局部增加模型请求限流。若担心影响现有聊天，可先仅对 `/pg/images/generations` 增加专用限流或复用 `ModelRequestRateLimit()`。

### 潜在缺陷 7：使用 `/api/pricing` 过滤模型存在模块访问冲突

Pricing 接口受 header nav module 控制，管理员关闭 pricing 时不应影响 Playground。

结论：不把 `/api/pricing` 作为唯一数据源；新增增强模型接口是更一致方案。

### 潜在缺陷 8：分发器读取请求体后影响 relay 二次解析

Playground 生图需要在 `Distribute()` 中提前读取 `model/group`，而 `Relay()` 随后还要读取完整图片请求。如果实现时绕开现有 reusable body 机制，会出现分发阶段成功、relay 阶段 body 为空或字段缺失的问题。

结论：新增逻辑必须复用 `common.UnmarshalBodyReusable`、`common.GetBodyStorage` 或现有 `getModelFromJSONBody`；测试必须覆盖带 `group` 的请求仍能在 image handler 中读到完整 `prompt` 和参数。

### 潜在缺陷 9：空 prompt 校验边界不清

`dto.ImageRequest` 标注了 `prompt binding:"required"`，但当前实际校验函数没有强制 prompt required。直接恢复全局校验可能影响已有 `/v1/images/generations` 客户端兼容性。

结论：首期至少在前端阻止空 prompt；若后端也要强校验，应优先做 `/pg/images/generations` 专用校验或确认全局兼容影响后再改。

## 设计审查与文档质量门禁

每个阶段开发前后都需要按以下问题回看方案本身，避免方案文档和代码实现脱节：

- 是否符合目标：首期只承诺 Playground 文生图，不把聊天补全、图生图、遮罩编辑、流式生图混在一个交付里。
- 是否完善：权限、分组、模型能力、渠道分发、上游路径、计费、日志、错误、历史、i18n、构建、测试都有对应设计和验收项。
- 是否正确：`/pg/images/generations` 必须走 `RelayModeImagesGenerations` 与 `RelayFormatOpenAIImage`，不能走 `/pg/chat/completions`、`/v1` TokenAuth 或 `TextHelper`。
- 是否规范一致：Go JSON 操作遵循 `common.*`，前端使用 Bun 和现有组件/i18n 模式，数据库兼容要求不被无关改动破坏，不修改受保护项目标识。
- 是否存在上下设计冲突：前端模型过滤依赖 `supported_endpoint_types`，后端必须提供稳定数据；前端传 `group`，后端必须只用于分发且不透传上游；默认参数必须与 DTO 和 adaptor 能力一致。
- 是否有无关但逻辑关联遗漏：`/pg` 限流、body 复用、localStorage 配额、pricing 模块关闭、auto group、失败退款、日志展示、旧聊天模式回归都要检查。
- 是否有文档自身缺陷：文档路径应位于现有 `docs/` 体系，文件使用 UTF-8；代码路径、接口名、字段名与当前仓库一致；阶段清单能被开发者逐项执行，不只停留在描述性建议。
- 是否有乱码风险：中文 Markdown、locale JSON、终端输出和浏览器显示都要用 UTF-8 验证；如果 PowerShell 默认编码显示乱码，应以 `Get-Content -Encoding UTF8` 或编辑器 UTF-8 视图复核文件本身。

## 最终建议

建议按“后端入口优先、模型能力接口其次、前端双模式最后”的顺序开发。这样每个阶段都能独立验证：

1. 后端先用 curl 或浏览器请求验证 `/pg/images/generations` 能走通。
2. 模型接口再保证只显示正确模型。
3. 前端最后接入，避免 UI 做完后发现权限或分发链路不成立。

首期完成标准：

- 登录用户在 Playground 切换到 Image 模式。
- 下拉只显示支持 `image-generation` 的模型。
- 选择分组、输入 prompt 后生成图片。
- 图片能显示、复制链接或下载。
- 计费、日志、错误、i18n、构建和测试均通过。

## 多厂商生图重构（2026-07-15）

### 当前架构结论

现有实现已经形成完整 GPT 图片调用链：`/pg/images/generations` 经登录态分发进入 `RelayFormatOpenAIImage`，由 `relay.ImageHelper` 完成模型映射、渠道适配、响应归一化、日志和计费。此次重构保留该入口及统一的 `dto.ImageResponse`，不改动用户鉴权和核心扣费路径。

现存问题不是单个下拉框缺陷，而是能力来源分裂：

- 前端只允许 `gpt-image-2`，并把不认识的模型强制归一化为该模型。
- 图片页使用全部聊天分组，却只加载当前分组的模型；选中无图片模型分组后，组合选择器会被禁用。
- `size` 同时承担像素尺寸、宽高比和分辨率三种语义，`quality` 还被 Imagen 适配器用来表达 `imageSize`。
- 图片端点判断只按公开模型名，不考虑渠道类型和 `model_mapping` 后的真实上游模型。
- Gemini `imagen-*` 已支持 `:predict`，但 Gemini 原生图片模型需要 `generateContent + responseModalities + imageConfig`，不能只增加模型白名单。

### 目标架构

统一入口保持为：

```text
Playground /pg/images/generations
  -> group-aware image capability discovery
  -> normal relay distribution and model mapping
  -> provider image adaptor
     -> OpenAI/GPT images
     -> xAI images
     -> Gemini Imagen predict
     -> Gemini native generateContent
  -> dto.ImageResponse
  -> existing billing and logs
```

能力判断由 `pkg/imagecapability` 统一提供。能力键是“渠道类型 + 映射后模型”，同一分组和公开模型存在多个候选渠道时，对参数能力取保守交集，确保重试切换渠道后仍支持前端提交的参数。

### 参数契约

- GPT 使用 `size` 表示像素尺寸，并按能力公开 `quality`、`output_format`、编辑、审核和压缩选项。
- xAI 使用独立的 `aspect_ratio` 与 `resolution`，不再把两者复用到单个 `size` 字段。
- Imagen 使用 `aspectRatio` 与 `imageSize`，兼容旧请求的 `size/quality` 仅作为后端回退。
- Gemini 原生图片使用 `generateContent` 的 `generationConfig.responseModalities` 与 `imageConfig`，响应从 `inlineData` 归一化。
- 对外 `dto.ImageRequest` 保留旧字段，并新增可选 `aspect_ratio`、`resolution`，避免破坏现有 OpenAI 兼容客户端。
- 首期多厂商扩展只开放已实现的操作。GPT 保留参考图编辑；xAI、Imagen、Gemini 原生暂时只开放生成。

### 文件职责

- `pkg/imagecapability/types.go`：供应商无关的图片能力结构和尺寸模式。
- `pkg/imagecapability/registry.go`：GPT、xAI、Imagen、Gemini 原生和兼容图片模型规则。
- `common/model_mapping.go`：渠道与能力发现共用的链式模型映射解析。
- `dto/image_capability.go`：用户图片分组、模型及能力的 API DTO。
- `model/ability.go`：按用户可用分组读取启用 ability、渠道类型和模型映射。
- `service/image_capability.go`：构建分组级图片模型清单、auto 虚拟分组和候选渠道能力交集。
- `controller/user.go`：提供登录用户专用图片模型能力接口。
- `relay/channel/xai`：显式转换宽高比与分辨率并复用 OpenAI 图片响应。
- `relay/channel/gemini`：区分 Imagen 与 Gemini 原生图片协议并统一响应。
- `web/default/src/features/playground/hooks/use-playground-image-options.ts`：一次加载已过滤的图片分组及模型。
- `web/default/src/features/playground/lib/image-generation-capabilities.ts`：按服务端能力归一化配置，不再判断模型名称。
- `web/default/src/features/playground/components/playground-image-input.tsx`：按能力动态显示像素尺寸、宽高比、分辨率、质量和格式。

### 分阶段开发清单

| 阶段 | 任务 | 状态 | 验收 |
| --- | --- | --- | --- |
| 1 | 能力注册表、统一模型映射、架构文档和单元测试 | 已完成 | registry、mapping、现有 relay helper 测试通过 |
| 2 | 用户图片分组/模型能力接口和 auto 分组解析 | 已完成 | 无图片模型分组不返回；映射与候选渠道交集有测试 |
| 3 | xAI、Imagen、Gemini 原生请求与响应适配 | 已完成 | 参数转换、inlineData、usage 和错误响应测试通过 |
| 4 | 前端能力驱动控件、原子回退和选择器修复 | 已完成 | GPT 回归；Grok/Gemini 控件正确；空分组不可见 |
| 5 | 全量审查、格式、乱码、测试和生产构建 | 已完成 | 全仓 Go/TS/build 与本次文件 lint/format/diff 检查通过 |
| 6 | 提交并推送 `main` | 已完成 | 远端 `main` 包含最终提交 |

### 阶段审查要求

每阶段结束必须检查：实际路由与能力结果一致；旧 GPT 请求不变；候选渠道重试不暴露不支持参数；API 字段兼容；计费数量有边界；新增中文文档和 locale 均为 UTF-8；不存在替换字符、错误转义、调试输出、废弃硬编码或未使用文件。任何一项不通过，不进入下一阶段。

### 范围边界

本次不同时引入服务端异步任务系统。同步请求仍可能受到边缘代理超时影响，但异步任务涉及任务持久化、幂等、结算和状态查询，应在多厂商协议稳定后独立实施。能力 API 与统一响应结构会为后续异步化保留边界。

### 本次阶段自检记录

- 阶段 1：完成能力注册表和统一模型映射。发现并消除了旧 `ImageGenerationModels` 双规则源；registry、mapping、relay helper 测试通过。
- 阶段 2：完成用户图片能力接口。验证无能力分组过滤、禁用渠道排除、auto 分组聚合、映射后能力判断和多候选渠道保守交集；model、service、controller、middleware 测试通过。
- 阶段 3：完成 xAI、Imagen、Gemini 原生协议。补齐显式宽高比/分辨率、Gemini 分辨率别名、`inlineData`、MIME、usage 估算和图片编辑拒绝；供应商适配器测试通过。
- 阶段 4：完成前端能力驱动重构。移除 `gpt-image-2` 白名单和全局参数列表，改为一次加载有效图片分组；原子回退、参数隔离、参考图清理和 payload 测试通过。
- 阶段 5：全仓 `go test ./...` 通过；前端 `typecheck`、本次文件定向 lint、4 个行为测试和 production build 通过；`git diff --check`、gofmt、locale JSON、UTF-8 替换字符、冲突标记、许可证首行和敏感信息扫描无本次问题。
- 阶段 6：最终暂存内容通过差异完整性检查，按本清单提交并推送至远端 `main`。
- 已知仓库基线：全仓 lint、全仓 format/copyright check 和 `go vet` 仍会命中与本次无关的历史文件；本次涉及文件不在对应错误清单中。当前环境没有 Chromium/Playwright，未执行截图型浏览器验证，不影响编译、构建与纯逻辑交互测试结论。
