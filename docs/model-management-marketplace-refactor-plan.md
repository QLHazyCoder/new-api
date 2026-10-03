# new-api 模型管理与模型广场完整重构修复方案

- 盘点日期：2026-09-29
- 代码目录：`/opt/qlh-main/new-api`
- 文档性质：源码、历史提交、数据库和运行态证据基础上的实施方案；不是“已完成”声明
- 关联记录：
  - `docs/model-catalog-endpoint-capabilities.md`
  - `docs/model-marketplace-refactor-handoff.md`
  - `docs/custom-feature-preservation-checklist.md`
  - `docs/upstream-merge-v1.0.0-rc.39.md`

> 本文只描述如何完整修复和重构。代码、数据库、镜像和线上流量在实施阶段分别验证，不能用本文替代部署验收。

## 一、项目整体分析

### 1. 业务边界

模型管理页和模型广场不是同一个数据视图：

| 视图 | 主要目的 | 当前主要接口 | 权威事实源 |
| --- | --- | --- | --- |
| 模型管理 | 管理目录元数据、匹配规则、可见性和模型级定价 | `/api/models`、`/api/option/model_pricing` | `models`、定价 options、派生渠道数据 |
| 模型广场 | 向用户展示价格、分组、端点、目录规格和调用示例 | `/api/pricing` | enabled abilities、channels、mapping、插件路由、`models` 元数据、图片能力配置 |
| 用户模型选择器 | 只展示当前用户/分组实际可请求的模型和端点 | `/api/user/models?with_endpoint_types=true` | 当前用户可用 groups 内的 abilities 和 endpoint resolver |
| 图片 Playground | 展示真实可用的图片模型与参数 | `/api/user/image-models` | 用户分组、图片端点、`image-capabilities.json` |
| 管理诊断 | 解释端点来自哪个渠道/插件以及为什么被拒绝 | 当前没有完整安全 DTO | channel、ability、mapping、plugin generation、metadata |

这些视图可以共享模型名称，但不能共享未经范围过滤的端点或能力结果。

### 2. 当前端到端调用链

```text
models metadata
  + model name rule / status / endpoints
  + catalog specification fields
        |
        +-------------------------------+
        |                               |
enabled abilities + channels       admin /api/models
  + channel.model_mapping            + derived channels/groups
  + channel setting                  + square_state
  + plugin bindings                  + metadata editor
  + routing generation
  + image-capabilities.json
        |
        v
model.GetPricing()
        |
        +--> model-level pricing aggregate
        +--> supported_endpoint_types
        +--> image_capabilities
        +--> group visibility
        +--> description/icon/tags/vendor
        |
        v
GET /api/pricing
        |
        v
web/src/features/pricing
```

当前 `/api/pricing` 的模型对象没有再次读取 `/api/models`，因此只要 `model.Pricing` 没有字段，广场详情就不可能恢复这些数据。

### 3. 已验证的运行态证据

当前活动节点为 `new-api-green`。从容器内部读取 `/api/pricing`：

- `claude-sonnet-5` 原始对象的 keys 包含价格、分组、标签、供应商和端点，但不包含 `context_length`、`max_output_tokens`、`knowledge_cutoff`、`release_date`、`parameter_count`、`input_modalities`、`output_modalities`。
- `models` 表仍有七个历史列。
- `claude-sonnet-5` 数据库记录仍为 `context_length=1000000`、`max_output_tokens=128000`、`input_modalities=text,image`、`output_modalities=text`。
- `claude-opus-5` 数据库对应值为零或空，说明之后提交的规格确实被静默丢失。

这不是缓存单独造成的问题：当前 Go `Pricing` 结构体根本没有这些 JSON 字段，重新计算缓存也不会产生它们。

### 4. 历史回归证据

`076da1ef95be` 曾增加完整的模型规格链路：

- `model.Model` 增加七个持久化字段。
- `Model.Update()` 的 `Select(...)` 写入七个字段。
- `model.Pricing` 复制七个字段到 `/api/pricing`。
- 前端模型表单和详情页读取这些字段。

`dd3b02c35297` 又明确了硬规格只能使用后端显式数据，禁止前端按模型名猜测。

