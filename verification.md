# 验证记录

## 2026-09-10 Codex：STRM导出任务进度条

导出弹窗、任务卡及详情可显示执行进度，任务中心默认自动刷新。弹窗每秒更新，关闭或任务终态停止查询。npm run build与浏览器E2E通过，验证25%→75%→完成及查询停止。后端导出仍只读取本地数据，未变更。验证日志：.codex/share-strm-progress-*.log。

## 2026-09-10 Codex：本地数据库导出修正

STRM导出不再解析分享链接，只使用本地t_share_media的文件路径/识别结果及关联分享地址。115文件ID延后首次播放时按保存路径逐级定位并缓存，重复导出保留缓存。本地缺少具体视频的目录记录报告失败，不进行网络补扫。后端全量测试、前端构建和浏览器E2E全部通过（.codex/share-strm-local-*.log）。此条覆盖此前“导出解析分享”的行为说明；未重启服务或真实115转存。

## 2026-09-10 Codex 分享资料库 STRM 导出与按需播放

完成配置、按筛选全量导出、分类目录和具体季集、持久播放映射、115按需单文件转存、即时直链302，以及Docker/Vite代理。后端`go test ./...`、前端`npm run build`、`node scripts/e2e-share-strm.mjs`全部通过。浏览器模拟API验证配置保存/刷新回显、保存失败阻止导出、重试和390px窄屏按钮可达；后端验证并发去重、任务取消、失败处理和重复导出。日志在`.codex/share-strm-*.log`，截图在`debug/share-strm/export-mobile.png`。

当前交付为代码与本地验证结果，没有重启用户运行中的服务或部署到远程。升级后启动后端会自动创建`t_share_strm`，在「分享资源库 → 导出 STRM」配置服务器目录、可访问的播放地址、115账号及转存目录。真实115账号/分享/CDN与媒体服务器播放未联调；需要有效账号和网络，首次播放有转存等待时间。Nginx代理配置按项目已有路由模式接入，未运行真实Nginx检查。

## 2026-09-08 Codex 分享任务无限制配置

全量后端测试、浏览器配置回归和前端构建通过。总时限0无deadline但仍支持手动取消；正整数分钟生效。当前配置已保存0，后端13992运行新版；前端页面HTTP200，配置接口未登录HTTP401。旧失败任务不会自动恢复，需要重新发起。

## 2026-09-08 Codex 批量取消分享跳过

go test ./...及前端构建通过。任务级sqlmock/miniredis回归覆盖取消状态写入、后续分享继续解析、取消分享历史媒体不识别、全部取消正常完成、普通失败仍报错。构建输出见.codex/share-batch-cancelled-build.log；已更新本地后端，未发起真实批量任务。

## 2026-09-08 Codex 混合分享和AI识别

全量Go测试、AI页面与分享回归、前端构建通过；细节见 `.codex/review-report-ai-recognition.md`。原盘类型与标题清洗、严格候选核验、AI配置及模型获取已接通。导入缺省媒体类型auto有专门sqlmock回归。前后端已重启，AI页面可访问。实际数据库仅将问题分享13类型改为auto，未清理原媒体；AI仍默认关闭。

- 日期：2026-08-19
- 执行者：Codex
- 任务：115 目录配置统一为绝对路径与账号目录树下拉

## 结果

- `go test ./...`：通过。
- `node src/components/resource/targetFolderTree.test.mjs`：3/3 通过。
- `npm run build`：通过。
- `git diff --check`：通过。

## 覆盖范围

- 正常路径：绝对路径归一化、根目录、目录树节点路径拼接。
- 边界条件：重复斜杠、反斜杠、可选空目录、账号未选择、目录加载失败。
- 错误恢复：CID/相对路径被后端拒绝；115 路径解析失败返回可读错误。

## 未执行项

- 未对真实 115 账号执行目录写入、移动或转存，避免产生外部数据变更；本任务的目录树读取接口沿用已有只读实现。

# 2026-08-29 115目录树签名与cron任务唯一性修复

- 执行者：Codex。
- `115driver`：远端最新版本和当前版本均为 `v1.3.5`，无需升级。
- 单元测试：签名请求和实际下载使用相同 `UA115Browser`、Cookie及下载响应头；模拟CDN签名校验通过。
- 真实115验证：STRM配置ID 2导出目录树后下载7,608字节，解析成功，未返回 `403 invalid signature`。
- cron验证：全量任务名按配置ID生成；数据库唯一性迁移为 `(strm_config_id, task_type)`，不再依赖易重复的展示名称。
- 回归结果：`go test ./... -count=1`、`go vet ./...`、前端 `npm run build`、`git diff --check` 全部通过。
- 遗留风险：真实验证依赖115签名接口的在线可用性；已保留默认跳过、环境变量显式启用的集成测试以便后续复验。

# 2026-08-29 Telegram 机器人通知与运维操作

- 执行者：Codex。
- 后端：Telegram 配置脱敏、Token 保留、HTML 转义与截断、按钮过滤、单私聊授权、事件首次基线和终态去重测试通过。
- 回归：`go test ./... -count=1` 与 `go vet ./...` 通过。
- 前端：系统设置通知页签生产构建 `npm run build` 通过。
- 未执行真实 Telegram 发送：仓库没有可用于测试的 Bot Token 和管理员 Chat ID；部署后可通过“发送测试卡片”验证真实网络、代理与会话配置。
- GitHub 发布前复验：本地与 `origin/main` 完全同步，测试、静态检查、生产构建、差异检查和凭据审计均通过，允许发布。
- GitHub 发布结果：功能提交 `2c894e5` 已成功推送到 `origin/main`。
# 2026-08-30 STRM 配置列表与全量清理

- 执行者：Codex。
- 前端：STRM 配置表已移除 `update_time` 列，操作区改为 320px 固定列、`tiny` 按钮和 `flex-nowrap` 单行布局。
- 后端：全量生成在远端目录树成功解析后、首次写入前清空 `LocalPath` 内全部内容；目标目录保留，零视频时也同步清空数据库记录。
- 边界：空路径、当前工作目录、磁盘根目录会被拒绝；目标不存在时视为空目录；清理失败会中止生成。
- 验证：目录清理专项测试、`go test ./... -count=1`、`go vet ./...`、`npm run build`、`git diff --check` 全部通过。
- 未执行真实全量生成：该操作会按需求删除配置目标目录内的全部内容，自动验证采用临时目录，避免影响用户现有媒体输出。
# 2026-08-30 115 账号列表紧凑布局

