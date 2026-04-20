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