`4824d38fd` 合并 rc.39 时采用了上游版本的 `model/model_meta.go` 和 `model/pricing.go`，后端七字段消失，但前端类型、表单转换和详情渲染被保留，形成现在的“前端有契约、后端无实现”状态。

### 5. 开发清单和交接文档的结论

`docs/model-catalog-endpoint-capabilities.md` 已完成的阶段只覆盖：

- endpoint resolver；
- `openai_video` / `openai-video` 边界；
- 图片映射、能力文件和交集；
- 视频异步示例；
- 相关测试和发布前检查。

它没有定义通用模型规格字段的数据库、API、管理表单和广场详情契约。

`docs/model-marketplace-refactor-handoff.md` 补充指出了尚未闭环的事项：

- 全局端点聚合可能泄漏到用户分组；
- 陈旧 `models.endpoints` 可能污染公共目录；
- endpoint evidence 缺少 channel/plugin/upstream 维度；
- 图片端点和能力配置可能不一致；
- 视频请求 schema 和计费 schema 尚未分离；
- 渠道、插件和图片配置变更后的缓存失效不完整。

该交接文档当前仍是工作树中的未跟踪文件，本方案把它作为设计输入，不覆盖、不删除它。

### 6. 缺陷总表

#### P0：规格数据链路断裂

1. `web/src/features/pricing/components/model-details.tsx` 读取七个规格字段，但 `GET /api/pricing` 不返回它们。
2. `web/src/features/models/lib/model-form.ts` 的 schema 和 payload 保留七字段，但 `model-mutate-drawer.tsx` 没有任何规格输入控件。
3. `model/model_meta.go` 的 `Model` 不含七字段，`Update().Select(...)` 也不含七字段；未知 JSON 字段被 Go JSON 解码忽略，GORM 更新静默丢弃。
4. `Insert()` 也无法写入不在结构体中的字段，导致新建或编辑都无法补数据。
5. `AutoMigrate` 不会因为结构体字段被删除而删除旧列，因此数据库形成“有列、有历史值、无读写路径”的孤儿数据。

#### P1：事实源边界不完整

1. 前端 `PricingModel.capabilities` 没有后端结构体、数据库字段或管理端来源。
2. 输入/输出模态只有兼容性字符串解析，没有统一的服务端校验和规范化规则。
3. `/api/user/models?with_endpoint_types=true` 使用全局 `GetModelSupportEndpointTypes`，没有按当前用户 groups 重新解析 abilities。
4. `models.endpoints` 仍是持久化声明，和实时 resolver 结果合并后可能残留过期标准端点。
5. endpoint resolver 只返回字符串数组，无法解释来源、mapping、插件绑定和拒绝原因。
6. 图片 endpoint 存在但 capability 配置缺失时，广场、`/api/user/image-models` 和 Playground 的语义不一致。
7. API 详情页的 `VIDEO_PARAMS`、通用聊天参数和模型名正则不能代表实际模型请求能力。
8. `RateLimitsSection` 用确定性随机值生成 RPM/TPM/RPD，仍以“Rate limits”真实数据的形式展示。

#### P2：契约和维护性问题

1. 前端 `PricingModel.id` 是必填，但后端 `Pricing` 没有 id。
2. 前端 `supported_endpoint` 声明为字符串 map，后端实际返回 `{path, method}`。
3. `pricing_version` 同时存在于响应顶层和首个模型，不能作为稳定模型目录版本。
4. exact/prefix/suffix/contains 同一规则内没有确定性排序，重叠规则依赖数据库返回顺序。
5. P-10 被错误归档为 rc.40 等价，导致后续上游审计不再检查七字段子契约。

## 二、目标重构方案

### 1. 总体决策：选择 B+

不能采用“删除前端死字段和展示卡片”的方案 A。数据库仍有真实数据，历史提交也证明这是一个曾经存在的正式功能。

采用 B+：

1. 恢复七个模型规格字段的完整数据库、后端 API、管理表单和模型广场链路。
2. 保留端点/图片能力交接文档中的 resolver 和事实源边界。
3. 删除没有事实源的 `capabilities` 和静态伪能力，改为真实 endpoint/request capability。
4. 分离公共目录、用户分组和管理员诊断三种端点视图。
5. 修复缓存版本、元数据匹配和文档保护清单。