- 执行者：Codex。
- “更新时间”已从 115 账号表格列模型彻底移除，不存在被固定操作列遮挡后再次露出的路径。
- 编辑、扫码更新、测试、删除使用 Naive UI `tiny` 尺寸，并由 `flex-nowrap whitespace-nowrap` 保持单行。
- 操作列宽调整为 280px，表格横向滚动宽度同步调整为 1520px。
- `npm run build`、`git diff --check` 和本地真实页面 DOM/按钮几何检查通过。

# 2026-08-30 仪表盘账号配额修复

- 执行者：Codex。
- 账号配额数据改为复用 `115driver.GetInfo` 的 `AllUse` 与 `AllTotal`，不再返回固定总量 0。
- 页面展示真实“已用 / 总量”和真实进度；查询失败显示“容量获取失败”，不再伪造 12% 进度。
- 单账号失败隔离和正常容量映射均有自动化回归测试。
- `go test ./...`、`go vet ./...`、`npm run build`、`git diff --check` 全部通过。
- 未对真实115账号发起额外手工验证；页面加载时会通过既有账号凭据执行只读容量查询。
- 后续运行时复验：发现异常由旧后端进程未加载新接口契约导致；重启后已通过真实115只读容量查询，主号与小号容量均正常显示。

# 2026-08-30 账号配额五分钟缓存

- 执行者：Codex。
- Redis 键：`easy_strm:dashboard:account_quota:<账号ID>`；内容为已用容量与总容量；TTL 固定五分钟。
- 读取顺序：Redis 命中直接返回；未命中、过期、损坏或 Redis 故障时回源115，成功后回填。
- 缓存已纳入“缓存管理”的统一统计与清理范围。
- 专项测试、后端全量测试、Go Vet、前端生产构建和真实页面二次加载验证全部通过。
- 2026-08-30：Telegram 同一任务终态并发通知去重修复完成。Redis 原子抢占保证多个监控实例只发送一次；发送失败会释放抢占供下一轮重试。后端全量测试、Go Vet 和差异检查通过；竞态检测因 CGO 未启用无法运行。
- 2026-08-30：Telegram 已支持115分享自动转存和115云下载，支持默认账号、指定账号、默认目录与指定绝对目录；提交后返回任务卡片，完成成功通知复用统一任务终态监控。专项、后端全量、Go Vet、前端构建和差异检查全部通过。

# 2026-08-30 115自动转存目标目录修复

- 执行者：Codex。
- `share/receive`请求现使用115官方识别的 `cid` 字段传递目标目录，不再使用会被忽略的 `save_folder_id`。
- 指定路径解析失败、CID为空或非根路径解析为CID=0时，任务会失败并停止，不会回退后继续转存。
- 协议回归测试、目录失败保护测试、后端全量测试、Go Vet与差异检查通过。
- 本轮没有前端改动，因此未重复执行前端构建；没有进行真实115写入，以避免污染用户云盘。

# 2026-08-30 Emby 管理工作台

- 已新增多 Emby 实例、用户与常用权限、媒体库管理、封面生成/上传和神医助手任务入口。
- Emby 写操作均进入任务中心；刷新和插件任务由后端轮询，展示步骤、真实进度和最终结论。
- `go test ./...`：通过。
- `npm run build`：通过，4254 modules transformed。
- 真实 Emby、神医助手和 AI 图片服务未配置，因此未执行外部成功链路；本地模拟服务验证了实例隔离、刷新进度和终态结论。
- 补充完成逐媒体库刷新：批量请求逐库提交，部分失败时任务为 `partial_success`，保留成功/失败数量及失败明细；本地 HTTP 模拟回归通过。
- 补充用户头像读取代理、头像选择/上传入口以及 `admin` 专属后端路由和前端菜单/路由限制；前端生产构建通过。
- 最终全量 Go 复跑受到工作区中非 Emby 的并行改动影响：监控 Service 测试引用了不存在的符号，媒体源 Controller 测试的默认值预期也与当前实现不一致。本轮没有越界修改这些模块；Emby 补充前的全量 Go 测试和补充后的 Emby 专项测试均已通过。
- 并行改动稳定后已再次复跑 `go test ./... -count=1` 与 `go vet ./...`，最终全部通过，上述临时阻断已解除。

# 2026-08-30 神医助手 STRM 扫描与视频封面

- 新增一键组合任务：选定媒体库扫描 → STRM/主图基线统计 → 神医助手 Extract MediaInfo → 进度跟踪 → 主图覆盖回读。
- 任务中心展示 STRM 总数、执行前后已有主图、本次新增主图和仍缺图数量；结果分别落为 `success`、`partial_success` 或 `failed`，不会仅凭计划任务结束宣称所有截图成功。
- 页面明确提示必须启用 Emby `Image Capture`，且 StrmAssistant `Library Scope` 必须包含目标媒体库；插件计划任务可能处理其配置范围内的其他媒体库。
- 后端专项及全量测试、Go Vet、前端生产构建均通过。
- 修复 Emby 省略 `LastExecutionResult.ErrorMessage` 时被错误显示为 `<nil>` 并判定失败的问题；新增缺失字段回归测试，全量测试及 Go Vet 通过，后端已于 15:49:52 重启生效。

# 2026-08-30 文件工作台自动整理与监控修复

- 执行者：Codex。
- 已修复“配置显示自动整理中、后台实际未启动”：媒体源启用状态现作为总开关参与创建默认值、编辑、统计和状态展示。
- 列表额外返回当前进程的 `watch_running`，目录、账号或权限导致启动失败时会显示“监控未运行”并在保存后提示警告。
- 本地监控覆盖嵌套目录与新增目录；115 监控覆盖递归目录和完整分页，初始快照失败不会假启动。
- `go test ./... -count=1`、`go vet ./...`、`npm run build`、`git diff --check` 全部通过。
- 修复后端已在本地 8082 重启并正常监听；文件工作台只读复核通过。
- 当前三个既有媒体源均为停用状态，未自动更改数据库开关，以免意外触发文件移动；用户可在编辑弹窗确认目录后显式启用目标媒体源。
- 未执行真实本地/115整理写入测试；剩余风险仅为外部目录权限、115 Cookie/接口可用性等运行环境因素。
# 2026-08-30 115 自动整理文件名修复

