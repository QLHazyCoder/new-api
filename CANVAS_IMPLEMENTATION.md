# 无限画布：实施与阶段审查

最新状态（2026-10-23 UTC 修复审查）：已实现中文分组传输、空错误响应、独立分组/模型选择、复用游乐场生图参数与请求构造、宿主主题、生成后保存重试/归档恢复、来源运行关联、失败历史、运行终态识别、项目按需加载和视频参考字段适配。Go 受影响检查、三数据库、画布完整类型检查/生产构建、4 个前端文件 16 项测试及隔离业务浏览器验收通过；进入提交、Actions 与蓝绿发布阶段。Tools 仍使用独立存储，没有将画布私有文件发布到公共图床；最终发布身份与验证边界以文末为准。

## 项目整体分析与设计修订

唯一方案：独立子应用内嵌、完全免 API Key。new-api 登录会话承担身份、授权、分组和计费。画布不接收登录令牌、模型 Key、PAT，不存在其他凭据模式。

实施基线：new-api `4da7a937b`，现有 main 干净；Infinite Canvas v0.19.0 `e856c878e0a34651bb828e28f0af20d71016a7d4`。早期设计中 merge-rc42 的工作区状态已失效。

审查修正：

1. 唯一公开画布 API 为 `/api/canvas/*`，内部复用标准 `/v1` 中继/插件处理器，不新增另一套 `/pg` 入口。认证完成后，由服务器设置规范路径和会话来源。
2. iframe 使用 opaque-origin sandbox，全部业务存储经宿主桥到主数据库/私有文件。localStorage 仅为内存中的小型界面偏好；localforage 适配器不访问沙箱 IndexedDB。
3. 项目、引用、运行与用户偏好分别存储。JSON 在 MySQL 使用 LONGTEXT，其余使用 TEXT，避免 TEXT 的 64KiB 上限截断文档；API 仍限制文档为 10MiB。
4. AI 调用状态与响应归档状态分开。完整响应写入私有文件；中断、超限或文件失败不能标成归档成功。生成记录可读取原响应，异步任务可按原 task_id 读取视频/产物；只重试媒体读取，不能重新提交生成或二次扣费。没有持久任务的请求在进程崩溃时不承诺恢复未收到的上游结果。
5. 所有项目和资产查询同时带 user_id；管理员个人画布不继承任意用户任务读取权限。保存冲突返回 409 并保留待保存内容。
6. 构建锁定上游 SHA，适配 patch 和新文件在本仓库维护。CI 在全新目录获取源代码并应用适配；开发机并列仓库不是发布依赖。
7. 分组模型选择项以分组名称和能力生成稳定 ID，目录顺序变化不能让已保存的选择落到其他分组；启动时项目和资产状态读取失败进入宿主重试，不能显示为新账户。
8. 引用检查覆盖节点 storageKey 和参考图列表；文档校验覆盖节点、连线、会话和视口，防止导入损坏下一次加载。生成入口移除内部 group 字段与请求幂等头，会话 Authorization、Cookie 在认证后移除。
9. 持久任务在统一提交完成位置关联运行记录，覆盖图像、Responses 与视频；清理仅在主节点启动。生成记录分页读取；取消接口只在确实找到本地观察时报告取消成功。
10. 子应用取消读取响应体时立即释放流记录和监听器，忽略迟到的结束消息，避免已关闭流被二次关闭；增加对应生产适配器回归测试。
11. 生产静态服务对 `/canvas-app/index.html` 的标准 `http.FileServer` 行为会重定向到 `./`，随后目录规范化与 Gin 的斜杠重定向形成循环。专用 index 处理直接返回入口 HTML；宿主明确使用 `/canvas-app/index.html`，不依赖目录重定向。
12. 消息通道建立与画布完成启动分开：`ready` 仅确认通道，`loaded` 必须在配置、项目与资产初始化完成且 React 首次提交后发送。30 秒启动期限覆盖整条链路；动态模块加载失败主动通知宿主，失败或超时关闭旧通道和请求，只在用户点击重试后重新加载。
13. 旧版入口与无斜杠目录的 301 曾携带 `Cache-Control: max-age=604800`。服务端直接返回 index 不能撤销浏览器已缓存的重定向；iframe 的入口增加当前构建版本 `v` 参数，绕过旧缓存，并继续沿用专用 index 的 `no-cache`。不能把新浏览器能打开当作旧浏览器升级已通过。

## 文件结构与职责

| 路径 | 职责 |
| --- | --- |
| `model/canvas.go` | 五表、文档验证、乐观锁、引用和归属 |
| `common/canvas.go` | 仅服务端使用的画布来源标识 |
| `service/canvas_catalog.go` | 本人分组、渠道和明确协议能力的模型目录 |
| `service/canvas_storage.go` | 原子文件存储、私有媒体与清理 |
| `controller/canvas.go` | 项目/资产/偏好、bootstrap、运行状态 |
| `controller/canvas_run.go` | 幂等屏障、响应归档、取消观察和任务关联 |
| `middleware/canvas.go` | 唯一会话认证、模块开关、组/模型与规范路径 |
| `middleware/canvas_static.go` | 沙箱静态资源的 CSP、CORS 与内容安全策略 |
| `middleware/frontend_static_test.go` | 画布 index 入口和既有前端静态压缩行为回归 |
| `router/canvas-router.go` | 白名单接口和既有中继/插件链 |
| `web/src/features/canvas/` | 沙箱容器、固定操作消息桥、加载与错误恢复 |
| `web/src/routes/_authenticated/canvas/` | 登录后的独立控制台导航页 |
| `integrations/infinite-canvas/` | 精确版本锁、上游适配、构建脚本与许可 |
| `integrations/infinite-canvas/overlay/` | 通信、存储、会话配置、传输、布局、提示词和禁扩展适配 |
| `integrations/infinite-canvas/browser-host.ts` / `build-browser-host.mjs` | 浏览器测试入口，使用生产消息桥，仅替换测试身份头 |
| `integrations/infinite-canvas/browser-check.mjs` | Chromium 项目/图片持久化、页面跳转、生成记录和回收站验收 |
| `integrations/infinite-canvas/browser-startup-check.mjs` | 完整控制台与画布生产构建的启动、模块失败、初始化挂起/失败和重试回归；仅使用本机临时 HTTP 服务及假 API，不访问生产账户或数据库 |
| `Dockerfile` | 镜像构建在打包控制台之前执行完整类型检查，并复制既有测试引用的跨目录 fixture；画布继续使用独立 builder 的类型检查与构建 |
| `web/package.json` / `web/bun.lock` | 将现有测试已使用的 `happy-dom` 固定为本机已有的 20.11.1 开发依赖，保证冻结安装可重复 |
| `web/src/features/docs/__tests__/reading.test.tsx` | 使用等价的复制后排序断言，兼容现有 ES2022 类型库，保持请求列表不被修改 |
| `model/canvas_test.go` / `controller/canvas_test.go` / `service/canvas_test.go` | 三数据库、数据接口、归档、目录与隔离 HTTP harness |
| 既有认证、限流、资金、导航测试文件 | 增加画布回归断言，保留原模型 Token 和 Playground 行为 |

