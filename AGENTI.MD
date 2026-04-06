# AGENTI

## 1. 项目概览

`easy-strm` 是一个以 Go + Vue 3 实现的全栈项目，最初核心能力是：

- 管理 115 网盘账号
- 解析 115 文件直链
- 生成与同步 `.strm` 媒体文件
- 提供任务、日志、系统设置等后台能力

当前项目已经明显扩展到第二条业务主线：

- `MediaManager` 媒体管理模块
- 支持本地目录与 115 云盘两类媒体源
- 支持文件浏览、文件操作、TMDB 识别、重命名、整理流程

从当前工作区状态看，`MediaManager` 是最近一轮开发重点，且仍处于“功能基本成型、但还在持续打磨”的阶段。

## 2. 技术栈

### 后端

- Go `1.25.0`
- Gin
- PostgreSQL
- Redis
- robfig/cron
- 115 SDK: `github.com/SheltonZhu/115driver`
- TMDB 调用使用标准 `net/http`

### 前端

- Vue 3
- Vite
- Element Plus
- Axios
- Vue Router

## 3. 仓库结构

```text
easy-strm/
├─ easy-strm/                # Go 后端
│  ├─ internal/
│  │  ├─ controller/         # HTTP 控制器
│  │  ├─ dao/                # 数据访问层
│  │  ├─ domain/             # 领域模型与请求/响应结构
│  │  ├─ pkg/                # 公共基础能力（如日志）
│  │  ├─ router/             # 目前基本未承载主路由注册
│  │  └─ service/            # 业务服务层
│  ├─ migrations/            # SQL 迁移脚本
│  ├─ main.go                # 应用入口
│  ├─ auth.go                # 认证、中间件、主路由注册核心文件
│  ├─ db.go                  # 数据库初始化与部分数据访问实现
│  └─ 115client.go           # 115 客户端能力
├─ easy-strm-front/          # Vue 前端
│  ├─ src/views/             # 页面级组件
│  ├─ src/utils/api/         # 前端 API 封装
│  └─ src/main.js            # 前端入口与路由注册
├─ docs/                     # 产品文档与设计文档
├─ deploy/                   # 部署相关文件
├─ docker/                   # Docker 配置
└─ README.md
```

## 4. 后端代码理解

### 4.1 启动流程

后端入口在 [main.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/main.go)：

1. 初始化 Windows 控制台 UTF-8
2. 加载配置
3. 初始化 Gin 与 CORS
4. 初始化日志
5. 创建 115 客户端
6. 初始化 PostgreSQL
7. 初始化 Redis
8. 迁移 Redis Key
9. 补齐默认管理员
10. 初始化 Cron
11. 注册认证路由与业务路由
12. 启动 `:8082`

### 4.2 路由组织方式

当前项目的路由注册并没有集中在 `internal/router`，而是主要写在 [auth.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/auth.go)：

- `SetupAuthRoutes(...)`
  - 登录、JWT、115 登录二维码相关接口
- `SetupAuthProtectedRoutes(...)`
  - 初始化 DAO / Service / Controller
  - 注册用户信息、115 管理、STRM、任务、设置、MediaManager 等需要认证的接口

这意味着：

- `auth.go` 是理解后端整体 API 装配关系的第一入口
- 新增后端接口时，不能只看 `controller`，还要回到 `auth.go` 检查是否已注册

### 4.3 分层职责

#### domain

位于 [internal/domain](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/domain)。

负责定义：

- 用户、115 账号、STRM 配置、任务等核心模型
- `MediaSource`、`MediaFile`、TMDB、重命名、整理等 MediaManager 相关结构
- 前后端交互使用的请求/响应结构体

#### dao

位于 [internal/dao](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/dao)。

职责：

- 直接访问 PostgreSQL / Redis
- 当前通过 [init.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/dao/init.go) 中的全局 `DB`、`RedisClient` 注入连接

注意：

- `dao.Init(db, redis)` 必须先执行，否则 DAO 层会因为全局连接未初始化而失败
- 这也是 [auth.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/auth.go) 中 `SetupAuthProtectedRoutes` 一开始就调用 `dao.Init(...)` 的原因

#### service

位于 [internal/service](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/service)。

职责：

- 承接业务规则
- 协调 DAO、外部服务和文件系统操作

当前重点服务：

- `Cloud115Service`
- `StrmService`
- `MediaSourceService`
- `FileOperationService`
- `TmdbService`
- `RenameService`
- `OrganizeService`

#### controller

位于 [internal/controller](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/controller)。

职责：