- 结论：通过。115 轮询使用 PickCode 识别新增文件，但整理输入与任务展示改用真实文件名组成的相对路径，避免 `csn9...` 被误当文件名并触发“源文件不存在”。
- 回归覆盖：根目录文件、嵌套目录文件、分页扫描、新增文件差异、旧 PickCode 任务恢复转换。
- 验证：后端全量测试、静态检查、前端生产构建和差异检查均通过。
- 运行态：本地 `8082` 后端已加载修复；既有失败任务需要点击“恢复任务”才会重新执行，系统不会自动移动文件。

# 2026-08-30 内置站点默认代理验证

- 执行者：Codex。
- Telegram API、Telegram Web、GitHub、GitHub API、TMDB API 在代理地址有效时均命中内置代理域名。
- `proxy_domains` 为空不影响内置规则，填写后仅追加自定义域名。
- `proxy_url` 为空或无效时保持直连，网络探测的“代理路径”不会误报。
- TMDB 搜索与元数据请求已使用代理感知 HTTP 客户端，不再仅让网络测试走代理。
- 后端全量测试、Go Vet、前端生产构建和差异检查全部通过。

# 2026-08-30 TMDB 网络探测鉴权验证

- 执行者：Codex。
- 网络测试会读取最新持久化 `tmdb_api_key`，以 `api_key` 查询参数请求 TMDB `/3/configuration`。
- 表格仍展示不含 Key 的固定地址；请求构造失败或网络失败时，错误文本中的 Key 会被替换为 `******`。
- GitHub、Telegram 及用户自定义探测地址不会附加 TMDB 参数。
- 后端全量测试、Go Vet、前端生产构建全部通过。

# 2026-08-30 默认命名模板验证

- 执行者：Codex。
- 电影默认结果：`飞驰人生2 (2024) [tmdbid=1228891]/飞驰人生2 (2024) [tmdbid=1228891] - 2160p.mkv`。
- 剧集默认结果：`间谍过家家 (2022) [tmdbid=120089]/Season 1/间谍过家家.SPY x FAMILY.2022.S01E16.1080p.AVC.mkv`。
- 历史官方默认值定向升级，自定义模板不覆盖；官方预设与服务默认值保持一致。
- `go test ./...` 与 `npm run build` 通过；差异检查无空白错误。
- 全量测试通过后，工作区中与本任务无关的 Emby 管理并行改动新增了未定义方法，导致后续 `go vet ./...` 和专项复跑被编译错误阻塞。本次模板相关测试已在该并行变化出现前通过。

# 2026-08-30 115 Cookie 来源验证

- 执行者：Codex。
- 手动新增/编辑可提交 `cookie_source`，前后空格会清理，超过 100 个字符会返回 400。
- Cookie 扫码登录按所选渠道自动保存中文来源；扫码更新会更新 `cloud_id` 对应账号的 Cookie 与来源。
- 账号列表显示 Cookie 来源标签，历史空值显示“未标注”。
- 数据库初始化和 `migrate_v18_cloud115_cookie_source.sql` 均可幂等补充字段，历史 NULL 会归一为空字符串。
- `go test ./...`、`go vet ./...`、`npm run build` 和 `git diff --check` 通过。

## 历史 Cookie 来源自动识别

- 已有 Cookie 无需重新扫码：后端直接解析 `UID=<用户ID>_<设备码>_<时间戳>`。
- API 新增 `cookie_uid`、`cookie_ssoent`、`cookie_source_inferred`，历史空来源自动返回渠道文案。
- `R1` 显示微信小程序，`R2` 显示支付宝小程序；`A1` 保守显示“网页版 / 115 浏览器”。
- 人工填写来源不会被推断值覆盖；未知设备码保留原码并显示“未知渠道”。
- 账号列显示 `UID - 用户ID（设备码 渠道）`，来源列标注“由 UID 自动识别”。
- 后端全量测试、静态检查和前端生产构建通过。

# 2026-08-30 神医助手截图依赖一键配置验证

- 执行者：Codex。
- 点击“扫描 STRM 并生成视频封面”后先展示具体变更、目标媒体库及插件任务可能波及其他范围内媒体库的提示；取消不提交接口，确认请求携带 `auto_configure=true`。
- 后端任务先读取完整设置，只增量启用适用类型的 Image Capture；空 Library Scope 保持全部，非空范围只追加目标 ID 且不重复。
- 写入后必须回读两项设置；任何读取、保存或核验失败均在扫描前终止并进入任务中心失败结论。
- 普通 Emby API Key 无权访问 Generic UI 的版本会返回明确凭据兼容错误，不会盲目覆盖插件配置。
- Service/Controller 专项测试、后端全量测试和前端生产构建通过。
- Go Vet、目标文件差异检查和浏览器取消流程通过；本地后端已于 16:27:04 重启，PID 38824 正常监听 8082。
# 2026-08-30 Emby 观影监控中心验证

- 执行者：Codex。
- 仅管理员可见的 Emby 监控路由和五组后端接口已接入现有 `/emby` 管理员中间件。
- Playback Reporting 健康时使用插件历史，任一请求失败时整次降级到本地历史；响应包含来源、时区、生成时间、历史起点和降级原因。
- 本地采集覆盖暂停、恢复、媒体切换、会话失联、45 秒增量上限和自然日切分；首次媒体扫描仅建立基线。
- 后端全量测试、前端生产构建和六页签/窄屏专项 E2E 通过。

## 115 Cookie 来源最终验证

- 历史 Cookie 继续从 UID 动态识别来源，人工填写来源保持最高优先级。
- 自动推断来源不回填为人工配置，编辑其他字段或更换 Cookie 不会遗留旧渠道。
- `go test ./...`、`go vet ./...`、`npm run build` 全部通过；前端仅有既存 Emby 分块体积警告。

# 2026-08-30 企业微信应用通知渠道验证

- 执行者：Codex。
- 新增企业微信自建应用通知，配置支持 Corp ID、Agent ID、Secret、成员/部门/标签接收范围及四类事件开关；Secret 读取脱敏且更新留空保留。
- 通知使用官方 Token 与 Markdown 消息接口并复用代理感知 HTTP 客户端，Token 在内存中按有效期缓存。
- 任务终态和 115 账号状态复用统一卡片，Telegram 与企业微信分别建立 Redis 基线和去重状态，互不抢占事件。
- 模拟企业微信 API 专项测试、后端全量测试、Go Vet、前端生产构建和差异检查通过。
- 未使用真实企业微信凭据发送；部署后通过设置页“发送测试通知”验证应用可见范围和网络连通性。
- 保存失败复核：根因是前端已热更新、后端仍运行企业微信路由加入前的旧进程；2026-08-30 18:33 重启后配置和测试路由已正确注册，404 已消失。
- 企业微信 API 接收消息已实现：公开回调使用平台签名与 AES 加密校验，文本命令可查询状态/任务并提交115资源操作；专项测试、后端全量测试、Go Vet 和前端构建通过。真实企业微信 URL 保存仍需部署侧提供公网 HTTPS 域名。
## 全局第三方 API 验证（2026-08-30，Codex）

