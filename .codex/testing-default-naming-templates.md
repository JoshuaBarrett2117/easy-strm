# 默认命名模板测试记录

- 日期：2026-08-30
- 执行者：Codex
- 任务：将电影和剧集 TMDB 目录模板设为系统默认模板

## 测试结果

1. 模板专项测试：通过。
   - 命令：`go test ./internal/service -run "TestApplyTemplate|TestGetDefaultTemplate|TestNormalizeBuiltinTemplate|TestOrganizeService_BuildTargetPath"`
   - 结果：`ok easy-strm/internal/service`
2. 后端全量测试：通过。
   - 命令：`go test ./...`
   - 结果：根包、controller、dao、domain、service 全部通过。
3. 前端生产构建：通过。
   - 命令：`npm run build`
   - 结果：Vite 完成 4254 个模块转换并成功产出构建文件。
4. 差异检查：通过，无空白错误。
5. 后续静态检查：被工作区中与本任务无关的 Emby 管理并行改动阻塞，缺少 `waitForLibraryIdle`、`finishUnknownTask` 等方法；该问题在本次全量测试通过后出现，本任务未修改相关文件。

## 覆盖范围

- 电影目录和文件名包含年份、TMDB ID、画质及扩展名。
- 剧集目录使用未补零的 `Season N`，文件名季集号使用补零的 `SxxExx`。
- 剧集英文标题、画质、来源和编码按有值条件输出。
- 两代历史官方默认模板自动归一化为新默认模板。
- 自定义模板不参与数据库迁移。
