# Easy-STRM

面向个人媒体库的媒体整理与 `.strm` 生成工作台。项目使用 Go + Vue 3 构建，将本地目录、115 云盘和 115 分享中的影视文件接入一条可追踪的处理链路：浏览、识别、整理、刮削、生成 STRM，并可刷新 Emby 媒体库。

```text
媒体源 / 115 分享
  -> 文件浏览或分享扫描
  -> TMDB / MetaTube / AI 辅助识别
  -> 重命名、分类与整理预览
  -> 移动、复制或链接
  -> NFO 与图片刮削 / STRM 导出
  -> 任务中心、定时任务与通知追踪
```

## 功能特性

### 媒体整理

- 接入本地目录和 115 云盘媒体源，支持启停、排序、监控目录、整理目标、媒体类型和冲突策略配置。
- 文件工作台提供分页浏览、搜索、筛选、批量选择与面包屑导航；支持本地与 115 文件的移动、复制、删除和重命名，本地还支持硬链接、软链接。
- 统一文件管理支持本地与 115 账号之间的双栏浏览、复制、剪切和粘贴；跨存储传输由后台任务执行，并支持取消和结果追踪。
- 支持电影、剧集、季集和特别篇的文件名解析、TMDB 候选匹配、手动修正与识别缓存；电影可选择 MetaTube 作为数据源。
- 提供独立识别测试页，可验证文件名解析与匹配结果，并维护识别规则。
- 可按分类策略和命名模板生成整理预览，再执行移动、复制、硬链接或软链接；支持冲突处理和可选的元数据刮削。
- 刮削可生成 NFO、海报、背景图和剧照等媒体服务器可识别的元数据文件。

### STRM 与播放

- 可为本地或 115 媒体源按配置批量生成 STRM；115 文件使用直链播放，并在任务中心展示进度、失败原因与取消状态。
- 分享资源库可将已识别的 115 分享文件导出为 STRM：按电影/剧集目录结构落盘，并通过稳定播放地址按需转存、获取 115 直链后返回 302。
- 分享 STRM 支持手动全量导出与定时增量导出；增量导出只补写缺失或变化的文件，普通 STRM 和分享 STRM 可安全共存于同一输出目录。
- 支持播放记录、Emby 媒体库刷新，以及多个 Emby 实例、用户、媒体库和封面相关管理能力。

### 115 资源与分享资料库

- 115 云管理支持 Cookie 扫码登录、账号状态/优先级/用途管理和账号来源标注。
- 资源聚合支持解析 115 分享链接并转存，也支持磁力、ed2k 等 115 云下载任务。
- 分享管理支持批量文本导入、链接与提取码编辑、递归扫描视频文件、批量或单条识别、失败重试、待识别续跑、取消分享标记及一键清空识别结果。
- 分享资源库按作品聚合多份分享来源，可按 TMDB ID、类型、年份、题材、评分、地区和有效分享筛选；电视剧保留季集映射，并支持作品海报墙浏览。
- 分享识别可自动判断电影/剧集类型；对于复杂标题、类型不明或常规检索不可靠的情况，可使用 OpenAI 兼容接口进行 AI 辅助建议，结果仍会经媒体数据源核验。

### 自动化与运维

- 本地目录使用文件监听，115 目录使用定期扫描；新视频可自动进入整理流程。
- 统一定时任务管理 STRM 生成、分享 STRM 增量导出、账号冷却恢复、日志清理和识别缓存清理等任务，支持 Cron 表达式、时区、启停和执行记录。
- 任务中心统一追踪整理、传输、分享识别、云下载、STRM、Emby 操作等异步任务，并支持可取消任务。
- 支持 Telegram 管理员私聊和企业微信应用回调提交分享转存、云下载任务；也可配置通知、日志保留、网络测试、Redis/识别缓存管理。
- 提供 API Key 与 MCP JSON-RPC 入口，便于外部自动化读取任务概览、列表、详情和取消任务。

## 技术栈

| 层级 | 技术 |
| --- | --- |
| 前端 | Vue 3、Vite、Naive UI、Tailwind CSS |
| 后端 | Go 1.25、Gin |
| 数据与任务 | PostgreSQL、Redis、Cron |
| 外部服务 | 115、TMDB、MetaTube、Emby、Telegram、企业微信、OpenAI 兼容 API |
| 部署 | Docker、Nginx、Supervisor |

