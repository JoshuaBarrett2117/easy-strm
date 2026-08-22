# Operations Log

## 2026-08-19 Codex 115 绝对目录与目录树下拉

- 工具降级：当前未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`，使用 `rg`、本地文件读取和 `update_plan` 替代。
- 扫描文件工作台、STRM、115 账号转存、资源转存和离线下载中的目录配置入口，复用现有 `/115/files` 与目录树转换工具。
- 将 `TargetFolderPicker` 改为 Naive UI 下拉树，并让所有关联 115 账号的目录入口保存 `/影视资源` 形式的绝对路径。
- 后端媒体源配置拒绝 CID，浏览、自动监控和整理扫描通过 `GetCIDByPath` 在运行时解析目录。
- 验证通过：`go test ./...`、目录树 Node 测试、`npm run build`、`git diff --check`。

## 2026-08-18 Codex 云下载大批量队列提交

- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`，分别改用结构化上下文文件、`update_plan` 与 `rg`；任务不需要外部资料，未调用网络搜索。
- 使用 `rg`、`Get-Content`、`git status` 扫描云下载 domain/service/controller/DAO/前端 API 与测试链路。
- 对照了三种既有后台模式：`TransferScheduler` 的 channel worker、`WatchService` 的受控 goroutine、`ShareTransferService` 的 queued 任务状态。
- 接口决策：100条及以下保持同步受理；超过100条立即创建本地任务并返回 `queued=true`，后台单 worker 按100条顺序请求115。
- 使用 `apply_patch` 完成 domain 响应契约、Service 队列 worker、前端提交提示与轮询状态修改，并补充201条分批单测。
- 执行后端专项测试、`go test ./... -count=1` 与前端 `npm run build`，全部通过；未向真实115账号写入大批量测试任务。
- 附加 `-race` 检查因当前 Go 环境未启用 CGO 无法执行（`-race requires cgo`），已在测试与验证报告记录。

## 2026-07-15 Codex 重构验收与发布

- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`，分别改用结构化上下文扫描、`update_plan`、`rg`/`git diff`；本任务无外部资料需求，未调用网络搜索。
- 使用 `git status -sb`、`git diff --stat`、`rg --files` 扫描工作区、重构范围、运行入口与 E2E 脚本。
- 确认前端开发服务为 `npm run dev`（Vite 3001），后端为 `go run .`（Gin 8082），Vite 将 `/api` 代理到后端。
- 确认前端已迁移到 Naive UI，而多个 `scripts/e2e-*.mjs` 仍依赖 `.el-*` 旧 DOM 选择器，是本轮主要回归风险。
- GitHub 发布前置检查通过：`gh` 已安装且已登录，远程为 `JoshuaBarrett2117/easy-strm`。
- 首次执行 `go test ./...` 失败：历史临时工具 `easy-strm/codex_e2e_user.go` 与正式入口 `main.go` 重复定义 `main()`；该文件仅用于一次性创建测试用户且未被版本控制，已删除以恢复标准 Go 构建入口。
- 启动真实后端 `go run .`（8082）与前端 `npm run dev -- --host 127.0.0.1`（3001），使用现有忽略配置连接 PostgreSQL、Redis 与 115 账号。
- 浏览器真实验证通过登录、文件工作台、媒体源浏览、视频选择、TMDB 候选/手动搜索入口；项目 Playwright 继续完成真实整理和 STRM 闭环。
- 将全部 `easy-strm-front/scripts/e2e-*.mjs` 从 Element Plus `.el-*` 私有类迁移到 Naive UI DOM、ARIA 角色、业务文本和稳定 `data-testid`；执行 `node --check` 确认全部脚本语法有效，`rg '\.el-'` 结果为空。
- 为 `TaskCard`、TMDB 搜索结果、整理预览新文件名编辑动作补充稳定测试/可访问属性，降低脚本对框架内部 class 的依赖。
- 修复异步整理成功事件仍打开空同步结果弹窗的问题：异步任务提交后刷新文件列表并交由任务中心追踪。
- 真实 115 主流程通过：浏览源目录、TMDB 识别、复制整理、任务完成、STRM 配置自动匹配、STRM 任务完成、本地文件落盘；测试创建的远端目录已删除。
- 清理本轮运行产生的时间戳报告，恢复后端启动时改写的 `easy-strm/debug_directory_tree.txt`，避免提交运行时产物。

## 2026-04-13 Codex

- 发现浏览器全流程脚本默认前端地址仍是 `http://localhost:5173`，而项目实际 Vite 端口是 `3001`。
- 已将 [`easy-strm-front/scripts/e2e-fullflow.mjs`](../easy-strm-front/scripts/e2e-fullflow.mjs) 的默认前端地址改为 `http://localhost:3001`。
- 已重新启动后端 `easy-strm.exe`，确认监听 `8082`。
- 已重新启动前端 Vite 开发服务，确认监听 `3001`。
- 已执行浏览器全流程测试，结果 `40/40 PASS`。
- 已执行前端打包和后端 DAO 单测，均通过。
- 已补充整理弹窗预览刷新专项脚本：`easy-strm-front/scripts/e2e-organize-preview-refresh.mjs`
- 已实测专项脚本通过，确认“刷新预览”后按钮 loading 与弹窗遮罩都会退出。
- 已将专项验证并入总回归脚本 `easy-strm-front/scripts/e2e-fullflow.mjs`。
- 已执行新的总回归，结果 `41/41 PASS`，新增用例 `TC-ORG-UI-001` 通过。

