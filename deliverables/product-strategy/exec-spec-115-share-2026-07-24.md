# 115 分享链接一键转存 — 执行规格

- **需求编号**：RA-001
- **目标读者**：一线开发工程师（后端 Go / 前端 Vue 3）
- **前置 PRD**：[prd-resource-aggregation-2026-07-24.md](./prd-resource-aggregation-2026-07-24.md) 第 6.1 节
- **前置路线图**：[roadmap-resource-aggregation-2026-07-24.md](./roadmap-resource-aggregation-2026-07-24.md) 第 5 节
- **版本**：v1.0
- **日期**：2026-07-24
- **状态**：待开发

---

## 1. 功能概述

### 一句话描述

用户在 easy-strm 粘贴 115 分享链接，系统解析目录树后一键将文件转存到用户的 115 网盘，并可选自动触发 Watch 整理 → STRM → Emby 全链路。

### 核心价值

- **补全"获取 → 整理"断裂链路**：当前用户需在 115 App 手动转存，再回到 easy-strm 触发整理；本功能将两端串联为一次操作。
- **差异化壁垒**：8 个竞品在"115 分享解析 → 自动转存 → STRM"链路上均为空白。

### 目标用户

| 画像 | 使用频率 | 核心场景 |
| --- | --- | --- |
| 老张（NAS 效率派） | 周级 | 批量转存 115 分享链接（数百文件），转存后自动整理 |
| 阿伟（影音发烧友） | 周级 | 偶得高质量分享链接，选择 4K/HDR 文件转存并立即在 Emby 观看 |

### 成功标准

| 指标 | 目标值 | 验证方式 |
| --- | --- | --- |
| 转存成功率 | > 85%（首月 > 90%） | PostHog 埋点 + 任务中心统计 |
| 端到端耗时（一次操作） | < 3s 解析 + < 30s 选文件提交（不含转存传输） | 性能测试 |
| 100 文件批量转存 | < 60s（秒传场景） | 性能测试 |
| 转存使用率（周活用户中） | ≥ 25% | PostHog 埋点 |
| 转存 → 整理成功率 | ≥ 85% | 任务中心统计 |

---

## 2. 后端接口契约

> 现有后端栈：Go + Gin + PostgreSQL + Redis（任务状态）。
> 复用现有 `internal/controller/task_controller.go` 的任务查询模式，新增接口使用 `POST/GET` 标准 RESTful。

### 2.1 `POST /api/v1/resource/115-share/parse`

**描述**：解析 115 分享链接，返回分享目录的文件树（平铺列表，含文件夹层级路径）。

```
POST /api/v1/resource/115-share/parse
  Content-Type: application/json

  Request:
    {
      "url": "https://115.com/s/swexample123?password=demo1",
      "password": "demo1"          // 可选；如果 URL 已含密码则可为空
    }

  Response 200:
    {
      "code": 0,
      "data": {
        "shareCode": "swexample123",
        "folderName": "4K Movie Collection 2024",   // 分享根目录名称
        "files": [
          {
            "name": "The Matrix (1999) 4K HDR.mkv",
            "size": 21474836480,                     // 字节
            "type": "video",                         // video | audio | image | document | folder | other
            "path": "/电影/科幻",                      // 分享中的相对路径
            "pickCode": "xxx",                       // 115 pickcode，用于后续转存
            "sha1": "abc123...",                     // 文件 SHA1
            "isDir": false,
            "children": null
          }
        ],
        "totalFiles": 195,
        "totalSize": 382412345678                     // 字节
      }
    }

  Response 4xx/5xx:
    {
      "code": <error_code>,
      "message": "<用户可读的中文错误消息>"
    }

  Error codes:
    INVALID_URL       - URL 不是合法的 115 分享链接格式
    PASSWORD_REQUIRED - 链接需要提取码但请求中未提供
    SHARE_EXPIRED     - 分享链接已失效/过期
    SHARE_NOT_FOUND   - 分享链接不存在
    PARSE_FAILED      - 解析失败（上游 115 API 返回异常）
    RATE_LIMITED      - 115 限频
    AUTH_EXPIRED      - 当前 115 账号 Cookie 过期
```

**Notes**：
- 分享链接格式支持 3 种：
  1. 标准分享页：`https://115.com/s/<share_code>?password=<xxx>`
  2. 短链接：`https://115.com/s/<share_code>`（需额外传入 password）
  3. SHA1 直链：`https://115.com/s/<share_code>#<sha1>`（单文件分享）
- 大目录（>200 文件）分页返回，第一页默认 200 条，前端可通过 `page`/`pageSize` 参数翻页。
- URL 解析逻辑：正则提取 shareCode（`/s/(\w+)`），URL query 中提取 `password`。

### 2.2 `GET /api/v1/resource/115-share/files`

**描述**：分页获取分享目录文件列表（当 parse 结果超过 200 条时使用）。

