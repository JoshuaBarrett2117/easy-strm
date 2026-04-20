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
