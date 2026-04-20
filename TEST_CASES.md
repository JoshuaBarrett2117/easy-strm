# Easy-STRM 测试用例文档

## 1. 测试现状概述

### 1.1 现有测试（8个测试文件）

| 测试文件 | 测试范围 | 测试技术 |
|---------|---------|---------|
| `internal/dao/media_source_dao_test.go` | MediaSource CRUD | sqlmock |
| `internal/service/organize_service_115_test.go` | 115整理服务 | sqlmock + fake client |
| `internal/service/organize_service_local_test.go` | 本地整理服务 | sqlmock + fake fs |
| `internal/service/rename_service_test.go` | 重命名服务 | - |
| `internal/service/tmdb_service_test.go` | TMDB API | httptest mock server |
| `internal/service/file_operation_service_test.go` | 文件操作服务 | - |
| `internal/service/media_source_service_test.go` | 媒体源服务层 | sqlmock |
| `internal/service/category_organize_test.go` | 分类整理 | - |

### 1.2 测试覆盖率分析

**已有测试的模块：**
- ✅ MediaSource DAO / Service
- ✅ OrganizeService (115 + Local)
- ✅ TmdbService
- ✅ RenameService

**缺少测试的模块：**
- ❌ TaskService / TaskController
- ❌ CronService
- ❌ WatchService
- ❌ EmbyService
- ❌ DashboardService
- ❌ ScrapeService
- ❌ Cloud115Service (115 API)
- ❌ 通知服务 (Telegram/ServerChan/Email)
- ❌ 认证中间件
- ❌ 前端组件（无测试）

---

## 2. 测试用例清单

### 2.1 单元测试用例

#### 2.1.1 任务系统（TaskService）

```go
// TestTaskCreate_ValidParams
// 测试：创建任务成功，验证任务ID、状态、创建时间
// 期望：返回任务ID，状态为pending，Redis中存储正确

// TestTaskCancel_SetCancelFlag
// 测试：取消运行中的任务
// 期望：Redis设置cancel flag，任务状态更新为cancelled

// TestTaskCancel_CheckByWorker
// 测试：任务worker检查cancel flag并退出
// 期望：worker检测到flag后立即退出，progress保留

// TestTaskResume_ClearCancelFlag
// 测试：恢复已取消的任务
// 期望：清除cancel flag，状态更新为pending

// TestTaskProgress_AddFileID
// 测试：记录任务进度
// 期望：Redis Set中添加文件ID

// TestTaskProgress_IsFileProcessed
// 测试：检查文件是否已处理（断点续传）
// 期望：已处理返回true，未处理返回false

// TestGetUnifiedTasks_MultipleTaskTypes
// 测试：获取统一任务列表，包含多种任务类型
// 期望：返回STRM生成、整理、Cron任务混合列表

// TestTaskPriority_HighPriorityFirst
// 测试：高优先级任务先执行
// 期望：按priority字段排序，高优先级在前
```

#### 2.1.2 监控服务（WatchService）

```go
// TestWatchService_StartLocalWatcher
// 测试：启动本地目录监控
// 期望：fsnotify添加监控路径，无错误

// TestWatchService_StopLocalWatcher
// 测试：停止本地目录监控
// 期望：fsnotify移除监控路径

// TestWatchService_DebounceEvent
// 测试：防抖机制，30秒内重复事件只处理一次
// 期望：最后一个事件30秒后触发处理函数

// TestWatchService_115Polling_NewFiles
// 测试：115轮询检测到新文件
// 期望：识别新增文件，触发自动整理

// TestWatchService_TriggerAutoOrganize
// 测试：触发自动整理流程
// 期望：调用OrganizeService，执行整理

// TestWatchService_StartAll
// 测试：启动所有已启用监控的媒体源
// 期望：正确启动本地监控和115轮询
```

#### 2.1.3 Emby集成（EmbyService）

```go
// TestEmbyService_GetStatus_Valid
// 测试：获取Emby连接状态（有效配置）
// 期望：返回emby_version、can_connect=true

// TestEmbyService_GetStatus_Invalid
// 测试：获取Emby连接状态（无效配置）
// 期望：返回错误，can_connect=false

// TestEmbyService_GetLibraries
// 测试：获取Emby媒体库列表
// 期望：返回Movies、TV Shows等库

// TestEmbyService_RefreshLibrary
// 测试：刷新Emby媒体库
// 期望：POST /Library.Refresh成功

// TestEmbyService_RefreshLibraryByPath
// 测试：按路径刷新媒体库
// 期望：仅刷新包含指定路径的库项
```

