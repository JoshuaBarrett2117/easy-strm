# 当前功能细节与浏览器验证报告

- 日期：2026-05-27
- 执行者：Codex
- 项目：easy-strm
- 目的：整理当前项目功能细节，并基于功能细节进行一轮浏览器验证。

## 1. 功能总览

当前项目定位是个人媒体库整理与 `.strm` 生成工作台。主链路为：

```text
媒体源接入 -> 文件浏览/同步索引 -> TMDB 识别 -> 入库/整理 -> STRM 生成 -> 任务追踪 -> 待处理修正
```

前端采用 Vue 3 + Vite + Element Plus，后端采用 Go + Gin，数据依赖 PostgreSQL + Redis。后端分层为 `domain`、`dao`、`service`、`controller`。

## 2. 前端页面与功能细节

### 登录

- 路由：`/login`
- 页面：`easy-strm-front/src/views/Login.vue`
- 能力：
  - 用户名、密码登录。
  - 登录后保存 `token` 和 `user_id`。
  - 已登录访问登录页会跳转到 `/dashboard/home`。
  - 未登录访问后台会跳转回登录页。

### 后台壳层

- 路由：`/dashboard`
- 页面：`easy-strm-front/src/views/Dashboard.vue`
- 能力：
  - 三组导航：资源整理、支撑配置、运维观察。
  - 顶部快捷入口：资产台账、任务中心。
  - 深色/浅色主题切换。
  - 退出登录。
  - 移动端侧栏展开/收起。

### 首页

- 路由：`/dashboard/home`
- 页面：`easy-strm-front/src/views/dashboard/DashboardHome.vue`
- 能力：
  - 主流程快捷入口：资产台账、同步入库、待处理、任务中心。
  - 展示 STRM 资产、云盘账号、运行中任务、启用媒体源等指标。
  - 展示运行中/今日完成/今日失败任务。
  - 展示媒体源统计、账号配额、运行资源监控。
  - 展示近 7 天 STRM/整理趋势。
  - 展示网络探针和最近入库记录。

### 同步入库

- 路由：`/dashboard/sync-tasks`
- 页面：`easy-strm-front/src/views/SyncTasks.vue`
- API：`getMediaSources`、`getMediaSyncIndex`、`runFullMediaSync`、`runIncrementalMediaSync`、`runMediaLibraryPipeline`
- 能力：
  - 选择媒体源。
  - 查看同步索引。
  - 执行全量同步。
  - 执行增量同步。
  - 执行入库流水线。
  - 从同步结果跳转资产台账、待处理、任务中心。

### 媒体资产台账

- 路由：`/dashboard/media-library`
- 页面：`easy-strm-front/src/views/MediaLibrary.vue`
- API：`getMediaLibraryItems`、`runMediaLibraryItemPipeline`、`generateMediaLibraryItemStrm`、`refreshMediaLibraryItemServer`
- 能力：
  - 按媒体源、同步状态、健康状态过滤资产。
  - 查看资源条目、健康状态、识别状态、同步状态、STRM 状态、元数据状态。
  - 查看源路径、目标路径和最近任务。
  - 单条执行入库。
  - 单条生成 STRM。
  - 单条刷新媒体服务器。
  - 跳转同步页、待处理页、任务中心。

### 待处理

- 路由：`/dashboard/pending-media`
- 页面：`easy-strm-front/src/views/PendingMedia.vue`
- API：`getPendingMediaItems`、`identifyPendingMediaItem`、`runPendingMediaItem`、`ignorePendingMediaItem`
- 能力：
  - 查看识别失败或待人工处理资源。
  - 按媒体源过滤。
  - 人工填写标题、年份、类型、季集、TMDB ID。
  - 保存识别结果。
  - 保存并重新入库。
  - 单条重新入库。
  - 忽略待处理项。
  - 跳转资产台账和任务中心。

### 任务中心

- 路由：`/dashboard/tasks`
- 页面：`easy-strm-front/src/views/dashboard/TaskCenter.vue`
- API：`getUnifiedTaskList`、`getTaskDetail`、`cancelTask`、`resumeTask`
- 能力：
  - 查看统一任务队列。
  - 按任务状态统计。
  - 自动刷新任务列表。
  - 查看任务详情。
  - 查看任务步骤时间线。
  - 查看失败项与失败分类。
  - 取消可取消任务。
  - 恢复可恢复任务。
  - 支持 `task_id` query 深链打开详情。

### 文件工作台