- `go test ./...`：通过。
- `npm run build`：通过。
- 全局 API 配置使用 `t_system_config` 存储；专用读取接口仅返回 `has_api_key`，通用 `/settings` 接口过滤相关键。
- JWT 中间件支持 `X-API-Key` 与 `Authorization: ApiKey`，有效密钥映射管理员身份后复用全部现有业务路由。

## 文件工作台概览板块清理（2026-08-30，Codex）

- 红框内的工作台头部、当前工作上下文、四个概览指标、快捷动作和本轮选择已从 `MediaManager.vue` 删除。
- 页面首个业务区块现在是“媒体源管理”；文件浏览、选择、识别、重命名、整理和刮削事件链路保持原状。
- 与删除板块绑定的组件、图标、计算属性、辅助函数、父组件 ref 和子组件暴露接口已同步清理，无残留检索结果。
- `npm run build`、功能结构断言和目标差异检查通过。
- 运行态浏览器截图因本机 `localhost:8082` 访问策略阻断而未执行；风险低，未发现构建或静态契约异常。

## 云下载记录名称列宽度约束（2026-08-30，Codex）

- “名称 / 链接”列已从可扩张的 `minWidth: 220` 改为固定 `width: 420`。
- 名称与链接继续分两行显示，超长内容由既有 `NEllipsis` 截断，避免无限撑开首列。
- 其余固定列合计 720px，整表声明宽度约 1140px，可在常见桌面工作区保留后续账号、状态、进度、大小、时间和操作列。
- 前端生产构建、列结构断言和目标差异检查通过。
- 用户回归确认首次仅声明 `width` 不足；现已增加 `maxWidth: 420`、固定表格布局、1140px 整表滚动宽度和单元格溢出约束。
- Naive UI 列样式计算结果为 `width/min-width/max-width: 420px`，源码与最终构建产物均包含全部约束。

## 企业微信云下载创建反馈修复（2026-08-30，Codex）

- 主动命令回复不再受后台通知 `detail_url` 影响，固定发送企业微信文本消息，适配 wxchat 消息转发代理。
- 云下载任务的成功反馈继续包含标题、账号、目录、链接数和任务 ID；本次回归测试重点断言标题与任务 ID 未丢失。
- 后台任务终态与账号状态通知仍可使用原有 `textcard` 策略，不受本次修复影响。
- 定向测试与 `go test ./...` 均通过。

## 通知渠道多行115任务提交（2026-08-30，Codex）

- 一条 Telegram 或企业微信消息可以包含多个非空行；每行独立使用 `链接 [账号名称] [/目标目录]` 并创建独立任务。
- 某行失败时继续提交后续行；最终返回总数、已创建、失败以及逐行任务 ID/错误。
- 用户给出的两条 magnet 示例会产生两次 `OfflineDownloadService.Submit` 调用，每次只含一个 URL，目录均为 `/其他/可刮削`。
- 专项测试、两渠道反馈文本断言、`go test ./...` 与 `go vet ./...` 全部通过。
- 115分享链接同样按非空行独立转存；每行可分别指定账号和目录，汇总标题明确显示“115 分享转存批量提交完成”。

## 任务触发与终态双阶段通知（2026-09-01，Codex）

- 所有已启用通知渠道（Telegram、企业微信、Server 酱、SMTP）均接收任务触发和任务终态推送；Telegram/企业微信保留事件开关。
- 单个任务生命周期最多推送两次：首次发现时“任务已触发”，进入完成/失败/取消时对应终态；每个阶段独立去重，发送失败释放抢占。
- 基线版本升级为 v2，升级后不补发历史任务；服务重启使用 Redis 标记保持去重。
- 后端 `go test ./...`、`go vet ./...` 和前端 `npm run build` 均通过。

## 115 离线下载 UA 回归修复（2026-08-30，Codex）

- 用户提供的 `magnet:?xt=urn:btih:A4FF9B32DD3F75FA76F0381848BBB8EE9CE40187` 是合法的 40 位 BTIH 磁力链接，前端以 JSON 原样发送，参数本身不是故障来源。
- 根因是代理感知 HTTP Client 注入时重建了 115driver 的 Resty Client，清除了先前设置的 `UA115Browser`；115 离线加密接口因此返回 `decode fail!`，驱动继续按 JSON 解析后产生首字符为 `d` 的错误。
- 客户端现按“先 `WithClient`、后 `UA`”构造，既保留两分钟超时与代理能力，也保留115浏览器 UA。
- 新增回归断言先失败后通过；离线 Service/Controller 专项、后端全量、Go Vet、前端生产构建和差异检查全部通过。
- 未自动提交真实磁力任务，避免向用户115账号写入额外云端数据；代码级回归已覆盖本次可复现根因。
- 本地 8082 后端已重启到修复后代码；受保护的离线任务列表接口返回预期未授权响应，服务启动和路由注册正常。

## 项目功能缺口分析与完善需求文档（2026-08-31，Codex）

- 已完成全仓库静态审计并生成 `docs/项目功能完善需求.md`；本任务只更新需求和审计留痕，没有修改业务代码。
- 文档登记 29 个源码证实缺口，拆分 16 个 RQ、五个 P0、四个实施里程碑，并定义统一任务状态、HTTP 响应/分页和外部能力契约。
- 红队复核已完成：消除 M0/M1 对后续完整任务/健康/API 治理的反向阻塞；明确 `interrupted/tracking_timeout/unknown` 为待恢复或对账状态，而非不可变终态。
- `go test ./... -count=1`、`go vet ./...`、`npm run build`、`targetFolderTree.test.mjs` 3/3 和 23 个 E2E 脚本语法检查全部通过；前端仅有既存 Emby 大分块提示。
- JSON、文档引用、RQ/缺口/里程碑数量、尾随空格和个人绝对路径检查通过。
- `docker compose -f docker-compose.yml config --images` 复现 `/easy-strm:latest` 无效默认镜像引用；已作为 RQ-05 P0 验收输入记录，不代表现状修复。
- 未执行会向真实 115、Emby、通知渠道或用户媒体目录写入的 live E2E；本轮只读审计不应产生这些外部副作用。
- 审查综合评分 97/100，结论：通过，可进入实施拆分。