### 2. 目标事实源矩阵

| 数据 | 权威来源 | 目标展示语义 | 禁止行为 |
| --- | --- | --- | --- |
| 上下文长度 | `models.context_length` | 目录声明的最大输入窗口 | 不用于 relay 限制，不从名称猜测 |
| 最大输出 | `models.max_output_tokens` | 目录声明的单次最大输出 | 不用于强行截断请求 |
| 知识截止/发布日期 | `models.knowledge_cutoff`、`release_date` | 明确标记为目录资料 | 不自动从供应商名称推导 |
| 参数量 | `models.parameter_count` | 目录展示字符串 | 不把空值渲染为估算值 |
| 输入/输出模态 | `models.input_modalities`、`output_modalities` | 人工维护的目录声明 | 不等同于当前渠道实际支持 |
| 实际端点 | enabled abilities + channel + mapping + plugin generation | 可调用协议和路径 | 不由 metadata 凭空创造标准端点 |
| 图片参数 | endpoint + `image-capabilities.json` | 尺寸、比例、质量、格式等真实能力 | 不使用模型名兜底猜默认值 |
| 视频参数 | plugin request profile | 模型级请求字段和枚举 | 不把 billing schema 当请求 schema |
| 价格/计费 | ratio、fixed price、billing expression、usage schema | 用户可见价格和用量单位 | 不用目录规格推算价格 |
| 性能 | 性能指标 API | 实际窗口内统计 | 不使用随机值冒充真实限制 |

### 3. 模型规格数据契约

#### 存储兼容

- 保留现有七个数据库列，不做删除性迁移。
- 数值字段继续使用零表示未提供；有效值必须为正整数。
- 日期规范为 `YYYY-MM`；旧的空字符串继续表示未提供。
- `parameter_count` 保留字符串格式，例如 `70B`、`405B`。
- 模态字段数据库仍使用现有字符串列；写入时规范化为 JSON 数组字符串，读取时兼容历史逗号字符串。

#### API 兼容

- `/api/models` 和 `/api/pricing` 保留七个字段名。
- 管理请求接受历史字符串和数组形式的模态输入，服务端统一解析、去重、校验。
- 前端使用一个共享 `normalizeCatalogItems`，不在各组件重复解析。
- 空值必须能清除历史值；不能用 JavaScript `||` 把合法输入和未提供混淆。
- `Model` 持久化结构和管理 API DTO 分离；未知字段必须返回明确错误，不能静默丢弃。

#### 规格来源和同步

- 七个规格默认是人工维护的目录数据。
- 现有 `MetadataSyncFields` 继续只同步描述、图标、标签、供应商、端点、匹配规则和状态，不能让上游同步覆盖人工规格。
- 后续若增加外部规格同步，必须新增来源、版本和字段级冲突策略，不能直接复用现有同步接口。

### 4. 管理页面改造

#### 表单

在 `model-mutate-drawer.tsx` 的“Model metadata”中增加独立的“Catalog specification”区块：

- Context length；
- Max output tokens；
- Knowledge cutoff；
- Release date；
- Parameter count；
- Input modalities；
- Output modalities。

控件要求：

- 数值输入使用正整数校验，并允许清空。
- 日期使用年月选择或 `YYYY-MM` 文本校验。
- 模态使用枚举多选，不允许任意字符串污染目录。
- 保存前显示空字段数量；保存后重新读取 API，而不是只相信本地 mutation 响应。
- prefix/contains/suffix 规则显示影响的匹配模型数量和“规格会应用到匹配模型”的提示。

#### 管理表格

增加“规格完整度”列或过滤器：

- `0/7`：没有规格；
- `1-6/7`：部分规格；
- `7/7`：七项完整；
- 另显示“有实际渠道/无实际渠道”，避免把目录元数据误解为可调用能力。

从写入 payload 移除 `enable_groups`、`quota_types` 等派生字段，只保留只读展示。

### 5. 模型广场改造

#### 概览和详情

- 有值字段显示真实值。
- 部分字段缺失时显示“未提供”，不隐藏整个规格区域。
- 全部缺失时显示“该模型尚未维护目录规格”，而不是让用户误以为模型没有这些属性。
- 规格区域标注为“目录声明”，不暗示它是所有供应商渠道共同保证的运行时限制。
- 删除没有后端来源的 `capabilities` 字段和能力 pill；模态保留为目录声明。
- 真实可调用能力通过 API tab 的端点、请求参数、图片能力和示例表达。

