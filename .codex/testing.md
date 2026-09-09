# 2026-08-18 云下载大批量队列提交

## 2026-09-08 Codex 分享任务时限验证

go test ./...全部通过。新增时限上下文及配置Service/Controller测试；node scripts/e2e-share-task-settings.mjs通过，覆盖默认无限制、90分钟保存重开、切回无限制和刷新回显。npm run build通过，详见.codex/share-task-timeout-build.log。无限制通过无deadline及主动取消断言验证，无需真实等待30分钟。

## 2026-09-08 Codex 批量识别跳过取消分享

新增TestCancelledShareDoesNotFailBatch，先复现后修复，3个场景通过；Go全量测试与前端构建通过。验证依赖sqlmock和miniredis，无真实网盘读写。取消计数通过任务metadata在页面展示，历史媒体不进入识别队列。

## 2026-09-08 Codex AI与混合识别验证

- `go test ./...` 全部通过；额外 `TestImportedShareDefaultsToAuto` 通过，确认未传类型以auto入库。
- `node scripts/e2e-ai-recognition.mjs`、`e2e-share-gallery.mjs`、`e2e-share-clear.mjs` 通过，API为模拟端点。
- `npm run build` 通过，保留已有大分块提示；日志 `.codex/ai-recognition-build.log`。
- 实际服务已重启；v25迁移在当前PostgreSQL执行，验证默认值为auto；AI页面HTTP200。未调用真实AI供应商或运行收费请求。

- 后端全量：`go test ./... -count=1`，通过。
- 后端专项：`go test ./internal/service -run 'TestOfflineDownloadSubmit(LargeBatchQueuesAndChunks|HappyPathTracksToCompleted|BatchFatalErrorNormalized)$' -count=1 -v`，通过。
- 专项断言：201 条输入立即返回 `queued=true`，后台实际调用批次大小为 `[100 100 1]`。
- 前端生产构建：`npm run build`，通过，4248 modules transformed。
- 格式检查：`git diff --check`，通过。
- 竞态检测：`go test -race ./internal/service -run TestOfflineDownloadSubmitLargeBatchQueuesAndChunks -count=1` 未执行，当前 Go 环境未启用 CGO（`-race requires cgo`）；常规专项与全量测试均已通过。
- 未执行真实115大批量请求，避免向用户账号实际创建201个下载任务；115客户端调用边界由注入桩自动验证。

# 2026-07-17 亮色主题与全模块测试

- 前端构建：通过，4231 modules transformed。
- 后端测试：`go test ./...`、`go vet ./...` 通过。
- Docker：PostgreSQL、Redis 健康，主应用运行于 `:8082`。
- E2E 静态检查：23 个 `e2e-*.mjs` 全部通过 `node --check`。
- API 基线：登录后 20+ 个模块查询接口返回正常 JSON。
- 核心闭环：媒体源创建 -> 全量同步 -> 索引 -> 资产台账 -> 任务 -> 删除清理，通过。
- 文件操作：浏览、搜索、重命名、复制、移动、删除，通过。
- 待处理：创建、识别、重新入库、清理，通过。
- 配置操作：分类 CRUD、日志配置更新恢复、网络探针、缓存定向清理，通过。
- 缺陷复现：全新数据库 `/notify/config` 返回 500，缺少 `t_notification_config`。
- 缺陷复测：补齐初始化建表后接口恢复成功。
- 浏览器：Chrome localhost 访问被企业策略阻止，未执行真实点击测试，未尝试绕过。
- 详细矩阵：[docs/开发与测试.md](../docs/开发与测试.md)

# 2026-07-16 UI 重构验证

- `git pull --ff-only origin main`：通过，快进到 `3664027`。
- `npm run build`（`easy-strm-front`）：通过，4231 modules transformed。
- `go test ./...`（`easy-strm`）：通过，所有 Go 包通过。
- `git diff --check`：通过，无空白错误。
- Codex in-app Browser：登录页桌面 1280×720 通过；窄屏 390×844 通过，`scrollWidth=390`，无横向溢出。
- 工作台真实页面：因浏览器没有登录态，访问 `/dashboard/home` 按既有路由守卫重定向 `/login`，未绕过认证注入状态；工作台通过构建与静态模板检查。
- 发现并修复：全局 reset 覆盖 Tailwind 间距工具类，已移除 `*` 的 margin/padding reset，仅保留 box-sizing 并将 body margin 置零。

# Testing Log

## 2026-07-15/16 重构验收与 E2E 选择器迁移

- `go test ./...`（`easy-strm`）：通过。
- `go vet ./...`（`easy-strm`）：通过。
- `gofmt -l`（全部 Go 文件）：通过，无未格式化文件。
- `npm run build`（`easy-strm-front`）：通过，4231 modules transformed。
- `npm run e2e:resource-platform`：通过，覆盖同步入库、资产台账、STRM 任务深链、待处理修正与重新入库。
- `npm run e2e:organize-preview-refresh`：通过，真实后端下预览完成后遮罩退出、表格出现、刷新按钮恢复。
- `npm run e2e:organize-preview-cancel`：通过，真实后端下预览状态轮询启动，关闭后停止，重新打开无残留 loading。
- `node scripts/e2e-fullflow.mjs`：通过，40/40 PASS。
- `node scripts/e2e-cloud115-strm-generate.mjs`：核心用例 6 PASS、0 FAIL；覆盖真实登录、115 浏览、TMDB 识别、异步整理任务完成、STRM 自动匹配、STRM 文件落盘。首次清理源 ID 已过期，修正为当前源 `186` 后删除两次测试远端目录。
- `node --check scripts/e2e-*.mjs`：全部通过。
- `rg '\.el-' scripts -g 'e2e-*.mjs'`：无匹配。
- 首次 `go test ./...` 失败原因：未跟踪的一次性 `codex_e2e_user.go` 与正式入口重复 `main()`；删除临时工具后复跑通过。

## 环境

- 日期：2026-04-18
- 执行者：Codex
- 后端：`http://127.0.0.1:8082`
- 前端：`http://127.0.0.1:3001`
- 账号：`admin / admin`

## 已执行

1. 后端全量单元测试
   - 命令：`go test ./...`
   - 结果：失败
   - 关键信息：`easy-strm/internal/service/organize_service_115_test.go:435` 对 `getPreferredIdentifyResult` 的调用参数与当前函数签名不一致，导致 `easy-strm/internal/service` 无法编译

2. 前端生产构建
   - 命令：`npm run build`
   - 结果：通过

3. 浏览器全流程回归
   - 命令：`node scripts/e2e-fullflow.mjs`
   - 结果：`41/41 PASS`
   - 报告：`docs/测试执行报告_20260418_002318.md`
   - 截图目录：`debug/e2e-screenshots/fullflow/fullflow_20260418_002318/`
   - 网络跟踪：`debug/e2e-network/fullflow_20260418_002318.json`

4. 补充按钮点测
   - 方式：Playwright 分模块点测
   - 结果：通过
   - 覆盖：
     - UserInfo 刷新
     - Dashboard 任务/日志/网络弹窗
     - Cloud115 新增、扫码登录、扫码更新、编辑、测试
     - STRM 新增、编辑、删除确认、全量生成确认、Cron 快捷生成器、任务详情
     - Settings 通用保存/重置、Emby 按钮组、模板标签插入
     - Category Strategy 新增、保存、删除确认

5. 按钮静态盘点
   - 命令：`rg -o "<el-button|<button" ... | Group-Object`
   - 结果：18 个 Vue 文件共识别 `96` 个按钮入口
   - 备注：移动端下拉菜单按钮与桌面按钮存在复用同一处理函数的情况，本轮按功能去重验证

## 修复回归

1. 修复项
   - 文件：`easy-strm/internal/service/organize_service_115_test.go`
   - 内容：为 `getPreferredIdentifyResult(...)` 测试调用补齐缺失的 `sourceID` 参数
   - 结果：后端全量单元测试恢复可编译、可执行

2. 后端全量单元测试
   - 命令：`go test ./...`
   - 结果：通过

3. 前端生产构建
   - 命令：`npm run build`
   - 结果：通过

4. 浏览器全流程回归
   - 命令：`node scripts/e2e-fullflow.mjs`
   - 结果：`41/41 PASS`
   - 报告：`docs/测试执行报告_20260418_082142.md`
   - 截图目录：`debug/e2e-screenshots/fullflow/fullflow_20260418_082142/`
   - 网络跟踪：`debug/e2e-network/fullflow_20260418_082142.json`

## 备注

