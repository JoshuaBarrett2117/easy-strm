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

# 2026-08-29 115目录树签名与cron任务唯一性修复

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`，使用计划工具、`rg`、PowerShell、`apply_patch`、Go 测试和真实115只读下载验证完成等价流程。
- 上下文扫描：定位 `115_directory_tree.go`、`115client.go`、`db_strm_config.go`、`internal/service/strm_service.go`、`db.go` 以及相邻下载和cron测试实现。
- 依赖检查：执行 `go list -m -versions`、`go list -m -json @latest` 和 `go list -m -u -json`；`github.com/SheltonZhu/115driver v1.3.5` 仍为最新版本，未降级或修改依赖。
- 签名修复：显式使用 `UA115Browser` 获取下载地址，实际下载复用 `DownloadInfo.Header` 中的User-Agent和Cookie，重定向时继续透传同一组签名请求头；删除固定调试文件写入副作用。
- cron修复：`task_name` 改为展示字段，全量任务名称使用 `strm_config_id`；数据库移除任务名全局唯一约束，新增 `(strm_config_id, task_type)` 唯一索引，并提供启动归一化和 `migrate_v16_cron_task_identity.sql`。
- 真实验证：使用现有 STRM 配置 ID 2 触发115目录树导出并完成签名下载，获得7,608字节内容，解析通过且未出现403。
- 本地验证：签名定向测试、后端全量测试、Go Vet、前端生产构建和差异检查通过。

# 2026-08-29 GitHub main 发布准备

- 执行者：Codex。
- 同步检查：执行 `git fetch origin main`，本地 `main` 与 `origin/main` ahead/behind 均为0。
- 发布范围：当前工作区内115目录树签名修复、cron任务身份迁移、单元/真实集成测试和审计记录的全部变更。
- 凭据检查：扫描全部新增文件，未发现Cookie、Token、密码、Secret或API Key硬编码。
- 发布前验证：`go test ./... -count=1`、`go vet ./...`、`npm run build` 和 `git diff --check` 全部通过。

# 2026-08-29 Telegram 机器人通知与运维操作

- 执行者：Codex。
- 工具降级：当前环境未提供 `sequential-thinking`、`code-index`、`shrimp-task-manager`、`exa`，使用 `rg`、PowerShell、结构化上下文和计划工具完成等价流程。
- 上下文扫描：确认已有 Telegram 单向文本发送与通知配置表，缺少机器人长轮询、命令操作、事件接线和前端配置。
- 技术决策：采用 `github.com/go-telegram/bot v1.24.0`、单一私聊 Chat ID、长轮询、HTML 卡片和 Inline Keyboard。
- 后端实现：新增通知配置服务、Telegram 机器人生命周期与命令处理、统一卡片模型、任务/账号事件监控和专用配置接口。
- 前端实现：系统设置新增通知页签，支持脱敏 Token、Chat ID、事件开关、运行状态和测试卡片。

# 2026-08-29 Telegram 功能 GitHub main 发布准备

- 执行者：Codex。
- 分支与同步：本地位于 `main`；执行 `git fetch origin main` 后，本地与远端 ahead/behind 均为 0。
- 凭据审计：扫描全部修改和新增文件，未发现真实 Telegram Bot Token 或私钥。
- 发布门禁：`go test ./... -count=1`、`go vet ./...`、`npm run build`、`git diff --check` 全部通过。
- 发布策略：创建功能提交并以非强制方式推送到 `origin/main`；推送完成后核验远端提交。
- 发布结果：功能提交 `2c894e5` 已成功推送到 GitHub `origin/main`，未使用强制推送。
# 2026-08-30 STRM 配置列表与全量清理

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`，使用 `update_plan`、`rg`、PowerShell、`apply_patch` 与本地测试完成等价流程。
- 上下文扫描：检查 `StrmConfig.vue`、`cron.go`、`strm.go`，以及 MediaSourceList、FileBrowser、FileManagerService 三类相似实现。
- 根因：更新时间列仍存在于表格列模型；操作容器允许换行；全量生成只清理数据库记录，未清理本地目录。
- 充分性检查：接口契约、技术选型、清理风险与本地验证方式均已明确。
- 测试先行：新增目录清理回归测试，旧实现按预期失败；实现后专项测试通过。
- 前端实现：删除 `update_time` 列，操作按钮改用 `tiny` 尺寸和禁止换行布局，表格横向宽度同步收窄。
- 后端实现：`CleanupStrmFiles` 改为保留目标目录并清空全部子项；`RunFullStrmGenerate` 在生成前调用，零视频也清理。
- 验证结果：`go test ./... -count=1`、`go vet ./...`、`npm run build`、`git diff --check` 全部通过。
- 浏览器验证降级：本地前后端未运行；为避免连接现有数据库并加载定时任务，未启动真实后端，使用生产构建与静态列模型核对替代。
- 用户请求验证后启动本地服务：后端 `go run .` 监听 `:8082`，前端 `npm run dev -- --host 127.0.0.1` 监听 `:3001`；标准输出与错误日志写入 `.codex/runtime/*-strm-ui.*.log`。
- 可达性检查：前端根地址返回 HTTP 200；后端根地址返回预期 HTTP 404，Gin 服务和业务路由已成功监听，启动日志无错误。