## 2026-04-13 Codex

- 重点修复项来自后端：`TaskRedisDAO` 全局 Redis 客户端兼容 `dao.RedisClient` 与旧 `redisClient`。
- 该修复已由后端单测与浏览器全流程验证覆盖。

## 2026-04-18 Codex

- 新增并统一整理 [`docs/项目开发规范.md`](../docs/项目开发规范.md)，将历史测试里反复出现的坑收敛为全项目通用开发规范。
- 重点覆盖初始化顺序、测试签名同步、前后端响应结构统一、外部依赖失败判定、中文片名识别、前端按钮与弹窗回归等通用问题。
- 补充“不能再中文乱码”规范，统一要求源码、脚本、文档、日志和测试数据使用 UTF-8，并在中文文件名/提示/样例上做真实验证。
## 2026-04-19 Codex

- 清理 `WatchService` 中自动整理相关的乱码用户文案，统一为可读中文。
- 重新验证 `go test ./internal/service -count=1` 与 `go test ./... -count=1`，均通过。
## 2026-04-19 Codex

- 为 `WatchService` 增补失败分类和边界测试，覆盖 `target_path`、`scan_failed`、`cloud115_failed`、`cloud115_auth_failed`、`buildWatchFailureItemsFromIDs`。
- 执行 `gofmt -w` 后重新验证 `go test ./internal/service -count=1` 与 `go test ./... -count=1`，均通过。
## 2026-04-19 Codex

- 在媒体源表单提交前补充兜底逻辑：关闭监控时自动清除自动整理状态，避免提交冲突配置。
- 重新执行前端构建 `npm run build`，通过。
## 2026-04-19 Codex

- 补齐任务详情失败分组对 `partial_failed` 和 `conflict_skipped` 的显示，避免落入“其他失败”。
- 重新执行前端构建 `npm run build`，通过。
## 2026-04-19 Codex

- 增补 `summarizeAutoOrganizeResults` 与 `resolveWatchOrganizeDefaults` 的边界测试，覆盖混合失败分组和 115 目标模式收敛。
- 修正测试样例后重新执行 `go test ./internal/service -count=1` 与 `go test ./... -count=1`，均通过。
## 2026-04-19 Codex

- 在 `MediaSourceService` 里强制归一化监控配置：`watch_enabled=false` 时自动关闭 `auto_organize`。
- 新增创建媒体源的单测，验证监控关闭时不会保存冲突配置。
- 重新执行 `go test ./internal/service -count=1` 与 `go test ./... -count=1`，均通过。
## 2026-04-19 Codex

- 修复 `TaskCard` 中自动整理任务卡片的监控文案与状态文案，恢复为可读中文，并保持监控失败标签展示。
- 重新执行前端构建 `npm run build`，通过。

## 2026-04-24 Codex

- 修复 `easy-strm-front/src/views/Cloud115.vue` 中 115 账号“测试”按钮缺少明确成功反馈的问题，成功后统一显示可关闭的 `ElMessage` 成功提示。
- 排查发现异常弹窗根因不是接口失败，而是入口 `easy-strm-front/src/main.js` 未显式引入 `ElMessage` 服务组件样式，导致消息提示渲染异常并出现裸样式文本。
- 已在入口补充 `element-plus/es/components/message/style/css` 样式引入，收敛消息提示样式链路。
- 已执行前端构建 `npm run build`，通过。

## 2026-05-06 Codex