```
GET /api/v1/resource/115-share/files?shareCode=swexample123&password=demo1&page=2&pageSize=200&type=video&keyword=蝙蝠侠

  Query params:
    shareCode  - (required) 分享码
    password   - (optional) 提取码
    page       - (optional, default 1) 页码
    pageSize   - (optional, default 200, max 500) 每页数量
    type       - (optional) 筛选类型: video | audio | image | document | folder | other
    keyword    - (optional) 文件名搜索关键词

  Response 200:
    {
      "code": 0,
      "data": {
        "files": [ ... ],      // 同 parse 接口的 files 结构
        "page": 2,
        "pageSize": 200,
        "totalFiles": 1950,
        "totalSize": 1250000000000
      }
    }
```

### 2.3 `POST /api/v1/resource/115-share/transfer`

**描述**：提交批量转存任务（异步），返回任务 ID 供前端轮询。

```
POST /api/v1/resource/115-share/transfer
  Content-Type: application/json

  Request:
    {
      "shareCode": "swexample123",
      "password": "demo1",
      "targetCloud115Id": 1,                            // 目标 115 账号 ID（Cloud115.id）
      "targetDirectory": "/媒体库/待整理",                // 目标目录绝对路径
      "files": [                                         // 要转存的文件 pickCode 列表
        { "pickCode": "xxx", "name": "The Matrix.mkv", "size": 21474836480 },
        { "pickCode": "yyy", "name": "Inception.mkv", "size": 18625288192 }
      ],
      "conflictStrategy": "skip",                        // skip | overwrite | rename
      "autoOrganize": true                               // 转存完成后是否自动触发整理
    }

  Response 200:
    {
      "code": 0,
      "data": {
        "taskId": "share-transfer-<uuid>",               // 如 "share-transfer-a1b2c3d4"
        "totalFiles": 23,
        "estimatedSize": 356000000000
      }
    }

  Error codes:
    INVALID_PARAMS        - 必填字段缺失（如 targetDirectory 为空）
    SHARE_EXPIRED         - 分享链接在确认转存时已失效
    AUTH_EXPIRED          - 115 Cookie 过期
    INSUFFICIENT_SPACE    - 目标目录空间不足（转存前检查）
    DIRECTORY_NOT_FOUND   - 目标目录不存在（注意：系统会自动创建，此错误仅当创建失败时返回）
    NO_MEDIA              - 所选文件列表不包含任何媒体文件（可选警告，不阻止提交）
```

**Notes**：
- 后端立即返回 taskId，实际转存通过 Redis 任务队列异步执行。
- 复用现有 `task.go` 的任务框架，新增 `TaskTypeShareTransfer = "share_transfer"`。
- 文件去重：转存前查询目标目录下是否存在同名同 SHA1 文件，有则秒传跳过。

### 2.4 `GET /api/v1/resource/115-share/transfer/{taskId}`

**描述**：查询单个转存任务的状态和进度。

```
GET /api/v1/resource/115-share/transfer/share-transfer-a1b2c3d4

  Response 200:
    {
      "code": 0,
      "data": {
        "taskId": "share-transfer-a1b2c3d4",
        "status": "transferring",              // pending | transferring | completed | partial_failed | failed | cancelled
        "progress": 68,                        // 百分比 0-100
        "totalFiles": 23,
        "processedFiles": 16,
        "successFiles": 15,
        "failedFiles": 1,
        "skippedFiles": 0,
        "failedItems": [
          {
            "name": "corrupted_file.mkv",
            "error": "SECOND_TRANSFER_FAILED",
            "retryable": true
          }
        ],
        "currentFile": "Inception.mkv",
        "estimatedRemaining": "约 3 分钟",
        "createTime": "2026-07-24 14:30:00",
        "updateTime": "2026-07-24 14:32:15"
      }
    }
```

### 2.5 `POST /api/v1/resource/115-share/transfer/{taskId}/cancel`

**描述**：取消正在进行的转存任务。

```
POST /api/v1/resource/115-share/transfer/share-transfer-a1b2c3d4/cancel

  Response 200:
    {
      "code": 0,
      "data": {
        "taskId": "share-transfer-a1b2c3d4",
        "status": "cancelled",
        "completedFiles": 16,
        "cancelledFiles": 7,
        "message": "已转存 16 个文件，已取消 7 个文件（已转存的不会回滚）"
      }
    }
```

### 2.6 `POST /api/v1/resource/115-share/transfer/{taskId}/retry`

**描述**：重试失败的任务（仅重试失败的文件）。

```
POST /api/v1/resource/115-share/transfer/share-transfer-a1b2c3d4/retry

  Response 200:
    {
      "code": 0,
      "data": {
        "taskId": "share-transfer-a1b2c3d4",
        "retriedFiles": 2,
        "message": "已重新提交 2 个失败文件的转存"
      }
    }
```