# 2026-08-30 本地登录失败诊断

- 执行者：Codex。
- 后端日志显示浏览器旧 JWT 已过期，但登录页随后正常调用 `POST /login`，因此旧 Token 不是最终阻塞点。
- `admin` 用户可从数据库正常查询；提交默认密码 `admin` 的前端 MD5 后，后端因数据库已存密码哈希不同而返回 HTTP 401。`joshua` 用户在用户表中不存在。
- 启动日志确认本地进程实际连接局域网 PostgreSQL 的 `easy_strm_prod` 数据库和同机 Redis，并非独立本地数据库。
- 根因：`SeedAdmin` 仅在 `admin` 不存在时写入默认密码；共享数据库已有 admin，因此不会重置为默认密码。
- 未修改用户记录或数据库配置，避免未经确认影响共享环境账号。
# 2026-08-30 115 账号列表紧凑布局

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`，使用计划工具、`rg`、PowerShell、`apply_patch`、Vite 构建和本地页面验证替代。
- 上下文：核对 Cloud115、StrmConfig、MediaSourceList 和 FileBrowser 的表格操作区模式。
- 实现：移除 `update_time` 列；四个操作按钮改为 `tiny`；容器改为 `flex-nowrap whitespace-nowrap`；操作列从 320px 收窄至 280px，横向滚动宽度从 1720 调整为 1520。
- 验证：`npm run build` 与 `git diff --check` 通过；本地已登录页面表头不含更新时间，两行四个操作按钮的纵坐标分别一致，确认单行展示。
- 页面验证全程只读，未点击编辑、扫码更新、测试或删除。

# 2026-08-30 仪表盘账号配额修复

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`，使用结构化分析、`update_plan`、`rg`、PowerShell、`apply_patch` 和本地自动化测试替代。
- 检索：使用 `rg` 定位 `DashboardHome.vue`、`dashboard_service.go`、115 客户端及 `115driver v1.3.5` 的 `GetInfo`/`SpaceInfo` 契约。
- 根因：后端把 `total` 固定为 0，`quota_used` 又没有实时同步；前端将零比例替换为 12% 假进度，因此只能看到 `0 B` 和无意义进度条。
- 实现：为 Dashboard Service 注入最小容量查询接口；主程序复用现有 `Client` 调用 `115driver.GetInfo`；逐账号返回已用、总量、真实比例和可用状态；前端展示“已用 / 总量”，失败账号显示明确状态。
- 测试：新增正常容量、比例上限和单账号失败隔离回归测试。
- 验证：Dashboard 专项测试、后端全量测试、Go Vet、前端生产构建和差异检查通过。

## 运行时异常复核

- 用户反馈页面显示两个账号均“容量获取失败”。
- 进程核对：`8082` 上的 `easy-strm.exe` 于 10:45 启动，早于容量修复代码，仍返回不含 `available` 的旧契约；前端已热更新，因此把缺失字段判定为失败。
- 处置：仅停止端口 `8082` 上已核验的本地 `easy-strm.exe`，以 `go run .` 隐藏重启；新后端于 11:12:22 成功监听。
- 真实只读验证：仪表盘显示115主号 `21.5 TB / 63.4 TB`、115小号 `1.5 TB / 15.6 TB`；后端两次 Dashboard 请求均返回 HTTP 200，无容量查询警告。
- 本轮未新增代码修改，异常根因是运行进程未重载。

# 2026-08-30 账号配额五分钟缓存