- 根据用户“参考 MoviePilot、Symedia、cloud-media-sync，不接入 PT，聚焦 115 与本地媒体库管理”的产品边界，新增并修订 [`docs/MoviePilot参考需求新增与重构.md`](../docs/MoviePilot参考需求新增与重构.md)。
- 已将待处理清单独立页面、STRM 输出“规则默认 + 配置覆盖”、115 增量“生活事件优先 + 30 分钟差异对账兜底”、Emby/Jellyfin/Plex 字段预留、动画按电视剧扩展分类等决策写入需求文档。
- 新增 [`docs/媒体库管理实施路线图.md`](../docs/媒体库管理实施路线图.md)，拆分 Phase 0 到 Phase 5，明确第一批开发任务为同步索引、任务步骤、待处理项、同步服务骨架、任务中心步骤展示和待处理页空状态。
- 新增 [`.codex/context-scan-moviepilot-requirements.json`](context-scan-moviepilot-requirements.json) 记录本轮上下文扫描、参考来源、关键疑问与充分性检查。
- 落地媒体库管理 Phase 0/1：新增迁移 `migrate_v14_media_library_sync.sql`，新增同步索引、任务步骤、待处理清单 domain/DAO/service/controller，扩展 `TaskService` 步骤接口。
- 新增同步 API：`POST /media/sources/:id/sync/full`、`POST /media/sources/:id/sync/incremental`、`GET /media/sources/:id/sync/index`，并新增媒体库与待处理 API。
- 新增前端页面 `MediaLibrary.vue`、`PendingMedia.vue`、`SyncTasks.vue`，Dashboard 菜单新增媒体库、待处理、同步任务，任务详情抽屉新增步骤时间线。
- 完整验证：`go test ./...` 通过，`npm run build` 通过，两个 Playwright E2E 通过，新增同步接口冒烟通过。

## 2026-05-06 Codex 复测补充

- 复查发现 `GET /media/library/items/:id` 仍是预留响应，已补齐 `MediaSyncIndexDAO.GetByID` 和媒体库详情聚合响应。
- 复查发现启动建表逻辑缺少迁移脚本中的更新时间触发器，已在 `EnsureMediaLibraryTables` 同步创建 `update_media_library_timestamp` 与三张表触发器。
- 为 `MediaSyncIndexDAO.GetByID` 新增 DAO 单测，保持媒体库详情查询有单元覆盖。
- 重启本地后端后完成完整验证：`go test ./...`、`npm run build`、`npm run e2e:organize-preview-refresh`、`npm run e2e:organize-preview-cancel` 均通过。
- 新增能力 API 冒烟覆盖媒体源创建、全量同步、同步索引、媒体库列表/详情、任务步骤、待处理创建/识别/忽略，结果通过。

## 2026-05-07 Codex

- 按用户要求一次性推进五项后续能力，新增 `MediaLibraryPipelineService`，将同步索引接入“识别 -> 待处理 -> STRM -> 媒体服务器刷新”的入库流水线。
- `MediaSyncService` 在写入索引后自动执行 pipeline，并补充 `identify_metadata`、`generate_strm`、`refresh_server` 任务步骤。
- `PendingMediaDAO` 新增开放待处理项复用逻辑，避免同一源文件反复同步时堆积重复待处理项。
- 待处理“入库”接口改为实际执行单项 pipeline，人工识别后的 TMDB 信息会回写同步索引，并将待处理项置为 `completed`。
- 媒体库 API 新增单项 pipeline、生成 STRM、刷新媒体服务器动作；同步 API 新增源级 pipeline 动作。
- 前端 `MediaLibrary.vue` 增加“入库 / STRM / 刷新库”操作，`SyncTasks.vue` 增加“执行入库”，`PendingMedia.vue` 的“入库”改为真实执行并显示 loading。
- 已执行完整验证：`go test ./...` 通过、`npm run build` 通过、两条 Playwright E2E 通过、Phase 2 API 冒烟通过。

## 2026-05-07 Codex 115 生活事件流

- 参考 `p115client` 的 `life_behavior_detail_app` 实现，确认 115 生活事件明细接口为 `GET https://proapi.115.com/android/behavior/detail`，参数包含 `offset`、`limit`、`type`、`date`。
- 在 `main.Client` 新增 `GetLifeEvents`，直接用当前 115 Cookie 拉取事件流，并容忍不同字段名：`file_id/fid/cid`、`pick_code/pickcode/pc`、`parent_id/pid/category_id`、`file_name/name/title`。
- 在 `MediaSyncService` 新增源级事件游标：`media_source:{source_id}:cloud115_life_cursor`，记录最近事件 ID、更新时间和最近目录对账时间。
- 115 增量同步策略调整为：事件接口可用且无新事件时跳过扫描；首次无游标、事件接口失败、客户端不支持或超过 24 小时未对账时回退目录扫描；发生有效事件后执行目录扫描对账并推进游标。
- 新增 `media_sync_service_test.go` 覆盖事件过滤、忽略浏览类事件、更新时间兜底、游标提取和事件类型汇总。
- 完整验证：`go test ./...` 通过，`npm run build` 通过，`npm run e2e:organize-preview-refresh` 和 `npm run e2e:organize-preview-cancel` 均通过。

