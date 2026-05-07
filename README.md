# easy-strm

轻量级媒体整理与 `.strm` 生成工具，当前仓库采用 Go 后端 + Vue 3 前端，围绕以下主链路工作：

`媒体源接入 -> 文件浏览 -> TMDB 识别 -> 重命名/整理 -> STRM 生成 -> 任务追踪`

## 项目结构

```text
easy-strm/
├── easy-strm/         # Go 后端
├── easy-strm-front/   # Vue 3 前端
├── deploy/            # Docker 部署文件
├── docs/              # 合并后的长期维护文档
└── debug/             # 本地调试与 E2E 产物
```

## 技术栈

| 组件 | 技术 |
| --- | --- |
| 后端 | Go 1.25 + Gin |
| 前端 | Vue 3 + Vite + Element Plus |
| 存储 | PostgreSQL + Redis |
| 测试 | Go test + 前端构建 + Playwright 脚本 |

## 本地开发

### 1. 启动后端

```powershell
Set-Location .\easy-strm
go mod tidy
go run .
```

### 2. 启动前端

```powershell
Set-Location .\easy-strm-front
npm install
npm run dev
```

默认开发地址以本地实际启动结果为准。

## Docker 部署

```powershell
Set-Location .\deploy
Copy-Item .env.example .env
# 按需修改 .env
docker compose up -d
```

常用命令：

```powershell
docker compose up -d
docker compose logs -f
docker compose restart
docker compose down
```

部署目录位于 [deploy](/C:/Users/a3875/Documents/code/easy-strm/deploy)。

## 核心文档

文档已收敛为以下 2 份长期维护文档：

- [产品与架构](/C:/Users/a3875/Documents/code/easy-strm/docs/产品与架构.md)
- [开发与测试](/C:/Users/a3875/Documents/code/easy-strm/docs/开发与测试.md)

## 当前能力概览

- 媒体源管理：本地与 115 媒体源
- 文件浏览：目录浏览、筛选、批量选择
- 元数据处理：TMDB 识别、手动修正、命名模板
- 整理能力：预览、复制/移动、冲突策略、结果回看
- STRM：配置管理、生成任务、任务追踪
- 自动化：监控、自动整理、统一任务中心

## 验证基线

日常改动至少应覆盖：

- Go 单元测试
- 前端 `npm run build`
- 关键浏览器主流程回归

详细规范与测试现状见 [开发与测试](/C:/Users/a3875/Documents/code/easy-strm/docs/开发与测试.md)。