- 执行者：Codex。
- 上下文扫描：检查 TMDB、分享解析、任务、通知事件等 Redis 缓存模式，以及缓存管理页与 miniredis 测试约定。
- 技术决策：新增 `AccountQuotaCache` DAO，键为 `easy_strm:dashboard:account_quota:<账号ID>`，JSON 保存 `used/total`，TTL 固定五分钟。
- 读取策略：Dashboard Service 优先读 Redis；命中即返回且不调用115；未命中、过期、损坏或 Redis 异常时调用115并回填。缓存故障不阻断页面。
- 管理集成：新增“115账号容量缓存”缓存组，支持现有缓存管理页统计和清理。
- 自动验证：覆盖TTL、到期未命中、损坏缓存删除、缓存优先、未命中回源、回源回填和单账号失败隔离。
- 运行验证：重启本地后端后首次加载调用115并写缓存；九秒后再次加载仍正常显示容量，日志没有新的115驱动调用，确认优先命中缓存。
- 2026-08-30：新增 Telegram 115 分享自动转存与云下载。复用现有分享解析/转存、离线下载和任务终态通知；实现 `链接 [账号名称] [/目录]` 语法、分享资源号选择、云下载 VIP/兼顾/资源类型排序、优先级与 ID 决胜、指定账号和标准 URL 密码解析。执行者：Codex。
- 2026-08-30：修复 Telegram 任务终态重复通知。根因是 Redis `EXISTS -> 发送 -> SET` 去重流程存在并发竞态；改为 `SET NX` 原子抢占发送权，发送失败释放抢占以允许重试。新增 8 路并发去重和发送失败释放回归测试。执行者：Codex。

# 2026-08-30 115自动转存目标目录修复

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`、`exa`，使用结构化分析、`update_plan`、`rg`、PowerShell、`apply_patch` 和本地测试替代。
- 扫描：检索 `TransferDirectory`、`TargetDirectory`、`GetCIDByPath`、`ReceiveShare` 及三个相似的115目录操作链路。
- 根因：`ShareTransferService` 已传入目标CID，但 `Client.ReceiveShare` 使用115不识别的 `save_folder_id` 表单字段；接口要求字段名为 `cid`，未知字段被忽略后文件进入默认的“最近接收”。
- 测试先行：新增表单协议回归测试，首次执行因构造函数不存在而失败；实现后通过。
- 实现：统一由 `buildReceiveShareForm` 构造请求并发送 `cid`；目标路径解析失败、空CID或非根路径返回CID=0时停止任务并记录明确错误，不再静默回退。
- 专项验证：根包协议测试与ShareTransferService失败保护/向后兼容测试通过。

# 2026-08-30 Emby 管理工作台

- 执行者：Codex。
- 工具降级：当前会话未提供 sequential-thinking、code-index、shrimp-task-manager；使用结构化需求文件、`rg`/代码阅读及内置任务清单替代。
- 新增 `t_emby_server`、媒体源 `emby_server_id` 关联和旧单实例配置迁移。
- 新增多实例、用户/权限/媒体库授权、媒体库 CRUD、刷新、封面及神医助手 API。
- 复用 Redis 任务中心；Emby 写操作记录实例、目标、发起方式、执行步骤、真实进度和最终结论。
- 前端新增 Emby 管理一级入口及实例、用户、媒体库、封面、插件和最近任务页面；任务中心增加 Emby 筛选与步骤展示。
- 新增 OpenAI 官方及兼容图片接口配置，手动/拼图/AI 封面均在确认后应用。
- 新增服务刷新终态、实例隔离和 Controller 参数测试；完成全量 Go 测试与前端构建。

## Emby 最终收口

- 使用 `rg`、PowerShell 和代码阅读复核刷新、头像、任务中心和路由权限链路；现有会话仍未提供 `sequential-thinking`、`code-index`、`shrimp-task-manager`。
- 使用 `apply_patch` 将“刷新全部”改为逐库提交，增加 `partial_success`、失败明细、成功/失败统计及本地模拟回归测试。
- 使用 `apply_patch` 增加用户头像读取代理与前端预览/上传入口，浏览器不直接接触 Emby API Key。
- 使用 `apply_patch` 将新增 Emby 管理接口纳入 `admin` 中间件，并在前端隐藏非管理员菜单、阻止直接路由访问。
- 使用 `gofmt`、Go 测试、Go Vet、Vite 构建和 `git diff --check` 验证；末次全量 Go 复跑被工作区同时发生的非 Emby 测试变化阻断，详情记录于 `.codex/testing.md`。
- 并行改动稳定后再次执行后端全量测试和 Go Vet，最终均通过。

# 2026-08-30 神医助手 STRM 扫描与视频封面

- 通过 StrmAssistant 公开源码和 Wiki 核对契约：截图由 `Extract MediaInfo` 计划任务结合媒体库 `Image Capture` 完成，不存在独立的“扫描 STRM”插件计划任务。
- 后端新增 `strm_scan_capture` 组合动作，串联 Emby 媒体库扫描、插件任务、有限轮询和选定媒体库结果回读。
- 前端新增独立操作卡、必选媒体库、配置前置提示；任务中心新增 STRM 和主图统计字段。
- 使用本地 HTTP 模拟测试验证完整链路，并执行全量 Go 测试、Go Vet 和 Vite 构建。

# 2026-08-30 文件工作台自动整理与监控修复

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`、`exa`，使用结构化分析、`update_plan`、`rg`、PowerShell、应用内浏览器只读核对、`apply_patch` 和本地自动化测试替代。
- 运行证据：后端启动后没有任何监控初始化请求；页面热更新按 `enabled` 重算后，3 个媒体源均明确显示“媒体源已停用”，监控统计由错误的 2 变为真实的 0。
- 根因：前端媒体源表单遗漏 `enabled`，新建请求被后端 bool 零值保存为停用；状态展示却只看 `watch_enabled/auto_organize`，造成“显示运行、后台过滤”的假开启。
- 实现：创建接口缺省 `enabled=true`；编辑表单恢复媒体源状态开关；统计与状态文案同时校验 `enabled`；115 关联账号按账号列表映射显示。
- 运行态：媒体源列表由 WatchService 填充 `watch_running`，配置开启但启动失败时显示“监控未运行”，保存动作返回警告而非假成功提示。
- 监控增强：本地递归注册已有和新建子目录；115 递归完整分页；115 初始快照失败时拒绝启动，避免误整理历史文件。
- 测试先行：新增测试在旧实现下因默认值错误和递归扫描函数缺失而失败，实现后专项与全量测试通过。
- 运行处置：核验并重启 8082 上由 `go run` 启动的本地后端；未替用户开启媒体源，未触发真实文件整理或115写操作。
# 2026-08-30 115 自动整理文件名修复