- 参数绑定
- HTTP 状态码与响应包装
- 调用 service 完成业务逻辑

一个比较明显的现实情况是：

- 本地媒体源相关逻辑多数下沉到了 service
- 115 云盘文件浏览部分由于依赖运行中的 115 client，仍保留了较多 controller 层处理

### 4.4 MediaManager 当前实现状态

这是当前项目最值得优先理解的部分。

核心文件：

- [media_source_controller.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/controller/media_source_controller.go)
- [file_operation_controller.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/controller/file_operation_controller.go)
- [organize_controller.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/controller/organize_controller.go)
- [rename_controller.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/controller/rename_controller.go)
- [tmdb_controller.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/controller/tmdb_controller.go)

配套服务：

- [media_source_service.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/service/media_source_service.go)
- [file_operation_service.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/service/file_operation_service.go)
- [organize_service.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/service/organize_service.go)
- [rename_service.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/service/rename_service.go)
- [tmdb_service.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/service/tmdb_service.go)

目前已经可以看出的能力边界：

- 媒体源 CRUD
- 本地目录浏览
- 115 云盘目录浏览
- 文件移动 / 复制 / 删除 / 重命名
- TMDB 搜索与识别
- 批量识别、批量重命名
- 自动整理链路

但也有几个重要现实点：

- `MediaSourceService` 对本地媒体源支持较完整
- 115 文件浏览逻辑主要在 controller 中临时完成，说明这一块分层还可以继续收敛
- `FileOperationService` 中部分 115 云盘操作仍带有“占位实现”痕迹，并不全是完整 API 落地
- TMDB 功能依赖系统配置中的 `tmdb_api_key`

### 4.5 旧业务主线

原有主线仍然保留并可继续迭代：

- 115 账号管理
- STRM 配置管理
- 全量 / 增量生成
- 任务监控
- 日志查看
- 系统配置

这部分逻辑分散在：

- [auth.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/auth.go)
- [db.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/db.go)
- [strm.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/strm.go)
- [task.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/task.go)
- [cron.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/cron.go)

## 5. 前端代码理解

### 5.1 前端入口

前端入口在 [src/main.js](/C:/Users/a3875/Documents/code/easy-strm/easy-strm-front/src/main.js)：

- 创建 Vue 应用
- 注入 Router
- 注入 Element Plus
- 注册页面路由

主页面结构：

- `/login`
- `/dashboard/user-info`
- `/dashboard/cloud115`
- `/dashboard/strm-config`
- `/dashboard/media-manager`
- `/dashboard/settings`

### 5.2 API 封装

前端 API 位于 [src/utils/api](/C:/Users/a3875/Documents/code/easy-strm/easy-strm-front/src/utils/api)。

其中：

- [request.js](/C:/Users/a3875/Documents/code/easy-strm/easy-strm-front/src/utils/api/request.js)
  - 基于 Axios
  - `baseURL` 为 `/api`
  - 请求拦截器会自动带上本地 `token`
  - 401 时会清理凭据并跳回登录页
- [media.js](/C:/Users/a3875/Documents/code/easy-strm/easy-strm-front/src/utils/api/media.js)
  - 是 MediaManager 的主要 API 汇总文件

### 5.3 Dashboard

[Dashboard.vue](/C:/Users/a3875/Documents/code/easy-strm/easy-strm-front/src/views/Dashboard.vue) 是后台壳层：

- 左侧菜单
- 页面标题切换
- 日志弹窗
- 任务弹窗
- 退出登录

它既是布局入口，也是很多全局后台能力的承载点。

### 5.4 MediaManager 页面

[MediaManager.vue](/C:/Users/a3875/Documents/code/easy-strm/easy-strm-front/src/views/MediaManager.vue) 是当前最重的前端页面之一，承担了：

- 媒体源表格
- 文件浏览弹窗
- 搜索 / 分页 / 面包屑
- TMDB 识别弹窗
- 单文件与批量重命名
- 删除操作

这是一个典型的“单文件承载很多交互状态”的页面，后续如果继续扩展，建议优先考虑拆分子组件或抽离组合式逻辑。

### 5.5 前后端数据风格

当前前后端之间的响应格式并不完全统一，前端里已经能看到多处类似处理：

- `response.data.data`
- `response.data.data.data`

这说明历史接口包装风格并不完全一致。后续新增接口时，最好统一响应结构，避免前端继续堆分支兼容。

## 6. 数据与迁移

数据库迁移目录在 [migrations](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/migrations)。

当前可见的关键迁移包括：

- `migrate_v3.sql`
- `migrate_v4.sql`
- `migrate_v5.sql`
- `migrate_v6_media_manager.sql`
- `migrate_v7_tmdb.sql`