- 路由：`/dashboard/media-manager`
- 页面：`easy-strm-front/src/views/MediaManager.vue`
- 组件：`MediaSourceList.vue`、`FileBrowser.vue`、`OrganizeDialog.vue`、`OrganizeResultDialog.vue`、`TmdbIdentifyDialog.vue`、`TmdbCandidatesDialog.vue`、`RenamePreviewDialog.vue`、`ManualIdentifyDialog.vue`
- 能力：
  - 媒体源编排：新增、编辑、删除、浏览。
  - 媒体源类型：本地存储、115 云盘。
  - 媒体源默认规则：媒体类型、冲突策略、整理方式、整理目标目录。
  - 监控配置：监控目录、目录监控、自动整理、轮询间隔、Emby 媒体库关联。
  - 文件浏览：目录面包屑、返回上级、刷新、搜索、分页、双击进入目录。
  - 文件操作：重命名、删除、复制/移动/批量操作。
  - TMDB：单文件自动识别、候选结果选择、手动搜索、批量识别、目录批量识别。
  - 重命名：单文件重命名、批量重命名预览、执行重命名。
  - 整理：选择候选、异步预览、刷新预览、执行整理、失败项重试、手动识别覆盖、手动文件名覆盖。
  - 刮削：单文件刮削、批量刮削、目录批量刮削。
  - 整理结果：查看成功/失败列表，支持生成 STRM 和刷新 Emby。

### STRM 配置

- 路由：`/dashboard/strm-config`
- 页面：`easy-strm-front/src/views/StrmConfig.vue`
- API：`getStrmConfigList`、`createStrmConfig`、`updateStrmConfig`、`deleteStrmConfig`、`generateFullStrmConfig`、`getStrmTaskStatus`、`getCronTasks`、`updateCronTask`、`runCronTask`
- 能力：
  - 配置 115 账号、网盘目录、本地目录、扩展名、同步模式、转存账号、转存目录、清理策略、并发数。
  - 新增、编辑、删除 STRM 配置。
  - 手动全量生成 STRM。
  - 查看生成任务进度。
  - 管理关联 Cron 任务。
  - 启用/停用定时任务。
  - 立即运行定时任务。
  - 查看定时任务状态。

### 115 云管理

- 路由：`/dashboard/cloud115`
- 页面：`easy-strm-front/src/views/Cloud115.vue`
- API：`getCloud115List`、`createCloud115`、`updateCloud115`、`deleteCloud115`、`testCloud115Connection`、`get115LoginChannels`、`get115QRCode`、`check115LoginStatus`、`confirm115Login`、`get115OpenQRCode`、`check115OpenLoginStatus`、`confirm115OpenLogin`
- 能力：
  - 查看账号池总览、账号结构、优先级、转存覆盖。
  - 新增、编辑、删除 115 账号。
  - Cookie 手动接入。
  - 扫码登录新增账号。
  - 扫码更新账号。
  - 支持 115 登录渠道选择。
  - 支持 Open API 扫码登录。
  - 测试账号可用性。
  - 账号类型：资源号、VIP 观影号、兼顾。
  - 账号状态：正常、冷却中、禁用。
  - 转存账号、转存目录、秒传方式配置。

### 整理规则

- 路由：`/dashboard/category-strategy`
- 页面：`easy-strm-front/src/views/CategoryStrategy.vue`
- API：`getMediaCategories`、`createMediaCategory`、`updateMediaCategory`、`deleteMediaCategory`
- 能力：
  - 电影/电视剧分类策略分 Tab 管理。
  - 新增、编辑、删除分类。
  - 配置目标目录。
  - 配置匹配规则：Genre、国家/地区、语种、年份、标题关键字。
  - 展示规则摘要。
  - 默认分类兜底。

### 系统设置

- 路由：`/dashboard/settings`
- 页面：`easy-strm-front/src/views/Settings.vue`
- API：`getSettings`、`updateSettings`、`getTmdbConfig`、`updateTmdbApiKey`、`getEmbyStatus`、`getEmbyLibraries`
- 能力：
  - Alist 服务地址与访问令牌。
  - TMDB API Key 与语言配置。
  - 日志保留天数。
  - 代理服务器地址与代理站点列表。
  - Emby 集成启用、服务地址、API Key、连接测试。
  - 整理后同步刮削开关。
  - NFO、海报、背景图、剧照下载开关。
  - 电影和电视剧命名模板配置。
  - 模板变量快捷插入。

### 系统日志

- 路由：`/dashboard/system-logs`
- 页面：`easy-strm-front/src/views/dashboard/SystemLogs.vue`
- API：`getLogFiles`、`getLogFileContent`、`getLogConfig`、`updateLogConfig`
- 能力：
  - 选择日志文件。
  - 查看日志内容。
  - 刷新日志。
  - 自动刷新。
  - 调整日志保留天数。

### 网络测试

- 路由：`/dashboard/network`
- 页面：`easy-strm-front/src/views/dashboard/NetworkCenter.vue`
- API：`getNetworkProbeSites`、`testNetworkConnectivity`
- 能力：
  - 查看探测站点。
  - 重新探测。
  - 展示连通状态、代理路径、HTTP 状态码、耗时和错误信息。

