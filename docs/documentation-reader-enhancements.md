# 文档阅读页增强设计与开发记录

## 1. 项目整体分析

公开阅读由 New API 的 `/docs`、`/docs/{slug}` 和 `PublicLayout` 承载，内容来自同域
`/api/public/docs*` 的已发布快照。Tools 继续维护目录、草稿、发布和 `docs.db`。本次调整只
涉及 New API 阅读前端，不修改数据库、后端接口、Caddy 或 Tools。

第一阶段已完成目录换行、标题锚点和代码复制，但 2200px 居中外壳叠加 960px 正文上限，
在用户的 3828px 屏幕上仍产生大片空白。原验收只覆盖至 1920px 且强调不溢出，未验证
页面空间利用，不能证明超宽屏设计达标。本轮按用户确认的 PC 优先方案修正。

共享 Markdown 仍使用 Marked、DOMPurify 和 HTML 字符串输出。同源文档导航已经使用
内部路由，旧缓存导致的浏览器新标签现象不增加特殊兼容逻辑。

## 2. 重构方案

- PC 视口使用全宽布局，不设 2200px 外壳上限，不居中收窄；1024px 起左侧固定 380px
  文档目录，名称和文章标题自动换行。
- 正文填满剩余区域，去掉 960px 上限；1280px 起右侧固定 380px 文章目录并贴视口右侧，
  章节文字可用宽度相应增加，H3 不再缩小字号。正文使用 16px 基础字号和 32px 段落行高。
- 小于 1280px 直接隐藏章节目录，删除 `MobileArticleToc`、其事件/焦点状态、两项专用
  测试及七种语言的开关文案；不再维护手机章节抽屉。小于 1024px 的文档选择沿用既有
  左侧抽屉，不新增移动交互。没有 H2/H3 时正文占满全部剩余宽度。
- 文章目录仅展示 H2/H3，正文所有 Markdown 标题都有稳定锚点。使用同一个 Marked 解析器
  提取标题和渲染正文，避免代码、数学块或嵌套引用造成目录与锚点错位。
- 标题纯文本保留中文和数字，空标题使用 `section`；生成 ID 时统一检查已占用集合，重复标题
  及原本带有 `-2` 的标题也不会撞 ID。修改标题可能改变对应章节链接，文章 slug 不受影响。
- 点击文章目录使用 TanStack Router 更新 hash，在当前标签内滚动；加载带 hash 的深链后再次
  定位到已加载标题。IntersectionObserver 配合按帧合并的滚动/尺寸事件维护当前章节，长段落
  或快速滚动也可高亮，文末较短的最后章节也能正确选中。标题预留 96px 顶部空间，并遵守
  减少动态效果的系统设置。外部图片和字体加载后校正深链位置；用户主动交互后停止校正。
- 普通 fenced code 使用右上角复制操作栏，保留代码缩进、空行和字符，只复制代码正文。
  行内代码、缩进代码、公式及流程/时序图不增加按钮。成功提示保持约两秒，失败使用现有
  Sonner 错误提示。文章切换或卸载时清理监听器、定时器及 Observer。
- 复制按钮的语言变更只更新控件，章节高亮更新不重建正文，避免替换已被观察的标题节点。
- 链接保持标准 Markdown：文字链接显示文字，裸 URL 显示地址；HTTP、HTTPS、mailto 链接
  可点击并保留查询参数/片段，外链带 `noopener noreferrer`。危险协议经 DOMPurify 过滤。
  HTML 禁用、独立图片 URL 和 HTTPS 图片边界保持原契约。

## 3. 文件结构与职责