### 2.7 接口汇总

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| POST | `/api/v1/resource/115-share/parse` | 解析分享链接 |
| GET | `/api/v1/resource/115-share/files` | 分页查询文件列表 |
| POST | `/api/v1/resource/115-share/transfer` | 提交转存任务 |
| GET | `/api/v1/resource/115-share/transfer/{taskId}` | 查询转存进度 |
| POST | `/api/v1/resource/115-share/transfer/{taskId}/cancel` | 取消转存 |
| POST | `/api/v1/resource/115-share/transfer/{taskId}/retry` | 重试失败项 |

---

## 3. 数据模型

### 3.1 现有任务体系分析

现有系统使用 **Redis** 存储任务状态（见 `internal/dao/task_dao.go`），TaskStatus 结构定义在 `task.go`：

```go
type TaskStatus struct {
    TaskID         string   `json:"task_id"`
    TaskType       TaskType `json:"task_type"`
    TaskName       string   `json:"task_name"`
    Status         string   `json:"status"`   // pending | running | completed | failed | cancelled
    Progress       int      `json:"progress"`
    TotalFiles     int      `json:"total_files"`
    ProcessedFiles int      `json:"processed_files"`
    SuccessFiles   int      `json:"success_files"`
    FailedFiles    int      `json:"failed_files"`
    ErrorMessage   string   `json:"error_message"`
    CreateTime     string   `json:"create_time"`
    // ...
}
```

**决策：复用 Redis 任务框架进行扩展，不新建 PostgreSQL 表。**

### 3.2 扩展方案

#### 3.2.1 新增 TaskType

在 `task.go` 中新增常量：

```go
TaskTypeShareTransfer   TaskType = "share_transfer"    // 115 分享链接转存
TaskTypeShareParse      TaskType = "share_parse"       // 115 分享链接解析
```

#### 3.2.2 扩展 TaskStatus 结构

当前的 `TaskStatus` 基于 `map[string]interface{}` 存储，天然支持动态字段。转存任务在 Redis 中新增以下 `metadata` 字段（通过 `UpdateMetadata` API 写入）：

```json
{
  "metadata": {
    "task_category": "resource_aggregation",    // 固定值，用于任务中心分类筛选
    "task_subtype": "share_transfer",           // 子类型
    "share_code": "swexample123",               // 分享码
    "share_folder_name": "4K Movie 2024",       // 分享目录名
    "target_cloud115_id": 1,                    // 目标 115 账号
    "target_directory": "/媒体库/待整理",         // 目标目录
    "conflict_strategy": "skip",                // skip | overwrite | rename
    "auto_organize": true,                      // 是否自动整理
    "skipped_files": 0,                         // 跳过的文件数（秒传已存在）
    "failed_items": [
      {
        "name": "file1.mkv",
        "error": "SECOND_TRANSFER_FAILED",
        "retryable": true
      }
    ],
    "current_file": "Inception.mkv",            // 当前正在处理的文件
    "source_share_url": ""                      // 前端可选记录（脱敏后）
  }
}
```

#### 3.2.3 Redis Key 设计

| Key | 类型 | 用途 |
| --- | --- | --- |
| `easy_strm:task:share-transfer-{uuid}` | String(JSON) | 转存任务状态 |
| `easy_strm:task:list` | List | 全局任务列表（复用已有 `RPush`） |
| `easy_strm:task:progress:share-transfer-{uuid}` | Set | 已完成转存的 pickCode 集合（支持恢复） |
| `easy_strm:task:cancel:share-transfer-{uuid}` | String | 取消标记 |

#### 3.2.4 转存结果日志（PostgreSQL）

对于需要持久化且查询较多的转存历史记录，新建一张轻量表：

```sql
-- 迁移脚本：migrate_v14_share_transfer_log.sql
CREATE TABLE IF NOT EXISTS t_share_transfer_log (
    id SERIAL PRIMARY KEY,
    task_id VARCHAR(64) NOT NULL,                     -- 对应 Redis 中的 taskId
    share_code VARCHAR(32) NOT NULL,                  -- 115 分享码
    share_folder_name VARCHAR(500),                   -- 分享目录名
    file_name VARCHAR(500) NOT NULL,                  -- 文件名
    file_pick_code VARCHAR(64) NOT NULL,              -- pickCode
    file_size BIGINT DEFAULT 0,                       -- 文件大小（字节）
    file_sha1 VARCHAR(64),                            -- 文件 SHA1
    cloud115_id INTEGER NOT NULL,                     -- 目标 115 账号 ID
    target_directory VARCHAR(1000),                   -- 目标目录
    status VARCHAR(32) NOT NULL DEFAULT 'pending',    -- pending | success | failed | skipped
    error_message TEXT,                               -- 失败原因
    is_second_transfer BOOLEAN DEFAULT FALSE,         -- 是否秒传（已存在）
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_t_sstl_task_id ON t_share_transfer_log(task_id);
CREATE INDEX idx_t_sstl_share_code ON t_share_transfer_log(share_code);
CREATE INDEX idx_t_sstl_cloud115_id ON t_share_transfer_log(cloud115_id);
CREATE INDEX idx_t_sstl_status ON t_share_transfer_log(status);
CREATE INDEX idx_t_sstl_create_time ON t_share_transfer_log(create_time);

COMMENT ON TABLE t_share_transfer_log IS '115 分享转存日志，每条记录对应一个文件的转存结果';
COMMENT ON COLUMN t_share_transfer_log.is_second_transfer IS '是否通过秒传完成（文件已存在于目标网盘）';
```