#### 2.1.4 Dashboard服务

```go
// TestDashboardService_GetStats_AllSuccess
// 测试：获取完整统计数据（全部成功）
// 期望：返回accounts/media_sources/strm_files/tasks/storage

// TestDashboardService_GetStats_PartialFailure
// 测试：部分数据源失败时的容错
// 期望：失败的数据源返回零值，其他正常返回

// TestDashboardService_StorageInfo
// 测试：获取115账号存储信息
// 期望：返回used/total/percentage

// TestDashboardService_TaskStats
// 测试：任务统计（今日运行/完成/失败）
// 期望：正确统计当日任务数量
```

#### 2.1.5 通知服务

```go
// TestNotificationService_SendTelegram
// 测试：发送Telegram通知
// 期望：HTTP POST到Bot API成功

// TestNotificationService_SendServerChan
// 测试：发送Server酱通知
// 期望：HTTP POST到SC URL成功

// TestNotificationService_SendEmail
// 测试：发送邮件通知
// 期望：SMTP连接成功，邮件发送

// TestNotificationService_RetryOnFailure
// 测试：发送失败时重试
// 期望：首次失败后重试3次

// TestNotificationService_FormatMessage
// 测试：消息格式化（任务完成/失败/整理结果）
// 期望：正确生成Markdown格式消息
```

#### 2.1.6 NFO刮削服务

```go
// TestScrapeService_ScrapeMovieNFO
// 测试：刮削电影NFO
// 期望：生成movie.nfo，包含title/year/plot等

// TestScrapeService_ScrapeTVNFO
// 测试：刮削剧集NFO
// 期望：生成tvshow.nfo + season/episode nfo

// TestScrapeService_DownloadPoster
// 测试：下载海报图片
// 期望：TMDB图片下载到本地目录

// TestScrapeService_ScrapeFiles_Batch
// 测试：批量刮削多个文件
// 期望：逐个刮削，失败继续，汇总结果
```

#### 2.1.7 认证中间件

```go
// TestJWTMiddleware_ValidToken
// 测试：有效JWT Token
// 期望：请求通过，user_id注入context

// TestJWTMiddleware_ExpiredToken
// 测试：过期Token
// 期望：返回401，错误消息"token已过期"

// TestJWTMiddleware_InvalidToken
// 测试：无效Token
// 期望：返回401，错误消息"token无效"

// TestJWTMiddleware_MissingToken
// 测试：缺失Token
// 期望：返回401，错误消息"authorization header required"
```

#### 2.1.8 Cron服务

```go
// TestCronService_AddTask
// 测试：添加Cron任务
// 期望：任务添加到调度器，定时执行

// TestCronService_RemoveTask
// 测试：移除Cron任务
// 期望：从调度器中移除

// TestCronService_RunFullStrmGenerate
// 测试：手动触发全量STRM生成
// 期望：任务创建，异步执行，进度更新

// TestCronService_RunIncrementalSync
// 测试：增量同步
// 期望：仅同步新增/修改的文件
```

---

### 2.2 集成测试用例

#### 2.2.1 API端到端测试

```go
// TestE2E_CreateMediaSource
// 流程：POST /media/sources -> GET /media/sources/:id -> PUT -> DELETE
// 期望：完整CRUD流程，数据一致

// TestE2E_OrganizeWorkflow
// 流程：选择文件 -> TMDB识别 -> 确认 -> 整理 -> 生成STRM
// 期望：端到端流程成功，文件正确移动/重命名

// TestE2E_TaskCancelWorkflow
// 流程：创建任务 -> 运行中取消 -> 查询状态
// 期望：任务正确取消，状态更新

// TestE2E_EmbyRefreshWorkflow
// 流程：配置Emby -> 添加媒体源绑定 -> 整理 -> 自动刷新
// 期望：整理完成后Emby库刷新成功
```

#### 2.2.2 完整工作流测试

