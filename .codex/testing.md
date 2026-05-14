# Testing Log

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