- Emby 未配置时，`/emby/libraries` 与 `/emby/refresh` 返回 `500` 属于预期异常路径，已按预期判定通过。
- 本地媒体源触发 `STRM` 生成接口返回 `400`，提示仅 `cloud115` 类型支持，属于预期限制。
- 真实 115 扫码登录成功、真实 Emby 连通刷新、真实 115 全量 STRM 生成未在本轮环境中完成成功路径验证。
## 2026-04-19

- 命令：`go test ./internal/service -count=1`
- 结果：通过
- 命令：`go test ./... -count=1`
- 结果：通过
## 2026-04-19

- 命令：`gofmt -w easy-strm/internal/service/watch_service.go easy-strm/internal/service/watch_service_test.go`
- 结果：通过
- 命令：`go test ./internal/service -count=1`
- 结果：通过
- 命令：`go test ./... -count=1`
- 结果：通过
## 2026-04-19

- 命令：`npm run build`
- 结果：通过

## 2026-08-29 Telegram 机器人通知与运维操作

- 执行者：Codex。
- `go test ./... -count=1`：通过；覆盖卡片转义/截断、按钮、配置默认值与 Token 保留、私聊授权、任务动作和 Redis 事件去重。
- `go vet ./...`：通过。
- `npm run build`：通过，通知设置页签完成生产构建。
- `git diff --check`：通过。
- 真实 Telegram 测试未执行：未配置 Bot Token 和管理员 Chat ID，部署后使用页面测试按钮验证。

## 2026-08-29 Telegram 发布门禁复验

- 执行者：Codex。
- `go test ./... -count=1`：通过。
- `go vet ./...`：通过。
- `npm run build`：通过。
- `git diff --check`：通过。
- 敏感信息扫描：全部变更文件未发现真实 Telegram Bot Token 或私钥。

## 2026-08-29

- 执行者：Codex。
- 命令：`go test . ./internal/service -run "TestDownloadDirectoryTreeFile|TestBuildFullGenerateCronTaskName" -count=1`
- 结果：通过；覆盖签名UA一致、签名请求头与Cookie复用、403错误详情以及cron配置ID命名。
- 命令：`$env:EASY_STRM_REAL_115_TEST='1'; $env:EASY_STRM_STRM_CONFIG_ID='2'; go test . -run '^TestDirectoryTreeDownloadSignatureReal$' -count=1 -v`
- 结果：通过；真实115目录树导出和签名下载成功，下载7,608字节并解析通过。
- 命令：`go test ./... -count=1`
- 结果：通过。
- 命令：`go vet ./...`
- 结果：通过。
- 命令：`npm run build`
- 结果：通过。
- 命令：`git diff --check`
- 结果：通过。

## 2026-08-22 文件管理全链路真实验证

- `go test ./...`：通过，包含下载 Cookie 规范化、跨账号下载上传、目录递归、失败不删源、下载失败无残留测试。
- `go vet ./...`：通过。
- `npm run build`：通过，Vite 4252 modules transformed。
- `git diff --check`：通过，仅有 LF/CRLF 提示。
- 浏览器真实矩阵：本地到本地复制/剪切/删除、本地到115、115到本地、同账号115复制/剪切、跨账号115复制/剪切、115删除全部通过。
- 115下载文件大小 13,090 字节；跨账号操作后主号、小号项数变化与源删除时序均符合预期。

## 2026-08-22 文件管理问题修复

- 定向测试：`go test ./internal/service -run TestFileManagerCrossAccountDirectoryUsesRapidTransfer -count=1`，通过；断言秒传源参数为 `file-id`。
- 后端全量测试：`go test ./...`，通过。
- 后端静态检查：`go vet ./...`，通过。
- 前端生产构建：`npm run build`，通过，Vite 转换 4252 个模块。
- 差异检查：`git diff --check`，通过；仅有 Git 的 LF/CRLF 工作区提示。
- 浏览器功能测试：新增“产品测试”媒体源后，文件管理页成功显示 15 个子目录和 `产品清单.json`，合计 16 项。
- 外部写入边界：未再次执行跨账号 115 实际复制，避免在未指定目标目录的情况下修改用户云盘；该分支由回归测试覆盖。

## 2026-08-20 统一文件管理页

- 执行者：Codex。
- `go test ./internal/service ./internal/controller`：通过；覆盖本地浏览与越界拒绝、本地剪切、目录递归上传、跨115账号目录秒传、Controller 参数校验和任务创建。
- `go test ./...`：通过；`easy-strm`、controller、dao、domain、service 全部成功。
- `go vet ./...`：通过，无静态检查问题。
- `npm run build`：通过，Vite 转换 4252 个模块并生成 `FileManager` 独立产物。
- `git diff --check`：通过，仅有工作区既有 LF/CRLF 提示，无空白错误。
- 未对真实本地媒体库或115账号执行写操作；上传、下载、秒传与删除通过注入假客户端验证调度契约。

## 2026-08-20 文件名整理识别验证

- 执行者：Codex。
- 单元测试：`go test ./internal/service -run TestParseFilenameFairyTailHundredYearsQuest -v`，通过；解析为标题 `妖精的尾巴 百年任务`、类型 `tv`、`S01E05`。
- 功能探针：使用项目当前 TMDB 配置调用 `GetCandidatesWithPath`，TMDB 返回 HTTP 401 `Invalid API key`；解析成功，在线元数据识别失败。
- 全量回归：`go test ./...`，全部通过。

## 2026-08-20 文件名识别测试页与规则配置

- 执行者：Codex。
- 定向测试：`go test ./internal/service ./internal/controller -run "Filename|Recognition" -v`，通过。
- 覆盖：5 类默认剧集模板、S00 特别篇、画质元数据保留、自定义规则保存后立即生效、无效正则、缺少 episode 捕获组、解析接口空参数、规则接口成功与失败响应。
- 后端回归：`go test ./...`，通过。
- 静态检查：`go vet ./...`，通过。
- 前端构建：`npm run build`，通过。
- 浏览器冒烟：识别测试路由、导航菜单、测试表单、样例按钮、规则编辑列表在本地 Vite 页面正常渲染；认证会话过期后由既有拦截器跳转登录。

## 2026-08-19 115 绝对目录与目录树下拉

- 执行者：Codex。
- 单元测试：`go test ./...`，通过。
- 前端目录树测试：`node src/components/resource/targetFolderTree.test.mjs`，3/3 通过。
- 前端生产构建：`npm run build`，通过，4248 个模块完成转换。
- 静态差异检查：`git diff --check`，通过。

## 2026-08-10 GitHub main 发布前测试

- 后端：在 `easy-strm` 执行 `go test ./...`，全部通过。
- 后端静态检查：在 `easy-strm` 执行 `go vet ./...`，全部通过。
- 前端：在 `easy-strm-front` 执行 `npm run build`，Vite 生产构建通过，共转换 4248 个模块。
- 前端单元测试：执行 `node --test src/components/resource/targetFolderTree.test.mjs`，3/3 通过。
- Git 质量检查：`git diff --check` 通过；真实分享凭据定向扫描无匹配。

## 2026-08-09 仪表盘首屏空白回归验证

- 执行者：Codex。
- 命令：`npm run build`；结果：通过，Vite 完成 4247 个模块转换。
- 命令：`git diff --check`；结果：通过，无空白错误，仅存在工作区既有的 LF/CRLF 提示。
- 桌面浏览器（1280×720）：Hero 高度 328px，两个光晕子元素计算样式均为 `position: absolute`，后续工作流与 Hero 间距为 24px。
- 窄屏浏览器（390×844）：页面 `scrollWidth` 为 390px，无横向溢出；Hero 高度 569px，后续工作流间距为 20px。

## 2026-08-09 资源聚合目录选择器回归验证

- 执行者：Codex。
- 回归测试首次运行：失败，缺少待实现的 `targetFolderTree.js`，证明测试在实现前处于红灯状态。
- 命令：`node --test src/components/resource/targetFolderTree.test.mjs`；结果：3/3 通过，覆盖 Axios 响应提取、115 原始目录字段归一化和嵌套路径拼接。
- 命令：`npm run build`；结果：通过，Vite 完成 4248 个模块转换。
- 命令：`git diff --check`；结果：通过，无空白错误，仅有工作区既有 LF/CRLF 提示。
- 真实浏览器：根目录列表展示通过；子目录懒加载通过；路径选择回填 `/云下载/LOL-ERICHAND` 通过；验证后恢复 `/`。

## 2026-08-09 前后端手动验证