## 2026-05-07 Codex 115 真实账号冒烟

- 用户确认库内两个 115 账号 Cookie 可用后，新增临时手动测试读取 `t_cloud_115`，不输出 Cookie 明文，仅输出账号 ID、名称、Cookie 长度和事件接口返回概要。
- 真实事件接口冒烟通过：账号 `id=2` 返回 `count=38010`、取回 `10` 条、`next_page=true`；账号 `id=3` 返回 `count=84`、取回 `9` 条、`next_page=true`。
- 检查库内启用 115 媒体源，确认存在 `id=186`、名称“小号整理”、路径 `/`、绑定账号 `id=3`。
- 尝试通过 HTTP API 触发源 `186` 增量同步时，默认 `admin/admin` 登录失败；未猜测密码。为避免直接对现有根目录媒体源执行非递归目录扫描并改写索引状态，本轮未通过临时测试强行触发完整同步写入链路。
- 临时手动测试文件已删除，重新执行 `go test ./...` 通过。

## 2026-05-07 Codex 浏览器全局仿真

- 按用户要求执行浏览器全局仿真。优先尝试 Codex in-app browser，但当前环境未发现可连接 IAB 后端，降级为项目 Playwright 浏览器自动化。
- 执行 `npm run e2e:organize-preview-refresh`，通过，确认整理弹窗刷新预览后 loading 和遮罩退出。
- 执行 `node scripts/e2e-fullflow.mjs`，通过，结果 `40/40 PASS`，覆盖登录、导航、Dashboard 接口、媒体源 CRUD、文件浏览/操作、整理预览/执行、刮削、日志、Emby 限制路径等。
- 补充一次媒体库三页浏览器仿真：创建临时本地媒体源，在“同步任务”页点击全量同步并看到 `扫描 1，写入 1，失效 0`，在“媒体库”页确认同步条目显示，在“待处理”页确认页面正常加载；测试结束删除临时媒体源。
- 单独验证 `/dashboard/cloud115`，确认“账号池总览”正常加载且表格显示 2 个账号。
- 执行 `npm run e2e:organize-preview-cancel`，通过，确认预览任务取消链路仍可用。

## 2026-05-14 Codex 资源整理平台首版重构收尾

- 按用户给定计划，将后台信息架构收敛为“媒体源 -> 同步索引 -> 媒体资产台账 -> 入库/STRM/刷新库 -> 任务中心 -> 待处理修正”的主流程。
- 重排 Dashboard 导航：资源整理入口前置为首页、资产台账、同步入库、待处理、任务中心；115 云管理、STRM 配置、整理规则、系统设置和日志/网络/缓存降级为支撑与运维入口。
- 强化 `MediaLibrary.vue`、`SyncTasks.vue`、`PendingMedia.vue` 三个页面，补齐 source_id 深链、状态摘要、任务跳转和主流程动作反馈。
- 删除已失效的 `StrmGenerator.vue`、旧 `src/utils/api.js`，并移除指向旧接口的 `/strm/task/all`、`/strm/config/:id/files` API 封装。
- 后端将单条资产的 STRM 生成和媒体服务器刷新包装为可追踪任务，接口返回 `task_id`，并补充单元测试。
- 新增 `npm run e2e:resource-platform`，用 Playwright + 模拟 API 覆盖同步入库、资产台账、STRM 任务深链、待处理修正并入库。
- 移除 `docker-compose.yml` 顶层废弃 `version` 字段，`docker compose config` 已无 obsolete warning。

## 2026-05-15 Codex 大文件深拆与上线验证

- 后端深拆：`organize_service.go` 拆分为任务、扫描、识别缓存、分类匹配、路径处理、执行与 115 云盘整理等职责文件；`115client.go` 拆分为事件流、文件操作、目录树、开放平台和秒传模块；`db.go` 拆分为用户、115账号、STRM配置、系统配置、STRM文件、定时任务、通知配置和 DB 实例模块；`auth.go` 拆分出 Token、公开路由、转换器和目录树辅助函数。
- 前端深拆：`Cloud115.vue` 抽离纯展示映射、脱敏、二维码状态、渠道提示和排序逻辑到 `src/utils/cloud115Display.js`，页面继续保留交互编排。
- 细粒度测试：新增 `115_life_events_test.go` 覆盖 115 事件流 JSON 辅助函数；新增 `auth_tokens_test.go` 覆盖 JWT 生成、校验与错误密钥拒绝。
- 上线验证：`go test ./...`、`go vet ./...`、`npm run build`、`npm run e2e:resource-platform`、`npm run e2e:organize-preview-refresh`、`npm run e2e:organize-preview-cancel` 均通过。
- 体积变化：`organize_service.go` 从约 2200 行降至 549 行；`115client.go` 从约 1800 行降至基础客户端文件；`db.go` 从约 1900 行降至 936 行；`auth.go` 降至 815 行；`Cloud115.vue` 从 1335 行降至 1212 行。