- 根据任务中心截图和本地后端日志定位：115 监控以 PickCode 做增量判重后，错误地把 PickCode 直接作为 `OrganizeDirectory` 的 `file_ids`，导致扫描不到源文件，任务中也只能显示 `csn9...` 标识。
- 调整 `collectCloud115VideoFileSet`：仍以 PickCode/文件 ID 作为稳定判重键，同时保存文件名及相对监控目录的完整路径；新增文件触发整理时传递相对路径。
- 增加旧失败任务兼容：恢复 115 自动整理任务时，将历史 PickCode 重新解析为当前相对文件路径。
- 已执行定向回归、`go test ./... -count=1`、`go vet ./...`、`npm run build` 和 `git diff --check`，均通过。

# 2026-08-30 内置站点默认代理

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`、`exa`；使用结构化分析、`update_plan`、`rg`、PowerShell、`apply_patch` 和本地测试替代。
- 扫描：定位网络探测、代理域名归一化、TMDB/Telegram/Emby HTTP 客户端及系统配置页面。
- 根因：空 `proxy_domains` 被解释为不匹配任何站点，且 TMDB Service 使用独立直连客户端。
- 测试先行：新增内置域名和自定义追加测试，并为 TMDB HTTP 客户端注入补断言；旧实现按预期编译失败，实现后专项测试通过。
- 实现：代理地址有效时默认代理 Telegram、GitHub、TMDB；自定义域名作为追加规则；TMDB 真实请求接入代理感知客户端；前端字段改为可选追加项并明确内置站点。
- 审查修正：内置域名始终存在后，补充代理地址有效性判断，确保未配置 `proxy_url` 时探测结果与实际请求均显示并保持直连。
- 验证：专项测试、`go test ./... -count=1`、`go vet ./...`、`npm run build` 和目标文件 `git diff --check` 全部通过。

# 2026-08-30 TMDB 网络探测鉴权

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`、`exa`；使用结构化分析、`update_plan`、`rg`、PowerShell、`apply_patch` 和本地测试替代。
- 根因：网络探测直接访问 TMDB `/3/configuration`，没有携带系统配置中的 `tmdb_api_key`，因此代理连通时仍固定返回 HTTP 401。
- 测试先行：新增 TMDB 参数附加、原查询参数保留、非 TMDB 地址不修改和错误密钥脱敏测试；旧实现按预期编译失败。
- 实现：每轮探测动态读取最新 TMDB Key，仅为 `api.themoviedb.org` 实际请求附加 `api_key`；结果继续返回原始公开 URL，底层错误统一替换明文 Key。
- 验证：专项测试、后端全量测试、Go Vet、前端生产构建和差异检查全部通过。