```go
// TestWorkflow_LocalMediaOrganize
// 场景：用户整理本地媒体
// 步骤：
//   1. 添加本地媒体源 (path=C:\media)
//   2. 浏览文件，选择电影文件
//   3. TMDB自动识别
//   4. 确认目标路径
//   5. 执行整理（移动+重命名）
//   6. 生成STRM
//   7. 刷新Emby（如配置）
// 期望：全部步骤成功，文件正确整理

// TestWorkflow_Cloud115Organize
// 场景：用户整理115云盘媒体
// 步骤：
//   1. 添加115账号（扫码登录）
//   2. 添加115媒体源
//   3. 浏览云盘文件
//   4. TMDB识别
//   5. 执行整理（复制到目标目录）
//   6. 生成STRM
// 期望：云盘操作成功，STRM生成正确

// TestWorkflow_WatchAndAutoOrganize
// 场景：监控文件夹自动整理
// 步骤：
//   1. 配置媒体源启用监控
//   2. 复制新文件到监控目录
//   3. 等待30秒防抖
//   4. 自动触发识别
//   5. 自动执行整理
// 期望：新文件自动整理完成

// TestWorkflow_IncrementalStrmGeneration
// 场景：增量STRM生成
// 步骤：
//   1. 创建STRM配置
//   2. 执行全量生成（100个文件）
//   3. 中途取消任务
//   4. 恢复任务
//   5. 验证已处理文件被跳过
// 期望：断点续传正确，已处理文件不重复处理
```

---

### 2.3 前端测试用例

#### 2.3.1 组件单元测试

```typescript
// MediaSourceList.test.ts
describe('MediaSourceList', () => {
  test('renders media source table', () => {})
  test('opens add dialog', () => {})
  test('opens edit dialog with data', () => {})
  test('validates form fields', () => {})
  test('handles delete confirmation', () => {})
  test('toggles enabled status', () => {})
})

// FileBrowser.test.ts
describe('FileBrowser', () => {
  test('displays breadcrumb navigation', () => {})
  test('navigates to subdirectory', () => {})
  test('filters files by search', () => {})
  test('selects multiple files', () => {})
  test('batch actions work correctly', () => {})
})

// OrganizeDialog.test.ts
describe('OrganizeDialog', () => {
  test('previews organize results', () => {})
  test('validates target path', () => {})
  test('shows progress during execution', () => {})
  test('displays results summary', () => {})
  test('retry failed items works', () => {})
})

// TmdbIdentifyDialog.test.ts
describe('TmdbIdentifyDialog', () => {
  test('searches TMDB', () => {})
  test('displays top 3 candidates', () => {})
  test('select candidate fills form', () => {})
  test('manual search works', () => {})
})
```

#### 2.3.2 E2E测试（Playwright）

```typescript
// login.spec.ts
test('user can login', async ({ page }) => {
  await page.goto('/login')
  await page.fill('[name=username]', 'admin')
  await page.fill('[name=password]', 'password')
  await page.click('[type=submit]')
  await expect(page).toHaveURL('/dashboard')
})

// media-source-crud.spec.ts
test('CRUD media source', async ({ page }) => {
  // Create
  await page.goto('/media')
  await page.click('text=添加媒体源')
  await page.fill('[name=name]', 'test')
  await page.click('text=确定')

  // Read
  await expect(page.locator('text=test')).toBeVisible()

  // Update
  await page.click('text=编辑')
  await page.fill('[name=name]', 'test-updated')
  await page.click('text=确定')
  await expect(page.locator('text=test-updated')).toBeVisible()

  // Delete
  await page.click('text=删除')
  await page.click('text=确定')
  await expect(page.locator('text=test-updated')).not.toBeVisible()
})

// organize-workflow.spec.ts
test('complete organize workflow', async ({ page }) => {
  // Select files -> Identify -> Organize
})
```

---

### 2.4 边界测试用例