### 缓存管理

- 路由：`/dashboard/cache`
- 页面：`easy-strm-front/src/views/dashboard/CacheCenter.vue`
- API：`getCacheOverview`、`clearCacheGroup`
- 能力：
  - 查看缓存概览。
  - 按缓存组查看条目数和存储位置。
  - 清理单组缓存。
  - 清理全部缓存。

## 3. 后端接口覆盖

后端公开接口：

- `POST /login`
- `POST /auth/login`
- `GET /direct-link`

后端鉴权接口按模块覆盖：

- Dashboard：`/dashboard/stats`、`/dashboard/overview`、`/dashboard/resource-monitor`、`/dashboard/trends/:kind`
- 缓存：`/cache/overview`、`/cache/clear`
- 用户：`/user/info`
- 媒体分类：`/media/categories`
- 115 登录：`/115/login/channels`、`/115/qrcode`、`/115/login/status`、`/115/login/confirm`
- 115 Open API：`/115/open/qrcode`、`/115/open/login/status`、`/115/open/login/confirm`
- 115 账号：`/cloud115`、`/auth/cloud115/:id`
- 115 文件：`/115/files`、`/115/direct-link`、`/115/export-dir`、`/115/export-dir/status`
- 通知：`/notify/config`、`/notify/test`
- 设置：`/settings`
- 网络：`/network/test`
- STRM：`/strm/config`、`/strm/config/:id/generate/full`、`/strm/config/:id/generate/incremental`、`/strm/task/:task_id`、`/strm/config/generate/from-organize`
- 任务：`/tasks`、`/tasks/unified`、`/tasks/:task_id`、`/tasks/:task_id/cancel`、`/tasks/:task_id/resume`
- Cron：`/cron/tasks`、`/cron/task/:id/run`、`/cron/task/:id/status`
- 媒体源：`/media/sources`、`/media/sources/:id/sync/full`、`/media/sources/:id/sync/incremental`、`/media/sources/:id/sync/index`、`/media/sources/:id/pipeline`
- 媒体文件：`/media/files`、`/media/files/search`、`/media/files/move`、`/media/files/copy`、`/media/files/delete`、`/media/files/rename`、`/media/files/batch`、`/media/files/preview`
- 媒体库：`/media/library/items`、`/media/library/items/:id/pipeline`、`/media/library/items/:id/strm`、`/media/library/items/:id/refresh-server`
- 待处理：`/media/pending`、`/media/pending/:id/identify`、`/media/pending/:id/run`、`/media/pending/:id/ignore`
- 自动整理：`/media/organize/candidates`、`/media/organize/preview`、`/media/organize/execute`、`/media/organize/batch-identify`、`/media/organize/batch-rename-preview`、`/media/organize/identify`、`/media/organize/presets`
- TMDB：`/media/tmdb/search`、`/media/tmdb/identify`、`/media/tmdb/auto-identify`、`/media/tmdb/batch-identify`、`/media/tmdb/movie/:id`、`/media/tmdb/tv/:id`、`/media/tmdb/config`
- 刮削：`/media/scrape/file`、`/media/scrape/files`、`/media/scrape/directory`
- Emby：`/emby/status`、`/emby/libraries`、`/emby/refresh`
- 日志：`/logs`、`/logs/:filename`、`/logs/config`

## 4. 本轮验证

### 已通过

```powershell
Set-Location .\easy-strm
go test ./...
```

结果：通过。

```powershell
Set-Location .\easy-strm-front
npm run build
```

结果：通过。

```powershell
Set-Location .\easy-strm-front
npm run e2e:resource-platform
```

结果：通过，输出：

```json
{
  "ok": true,
  "checked": [
    "sync",
    "ledger",
    "strm task link",
    "pending identify and run"
  ]
}
```

### 浏览器覆盖细节

`npm run e2e:resource-platform` 使用 Playwright 浏览器和模拟 API，实际覆盖：

- 进入同步入库页。
- 选择媒体源并执行增量同步。
- 看到同步任务结果。
- 跳转资产台账。
- 对资产条目执行 STRM。
- 看到 STRM 任务 ID。
- 跳转任务中心并查看任务详情。
- 进入待处理页。
- 打开人工修正弹窗。
- 填写标题和 TMDB ID。
- 保存并重新入库。
- 看到重新入库任务 ID。

## 5. 受限项与未覆盖原因

- Docker Desktop Linux daemon 未运行，无法用 Docker 启动 PostgreSQL、Redis 和主应用。
- 本机 `127.0.0.1:5432` 不通，PostgreSQL 不可用。
- 本机 `127.0.0.1:6379` 不通，Redis 不可用。
- 后端真实服务依赖 PostgreSQL 与 Redis，因此本轮无法完成真实后端启动。
- Codex in-app Browser 访问 `http://127.0.0.1:3001` 被企业网络策略拦截，已按策略停止，不做绕行。

