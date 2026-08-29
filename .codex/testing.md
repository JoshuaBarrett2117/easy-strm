# 2026-08-18 云下载大批量队列提交

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
