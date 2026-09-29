# 模型广场端点与图片能力维护记录

## 项目整体分析

- `abilities` 加上已启用渠道才代表可用能力；`channels.model_mapping` 将公开名称解析为上游模型。
- 插件路由索引按渠道绑定、模型和协议声明提供标准端点；内部 `openai_video` 与公开 `openai-video` 不能混用。
- 图片能力使用可热加载的 `/data/image-capabilities.json`（支持 `IMAGE_CAPABILITY_CONFIG_FILE`），插件协议不能替代尺寸、比例等可用参数配置。

## 重构方案

1. 模型广场从已启用渠道与实际插件协议推导端点，保留普通渠道既有适配器行为。
2. Playground 与模型广场共用映射后的图片配置；多渠道取参数交集，缺少配置时不声称未知参数可用。
3. 启动时幂等迁移 `models.endpoints` 的 `openai_video`；冲突保留规范键并记录日志。插件独占渠道的精确模型删除错误的默认聊天声明；新写入旧键报错。插件源码的内部协议不变。
4. 图片示例使用实际默认参数，视频示例使用标准异步创建、状态查询和内容接口；价格与性能统计不变。

## 文件结构设计

| 路径 | 职责与调整原因 |
| --- | --- |
| `model/ability.go` | 定价能力查询携带渠道映射与插件设置，排除已禁用渠道。 |
| `model/pricing_endpoint_resolver.go` | 按模型、插件绑定和实际协议路由解析公开端点。 |
| `model/pricing.go` | 汇总公开端点和图片能力；配置热加载后刷新缓存，不改计费数值。 |
| `common/endpoint_defaults.go` | 维护公开的 `openai-video` 默认路径 `/v1/videos`。 |
| `model/model_endpoint_migration.go` | 幂等迁移历史模型元数据；不改插件协议或计费选项。 |
| `model/main.go` | 完成表迁移后调用一次历史公开键迁移。 |
| `model/model_metadata_sync.go` | 拒绝在新公开元数据中写入内部协议名。 |
| `common/model.go` | 映射公开图片名到上游能力，尊重配置的渠道排除规则。 |
| `pkg/imagecapability/registry.go` | 继续加载图片能力文件，并提供配置版本用于缓存失效。 |
| `dto/image_capability.go` | 统一 Playground 和广场输出的图片能力 DTO。 |
| `service/image_capability.go` | Playground 只列出兼具路由与能力配置的图片模型。 |
| `controller/user.go` | 用户模型选项读取真实汇总端点，不再按模型名添加图片类型。 |
| `web/src/features/pricing/types.ts` | 承接广场的图片能力返回值。 |
| `web/src/features/pricing/lib/mock-stats.ts` | 参数表只展示选中端点和已配置的图片参数。 |
| `web/src/features/pricing/lib/media-samples.ts` | 图片与视频标准端点的可复制请求、轮询及下载示例。 |
| `web/src/features/pricing/components/model-details-api.tsx` | 复用现有标签页、表格和代码块，联动端点示例及参数。 |
| `model/pricing_endpoint_test.go` | 模型映射、插件绑定/多协议及禁用渠道回归。 |
| `model/model_endpoint_migration_test.go` | 旧键/冲突迁移、幂等性与新键校验回归。 |
| `controller/user_models_test.go` | 用户可选端点按真实渠道输出的回归。 |
| `controller/model_list_test.go` | 旧夹具补充真实渠道，保持计费模型列表回归有效。 |
| `service/image_capability_test.go` | 未绑定插件图片模型不得出现在 Playground。 |
| `common/model_mapping_test.go` | 图片映射与配置排除规则回归。 |
| `pkg/imagecapability/registry_test.go` | 热加载配置版本递增回归。 |
| `web/src/features/pricing/lib/__tests__/media-samples.test.ts` | 图片可选参数及视频创建/查询/内容示例回归。 |
| `web/src/features/pricing/__tests__/model-details-api.test.tsx` | 用户切换媒体端点时参数表同步切换。 |
| `docs/custom-feature-preservation-checklist.md` | 在 P-13 记录本次跨模块维护入口。 |
| `docs/model-catalog-endpoint-capabilities.md` | 记录设计、职责、阶段自检与发布前检查。 |

## 开发清单

| 阶段 | 任务 | 状态 |
| --- | --- | --- |
| 0 | 核对渠道、插件、映射、图片配置与广场展示 | 已完成 |
| 1 | 后端模型级解析、迁移与图片能力 | 已完成；全量 Go 测试通过 |
| 2 | 前端参数及可复制媒体示例 | 已完成；相关测试/类型检查/构建通过 |
| 3 | 全量测试、差异审阅与验收 | 已完成；尚未部署，需上线后只读核对 |

## 阶段自检记录

| 阶段 | 状态 | 发现问题 | 修复动作与更新结果 |
| --- | --- | --- | --- |
| 0：梳理 | 完成 | 目录忽略映射/绑定；图片配置与插件协议分散；视频误用聊天示例。 | 明确插件负责路由、图片配置负责可选参数；按此划分后端与前端职责。 |
| 1：后端 | 完成 | 旧图片模型名兜底覆盖排除规则；旧公开键与插件内部键混用；禁用渠道仍可能提供目录能力。 | 统一解析器、真实渠道筛选、旧键幂等迁移及受限查询；相关 Go 回归通过。 |
| 2：前端 | 完成 | 图片/视频参数为无来源的静态猜测；选择视频端点后仍可能显示聊天参数。 | 使用后端图片能力构造示例，视频采用创建/轮询/下载；静态限流逻辑保持不变，端点切换和 267 个相关测试用例通过。 |
| 3：复审 | 完成 | 一项旧 Go 测试缺少渠道夹具；并发执行构建干扰根包测试。 | 补齐夹具，分开执行构建与全量 Go 回归，`go test ./...`、类型检查及生产构建通过；目标文件 lint 无错误。 |

- 旧静态限流模拟代码仍有 3 条与本次无关的嵌套三元表达式 lint warning，本阶段未调整静态指标或 RPM/TPM/RPD。
- 已核对未变更价格和插件内部 `openai_video`；未更改生成产物与部署配置。新功能所在的服务器代码尚未发布。
- 发布前应备份 `models.endpoints`；新版本启动后只读核对迁移日志、广场端点和 Playground 生成功能。

## 最终审查结论

代码与回归测试验收通过，无已知未修复的本次功能缺陷。启动迁移需在正式发布时执行，且尚未经过真实线上数据迁移验证；未发布前，本记录不代表线上端点或数据库已经更新。