## 开发清单与阶段自检

| 阶段 | 状态 | 验收与审查记录 |
| --- | --- | --- |
| 0 设计与基线 | 已审查 | 当前 main 干净，修正六处契约，单一免 Key 方案 |
| 1 数据与存储 | 已审查 | SQLite 3.50.4、MySQL 8.2.0、PostgreSQL 18.4；重复迁移、大文档、CAS、owner check、软删除/恢复与引用保护通过 |
| 2 会话与中继 | 已审查 | 真实会话认证、PAT/匿名拒绝、会话撤销、服务端开关、规范 URL.Path/RequestURI、插件会话限流通过；钱包/订阅混合结算且不创建 Token 已验证 |
| 3 画布适配 | 已审查 | Chromium 使用 opaque sandbox 和生产桥；项目重载、图片生成/归档/重载、四页跳转、生成记录和回收站还原通过，无控制台错误 |
| 4 导航与构建 | 已审查 | Chat 分组 Playground 后“无限画布”；管理员/用户模块配置；上游 SHA 全新网络检出、类型检查、双前端生产构建与 Bun 1.4.0 Docker 画布 builder 通过 |
| 5 完整审查 | 已审查，有明确验证边界 | 全量 Go、vet、独立子模块、Go 二进制、受影响前端 31 项、流取消与 UTF-8 检查通过；全量前端受既有测试卡点和服务器资源约束未完成，见下文 |

容量采用已设计值：项目 10MiB，图像输入 20MiB、归档 50MiB，其余媒体复用现有下载上限；超限拒绝写入。回收站 30 天，运行元数据 180 天。运行与文件清理均不能删除仍被引用的资产。

桥接最多并发 16 个请求，逐个消息分片确认消费后再读下一个分片，避免 SSE/视频堆积。完整启动 30 秒未完成显示重试；宿主调用既有会话刷新，不向子应用传递凭据。媒体上传与响应归档分别受服务端大小限制约束，错误不能触发生成重试。

## 数据与接口的最终对应

| 数据表 | 实际职责与一致性 |
| --- | --- |
| canvas_projects | 文档、标题、schema_version、revision、用户归属与软删除；CAS 和媒体引用在同一事务更新 |
| canvas_assets | 用户媒体、MIME、大小、SHA256、私有 storage_key、来源运行；文件原子写入，DB 失败清理新文件 |
| canvas_project_asset_refs | user_id + scope + project_id + asset_id；scope 区分项目与偏好/生成历史引用，避免 ID 碰撞或清理误删 |
| canvas_generation_runs | 用户/客户端幂等键、规范 operation、model/group、状态、task_id、归档路径与状态；费用仍以现有账单为准 |
| canvas_user_settings | 每个用户的 namespace/key 文档与 revision；保存时检查其所有媒体引用并在事务中维护引用 |

运行 API、数据 API 和固定消息映射见 router/canvas-router.go、web/src/features/canvas/bridge.ts；子应用只使用桥，不发任意 URL/自带认证头。/api/canvas/state/:namespace/:entry_key 仅开放 app_state、图片/视频历史与提示词缓存四个 namespace。

模型目录直接按用户可用分组查询启用渠道能力，不从价格推断、不合并其他分组能力。文字走 Responses，图像和视频走现有主机协议；语音要求模型元数据明确声明 /v1/audio/speech。缺少能力返回 422，管理员需正确配置元数据或渠道，不猜测模型名称。

构建命令：在仓库根执行 sh integrations/infinite-canvas/build.sh，它使用精确 SHA 全新检出、冻结依赖锁、类型检查、Vite 生产构建，复制到 web/dist/canvas-app/。new-api 控制台需先构建，画布后构建；Docker 使用独立 builder，再把两套产物合并到 Go embed。MIT 许可随产物保留。首次集成阶段只推代码；本轮已获用户授权，在 Actions 成功、镜像身份与候选业务验收一致后执行蓝绿发布，结果见文末。

## 可重现验证

- go vet -p 2 ./...、go test -p 2 ./...、go build -p 2 .：根模块通过；最终修订后受影响六个完整包再次通过，最终二进制构建通过。
- GOWORK=off go test ./...：分别在 relaykit、tokenkit 执行，均通过。
- TEST_CANVAS_MYSQL_DSN=... TEST_CANVAS_POSTGRES_DSN=... go test ./model -run '^TestCanvas' -count=1 -v：隔离的真实三数据库，创建、迁移两次、保留既有用户、中文大文档、CAS、归属、引用、清理验证；测试库自动删除。
- bun run typecheck 与画布/侧栏/既有 Playground SSE、图片能力和请求构造六个测试文件：最终 31 项通过，包含流式 UTF-8、取消后迟到消息、未知操作拒绝、路径穿越、multipart、无凭据消息、账号切换销毁、沙箱权限、握手失败重试、导航与模块开关。
- sh integrations/infinite-canvas/build.sh：已在无并列源码依赖的全新网络检出中通过；Docker canvas-builder 亦在 Bun 1.4.0 中完成冻结安装、typecheck 和 Vite 生产构建。
- 项目 shadcn-ui 技能用于复用 Main、LoadingState、ErrorState。iframe/消息桥是既有组件无法承担的能力，保持在 canvas feature，不新增通用加载/错误 UI。
- CANVAS_BROWSER_TEST=1 go test ./controller -run '^TestCanvasBrowserHarness$'：仅启动临时测试 DB 的 HTTP harness；不启动开发服务，不读取生产用户/渠道。先用 build-browser-host.mjs 打包生产桥测试入口，再以 browser-check.mjs 验证沙箱、项目创建/重载、中文 emoji 图片生成/归档/重载、页面切换、生成记录和回收站还原，无控制台错误。测试使用 1px PNG 假上游，不调用付费模型；文字/视频/语音真实渠道输出尚未实测。
- 修改文件均以严格 UTF-8 解码；七语言九个新增键齐全，无替换字符、NUL 或冲突标记；git diff --check 通过。浏览器已核对中文和 emoji。新增 Web 文件保留现有许可证头，上游 MIT 随产物分发。

扩大前端回归的已知边界：不排除任何文件的全量 Vitest 在既有 `channel-configuration.test.tsx` 中长时间无结果，调试栈停在 jsdom 的 CSS 选择器匹配。未修改的 `4da7a937b` 独立检出、复用同一安装依赖运行该文件，亦在 60 秒受控超时内未完成。未修改渠道代码、测试或公共测试配置；该项不能记为通过。排除该文件后扩大回归已产生通过结果，收到服务器占用约束后主动终止，未跑完全套，不能声称全量前端通过。最终受影响六文件 31 项独立完成并通过。