## 快速开始

### Docker 部署（推荐）

准备 Docker Compose 后，在仓库根目录执行：

```powershell
Copy-Item .env.example .env
```

编辑 `.env`，至少设置以下配置：

- `SERVER_URL`：用户和媒体服务器实际可访问的应用地址；它会写入 115 与分享 STRM 的播放地址。
- `JWT_SECRET`、`PG_PASSWORD`：替换示例中的默认值。
- `APP_PORT`：应用对外端口；如需从宿主机管理 PostgreSQL / Redis，再按需设置对应暴露端口。

然后启动：

```powershell
docker compose up -d
docker compose ps
```

访问 `http://localhost:80`（或配置的 `APP_PORT`）。首次启动会创建默认管理员账号：`admin` / `admin`。请登录后立即修改密码，并妥善保管 `.env` 中的密钥。

> 使用本地媒体源、STRM 输出目录或供 Emby/Jellyfin 读取的目录时，请在 `docker-compose.yml` 中为应用增加所需的宿主机目录挂载，并在系统内填写容器内的绝对路径。例如将宿主机 `D:\Media` 挂载为容器内 `/media` 后，媒体源和输出目录应使用 `/media`。

也可以使用发布镜像的部署文件：

```powershell
Set-Location .\deploy
Copy-Item .env.example .env
docker compose up -d
```

### 本地开发

前置条件：Go 1.25+、Node.js 20+、PostgreSQL 16+、Redis 7+。

先根据 `.env.example` 准备数据库和 Redis 连接环境变量，再分别启动后端与前端：

```powershell
Set-Location .\easy-strm
go run .
```

```powershell
Set-Location .\easy-strm-front
npm install
npm run dev
```

## 常用流程

1. 在“115 云管理”完成账号登录，或在“文件工作台”新建本地/115 媒体源。
2. 在文件工作台浏览并选择视频，识别媒体信息后确认重命名与整理预览。
3. 需要元数据时开启刮削；需要在线播放时配置 STRM 输出与访问地址。
4. 对分享资源，先在“分享管理”导入链接并完成扫描/识别，再到“分享资源库”筛选作品并导出 STRM。
5. 在任务中心查看进度、失败详情和可取消任务；按需在“定时任务管理”创建自动任务。

## 使用边界

- 项目面向个人媒体库管理，不提供 PT、下载器、资源搜索、订阅或插件市场能力。
- 本地目录访问被限制在已配置媒体源的根目录范围内。
- 115、TMDB、MetaTube、Emby、通知与 AI 能力取决于你的网络和账号配置；请只配置自己有权使用的服务与媒体内容。
- 分享 STRM 首次播放可能触发文件转存，需为目标 115 账号预留可用空间；媒体服务器需要能够访问配置的播放地址。
- 当前为单工作台登录模式，不提供多用户角色体系。

## 验证

后端：

```powershell
Set-Location .\easy-strm
go test ./...
go vet ./...
```

前端：

```powershell
Set-Location .\easy-strm-front
npm run build
```

相关 E2E：

```powershell
Set-Location .\easy-strm-front
npm run e2e:organize-preview-refresh
npm run e2e:organize-preview-cancel
npm run e2e:emby-monitor
```

## 项目结构

```text
easy-strm/
├── easy-strm/         # Go 后端：Gin、业务服务、DAO、数据库迁移
├── easy-strm-front/   # Vue 3 前端：页面、组件和 API 封装
├── deploy/            # 使用 Docker Hub 镜像的部署文件
├── docker/            # Nginx、Supervisor 与容器启动配置
├── docs/              # 产品、架构、开发与测试文档
└── debug/             # 本地调试与 E2E 产物
```

## 文档

- [产品与架构](docs/产品与架构.md)
- [开发与测试](docs/开发与测试.md)
- [115 转存、自动整理与刮削设计](docs/design-115-transfer-auto-organize-scrape.md)
- [Emby 管理说明](docs/Emby管理.md)

## 许可证

仓库当前未提供许可证文件。使用、修改或分发前，请先与仓库维护者确认适用授权。