---

## 4. 组件树 & 交互详细说明

### 4.1 页面路由

```
/dashboard/resources/transfer  →  ResourceTransfer.vue（新增）
```

父路由 `/dashboard/resources` 使用 `ResourceAggregation.vue`（Shell 框架，含 Tab 切换：115 转存 | 追剧订阅 | RSS 监控）。

### 4.2 组件树

```
ResourceAggregation.vue（新增，Tab 容器）
└── ResourceTransfer.vue（新增页面）
    ├── ShareLinkInput.vue
    │   States: default | parsing | parsed | error
    ├── FileSelector.vue
    │   States: loading | loaded | filtered | empty | selected_summary
    ├── TargetFolderPicker.vue
    │   States: collapsed | expanded | selecting
    ├── ConflictPreviewModal.vue（可选）
    │   States: hidden | visible（显示冲突文件列表）
    └── TransferProgress.vue
        States: queued | transferring | completed | partial_failed | failed | cancelled
```

### 4.3 组件详细交互说明

#### 4.3.1 ShareLinkInput.vue

```
┌─ ShareLinkInput ────────────────────────────────────────────────────────┐
│                                                                          │
│  state: default                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  📎 115 分享链接  ┌──────────────────────────────────────────┐    │   │
│  │                  │ https://115.com/s/xxxxx?password=demo1    │    │   │
│  │                  └──────────────────────────────────────────┘    │   │
│  │                  [粘贴] [解析]                                    │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  state: parsing                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  ⏳ 正在解析分享链接...                                           │   │
│  │  ████████████░░░░░░░░░░░░  60%（解析目录结构）                     │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  state: parsed                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  ✅ 解析完成  "4K Movie Collection 2024" · 195 个文件 · 356.2 GB  │   │
│  │  ┌──────────────────────────────────────────────────────────┐    │   │
│  │  │  [重新解析]                                      [清空]    │    │   │
│  │  └──────────────────────────────────────────────────────────┘    │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  state: error                                                           │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  ❌ 解析失败：分享链接已失效，请检查链接是否正确。                    │   │
│  │  ┌──────────────────────────────────────────────────────────┐    │   │
│  │  │  [重试]                                                  │    │   │
│  │  └──────────────────────────────────────────────────────────┘    │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘

交互细节：
- default: 输入框支持 Ctrl+V 粘贴，支持拖入文本；点击"粘贴"按钮读取剪贴板内容。
- parsing: 显示进度条动画，后端返回文件总数后切换为百分比；超时 30s 后自动进入 error 状态。
- parsed: 显示汇总信息（目录名、文件数、总大小）；"重新解析"回到 default 状态。
- error: 针对不同错误码显示不同文案（见第 5 节错误矩阵）。
```

#### 4.3.2 FileSelector.vue

```
┌─ FileSelector ──────────────────────────────────────────────────────────┐
│                                                                          │
│  state: loading                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  ⏳ 正在加载文件列表...                                           │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  state: loaded                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  ☑ 全选 (195)  ☐ 反选  [仅视频 ▽]  [仅音频 ▽]  [仅文档 ▽]       │   │
│  │  ┌────────────────────────────────────────────────────────────┐  │   │
│  │  │ 🔍 搜索文件名...                                           │  │   │
│  │  └────────────────────────────────────────────────────────────┘  │   │
│  │──────────────────────────────────────────────────────────────────│   │
│  │ ▼ 📁 电影 (120)                                                  │   │
│  │   ☑ The Matrix (1999) 4K HDR.mkv................  20.0 GB   🎬 │   │
│  │   ☑ Inception (2010) BluRay 1080p.mkv.........   15.5 GB   🎬 │   │
│  │   ☐ Batman vs Superman (2016).mp4..............    4.2 GB   🎬 │   │
│  │   ... (117 个文件)                                              │   │
│  │                                                                  │   │
│  │ ▶ 📁 剧集 (45) [点击展开]                                        │   │
│  │ ▶ 📁 纪录片 (30) [点击展开]                                       │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  state: filtered                                                        │
│  同上布局，已应用筛选；文件夹标题旁显示筛后数量。                          │
│                                                                          │
│  state: empty                                                           │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  📂 该分享中没有文件                                              │   │
│  │  [返回]                                                          │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  底部操作栏 (sticky, 始终可见)                                           │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  共 195 个文件 | 已选 23 个 | 总大小 356.2 GB                      │   │
│  │  [上传]              [仅转存] [转存并整理 ▾]                       │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘

交互细节：
- 虚拟滚动：文件数 > 50 时启用虚拟滚动（复用 vue-virtual-scroller 或项目现有虚拟列表方案）。
- 文件夹展开：点击文件夹标题懒加载子文件列表，避免一次性渲染所有文件。
- 类型筛选：下拉菜单包含 video/audio/image/document/folder/other 选项；默认仅勾选 media 类型。
- 搜索：本地前端过滤（已加载到内存的文件列表），300ms 防抖。
- 底部统计实时更新："185 个文件 (共 195 个，已应用筛选)，已选 23 个，总大小 356.2 GB"。
- "转存并整理"按钮：点击后下拉显示"转存到 Watch 目录"vs"转存到指定目录"；如果目标是 Watch 目录则后续自动触发整理。
```