资源收尾：浏览器 harness、临时基线 worktree、隔离 MySQL/PostgreSQL 容器和扩大回归进程已关闭或清理；没有重启生产容器、修改生产数据库或启动开发预览服务。后续必要校验限制并发并使用低调度优先级。

## 2026-10-22 持续加载问题专项复核

### 已取得的证据与边界

1. 生产 `main-0c3874f`：`/canvas-app/index.html`、入口模块、启动模块和 CSS 均可返回正确内容。使用线上完整控制台与真实静态资源，在隔离的浏览器 API fixture 下分别访问 `api.qlhazycoder.top/canvas` 和 `coderapi.vip/canvas`，均显示画布库，无页面脚本异常。这只验证该浏览器和 fixture 初始化，不代表用户浏览器或生产用户数据已通过。
2. 在同一线上完整页面中仅阻断 `start-*.js`，稳定出现动态模块加载失败；超过 30 秒后 iframe 仍为空白、`aria-busy=false`、没有错误或重试入口。该复现不调用生产登录、画布写接口或任何付费模型。
3. 源码对应两处缺陷：宿主收到握手 `ready` 即清除启动期限；入口 `ready.then(import)` 没有拒绝处理，无法把模块加载失败反馈给宿主。修复使用独立的 `loaded` 通知和模块加载异常通知，沿用既有错误和重试组件。
4. 首轮现场保留的 `/tmp/canvas-live-headers.txt` 明确记录目录 301 循环，其中无斜杠目录返回一周缓存；`middleware/Cache` 与首次上线源码一致。浏览器脚本不使用 `page.route` 的缓存用例中，先播种原 301，再切回当前已修复的服务器；刷新后仍进入 `chrome-error://chromewebdata/`，未带版本的入口请求数在升级前后均为 2，证明浏览器没有重新请求入口。这是已复现的旧客户端升级缺陷。
5. 修复将实际 iframe 入口改为 `/canvas-app/index.html?v=<构建标记>#/canvas`，保留原安全策略及完全免 Key 方案。回归必须保留旧缓存，验证升级后实际画布库可见，不能通过测试拦截器禁用 HTTP 缓存来回避问题。用户截图图标本身不作为扩展故障证据；尚未获取该用户浏览器的抓包，不据此断言具体扩展或浏览器错误页样式。
6. 镜像构建新增的完整类型检查发现：Docker 原上下文没有携带两个跨目录测试 fixture；已有测试未声明 `happy-dom`；文档测试使用了超出 ES2022 类型库的 `toSorted`。逐项补齐文件/开发依赖并改用等价排序断言，保留完整类型检查，不排除测试文件。

### 阶段自检记录