- 执行者：Codex。
- `Test-NetConnection 192.168.31.12 -Port 15432`：通过，PostgreSQL 端口可达。
- `Test-NetConnection 192.168.31.12 -Port 16379`：通过，Redis 端口可达。
- 后端 `go run .`：通过，数据库与 Redis 初始化成功，监听 `:8082`。
- 前端 `npm run dev -- --host 127.0.0.1`：通过，监听 `127.0.0.1:3001`。
- `curl.exe --noproxy "*" http://127.0.0.1:3001/`：HTTP 200。
- `curl.exe --noproxy "*" http://127.0.0.1:8082/`：HTTP 404，符合根路径未注册的现状，同时证明 Gin 可响应。
- 浏览器登录：默认管理员登录成功，跳转 `/dashboard/home`。
- 页面巡检：仪表盘、任务中心、资源聚合、115 云下载、文件工作台、STRM 配置、115 云管理、整理规则、系统设置、系统日志、网络测试、缓存管理均成功渲染对应页面标题。
- 交互巡检：失败任务详情弹窗可打开；资源聚合两个页签可切换；媒体源文件浏览弹窗可打开。
- 未执行：真实转存、云下载、删除、整理、刮削、保存配置等有业务副作用的动作；本轮没有代码变更，因此未重复执行单元测试与生产构建。

## 2026-05-27 功能盘点与浏览器验证

- 任务：整理当前项目功能细节，并基于功能细节进行一轮浏览器测试。
- 执行者：Codex。
- 功能盘点依据：`README.md`、`docs/产品与架构.md`、`docs/开发与测试.md`、`easy-strm-front/src/main.js`、`easy-strm-front/src/views/**`、`easy-strm-front/src/utils/api/**`、`easy-strm/auth.go`、`easy-strm/auth_routes_public.go`。
- 环境检查：
  - Docker Desktop Linux daemon 未运行，`docker ps` 无法连接 `npipe:////./pipe/dockerDesktopLinuxEngine`。
  - 本机 `127.0.0.1:5432` PostgreSQL 不通。
  - 本机 `127.0.0.1:6379` Redis 不通。
  - 因数据库与 Redis 不可用，无法启动真实后端完成真实账号/真实 API 浏览器全链路。
- 已执行验证：
  - 命令：`npm run build`
  - 工作目录：`easy-strm-front`
  - 结果：通过，Vite 生产构建完成。
  - 命令：`npm run e2e:resource-platform`
  - 工作目录：`easy-strm-front`
  - 结果：通过，输出 `ok=true`，覆盖 `sync`、`ledger`、`strm task link`、`pending identify and run`。
  - 命令：`go test ./...`
  - 工作目录：`easy-strm`
  - 结果：通过，所有 Go 包测试通过。
- 浏览器工具状态：
  - 已按 Browser 插件连接 Codex in-app Browser。
  - 访问 `http://127.0.0.1:3001` 时被企业网络策略拦截，未继续绕过策略。
  - 本轮浏览器成功路径以仓库内置 Playwright E2E `npm run e2e:resource-platform` 为准。

## 2026-05-27 真实后端本地启动浏览器验证

- 任务：按用户提供的远端 PostgreSQL/Redis 依赖，本地启动后端与前端并进行浏览器测试。
- 执行者：Codex。
- 环境：
  - 后端：`http://127.0.0.1:8082`
  - 前端：`http://127.0.0.1:3001`
  - PostgreSQL：远端 `easy_strm` 库，端口连通。
  - Redis：远端实例，端口连通。
- 启动结果：
  - 后端 `go run .` 启动成功，数据库初始化、Redis 连接、Cron 加载和路由注册完成。
  - 前端 `npm run dev -- --host 127.0.0.1 --port 3001 --strictPort` 启动成功。
  - 后端启动日志提示历史媒体源 `测试本地媒体源` 的本地目录不存在，该告警不阻塞服务启动。
- 登录准备：
  - 远端库中 `admin/admin` 不是有效登录。
  - 创建临时测试用户执行浏览器验证，验证完成后已删除该用户。
- 浏览器脚本：
  - 文件：`.codex/real-backend-browser-e2e-2026-05-27.mjs`
  - 结果：通过，`ok=true`。
  - 覆盖：登录、13 个后台路由渲染、同步入库页临时媒体源展示、触发全量同步、资产台账显示测试文件。
- 页面巡检：
  - `/dashboard/home`：通过。
  - `/dashboard/media-library`：通过。
  - `/dashboard/sync-tasks`：通过。
  - `/dashboard/pending-media`：通过。
  - `/dashboard/tasks`：通过。
  - `/dashboard/media-manager`：通过。
  - `/dashboard/strm-config`：通过。
  - `/dashboard/cloud115`：通过。
  - `/dashboard/category-strategy`：通过。
  - `/dashboard/settings`：通过。
  - `/dashboard/system-logs`：通过。
  - `/dashboard/network`：通过。
  - `/dashboard/cache`：通过。
- 核心链路：
  - 临时本地媒体源创建成功。
  - 同步入库页能显示临时媒体源。
  - 点击全量同步后页面出现同步/任务反馈。
  - 媒体资产台账能显示测试文件 `Codex.Real.Backend.2026.1080p.mkv`。
  - 临时媒体源由脚本清理。
- 截图：
  - `debug/real-backend-browser-20260527/login-dashboard.png`
  - `debug/real-backend-browser-20260527/dashboard_home.png`
  - `debug/real-backend-browser-20260527/dashboard_media-manager.png`
  - `debug/real-backend-browser-20260527/dashboard_cloud115.png`
  - `debug/real-backend-browser-20260527/core-flow-library.png`
- 观察：
  - 浏览器 console 捕获到 2 条 `403 Forbidden` 资源加载错误，未触发 `pageerror`，且路由巡检均未出现页面级网络错误。

## 2026-05-27 按钮矩阵浏览器验证

- 任务：继续验证主要按钮是否可用。
- 执行者：Codex。
- 脚本：`.codex/button-matrix-e2e-2026-05-27.mjs`
- 环境：
  - 后端：`http://127.0.0.1:8082`
  - 前端：`http://127.0.0.1:3001`
  - 数据库与 Redis：用户提供的远端实例。
- 结果：通过，`ok=true`，`failed=[]`。
- 覆盖通过的按钮组：
  - 登录页按钮。
  - 顶部快捷入口与主题切换按钮。
  - 首页 4 个快捷入口。
  - 同步入库：刷新媒体源、全量同步、增量同步、执行入库、查看资产台账。
  - 资产台账：同步入库、待处理、行内入库、STRM、刷新库。
  - 待处理：查看资产台账、修正弹窗保存路径、忽略路径触达。
  - 任务中心：刷新与详情按钮触达。
  - 文件工作台：新增媒体源弹窗、媒体源浏览弹窗、文件浏览刷新。
  - 115 云管理：新增账号弹窗、扫码登录弹窗、账号编辑弹窗、账号测试按钮触达。
  - 整理规则：刷新、新增取消、选择分类、删除取消。
  - STRM 配置：新增、编辑、删除取消。
  - 系统设置：重置、保存 TMDB 配置、测试 Emby 连接。
  - 系统日志：刷新、自动刷新。
  - 网络测试：重新探测。
  - 缓存管理：刷新概览、清理全部缓存取消。
- Warning：
  - 任务中心“详情”按钮可点击，但自动化未观察到详情抽屉打开，需要单独排查。
- 清理：
  - 临时 STRM 配置已删除。
  - 临时分类已删除。
  - 临时媒体源已删除。
  - 临时测试用户已删除并确认无法登录。
- 截图：
  - `debug/button-matrix-20260527/button-matrix-final.png`
- 观察：
  - 浏览器 console 捕获到 3 条 `403 Forbidden` 资源加载错误，未触发 `pageerror`。

## 2026-05-15

- 命令：`go test ./...`
- 工作目录：`easy-strm`
- 结果：通过，新增 115 事件流辅助函数与 JWT Token 单元测试均通过。
- 命令：`go vet ./...`
- 工作目录：`easy-strm`
- 结果：通过。
- 命令：`npm run build`
- 工作目录：`easy-strm-front`
- 结果：通过，Vite 生产构建完成。
- 命令：`npm run e2e:resource-platform`
- 工作目录：`easy-strm-front`
- 结果：通过，返回 `ok=true`，覆盖同步、台账、STRM 任务深链、待处理识别与入库。
- 命令：`npm run e2e:organize-preview-refresh`
- 工作目录：`easy-strm-front`
- 结果：通过，返回 `ok=true`，整理预览刷新后表格与操作按钮状态正常。
- 命令：`npm run e2e:organize-preview-cancel`
- 工作目录：`easy-strm-front`
- 结果：通过，返回 `ok=true`，预览取消状态轮询正常。
- 命令：`docker compose config`
- 工作目录：项目根目录
- 结果：通过，Compose 配置可解析。
- 命令：`docker build -t easy-strm:codex-check .`
- 工作目录：项目根目录
- 结果：未执行成功，环境阻塞为 Docker Desktop Linux daemon 未运行：`failed to connect to the docker API at npipe:////./pipe/dockerDesktopLinuxEngine`。