#### 4.3.3 TargetFolderPicker.vue

```
┌─ TargetFolderPicker ────────────────────────────────────────────────────┐
│                                                                          │
│  state: collapsed                                                       │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  目标目录: /媒体库/待整理                      剩余 1.2 TB  [选择] │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  state: expanded                                                        │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  目标目录                                                         │   │
│  │  ┌─────────────────────────────────────────────────────────┐     │   │
│  │  │ ▼ /                                                      │     │   │
│  │  │   ▼ 📁 媒体库                                             │     │   │
│  │  │       📁 电影           (选中)                               │     │   │
│  │  │     ▶ 📁 剧集                                             │     │   │
│  │  │     ▶ 📁 待整理         (当前)                              │     │   │
│  │  │   ▶ 📁 软件                                               │     │   │
│  │  │   ▶ 📁 图片                                               │     │   │
│  │  └─────────────────────────────────────────────────────────┘     │   │
│  │  可用空间: 1.2 TB / 2 TB                                        │   │
│  │  [新建文件夹] [确认选择]                                          │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘

交互细节：
- 复用现有 `cloud115_file_controller.go` 的目录列表接口获取用户 115 目录树。
- 空间不足时（选中空间 < 选中文件总大小），显示红色警告并禁用确认按钮。
- 默认为用户在 Cloud115.TransferDirectory（数据库表 t_cloud115）中配置的转存目录。
```

#### 4.3.4 ConflictPreviewModal.vue

```
┌─ ConflictPreviewModal ──────────────────────────────────────────────────┐
│                                                                          │
│  state: visible                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  文件冲突处理                                          ×         │    │
│  │──────────────────────────────────────────────────────────────────│    │
│  │  目标目录中已存在 5 个同名文件，请选择处理方式：                    │    │
│  │                                                                  │    │
│  │  ┌───────────────────────────────────────────────────────────┐  │    │
│  │  │ ☐ 跳过已存在的文件（不覆盖）                                │  │    │
│  │  │ ☐ 覆盖已存在的文件                                        │  │    │
│  │  │ ☐ 自动重命名（如 "file (1).mkv"）                         │  │    │
│  │  └───────────────────────────────────────────────────────────┘  │    │
│  │                                                                  │    │
│  │  冲突文件列表：                                                  │    │
│  │  ┌───────────────────────────────────────────────────────────┐  │    │
│  │  │ · The Matrix (1999).mkv (目标已有，2.1 GB vs 20.0 GB)     │  │    │
│  │  │ · Inception (2010).mkv (目标已有，15.0 GB vs 15.5 GB)     │  │    │
│  │  │ · ... (3 个)                                              │  │    │
│  │  └───────────────────────────────────────────────────────────┘  │    │
│  │                                                                  │
│  │  [取消转存]                               [确认并开始转存]       │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘

交互细节：
- 转存前异步对比目标目录文件，仅在存在冲突时弹出。
- 默认选中"跳过已存在的文件"。
- 冲突检测基于文件名 + 文件大小比较，标记"文件大小不同"的项。
```

#### 4.3.5 TransferProgress.vue