# 2026-08-30 默认命名模板调整

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`shrimp-task-manager`、`code-index`、`exa`；使用结构化分析、`update_plan`、`rg`、PowerShell、`apply_patch` 和本地测试替代。
- 扫描：定位 `RenameService` 内置模板、系统配置读取、数据库官方预设和旧值迁移、设置页面变量标签及模板测试。
- 实现：电影默认目录加入年份和 TMDB ID；剧集默认目录加入年份、TMDB ID 与 `Season N`，文件名按条件输出英文标题、画质、来源和编码。
- 兼容：只将与两代历史官方默认模板完全一致的系统配置和官方预设升级，自定义模板不覆盖。
- 验证：模板专项测试、后端全量测试、前端生产构建和差异检查通过；随后工作区并行新增的 Emby 管理未完成代码阻塞了 `go vet` 和测试复跑，已作为非本任务风险记录。

## 2026-08-30 Codex — 115 Cookie 来源

- 工具降级：当前会话未提供 sequential-thinking、code-index、shrimp-task-manager，改用模型深度分析、`rg`/PowerShell 只读检索和 `update_plan`。
- `view_image`：核对参考图，确认需求是把 Cookie 获取端/渠道作为账号可见元数据。
- `rg`、`Get-Content`、`git diff/status`：定位 Cloud115 模型、双 DAO 链路、Controller 回调、扫码登录及前端表格/表单，并确认工作区已有未提交改动。
- 决策：新增自由文本 `cookie_source`；扫码渠道自动写入中文标签；历史数据保持空值并显示“未标注”；同步修正扫码更新未消费 `cloud_id` 的问题。
- `apply_patch`、`gofmt`：完成数据库迁移、双模型/DAO、HTTP 契约、扫码创建/更新、前端表单与列表、文档和测试修改。
- 首轮 `go test ./...`：发现并修正既有 sqlmock 的旧列断言；第二轮全量测试通过。
- `npm run build`：Vite 生产构建通过，4254 个模块完成转换。
- `go vet ./...`：后端静态检查通过。
- `git diff --check`：无空白错误，仅有工作区既有换行符提示。

## 2026-08-30 Codex — 已有 Cookie 渠道来源资料核查

- 工具降级：项目要求优先 exa，但当前会话未提供；按 Browser skill 使用网页检索，并结合 GitHub API 与本地 Go 模块缓存核对源码。
- p115client 文档与源码确认：`UID` 值格式为 `<user_id>_<ssoent>_<timestamp>`，可直接从已有 Cookie 提取设备码；`R1=wechatmini`、`R2=alipaymini`。
- p115client 还提供 `login_device`、`login_devices`、`login_online` 等接口，但文档明确当前 Cookie 不一定出现在设备列表中，不能以账号最近设备替代当前 Cookie 的来源。
- 115driver v1.3.5 已有 `GetInfo()`，请求 `https://webapi.115.com/files/index_info`，响应包含 `login_devices_info.list[].ssoent/is_current`；适合作为在线校验或补充信息。
- 结论：历史 Cookie 来源优先本地解析 UID，零网络开销且对应当前 Cookie；接口只作为 UID 异常/未知码时的补充，不应作为主判断路径。

## 2026-08-30 Codex — 历史 Cookie 来源自动识别实现

- `rg`、`Get-Content`、`git diff`：复核上一阶段 `cookie_source` 模型、API 与页面现状，保留工作区既有修改。
- `apply_patch`：新增 domain 层 UID 三段式解析与完整 ssoent 映射；Controller 对历史空来源进行只读推断并返回 UID/设备码；Vue 账号列组合显示 UID 与渠道。
- 规则：人工来源优先；A1 保留“网页版 / 115 浏览器”歧义；未知码显示“未知渠道”；无法解析时显示“未标注”。
- `gofmt`、`go test ./...`：格式化并执行后端全量测试，全部通过。
- `go vet ./...`：静态检查通过。
- `npm run build`：Vite 生产构建通过，4843 个模块完成转换；仅输出既存分块大小提示。

## 2026-08-30 Codex — 神医助手截图依赖一键配置

- 工具降级：当前会话未提供 sequential-thinking、code-index、shrimp-task-manager、exa，改用结构化分析、`rg`、PowerShell、`apply_patch` 与本地测试。
- 源码核验：StrmAssistant 的空 `LibraryScope` 表示全部媒体库；适用类型的 `ImageFetchers` 包含 `Image Capture` 才代表真正启用。
- 浏览器只读核验：确认 Emby Generic UI 使用 `UI/View` 读取配置、`UI/Command` 的 `PageSave` 保存配置；未点击保存或修改真实 Emby 设置。
- 实现：确认弹窗后由同一后台任务读取、增量修改、回读核验 Image Capture 与 Library Scope，核验成功后才扫描媒体库并触发 Extract MediaInfo。
- 兼容：Generic UI 不接受普通服务器 API Key 时明确失败并停止，不盲写或伪报配置成功。
# 2026-08-30 Codex — Emby 观影监控中心