#### 类型修正

- `PricingModel.id` 改为可选或从 pricing 类型删除。
- `PricingData` 增加顶层 `pricing_version`。
- `supported_endpoint` 使用 `{ path?: string; method?: string }`。
- 七个规格字段成为正式契约，不再保留“rc.39 不需要”的误导性注释。

### 6. Endpoint resolver 和三种聚合视图

保留 `docs/model-marketplace-refactor-handoff.md` 的设计方向，新增或拆分以下后端层：

```go
type ModelEndpointEvidence struct {
    PublicModel       string
    UpstreamModel     string
    ChannelID         int
    ChannelType       int
    Group             string
    PluginKey         string
    InternalProtocol  string
    PublicEndpoint    string
    Method            string
    Path              string
    Source            string
    Eligible           bool
    RejectionReason   string
}
```

规则：

1. 先解析 public model 到 upstream model，再查询 plugin generation。
2. type-60、type-61 和普通 adaptor 分别处理 binding，不用 channel type 名称猜能力。
3. `openai_video` 只存在于插件内部协议；公开 metadata 和 `/api/pricing` 使用 `openai-video`。
4. 标准端点路径来自 `common.GetDefaultEndpointInfo`；metadata 只能补充非标准 custom endpoint。
5. plugin-only 模型没有真实 Chat ability 时，不因为旧 `models.endpoints` 添加 `openai`。
6. 同一模型多渠道、多插件时保留 evidence，不能只返回扁平字符串数组。

三种接口语义：

| 视图 | 端点范围 | 对外字段 |
| --- | --- | --- |
| 公共 `/api/pricing` | 当前公开目录所有 enabled route | 端点摘要、脱敏来源、图片能力、规格和价格 |
| 用户 `/api/user/models` | 当前用户/指定 group 的 abilities | 只返回该范围实际支持的端点 |
| 管理 `/api/models` 诊断 | 全部 channel/plugin/upstream evidence | 可返回 channel ID、plugin key、拒绝原因，但不得返回密钥 |

### 7. 模型元数据匹配和端点声明

匹配优先级固定为：

```text
exact > prefix > suffix > contains
```

同类规则再按：

1. 匹配字符串长度降序；
2. 数据库 ID 升序；
3. 结果必须稳定、可测试。

`models.endpoints` 的目标语义：

- 标准 endpoint 以实时 resolver 为准；
- metadata 只允许声明非标准路径或补充目录描述；
- 启动迁移继续处理 `openai_video -> openai-video`；
- 新写入拒绝内部协议键；
- plugin-only exact model 只有在确认没有其他 Chat route 时清理旧 `openai` 声明。

### 8. 图片、视频和静态展示修复

#### 图片

- 保持 `/data/image-capabilities.json` 为图片参数事实源。
- 能力匹配使用映射后的 upstream model，返回仍使用 public model。
- image endpoint 存在但没有能力规则时，广场显示“能力未配置”，Playground 按既定策略隐藏或禁用，不生成默认参数。
- 多渠道能力继续使用保守交集，并在详情中说明“共同支持能力”。

#### 视频

- `usageProfiles` 继续只负责计费 schema。
- 新增或扩展 `requestProfiles`，至少表达 required、type、enum、range、public visibility 和 examples。
- 没有 request profile 时，API tab 只显示标准创建、轮询、内容下载流程和“插件未声明请求参数”。
- 切换 endpoint 时必须同时切换 path、参数、示例和请求格式，不能回退到 Chat Completions。

#### 静态伪数据

- 删除或明确隔离 `mock-stats.ts` 中的随机 RPM/TPM/RPD。
- `VIDEO_PARAMS` 和按模型名猜测的 common parameter profile 不得继续作为“支持的参数”事实。
- App ranking 若保留，必须显示预览/模拟标识；不能与真实性能指标混用。

### 9. 缓存、版本和失效

目录版本应由以下事实共同决定：

- models metadata 更新时间或内容 hash；
- abilities/channel status、models、mapping、setting；
- active plugin routing generation；
- image capability configuration version；
- billing expression/usage schema version。