## 2026-05-15 Codex 大文件继续深拆

- 继续按“同包拆分、保持公开签名、先拆低风险边界”的策略推进：`cloud115_controller.go` 拆出登录、文件操作和通知配置处理；`organize_controller.go` 拆出执行任务与识别/更名处理；`media_source_controller.go` 拆出文件浏览和纯辅助函数。
- DAO 继续拆分：`tmdb_cache_dao.go` 拆出 `rename_preset_dao.go`、`media_file_cache_dao.go`、`dao_nullable.go`，保留原 SQL 行为和缓存 key 语义。
- 服务层继续拆分：`media_source_service.go` 拆出文件浏览与辅助函数；`rename_service.go` 拆出模板渲染、文件名解析、生成名归一化；`tmdb_service.go`、`watch_service.go`、`scrape_service.go` 已按前序拆分继续保持包级测试通过。
- 根包继续拆分：`db.go` 先抽出模型结构体到 `db_models.go`，降低初始化文件职责混杂度。
- 新增细粒度测试：controller 层覆盖媒体源文件 helper、整理执行请求默认值与失败分类；service 层覆盖媒体源文件 helper、文件类型、排序、过滤和面包屑。
- E2E 修正：`e2e-resource-platform.mjs` 中任务标题断言改为 `.first()`，避免同一标题同时出现在任务卡片和详情表格时触发 Playwright strict mode 冲突。

## 2026-05-27 Codex 功能盘点与浏览器验证

- 按用户要求对当前项目功能面做一轮整理，先读取 `README.md`、`docs/产品与架构.md`、`docs/开发与测试.md`，再对齐前端路由、页面入口、API 封装和后端路由注册。
- 确认当前后台共有 13 个主要前端入口：首页、资产台账、同步入库、待处理、任务中心、文件工作台、STRM 配置、115 云管理、整理规则、系统设置、系统日志、网络测试、缓存管理。
- 确认后端业务 API 覆盖 Dashboard、认证、媒体分类、115 登录/账号/文件/直链、通知配置、系统配置、网络测试、STRM、任务、Cron、媒体源、媒体库、待处理、文件操作、自动整理、TMDB、刮削、Emby、日志和缓存。
- 环境检查发现 Docker Desktop Linux daemon 未运行，`127.0.0.1:5432` 和 `127.0.0.1:6379` 均不可连接，因此真实后端无法启动完成真实数据浏览器全链路。
- 已执行 `go test ./...`，结果通过。
- 已执行 `npm run build`，结果通过。
- 已执行 `npm run e2e:resource-platform`，结果通过，覆盖同步、台账、STRM 任务深链、待处理识别与入库。
- 已尝试 Codex in-app Browser 访问 `http://127.0.0.1:3001`，被企业网络策略拦截；按 Browser 策略停止，不使用绕行方式。

## 2026-05-27 Codex 真实后端本地启动浏览器验证

- 使用用户提供的远端 PostgreSQL/Redis 连接信息启动后端；连接均成功，后端完成数据库初始化、Redis 初始化、Cron 加载和路由注册，监听 `:8082`。
- 启动前端 Vite 开发服务，监听 `127.0.0.1:3001`。
- 发现 `admin/admin` 在远端库中不是有效登录；为避免修改 admin，创建临时测试用户用于浏览器登录，测试完成后删除。
- 新增一次性验证脚本 `.codex/real-backend-browser-e2e-2026-05-27.mjs`，使用真实前后端执行登录、全路由巡检和临时媒体源同步链路。
- 浏览器验证结果 `ok=true`：13 个后台路由标题均匹配，无页面级网络错误，临时本地媒体源能在同步入库页显示，触发全量同步后资产台账能显示测试文件。
- 截图输出到 `debug/real-backend-browser-20260527/`。
- 清理：临时测试用户已删除；临时媒体源由脚本通过 API 删除；临时 Go 用户管理 helper 已删除。

## 2026-05-27 Codex 按钮矩阵浏览器验证