| 文件 | 调整和职责 |
| --- | --- |
| `web/src/features/docs/docs-layout.tsx` | 全宽外壳、贴左 380px 目录、保留既有窄屏文档选择 |
| `web/src/features/docs/document-tree.tsx` | 长标题与文件夹名称完整换行、悬浮完整名称 |
| `web/src/features/docs/docs-article.tsx` | 获取文章和标题模型；正文填满两侧目录之间；移除移动章节入口 |
| `web/src/features/docs/article-toc.tsx` | 贴右 380px 粘性章节列表；删除手机章节抽屉及相关状态 |
| `web/src/features/docs/use-article-navigation.ts` | 新增章节滚动、hash 恢复、当前章节高亮和生命周期清理 |
| `web/src/features/docs/markdown.ts` | 图片规则保持；新增标题 ID 去重和规范化 |
| `web/src/features/docs/markdown-view.tsx` | 文档正文排版、HTML 禁用、HTTPS 图片、标题 ID、代码复制 |
| `web/src/components/ui/markdown.tsx` | 兼容扩展：标题提取、可选标题 ID 及 fenced code 复制；默认调用不增加控件 |
| `web/src/features/docs/__tests__/markdown.test.tsx` | 标题/代码/图片/链接的渲染与剪贴板行为 |
| `web/src/features/docs/__tests__/reading.test.tsx` | 文章目录交互、深链、切换清理和匿名阅读回归 |
| `web/src/components/ui/__tests__/markdown.test.tsx` | 非文档调用默认行为及现有公式渲染回归 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json` | 保留目录标题；清理移除的手机章节开关文案 |
| `docs/custom-feature-preservation-checklist.md` | 在 P-37 补充公开阅读交互和安全契约 |
| `docs/documentation-reader-enhancements.md` | 本次设计、文件职责、阶段清单、自检与验收记录 |

复用检查：`CopyButton` 适用于 React 元素树，当前 HTML 字符串输出不能直接嵌入该组件。
因此采用计划中的事件委托和现有 `copyToClipboard` 工具，按钮复用 `buttonVariants`；
不创建第二套剪贴板降级逻辑，不新增依赖。布局按项目 shadcn-ui 技能复用现有主题颜色、
间距和组件能力；不调整其他页面的 `PublicLayout` 或顶部导航样式。

## 4. 开发清单

| 阶段 | 任务 | 状态 | 自检与修复 |
| --- | --- | --- | --- |
| PC 布局修正 | 左右各 380px、自适应正文、标题换行 | 完成 | 八种屏宽实测通过，3828px 正文为 3068px；左右各 380px 且贴视口边缘 |
| 章节导航简化 | 桌面 H2/H3、深链、高亮；窄屏隐藏章节目录 | 完成 | 删除手机章节抽屉，桌面导航与复制回归通过；窄屏直接隐藏章节导航 |
| 代码复制 | 原文复制、成功恢复、失败提示、生命周期清理 | 完成 | 保留缩进、空行及特殊字符；复制成功/失败和两秒恢复测试通过 |
| 链接与安全 | 标准链接、危险协议、原始 HTML 和图片边界 | 完成 | HTTP/HTTPS/mailto 可点击；危险协议被过滤；代码 URL 和图片规则通过 |
| 最终审查 | 文档/共享测试、类型、lint、格式、构建、PC 浏览器布局 | 完成 | 46 项测试及最新构建通过，八种屏宽几何检查通过 |

## 5. 阶段自检记录

| 发现的问题 | 修复动作 | 验证结果 |
| --- | --- | --- |
| 超宽屏外壳居中且正文上限导致大块留白 | 去掉两层宽度限制，左右各 380px，正文自动扩展 | 修正前 3828px 实测左栏起点 814px、正文 960px；当前起点 0、正文 3068px、右栏右边缘 3828px |
| 嵌套标题或数学块使目录和渲染顺序可能不同，重复后缀可能撞 ID | 提取和渲染共用 Marked 扩展；使用已占用 ID 集合去重 | 中文、嵌套、行内格式、Setext 标题和重复后缀测试通过 |
| 当前章节或语言变化可能替换正文 DOM，使观察节点和焦点失效 | 稳定 HTML 渲染对象；翻译更新仅修改代码按钮 | 标题引用稳定性、复制状态和章节交互测试通过 |
| 文末最后一节较短，标题不能滚至页面顶部 | 到达文末时选中最后章节 | 自动测试和浏览器末节点击通过 |
| 深链首次定位后，图片/字体加载改变正文高度 | 加载完成后校正；主动滚动、触摸、点击或按键后停止校正 | 延迟图片、主动滚动中止和带中文片段刷新通过 |
| 手机章节交互增加维护且不符合用户 PC 优先要求 | 删除章节抽屉、焦点和关闭事件；窄屏直接隐藏 | 新增单套桌面章节导航测试；窄屏由浏览器确认无额外章节按钮 |
| Marked 默认代码块带额外换行，不能直接作为复制结果 | 可复制代码块转义原始 token 文本，事件委托读取对应 code 文本 | 缩进、空行、特殊字符、失败提示和成功恢复通过 |

## 6. 最终审查结论

本轮 PC 布局修正验收通过。没有数据库、公开 API、Caddy、Tools、依赖或公共顶部导航样式
变更。保留 PC 章节高亮、深链、安全图片/链接和代码复制能力，删除手机专用章节交互。

| 检查 | 实际命令或方式 | 结果 |
| --- | --- | --- |
| 功能回归 | `bun run test -- src/features/docs/__tests__ src/components/ui/__tests__/markdown.test.tsx src/lib/__tests__/navigation-target.test.ts --maxWorkers=1` | 6 个文件、46 项测试全部通过；替换原有两项手机章节交互测试 |
| 类型检查 | `bun run typecheck` | 通过 |
| 定向 lint | `bun x --no-install oxlint -c .oxlintrc.json src/features/docs/article-toc.tsx src/features/docs/docs-layout.tsx src/features/docs/docs-article.tsx src/features/docs/markdown-view.tsx src/features/docs/__tests__/reading.test.tsx` | 退出码 0，无 error 或 warning |
| 格式检查 | `node /tmp/new-api-docs-qa.KUeLRL/format.mjs --check` | 12 个前端变更文件通过，检查过程中保护并恢复原始版权头 |
| 生产构建 | `bun run build` | 最新产物构建通过 |
| 浏览器验收 | `node /tmp/new-api-docs-qa.KUeLRL/reader-qa.mjs` | 3828/2560/1920/1440/1280/1024/900/390px 八种屏宽通过，检查了全宽利用、章节定位、粘性、复制、同标签导航、空状态和 404 |
| 真实文章预览 | `node /tmp/new-api-docs-qa.KUeLRL/reader-qa.mjs --published-preview` | 只读取得已发布的《New API 视频插件接入指南》，用本轮构建生成 1920px 和 3828px 预览，页面无脚本错误 |
| 请求与页面错误 | 浏览器全量请求和 `pageerror` 采集 | `/tools` 请求 0，页面脚本错误 0，文档数据仅访问 `/api/public/docs*` |
| 文案与差异 | 七种语言 JSON/新增文案校验、既有版权头逐文件对比、`git diff --check` | 全部通过 |

左右侧统一为 380px 的后续调整只修改两处布局数值及相关记录：保留上表 46 项功能回归
基线，本次重新运行类型、定向 lint、格式、构建、八种屏宽浏览器测量和真实文章预览。

关键 PC 几何结果（CSS px）：

| 视口宽度 | 左栏起点/宽度 | 正文起点/宽度 | 章节栏宽度/右边缘 |
| --- | --- | --- | --- |
| 3828 | 0 / 380 | 380 / 3068 | 380 / 3828 |
| 2560 | 0 / 380 | 380 / 1800 | 380 / 2560 |
| 1920 | 0 / 380 | 380 / 1160 | 380 / 1920 |

无 H2/H3 文章在 2560px 下正文实测为 2180px，确认不会为隐藏的章节栏预留空白。

浏览器使用本机现有 Chromium，拦截请求并从 `web/dist` 提供最新构建文件，公开 API 和文章
内容使用独立 fixture。未启动本地开发/预览服务，未连接生产文档数据库。验收报告及截图为
临时产物：`/tmp/new-api-docs-qa.KUeLRL/pc-report.json`、`pc-1920.png`、`pc-3828.png`；临时脚本
不加入项目依赖或 Git。数据库发布/下架规则沿用原实现，本次只验证对应公开读取的 UI 契约。
真实文章预览分别位于同一目录的 `published-1920.png` 和 `published-3828.png`；这两张图使用
线上公开文章正文与本地最新构建，站点外壳及目录数据为独立验收 fixture，未修改线上内容。

发布边界：本轮 PC 布局修正完成后保留在工作区，不自动提交、推送或部署。