## 2026-04-24

- 修复项：`easy-strm-front/src/views/Cloud115.vue`
- 内容：115 账号“测试”按钮成功后改为显示明确的 `ElMessage` 成功提示，文案包含账号名与可访问项数。
- 修复项：`easy-strm-front/src/main.js`
- 内容：补充 `element-plus/es/components/message/style/css`，修复 `ElMessage` 样式未加载导致的异常弹窗展示。
- 命令：`npm run build`
- 结果：通过

## 2026-05-06

- 变更范围：媒体库管理 Phase 0/1 落地，包含同步索引、任务步骤、待处理清单、本地/115 同步接口、媒体库/待处理/同步任务前端入口。
- 命令：`go test ./internal/dao`
- 结果：通过
- 命令：`go test ./...`
- 结果：通过
- 命令：`npm run build`
- 结果：通过
- 命令：`npm run e2e:organize-preview-refresh`
- 结果：通过，输出 `ok=true`，截图 `debug/preview_refresh_20260507_000457.png`
- 命令：`npm run e2e:organize-preview-cancel`
- 结果：通过，输出 `ok=true`，截图 `debug/preview_cancel_20260507_000457.png`
- 接口冒烟：登录、创建本地媒体源、执行全量同步、查询同步索引、查询媒体库列表、查询任务步骤详情
- 结果：通过，`scanned=1`、`changed=1`、`index_total=1`、`library_total=1`、`step_count=3`、`task_status=completed`

### 备注

- E2E 启动了本地后端 `http://127.0.0.1:8082` 和前端 `http://127.0.0.1:3001`。
- 直接访问 localhost 时当前 shell 环境会走代理并返回 `502`，接口冒烟已使用 `-NoProxy` 和 `NO_PROXY=127.0.0.1,localhost` 绕开代理。

## 2026-05-06 复测补充

- 修复项：补齐媒体库条目详情接口 `GET /media/library/items/:id`，并让启动建表逻辑同步创建更新时间触发器。
- 命令：`go test ./...`
- 结果：通过。
- 命令：`npm run build`
- 结果：通过。
- 命令：`npm run e2e:organize-preview-refresh`
- 结果：通过，输出 `ok=true`，截图 `debug/preview_refresh_20260507_001032.png`。
- 命令：`npm run e2e:organize-preview-cancel`
- 结果：通过，输出 `ok=true`，截图 `debug/preview_cancel_20260507_001032.png`。
- 接口冒烟：登录、创建本地媒体源、执行全量同步、查询同步索引、查询媒体库列表、查询媒体库详情、查询任务步骤、创建待处理项、人工识别、忽略待处理项。
- 结果：通过，`source_id=208`、`index_total=2`、`library_total=2`、`detail_id=2`、`health=ok`、`task_status=completed`、`step_count=3`、`pending_status=ignored`、`tmdb_id=12345`。

## 2026-05-07 Phase 2 完整落地验证

- 变更范围：媒体库入库流水线、待处理自动生成与重新入库、STRM 输出策略、同步任务增强、媒体库/同步任务/待处理前端操作闭环。
- 命令：`go test ./...`
- 结果：通过。
- 命令：`npm run build`
- 结果：通过。
- 命令：`npm run e2e:organize-preview-refresh`
- 结果：通过，输出 `ok=true`，截图 `debug/preview_refresh_20260507_080300.png`。
- 命令：`npm run e2e:organize-preview-cancel`
- 结果：通过，输出 `ok=true`，截图 `debug/preview_cancel_20260507_080300.png`。
- 接口冒烟：登录、创建本地媒体源、执行全量同步、同步后 pipeline 自动生成待处理项、人工识别待处理项、重新入库、查询媒体库索引。
- 结果：通过，`source_id=212`、`sync_scanned=1`、`sync_failed=0`、`pending_status=completed`、`library_identity=identified`、`tmdb_id=65432`、`library_total=1`。

## 2026-05-07 115 生活事件流接入验证

- 变更范围：接入 `https://proapi.115.com/android/behavior/detail` 真实 115 生活事件流，新增源级游标，并将 115 媒体源增量同步调整为“事件优先、目录扫描兜底、24 小时周期对账”。
- 资料依据：`p115client` 的 `life_behavior_detail_app` 实现与 `tool/life.py` 行为说明，确认该事件流会缺少回收站还原等部分事件，因此保留目录对账兜底。
- 命令：`go test ./...`
- 结果：通过。
- 命令：`npm run build`
- 结果：通过。
- 命令：`npm run e2e:organize-preview-refresh`
- 结果：通过，输出 `ok=true`，截图 `debug/preview_refresh_20260507_093957.png`。
- 命令：`npm run e2e:organize-preview-cancel`
- 结果：通过，输出 `ok=true`，截图 `debug/preview_cancel_20260507_093957.png`。
- 新增单元测试：115 事件游标过滤、浏览类事件忽略、更新时间兜底、事件类型汇总。
- 真实账号冒烟：读取库内两个 115 账号 Cookie，调用 `GetLifeEvents(0, 10, "", "")` 均通过；账号 `id=2` 返回 `count=38010`、`returned=10`、`next_page=true`，账号 `id=3` 返回 `count=84`、`returned=9`、`next_page=true`。
- 媒体源检查：库内存在启用的 115 媒体源 `id=186`、名称“小号整理”、路径 `/`、账号 `id=3`。
- 已知边界：由于 API 登录账号不是默认 `admin/admin`，未通过 HTTP 鉴权接口触发同步；为避免对现有根目录媒体源执行非递归目录扫描并影响索引状态，本轮真实测试只验证 115 生活事件接口成功路径与库内媒体源配置可读性。

## 2026-05-07 浏览器全局仿真

- 方式：优先尝试 Codex in-app browser，当前环境未发现可连接 IAB 后端，降级使用项目 Playwright 浏览器自动化。
- 命令：`npm run e2e:organize-preview-refresh`
- 结果：通过，输出 `ok=true`，截图 `debug/preview_refresh_20260507_140455.png`。
- 命令：`node scripts/e2e-fullflow.mjs`
- 结果：通过，`40/40 PASS`，报告 `docs/测试执行报告_20260507_140509.md`，截图目录 `debug/e2e-screenshots/fullflow/fullflow_20260507_140509`。
- 补充浏览器仿真：创建临时本地媒体源，进入“同步任务”页点击全量同步，确认结果 `扫描 1，写入 1，失效 0`，进入“媒体库”页确认条目显示，进入“待处理”页确认页面可访问；临时媒体源已删除。
- 115 管理页浏览器验证：进入 `/dashboard/cloud115`，确认“账号池总览”加载成功，账号表格显示 `2` 行，截图目录 `debug/e2e-screenshots/global-browser-cloud115/20260507061019`。
- 命令：`npm run e2e:organize-preview-cancel`
- 结果：通过，输出 `ok=true`，截图 `debug/preview_cancel_20260507_141028.png`。

## 2026-05-14 资源整理平台首版重构验证

- 命令：`go test ./...`
- 工作目录：`easy-strm`
- 结果：通过，controller、dao、domain、service 测试均通过。
- 命令：`npm run build`
- 工作目录：`easy-strm-front`
- 结果：通过，Vite 生产构建完成，`1653 modules transformed`。
- 命令：`npm run e2e:resource-platform`
- 工作目录：`easy-strm-front`
- 结果：通过，输出 `ok=true`，覆盖 `sync`、`ledger`、`strm task link`、`pending identify and run`。
- 命令：`docker compose config`
- 工作目录：项目根目录
- 结果：通过，Compose 配置可解析，已无 `version` 字段废弃警告。
- 命令：`docker build -t easy-strm:codex-check .`
- 工作目录：项目根目录
- 结果：未执行成功，环境阻塞为 Docker Desktop Linux daemon 未运行：`failed to connect to the docker API at npipe:////./pipe/dockerDesktopLinuxEngine`。

## 2026-05-15 大文件继续深拆验证