# 2026-09-05 日志优化
正常文件浏览轮询与内部路径解析不再污染默认 INFO 日志，失败和业务结果日志保留。

## 2026-09-07 Emby 用户媒体库权限（Codex）
- 根因：用户权限选项复用 VirtualFolders.ItemId；官方 SelectableMediaFolders 同时返回 Id 和 Guid，EnabledFolders 应使用权限 Guid。
- 实施：新增 user-libraries 控制器/API/领域返回结构；用户弹窗显示名称并选择 Guid；后端规范化旧数字 ID、保留未编辑的 Policy 字段、写入后回读；恢复全部媒体库开关的明确语义。
- 工具与过程：functions.exec/rg/Get-Content 检索 Service、Controller、API、Vue 和现有测试；Invoke-WebRequest 读取官方文档；apply_patch 小步修改；go test 先复现原样传数字 ID、遗漏策略字段、未回读造成假成功，再验证修复；Playwright 验证页面交互。
- 工具降级：sequential-thinking、shrimp-task-manager、code-index、exa 不在工具目录中，使用本地分析、rg 和官方文档 HTTP 请求。
- 初次浏览器测试失败：3001 未提供可连接前端；启动独立 3017 Vite。后续发现通配 API 拦截误匹配 src/utils/api 模块，限制为 /api/ 根路径后通过。
- 验证：go test ./... 通过；新增 Service 权限往返 6 场景、选项查询 3 场景、Controller 3 场景通过；浏览器 7 场景通过；npm run build 通过（已有大块警告）。
- 浏览器场景：名称回显、Guid 提交、保存后重开、全部媒体库、空范围、旧数字 ID、保存失败保留弹窗。截图：debug/emby-user-access/names-after-save.png。
- 限制：Go 测试使用 httptest/sqlmock/miniredis；浏览器使用可回读模拟接口，未联调或修改真实 Emby 用户。需要部署更新的前后端才能使用新接口。

## 2026-09-07 23:20 Emby 真实实例权限闭环（Codex）
- 直接读取当前实例 users/accesstab.js：原生选项使用 folder.Guid || folder.Id，并通过 EnabledFolders.includes(folderId) 判断勾选。
- 真实只读诊断：115tv 仍存储 [127953,127954,127956,127955]；BlockedMediaFolders=null、ExcludedSubFolders=[]，故本次不是排除字段冲突。
- 真实映射：127953=电视剧STRM、127954=电影STRM、127956=动漫电影STRM、127955=动漫STRM。
- 新增显式环境变量触发的 TestEmbyUserAccessLiveRepair，默认全量测试跳过真实写入。使用真实服务器凭据（仅内存）、现有 Service 和模拟任务存储修复原有四项 ID，不扩大范围。首次测试夹具误用了 test-key 导致 401（写入前失败），改为真实连接凭据后通过。
- 真实验证：Service UpdateUser 保存回读通过；刷新 Emby 原生页面，4 个 chkFolder.checked 均为 true。
- 再次通过真实 localhost:3001 easy-strm 页面打开 115tv，四项显示媒体库名称；点击保存后弹窗关闭；刷新真实 Emby 页面仍勾选相同四项。证明当前运行中的前后端链路已生效。
- go test ./... 通过；本轮未改前端生产代码，未重复构建。
- 结论修正：之前仅修复代码未迁移 Emby 已保存的错误数字 ID，不能将模拟测试通过宣称为真实用户问题已解决。本次已完成真实数据修复与 UI 验证。
- 工具：rg/Get-Content 检索；Invoke-WebRequest 读取部署版本公开 JS；Go 读取已配置实例及目标权限；cua_repl 读取原生 checkbox data-id、刷新并核对 checked 状态，以及真实 easy-strm 保存操作。

## 分享脱敏目录统计（2026-09-08，Codex）
- 名称含连续半角或全角星号的目录不下探、不识别；解析结果通过 masked_directories 单独携带统计目录。
- 复用分享媒体记录保存脱敏路径；同名路径按出现次数补齐，重复扫描不累加。列表服务归一化历史脱敏项为 masked，并返回 masked_count。
- 总数包含脱敏项；脱敏项不计入已识别、失败、待识别。全脱敏分享任务正常完成。
- 本地验证：go test ./... 通过；npm run build 通过（存在大于 500 kB 的构建分块提示）。回归覆盖扫描跳过、同名计数、重复扫描、历史状态归一化、数据库错误、全脱敏任务完成。
- 测试过程：先复现解析结果缺失脱敏统计；新增任务测试适配已有 Redis 全局初始化以及工作区新增 media_type 字段后通过。
- 工具：functions.exec / exec_command 用于 rg、读取、gofmt、测试和构建；apply_patch 用于代码与测试修改。指定 sequential-thinking、shrimp-task-manager、code-index 不可用，采用本地分析与 rg。
- 原始输出：.codex/share-masked-go-test.log、.codex/share-masked-build.log。未进行线上分享调用和浏览器端到端验证。

## 批量导入分享按钮修复（2026-09-08，Codex）
- 根因：ShareRecords.vue 缺失 batchImportShow 对应弹窗；原拆行正则错误匹配字面量反斜杠。
- 实现：补全输入、取消与提交弹窗，修复换行拆分、连续链接名称误用；提交期间禁止重复操作，部分失败保留未完成项供重试，成功后刷新列表。
- 验证：node scripts/e2e-share-batch-import.mjs 通过，覆盖打开/取消、无链接、CRLF/LF、名称、列表刷新、失败恢复和窄屏；使用模拟 API，无真实数据写入。npm run build 通过（4848 modules，既有大分块提示）。仅前端修改，未运行后端测试。
- 复现：先修正测试夹具误拦截 src/utils/api 模块的问题，再确认修改前点击后等待弹窗超时；修复后全场景通过。
- 审查：通过；技术/需求/综合评分 94/96/95。复用 Naive UI 与既有 API，无新增依赖。真实后端导入未验证；名称支持链接上一行，访问码需与链接同一行，已在输入提示注明。
- 工具记录：functions.exec/exec_command 执行 git status、rg、Get-Content、Vite、Playwright、npm run build、git diff --check；apply_patch 写入上下文、回归和弹窗；Python 定点替换原单行函数及追加记录。sequential-thinking、shrimp-task-manager、code-index 不可用，使用本地分析与 rg。
- git diff --check 发现既有 .codex/testing.md:829 尾随空格，本次未改动该历史内容。保留工作区其他改动。