失效矩阵：

| 变化 | 必须失效 |
| --- | --- |
| 模型规格、状态、匹配规则 | `/api/pricing`、模型广场、管理列表 |
| channel status/type/models/mapping/setting | pricing、endpoint map、用户模型选项、图片模型选项 |
| ability enabled/group | pricing、分组模型列表、用户模型选项 |
| plugin activate/deactivate/update | routing generation、endpoint/request capability、pricing |
| image capability 文件 | pricing image capabilities、Playground image models |
| billing expression/usage profile | pricing snapshot、模型详情价格、定价编辑器 |

`/api/pricing` 返回顶层 `pricing_version`，并支持 ETag 或等价的条件请求。React Query 的五分钟 stale time 不能是唯一同步机制。

### 10. 数据迁移与 31 个模型回填

迁移顺序：

1. 只读盘点七列、非空值、异常格式和目标模型列表。
2. 先发布后端读写链路，再通过管理 API 或受控导入工具回填。
3. 每条回填必须执行 API 写入、API 读回、`/api/pricing` 读回三重验证。
4. 记录成功、跳过、格式错误和冲突数量，不直接覆盖人工已有值。
5. 未经单独授权不直接修改生产 MySQL；不删除七个历史列。
6. 旧 `models.endpoints` 迁移和规格回填分开执行，避免把端点清理误认为规格迁移成功。

## 三、文件结构设计

> 本节中的路径分为两类：已经存在的路径是在其现有职责上调整；当前不存在的 `dto/model_catalog.go`、`model/model_endpoint_evidence.go` 等路径是目标结构，必须在实施清单中新增并经过测试后才能视为完成。规划文件不代表这些文件已经创建。

### 后端和 DTO

| 路径 | 作用 | 变更 |
| --- | --- | --- |
| `model/model_meta.go` | 模型目录实体、规则匹配、管理读写 | 恢复七字段；修正匹配确定性；保留派生字段只读 |
| `model/model_metadata_sync.go` | 上游元数据同步和校验 | 明确七字段手工维护；保留 endpoint 内部键校验 |
| `model/pricing.go` | `/api/pricing` 聚合 | 复制七字段；只编排 resolver 和能力结果 |
| `dto/model_catalog.go` | 目录 request/response DTO | 新增字段白名单、日期、数值和模态校验 |
| `model/model_endpoint_evidence.go` | 端点证据类型 | 新增来源、路径、插件和拒绝原因结构 |
| `model/model_endpoint_resolver.go` | ability 级解析 | 统一普通 adaptor、type-60、type-61 和插件协议 |
| `model/model_endpoint_aggregate.go` | 三种端点视图 | 公共、用户分组、管理员诊断分开聚合 |
| `model/model_endpoint_cache.go` | 版本和缓存 | 纳入 metadata/channel/plugin/image 版本 |
| `model/main.go` | 数据表迁移入口 | 只做兼容检查和非破坏性规范化 |
| `controller/model_meta.go` | 管理模型 API | 改用显式 DTO，拒绝未知字段 |
| `controller/pricing.go` | 公共 pricing API | 返回规格、顶层版本和安全端点摘要 |
| `controller/user.go` | 用户模型选项 | 按用户/分组重新计算 endpoint |
| `dto/model_endpoint.go` | 对外安全 endpoint DTO | 管理证据和公开摘要分层 |
| `pkg/jsplugin/request_schema.go` | 插件请求参数 schema | 新增 request profile 时使用 |

### 前端