- 命令：`go test ./internal/controller`
- 工作目录：`easy-strm`
- 结果：通过，覆盖 controller 拆分后的包级编译与新增细粒度测试。
- 命令：`go test ./internal/dao`
- 工作目录：`easy-strm`
- 结果：通过，覆盖 TMDB/更名预设/媒体文件缓存 DAO 拆分。
- 命令：`go test ./internal/service`
- 工作目录：`easy-strm`
- 结果：通过，覆盖媒体源服务、重命名服务、TMDB、watch、scrape 等服务拆分。
- 命令：`gofmt -l (rg --files -g "*.go")`
- 工作目录：`easy-strm`
- 结果：通过，无未格式化 Go 文件。
- 命令：`go test ./...`
- 工作目录：`easy-strm`
- 结果：通过，所有 Go 包测试通过。
- 命令：`go vet ./...`
- 工作目录：`easy-strm`
- 结果：通过。
- 命令：`npm run build`
- 工作目录：`easy-strm-front`
- 结果：通过，Vite 生产构建完成。
- 命令：`npm run e2e:resource-platform`
- 工作目录：`easy-strm-front`
- 结果：首次因重复文本 strict mode 失败，修复脚本定位后复跑通过，输出 `ok=true`。
- 命令：`npm run e2e:organize-preview-refresh` / `npm run e2e:organize-preview-cancel`
- 工作目录：`easy-strm-front`
- 结果：当前环境未运行后端 `127.0.0.1:8082`，登录等待 `/dashboard/**` 超时；该阻塞为本地服务依赖不可用，不是本轮重构引入的编译或构建失败。
- 命令：`docker compose config`
- 工作目录：项目根目录
- 结果：通过。
- 命令：`docker build -t easy-strm:codex-check .`
- 工作目录：项目根目录
- 结果：未执行成功，Docker Desktop Linux daemon 未运行：`failed to connect to the docker API at npipe:////./pipe/dockerDesktopLinuxEngine`。
## 2026-04-19

- 命令：`npm run build`
- 结果：通过
## 2026-04-19

- 命令：`gofmt -w easy-strm/internal/service/watch_service_test.go`
- 结果：通过
- 命令：`go test ./internal/service -count=1`
- 结果：通过
- 命令：`go test ./... -count=1`
- 结果：通过
## 2026-04-19

- 命令：`gofmt -w easy-strm/internal/service/media_source_service.go easy-strm/internal/service/media_source_service_test.go`
- 结果：通过
- 命令：`go test ./internal/service -count=1`
- 结果：通过
- 命令：`go test ./... -count=1`
- 结果：通过
## 2026-04-19

- 命令：`npm run build`
- 结果：通过
# 2026-08-30 STRM 配置列表与全量清理

- 执行者：Codex。
- 回归测试（修复前）：`go test . -run '^TestCleanupStrmFiles' -count=1 -v` 失败，确认旧实现会删除目标目录本身，且空路径不会报错。
- 回归测试（修复后）：同一命令通过，覆盖普通文件、隐藏文件、嵌套目录、空路径、当前目录和磁盘根目录。
- 后端全量：`go test ./... -count=1`，全部通过。
- 后端静态检查：`go vet ./...`，通过。
- 前端生产构建：`npm run build`，4253 modules transformed，构建通过。
- 差异检查：`git diff --check`，通过。
- 页面验证说明：本地前后端服务均未运行；为避免启动后端时连接现有数据库并加载定时任务，本次未执行真实页面数据交互，前端以生产构建和列模型静态核对完成验证。
# 2026-08-30 115 账号列表紧凑布局

- 执行者：Codex。
- 前端生产构建：`npm run build`，4253 modules transformed，构建通过。
- 差异检查：`git diff --check`，通过。
- 本地页面只读验证：访问 `/dashboard/cloud115`，表头为 ID、名称、账号类型、状态、优先级、Cookie、转存账号、转存目录、秒传方式、创建时间、操作，不含更新时间。
- 几何验证：两行的编辑、扫码更新、测试、删除按钮高度均为 22px；每行四个按钮的 top 坐标分别完全一致，确认处于同一排。
- 未点击测试、扫码更新、编辑或删除按钮，未产生账号数据变更。

# 2026-08-30 仪表盘账号配额修复

- 执行者：Codex。
- 专项测试：`go test ./internal/service -run Dashboard -count=1`，通过。
- 后端全量：`go test ./...`，全部通过。
- 后端静态检查：`go vet ./...`，通过。
- 后端静态检查：`go vet ./...`，通过。
- 前端生产构建：`npm run build`，4253 modules transformed，构建通过。
- 差异检查：`git diff --check`，通过；仅有工作区既存 LF/CRLF 提示。
- 覆盖：真实已用/总容量映射、容量比例计算、超过100%时的显示上限、单账号查询失败不影响其他账号。
- 运行时复验：重启旧后端后，使用已登录本地浏览器只读打开 `/dashboard/home`；115主号显示 `21.5 TB / 63.4 TB`，115小号显示 `1.5 TB / 15.6 TB`，不再出现“容量获取失败”。

# 2026-08-30 账号配额五分钟缓存

- 执行者：Codex。
- 专项测试：`go test ./internal/dao ./internal/service -run 'AccountQuota|Dashboard' -count=1`，通过。
- 后端全量：`go test ./... -count=1`，全部通过。
- 后端静态检查：`go vet ./...`，通过。
- 前端生产构建：`npm run build`，4253 modules transformed，构建通过。
- TTL 验证：写入后 TTL 精确为五分钟；miniredis 快进五分钟后读取为未命中。
- 缓存策略验证：命中时115调用次数为零；未命中或坏缓存时回源并回填；坏 JSON 会主动删除。
- 真实只读验证：首次及二次加载均显示主号 `21.5 TB / 63.4 TB`、小号 `1.5 TB / 15.6 TB`；第二次请求无新增115驱动调用。
- 2026-08-30 Telegram 重复通知修复：专项测试最初因本机 Redis 未运行而跳过，随后引入 `miniredis` 消除外部依赖；`go test ./internal/service -run "TestNotificationEventMonitor" -count=1 -v` 的 3 个测试均实际通过，其中包含 8 路并发去重和发送失败释放抢占。`go test ./... -count=1`、`go vet ./...`、`git diff --check` 通过；`go test -race` 因当前 Windows Go 环境未启用 CGO 而无法执行。
- 2026-08-30 Telegram 115资源操作：语法解析、分享/云下载账号选择、优先级与ID决胜、指定账号、默认/指定目录、分享密码解析、真实Service提交参数和Telegram消息路由专项测试通过；后端 `go test ./... -count=1`、`go vet ./...`、前端 `npm run build`、`git diff --check` 通过。

# 2026-08-30 115自动转存目标目录修复

- 执行者：Codex。
- 修复前回归：`go test . -run TestBuildReceiveShareFormUsesCID -count=1` 因缺少正确表单构造而失败，确认测试先于实现落地。
- 协议专项：`go test . -run TestBuildReceiveShareFormUsesCID -count=1`，通过；覆盖 `cid`、`share_code`、`receive_code`、`file_id`，并断言不发送 `save_folder_id`。
- Service专项：`go test ./internal/service -run 'TestExecuteTransferStopsWhenTargetDirectoryCannotResolve|TestExecuteTransferAutoOrganizeDisabledDoesNotCreateChildTasks' -count=1`，通过。
- 后端全量：`go test ./... -count=1`，全部通过。
- 后端静态检查：`go vet ./...`，通过。
- 差异检查：`git diff --check`，通过；仅输出工作区既存LF/CRLF提示。
- 未执行真实115写入测试，避免向用户云盘生成或错放测试文件。

# 2026-08-30 Emby 管理工作台

- 专项测试：`go test ./internal/service -run 'TestEmbyManagement|TestEmbyRefreshTask' -count=1`，通过。
- 后端全量：`go test ./...`，全部通过。
- 前端生产构建：`npm run build`，4254 modules transformed，构建通过。
- 覆盖：指定实例 API Key 与用户列表隔离；媒体库刷新提交、后端轮询、100% 进度、步骤和成功结论；无效实例 ID 参数拒绝。
- 未执行真实 Emby/神医助手/AI 接口成功链路，原因是当前环境没有对应服务地址和密钥；相关 HTTP 调度及异常路径由本地模拟服务验证。

## 补充验证：逐库部分成功、头像与管理员边界