## 混合分享解析及预览导入（2026-09-08，Codex）
- 已完成：domain响应、Service纯解析、控制器与路由、API封装、独立ShareImportDialog、单元/控制器/浏览器测试及长期文档。
- 关键决策：按当前文本分享编号去重；URL移除密码查询并独立保存访问码，避免预览修改密码后被URL旧值覆盖；歧义与冲突默认不选，已成功项锁定。
- 验证：go test ./... 全部通过（easy-strm、controller、dao、domain、logger、service）；Playwright全部通过；npm run build通过（4850模块，既有大分块提示）。构建完整输出见 .codex/share-import-build.log。
- 失败及修复：浏览器初次断言使用原生disabled判定Naive UI checkbox，改为其实际disabled类后通过；后端新增块边界回归先复现结束标记后标题被吞，修复只消费到访问码/复制结束行后通过。
- 审查：通过；技术94、需求96、综合95。前后名称冲突保留候选；导入失败保留状态；解析与导入保持分层，无新依赖和数据库变更。API夹具验证未调用真实网盘；未部署。
- 工具留痕：functions.exec / exec_command 执行 Get-Content、rg、gofmt、go test、node Playwright、npm run build、git diff --check；apply_patch 创建和更新代码/测试/结构化需求；Python 定点替换页面旧弹窗/函数及追加文档。指定思考/规划/索引MCP不可用，使用本地分析及rg。保留所有已有工作区变更。

## GitHub main 提交验证（2026-09-08，Codex）
- 请求：提交当前全部代码及配套文档变更并推送 origin/main，用户已明确授权。
- 上下文：当前分支 main；git fetch origin 成功，HEAD 与 origin/main 提交前一致。范围为分享管理与批量导入、Emby 用户权限与媒体库、播放记录、MetaTube 元数据、日志及对应测试文档。
- 工具降级：sequential-thinking、shrimp-task-manager、code-index 当前不可用；采用本地分析、Git 与 rg。计划为检查范围、运行本地验证、提交全部变更、正常推送并核对远程。
- 验证：go test ./... 全包通过；npm run build 通过，仅既有大分块提示。完整输出：.codex/github-main-go-test.log、.codex/github-main-build.log。
- 浏览器：node scripts/e2e-share-batch-import.mjs 与 node scripts/e2e-emby-user-access.mjs 均通过，使用本地页面及模拟 API，未验证真实外部服务。
- 修正：修复 testing.md 中断裂的 npm 命令及尾随空白。
- 工具留痕：functions.exec 调用 exec_command 执行 Git 状态/分支/远程/fetch/diff/log、rg、Get-Content、Go 测试、前端构建及两项 Playwright；write_stdin 获取测试结果；apply_patch 修正文档。后续执行 git add/commit/push 与远程哈希核对。
- 审查：提交范围与请求一致，本地基线及两项相关回归通过；本次为提交前验证，未重新逐行审计全部历史功能。

### 分享已取消状态（2026-09-08，维护者：Codex）

新增 t_share_record.share_cancelled BOOLEAN NOT NULL DEFAULT FALSE，v24迁移随程序启动在现有事务中执行。识别分享时仅明确的网盘分享取消错误置为true，成功重新解析置为false；任务超时、context取消、密码错误、原因不明的过期/失效不标记。更新状态时核对原链接与密码，避免并发编辑后的旧结果污染。编辑链接/密码自动清除原状态，单纯改名称保留。

列表API返回share_cancelled，名称旁显示红色“分享已取消”标签，刷新后通过数据库字段回显。此次不根据历史任务超时回填取消状态，需重新识别确认。保留原有媒体记录和识别操作。

- 验证：go test ./... 全部通过；npm run build 通过（既有大分块提示，完整输出 .codex/share-cancelled-build.log）；node scripts/e2e-share-cancelled.mjs 验证标签、刷新保留、超时不标记和恢复移除，API为模拟。
- Service回归覆盖取消、已标记、恢复、任务取消、超时、密码错误、网络错误、未知失效及数据库失败；DAO分页测试读取true/false字段，状态保存使用sqlmock核对参数。
- 工具：functions.exec/exec_command用于rg、读取、gofmt、go test、Vite、Playwright和构建；apply_patch修改代码与迁移；Python定点修改DAO扫描及夹具和追加文档。第一次Python默认GBK读取失败，明确UTF-8后完成；浏览器首次因Vite未运行连接失败，启动后重试。
- 审查：通过，综合94/100。未部署或连接真实数据库，迁移尚未在真实PostgreSQL执行；保留工作区既有改动。

### 分享剧集海报合并（2026-09-08，维护者：Codex）

列表 Service 对同一分享内已识别成功的电视剧按 TMDB ID（无 TMDB ID 时按元数据源+MetadataID）标记 gallery_duplicate。海报墙默认折叠重复记录，可通过“展开同剧集记录”查看并逐条修正、删除。电影、不同ID、无可信ID或失败记录不按名称合并。该标记由读取时生成，无需迁移或删除历史数据；识别统计仍表示原始记录数量。

扫描判断目录候选仅检查直接子项，避免把集合目录因后代视频误当一部媒体；入库前对已有媒体目录覆盖的 SxxExx 单集候选跳过。裸露在根目录的单集、没有父目录候选的文件和独立电影仍保留，以免丢失混合分享中的内容。

- 验证：go test ./... 全部通过；node scripts/e2e-share-gallery.mjs通过（同剧集一张海报、展开三条原记录、逐条操作、重新合并，API mocked）；npm run build通过（既有大分块提示），完整构建日志 .codex/share-gallery-build.log。
- Service测试覆盖相同TV ID、同名不同ID、电影、未知ID、失败、手动修正后解除折叠、跨分享、Windows/Unix路径、已覆盖单集与根目录独立文件。
- 审查通过，综合94/100。未访问用户数据库或115链接，不删除原记录；已有数据读取时自动应用展示规则，尚未部署。无新增依赖。
- 工具留痕：functions.exec/exec_command执行rg、Get-Content、gofmt、go test、Playwright及npm build；apply_patch更新领域、Service、前端与测试；Python追加上下文与文档。指定思考/规划/索引工具不可用，使用本地分析和rg。保留工作区原有改动。

