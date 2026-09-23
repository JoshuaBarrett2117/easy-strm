# Emby 管理

- 更新日期：2026-09-23
- 维护者：Codex

## 功能概览

用户“允许访问的媒体库”通过 Emby `/Library/SelectableMediaFolders` 获取名称与权限 `Guid`。`EnabledFolders` 使用该 `Guid`；媒体库刷新、封面等操作仍使用 `ItemId`，两者不可混用。easy-strm 的 `/emby/servers/:server_id/user-libraries` 返回 `data`、`total`，选项包含 `id`（权限 Guid）、`item_id`、`name`。

保存时后端将已知的旧 ItemId 映射为 Guid，保留原有策略中页面没有编辑的字段，提交后回读用户权限。范围不一致或回读失败时返回错误，不报告成功。“全部媒体库”勾选时清空隐藏的旧选择；未勾选且选择为空表示不允许访问任何媒体库。

协议来源：[Emby SelectableMediaFolders](https://dev.emby.media/reference/RestAPI/LibraryService/getLibrarySelectablemediafolders.html)。本地浏览器回归：在前端目录执行 `node scripts/e2e-emby-user-access.mjs`（通过 `E2E_FRONTEND_URL` 指定本地前端地址），使用模拟接口验证名称与保存参数，不修改真实 Emby 用户。

easy-strm 的 Emby 管理工作台位于“支撑配置 → Emby 管理”，用于统一维护多个 Emby 实例、用户、媒体库、媒体库封面及神医助手任务。

该工作台和对应管理接口仅允许 easy-strm 的 `admin` 用户访问；其他登录用户不会看到菜单，直接访问路由会回到仪表盘，直接请求接口会返回 403。

现有系统设置中的单实例 Emby 配置会在数据库升级时迁移为默认实例；已有媒体源的 Emby 媒体库关联也会绑定到该默认实例。

## 观影监控中心

“运维观察 → Emby 监控”仅对 easy-strm 的 `admin` 用户开放，并按 Emby 实例独立展示以下数据：

- 概览：近七天每小时活跃用户、自然日/周/月/历史观看时长、活跃用户和实时会话。
- 用户、媒体和客户端排行：支持日、昨日、周、月和总榜；媒体可切换电影与剧集。
- 活跃热力图：支持今日、昨日和近七天，可筛选单个用户。
- 最近入库：可按 Emby `DateCreated` 或 easy-strm 首次发现时间查看电影和剧集。

历史数据优先读取 [Playback Reporting](https://github.com/faush01/playback_reporting) 的管理员接口。插件缺失或请求失败时，整次统计自动降级为 easy-strm 本地采集结果，并在页面显示统计起始时间和降级原因；两套历史不会混合相加。

easy-strm 每 30 秒采集播放会话，仅累计连续确认处于播放状态的墙钟时间；暂停、会话失联、媒体切换和自然日跨界都会结束或切分记录。每 5 分钟扫描媒体列表，第一次只建立基线，避免把原有媒体误报为新入库。排行、热力图和入库查询缓存 60 秒，Redis 不可用时直接查询数据源。

## 任务中心

Emby 实例、用户、媒体库、封面和神医助手写操作都会生成任务中心记录。媒体库刷新和神医助手任务由后端轮询 Emby，前端只读取 easy-strm 任务状态。

“刷新全部媒体库”会逐库提交请求并记录每个失败项；部分媒体库提交失败、其余媒体库确认完成时，任务终态为 `partial_success`，任务详情展示成功数量、失败数量和 Emby 返回的失败摘要。

## 用户头像

用户列表通过 easy-strm 后端代理读取 Emby 头像，不向浏览器暴露实例 API Key。新增或编辑用户时可选择 JPG、PNG 或 WebP 头像，用户资料保存后上传，并生成独立的任务中心记录。

任务状态包括：

- `pending`：等待执行或等待用户确认封面。
- `running`：Emby 操作正在执行。
- `success`：已取得成功终态。
- `partial_success`：批量操作部分成功。
- `failed`：执行失败。
- `cancelled`：任务已取消。
- `unknown`：Emby 已接受请求，但在跟踪期限内未取得可信终态。

任务详情展示 Emby 实例、操作目标、发起方式、真实进度、执行步骤和最终结论。无法取得可靠百分比时只显示阶段说明。

## 媒体库封面

- 手动上传支持 JPG、PNG 和 WebP，选择文件后必须预览确认才会上传。
- 海报拼图默认读取最近入库且拥有主海报的六个媒体条目，生成 2×3 拼图。
- AI 封面支持 OpenAI 官方 Images API 和 OpenAI 兼容接口；生成结果保存为临时预览，确认后才上传到 Emby。
- AI 配置位于封面弹窗中，API Key 查询时只返回脱敏值。

## 神医助手

easy-strm 通过 Emby 插件列表和计划任务接口检测 StrmAssistant，并提供媒体信息提取、外挂字幕扫描和元数据刷新入口。easy-strm 不负责安装、升级或卸载插件；仅在用户确认“扫描 STRM 并生成视频封面”时，对该功能依赖的 `Library Scope` 做保留原值的增量修改。

### 扫描 STRM 并生成视频封面

神医助手页签提供“扫描 STRM 并生成视频封面”组合任务：

1. 页面弹窗说明即将修改的设置，用户确认后才创建任务。
2. 读取媒体库完整 `LibraryOptions` 和神医助手媒体信息提取设置。
3. 仅为适用的视频类型把 `Image Capture` 加入 `ImageFetchers`，保留其他媒体库选项。
4. `Library Scope` 为空时保持为空（空值代表全部媒体库）；非空时去重并增量加入所选媒体库，同时保留其他插件设置。
5. 回读媒体库与插件设置，确认两项配置均已生效。
6. 扫描用户选择的 Emby 媒体库，让新生成的 `.strm` 文件进入 Emby。
7. 回读该媒体库的 STRM 视频数量和现有主图数量。
8. 触发神医助手 `Extract MediaInfo` 计划任务并跟踪真实进度和终态。
9. 再次读取所选媒体库，记录新增主图和仍缺少主图的数量。

配置写入、回读或核验任一步失败时，任务会在任务中心记录可读错误并停止，不会继续提交媒体库扫描或神医助手任务。神医助手的计划任务按插件配置范围执行，因此可能同时处理范围内的其他媒体库；easy-strm 的结果统计只针对页面选择的媒体库。

部分 Emby 版本的 Generic UI 插件配置接口只接受管理员用户会话令牌，普通服务器 API Key 会返回 `user` 为空。easy-strm 不会在无法读取完整插件配置时盲目覆盖；此时任务会失败并提示当前凭据无法访问插件配置接口，避免丢失神医助手的其他设置。

插件任务成功结束不代表每个 STRM 都生成了图片。easy-strm 会根据回读结果设置终态：全部有主图为 `success`，部分仍缺图为 `partial_success`，全部仍无主图为 `failed` 并提示检查 Image Capture、Library Scope 和 ffmpeg 配置。

未检测到插件或对应计划任务时，页面会禁用操作并展示原因。安装说明见 [StrmAssistant Wiki](https://github.com/sjtuross/StrmAssistant/wiki)。

实现依据：

- [ExtractMediaInfoTask.cs](https://github.com/sjtuross/StrmAssistant/blob/main/StrmAssistant/ScheduledTask/ExtractMediaInfoTask.cs)
- [视频截图增强（Image Capture）](https://github.com/sjtuross/StrmAssistant/wiki/%E8%A7%86%E9%A2%91%E6%88%AA%E5%9B%BE%E5%A2%9E%E5%BC%BA-(Image-Capture))

## 数据升级

迁移脚本：`easy-strm/migrations/migrate_v17_emby_management.sql` 和 `easy-strm/migrations/migrate_v19_emby_monitor.sql`。

监控迁移新增播放事件、媒体首次发现和采集状态表，均按 `t_emby_server` 隔离；删除实例会级联清理 easy-strm 中对应的监控历史，但不会删除 Emby 服务器中的原始媒体文件。

## 媒体库编辑与定时任务

媒体库编辑使用路由中的 ItemId，在 `/Library/VirtualFolders/LibraryOptions` 请求体中传递 `Id` 和合并后的 `LibraryOptions`，保留页面未编辑的配置。名称通过 `/Library/VirtualFolders/Name` 修改，目录通过 `/Library/VirtualFolders/Paths` 的 `pathInfo` 查询参数添加或移除；不要把 `PathInfo` 作为 JSON 请求体传递，否则部分 Emby 版本会返回 `Unrecognized Guid format`。编辑已有媒体库时不能修改内容类型。多步骤操作失败时返回具体的部分完成信息，刷新后可继续处理。

“定时任务管理”按当前实例展示 Emby `/ScheduledTasks` 返回的任务、分类、状态、进度、触发规则与最近执行结果。点击“立即触发”调用 `/ScheduledTasks/Running/{Id}`；运行中和取消中的任务禁止重复触发。页签每 5 秒刷新，切换实例、离开页签和卸载组件时停止旧轮询并丢弃过期响应。

对应 easy-strm 接口为 `GET /emby/servers/:server_id/scheduled-tasks` 与 `POST /emby/servers/:server_id/scheduled-tasks/:task_id/run`，复用管理权限。任务中心的“Emby定时任务触发”记录只表示请求提交结果，实际远端执行结果以定时任务列表为准，不修改 Emby 的定时配置。

协议参考：https://dev.emby.media/reference/RestAPI/ScheduledTaskService.html 。本地验证覆盖缺失 Id 的编辑回归、配置保留、定时任务状态检查、上游错误及列表接口。