- `go test ./internal/service -run 'TestEmbyManagement|TestEmbyRefresh' -count=1`：通过；新增“一库成功、一库 HTTP 500”场景，断言 `partial_success`、成功/失败计数、逐库失败明细和结论。
- `go test ./internal/service ./internal/controller -count=1`：在补充修改后首次执行通过。
- `npm run build`：再次通过，4254 modules transformed；覆盖用户头像读取/上传交互、管理员菜单过滤和任务中心统计调整。
- `go test ./... -count=1` 与 `go vet ./...`：在本轮早期均通过；管理员边界补充后再次执行时，被工作区同时出现的非 Emby 变更阻断：`watch_service_test.go` 引用了当前不存在的 `collectLocalWatchTree`、`collectCloud115VideoFileSet` 和 `cloud115WatchPageSize`，另有既存媒体源 Controller 默认值断言失败。Emby 代码在这些变化出现前已通过全量测试，之后前端构建及主包编译继续通过；未修改上述不相关模块。
- 待并行工作区改动稳定后再次执行 `go test ./... -count=1` 和 `go vet ./...`：全部通过，最终无测试阻塞。

# 2026-08-30 神医助手 STRM 扫描与视频封面

- 专项测试：`go test ./internal/service -run 'TestStrmScanCaptureWorkflow|TestEmbyManagement|TestEmbyRefresh' -count=1`，通过。
- 后端全量：`go test ./... -count=1`，全部通过。
- 后端静态检查：`go vet ./...`，通过。
- 前端生产构建：`npm run build`，4254 modules transformed，构建通过。
- 模拟链路覆盖：媒体库扫描、STRM 数量回读、Extract MediaInfo 提交、计划任务 50% 真实进度、远端成功终态、执行前后主图覆盖统计以及最终结论。
- 未执行真实截图：当前环境没有可供写入验证的 Emby/StrmAssistant 实例；没有修改用户插件配置或媒体库图片。

## `<nil>` 误判回归

- 真实任务 `emby-plugin-63171fd4-5643-4f5d-a9f4-392947b3a349` 已完成媒体库扫描并发现 87 个 STRM，但在跟踪截图任务时将缺失的 `ErrorMessage` 字段误转为字符串 `<nil>`。
- 回归测试先移除模拟响应中的 `ErrorMessage` 字段，修复前稳定失败；空值归一化修复后通过。
- `go test ./... -count=1` 与 `go vet ./...`：全部通过。

# 2026-08-30 文件工作台自动整理与监控修复

- 修复前专项：`go test ./internal/controller ./internal/service -run 'TestMediaSourceControllerCreateDefaults|TestCollect(Local|Cloud115)' -count=1`，按预期失败；Controller 实际传入 `enabled=false`，Service 缺少递归扫描实现。
- 修复后专项：同一命令通过；另执行 Watch 相关专项测试通过。
- 运行态专项：Controller 响应覆盖 `watch_running=false/true`，WatchService 覆盖本地、115 和不存在三种运行态查询。
- 后端全量：`go test ./... -count=1`，全部通过。
- 后端静态检查：`go vet ./...`，通过。
- 前端生产构建：`npm run build`，4254 modules transformed，构建通过。
- 差异检查：`git diff --check`，通过；仅有工作区既存 LF/CRLF 提示。
- 页面只读验证：当前 3 个媒体源真实 `enabled=false`，页面显示“媒体源已停用”，已开启监控统计为 0；115 关联账号显示“115小号”；编辑弹窗三个开关状态依次为 `false/true/true`（媒体源停用、目录监控开启、自动整理开启）。
- `go test -race` 未执行：当前 Windows Go 环境提示 `-race requires cgo`；常规测试和 `go vet` 均通过。
- 未执行真实整理：避免启用后移动用户文件或向115发起写操作。
# 2026-08-30 115 自动整理文件名修复验证

- `go test ./internal/service -run 'TestCollectCloud115VideoFileSetRecursesAndPaginates|TestFindNewCloud115FileIDsReturnsRelativePathsInsteadOfPickCodes|TestWatchServiceCloud115WatchState' -count=1`：通过。
- `go test ./... -count=1`：通过。
- `go vet ./...`：通过。
- `npm run build`：通过，Vite 生产构建完成。
- `git diff --check`：通过；仅输出工作区既有 LF/CRLF 提示。
- 本地后端重启后监听 `8082`，115 媒体源 10 初始快照识别到 13 个视频文件，监控启动成功。

# 2026-08-30 内置站点默认代理

- 测试先行：`go test ./... -run "Test(BuildProxyDomains|SettersAndHelpers)" -count=1` 在旧实现上按预期失败，缺少 `buildProxyDomains` 与 `SetHTTPClient`。
- 专项回归：`go test ./... -run "Test(BuildProxyDomains|ShouldUseProxy|SettersAndHelpers)" -count=1`，通过。
- 后端全量：`go test ./... -count=1`，全部通过。
- 后端静态检查：`go vet ./...`，通过。
- 前端生产构建：`npm run build`，4254 modules transformed，构建通过。
- 差异检查：目标文件 `git diff --check` 通过；仅有工作区既存 LF/CRLF 提示。
- 覆盖：五个内置探测地址、非内置地址、自定义域名追加、未配置代理地址保持直连、TMDB HTTP 客户端注入。

# 2026-08-30 TMDB 网络探测鉴权

- 测试先行：`go test . -run "Test(BuildNetworkProbeRequestURL|RedactNetworkProbeSecret)" -count=1` 在旧实现上按预期编译失败。
- 专项回归：`go test . -run "Test(BuildNetworkProbeRequestURL|RedactNetworkProbeSecret|BuildProxyDomains|ShouldUseProxy)" -count=1`，通过。
- 后端全量：`go test ./... -count=1`，全部通过。
- 后端静态检查：`go vet ./...`，通过。
- 前端生产构建：`npm run build`，4254 modules transformed，构建通过。
- 覆盖：TMDB Key 附加、原查询参数保留、其他站点不变、错误信息密钥脱敏。

# 2026-08-30 115 Cookie 来源

- 执行者：Codex。
- 首轮 `go test ./...`：业务代码完成编译，既有 Cloud115 sqlmock 因查询新增 `cookie_source` 列而失败；同步更新查询断言和行夹具后恢复。
- 后端全量：`go test ./...`，全部通过。
- 前端生产构建：`npm run build`，4254 modules transformed，构建通过。
- 差异检查：`git diff --check` 无空白错误，仅输出工作区既有 LF/CRLF 提示。
- 覆盖：扫码渠道到中文来源映射、未知渠道回退、来源去空格与 100 字符边界、API 响应字段、手动创建参数传递、扫码更新的 app/name/cloud_id 参数传递、Cloud115 既有查询回归。

## 历史 Cookie UID 自动识别增量验证

- 后端全量：`go test ./...`，全部通过。
- 后端静态检查：`go vet ./...`，通过。
- 前端生产构建：`npm run build`，4843 modules transformed，构建通过；仅有既存大分块提示。
- 覆盖：`UID=<用户ID>_<ssoent>_<时间戳>` 正常解析、字段名大小写、引号值、未知设备码、旧式 UID、非法用户 ID、缺失 UID、A1 歧义文案、人工来源优先、历史空值自动推断及 API UID/设备码字段。

# 2026-08-30 神医助手截图依赖一键配置

- `go test ./internal/service ./internal/controller -count=1`：通过。
- `go test ./... -count=1`：通过。
- `npm run build`：通过，Vite 生产构建完成。
- 新增覆盖：适用视频类型 Image Capture 增量启用、重复执行幂等、完整媒体库设置保留、空 Library Scope 保持全部、非空范围去重合并、插件其他设置保留、配置写入后回读核验、未确认请求拒绝。
- `go vet ./...` 与目标文件 `git diff --check`：通过；差异检查仅有既存 LF/CRLF 提示。
- 浏览器只读验证：选定“其他-不可刮削115”后弹窗完整展示三项操作和跨媒体库影响提示；点击取消后未触发任务。
# 2026-08-30 Emby 观影监控中心

- `go test ./internal/dao ./internal/service ./internal/controller`：通过。
- `go test ./...`：通过。
- `npm run build`：通过；ECharts 独立页面 chunk 产生体积提示，不影响构建。
- `npm run e2e:emby-monitor`：通过，输出 `{"ok":true,"tabs":6,"mobile":true}`。
- E2E 首轮失败暴露标签延迟挂载问题；修复首次挂载立即加载后通过。

## 历史 Cookie 来源最终回归

- `go test ./...`：通过。
- `go vet ./...`：通过。
- `npm run build`：通过，4843 modules transformed；仅有既存大分块提示。
- 额外边界：历史 UID 推断来源不会在普通编辑时回填并固化，Cookie 发生变化后仍可按新 UID 重新识别。

## 2026-08-30 企业微信应用通知渠道