```go
// TestBoundary_EmptyFileList
// 测试：整理空文件列表
// 期望：返回提示"请选择要整理的文件"

// TestBoundary_DuplicateFilename
// 测试：目标目录存在同名文件
// 期望：根据策略处理（跳过/覆盖/重命名）

// TestBoundary_NetworkTimeout
// 测试：115 API请求超时
// 期望：重试机制生效，超时返回错误

// TestBoundary_DiskFull
// 测试：目标磁盘空间不足
// 期望：返回错误提示，不执行移动

// TestBoundary_InvalidPathChars
// 测试：文件名包含非法字符
// 期望：自动过滤或替换非法字符

// TestBoundary_LargeFileCount
// 测试：大量文件（10000+）处理
// 期望：分批处理，进度正确显示，无内存溢出

// TestBoundary_ConcurrentTasks
// 测试：多个任务同时执行
// 期望：任务队列正确，资源不冲突

// TestBoundary_TokenExpiredDuringTask
// 测试：任务执行中JWT过期
// 期望：Token刷新后继续执行，或优雅退出
```

---

## 3. 测试优先级建议

### 3.1 第一优先级（核心功能）

| 模块 | 原因 |
|-----|------|
| TaskService (取消/恢复/进度) | 新功能，需验证核心逻辑 |
| WatchService | 新功能，核心自动化能力 |
| OrganizeService | 核心功能，已有部分测试需补充 |
| EmbyService | 新功能，与外部系统集成 |

### 3.2 第二优先级（关键路径）

| 模块 | 原因 |
|-----|------|
| DashboardService | 展示数据准确性 |
| 认证中间件 | 安全关键 |
| CronService | 定时任务可靠性 |

### 3.3 第三优先级（完善覆盖）

| 模块 | 原因 |
|-----|------|
| 通知服务 | 辅助功能 |
| ScrapeService | 辅助功能 |
| 前端测试 | UI正确性 |

---

## 4. 测试数据准备

### 4.1 测试用MediaSource

```yaml
# 本地媒体源
- id: 1
  name: "本地电影"
  source_type: "local"
  path: "C:/test/media/movies"
  organize_target_path: "C:/test/organized/movies"
  enabled: true
  auto_organize: true
  watch_enabled: true
  watch_interval: 60
  emby_library_id: "library-1"

# 115媒体源
- id: 2
  name: "115剧集"
  source_type: "cloud115"
  cloud115_id: 1
  organize_target_path: "/organized/tv"
  enabled: true
```

### 4.2 测试用TMDB数据

```yaml
# 电影识别结果
- id: 550
  title: "Fight Club"
  release_date: "1999-10-15"
  overview: "A ticking-Loss bomb..."
  vote_average: 8.4
  poster_path: "/pB8BM7pdSp6B6Ih7QZ4DrQ3PmJK.jpg"

# 剧集识别结果
- id: 1396
  name: "Breaking Bad"
  first_air_date: "2008-01-20"
  overview: "A high school chemistry teacher..."
  vote_average: 8.9
```

---

## 5. 测试执行计划

### 5.1 本地开发测试
```bash
# 运行所有单元测试
go test ./... -v

# 运行特定模块测试
go test ./internal/service -v -run TestTask

# 运行测试并生成覆盖率
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### 5.2 CI/CD集成
```yaml
# .github/workflows/test.yml
- name: Run Tests
  run: |
    go test ./... -v -race
    npm run test
    npm run test:e2e
```

---

## 6. 现有测试优化建议

### 6.1 organize_service_115_test.go

**问题：** 测试使用 fake client，mock 不够精细

**优化：** 分离"Happy Path"和"边界情况"测试
- 添加网络超时测试
- 添加115 API错误码测试
- 添加配额不足测试

### 6.2 tmdb_service_test.go

**问题：** 仅测试成功路径

**优化：** 添加失败场景
- API限流 (429)
- 无结果返回
- 网络超时

### 6.3 media_source_dao_test.go

**问题：** CRUD测试覆盖不全

**优化：** 添加
- Update测试
- 事务回滚测试
- 并发操作测试

---

## 7. 测试报告模板

每次发布前填写：

```markdown
## 测试报告 - vX.X.X

### 测试执行
- 单元测试: X个 / 通过X个 / 失败X个
- 集成测试: X个 / 通过X个 / 失败X个
- E2E测试: X个 / 通过X个 / 失败X个
- 覆盖率: X%

### 已知问题
| ID | 描述 | 严重度 | 状态 |
|----|------|--------|------|
| BUG-001 | ... | 高 | Open |

### 测试结论
- [ ] 可以发布
- [ ] 需要修复后发布
```