| 阶段 | 状态 | 检查与发现 |
| --- | --- | --- |
| 原因验证 | 已复现两个故障链 | 模块阻断后的永久空白；旧 301 缓存造成服务器修复后依然进入浏览器错误页；源码、旧响应头与同浏览器升级结果闭环 |
| 启动链和缓存修复 | 已通过 | 三项回归断言先失败：握手过早标就绪、后续超时失效、入口无版本；修复后宿主/桥及文档相关 3 文件共 23 项通过，相关文件 lint 无错误；保留 opaque sandbox、既有登录会话及单一免 Key 设计 |
| 完整构建和浏览器验收 | 已通过 | [Actions 39798035698](https://github.com/QLHazyCoder/new-api/actions/runs/39798035698) 对精确提交 `0858fac13bcd8f8845141496030edce2c5ba650a` 完成 amd64/arm64 冻结安装、完整控制台/子应用类型检查、双前端构建和镜像发布。浏览器脚本读取候选容器实际资源，5 条流程全部通过；切流后对公网资源再次全部通过，均无页面脚本异常 |
| 资源控制 | 已执行 | 浏览器/测试最多 1 CPU、1GB；本机全量类型检查上限 1 CPU、1.5GB、无交换，三次受限终止（已尝试 GC 目标和单线程）；生产构建编译阶段结束但进程未成功退出，不能记为通过。不提高本机硬上限，后续完整类型检查与构建以对应提交的镜像 Actions 日志为验收门槛 |

本次文件没有迁移、删除或数据库结构变更。新增文件只有上述专项浏览器回归脚本；宿主入口、桥及子应用入口/启动文件职责保持不变。只提交本轮文件，保留其他正在进行的分组相关修改。

专项脚本的运行方式（仓库根目录，所需 Playwright 与 Chromium 安装在隔离目录）：

```bash
CANVAS_STATIC_ORIGIN=http://<候选容器IP>:3000 \
CANVAS_PLAYWRIGHT_MODULE=/绝对路径/playwright-core/index.mjs \
CANVAS_CHROMIUM_PATH=/绝对路径/headless_shell \
node integrations/infinite-canvas/browser-startup-check.mjs
```

实际结果：`cached-redirect`、`success`、`module-failure`、`bootstrap-stall`、`bootstrap-failure` 全部 PASS；后 3 项均验证错误状态出现后点击重试可以重新显示画布库。只有缓存用例不安装路由拦截器，以保留浏览器真实 HTTP 缓存。候选容器及公网的 API 均不参与这些测试，避免改动生产用户数据。

## 安全审查依据

采用 [OWASP ASVS 5.0.0](https://owasp.org/projects/asvs)、[认证](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[会话](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)、[CSRF](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html) 对会话撤销、服务端授权、Bearer 请求、秘密排除与消息来源做具体验证，不声称完整 ASVS 认证。

## 最终审查结论

首次集成阶段已完成单一免 API Key 实现、文件职责、数据库/接口和阶段检查；后续线上截图证明原验收未覆盖所有启动失败及旧客户端升级路径。专项复核已复现并修复旧 301 缓存未绕过、消息通道握手过早结束启动保护、动态模块失败未回报三处缺陷。完整构建、候选容器及公网资源的五条浏览器回归通过，`main-0858fac` 已于 2026-10-22 14:57:33 UTC 切流到蓝槽；超过 5 分钟的多轮公开版本与健康检查通过，旧绿槽于 15:05:13 UTC 优雅退出，exit=0。镜像 digest、回滚文件及中继日志的观测边界已同步到上级仓库 `deploy/项目更新标准流程.md`。

验收范围须保留三项事实：全量前端回归未完成；未对真实付费渠道执行文字、视频、语音生成；尚未拿到用户浏览器实际 DOM/Console，截图彩色加载状态的来源未证实。原 Chromium 图片验收使用假上游，不能当作所有真实渠道验证。本次发布已校验精确提交和 OCI revision，先验收备用容器再切流；不声称对所有真实上游请求完成验收。

## 2026-10-22 生成与产品集成全面审查

### 一、项目整体分析

本节回应最新两次反馈：模型列表与游乐场不一致、冗长分组说明、主题跟随、画布改造范围、Tools 图床/媒体床接入，以及图片生成失败。当前任务为全面分析；本节记录已证实的问题和待实施方案，不把方案当作已经上线的结果。

#### 运行与源码基线

- 线上活动蓝槽仍为 `0858fac13bcd8f8845141496030edce2c5ba650a` / `main-0858fac`，镜像与上述部署记录一致、Docker healthy。
- 本地 new-api 已前进到 `0bab0308bc31ad912f07b27e934837b151a5eb66`，该提交为其他分组设置工作；本次所检查的 canvas 控制器、中间件、消息桥和 overlay 与生产提交一致。
- 构建使用 Infinite Canvas `v0.19.0` 的精确提交 `e856c878e0a34651bb828e28f0af20d71016a7d4`，不能把并列仓库最新 main 自动当作发布版本。
- 确实已经改造无限画布：new-api 在构建时叠加 overlay、别名与补丁，替换认证/传输/存储/布局等边界，禁用独立 Key、Agent、用户脚本、WebDAV 等入口。画布节点编辑器主体仍来自上游；并列上游仓库没有本轮修改。
- 初始集成提交影响 new-api 的 70 个文件、增加 4509 行、删除 14 行，包含 Go API/数据模型、测试、文档、翻译及构建集成，不能将这些数字误认为全部都是对上游画布编辑器的重写。
- Tools 线上 revision 为 `789b6b1b60ac07dd25e90d314c5360b5dff0f988`。本次检查的图床、媒体床和上传认证文件与本地 Tools HEAD 无差异；其他 Tools 工作区修改不在本次范围。

#### 已复现的图片失败链

```text
选择 g-Codex%E7%89%B9%E6%83%A0-image::gpt-image-2
    → 配置适配恢复真实分组 Codex特惠
    → 把中文直接放进 X-Canvas-Group
    → AxiosHeaders 过滤不允许的头字符，值变成 Codex
    → 宿主把 Codex 传给 /api/canvas/v1/images/generations
    → CanvasRelay 查询可用分组，Codex 不存在，返回空响应体的 403
    → 自定义 Axios adapter 在检查 response.ok 之前执行 response.json()
    → 界面显示 ReadableStream close / Unexpected end of JSON input
```

证据相互对应：

1. 生产 2026-10-22 18:34:22 UTC 有 10 次画布生图 POST 返回 403，18:34:31 UTC 又有 1 次返回 403；日志本地时区显示为次日 02:34。示例请求 ID：`202610221834222449385208268d9d6zeCY0E1D`、`202610221834316043538408268d9d6e0ubegpS`。
2. 对应两条保存的界面历史均选中上述完整模型 ID，一次 10 张、一次 1 张，成功数均为 0。该账户的 `canvas_generation_runs` 没有记录；CanvasRelay 位于创建运行和发起中继之前，这些拒绝没有进入图片生成上游。不能据此评价其他独立请求或账户总账。
3. 当前可用分组中存在 `Codex特惠`，不存在 `Codex`；相应模型能力存在，不能把这次失败解释成图片模型本身不可用。
4. 使用线上发布的完整控制台、画布 JS 和生产消息桥，所有 API 由隔离 fixture 响应。选择同名中文分组后，截获的真实浏览器请求头为 `X-Canvas-Group: Codex`；返回空 403，复现截图完全相同的异常文本。
5. 用该画布安装依赖中的 AxiosHeaders 单独验证：`Codex特惠 → Codex`、`GPT-Pro-正价 → GPT-Pro-`、`图片模型-原生4K → -4K`，而 `image`、`default` 保持原值。
6. 以纯 ASCII `default` 为对照，返回 1px PNG 的假生成响应，画布成功显示结果；这证明默认 ASCII fixture 会漏掉中文分组故障，不是对真实付费渠道的验收。
7. 原生 `Headers` 也不能直接接收这种中文值，会抛出 `String contains non ISO-8859-1 code point`。只绕过 Axios 不能解决文字等 fetch 调用路径的问题。

因此：403 是分组传输损坏的直接结果；JSON/流关闭异常是错误读取顺序造成的二次错误。不能通过取消分组权限判断、静默选择另一个分组或注入 Key 修复。

#### 已确认问题清单

| ID / 优先级 | 事实与证据级别 | 用户影响 |
| --- | --- | --- |
| C01 / P0 | 中文分组经原始 HTTP 头传递，被 Axios 过滤；线上资源浏览器复现 | 多个中文分组生成被拒绝；原生 fetch 路径也不支持直接中文头 |
| C02 / P0 | `transport.ts` 先 `response.json()` 后 `response.ok`；空 403 已复现 | 真实 HTTP 错误被掩盖成 JSON/ReadableStream 错误，无法定位请求 |
| C03 / P1 | 将假生成响应设为成功、资源上传设为失败，界面显示“生成失败”；点击重试后生成 POST 从 1 次变为 2 次 | 存储失败可能触发重复付费生成；这是独立于本次 403 的另一条可复现缺陷 |
| C04 / P1 | `config.ts` 用 `bootstrap.groups[group]`（分组描述）作为 channel.name；上游标签拼接 `模型（channel.name）` | 同一模型重复很多次、说明过长、真实分组名不清楚、列表截断 |
| C05 / P1 | CanvasModelOption 只有 name/group/capability；游乐场另有 image capabilities | 未复用尺寸、画质、编辑能力、输出格式等约束；通用画布控件与实际渠道能力可能不一致 |
| C06 / P1 | 线上资源 fixture 中外层 `font-inter light`，子页面 `dark`；overlay 持久化独立主题并显示切换按钮 | 外浅内深、两处设置冲突，不会跟随宿主最终解析的主题 |
| C07 / 功能缺口 | 画布只写私有 canvas API/文件，没有调用 Tools 存储；两仓源码和运行版本已核对 | 画布图片、音视频不会自动出现在工具中心图床/媒体床中 |
| C08 / P1 | 上游视频发送 `image[]` 或 `first_frame/last_frame`；直接调用现有插件解码器已验证拒绝 | Sora 要求 `input_reference`；豆包要求 metadata.content URL 引用，通用参考图表单无法直接兼容 |
| C09 / P2 | 上游模型图标使用 `/icons/openai.svg`；线上返回 text/html，`/canvas-app/icons/openai.svg` 才返回 SVG | 截图模型名前的图标破损属于真实路径问题 |
| C10 / P1 | adapter 没有处理 Axios 的 `config.timeout`；消息桥只有调用方 signal | 上游原本设置的 10 分钟超时不会被自定义 adapter 自动执行，服务端/网络挂起时没有完整客户端期限 |
| C11 / P1 | 界面历史与服务器运行分属 settings/runs；当前失败历史没有保留 error/status/request ID | 左侧显示失败，右上服务器运行却没有条目，刷新后失败原因丢失；宿主只传递 Content-Type 和 Run-ID，丢失服务端请求 ID |
| C12 / 容量风险 | 启动时逐个读取所有项目完整文档，完成后才渲染，宿主有 30 秒启动期限 | 项目数/文档较大时启动时间和内存随所有项目增长；这是源码确认的容量风险，本次未对生产做压力验证 |
| C13 / 状态风险 | CanvasGeneration 主要以 HTTP 状态和上下文判断成功，runs 查询对关联 Task 另做投影 | 对 HTTP 200 内承载的业务错误、SSE 终态不能仅据 200 标记生成成功；尚未以真实文字/视频上游完成验证 |

### 二、重构与修复方案

#### 1. 分组传输与错误处理：先修故障，不改变计费选择

- 保留 `(group, model, capability)` 的真实身份和服务器权限检查。同一模型不同分组可能有不同可用性、优先级和计费，不能只按模型名去重后默认采用字典序第一个分组。
- 确定统一的头编码契约：所有画布请求发送前对原始分组执行 `encodeURIComponent`，服务器使用对应的严格、单次解码后才检查权限。ASCII 名称自然保持原样；非法编码返回带稳定 code 的 400，不猜测、不尝试第二种分组解释。消息桥中的字符串可以保留 Unicode，但进入任何 HTTP Header 之前必须按此契约编码。
- 生成 POST 的 JSON 和 multipart 均遵循同一编码规则；编码后的路由字段不作为上游生成参数转发。视频/产物读取使用 task_id 和服务器验证的 user_id，按已存任务取得原渠道，不按当前选择的分组重新调度或计费。
- 所有拒绝返回结构化错误，先保留 status/code/request ID，再按 Content-Type 读取错误体。空错误体显示明确的 HTTP 错误，不以 JSON 解析失败替换原始错误。
- `X-Oneapi-Request-Id`、`X-Canvas-Run-ID` 在桥接后作为非秘密排障字段保留。恢复 stream/abort/end 处理的错误归属，不能只捕获异常后伪造成功空结果。

这一阶段不需要数据库迁移，不修改模型 Key、渠道价格、用户权限或自动分组策略。

#### 2. 模型与生图参数：与游乐场共用业务契约

- 页面保留“无限画布”导航位置。画布内模型配置采用独立的“分组”“模型”选择，模型行只显示模型名，不再把长分组说明拼进模型标签。分组说明按本次反馈从该交互中移除；真实分组选择仍然保留。
- 复用游乐场 `GetUserImageModelGroups` / image capabilities 的业务解析，能力不能从价格、模型名或其他分组推断。调整 bootstrap 增加按组的图片能力数据，保留文字/视频/语音目录和服务端校验。
- 复用已有 ModelGroupSelector 的交互语义及现有图片参数的归一化规则；受 iframe 隔离约束，需要在画布组件边界增加适配，不能声称可以直接跨 React 树插入宿主组件。
- 输入项只展示当前能力支持的尺寸、分辨率、质量、编辑、输出格式等选项。暂不可用的能力明确不可选；没有能力数据时不显示一套猜测的通用配置。
- 保存的 `g-<已编码分组>-<能力>::<模型>` ID 必须解析回原分组/模型，保持用户选择，不因新 UI 自动落到其他分组。旧画布、生成历史及偏好都在迁移检查范围，不能只测试新账户。
- 上游 10 张工作台当前并发发出 10 次 `n=1`，与游乐场的后台任务调度不同。将“总张数”和“单次 n / 并发数”分开，按受限队列执行，不能靠一条含糊的“批量”文案掩盖实际请求数量。

#### 3. 主题：宿主为唯一来源

- 来源是 new-api `ThemeProvider.resolvedTheme`，包括用户设置及 system 模式已经解析后的 light/dark。
- 通过现有可信 MessageChannel 在初始握手及运行中发送主题；子应用接收后更新自己的主题 store、CSS class 与 Ant Design theme。
- 移除右上角和画布内部的独立浅色/深色操作入口；不再恢复或写入 `canvas:preferences.theme`，旧值可被忽略，其他偏好保留。
- 主题变化不重新加载 iframe，不能打断正在运行的生成、编辑内容或保存。
- 先覆盖明确要求的明暗同步。new-api 的配色预设、字体、圆角和画布各项目的背景不是同一个配置：要同步主色等设计变量，必须明确映射 CSS/Ant Design token；不能承诺只传 light/dark 就等于完整系统皮肤继承。

这一阶段主要是父/子页面消息和 UI 边界的小范围改动，不需要重写画布引擎或迁移业务表。

#### 4. 生成成功、归档失败和重试分开

- 将 generation 状态与资产保存状态独立表示。生成已经成功但存储失败时保留 Blob/归档 Run-ID，提示“图片已生成，保存失败”，提供“重试保存”。
- 重试保存只允许重读原响应或重新上传原图片，不能再次调用 images/generations。
- 网络中断且生成结果不确定时先按同一 request/run 身份查状态。不能因为前端显示失败就自动创建新的计费请求。
- 手动“重新生成”是另一项明确动作；服务端幂等键、请求摘要和原 run 关联继续起作用。
- 普通 b64 图片上传目前没有携带来源 run，`source_run_id` 关联不完整。应补齐来源验证和关联，让恢复路径可以定位原图；已有数据库字段可以复用。
- 失败历史保存安全的 error_code/http_status/request_id/run_id 与归档状态，不能保存 Token、Key 或完整内部异常。

#### 5. Tools 图床／媒体床：尚未接入，属于跨服务工作

当前数据链是：画布 → new-api canvas API → 主数据库的 canvas 表和 `/data/canvas-assets`；Tools 则使用独立 SQLite 索引和自己的文件目录。

现有 Tools 公共上传要求 New API 模型 Key，管理端上传固定归属管理员。两者都不能直接拿来充当免 Key 画布用户上传：前者违背已定认证模式，后者会丢失真实用户归属。

正确的接入边界：new-api 继续完成登录会话与用户授权，宿主经受限服务间接口委托 Tools 存储；Tools 必须核验委托身份、真实 owner、大小/MIME、请求幂等和来源。子应用不获得模型 Key 或管理员 Tools Token，也不让浏览器自行声称 owner。

必须同时设计以下产品和数据规则：

- canvas 项目私有文件与可公开图床 URL 的可见性不能混用。使用图床公开链接需要明确发布语义；不能把用户上传的私有参考图自动变成任何人可读取的链接。
- Tools 默认上传有效期为 7 天，canvas 引用保护和长期项目并非这一生命周期。需要长久保留/引用租约或明确独立的公开副本策略，不能直接用会过期的 URL 替换项目的私有引用。
- Tools 管理列表需要识别 `source=canvas`、owner、远端关联 ID、可见性、到期和引用状态。删除、过期、软删除/恢复和两个服务之间的失败补偿必须闭环。
- 若统一存储，new-api 保留资产逻辑 ID/归属/项目引用，Tools 持有实际存储对象；两套数据库之间不能假设有一笔原子事务。上传、绑定、重试、清理分别要求幂等和可核对状态。
- 不迁移正在使用的文件到一半就删除原件。需要逐项核验旧文件/哈希、重载、引用和删除保护后才能切换存储所有权。

是否把 Tools 作为统一的私有存储管理后端，是新增跨服务产品范围；本次事实核对已完成，没有擅自将私有资源公开或切换存储。

#### 6. 视频、语音、文字与静态资源

- 视频以 new-api 已发布 host protocol / 插件请求契约适配字段，不能把所有模型强制套入上游画布通用 form。Sora 单张 input_reference、豆包 metadata.content 引用分别有确定契约；前后帧/参考视频/参考音频只在协议明确支持时展示。
- 文本流、图像 JSON、音频 Blob、视频任务/内容分别验证格式与错误，不能以“视频菜单可以打开”代替生成/轮询/下载的闭环。
- 适配器兑现 `config.timeout`、调用方 signal、账号切换和关闭页面的终止语义；客户端期限不等于取消已在服务端完成的计费任务。
- 将图标路径改成构建 base 下的 `/canvas-app/icons/...`，并检查其他绝对资源路径。不要通过把所有 404 改成 200 HTML 掩盖静态文件缺失。
- 项目目录应按需加载，避免渲染前读取所有文档；明确处理单个文档失败，防止“加载不全”被错误保存成完整项目集合。

### 三、文件结构与数据库职责

本次分析没有迁移、删除或重组生产文件。后续按下列边界修改，主体编辑器保留：

| 文件／目录 | 当前职责及后续调整范围 |
| --- | --- |
| `integrations/infinite-canvas/source.lock` | 保留上游精确版本；升级必须另行验证补丁边界 |
| `integrations/infinite-canvas/prepare.mjs` | 在一次性构建目录应用受控适配；补标签、图标和必要组件边界，禁止静默 patch 失败 |
| `integrations/infinite-canvas/overlay/config.ts` | 账户模型配置、旧偏好解析、分组编码、能力数据；去掉独立主题持久化 |
| `integrations/infinite-canvas/overlay/transport.ts` | Axios/fetch 传输、HTTP 错误、期限与返回格式；不再从被过滤的中文头恢复路由身份 |
| `integrations/infinite-canvas/overlay/bridge.ts` | 字节流与主题事件；维护 end/error/cancel 状态和请求关联 |
| `integrations/infinite-canvas/overlay/start.tsx` | 首次主题应用、初始化和渲染；不加载私有凭据 |
| `integrations/infinite-canvas/overlay/layout.tsx` | 移除独立主题按钮，明确历史/归档界面语义 |
| `integrations/infinite-canvas/overlay/storage.ts` / `localforage-storage.ts` | 项目与资产持久化；处理重试保存、来源关联、待保存内容和按需加载 |
| `web/src/features/canvas/index.tsx` / `bridge.ts` | 会话宿主、主题下发、固定 API 操作及响应排障信息；保留沙箱隔离 |
| `middleware/canvas.go` | 分组单次解码、逐请求权限与模型能力检查、结构化拒绝；不改为自动挑选分组 |
| `service/canvas_catalog.go` / `controller/canvas.go` | 复用既有能力数据并提供 bootstrap；保留组/模型身份及资产所有权 |
| `web/src/components/model-group-selector.tsx` / `features/playground` / `service/image_capability.go` | 已有选择与图片能力事实来源；画布对齐它们，不另造参数规则 |
| `controller/canvas_run.go` | 生成身份、归档、请求幂等和任务关联；成功/失败语义不能仅靠 HTTP 200 |
| `new_api_tools/backend/internal/handler/media_images.go` / `media_assets.go` / `public_upload_auth.go` | 当前公共/管理上传接口；未来接入需专门的会话委托入口，不能混用管理员上传 |
| `new_api_tools/backend/internal/service/media` / `mediaasset` | 存储索引、公开 URL、有效期与清理；统一存储时需要来源、可见性及引用保护 |
| 现有 canvas 单元测试及 `browser-check.mjs` / `browser-startup-check.mjs` | 补真正的生成、中文分组、保存失败/重试和媒体契约回归；不能只跑启动 |

| 数据表／存储 | 实际所有权和当前缺口 |
| --- | --- |
| `new_api.canvas_projects` | 用户项目文档与版本；应保持旧项目/模型选择可恢复 |
| `new_api.canvas_assets` + `/data/canvas-assets` | 私有媒体；已有 SourceRunID，但普通 b64 上传未完整关联；没有 Tools 对象映射 |
| `new_api.canvas_project_asset_refs` | 项目/偏好/历史引用保护；Tools 清理不会读取这张表 |
| `new_api.canvas_generation_runs` | 进入生成链后的幂等/响应归档；权限层拒绝不会创建 run，不能据此说用户未尝试过生成 |
| `new_api.canvas_user_settings` | 偏好及界面图片/视频历史；已保存历史缺少失败原因和服务端关联 ID，主题应停止独立保存 |
| Tools `image-upload.db.media_objects` | 图床索引、真实 owner、公开 URL、有效期；与 canvas_assets 无自动映射 |
| Tools `media-assets.db.media_assets` | 视频/音频媒体床索引、真实 owner、公开 URL、有效期；与 canvas_assets 无自动映射 |

分组编码、错误处理、模型标签、能力参数、主题、现有运行/历史关联的第一轮修正可使用现有表与 JSON 文档，原则上不需要新增数据库表。统一 Tools 存储的关联、可见性与引用保护另需明确字段和迁移测试，不能声称只需修改前端上传 URL。

### 四、开发清单（以 2026-10-23 实施复核更新）

| 阶段 | 任务 | 当前状态 | 放行标准 |
| --- | --- | --- | --- |
| A 故障修复 | C01/C02 编码、权限保留、HTTP 错误与 request ID | 已实现并通过单测/浏览器 fixture | 中文/ASCII/含百分号组单次解码；非法 UTF-8、越权和空错误体均可读；分组选择与计费路由未改变 |
| B 防重复生成 | C03/C11/C13 保存独立、归档恢复、历史关联与终态 | 已通过控制器与浏览器回归 | 工作台与节点保存失败记录原 Run-ID；原历史写入完成后才能重试保存；重试只读取/保存原结果；HTTP 200 JSON/SSE 业务错误不记为成功 |
| C 控制台一致性 | C04/C05/C06/C09 模型/参数、主题和图标 | 已通过完整构建与浏览器回归 | 分组与模型分开，无长描述；复用游乐场实际 normalizer/payload builder；明暗实时同步且不重载 iframe |
| D 多模态契约 | C08/C10 参考媒体字段、内容读取、期限与取消 | 已通过编译和 Sora 字段回归；保留真实渠道边界 | Sora 使用 input_reference；豆包使用 metadata.content；未知混合协议与尚不支持的私有参考音视频在提交前拒绝；期限覆盖响应体 |
| E 容量与恢复 | C12 项目按需加载、部分读取失败与写回保护 | 已实施；摘要查询三数据库通过 | 列表不读取完整文档；打开/导出时才加载；未加载文档的重命名先读原文档保留节点；失败不写空文档 |
| F Tools 接入 | 会话委托、可见性、归属、存储关联、清理 | 当前未接入，范围已梳理 | 双服务所有权和失败补偿完整，过期/删除不破坏被引用的私有资产 |
| G 总审与发布 | 文档/字符/回归、精确镜像、备用容器及真实用户验收 | 代码阶段已通过，提交/Actions/蓝绿待执行 | 修复提交与 OCI 一致，候选浏览器业务链通过再切流，不把假生成当真实渠道验证 |

### 五、阶段自检记录与验证边界

| 检查 | 实际结果 |
| --- | --- |
| 生产版本、日志和只读数据库核对 | 完成；匹配中文模型选择、两批 10+1 请求及 403；没有执行生产 DB 写入 |
| 线上完整 JS + 中文分组 + 空 403 | 复现请求头变为 Codex，及截图相同的 JSON/ReadableStream 错误 |
| 同一线上资源 + ASCII 组 + 成功假图片 | 显示 1px 图片，说明原 ASCII 成功测试覆盖不足 |
| 同一线上资源 + 生成成功 + 存储失败 + 手动重试 | 生图 POST 从 1 次变为 2 次，确认保存失败会触发再次生成 |
| Sora/豆包现有协议解码器 + 画布参考图字段 | `image[]`、`first_frame` 均被相应解码器拒绝；无上游网络调用 |
| 主题与图标 | 浏览器明确外层 light / 子页 dark；错误图标 URL 为 HTML，正确 base 路径为 SVG |
| Tools 实际接口与存储 | 已核对部署源码、认证、owner 和索引；canvas 未对接 |
| 资源 | 浏览器和纯解码验证限制 1 CPU、1GB 或 512MB；一次只运行一个浏览器，无生产压力测试 |
| 代码状态 | 中途未验证的自动选组等实现草案已撤回，恢复已提交的业务源码；只保留本节分析文档和 `/tmp` 隔离诊断脚本，没有发布新镜像 |

诊断脚本：`/tmp/canvas-audit-generation.mjs`。它把所有 `/api/*` 请求响应在浏览器中隔离，只读取线上公共静态资源；不使用生产登录凭据、不上传到生产资产接口、不调用付费模型。`CANVAS_AUDIT_GROUP` 控制中文/ASCII 组；`CANVAS_AUDIT_IMAGE_SUCCESS=1` 返回假图片；`CANVAS_AUDIT_ASSET_FAILURE=1` 复现保存失败后的重复生成。

安全审查以官方认证、会话、授权指南为边界：保留最小权限、逐请求授权和资源归属，不把用户 Key 放入子应用、不把管理员身份当普通用户身份。相关官方依据为 [Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)、[Authorization](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)；不声称完成整个应用的 ASVS 认证。

### 六、2026-10-22 分析阶段结论（历史记录）

本轮已经找到了可复现的生成失败原因，并确认 UI、主题和媒体集成不是完整复用了 new-api/Tools 能力。原先“启动已修复、构建/假图片通过”不足以支持“所有画布业务都已完成”的结论。

当时已完成全面分析及分阶段改造边界，业务缺陷尚待实施。后续执行结果以 2026-10-23 修复章节为准；Tools 统一存储仍需独立的可见性、归属与生命周期产品约定。

## 2026-10-23 修复执行与阶段审查

### 已实施的源码变更

| 文件 | 变更与边界 |
| --- | --- |
| `middleware/canvas.go` | 对 `X-Canvas-Group` 执行 ASCII 检查、单次百分号解码和 UTF-8 校验；无效值返回 `canvas_invalid_group`，越权返回 `canvas_group_forbidden`，不自动改用其他分组。 |
| `middleware/canvas_test.go` | 覆盖 ASCII、中文编码、编码百分号、残缺 UTF-8、原始 Unicode 和控制字符。 |
| `service/canvas_catalog.go` / `controller/canvas.go` | bootstrap 复用 `GetUserImageModelGroups` 的图片能力，不在每次生成重查图片能力；能力查询失败明确报错。目录保留分组/模型/能力身份，视频适配依据真实渠道类型或已绑定插件，不猜公开模型别名；同组协议冲突拒绝提交。移除 bootstrap 未使用的分组说明。 |
| `integrations/infinite-canvas/overlay/config.ts` | 分组 channel 名改为真实分组名；模型 ID 仍保留分组用于授权和计费；画布请求头只在 HTTP 边界执行 `encodeURIComponent`；旧的独立主题偏好不再读取或写回。 |
| `integrations/infinite-canvas/overlay/bridge.ts` / `transport.ts` | 增加宿主主题消息、空/非 JSON 响应安全解析、结构化 HTTP 错误、Axios timeout 和 AbortSignal；桥转发 `X-Request-ID`/`X-Oneapi-Request-Id`。 |
| `integrations/infinite-canvas/overlay/start.tsx` / `layout.tsx` | 首次渲染使用宿主主题，运行中接收主题变化；删除画布内部独立浅色/深色按钮。 |
| `integrations/infinite-canvas/prepare.mjs` | 精确上游 SHA 的一次性构建补丁；复制当前仓库游乐场 types、normalizer、payload builder，保持实际请求语义共用；补节点和工作台保存重试、受限并发、历史关联、导出及背景工具栏。关键上游边界变化使构建失败。 |
| `integrations/infinite-canvas/overlay/model-picker.tsx` | 基于子应用现有 Ant Design Select 组合独立分组/模型选择，保留旧模型 ID。宿主 ModelGroupSelector 属于另一 React 树及组件库，不能直接插入沙箱；复用它的选择语义，不修改宿主公共控件。 |
| `integrations/infinite-canvas/overlay/image-settings-panel.tsx` / `image-api.ts` | 控件只展示当前能力允许的参数；实际调用直接复用游乐场的归一化/请求构造。单次 n=1，子应用图片请求最多并发 2；已生成结果的保存失败保留内存副本和 Run-ID，重试不提交生成。 |
| `integrations/infinite-canvas/overlay/image-settings.ts` / `video-settings.ts` | 组件与原有辅助函数的完整导出入口；与 TSX 面板分离，保留所有上游调用方使用的公开接口。 |
| `integrations/infinite-canvas/overlay/video-request.ts` / `video-settings-panel.tsx` | Sora 单参考图 multipart、明确时长与尺寸；豆包 JSON 参考图角色；对不支持的输入明确拒绝。视频下载仍走既有 owner-check 内容端点。 |
| `integrations/infinite-canvas/overlay/project.tsx` / `storage.ts` | 项目目录摘要、按需打开、失败重试；列表重命名和导出读取完整原文档，避免空摘要覆盖项目。资产上传携带来源运行并由服务器检查 owner。 |
| `integrations/infinite-canvas/overlay/messages.ts` | 子应用原有中英文两语言的新增界面和失败消息；不另加语言或外部运行依赖。 |
| `controller/canvas_run.go` | 对归档 JSON / SSE 识别业务错误、成功终态和截断，不改变既有账单结算；保存状态仍与生成状态独立。 |
| `integrations/infinite-canvas/browser-product-check.mjs` | 可复现的隔离业务浏览器验收：真实生产桥、中文组、主题、私有持久化/恢复、参数和视频参考字段；不访问生产账号或付费渠道。 |
| `Dockerfile` | canvas builder 复制共享游乐场源文件，使 CI 从全新目录构建时也使用同一参数规则。 |
| `web/src/features/canvas/index.tsx` / `bridge.ts` | 握手携带宿主 `resolvedTheme`，主题变化不重载 iframe；桥保留排障 request ID。 |
| `web/src/features/canvas/__tests__/host.test.tsx` | 增加宿主主题握手断言。 |

### 阶段自检结果

1. `gofmt`、`go test ./middleware ./service -run 'TestCanvas|TestCanvasCatalog' -count=1` 通过；中文分组解码测试覆盖了非法 UTF-8，服务目录现有分组隔离测试仍通过。
2. 画布精确上游 SHA `e856c878e0a34651bb828e28f0af20d71016a7d4` 在临时全新检出中完成 `tsc --noEmit` 和 Vite 生产构建。Vite 使用单核、2GB 内存封顶和显式 Node 堆上限；首次默认堆构建 OOM 后未被当作代码通过，第二次构建成功。
3. 画布宿主桥与主题测试 2 个文件、9 项测试通过。测试覆盖流式字节、取消后迟到消息、握手失败、账号切换、沙箱属性和深色宿主主题握手。
4. 受限本地浏览器 fixture 使用中文分组 `Codex特惠`、假 1px 图片和一次故意 503 的资产保存：服务端收到 `Codex%E7%89%B9%E6%83%A0`，模型标签为 `gpt-image-2`，保存重试后资产上传次数为 2，而生图 POST 仍为 1；未访问生产账号、数据库或真实付费模型。

### 验证边界与单独产品范围

- Tools 图床/媒体床仍未接入。现有两个服务的认证、owner、有效期和索引模型不允许把画布私有资产直接改成公开 URL；需要单独的会话委托和引用租约设计。
- Sora、豆包的参考图字段已按现有协议适配。其它视频模型按标准单 input_reference 协议；未宣称支持每个厂商的多参考图、私有参考视频和参考音频。当前不支持的输入在生成前拒绝，不公开用户素材，也不静默忽略。
- 前端图片历史记录安全的 error/httpStatus/code/requestId/runId 和失败保存状态；资产来源复用现有 SourceRunID。新界面与原有生成历史保持兼容；不修改五张 canvas 表结构。项目摘要查询、CAS、引用与隔离在 SQLite 3.50.4、MySQL 8.2.0、PostgreSQL 18.4 中验证。
- 未对真实付费文字、图片、视频或语音渠道发起请求；假上游浏览器结果不能替代真实渠道验收。

### 提交前最终审查记录

- 相关 Go 四包的 `TestCanvas` 均通过，包含真实会话/PAT 拒绝、中文头往返、越权、资金来源、资产来源 owner-check、幂等和 JSON/SSE 终态。真实三数据库矩阵验证摘要查询与既有画布数据；测试库已清理。首次矩阵因原测试打印大型 SQL 遭 broken pipe，关闭该测试连接的 SQL 日志后重跑通过，不把中断的运行写成通过。
- 宿主桥/主题 9 项、共用游乐场参数/请求构造 7 项通过，相关宿主文件 lint 和格式检查无 error。新增面板与请求适配的 lint 无 error；保留字符串拼接等风格 warning。TypeScript 与 Vite 完整画布生产构建通过；没有跳过文件或移除类型检查。
- `browser-product-check.mjs` 使用真实生产消息桥、opaque sandbox 和静态产物，验证：初始浅色/明暗即时同步、启动只读项目摘要、重命名保留节点、分组/模型分开与参数切换、中文请求头、空 403、第一次保存 503 后刷新恢复和只保存重试、错误条清除、Sora 单参考图字段。测试中保存重试/刷新阶段的生成 POST 始终为 1；另一次故意 403 为独立错误用例，不混入重复计费统计。视频用例验证提交字段与任务失败反馈，不当作真实视频成片验收。
- 保存竞态使用受控历史写入门：旧候选产物在 `saving must wait for the original history write` 断言得到 false/true 失败；最新产物同一用例通过。生成历史写入完成前，重试保存不可用；保存完成后按同一 Run-ID 清理对应媒体错误，不用其它偏好保存掩盖失败。
- 修改的 30 个文件均通过严格 UTF-8、替换字符/NUL、冲突标记及 `git diff --check` 检查；没有数据库表结构变更，没有 Key 注入模式，没有修改 Tools 或其它服务源码。
- 本机完整控制台类型检查受 1.5GB 资源上限终止，没有记为通过；Dockerfile 保留完整检查，必须等待本提交的双架构 Actions 全部成功后才允许发布。候选和公网仍须重跑启动与业务浏览器检查。

本节为提交前源码验收状态；最终镜像摘要、OCI revision、蓝绿切换、公开版本及观察结果在发布完成后补记，不预填成功结论。

### 首个候选启动发现的旧配置阻断

`5a6799792` 的双架构 Actions 40035509881 成功、镜像身份一致；候选绿槽健康，但启动检查发现此前已合入的 `0bab0308b` 严格分类校验拒绝生产旧 `auto` 类型绑定，导致其他有效分类未装载并持续报错。没有切流，公网仍由 `0858fac` 蓝槽服务，候选绿槽已正常停止。

补充范围仅为 `model/option.go` 的旧配置读取规范化和现有 `model/option_test.go` 回归：读取时移除系统自动路由项的冗余分类绑定，真实分组分类原样发布，写入仍严格校验，生产数据库原值保留。该项与画布分组路由/计费无关，不改变模型权限、倍率或 API Key。详细证据与验证见 `GROUP_TYPE_FIXES.md`。SQL 诊断已按 utf8mb4 复核中文，生产原值没有乱码。

补丁的 SQLite/MySQL/PostgreSQL 冷启动和重复重载、严格写入拒绝、原始 JSON 不变均通过；既有无效类型引用和事务故障回滚测试仍通过。重新构建的最终提交必须重新核对 OCI revision，并在候选日志跨至少一次配置同步周期无该错误后才切流。