- 工具降级：会话未提供 sequential-thinking、code-index、shrimp-task-manager、exa；使用结构化分析、`rg`、PowerShell、`apply_patch`、GitHub API 只读调研和本地测试替代。
- 上下文：确认未提交的 Emby 多实例实现为基线，新增独立 Monitor Domain/DAO/Service/Controller，避免把统计逻辑并入管理 Service。
- 数据：新增 v19 迁移、30 秒会话采集、5 分钟媒体基线、Playback Reporting 优先/本地整套降级和 60 秒 Redis 缓存。
- 前端：新增仅管理员可见的 Emby 监控菜单、六个模块、ECharts 图表、15 秒实时刷新和图片代理兜底。
- E2E：连续三次失败后暂停复盘，定位 Naive UI 标签延迟挂载导致首次进入不加载，增加 immediate 监听后恢复并通过。

## 2026-08-30 Codex — 历史 Cookie 来源最终审查

- 聚焦审查：确认人工来源优先、历史空来源按 UID 动态推断、未知设备码保留原码、无效 UID 不误判。
- `apply_patch`：修正编辑表单回填逻辑，UID 推断值仅用于展示，不会在编辑其他字段时被误固化为人工来源。
- `go test ./...`、`go vet ./...`：后端全量测试与静态检查通过。
- `npm run build`：Vite 生产构建通过，4843 个模块完成转换；仅保留既存 Emby 大分块提示。

## 2026-08-30 Codex — 已配置 Cookie 来源运行时刷新

- 截图复核：页面显示“Cookie UID 无法识别 / 未标注”，与当前源码预期不一致。
- Browser skill 降级：已登录页面没有可接管标签，本地 8082 被浏览器策略拦截；改用本地只读数据库诊断。
- 只读诊断：两个既有账号 UID 均为合法三段式，设备码分别为 R2、R1；未输出 Cookie 内容，临时诊断测试已删除。
- 根因：8082 仍由旧 `go run .` 进程提供服务，没有加载来源识别实现。
- 运行时处理：仅重启明确监听 8082 的 easy-strm 后端，未修改数据库 Cookie；新进程已于 17:42:55 正常监听。

# 2026-08-30 企业微信应用通知渠道

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`code-index`、`shrimp-task-manager`、`exa`；使用既有结构化上下文、计划工具、`rg`、PowerShell、`apply_patch` 和本地测试完成。
- 架构扫描：复用现有通知配置表、代理感知 HTTP 客户端、结构化通知卡片与任务/账号事件监控，不新增数据库表或第二套 HTTP 客户端。
- 后端实现：接入企业微信自建应用 `gettoken`、`message/send` 官方接口，增加内存 Token 缓存、Markdown 字节截断、Secret 脱敏/留空保留、专用配置与测试接口。
- 事件实现：通知监控按渠道加载事件开关，Redis 基线、任务终态抢占和账号状态快照按 Telegram/企业微信隔离。
- 前端实现：设置页通知页签新增企业微信应用表单、接收范围、事件开关、保存和测试操作。
- 验证：企业微信专项测试、后端全量测试、Go Vet、前端生产构建和差异检查通过。
- 运行时修复：用户保存时企业微信接口返回 404；确认 8082 后端启动时间早于企业微信路由源码修改时间，而 Telegram 路由正常存在。仅重启监听 8082 的 `easy-strm` 进程后，企业微信 GET/PUT/POST 路由均进入 JWT 中间件并返回未授权 401，证明新路由已生效。
- API 接收消息：引入企业微信官方示例 `wxbizmsgcrypt`，新增公开 GET/POST 回调、Token/AESKey 脱敏配置、签名与 Corp/Agent 校验、消息解密、Redis 去重和异步应用回复。文本支持帮助、状态、任务及115资源操作；前端新增回调 URL、随机 Token/AESKey 和接收开关。

# 2026-08-30 文件工作台概览板块清理

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`code-index`、`shrimp-task-manager`；使用计划工具、`rg`、PowerShell 和 `apply_patch` 完成上下文分析、规划与实现。
- 上下文扫描：确认截图红框对应 `MediaManager.vue` 中位于 `MediaSourceList` 前的工作台头部、当前上下文、概览指标、快捷动作和本轮选择。
- 依赖分析：保留文件浏览与批量操作仍依赖的 `currentSource`、`selectedFiles`、`getFileType`；删除仅服务于概览区的派生状态、导入和父子组件暴露接口。
- 实现：页面现在直接从“媒体源管理”开始，后续文件浏览、识别、重命名、整理和刮削对话框链路保持不变。

