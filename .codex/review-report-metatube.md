# MetaTube 数据源实现审查

- 日期：2026-09-02
- 执行者：Codex
- 任务 ID：metatube-source-20260902
- 技术评分：94/100
- 战略评分：95/100
- 综合评分：95/100
- 结论：通过

实现沿用现有 `TmdbService`、缓存、整理与 `ScrapeService`，没有新增平行识别流程。MetaTube 官方电影接口已由本地 HTTP 契约测试覆盖；字符串 ID 通过稳定合成 ID 兼容旧接口，并保留原始 source/provider/id。访问令牌不通过通用设置读取接口回显。剧集不错误调用 MetaTube，继续使用 TMDB。

验证：后端 `go test ./...` 通过；前端 `npm run build` 通过；`git diff --check` 无新增格式错误。