未完成真实成功路径：

- 真实登录后逐页点击所有菜单。
- 真实本地媒体源创建和文件整理执行。
- 真实 115 扫码登录、账号测试、云端文件浏览、直链、目录导出。
- 真实 TMDB 查询、识别、电影/剧集详情。
- 真实 Emby 连接、媒体库读取、刷新。
- 真实 STRM 文件落盘与 Cron 触发。
- 真实日志文件读取、缓存清理、网络探针执行。

## 6. 结论

当前项目功能面已经形成“资源整理主流程 + 支撑配置 + 运维观察”的后台结构。代码层面前后端功能入口完整，自动化验证层面 Go 测试、前端构建和资源平台核心浏览器 E2E 均通过。

真实全功能浏览器点测当前被本地服务依赖与 in-app Browser 企业策略限制阻塞；恢复 PostgreSQL、Redis、Docker 或可访问的后端环境后，建议复跑真实页面级测试，重点覆盖文件工作台、115 云管理、STRM 配置、系统设置和运维观察页的真实 API 成功路径。

## 7. 真实后端复跑补充

- 日期：2026-05-27
- 执行者：Codex
- 触发：用户提供远端 PostgreSQL/Redis 连接信息后复跑。

### 启动结果

- 后端已使用远端 PostgreSQL/Redis 启动成功，监听 `http://127.0.0.1:8082`。
- 前端已启动成功，监听 `http://127.0.0.1:3001`。
- 历史媒体源 `测试本地媒体源` 的目录不存在，后端 WatchService 启动时输出告警，但不阻塞服务。

### 浏览器验证

脚本：

```powershell
node .codex\real-backend-browser-e2e-2026-05-27.mjs
```

结果：通过，`ok=true`。

覆盖：

- 临时测试用户登录。
- 13 个后台路由逐页访问并验证标题：
  - 首页
  - 媒体资产台账
  - 同步入库
  - 待处理
  - 任务中心
  - 文件工作台
  - STRM 配置
  - 115 云管理
  - 整理规则
  - 系统设置
  - 系统日志
  - 网络测试
  - 缓存管理
- 创建临时本地媒体源。
- 同步入库页显示临时媒体源。
- 触发全量同步。
- 资产台账显示测试文件 `Codex.Real.Backend.2026.1080p.mkv`。
- 清理临时媒体源。

截图：

- `debug/real-backend-browser-20260527/login-dashboard.png`
- `debug/real-backend-browser-20260527/dashboard_home.png`
- `debug/real-backend-browser-20260527/dashboard_media-manager.png`
- `debug/real-backend-browser-20260527/dashboard_cloud115.png`
- `debug/real-backend-browser-20260527/core-flow-library.png`

清理：

- 临时测试用户已删除。
- 临时媒体源已删除。

观察：

- 浏览器 console 捕获 2 条 `403 Forbidden` 资源加载错误，但未触发 `pageerror`，也未影响路由巡检和核心同步链路。

## 8. 按钮矩阵复跑补充

- 日期：2026-05-27
- 执行者：Codex
- 脚本：`.codex/button-matrix-e2e-2026-05-27.mjs`

### 结果

通过，`ok=true`，没有 failed 项。

### 覆盖通过

- 登录页按钮。
- 顶部资产台账、任务中心快捷入口。
- 主题深色/浅色切换。
- 首页 4 个主入口。
- 同步入库：刷新媒体源、全量同步、增量同步、执行入库、查看资产台账。
- 资产台账：同步入库、待处理、行内入库、STRM、刷新库。
- 待处理：查看资产台账、修正弹窗保存路径、忽略路径触达。
- 任务中心：刷新、详情按钮触达。
- 文件工作台：新增媒体源弹窗、媒体源浏览弹窗、文件列表刷新。
- 115 云管理：新增账号弹窗、扫码登录弹窗、账号编辑弹窗、账号测试按钮触达。
- 整理规则：刷新、新增取消、选择临时分类、删除取消。
- STRM 配置：新增、编辑临时配置、删除取消。
- 系统设置：重置、保存 TMDB 配置、测试 Emby 连接。
- 系统日志：刷新、自动刷新开关。
- 网络测试：重新探测。
- 缓存管理：刷新概览、清理全部缓存取消。

### 清理

- 临时媒体源已删除。
- 临时分类已删除。
- 临时 STRM 配置已删除。
- 临时测试用户已删除，删除后登录确认失败。

### 残余项

- 任务中心“详情”按钮可点击，但自动化未观察到详情抽屉打开；该项建议单独排查。
- 本轮未执行高副作用真实动作：删除真实账号、删除真实文件、清理真实缓存、对既有 STRM 配置执行全量生成。