# 2026-08-30 企业微信云下载创建反馈修复

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`code-index`、`shrimp-task-manager`；使用计划工具、`rg`、PowerShell、`apply_patch` 和本地 Go 测试完成。
- 根因：云下载任务已成功创建并生成反馈卡片，但主动命令回复复用了普通通知的 `detail_url` 策略，因而发送为消息转发代理兼容性不稳定的 `textcard`。
- 修复：仅对用户主动命令回复使用配置副本并清空 `DetailURL`，强制发送精简文本；不修改持久化配置，也不影响后台任务和账号事件通知。
- 可观测性：回复成功后记录成员和反馈标题；失败仍保留警告日志。
- 回归：新增测试验证配置了 `detail_url` 时依然以文本回复，并保留“115 云下载已提交”和任务 ID。

# 2026-08-30 通知渠道多行115任务提交

- 执行者：Codex。
- 上下文：企业微信与 Telegram 共用 `TelegramResourceService`；原实现使用 `strings.Fields` 解析整条消息，只能识别第一个 URL，并会把第二行错误拼入目录。
- 实现：先按 CRLF/LF 的非空行拆分，再对每行独立解析、选择账号并调用既有分享转存或云下载提交链路；单行行为保持原样。
- 容错：多行中的参数错误、无效链接、账号不可用或提交失败会记录为该行失败，不中断后续行。
- 反馈：批量卡片包含总数、已创建、失败，以及基于 magnet `dn`/哈希生成的行标签、任务 ID 或错误原因；成功时提供“查看最近任务”操作。
- 工具降级：当前会话未提供 `sequential-thinking`、`code-index`、`shrimp-task-manager`；使用计划工具、`rg`、PowerShell、`apply_patch` 和本地测试完成。
- 分享转存补充验证：两条115分享链接分别完成解析与提交，各自应用密码、账号和目录；纯分享批次使用“115 分享转存批量提交完成”标题。

# 2026-09-01 任务触发与终态双阶段通知

- 执行者：Codex。
- 需求解释：将“所有任务”和“通知渠道”映射到统一任务中心中的全部任务，以及已启用的 Telegram、企业微信、Server 酱和 SMTP 渠道。
- 实现：任务监控为每个任务新增 `started` 事件，创建/发现任务时先发送“任务已触发”；终态仍按完成、失败、取消发送第二次通知。两类事件使用独立 Redis 键并分别抢占，发送失败可重试。
- 配置：Telegram 和企业微信新增 `notify_task_started` 字段与设置页开关，历史配置默认启用；Server 酱和 SMTP 没有独立事件开关，已启用时默认订阅全部任务事件。
- 升级边界：事件基线从 `v1` 升级到 `v2`，避免已有 Redis 历史任务在升级后补发触发通知；首次启用仍只建立基线。

# 2026-08-30 云下载记录名称列宽度约束

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`code-index`、`shrimp-task-manager`；使用计划工具、`rg`、PowerShell 和 `apply_patch` 完成。
- 根因：名称 / 链接列仅声明 `minWidth`，超长磁力链和 ED2K 链接可能继续撑开首列，挤出后续列。
- 实现：名称 / 链接列改为固定 `420px`，继续复用现有两行 `NEllipsis` 展示名称和链接，不改变数据与交互逻辑。
- 用户回归：仅设置 `width: 420` 后仍被长链接撑开。检查 Naive UI 2.41 源码确认默认 `tableLayout` 为 `auto`，且单独 `width` 只派生同值 `minWidth`，不会派生 `maxWidth`。
- 修正：增加 `maxWidth: 420`、`table-layout="fixed"` 和 `scroll-x="1140"`，并限制单元格容器溢出；宽屏显示全部列，窄屏按声明总宽度横向滚动。

# 2026-08-30 115 离线下载 UA 回归修复

