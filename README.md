# easy-strm

轻量级媒体整理与 `.strm` 生成工具，采用 Go 后端与 Vue 3 前端。

当前核心链路：

```text
媒体源接入 -> 文件浏览 -> TMDB 识别 -> 重命名/整理 -> 刮削 -> STRM 生成 -> 任务追踪
```

## 项目结构

```text
easy-strm/
├── easy-strm/         # Go 后端，Gin + PostgreSQL + Redis
├── easy-strm-front/   # Vue 3 前端，Vite + Naive UI
├── deploy/            # Docker 部署文件
├── docker/            # 容器运行配置
├── docs/              # 产品、架构、开发与测试文档
└── debug/             # 本地调试与 E2E 产物
```

## 当前功能

- 登录与工作台概览
- 本地与 115 媒体源管理
- 文件浏览、搜索、移动、复制、删除与重命名
- TMDB 自动识别、候选修正与识别缓存
- 电影/剧集命名模板、整理预览、分类策略与冲突处理
- NFO、海报、背景图和剧照刮削
- STRM 配置、全量生成、115 直链与任务状态
- 本地目录监控、115 目录轮询与自动整理
- Emby 媒体库刷新
- Cron、通知、日志、网络检测和缓存管理

资产台账、同步入库、待处理清单已经下线，系统不再创建或访问对应同步索引数据。系统设置中的全局 Alist 配置也已移除。

## 本地开发

后端：

```powershell
Set-Location .\easy-strm
go run .
```

前端：

```powershell
Set-Location .\easy-strm-front
npm install
npm run dev
```

## Docker 部署

```powershell
Set-Location .\deploy
Copy-Item .env.example .env
docker compose up -d
```

## 验证

```powershell
Set-Location .\easy-strm
go test ./...
go vet ./...
```

```powershell
Set-Location .\easy-strm-front
npm run build
```

更多信息见：

- [产品与架构](docs/产品与架构.md)
- [开发与测试](docs/开发与测试.md)