- 执行者：Codex。
- 企业微信专项：鉴权查询参数、消息接收字段、Agent ID、Markdown 内容、Token 复用、企业微信业务错误、HTTP 错误、接收范围校验和 2000 字节截断全部通过。
- 配置专项：Secret 不回传、空 Secret 保留旧值、非法 Agent ID 拒绝、兼容接口配置脱敏通过。
- 事件专项：首次启用不补发、同渠道终态去重、失败释放抢占、Telegram 与企业微信基线独立通过。
- `go test ./... -count=1`：通过。
- `go vet ./...`：通过。
- `npm run build`：通过；仅有既存 Emby 分块体积提示。
- `git diff --check`：通过；仅有工作区既存 LF/CRLF 提示。
- 未执行真实企业微信发送：仓库没有 Corp ID、Agent ID 和应用 Secret，部署后可在设置页发送测试通知。
- 运行时路由复测：重启前 `/notify/wecom/config` 返回 404；重启后 GET/PUT 配置接口与 POST 测试接口在无 Token 请求下均返回 401，与既有受保护接口一致。
- API 接收专项：官方算法生成的加密样本完成 GET echostr 校验、POST XML 解密、错误签名拒绝、Agent ID 不匹配拒绝、同 MsgId 两次投递只回复一次；Controller GET 返回明文 echostr、POST 返回 `success`。
- 回调配置专项：43 位 EncodingAESKey 校验、Token/AESKey 空值保留、读取脱敏以及兼容接口脱敏通过。

## 2026-08-30 文件工作台概览板块清理

- 执行者：Codex。
- 生产构建：在 `easy-strm-front` 执行 `npm run build`，Vite 成功转换 4844 个模块并完成产物生成；仅有既存 Emby 分块大于 500 kB 的提示。
- 功能结构测试：PowerShell 断言红框文案、摘要派生状态、`sourceListRef` 和 `defineExpose` 均已移除，并确认 `MediaSourceList` 仍位于 `FileBrowser` 之前；结果通过。
- 冒烟检查：`git diff --check` 对两个目标文件通过，仅输出既存 LF/CRLF 转换提示。
- 浏览器复核：内置浏览器访问 `http://localhost:8082/media-manager` 被本机客户端策略拦截，未取得运行态截图；构建与结构测试已覆盖本次纯展示层删除的主要风险。

## 2026-08-30 云下载记录名称列宽度约束

- 执行者：Codex。
- 生产构建：在 `easy-strm-front` 执行 `npm run build`，Vite 成功转换 4844 个模块并生成产物；仅有既存 Emby 分块体积提示。
- 功能结构测试：断言“名称 / 链接”列使用固定 `width: 420`、不再包含 `minWidth`，并保留两个 `NEllipsis` 分别展示名称和链接；结果通过。
- 差异检查：`git diff --check -- easy-strm-front/src/views/resources/OfflineDownload.vue` 通过，仅输出既存 LF/CRLF 转换提示。
- 用户回归后补充验证：确认 Naive UI 2.41 默认使用自动表格布局，普通 `width` 只生成同值 `minWidth`，因此首次修改不足以限制长链接。
- 修正后生产构建：`npm run build` 通过，成功转换 4844 个模块。
- Naive UI 实际样式测试：`createCustomWidthStyle({ width: 420, maxWidth: 420 })` 返回 `width/minWidth/maxWidth` 均为 `420px`。
- 构建产物测试：确认资源聚合产物包含 `scroll-x=1140`、`table-layout=fixed`、`width=420` 和 `maxWidth=420`；结果通过。首次断言按驼峰属性匹配编译产物失败，检查实际 Vue 编译形式后修正断言并通过，未修改业务实现。

## 2026-08-30 企业微信云下载创建反馈修复

- 执行者：Codex。
- 定向测试：`go test ./internal/service -run 'TestWeComCallbackService' -count=1`，通过。
- 全量后端测试：`go test ./...`，通过；service、controller、dao、domain 及主包均无失败。
- 覆盖场景：全局配置存在 `detail_url`、用户主动提交云下载、回复目标成员正确、发送配置强制纯文本、成功反馈保留任务 ID。

## 2026-08-30 通知渠道多行115任务提交

- 执行者：Codex。
- 专项：`go test ./internal/service -run 'Test(ParseTelegramResourceRequest|SelectTelegramResourceAccount|TelegramResourceService)' -count=1`，通过。
- 渠道反馈专项：`go test ./internal/service -run 'TestTelegramResourceService' -count=1`，通过；断言企业微信纯文本和 Telegram HTML 均包含汇总及两个任务 ID。
- 全量：`go test ./...`，通过。
- 静态检查：`go vet ./...`，通过。
- 覆盖：用户提供的两条 magnet 多行消息、相同逐行目录、每行单 URL/独立任务、全成功汇总、首行失败后继续、部分失败汇总、单行兼容。
- 分享转存专项：两条115分享链接产生两次解析和两次提交，逐行密码与目录正确，企业微信纯文本和 Telegram HTML 均包含两个转存任务 ID；通过。

## 2026-09-01 任务触发与终态双阶段通知

- 执行者：Codex。
- 监控专项：首次启用历史任务不补发；新 pending 任务发送一次“任务已触发”，状态变为 completed 后再发送一次“任务已完成”，重复轮询不重复。
- 配置专项：Telegram/企业微信历史配置缺少 `notify_task_started` 时默认开启；设置页可关闭；Server 酱/SMTP 已启用时默认订阅任务触发与四类任务结果。
- 验证命令：`go test ./...`、`go vet ./...`、`npm run build`。

## 2026-08-30 115 离线下载 UA 回归修复

- 执行者：Codex。
- 失败复现：`go test . -run TestGetOrCreateDriverConfiguresTimeoutAndRefreshesChangedCookie -count=1` 首次失败；输出 `115 Driver User-Agent=""`，证明超时客户端替换后 UA 丢失。
- 修复后定向测试：同一命令通过，同时覆盖两分钟 HTTP 超时、`UA115Browser`、同 Cookie 客户端复用及 Cookie 变化后重建。
- Service 功能测试：`go test ./internal/service -run 'Offline' -count=1` 通过。
- Controller 冒烟/接口测试：`go test ./internal/controller -run 'Offline' -count=1` 通过。
- 后端全量：`go test ./... -count=1` 通过，主包、controller、dao、domain、service 均无失败。
- 静态检查：`go vet ./...` 通过。
- 前端生产构建：`npm run build` 通过，Vite 转换 4844 个模块；仅有既存 `EmbyMonitor` 分块超过 500 kB 的提示。
- 差异检查：`git diff --check` 通过，仅输出工作区既存 LF/CRLF 转换提示。
- 未执行真实115提交：该操作会在用户账号创建实际离线任务；本地回归已直接验证同一故障所缺失的请求头，不需要以外部写操作作为自动测试前提。
- 运行时冒烟：重启本地后端后，8082 由新 `easy-strm` 进程监听；GET `/v1/resource/115-offline/tasks` 无 Token 返回 401 `Authorization header is required`，路由与鉴权初始化正常。

## 2026-08-31 项目功能缺口分析与完善需求文档

- 执行者：Codex。
- 后端功能/冒烟回归：在 `easy-strm` 执行 `go test ./... -count=1`，通过。
- 后端静态检查：在 `easy-strm` 执行 `go vet ./...`，通过。
- 覆盖基线：`go test ./... -cover -count=1` 通过；约 main 4.1%、controller 11.8%、dao 13.2%、domain 83.7%、service 42.8%、logger 0%。
- 跳过项检查：详细输出有 20 个 skip，其中 17 个 TaskService 测试因 localhost Redis 不可用而跳过；已作为 RQ-16 的测试治理输入，未把“命令通过”等同为风险闭环。
- 前端生产构建：在 `easy-strm-front` 执行 `npm run build`，通过；Vite 转换 4844 个模块，仅 `EmbyMonitor` 约 591.36 kB 的既存分块警告。
- 前端轻量单测：`node --test src/components/resource/targetFolderTree.test.mjs`，3/3 通过。
- E2E 静态冒烟：23 个 `e2e-*.mjs` 全部通过 `node --check`。
- 审计产物检查：新增 JSON 全部可解析；文档引用的仓库文件均存在；需求文档包含 16 个 RQ、29 个登记缺口和 4 个里程碑；无个人绝对路径、无尾随空格。
- Compose 诊断：`docker compose -f docker-compose.yml config --images` 返回 `/easy-strm:latest`、`postgres:16-alpine`、`redis:7-alpine`；命令成功解析，但首个镜像引用无效，已作为 P0 缺陷而不是通过项记录。
- 未执行 live 115/Emby/通知 E2E：这些测试会写入外部账号或用户媒体目录，不属于本次只读审计授权；风险已在主需求文档注明。