- 执行者：Codex。
- 工具降级：当前会话未提供 `sequential-thinking`、`code-index`、`shrimp-task-manager`、`exa`；使用显式根因分析、计划工具、`rg`、PowerShell、GitHub API 只读检索和本地测试替代。
- `git status`、`git diff`：确认工作区已有多项未提交修改，目标文件 `115client.go` 和 `115client_test.go` 属于现有超时/缓存改动，后续只做增量补丁。
- `rg`、`Get-Content`：追踪前端 JSON 请求、Controller、Service、`115_offline.go` 与 115driver v1.3.5 的离线加密响应解析链路；前端传参无误。
- GitHub API：核对 115driver issue #56，同样的 `invalid character 'd'` 对应上游响应 `decode fail!`，维护者明确要求使用 `UA115Browser`。
- 根因：当前构造顺序为 `driver.New(UA115Browser).SetHttpClient(...)`；`SetHttpClient` 重建 Resty 客户端并清空先前 User-Agent，导致离线接口解密失败。
- 充分性检查：接口契约、技术选型、风险和验证方式均已明确；进入回归测试与实现阶段。
- `apply_patch`：先在既有 `115client_test.go` 增加 UA 断言；首次定向测试按预期失败，实际 UA 为空字符串。
- `apply_patch`、`gofmt`：改用 115driver 自带 `WithClient(...)` 与 `UA(...)` Option，确保先注入代理感知超时客户端、后写入 `UA115Browser`；未新增依赖或自研离线协议。
- 定向验证：驱动构造、离线下载 Service、离线下载 Controller 测试全部通过。
- 全量验证：`go test ./... -count=1`、`go vet ./...`、`npm run build` 与目标差异检查通过；Vite 仅保留既存 Emby 大分块提示。
- 边界决策：未向真实115账号提交用户磁力链接，避免验证过程额外创建云端任务；本地测试已覆盖导致 `decode fail!` 的请求头根因。
- 运行时更新：确认 8082 仍由源码修改前启动的本项目 `go run .` 提供服务；只重启该后端，新的 `easy-strm` 进程于 23:45:47 正常监听。
- 运行时冒烟：未携带 Token 请求 `/v1/resource/115-offline/tasks` 返回预期 401，证明新进程已完成路由和鉴权初始化。

# 2026-08-30 项目功能缺口分析与完善需求文档

- 执行者：Codex。
- 请求：全仓库分析现有功能、缺失能力和未完善链路，输出可执行的完善需求文档。
- 工具降级：当前会话未提供 `sequential-thinking`、`code-index`、`shrimp-task-manager`、`exa`；本任务不需要外部事实搜索，使用计划工具、并行源码审查、`rg`、PowerShell、`apply_patch` 和本地验证替代。
- 工作区保护：开始时发现前后端与既有 `.codex` 留痕存在未提交修改；本任务不改业务代码，只新增独立命名的分析上下文和需求文档，并以追加方式维护留痕。
- 初步证据：后端注册了完整媒体、资源、任务、通知与 Emby 路由；同时发现 STRM 增量生成固定返回 501、115 Open API 登录为占位、115 云刮削明确跳过写入，进入针对性深挖。

# 2026-08-31 项目功能缺口分析与完善需求文档（完成）

- 执行者：Codex。
- 任务 ID：`project-gap-analysis-20260830`。
- 范围：审计 Go 后端、Vue 前端、PostgreSQL/Redis、Docker 部署、测试与长期文档；仅编写需求和审计留痕，未修改业务代码。
- 工作区保护：审计开始前已有大量未提交前后端修改；本任务未覆盖、回滚或整理这些改动，只新增独立命名的 `.codex/context-*-project-gap-*` 文件和 `docs/项目功能完善需求.md`，并追加共享留痕。
- 上下文收集：完成结构化请求、全仓快速扫描、三轮高优先级深挖和充分性检查；接口契约、技术选型、主要风险与验证方式均已明确。
- 并行审计：分别完成后端、前端、部署/文档审计，再交叉核对路由、Controller、Service、DAO、迁移、前端 API 和页面调用。
- 产出：登记 29 个有源码证据的缺口，形成 16 个需求包、P0-P3 优先级、M0-M3 里程碑、统一任务/API/capability 契约、成功指标、依赖、风险和非目标。
- 核心判断：暂停扩展 PT、资源搜索、订阅、下载器和插件市场，优先交付 RQ-01 路径与范围保护、RQ-02 STRM 安全生成、RQ-03 版本化迁移、RQ-04 Cron 运行态一致性、RQ-05 Docker 新装闭环。
- 红队复核：修正 M0/M1 对 RQ-06/RQ-13/RQ-15 的优先级倒挂，将条件终态、最小 capability、Compose live/ready 明确为前置切片；区分活跃态、待处理态和不可改写终态，并将取消终态竞态 G-07 调整为 P0。
- 红队增量证据：本地执行 `docker compose -f docker-compose.yml config --images` 复现默认镜像展开为无效的 `/easy-strm:latest`，已纳入 G-06/RQ-05 P0。
- 工具降级：当前会话未提供项目手册指定的 `sequential-thinking`、`code-index`、`shrimp-task-manager` 和 `exa`；使用计划工具、`rg`、PowerShell、并行代理、源码阅读和本地自动验证替代，并在需求文档中声明。
- 外部写入边界：未执行真实 115、Emby、通知或用户媒体目录写入 E2E，避免审计过程产生外部副作用。