- 用户要求继续验证按钮，新增 `.codex/button-matrix-e2e-2026-05-27.mjs`。
- 按“安全真实点击 + 高副作用确认/取消路径 + 临时对象 CRUD”策略执行，避免删除真实账号、真实文件或清空真实缓存。
- 首轮脚本暴露多处 Playwright 严格模式定位问题，已收紧到顶部区、主内容区、确认框或精确按钮名后复跑。
- 最终结果 `ok=true`，通过 15 个按钮组：登录、顶部快捷、首页入口、同步入库、资产台账、待处理、任务中心、文件工作台、115、整理规则、STRM、系统设置、日志、网络、缓存。
- 清理：临时测试用户删除后再次登录确认失败；临时媒体源、分类和 STRM 配置均由脚本删除。
- 残余 warning：任务中心“详情”按钮可点击，但本轮未观察到详情抽屉打开，需要单独排查。
# 2026-07-16 UI 重构与美化

- 执行者：Codex
- `git status --short --branch`：确认工作区干净，`main` 跟踪 `origin/main`。
- `git pull --ff-only origin main`：从 `c789030` 快进到 `3664027`。
- 工具降级：当前会话未提供 sequential-thinking、code-index、shrimp-task-manager，使用本地 `rg`、文件读取与 `update_plan` 替代。
- 上下文扫描：检查 Vue/Vite/Naive UI/Tailwind 配置、全局主题、应用壳层、登录页、仪表盘及公共组件。
- 方案决策：不新增依赖，不改 API 与路由；通过全局设计令牌、Naive UI 覆盖、应用壳层和公共组件统一全站视觉。
- 实现：重构全局设计令牌、Naive UI themeOverrides、Dashboard 侧栏/顶栏/内容容器、登录页、Dashboard 首页与三个公共组件。
- 浏览器验收：桌面与 390px 窄屏登录页通过；发现全局 reset 覆盖 Tailwind 间距后已修复并复验。
- 验证：`npm run build`、`go test ./...`、`git diff --check` 均通过。
- 清理：关闭本轮临时 Vite `:4173` 服务并结束浏览器验收标签页。

# 2026-07-17 本地服务启动

- 执行者：Codex
- 前端：`npm run dev -- --host 127.0.0.1 --port 3001 --strictPort` 启动成功，HTTP 200。
- 后端：先后使用 `config.yaml` 与 `.env.test` 的 PostgreSQL 配置启动，均被 `192.168.31.12:15432` 返回密码认证失败。
- Docker 降级检查：仓库包含 PostgreSQL/Redis Compose 服务，但本机 Docker Desktop Linux daemon 未运行，无法启动本地依赖。
- 当前状态：前端保持运行；后端需要有效 PostgreSQL 凭据或可用的 Docker daemon。

# 2026-07-17 亮色主题与全模块回归

- 使用截图和 `Dashboard.vue` 定位侧栏固定深色 class，改为浅色默认 + `dark:` 深色覆盖。
- 启动 Docker Desktop，并通过 Compose 启动 PostgreSQL、Redis 和 `easy-strm` 主应用。
- 真实 API 测试覆盖鉴权、Dashboard、媒体源、同步、资产台账、文件操作、待处理、任务、分类、设置、日志、网络、缓存、Cron、通知等模块。
- 创建的临时媒体源、分类、待处理数据和容器测试文件均已清理。
- 发现 `t_notification_config` 未在全新数据库初始化，补齐 `InitDB` 建表逻辑，重建容器后复测通过。
- Chrome 访问 localhost 被企业网络策略阻止，遵循策略未切换其他浏览器自动化绕过；在测试报告中记录为未覆盖项。
- 验证通过：`npm run build`、`go test ./...`、`go vet ./...`、23 个 E2E 脚本 `node --check`、Docker 镜像构建、`git diff --check`。

# 2026-07-22 本地服务启动

- 启动 Docker Desktop，并恢复 PostgreSQL、Redis、easy-strm 主应用容器。
- 启动 Vite 前端开发服务 `127.0.0.1:3001`。
- HTTP 验证：前端与后端首页均返回 200；未携带 Token 访问鉴权 API 返回预期 401。
- 按用户要求将后端切换至 NAS PostgreSQL `192.168.31.12:15432/easy_strm_prod`，运行时凭据未写入仓库文件。
- 使用独立容器 `easy-strm-nas-test` 连接 NAS 数据库，并复用本地 Redis；后端 `:8082` 与前端 `:3001` 均返回 200。
- 启动日志确认数据库与 Redis 连接成功，管理员登录只读冒烟通过。