| 路径 | 作用 | 变更 |
| --- | --- | --- |
| `web/src/features/models/types.ts` | 管理模型类型 | 七字段正式化，派生/持久字段分开 |
| `web/src/features/models/lib/model-form.ts` | 表单 schema 和转换 | 加强校验、清空语义和模态规范化 |
| `web/src/features/models/components/drawers/model-mutate-drawer.tsx` | 编辑抽屉 | 增加完整 Catalog specification 区域 |
| `web/src/features/models/components/models-columns.tsx` | 管理表格 | 增加规格完整度和数据状态 |
| `web/src/features/pricing/types.ts` | 广场 API 类型 | 修正 id、endpoint map、pricing version 和规格类型 |
| `web/src/features/pricing/hooks/use-pricing-data.ts` | 广场数据查询 | 处理顶层版本和缓存失效 |
| `web/src/features/pricing/components/model-details.tsx` | 模型详情 | 展示完整、部分和未提供的规格，不显示伪能力 |
| `web/src/features/pricing/components/model-details-api.tsx` | API tab | 使用真实 endpoint/request capability 联动示例 |
| `web/src/features/pricing/lib/mock-stats.ts` | 旧静态参数/模拟统计 | 删除伪事实或明确隔离为预览数据 |
| `web/src/features/pricing/lib/endpoint-evidence.ts` | 端点来源解释 | 新增安全摘要格式化 |
| `web/src/features/pricing/lib/request-samples.ts` | 请求示例 | 按 endpoint 和 request profile 生成 |
| `web/src/features/pricing/lib/media-capabilities.ts` | 图片/视频能力转换 | 统一能力到参数表的映射 |

### 测试和文档

| 路径 | 作用 |
| --- | --- |
| `model/model_catalog_test.go` | 七字段存取、校验、清空和规范化 |
| `model/model_endpoint_resolver_test.go` | channel、mapping、plugin 和 endpoint evidence |
| `controller/model_catalog_test.go` | API DTO、未知字段和权限边界 |
| `controller/model_endpoint_test.go` | 公共/用户/管理员三种视图和脱敏 |
| `controller/user_models_test.go` | 用户分组端点范围 |
| `model/pricing_endpoint_test.go` | 真实 endpoint、图片能力和禁用渠道 |
| `web/src/features/models/__tests__/metadata-editing.test.tsx` | 规格表单渲染、保存、清空和错误保留草稿 |
| `web/src/features/pricing/__tests__/model-details-catalog.test.tsx` | 完整/部分/未知规格展示 |
| `web/src/features/pricing/__tests__/endpoint-evidence.test.ts` | 多渠道、多插件和范围过滤 |
| `web/src/features/pricing/__tests__/model-details-api.test.tsx` | endpoint 切换、request profile 和示例联动 |
| `web/src/features/pricing/lib/__tests__/media-samples.test.ts` | 图片能力、视频异步流程和缺失配置 |
| `docs/model-catalog-endpoint-capabilities.md` | 补充规格契约和边界，保留端点/图片历史记录 |
| `docs/custom-feature-preservation-checklist.md` | 恢复 P-10 或新增 P-36 保护项 |
| `docs/upstream-merge-v1.0.0-rc.39.md` | 记录旧合并判断遗漏的规格子契约 |

## 四、开发清单

| 阶段 | 工作项 | 状态 | 完成条件 |
| --- | --- | --- | --- |
| 0 | 保存源码、数据库和活动节点快照 | 已完成 | 已确认 `/api/pricing` 缺字段、数据库保留列和运行态模型值 |
| 1 | 恢复七字段后端实体、DTO、API 和 pricing 复制 | 待实施 | create/update/get/pricing 全字段读回；清空值可验证 |
| 2 | 规范化模态、日期、数值和 metadata 匹配 | 待实施 | 历史 CSV/JSON 兼容；重叠规则稳定；非法值拒绝 |
| 3 | 完成管理端规格编辑和完整度 | 待实施 | 抽屉有七个输入；payload 无派生写字段；保存后重新读回 |
| 4 | 完成广场规格展示和类型契约 | 待实施 | 完整、部分、未知状态均正确；无 client inference |
| 5 | 完成 endpoint evidence 和用户分组端点 | 待实施 | 公共、用户、管理员三种视图隔离；无范围泄漏 |
| 6 | 统一图片能力并分离视频 request schema | 待实施 | 缺配置不虚构参数；计费 schema 不再冒充请求 schema |
| 7 | 清理静态参数、随机速率限制和死类型 | 待实施 | 无伪真实展示；模拟内容有明显标识 |
| 8 | 完善缓存、版本、旧键迁移和回填工具 | 待实施 | 配置变化可主动失效；迁移幂等；回填有审计读回 |
| 9 | 更新保护清单、文档和测试矩阵 | 待实施 | 文档状态与代码状态一致，P-10/P-36 有验证入口 |
| 10 | 全量测试、发布和线上只读核对 | 待实施 | Go、前端、插件、数据库、公共 API、用户 API 全部验收 |