# 2026-09-05 日志优化验证
- 115 文件列表、CID 解析及重复启动成功日志降为 DEBUG。
- go test ./...：通过。

## 分享管理功能（2026-09-06，Codex）
- go test ./internal/...：通过
- npm run build：通过


## 分享管理 1:N 重构（2026-09-06，Codex）
- go test ./...：通过
- npm run build：通过
- 数据迁移：migrate_v22_share_record_media.sql，旧单文件字段搬迁到 t_share_media，主表删除媒体字段


## 分享管理页面渲染修复（2026-09-06，Codex）
- 显式导入 Naive UI 表单、弹窗、表格、上传组件，消除 Vue failed to resolve component 警告。
- 移除 n-dynamic-input，改为 v-for 媒体行增删，避免 value undefined。
- npm run build：通过

## 分享文案自动解析（2026-09-06，Codex）

- 支持从纯链接、Markdown 链接及完整115分享文案提取 URL。
- 优先读取 URL 的 `password` 参数，缺失时读取“访问码/提取码/密码”。
- `go test ./...`：通过。
- `npm run build`：通过。
## 分享批量识别任务（2026-09-06，Codex）

- 批量识别接口改为异步任务，任务类型为 `share_identify`，使用现有 Redis 任务中心。
- 任务元数据包含准备媒体列表、识别媒体、汇总结果三个步骤及当前文件。
- 页面轮询任务详情并显示进度、成功数、失败数和当前文件。
- 表格使用 `:row-key="row => row.id"`，消除重复 key 和 `getKey is not a function`。
- `go test ./...`：通过；`npm run build`：通过；浏览器实际点击验证任务卡可见。
## 2026-09-06 分享管理回归验证（Codex）
- 浏览器验证：完整 115 文案新增后名称显示为“猎虎贰 (2026)等2个文件(夹)”；操作列显示编辑/删除；点击批量自动识别后创建任务并展示“准备媒体列表 → 获取分享媒体 → 识别媒体 → 汇总结果”步骤。
- 清理了浏览器验证产生的临时分享记录。

[2026-09-06] Codex：分享媒体详情升级。识别结果补充 poster_path，前端识别成功的媒体子行显示海报、中文名、原名、年份、类型和文件名；未识别分享记录禁用展开，仅保留主记录操作。
验证：go test ./... 通过；npm run build 通过；浏览器验证已识别子行可展开并显示“灿烂人生”等中文媒体信息，未识别状态由 expandable 条件阻止展开。

[2026-09-06] Codex：分享解析改为递归遍历所有子目录，并保留媒体相对路径；批量识别使用相对路径作为文件名，避免不同目录同名文件被去重漏掉；补充更多视频扩展名识别。大分享可完整建立媒体记录后统一识别。
验证：go test ./... 通过；npm run build 通过；后端已重启。

[2026-09-06] Codex：分享表格操作列新增“识别”按钮。新增单条分享识别接口，创建独立任务，仅解析并识别当前分享下的全部媒体；前端复用任务进度卡展示处理状态。
验证：go test ./... 通过；npm run build 通过；浏览器确认每条分享行显示识别按钮；后端已重启。

[2026-09-06] Codex：修复分享识别任务长期 pending/无错误问题。任务启动即标记 running；解析阶段记录当前分享、解析数量和日志；解析失败、媒体写入失败、超时和 panic 均写入任务错误及 metadata.parse_errors；任务增加 30 分钟超时和取消注册。服务启动时自动将遗留 pending/running 任务标记为失败并提示重新执行。分享识别继续复用 TmdbService 当前识别测试规则和详情补全逻辑。
验证：go test ./... 通过；npm run build 通过；后端已重启。

[2026-09-06] Codex：分析 swwhkzz3z5o 异常。22:18:14 失败发生在已成功读取大量目录后的单个 share/snap 子目录请求，底层为 Fake-IP/代理链路上的 wsarecv connection forcibly closed，非访问码或目录权限错误。新增 share/snap 目录分页级 4 次重试与线性退避，业务错误不重试；记录 share、dir、path、offset、attempt。识别测试与分享识别共用同一 TmdbService 实例及 filename recognition rule store，分享识别调用同一文件名解析链路并补充详情元数据。

[2026-09-06] Codex：按需求将分享解析分页大小调整为100；完整解析最多下探两层（根目录深度0，读取深度1和深度2目录的直接子项），深度2目录下的 Season 1/Season 2 等目录只作为结构项保留，不继续读取其媒体内容，后续可单独解析。后端已重启。
验证：go test ./... 通过；npm run build 通过。

[2026-09-06] Codex：修复深度限制后媒体数为0的问题。受控解析会将“目录下直接包含视频”或第二层目录下包含季目录的剧集/电影目录标记为 type=media；批量识别保留这些目录作为媒体候选，跳过普通目录和季目录内容。任务总数和主表媒体数因此按剧集/电影候选数量回写。
验证：go test ./... 通过；npm run build 通过；后端已重启。

## 2026-09-07 Emby 用户媒体库权限（Codex）
- 根因：用户权限选项复用 VirtualFolders.ItemId；官方 SelectableMediaFolders 同时返回 Id 和 Guid，EnabledFolders 应使用权限 Guid。
- 实施：新增 user-libraries 控制器/API/领域返回结构；用户弹窗显示名称并选择 Guid；后端规范化旧数字 ID、保留未编辑的 Policy 字段、写入后回读；恢复全部媒体库开关的明确语义。
- 工具与过程：functions.exec/rg/Get-Content 检索 Service、Controller、API、Vue 和现有测试；Invoke-WebRequest 读取官方文档；apply_patch 小步修改；go test 先复现原样传数字 ID、遗漏策略字段、未回读造成假成功，再验证修复；Playwright 验证页面交互。
- 工具降级：sequential-thinking、shrimp-task-manager、code-index、exa 不在工具目录中，使用本地分析、rg 和官方文档 HTTP 请求。
- 初次浏览器测试失败：3001 未提供可连接前端；启动独立 3017 Vite。后续发现通配 API 拦截误匹配 src/utils/api 模块，限制为 /api/ 根路径后通过。
- 验证：go test ./... 通过；新增 Service 权限往返 6 场景、选项查询 3 场景、Controller 3 场景通过；浏览器 7 场景通过；npm run build 通过（已有大块警告）。
- 浏览器场景：名称回显、Guid 提交、保存后重开、全部媒体库、空范围、旧数字 ID、保存失败保留弹窗。截图：debug/emby-user-access/names-after-save.png。
- 限制：Go 测试使用 httptest/sqlmock/miniredis；浏览器使用可回读模拟接口，未联调或修改真实 Emby 用户。需要部署更新的前后端才能使用新接口。

2026-09-07 Codex：日志输出接线回归通过；go test ./... 全包通过，npm run build 通过。服务日志与主程序共用 info/debug 输出，旧控制台日志不会回填。

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

2026-09-07 Codex：分享主表分页回归（404 媒体、其他分享、关键词、页码）通过；go test ./... 全通过；npm run build 通过，既有 chunk 警告。

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


## 2026-09-09（Codex）

- 后端 `go test ./...`：通过。
- 前端 `npm run build`：通过。
- 新增纯逻辑测试覆盖资源库筛选参数校验和五/六段 Cron 解析。
- PGlite 真实 PostgreSQL 引擎迁移回放：通过。
- Playwright：`e2e-share-gallery.mjs`、`e2e-library-schedules.mjs` 通过；新调度/资源库流程使用模拟 API，未做真实后端联调。
## 2026-09-09 资源库年份别名修复（Codex）

- 用户实测出现 PostgreSQL `syntax error at or near "year"`。DAO 页面投影的裸别名改为 `media_year AS "year"`，接口字段保持不变。
- 本地 PGlite 脚本增加从 DAO 提取实际页面投影的回归：修复前复现错误，修复后通过；此前仅执行聚合 CTE，未覆盖页面投影。
- `go test ./...` 全部通过；已构建并重启本地后端。前端代码无需变更。
- 使用 functions.exec、apply_patch、PowerShell、Go 和 PGlite；专用思考/规划/索引工具不可用，沿用本地分析和 rg 检索。
## 2026-09-09 分享管理任务刷新恢复（Codex）

- 分享管理挂载时调用统一任务列表，筛选最新的 `share_identify` 且状态为 `pending/running` 的任务，再复用原详情接口和 1 秒轮询恢复进度。
- 终态任务不会恢复轮询；原有新建任务流程保持不变。
- 前端 `npm run build` 通过。


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