# 2026-08-09 前后端启动与手动验证

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`，改用本地 `rg`、`update_plan`、PowerShell 与应用内浏览器完成上下文检查和验证。
- 仓库检查：工作区存在用户未提交改动，本轮未修改业务代码，也未覆盖或清理现有改动。
- 环境检查：Go 1.25.6、Node.js 24.13.0、npm 11.6.2 可用；前端 `node_modules` 已就绪；配置中的 PostgreSQL `15432` 与 Redis `16379` 端口可达。Docker Desktop daemon 未运行，但本地进程启动不依赖 Docker。
- 后端启动：在 `easy-strm` 执行 `go run .`，成功连接 PostgreSQL/Redis 并监听 `:8082`；日志写入 `.codex/backend-manual-verify.*.log`。
- 前端启动：在 `easy-strm-front` 执行 `npm run dev -- --host 127.0.0.1`，Vite 成功监听 `127.0.0.1:3001`；日志写入 `.codex/frontend-manual-verify.*.log`。
- HTTP 探测：绕过本机系统代理后，前端 `/` 返回 200；后端根路径返回预期 404，Gin 路由服务正常监听。
- 浏览器验证：使用本地默认管理员登录，检查仪表盘、任务中心、资源聚合及两个页签、文件工作台、STRM 配置、115 云管理、整理规则、系统设置、系统日志、网络测试、缓存管理。
- 观察项：失效 Token 首次进入根路由会让仪表盘 mounted hook 报“登录已过期”后再跳登录页；历史本地媒体源为 Linux 路径，在 Windows 浏览时返回“路径不存在”；一条历史失败任务只显示“导出目录树失败:”而没有具体原因。
- 收尾：前后端继续保持运行，便于用户继续手动验证。

# 2026-08-09 仪表盘首屏空白修复

- 执行者：Codex。
- 根据用户截图定位到 `.surface-card > * { position: relative }` 覆盖 Tailwind `absolute` 工具类，导致 Hero 的两个光晕装饰进入普通文档流并累计占据 480px 高度。
- 修改 `easy-strm-front/src/style.css`：将卡片高光并入 `background-image`，删除伪元素覆盖层和强制修改所有直接子元素定位的规则。
- 影响范围：修复仪表盘 Hero 空白，同时恢复 `StatCard` 等卡片中绝对定位装饰的正常语义。
- 验证：前端生产构建通过；桌面 1280px 下 Hero 高 328px、与工作流间距 24px；390px 下无横向溢出，Hero 与工作流间距 20px。

# 2026-08-09 资源聚合目录选择器修复

- 执行者：Codex。
- 真实接口复现：`GET /115/files?cloud115_id=2&cid=0&show_dir=1&offset=0&limit=500` 返回 200 和 16 条记录，其中 11 条为根目录。
- 根因一：组件先读取 `response.data.data` 得到数组，随后又把数组当对象读取 `.files/.data`，最终得到空数组。
- 根因二：115 原始目录字段为 `cid/n/ico`，组件只识别 `is_directory/name/path`，即使提取到数据也会全部过滤。
- 实现：新增 115 响应提取和目录字段归一化工具；目录树加入根目录节点、错误提示、账号切换重载和基于 `cid` 的子目录懒加载。
- 浏览器复验：根目录可展示 11 个目录；展开“云下载”后显示下一级目录；选择后完整路径正确回填，验证结束后恢复根路径 `/`。
- 安全边界：仅执行 GET 目录查询和本地表单选择，未提交云下载、转存、删除或云盘写操作。

# 2026-08-10 GitHub main 发布

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`，使用本地 Git、`rg`、PowerShell 和既有测试命令完成审计与发布。
- 发布范围：提交工作区内资源聚合、115 分享转存、云下载、自动整理/刮削、Dashboard、数据库迁移、测试、正式文档和交付材料等全部项目代码变更。
- 发布清理：将 `.workbuddy/`、`.codex/manual-verify-processes.json` 和异常 `nul` 文件加入 `.gitignore`，不提交本地运行产物。
- 凭据清理：真实 115 分享码与密码已从测试、POC 和文档中移除；live 测试改为读取 `EASY_STRM_LIVE_SHARE_CODE`、`EASY_STRM_LIVE_SHARE_PASSWORD`，未配置时跳过。
- 同步检查：执行 `git fetch origin main` 后，本地 `main` 与 `origin/main` 均无领先或落后。
- 发布前验证：`go test ./...`、`go vet ./...`、`npm run build`、目录选择器 3 条 Node 单元测试和 `git diff --check` 全部通过。

