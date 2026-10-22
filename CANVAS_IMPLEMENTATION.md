# 无限画布：实施与阶段审查

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
| `Dockerfile` | 镜像构建在打包控制台之前执行完整类型检查；画布继续使用独立 builder 的类型检查与构建 |
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

构建命令：在仓库根执行 sh integrations/infinite-canvas/build.sh，它使用精确 SHA 全新检出、冻结依赖锁、类型检查、Vite 生产构建，复制到 web/dist/canvas-app/。new-api 控制台需先构建，画布后构建；Docker 使用独立 builder，再把两套产物合并到 Go embed。MIT 许可随产物保留。本轮只推送代码，不部署现有容器。

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
4. 用户截图中的彩色加载图标不在现有 `LoadingState` 或画布入口中。尚无该浏览器实际 DOM、iframe URL、Console 或扩展信息，不能将图标认定为特定扩展，更不能据此认定其为故障原因。浏览器信息已请求，截图现象的归因仍待核实。

### 阶段自检记录

| 阶段 | 状态 | 检查与发现 |
| --- | --- | --- |
| 原因验证 | 部分已证实 | 完整页面的模块阻断复现与源码闭环；用户截图对应的浏览器链路尚未取得 |
| 启动链修复 | 已实现，回归继续 | 先编写两条失败测试，确认旧代码失败；修复后宿主/桥 8 项通过；保留 opaque sandbox、既有登录会话及单一免 Key 设计 |
| 完整构建和浏览器验收 | 进行中 | 浏览器脚本读取候选容器的实际公共页面与静态资源，所有 API 由本机假数据响应；旧镜像正常启动通过，阻断模块后按预期在错误状态断言失败，证明脚本覆盖本次缺陷；待新镜像复核四条完整流程 |
| 资源控制 | 已执行 | 浏览器/测试最多 1 CPU、1GB；本机全量类型检查上限 1 CPU、1.5GB、无交换，三次受限终止（已尝试 GC 目标和单线程）；生产构建编译阶段结束但进程未成功退出，不能记为通过。不提高本机硬上限，后续完整类型检查与构建以对应提交的镜像 Actions 日志为验收门槛 |

本次文件没有迁移、删除或数据库结构变更。新增文件只有上述专项浏览器回归脚本；宿主入口、桥及子应用入口/启动文件职责保持不变。只提交本轮文件，保留其他正在进行的分组相关修改。

## 安全审查依据

采用 [OWASP ASVS 5.0.0](https://owasp.org/projects/asvs)、[认证](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[会话](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)、[CSRF](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html) 对会话撤销、服务端授权、Bearer 请求、秘密排除与消息来源做具体验证，不声称完整 ASVS 认证。

## 最终审查结论

首次集成阶段已完成单一免 API Key 实现、文件职责、数据库/接口和阶段检查；后续线上截图证明原验收未覆盖所有启动失败路径。专项复核已复现并修复消息通道握手过早结束启动保护、动态模块失败未回报这两处缺陷，受影响 8 项测试、文件 lint 和 UTF-8 检查通过。完整构建与候选容器浏览器复核仍在进行，不能据此宣称用户截图的问题已经解决。

验收范围须保留三项事实：全量前端回归未完成；未对真实付费渠道执行文字、视频、语音生成；尚未拿到用户浏览器实际 DOM/Console，截图彩色加载状态的来源未证实。原 Chromium 图片验收使用假上游，不能当作所有真实渠道验证。后续发布须验证精确提交和 OCI revision，先验收备用容器，再切换流量并持续观察。
