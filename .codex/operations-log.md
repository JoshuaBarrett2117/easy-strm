# Operations Log

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
