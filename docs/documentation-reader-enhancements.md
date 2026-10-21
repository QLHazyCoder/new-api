# 文档阅读页增强设计与开发记录

## 1. 项目整体分析

公开阅读由 New API 的 `/docs`、`/docs/{slug}` 和 `PublicLayout` 承载，内容来自同域
`/api/public/docs*` 的已发布快照。Tools 继续维护目录、草稿、发布和 `docs.db`。本次调整只
涉及 New API 阅读前端，不修改数据库、后端接口、Caddy 或 Tools。

原页面左侧目录固定 280px 且长标题省略，正文最大 800px 并在大屏主区域居中，没有文章目录。
共享 Markdown 使用 Marked、DOMPurify 和 HTML 字符串输出，没有代码复制控件。现有同源
文档导航已经使用内部路由，旧缓存导致的浏览器新标签现象不增加特殊兼容逻辑。

## 2. 重构方案

- 外层宽度上限 2200px；1024px 起保留 340px 文档目录。目录名称和文章标题自动换行。
- 正文从左栏旁开始排布，宽度上限 960px；1280px 起在其右侧显示 240px 文章目录。
  小于 1280px 使用文章目录抽屉；小于 1024px 的文档目录沿用左侧抽屉。
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
  移动端选择章节后等待抽屉解除滚动锁再跳转；取消抽屉则按默认规则返回触发按钮。
- 链接保持标准 Markdown：文字链接显示文字，裸 URL 显示地址；HTTP、HTTPS、mailto 链接
  可点击并保留查询参数/片段，外链带 `noopener noreferrer`。危险协议经 DOMPurify 过滤。
  HTML 禁用、独立图片 URL 和 HTTPS 图片边界保持原契约。

## 3. 文件结构与职责

