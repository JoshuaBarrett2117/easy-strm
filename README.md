# easy-strm

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![Vue.js](https://img.shields.io/badge/Vue%203-4FC08D?style=flat-square&logo=vue.js)](https://vuejs.org/)
[![Gin](https://img.shields.io/badge/Gin-1.11-00A1D9?style=flat-square)](https://gin-gonic.com/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=flat-square&logo=postgresql)](https://postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-7-DC382D?style=flat-square&logo=redis)](https://redis.io/)

轻量级 115 网盘直链提取与 `.strm` 媒体文件生成工具，支持 Emby / Jellyfin / Plex 无缝接入。

## 技术架构

| 组件 | 技术选型 | 作用 |
| :--- | :--- | :--- |
| 前端 | Vue 3 + Element-Plus | 响应式交互与美观 UI |
| 后端 | Golang + Gin | 高并发任务处理 |
| 存储 | PostgreSQL + Redis | 数据持久化与任务队列 |

## 快速开始

### 环境依赖

- Go 1.24+
- Node.js 18+
- PostgreSQL 14+
- Redis 7+

### 本地开发

```powershell
# 前端
cd easy-strm-front
npm install
npm run dev

# 后端
cd easy-strm
go mod tidy
go run .
```

### Docker 部署

```bash
cd deploy
cp .env.example .env
# 编辑 .env 配置
docker-compose up -d
```

## 项目结构

```
easy-strm/
├── easy-strm/                 # Golang 后端
│   ├── internal/              # 分层架构
│   │   ├── domain/           # 领域模型
│   │   ├── dao/              # 数据访问层
│   │   ├── service/          # 业务逻辑层
│   │   └── controller/       # 接口控制层
│   ├── migrations/           # 数据库迁移
│   └── main.go               # 入口
├── easy-strm-front/          # Vue 前端
│   └── src/
│       ├── utils/api/        # API 模块化
│       ├── views/            # 页面组件
│       └── components/       # 公共组件
├── deploy/                   # 部署配置
├── docker/                   # Docker 配置
└── docs/                     # 文档
```

## 核心功能

| 功能 | 说明 |
| :--- | :--- |
| 115 扫码登录 | 多渠道扫码授权，自动续期 |
| 直链解析 | 精准提取 115 网盘真实下载直链 |
| STRM 全量生成 | 一键批量生成 .strm 挂载文件 |
| STRM 增量同步 | 仅对新增/变更文件生成 |
| 定时任务 | Cron 调度自动执行 |
| 任务监控 | 实时进度与状态查看 |
| 秒传同步 | 跨账号 SHA1 秒传文件 |

## API 概览

| 端点 | 方法 | 功能 |
| :--- | :--- | :--- |
| /api/user/login | POST | 用户登录 |
| /api/cloud115 | GET | 账号列表 |
| /api/strm/config | GET/POST | STRM 配置管理 |
| /api/strm/config/:id/generate/full | POST | 全量生成 |
| /api/strm/config/:id/generate/incremental | POST | 增量生成 |
| /api/task | GET | 任务列表 |
| /api/setting | GET/PUT | 系统设置 |

## 开发规范

### 重构原则

遵循 Martin Fowler 《重构》核心思想：单一职责、消除代码坏味道、保持函数短小精悍。

### 日志追踪

```
[INFO]  2026-03-25 17:16:22 [模块名] 中文日志说明
[DEBUG] 2026-03-25 17:16:22 [模块名] 详细调试信息
[ERROR] 2026-03-25 17:16:22 [模块名] 异常错误堆栈
```

## 默认账号

| 字段 | 值 |
| :--- | :--- |
| 用户名 | admin |
| 密码 | admin |

## 文档

- [产品需求文档](./docs/PRD.md)
- [产品路线图](./docs/ROADMAP_V3.md)

## License

MIT