```
┌─ TransferProgress ──────────────────────────────────────────────────────┐
│                                                                          │
│  state: queued                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  ⏳ 转存任务已提交，即将开始...                                    │   │
│  │  任务 ID: share-transfer-a1b2c3d4                                │   │
│  │  共 23 个文件，约 356.2 GB                                       │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  state: transferring                                                    │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  转存中...                                                       │   │
│  │  ██████████████░░░░░░░░░░░░  68%                                 │   │
│  │  已完成 16 / 23 个文件                                            │   │
│  │  成功 15 个 | 跳过 0 个 | 失败 1 个                               │   │
│  │  当前: Inception (2010).mkv (15.5 GB)                            │   │
│  │  预计剩余: 约 3 分钟                                              │   │
│  │  [取消转存]                                                       │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  state: completed                                                       │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  ✅ 转存完成！23 个文件全部成功                                   │   │
│  │  目标目录: /媒体库/待整理                                         │   │
│  │  耗时: 2 分 15 秒                                                 │   │
│  │  ┌──────────────────────────────────────────────────────────────┐│   │
│  │  │ ← [去文件工作台查看]  [触发 Watch 整理]  [关闭]               ││   │
│  │  └──────────────────────────────────────────────────────────────┘│   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  state: partial_failed                                                  │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  ⚠️ 转存部分完成！23 个文件中 20 个成功，3 个失败                 │   │
│  │  ┌──────────────────────────────────────────────────────────────┐│   │
│  │  │ 失败文件列表：                                               ││   │
│  │  │ · corrupted_file_1.mkv — 秒传失败（文件可能已损坏）  [重试]  ││   │
│  │  │ · large_file.iso — 目标空间不足                     [查看]   ││   │
│  │  │ · deleted_file.mp4 — 源文件已被分享者删除            [忽略]  ││   │
│  │  └──────────────────────────────────────────────────────────────┘│   │
│  │  ┌──────────────────────────────────────────────────────────────┐│   │
│  │  │ [重试全部失败项]                         [查看成功文件] [关闭]││   │
│  │  └──────────────────────────────────────────────────────────────┘│   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  state: failed                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  ❌ 转存失败！所有文件均未完成                                     │   │
│  │  原因: 115 Cookie 已过期，请前往"115 云管理"重新登录。             │   │
│  │  ┌──────────────────────────────────────────────────────────────┐│   │
│  │  │ [前往 115 云管理]  [重试]  [关闭]                             ││   │
│  │  └──────────────────────────────────────────────────────────────┘│   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  state: cancelled                                                       │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  🛑 已取消转存                                                        │   │
│  │  已转存 16 个文件，已取消 7 个文件（已转存的不回滚）               │   │
│  │  ┌──────────────────────────────────────────────────────────────┐│   │
│  │  │ [重新开始]                              [查看已转存] [关闭]   ││   │
│  │  └──────────────────────────────────────────────────────────────┘│   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘

交互细节：
- Progress 轮询：前端每 2 秒轮询 GET /api/v1/resource/115-share/transfer/{taskId}（或使用 SSE）。
- 取消确认：点击"取消转存"弹出二次确认对话框，说明已转存的不会回滚。
- 失败文件：可逐个重试，也可"重试全部"。
- 任务在后台执行，用户可导航到其他页面；重新进入时可根据 taskId 恢复进度。
```

---

## 5. 边界情况 & 错误处理矩阵

| 场景 | 触发条件 | HTTP 错误码 | 业务错误码 | 前端行为 | 后端行为 |
| --- | --- | --- | --- | --- | --- |
| **分享链接已失效** | 115 API 返回 404/分享过期 | 200 | `SHARE_EXPIRED` | ShareLinkInput → error 状态，显示"链接已失效，请联系分享者重新获取" | 标记任务失败，记录日志 |
| **需密码但未提供** | 链接需要提取码，请求中 password 为空 | 200 | `PASSWORD_REQUIRED` | ShareLinkInput → 展开密码输入框，提示"该分享需要提取码" | 返回错误，不发起解析 |
| **密码错误** | 提供的密码不正确 | 200 | `INVALID_PASSWORD` | ShareLinkInput → error 状态，显示"提取码错误" | 返回错误 |
| **115 Cookie 过期** | 115 API 返回 401 | 200 | `AUTH_EXPIRED` | 全局弹窗提示"115 登录已过期，请前往 115 云管理重新登录"；暂停所有进行中的 115 转存任务 | 暂停该账号所有 115 任务，标记为 failed |
| **目标目录空间不足** | 115 返回配额超限 / 预估空间不足 | 200 | `INSUFFICIENT_SPACE` | TargetFolderPicker 显示红色警告；TransferProgress → error，"目标目录剩余 X GB，需要 Y GB" | 标记任务失败，保留已转存文件 |
| **部分文件转存失败** | 个别文件秒传/上传失败（SHA1 无效、文件已删等） | 200 | `PARTIAL_FAILED` | TransferProgress → partial_failed，展示成功/失败明细，逐条可重试 | 对单文件失败做自动重试（1 次），仍失败则跳过并记录；整体任务状态 partial_failed |
| **全部转存失败** | 所有文件都转存失败 | 200 | `FAILED` | TransferProgress → failed，显示失败原因及建议 | 标记任务 failed |
| **网络超时** | 30s 无响应（解析或转存请求） | 200 | `TIMEOUT` | 提示"网络超时，请检查网络连接后重试" | 自动重试 1 次（指数退避 1s），仍超时则标记 failed |
| **批量转存限频** | 115 API 返回频率限制 | 200 | `RATE_LIMITED` | TransferProgress 显示"正在等待（115 限速），预计等待 X 秒..." | 自动延迟重试（退避 5s → 15s → 30s），在转移到 metadata 中记录等待时间 |
| **分享链接无符合类型的文件** | 用户选了"仅视频"但解析结果无视频文件 | - | `NO_MEDIA` | FileSelector → empty 状态变体，"未找到媒体文件，可切换筛选类型查看全部文件" | 不阻止提交，仅返回计数 0 |
| **相同文件已存在** | 目标目录有同名 + 同 SHA1 文件 | - | `DUPLICATE_SKIPPED` | 统计中显示为"跳过 N 个"；日志级别 info | 秒传跳过，不计入 success 也不计入 failed；冲突策略为 skip 时默认行为 |
| **同名不同 SHA1** | 目标目录有同名但不同 SHA1 的文件 | - | `CONFLICT` | ConflictPreviewModal 弹出，提供 skip / overwrite / rename 选项 | 根据用户选择的冲突策略处理（默认 skip） |
| **用户中途取消** | 用户点击"取消" | - | `USER_CANCELLED` | TransferProgress → cancelled，清理进度显示 | 设置取消标记，停止处理，已转存文件保留不回滚 |
| **目标目录不存在** | 用户输入的路径在 115 中不存在 | 200 | `DIRECTORY_NOT_FOUND` | TargetFolderPicker → 提示"目标目录不存在"，可点击自动创建 | 自动创建目录（递归创建父目录）；仅当创建失败时返回错误 |
| **分享目录为空** | 解析结果 files 数组为空 | 200 | `EMPTY_SHARE` | FileSelector → empty，"该分享内没有文件" | 正常返回，files = [] |
| **分享链接格式不合法** | URL 不匹配 115 分享链接正则 | 200 | `INVALID_URL` | ShareLinkInput → error，"请输入有效的 115 分享链接" | 返回错误 |
| **极大目录（1000+ 文件）** | 分享包含大量文件 | - | - | FileSelector → 分页加载（200/页），虚拟滚动渲染 | 后端分页返回，单次请求/页返回 200 条 |
| **服务重启中任务中断** | Docker restart / kill 信号 | - | - | 任务状态变为 pending/failed，用户需重新查看 | 基于 Redis Set 记录的已完成 pickCode 进行恢复 |