| 文件 | 调整和职责 |
| --- | --- |
| `web/src/features/docs/docs-layout.tsx` | 调整整体宽度、左侧 340px 栏和原有移动目录布局 |
| `web/src/features/docs/document-tree.tsx` | 长标题与文件夹名称完整换行、悬浮完整名称 |
| `web/src/features/docs/docs-article.tsx` | 获取文章、计算标题模型、组合正文及桌面/移动目录 |
| `web/src/features/docs/article-toc.tsx` | 新增章节导航列表、240px 粘性目录和右侧抽屉；复用 Sheet/Button |
| `web/src/features/docs/use-article-navigation.ts` | 新增章节滚动、hash 恢复、当前章节高亮和生命周期清理 |
| `web/src/features/docs/markdown.ts` | 图片规则保持；新增标题 ID 去重和规范化 |
| `web/src/features/docs/markdown-view.tsx` | 文档专用 Markdown：HTML 禁用、HTTPS 图片、标题 ID、代码复制 |
| `web/src/components/ui/markdown.tsx` | 兼容扩展：标题提取、可选标题 ID 及 fenced code 复制；默认调用不增加控件 |
| `web/src/features/docs/__tests__/markdown.test.tsx` | 标题/代码/图片/链接的渲染与剪贴板行为 |
| `web/src/features/docs/__tests__/reading.test.tsx` | 文章目录交互、深链、切换清理和匿名阅读回归 |
| `web/src/components/ui/__tests__/markdown.test.tsx` | 非文档调用默认行为及现有公式渲染回归 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json` | 新增文章目录文案 |
| `docs/custom-feature-preservation-checklist.md` | 在 P-37 补充公开阅读交互和安全契约 |
| `docs/documentation-reader-enhancements.md` | 本次设计、文件职责、阶段清单、自检与验收记录 |

复用检查：`CopyButton` 适用于 React 元素树，当前 HTML 字符串输出不能直接嵌入该组件。
因此采用计划中的事件委托和现有 `copyToClipboard` 工具，按钮复用 `buttonVariants`；
不创建第二套剪贴板降级逻辑，不新增依赖。目录抽屉遵循项目 shadcn-ui 技能的 Base UI
`render` 组合模式，保留可访问标题、关闭操作和焦点边界。

## 4. 开发清单

| 阶段 | 任务 | 状态 | 自检与修复 |
| --- | --- | --- | --- |
| 阅读布局 | 340px 左栏、960px 正文、240px 文章目录、标题换行 | 完成 | 七种屏宽通过；图片、表格和长代码均不撑宽页面 |
| 章节导航 | H2/H3、稳定锚点、深链、滚动高亮、移动抽屉 | 完成 | 嵌套标题、重复后缀、页尾短章节、图片加载位移和主动滚动均有验证 |
| 代码复制 | 原文复制、成功恢复、失败提示、生命周期清理 | 完成 | 保留缩进、空行及特殊字符；复制成功/失败和两秒恢复测试通过 |
| 链接与安全 | 标准链接、危险协议、原始 HTML 和图片边界 | 完成 | HTTP/HTTPS/mailto 可点击；危险协议被过滤；代码 URL 和图片规则通过 |
| 最终审查 | 文档/共享测试、类型、lint、格式、构建、浏览器布局 | 完成 | 47 项测试及最新构建产物验收通过，具体结果如下 |

## 5. 阶段自检记录

| 发现的问题 | 修复动作 | 验证结果 |
| --- | --- | --- |
| 大屏目录过窄、正文与目录之间留白过大 | 左栏 340px，正文紧邻左栏，右侧配置 240px 文章目录 | 1920px 下左栏 340px、正文 960px；长标题完整换行 |
| 嵌套标题或数学块使目录和渲染顺序可能不同，重复后缀可能撞 ID | 提取和渲染共用 Marked 扩展；使用已占用 ID 集合去重 | 中文、嵌套、行内格式、Setext 标题和重复后缀测试通过 |
| 当前章节或语言变化可能替换正文 DOM，使观察节点和焦点失效 | 稳定 HTML 渲染对象；翻译更新仅修改代码按钮 | 标题引用稳定性、复制状态和章节交互测试通过 |
| 文末最后一节较短，标题不能滚至页面顶部 | 到达文末时选中最后章节 | 自动测试和浏览器末节点击通过 |
| 深链首次定位后，图片/字体加载改变正文高度 | 加载完成后校正；主动滚动、触摸、点击或按键后停止校正 | 延迟图片、主动滚动中止和带中文片段刷新通过 |
| 关闭移动目录时焦点/滚动锁可能把页面拉回上方 | 选择后等待关闭完成并将焦点移至章节；Esc 仍回到按钮 | 手机浏览器位置检查和键盘焦点测试通过 |
| Marked 默认代码块带额外换行，不能直接作为复制结果 | 可复制代码块转义原始 token 文本，事件委托读取对应 code 文本 | 缩进、空行、特殊字符、失败提示和成功恢复通过 |

## 6. 最终审查结论

验收日期：2026-10-21。针对本次阅读页范围的功能与结构审查通过；没有数据库迁移或后端
接口变更，也没有增删依赖、改动路由配置或修改已有版权头。

| 检查 | 实际命令或方式 | 结果 |
| --- | --- | --- |
| 功能回归 | `bun run test -- src/features/docs/__tests__ src/components/ui/__tests__/markdown.test.tsx src/lib/__tests__/navigation-target.test.ts --maxWorkers=1` | 6 个文件、47 项测试全部通过 |
| 类型检查 | `bun run typecheck` | 通过 |
| 定向 lint | `bun x --no-install oxlint -c .oxlintrc.json src/features/docs src/components/ui/markdown.tsx src/components/ui/__tests__/markdown.test.tsx` | 退出码 0，无 error；4 条直接 ID 查询的风格提示保留，避免把片段当作 CSS 选择器 |
| 格式检查 | `node /tmp/new-api-docs-qa.KUeLRL/format.mjs --check` | 18 个前端变更文件通过，检查过程中保护并恢复原始版权头 |
| 生产构建 | `bun run build` | 最新产物构建通过 |
| 浏览器验收 | `node /tmp/new-api-docs-qa.KUeLRL/reader-qa.mjs` | 七种屏宽、目录换行、代码复制、粘性目录、深链刷新、手机抽屉、同标签导航、空状态和 404 均通过 |
| 请求与页面错误 | 浏览器全量请求和 `pageerror` 采集 | `/tools` 请求 0，页面脚本错误 0，文档数据仅访问 `/api/public/docs*` |
| 文案与差异 | 七种语言 JSON/新增文案校验、既有版权头逐文件对比、`git diff --check` | 全部通过 |

浏览器使用本机现有 Chromium，拦截请求并从 `web/dist` 提供最新构建文件，公开 API 和文章
内容使用独立 fixture。未启动本地开发/预览服务，未连接生产文档数据库。验收报告及截图为
临时产物：`/tmp/new-api-docs-qa.KUeLRL/report.json`、`desktop.png`、`mobile.png`；临时脚本
不加入项目依赖或 Git。数据库发布/下架规则沿用原实现，本次只验证对应公开读取的 UI 契约。

发布边界：本次只提交和推送代码，生产部署另行执行。