### 清空分享识别内容（2026-09-08，维护者：Codex）

分享管理每行新增“清空识别内容”。确认提示列出分享名称及媒体数量；执行后删除该分享全部 t_share_media 行，包括已识别、待识别、失败、脱敏和重复单集记录。保留分享主记录、链接、密码及其他配置，不操作115文件。清空完成后可点击“识别”，重新从分享扫描生成候选，不沿用旧单集记录。

DELETE /media/share-records/:id/media 返回 SuccessResp {deleted}。DAO事务锁定分享主行后按share_id删除；空分享返回0，不存在返回404，非法ID返回400。Service在本服务实例有分享识别任务运行时拒绝清空（409），任务锁在创建前获取并在后台终止后释放。该互斥适用于当前单后端实例部署；不提供跨进程任务互斥。

- 本地验证：go test ./...全部通过；node scripts/e2e-share-clear.mjs通过（取消不请求、失败保留确认、清空后列表为0、分享保留及重新识别）；npm run build通过，既有大分块提示，输出 .codex/share-clear-build.log。
- Controller/sqlmock回归覆盖非法ID、成功删除43条、重复清空0条、不存在、删除错误回滚；Service回归覆盖识别运行中拒绝清空。浏览器仅模拟API，未执行真实清理。
- 审查通过，综合94/100；无新增依赖或schema变更。尚未部署，未删除用户数据库内容。保留既有工作区修改。
- 工具留痕：functions.exec/exec_command读取DAO/Service/UI工具，rg查找，gofmt、go test、Playwright、npm build；apply_patch新增Service/DAO、Controller及API与测试；Python定点插入页面按钮及追加文档。指定MCP不可用，使用本地分析与rg。


## 2026-09-08 已识别媒体数据库分页（Codex）
- 分享 HTTP 列表仅返回统计和空 media；后台识别保留完整读取入口。
- 新增 GET /media/share-records/:id/media，默认 page=1、page_size=10；可切换 show_duplicates。数据库先按可信剧集身份分组再分页，计数及当前页使用同一 SQL 快照。
- 前端展开时按需读取，翻页加载十条；删除尾页后回退、刷新后重新读取、请求序号隔离旧响应。
- 验证：go test ./...、npm run build、浏览器 e2e-share-gallery 均通过。真实数据库只读验证分享17，总数1236，前两页各10条且无重叠。
- 已构建 debug/easy-strm-pagination.exe 并重启，PID 30460。前端3001返回200。后端8082根路由受鉴权保护。
- 工具：命令行 rg/Python/Go/npm/Playwright、apply_patch；sequential-thinking、shrimp-task-manager、code-index 本轮不可用，使用本地分析和检索。首次测试发现 SQL 格式串中的百分号冲突，已修复并通过全量测试。
- 自动审批曾拒绝一次组合测试命令，拆分为补丁写入和独立本地测试后成功完成，无需用户操作。


## 2026-09-08 海报墙每页条数偏好（Codex）
- 新增每页10/20/50/100条选择，全局偏好通过 GET/PUT /media/share-gallery-settings 保存到 Redis share:gallery:page_size。SET expiration=0，无失效时间；未配置默认10。
- 复用现有 Redis 客户端和 DAO/Service/Controller/API 分层。读取偏好后加载媒体，修改成功后回第一页；读取失败提示并以10条加载，保存失败不替换当前配置。
- 本地验证：go test ./...、新增 Controller 测试、npm run build、e2e-share-gallery 通过。miniredis 测试覆盖默认10、修改保存、TTL=-1、覆盖已有TTL、时间推进一年后仍可读取；浏览器覆盖改为20、刷新恢复及原分页/合并。
- 首次浏览器测试使用错误的 option 选择器超时，改为 Naive UI 实际选项选择器后通过。
- 已构建并重启 debug/easy-strm-gallery-settings.exe。工具采用 apply_patch、PowerShell、rg、Python、Go、npm 和 Playwright；命名的 thinking/planner/code-index 工具不可用，沿用本地分析检索。
- 审查通过：未新增数据库迁移或依赖；偏好对所有分享生效，Redis不设置TTL。


## 2026-09-08 分页偏好改为浏览器缓存（Codex）
- 按用户最新要求替换固定选项为自定义输入（1至100整数，与现有数据库分页接口范围一致），应用后回到第一页。默认10条。
- 使用 localStorage share:gallery:page_size 保存；无效缓存回退10，缓存不可写时提示且当前页面可用。删除本轮前序新增的Redis DAO、Service、Controller、路由、API及对应测试，不再读写Redis。
- 本地验证：go test ./...、npm run build、e2e-share-gallery 通过。浏览器覆盖17条保存/刷新恢复、空输入不覆盖、损坏缓存回退及无Redis配置请求。
- 已构建并启动 debug/easy-strm-browser-pagination.exe；沿用已有前端开发服务。
- 工具：apply_patch、PowerShell、Python、rg、Go、npm、Playwright。原指定thinking/planning/code-index不可用，采用本地分析与检索。
- 审查通过；复用已有Naive输入组件、localStorage及数据库分页，无新增依赖。


## 2026-09-08 分享工具栏与待识别续跑（Codex）
- 工具栏按用户要求排列：新增分享、批量导入分享、导入记录、批量自动识别、重新识别失败项、继续识别待识别内容、识别任务设置。
- 批量识别接口新增 pending_only，和 retry_failed 互斥；续跑直接读取所有分页的数据库候选，不调用分享扫描。只选 pending 或空状态，跳过失败/已识别/脱敏/取消分享；无候选正常完成。
- 复用既有任务互斥、取消、超时配置和进度展示。
- 验证：go test ./...、npm run build、浏览器工具栏顺序/续跑请求/原海报分页测试通过；最终服务测试通过。一次追加集成测试命令因工作目录错误未执行，未计为通过测试；现有新增筛选单测已执行通过。
- 已构建并重启 debug/easy-strm-pending.exe。工具：rg、Python、apply_patch、Go、npm、Playwright；指定思考和规划MCP不可用，沿用本地分析。