---

## 6. 非功能需求

### 6.1 性能

| 指标 | 目标 | 说明 |
| --- | --- | --- |
| 分享链接解析 | P50 < 3s, P95 < 10s | 200 文件以内的分享；超过 200 文件按分页首次加载 < 3s |
| 单文件秒传 | < 2s | 调用 115 copy/receive API |
| 100 文件批量转存 | < 60s | 含秒传判断 + API 调用；不含真实上传 |
| 前端首屏渲染（200 条文件） | < 1s | 虚拟滚动 |
| 前端筛选/搜索响应 | < 200ms | 本地过滤，300ms 防抖 |
| 任务状态轮询 | 每 2s 一次 | 前端定时器；可升级为 SSE |
| 数据库写入 | 批量 INSERT 100 条 < 1s | 转存日志表 |

### 6.2 可靠性

- **转存任务断点续传**：通过 Redis Set（`easy_strm:task:progress:{taskId}`）记录已完成转存的 pickCode，服务重启后可从 Set 中读取进度恢复。
- **失败自动重试**：单文件失败自动重试 1 次（间隔 1s）；限频失败自动退避重试（5s → 15s → 30s，最多 3 次）。
- **Cookie 过期检测**：在每次转存 API 调用前检查 115 Cookie 有效性（可通过一个小请求验证），过期时暂停所有该账号的任务并通知前端。

### 6.3 安全性

| 措施 | 说明 |
| --- | --- |
| URL 日志脱敏 | 分享链接 URL 在日志中不完整记录；仅记录 shareCode 前 4 位 + "***" |
| 密码不落盘 | 提取码（password）不写入 PostgreSQL 日志表；仅在 Redis 任务 metadata 中存储（Redis 默认启用密码或内网部署） |
| HTTPS | 所有前端 → 后端 API 调用依赖 Nginx/Caddy 反向代理的 HTTPS（如已配置） |
| 115 Cookie | 不新增 Cookie 存储位置；复用现有 t_cloud115 表加密存储 |

### 6.4 可观测性

- **结构化日志**：
  ```
  [INFO] ShareTransfer | shareCode=sw6z*** | action=parse | duration=2.3s | fileCount=195
  [INFO] ShareTransfer | taskId=xxx | action=transfer_start | fileCount=23 | targetDir=/媒体库/待整理
  [INFO] ShareTransfer | taskId=xxx | action=transfer_progress | completed=16/23 | failed=1
  [ERROR] ShareTransfer | taskId=xxx | action=transfer_file_failed | file=xxx | error=SECOND_TRANSFER_FAILED
  ```

