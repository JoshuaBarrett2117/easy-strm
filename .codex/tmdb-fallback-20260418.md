# TMDB Fallback Note

日期: 2026-04-18
执行者: Codex

## 记录

- 为 TMDB 识别增加标题变体回退搜索。
- 适配中文片名的标点差异与紧凑标题。
- 回归用例覆盖 `哆啦A梦：大雄的恐龙`。

## 验证

- `go test ./internal/service/...`
- `go test ./...`