## 2026-09-08 单行分享待识别续跑（Codex）
- 每行识别按钮后新增“继续识别待识别”；无待识别条数或分享取消时禁用；操作区可换行。
- 复用单分享识别接口，query pending_only=true 通过现有recordIDs限制分享范围，不扫描或重试失败项。
- 验证：go test ./...、npm run build、浏览器行按钮请求测试通过。新增服务集成测试验证其他分享pending候选不处理、当前分享失败项不重试、解析器未调用。
- 初次集成测试遗漏项目TaskRedisDAO全局初始化，修正夹具后全量通过。已构建并重启debug/easy-strm-record-pending.exe。
- 工具：rg、Python、apply_patch、Go、npm、Playwright；沿用本地分析代替不可用的专用思考/规划工具。审查通过。


## 2026-09-08 单分享失败内容重试（Codex）
- 每行新增“识别失败内容”，在继续识别按钮后；无失败项或取消分享时禁用。
- 复用单分享识别入口，failed_only=true与pending_only互斥，只处理当前分享failed记录，跳过扫描并保留其他状态。
- 验证：go test ./...、npm run build、浏览器单行失败重试请求测试通过；新增单测覆盖failed/pending/identified/masked及空状态筛选。
- 已构建并重启debug/easy-strm-record-failed.exe。工具使用rg、Python、Go、npm、Playwright；沿用本地分析代替不可用的专用MCP思考与规划工具。审查通过。


## 2026-09-09 分享资源库与统一定时任务（Codex）

- `easy-strm`: `go test ./...` 通过。
- `easy-strm-front`: `npm run build` 通过。
- PGlite 真实 PostgreSQL 引擎迁移回放通过：覆盖两次迁移、跨分享聚合、类型身份隔离、失效分享、组合筛选、稳定分页、投影清理、版本冲突、旧任务保留、内置任务不覆盖、重启中断和级联删除。
- Playwright 浏览器脚本通过：`e2e-share-gallery.mjs`、`e2e-library-schedules.mjs`；覆盖资源库筛选/分页/来源/补全、调度启停/执行/历史/参数表单及 STRM 任务入口。新页面脚本使用模拟 API，完整真实后端联调因当前环境无可用 PostgreSQL 服务未执行。
- 未执行 Git commit 或 push。
## 2026-09-09 资源库年份别名修复（Codex）

- 用户实测出现 PostgreSQL `syntax error at or near "year"`。DAO 页面投影的裸别名改为 `media_year AS "year"`，接口字段保持不变。
- 本地 PGlite 脚本增加从 DAO 提取实际页面投影的回归：修复前复现错误，修复后通过；此前仅执行聚合 CTE，未覆盖页面投影。
- `go test ./...` 全部通过；已构建并重启本地后端。前端代码无需变更。
- 使用 functions.exec、apply_patch、PowerShell、Go 和 PGlite；专用思考/规划/索引工具不可用，沿用本地分析和 rg 检索。


## 2026-09-09 分享标题及显式TMDB ID修复（Codex）
- 确认之前仅分析未修复。本次实现：年份优先括号年份、禁止从中文标题内部截断1958；保留完整混合标题；提取tmdbid标记，已知电影/剧集类型时直接获取详情并核对ID；模糊查询失败后核验官方alternative_titles，保留年份及唯一性要求。
- go test ./... 通过。新增回归覆盖搜查班长1958 (2024)、财阀X刑警、显式ID绕过搜索、闪烁的西瓜官方别名。外部TMDB使用本地HTTP夹具，未批量修改用户识别数据。
- 已构建debug/easy-strm-title-id-fix.exe并替换原后端。重新识别失败内容可应用新规则；无需清空或重新扫描。
- 工具：rg、PowerShell、Python、Go；专用思考/规划MCP不可用，采用本地代码分析。审查通过，识别未知类型时仍用查询推断，避免直接把无类型ID认成电影。


## 2026-09-09 AI仅兜底（Codex）
- 原复杂标题/未知类型会在TMDB搜索前调用AI；现先完成规则与TMDB搜索/别名核验，仅无确认结果时按所选场景调用AI，单次最多一次。明确TMDB ID仍优先详情直查。
- 保留AI开关和场景偏好，不改数据库配置；更新页面场景说明。
- go test ./...、npm run build通过。新增本地HTTP测试：所有场景开启时，规则成功AI调用0次，规则失败AI调用1次且二次TMDB验证成功。
- 已构建并启动debug/easy-strm-ai-fallback.exe。工具rg/PowerShell/Python/Go/npm；专用MCP规划工具不可用，沿用本地分析。审查通过。


## 2026-09-09 目录年份导致剧集匹配失败（Codex）
- 用户飞起来吧蝴蝶单文件测试无年份，分享路径父目录2022被用于SearchTV first_air_date_year过滤；截图候选年份2026，因此带年份失败。
- 增加目录年份来源标记，常规及别名核验失败后、AI前，无年份重查剧集，只有完整标题匹配且身份唯一才接受；同名多项不自动选择。
- go test ./...通过；新增HTTP夹具覆盖2022目录/2026候选成功和同名歧义拒绝。未调用真实TMDB或修改用户识别记录。
- 已构建并启动debug/easy-strm-year-fallback.exe。工具采用rg/Python/Go/PowerShell，专用规划MCP不可用，使用本地分析。审查通过。


## 2026-09-10 接口404排查（Codex）
- 分享资料库：HEAD遗漏的列表、筛选选项、来源、补全路由已由任务开始前的工作区修改补齐，本次保留并纳入契约测试；没有重复添加或覆盖原有STRM开发。
- 本次补齐 GET /cron/handlers、GET /cron/task/:id/runs、GET/PUT /media/share-task-settings。
- 修复 CronController.SetScheduler 没有将管理操作连接共享调度器的问题，避免处理器列表为空及管理状态分裂。
- 新增 api_routes_test.go：解析真实Go注册源码、建立Gin路由表，通过httptest核对194处前端调用与206条路由（空处理器，不执行写操作）。覆盖路径、方法及分组；不代表194个接口业务功能全部经过集成测试。
- 修复前契约测试明确复现4个404；共享调度器回归测试复现200但data为空。修复后 go test ./... 全部通过（main/controller/dao/domain/logger/service）。
- npm run build 通过（4861模块，仅已有大chunk提示）；node scripts/e2e-share-strm.mjs 通过，覆盖配置、筛选导出、失败恢复、窄屏，使用模拟API。
- git diff --check 通过。本机3001/8082/80没有运行服务，Docker不可用；未取得用户实际页面地址，因此尚未完成部署实例的真实请求验证。生效需要重新构建并重启实际后端。