- **PostHog 事件**：

  | 事件名 | 触发点 | 属性 |
  | --- | --- | --- |
  | `share_link_pasted` | 用户粘贴 115 链接 | trigger (manual/paste) |
  | `share_link_parsed` | 解析成功/失败 | result, duration, fileCount, totalSize, error_code |
  | `files_selected` | 用户确认文件选择 | selectedCount, totalCount, totalSize, filters_applied |
  | `transfer_submitted` | 用户点击转存 | fileCount, totalSize, conflictStrategy, autoOrganize |
  | `transfer_completed` | 转存完成 | taskId, totalFiles, successFiles, failedFiles, duration, isSecondTransfer |
  | `transfer_cancelled` | 用户取消 | taskId, completedFiles, cancelledFiles |
  | `auth_expired_detected` | Cookie 过期检测 | cloud115Id |
  | `rate_limited` | 115 限频触发 | taskId, retryCount, waitSeconds |

---

## 7. 技术风险 & 缓解

| 风险 | 概率 | 影响 | 缓解措施 | 验证方式 |
| --- | --- | --- | --- | --- |
| **115 分享链接解析 API 变动** | 中 | 解析功能全部失效 | 抽象 `ShareLinkParser` 接口层，支持多种解析策略 fallback（标准 API → HTML 抓取 → 用户手动输入 CID）；建立 Watchtower 监控解析成功率 | POC 阶段验证 115 API 稳定性；监控埋点异常告警 |
| **115 秒传（second transfer）机制限制** | 高 | 大文件需要真实上传，远超 30s 预期 | 优先在 POC 阶段用不同场景的文件验证秒传条件；如果秒传不可用，转存模式降级为"后台真实上传"，前端显示上传速度 | POC 测试：准备 5 种场景文件（小/中/大、common/rare hash） |
| **115 限频导致大目录转存堆积** | 高 | >500 文件的转存可能耗时 10+ 分钟，用户体验差 | 任务队列串行化处理，自动限制并发为 1；前端显示真实等待时间；支持"暂停/继续"控制 | 用 100+ 文件测试限频阈值 |
| **Watch 无法检测 115 秒传文件** | 中 | "转存并整理"流程断裂 | 转存成功后通过 API 回调直接触发整理（不走文件系统检测），而非依赖 Watch 轮询 | 集成测试：转存 → 回调触发整理 → STRM → Emby |
| **前端大文件列表渲染性能** | 中 | 1000+ 文件目录卡顿 | 虚拟滚动（vue-virtual-scroller）；分页加载（200/页）；懒加载子目录 | 构造 2000+ 文件的 mock 数据测试 |
| **Cookie 有效期过短导致任务挂起** | 低 | 长时间转存中 Cookie 过期 | 复用现有 115 登录状态检测；转存开始前验证 Cookie；过期时引导重新登录 | 观察生产环境 Cookie 有效时长 |

---

## 附录

### A. 后端实现清单

| 任务 | 文件路径 | 预估工时 |
| --- | --- | --- |
| 新增 `TaskTypeShareTransfer` | `task.go` | 0.1d |
| 新增 `ShareLinkInput` 结构体 | `internal/domain/models.go` | 0.1d |
| 新增 `ResourceController` | `internal/controller/resource_controller.go` | 0.5d |
| 新增 `ShareTransferService`（解析 + 转存） | `internal/service/share_transfer_service.go` | 1.5d |
| 扩展 115 API Client（分享解析 + 转存） | `115_openapi.go` | 1d |
| 新增 `ShareTransferLogDAO` | `internal/dao/share_transfer_log_dao.go` | 0.5d |
| 路由注册 | `internal/router/` | 0.5d |
| 数据库迁移脚本 | `migrations/migrate_v14_share_transfer_log.sql` | 0.2d |
| 单元测试 | `*_test.go` | 2d |
| 集成测试（转存 → 整理全链路） | `*_test.go` | 1.5d |

### B. 前端实现清单

| 任务 | 文件路径 | 预估工时 |
| --- | --- | --- |
| API 层封装 | `src/utils/api/resource.js` | 0.5d |
| `ResourceAggregation.vue`（Tab 容器） | `src/views/ResourceAggregation.vue` | 0.5d |
| `ResourceTransfer.vue`（转存页面） | `src/views/resources/ResourceTransfer.vue` | 0.5d |
| `ShareLinkInput.vue` | `src/components/resource/ShareLinkInput.vue` | 1d |
| `FileSelector.vue` | `src/components/resource/FileSelector.vue` | 2d |
| `TargetFolderPicker.vue` | `src/components/resource/TargetFolderPicker.vue` | 1d |
| `ConflictPreviewModal.vue` | `src/components/resource/ConflictPreviewModal.vue` | 0.5d |
| `TransferProgress.vue` | `src/components/resource/TransferProgress.vue` | 1.5d |
| 路由注册 | `src/router/` | 0.2d |
| 侧边栏菜单更新 | `src/components/layout/` | 0.2d |
| PostHog 埋点集成 | 各组件 | 1d |
| E2E 测试 | `tests/e2e/` | 1.5d |

### C. 变更日志

| 日期 | 版本 | 变更内容 | 作者 |
| --- | --- | --- | --- |
| 2026-07-24 | v1.0 | 初版执行规格 | 需求分析师（析客） |