这说明数据库演进顺序大致是：

1. 原始业务表结构
2. 引入 MediaManager
3. 引入 TMDB 相关缓存与扩展能力

如果后续继续开发 MediaManager，优先检查：

- 是否已有配套迁移
- 新增字段是否同时体现在 domain / dao / service / controller / frontend form 中

## 7. 测试现状

当前项目已经为 MediaManager 的一部分后端能力补了测试：

- [media_source_dao_test.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/dao/media_source_dao_test.go)
- [media_source_service_test.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/service/media_source_service_test.go)
- [file_operation_service_test.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/service/file_operation_service_test.go)

并提供了批处理脚本：

- [run_tests.bat](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/run_tests.bat)

该脚本的目标比较明确：

- 清测试缓存
- 跑 DAO 层指定测试
- 跑 Service 层指定测试
- 合并覆盖率并输出 HTML 报告

说明项目目前的测试重点仍集中在后端数据与服务层，前端自动化覆盖暂时不明显。

## 8. 当前工作区状态

当前仓库不是干净工作区，已经存在较多未提交变更，重点集中在：

- `easy-strm-front/src/views/MediaManager.vue`
- `easy-strm-front/src/utils/api/media.js`
- 多个 MediaManager / TMDB / Rename / Organize 后端文件
- `db.go`、`auth.go`、`115client.go` 等核心文件
- 新增迁移脚本与测试文件

这意味着：

- 后续改动前必须先确认哪些文件已经被用户或其他流程修改
- 不要假设当前工作区代表“稳定主线”
- 任何修复或重构都应尽量做增量式修改，不要贸然大面积改写

## 9. 协作建议

### 9.1 新增后端功能时

建议按下面顺序检查：

1. `domain` 是否需要新结构
2. `dao` 是否已有表结构支撑
3. `service` 是否需要承接业务规则
4. `controller` 是否需要新增接口
5. `auth.go` 是否完成路由注册
6. 前端 `src/utils/api` 是否已封装
7. 页面或组件是否已接入

### 9.2 修改 MediaManager 时

优先看这些文件：

- [media_source_controller.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/controller/media_source_controller.go)
- [media_source_service.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/service/media_source_service.go)
- [MediaManager.vue](/C:/Users/a3875/Documents/code/easy-strm/easy-strm-front/src/views/MediaManager.vue)
- [media.js](/C:/Users/a3875/Documents/code/easy-strm/easy-strm-front/src/utils/api/media.js)

### 9.3 修改接口时

特别注意：

- 当前响应结构存在历史包袱
- 前端有不少地方做了嵌套兼容处理
- 修改接口返回格式时，要同步检查所有消费点

### 9.4 修改 115 相关能力时

建议优先核实三件事：

1. 是否只是本地模拟 / 占位逻辑
2. 是否真正通过 115 client 落地
3. controller 中是否有因运行时依赖而保留的特殊处理

## 10. 建议的后续阅读顺序

如果是第一次正式接手这个项目，推荐阅读顺序：

1. [easy-strm/main.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/main.go)
2. [easy-strm/auth.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/auth.go)
3. [easy-strm/internal/domain/models.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/domain/models.go)
4. [easy-strm/internal/domain/media_source.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/domain/media_source.go)
5. [easy-strm/internal/service/media_source_service.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/service/media_source_service.go)
6. [easy-strm/internal/controller/media_source_controller.go](/C:/Users/a3875/Documents/code/easy-strm/easy-strm/internal/controller/media_source_controller.go)
7. [easy-strm-front/src/main.js](/C:/Users/a3875/Documents/code/easy-strm/easy-strm-front/src/main.js)
8. [easy-strm-front/src/views/Dashboard.vue](/C:/Users/a3875/Documents/code/easy-strm/easy-strm-front/src/views/Dashboard.vue)
9. [easy-strm-front/src/views/MediaManager.vue](/C:/Users/a3875/Documents/code/easy-strm/easy-strm-front/src/views/MediaManager.vue)
10. [easy-strm-front/src/utils/api/media.js](/C:/Users/a3875/Documents/code/easy-strm/easy-strm-front/src/utils/api/media.js)

## 11. 一句话结论

这是一个“原有 115 + STRM 后台系统，正在演进为带媒体管理能力的平台”的项目。

现在最值得关注的代码区域是：

- `auth.go` 的路由装配
- `internal/service` 的业务扩展
- `MediaManager.vue` 的前端复杂交互
- MediaManager 相关迁移、测试与未提交改动