# 2026-08-20 文件名整理识别验证

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`，改用 `rg`、PowerShell、`apply_patch` 和 Go 测试完成等价分析与验证。
- 定位 `OrganizeService -> IdentifyFileWithPath -> parseFilename -> SearchTV` 实际整理识别链路，并核对 3 个既有解析测试案例。
- 新增指定文件名回归测试，确认标题 `妖精的尾巴 百年任务`、类型 `tv`、季 `1`、集 `5`。
- 使用项目当前 TMDB 配置执行真实请求；TMDB 返回 HTTP 401 无效 API Key，未取得 TMDB ID、标准标题或候选列表。临时在线探针验证后已删除。
- 在 `easy-strm` 执行 `go test ./...`，全部通过。

# 2026-08-20 文件名识别测试页与规则配置

- 执行者：Codex。
- 工具降级：未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`，使用计划工具、`rg`、PowerShell、`apply_patch`、Go 测试和本地浏览器验证替代。
- 后端：新增可配置识别规则领域模型、默认模板、JSON 配置存储、编译/捕获组/示例校验、热更新缓存、本地解析接口和规则读写/重置接口。
- 规则模板：覆盖 `S01E05`、`1x05`、`Season 1 Episode 5`、`第1季第5集`、`EP05`、电影年份；纯集数动漫模板默认关闭以避免误判年份。
- 前端：新增“识别测试”路由与菜单，提供本地解析、TMDB 候选、季集展示、规则新增/删除/启停/排序/保存/恢复功能。
- 浏览器验证：页面路由、布局、样例入口和规则编辑区成功渲染；浏览器保存的旧会话 Token 过期后按既有全局逻辑跳转登录页，未使用或修改用户登录凭据。
- 运行验证：后端 `go test ./...`、`go vet ./...`、前端 `npm run build` 和 `git diff --check` 通过。

# 2026-08-20 统一文件管理页

- 执行者：Codex。
- 工具降级：当前环境未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`，使用 `update_plan`、`rg`、PowerShell、`apply_patch` 与本地测试替代。
- 上下文：检查现有本地文件操作、115 原生复制/移动、跨账号秒传、115driver 上传/下载/删除、任务中心及前端路由和 API 约定。
- 后端：新增统一位置、目录浏览、删除和异步传输契约；实现本地/115四种方向、同账号原生操作和跨账号递归秒传。
- 115 适配：新增单级建目录、批量删除、本地上传、带签名请求头下载到临时文件后替换目标文件。
- 前端：新增双栏文件管理页面、可复用文件栏组件、页级复制/剪切剪贴板、粘贴、删除确认、路由菜单和任务类型展示。
- 验证：服务与控制器定向测试、后端全量测试、Go Vet、前端生产构建和差异检查通过。

# 2026-08-20 GitHub main 发布

- 执行者：Codex。
- 发布范围：当前工作区内文件名识别测试与规则配置、统一文件管理页、跨位置传输、测试、审查记录和长期文档的全部变更。
- 同步检查：执行 `git fetch origin main`，本地 `main` 与 `origin/main` 领先/落后均为 0。
- 凭据检查：对全部新增和修改文件扫描 Cookie、Token、密码和 API Key 硬编码，未发现匹配。
- 发布前验证：`go test ./...`、`go vet ./...`、`npm run build` 和 `git diff --check` 全部通过。

# 2026-08-22 文件管理跨账号复制修复与本地目录验证

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`，使用计划工具、`rg`、PowerShell、`apply_patch`、Go 测试和本地浏览器验证完成等价流程。
- 诊断失败任务 `dd33e1b5-4dc2-4d43-8724-5b1623cfcfa8`，确认不同账号 115 秒传把 `pickcode` 错传给内部要求 `file_id` 的 `GetFile`，因此返回 `990002 参数错误`。
- 修复 `FileManagerService.copyCloudEntryAcrossAccounts`，跨账号文件秒传改为传入 `item.ID`；同步更新目录递归秒传回归测试。
- 创建并配置本地媒体源“产品测试”，路径为 `C:\Users\a3875\Downloads\产品`，自动整理与目录监控均关闭。
- 浏览器验证文件管理页成功读取该目录的 16 项内容；未执行任何 115 云盘写操作。
- 验证：`go test ./...`、`go vet ./...`、`npm run build`、`git diff --check` 全部通过。

# 2026-08-22 文件管理全链路回归与修复

- 执行者：Codex。
- 真实覆盖本地浏览/复制/剪切/删除、本地到115、115到本地、同账号115复制/剪切、主号到小号复制/剪切以及两账号115批量删除。
- 修复115下载：升级 `115driver` 至 v1.3.5，使用空 User-Agent 生成链接，规范化驱动返回的响应 Cookie；真实下载 13,090 字节文件成功。
- 修复跨账号复制：115私有秒传返回 `sig invalid`，文件管理改用受控临时文件下载后上传；目录递归处理，跨账号剪切仅在完整复制成功后删除源目录。
- 修复前端：禁用监控的媒体源仍可选择；异步粘贴按任务终态轮询并同步刷新双栏，传输中阻止重复提交。
- 清理：主号恢复至16项，小号恢复至11项；本地两个 `_Codex文件管理测试*` 目录已删除；`proxy_domains` 恢复为空。
- 验证：`go test ./...`、`go vet ./...`、`npm run build`、`git diff --check` 通过。