## 五、阶段自检记录和验收门槛

### 阶段 0：现状核对（已完成）

- 已核对 `model_meta.go`、`pricing.go`、model form、编辑抽屉和详情页。
- 已核对 `076da1ef95be`、`dd3b02c35297`、`4824d38fd` 历史行为。
- 已核对运行态 `/api/pricing` 和数据库七列/示例数据。
- 已识别工作树中已有用户改动和未跟踪交接文档，未覆盖它们。

### 阶段 1-2：后端规格契约

自检必须覆盖：

- `Model` 字段、`Insert`、`Update Select`、查询、JSON DTO 字段集合一致；
- 空值和清空值可写入；
- 七字段不会被 `MetadataSyncFields` 误覆盖；
- exact/prefix/suffix/contains 命中顺序稳定；
- `/api/pricing` 使用与 `/api/models` 相同的规格值；
- `status=0` 的目录元数据不会进入公共 pricing；
- 无未知字段静默丢弃。

### 阶段 3-4：前端管理和广场

自检必须覆盖：

- 编辑已有 `claude-sonnet-5` 时能看到 `1M/128K/text,image -> text`；
- `claude-opus-5` 的空值能显示为空而不是伪造默认值；
- 修改并清空字段后服务端和 pricing 都同步；
- 部分规格不导致整个详情区域消失；
- 规格字段不参与价格、端点和 API 请求参数推断；
- 端点 map 的 path/method 不再依赖强制类型转换。

### 阶段 5-7：能力和端点

自检矩阵至少包括：

| 场景 | 预期 |
| --- | --- |
| 一个普通 OpenAI channel | 只显示 adaptor 实际端点 |
| Task Plugin only | 只显示绑定 plugin protocol，不自动添加 Chat |
| 同一公开模型同时有 Chat 和 Video | 公共目录显示两者，evidence 区分来源 |
| Chat 只在 group-b，Video 只在 group-a | group-a 用户不获得 Chat |
| 图片 endpoint 无 capability rule | 不虚构 size/quality；Playground 按策略隐藏或标记 |
| 多渠道图片能力不同 | 使用明确的交集或 provider profile 策略 |
| 视频 billing enum 与 request schema 不同 | 参数表只来自 request profile |

### 阶段 8-10：发布前和线上验收

必须保留以下证据：

1. Git commit、镜像 revision 和活动 blue/green slot。
2. 数据库迁移前后的 `models` 列和值统计，禁止输出密钥。
3. `/api/pricing` 规格、端点、顶层版本和用户分组过滤结果。
4. `/api/user/models?with_endpoint_types=true` 的用户范围结果。
5. `/api/user/image-models` 与图片能力配置的一致性。
6. 插件 runtime generation 和 request profile 读回。
7. `go test ./...`、前端类型检查、目标 Vitest、lint、生产构建和 `git diff --check`。
8. 发布后等待缓存刷新，并重新执行上述公共/用户接口只读核对。

## 六、最终审查结论

当前问题的根因是 rc.39 合并时只恢复了上游文件形状，没有按 P-10 的数据和展示行为逐项证明等价。前端保留的七字段因此变成死契约，数据库旧数据变成孤儿数据，模型广场详情页又继续读取不存在的 `/api/pricing` 字段。

正确的最终状态必须同时满足：

1. 管理员可以可靠写入、清空和读回七个目录规格字段。
2. `/api/pricing` 返回同一份规格，广场不再因后端缺字段而永远空白。
3. 未提供的数据显示为未知，不通过模型名、标签或端点猜测。
4. 目录规格、真实端点、图片能力、视频请求能力和计费 schema 各自有明确事实源。
5. 用户模型选择器不再复用全局端点结果造成分组范围泄漏。
6. endpoint evidence、缓存版本、旧键迁移和文档保护项均有自动化验证。
7. 31 个模型的规格回填通过受控 API 和三重读回完成，不通过未经授权的生产数据库直改。

因此，本项目应执行 **B+ 完整恢复与边界修正方案**，不应采用仅删除遗留字段的 A 方案，也不应把端点/图片阶段的旧“已完成”结论扩展为模型规格链路已完成。
