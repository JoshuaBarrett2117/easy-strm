# 115 分享转存 → 自动整理 → 自动刮削 后端实现方案

> 架构师：高见远（easy-strm 后端）
> 范围：Go 后端（`easy-strm/`）。前端 `ResourceTransfer.vue` 已发送 `auto_organize`/`auto_scrape` 两个开关，本文只设计后端落地。
> 约定：所有相对路径相对于后端根目录 `easy-strm/`。

---

## 0. 调研结论速览（基于源码核实）

| # | 事实 | 源码依据 | 对设计的影响 |
|---|------|----------|--------------|
| F1 | `TransferRequest` 只有 `AutoOrganize`，**没有 `AutoScrape`**；`SubmitTransfer`/`executeTransfer` 全程不读这两个字段，转存只调 `ReceiveShare` 后终态，不触发任何整理/刮削 | `internal/domain/models.go:258-266`；`internal/service/share_transfer_service.go:318,420` | 需补 `AutoScrape` 字段并让 `executeTransfer` 终态后读开关 |
| F2 | 整理入口 `OrganizeService.OrganizeDirectory(sourceID int, …)` **必须**传媒体源 ID，内部 `mediaSourceService.GetByID(sourceID)` | `internal/service/organize_service.go:241,263` | ad-hoc 转存无媒体源，需「传入 `*MediaSource` 值」的新入口 |
| F3 | `organizeCloud115File(source *domain.MediaSource, …)` 直接消费 `*MediaSource`（读 `Cloud115ID`/`Path`/`OrganizeTargetPath`） | `internal/service/organize_execute.go:13` | 用**临时 MediaSource**（内存构造、不落库）即可驱动 115 整理 |
| F4 | 整理流程已内置刮削 `maybeScrapeOrganizedResult`，但因 `if !filepath.IsAbs(result.NewPath) { return }` 对 115 路径（如 `/电影/x.mkv`）直接跳过 | `internal/service/organize_service.go:444-462` | 115 整理**不会**顺带刮削 |
| F5 | `ScrapeService` 对 `SourceTypeCloud115` 直接返回 `"only local media sources support scraping"` / `"仅本地媒体源支持NFO刮削"` | `internal/service/scrape_service.go:74-76,95-97,154-156` | **115 刮削当前完全不可用**（NFO 写在本地文件系统，115 是远程 API） |
| F6 | 任务中心 `dao.TaskRedisDAO` 的 `Create/Get/UpdateMetadata/UpdateProgress/UpdateStatus/SetError` 均为 map 结构，**无父子任务概念** | `internal/dao/task_dao.go` | 链式任务用「独立任务 + `parent_task_id` 元数据」关联 |
| F7 | 任务类型枚举集中在 `main.TaskType`（`task.go`），但 `dao.TaskRedisDAO.Create` 接受任意字符串；watch 用 `watch_auto_organize` 常量（不在枚举内） | `task.go:16-29`；`internal/service/watch_service.go:38` | 新增 `share_transfer_organize`/`share_transfer_scrape` 并补进枚举 |
| F8 | `NewShareTransferService(client, shareTransferTaskDAO, shareTransferLogDAO, cloud115DAO)` **未注入** organize/scrape | `auth.go:63` | 需扩展构造函数并改 `auth.go` 接线 |
| F9 | `watch_service.triggerAutoOrganize` 是一套成熟的「对任意 115 账号+目录下的若干文件触发一次整理任务」可复用模式（建任务→`organizeService.OrganizeDirectory`→回写进度/元数据） | `internal/service/watch_service.go:594-665` | 直接借鉴其任务生命周期写法 |

---

## 1. 实现方案与框架选型

### 1.1 整体策略：**复用现有 organize 管线 + 新增「source 直传」入口 + 刮削按源类型分流**

- **不新建整理引擎**。115 整理已成熟（`OrganizeService` + `organizeCloud115File`）。缺口只是「没有媒体源对象」。
- **新增 source 直传入口** `OrganizeDirectoryForSource(ctx, source *domain.MediaSource, …)`，让 ad-hoc 转存用「临时 MediaSource」驱动既有整理逻辑，避免污染用户媒体源列表、也不破坏现有 6+ 调用方。
- **刮削分流**：
  - 复用已有媒体源（本地）时 → 走 `ScrapeService`（已可用）。
  - 目标为 **115 云盘** → 本期**优雅降级**：`Cloud115Scraper` 返回 `skipped` + 明确原因（"115 云盘暂不支持 NFO 写入"），**不报错、不阻断转存**；预留「NFO 内容生成 + 115 上传」实现位（后续迭代）。理由见 §8。
- **链式任务**：转存 / 整理 / 刮削各自是任务中心的**独立任务**，通过元数据 `parent_task_id`（指向转存任务）和 `stage`（`organize`/`scrape`）串联，前端按 `parent_task_id` 分组即可呈现为链。

### 1.2 为什么是「临时 MediaSource」而不是「复用一个持久媒体源」

- 方案 A（复用用户已配置的媒体源，前端传 `organize_source_id`）：**最推荐作为首选路径**。media_type、命名模板、冲突策略、目标路径全部来自既有配置，规则复用度最高，也最贴近用户预期。
- 方案 B（无 source 时构造临时 `*MediaSource`）：兜底。用 `TargetCloud115Id` + `TargetDirectory` 在内存里拼一个 `SourceType=cloud115` 的源，不写库。
- 结论：**两者并存**。请求带 `organize_source_id>0` 用 A；否则用 B。B 的 `MediaType` 由 `inferMediaTypeFromName` 推断（默认 `all`），`OrganizeTargetPath` 默认等于 `TargetDirectory`（就地整理，文件归到 `TargetDirectory/电影` 或 `TargetDirectory/剧名` 子目录）。

### 1.3 向后兼容（硬性要求）

`auto_organize == false && auto_scrape == false`（当前默认）时：
- `SubmitTransfer` 仅在元数据多写 2 个字段（`auto_organize:false, auto_scrape:false`）；
- `executeTransfer` 终态后**不**启动任何额外 goroutine；
- 不创建 organize/scrape 任务，不调用 `OrganizeService`/`Scraper`。
行为与原版 100% 一致。

---

## 2. 待修改 / 新增文件清单

| 操作 | 相对路径 | 说明 |
|------|----------|------|
| 修改 | `internal/domain/models.go` | `TransferRequest` 增加 `AutoScrape`、`OrganizeSourceID`、`OrganizeTargetPath`；新增 `PostTransferConfig`（可选，承载派生参数） |
| 修改 | `task.go` | `TaskType` 枚举 + `TaskTypeNames` 增加 `share_transfer_organize`、`share_transfer_scrape` |
| 修改 | `internal/service/organize_service.go` | 新增 `OrganizeDirectoryForSource`（接受 `*MediaSource`）；`PreviewOrganize`/`ListOrganizeCandidates` 增加 `*source` 直传重载，既有 `sourceID` 入口委托之 |
| 修改 | `internal/service/organize_execute.go` | `organizeFile`/`organizeCloud115File` 适配「已持有 source」路径（避免重复 `GetByID`） |
| 新增 | `internal/service/scrape_adapter.go` | 定义 `PostTransferScraper` 接口；`LocalScraper`（包装 `ScrapeService`）、`Cloud115Scraper`（本期降级实现） |
| 修改 | `internal/service/scrape_service.go` | 暴露 `GenerateMovieNFO`/`GenerateEpisodeNFO` 为 `ScrapeService` 方法（供 `Cloud115Scraper` 生成 NFO 内容，本期仅用于降级提示，后续接上传） |
| 修改 | `internal/service/share_transfer_service.go` | 构造函数注入 `OrganizeService` + `PostTransferScraper`；`SubmitTransfer` 落盘开关；`executeTransfer` 终态后触发编排 |
| 新增 | `internal/service/share_transfer_post.go` | 编排 goroutine `runPostTransferOrganize` / `runPostTransferScrape` + 纯函数 `buildAdhocOrganizeSource` / `inferMediaTypeFromName` / `resolveOrganizeTargetPath` / `listTransferredFileIDs` |
| 新增 | `internal/service/share_transfer_service_test.go` | 纯函数单测 + 编排单测（mock OrganizeService / Scraper） |
| 修改 | `internal/service/organize_service_test.go`（或新增 `organize_service_adhoc_test.go`） | `OrganizeDirectoryForSource` 单测 |
| 修改 | `auth.go` | `NewShareTransferService` 接线注入 `organizeService`、`scraper` |
| 修改 | `internal/controller/resource_controller.go` | 仅更新 `SubmitTransfer` 注释（逻辑无需改，已 `ShouldBindJSON` 自动接收新字段） |
| 新增（前端，仅建议） | `easy-strm-front/src/views/resources/ResourceTransfer.vue` | 可选：补充「媒体源」下拉（`organize_source_id`）与「整理目标路径」输入；后端不改也能跑（走方案 B） |

---

## 3. 数据结构与接口变更

### 3.1 `TransferRequest`（domain/models.go）

```go
// TransferRequest 转存任务请求
type TransferRequest struct {
    ShareCode        string                  `json:"share_code"`
    Password         string                  `json:"password"`
    TargetCloud115Id int                     `json:"target_cloud115_id"`
    TargetDirectory  string                  `json:"target_directory"`
    Files            []ShareTransferFileItem `json:"files"`
    ConflictStrategy string                  `json:"conflict_strategy"`
    AutoOrganize     bool                    `json:"auto_organize"`
    AutoScrape       bool                    `json:"auto_scrape"`        // 新增：转存完成后是否自动刮削
    OrganizeSourceID int                     `json:"organize_source_id"` // 新增：复用已有媒体源的规则（0=临时源）
    OrganizeTargetPath string                `json:"organize_target_path"` // 新增(可选)：整理目标路径，缺省=TargetDirectory
}
```

### 3.2 转存任务元数据（写入 `share_transfer` 任务）

在 `SubmitTransfer` 现有 `metadata` 基础上增补（向后兼容，仅多写字段）：

```go
metadata := map[string]interface{}{
    // —— 既有字段保留 ——
    "share_code": req.ShareCode, "share_folder_name": shareFolderName,
    "target_directory": req.TargetDirectory, "target_account_id": req.TargetCloud115Id,
    "conflict_strategy": req.ConflictStrategy,
    // —— 新增 ——
    "auto_organize": req.AutoOrganize,
    "auto_scrape":   req.AutoScrape,
    "organize_source_id": req.OrganizeSourceID,
    "organize_target_path": resolveOrganizeTargetPath(...),
    "organize_task_id": "",   // 终态后回填
    "scrape_task_id":   "",   // 整理成功后回填
    "parent_task_id":  "",    // organize/scrape 任务反向指向转存任务
}
```

### 3.3 新任务类型（task.go，package main）

```go
const (
    TaskTypeStrmGenerate    TaskType = "strm_generate"
    TaskTypeIncrementalSync TaskType = "incremental_sync"
    TaskTypeShareTransfer   TaskType = "share_transfer"
    TaskTypeShareParse      TaskType = "share_parse"
    TaskTypeShareTransferOrganize TaskType = "share_transfer_organize" // 新增
    TaskTypeShareTransferScrape   TaskType = "share_transfer_scrape"   // 新增
)
var TaskTypeNames = map[TaskType]string{
    TaskTypeStrmGenerate: "STRM文件生成", TaskTypeIncrementalSync: "增量同步",
    TaskTypeShareTransfer: "分享转存", TaskTypeShareParse: "分享解析",
    TaskTypeShareTransferOrganize: "转存后整理", TaskTypeShareTransferScrape: "转存后刮削",
}
```

### 3.4 `OrganizeService` 新入口

```go
// OrganizeDirectoryForSource 接受已构造的 MediaSource（支持临时源 / ad-hoc 转存）
// 与 OrganizeDirectory 行为一致，但不调用 GetByID，避免无 DB 媒体源时报错
func (s *OrganizeService) OrganizeDirectoryForSource(
    ctx context.Context,
    source *domain.MediaSource,
    sourcePath, targetPath, mediaType, template, conflictPolicy, operationMode string,
    fileIDs []string, useCategory bool,
    manualItems []domain.OrganizeManualOverride, renameItems []domain.OrganizeRenameOverride,
    progress func(OrganizeExecutionProgress), shouldStop func() bool,
) ([]OrganizeResult, error)
```

`OrganizeDirectory(sourceID int, …)` 改为：`source, _ := s.mediaSourceService.GetByID(sourceID); return s.OrganizeDirectoryForSource(ctx, source, …)`。

### 3.5 刮削适配器（新增 `scrape_adapter.go`）

```go
// PostTransferScraper 转存后刮削统一接口（按源类型分流）
type PostTransferScraper interface {
    Scrape(ctx context.Context, source *domain.MediaSource, filePaths []string) ([]ScrapeResult, error)
}

// LocalScraper 复用既有 ScrapeService（仅本地源可用）
type LocalScraper struct{ svc *ScrapeService }
func (l *LocalScraper) Scrape(ctx context.Context, source *domain.MediaSource, filePaths []string) ([]ScrapeResult, error) {
    return l.svc.ScrapeFiles(source.ID, filePaths) // 本地源有真实 ID
}

// Cloud115Scraper 115 云盘刮削（本期降级：返回 skipped + 原因，预留 NFO 生成+上传）
type Cloud115Scraper struct{ svc *ScrapeService; client Cloud115Client }
func (c *Cloud115Scraper) Scrape(ctx context.Context, source *domain.MediaSource, filePaths []string) ([]ScrapeResult, error) {
    results := make([]ScrapeResult, 0, len(filePaths))
    for _, fp := range filePaths {
        results = append(results, ScrapeResult{
            FilePath: fp, Success: false, Message: "115 云盘暂不支持 NFO 写入（本期未实现上传）",
        })
    }
    return results, nil // 注意：返回 nil error → 刮削任务标记为 completed(带 skipped 明细)，不阻断转存
}
```

选择逻辑（在 `ShareTransferService` 内）：

```go
func (s *ShareTransferService) selectScraper(source *domain.MediaSource) PostTransferScraper {
    if source.SourceType == domain.SourceTypeCloud115 {
        return s.cloud115Scraper
    }
    return s.localScraper
}
```

### 3.6 类图（Mermaid）

见同目录 `transfer-organize-scrape-class.mermaid`。

---

## 4. 程序调用流程

### 4.1 主时序：转存完成 → 整理 →（可选）刮削

见同目录 `transfer-organize-scrape-sequence.mermaid`。要点文字版：

1. **SubmitTransfer**：校验账号/目录 → `taskDAO.Create(share_transfer)` → 写含 `auto_organize/auto_scrape` 的元数据 → `go executeTransfer`。`auto=false` 时到此为止，与原版一致。
2. **executeTransfer**：逐个 `ReceiveShare` → 终态（completed / partial_failed / failed / cancelled）。
3. **终态钩子**（在 `executeTransfer` 末段，非 cancelled 且 `auto_organize`）：`go s.runPostTransferOrganize(taskId, req, targetAccount)`。
4. **runPostTransferOrganize**：
   - 构造源：`source = buildAdhocOrganizeSource(req, targetAccount, shareFolderName)`（带 `organize_source_id` 则改用 `mediaSourceService.GetByID`）。
   - `organizeTaskID = "share_transfer_organize_" + uuid`；`taskDAO.Create(organizeTaskID, share_transfer_organize, …)`；元数据写 `parent_task_id=taskId, stage="organize", source_id, source_path, target_path, media_type`。
   - 回填转存任务元数据 `organize_task_id`。
   - 列出目标目录：`fileIDs = listTransferredFileIDs(targetDirCID, targetAccount)`（取视频文件 pickcode，仅整理本次落盘文件，不碰目录中既有文件）。
   - `results = organizeService.OrganizeDirectoryForSource(ctx, source, targetDir, targetPath, mediaType, "", conflictPolicy, "move", fileIDs, true, nil, nil, onProgress, isCancelled)`。
   - 回写进度/终态到 organize 任务；若 `auto_scrape` 且非全失败 → `go s.runPostTransferScrape(...)`。
5. **runPostTransferScrape**：
   - `scrapeTaskID = "share_transfer_scrape_" + uuid`；`taskDAO.Create(..., share_transfer_scrape, ...)`；元数据写 `parent_task_id=taskId, stage="scrape"`；回填转存元数据 `scrape_task_id`。
   - `filePaths` = 整理后成功文件的 `NewPath`（115 路径）。
   - `results = selectScraper(source).Scrape(ctx, source, filePaths)`。
   - 115 → `Cloud115Scraper` 返回 skipped 明细，任务标记 `completed`（带 `scrape_skipped=N, scrape_reason=...`）；本地 → 真实刮削。

### 4.2 失败 / 部分失败 / 中断处理

| 场景 | 处理 |
|------|------|
| 转存 `failed`（全部失败） | 不触发整理/刮削；转存任务维持 `failed` |
| 转存 `partial_failed` | **仍触发**整理（只整理目标目录中实际落盘的视频文件）；整理只处理成功项 |
| 转存 `cancelled` | 不触发后续；若整理已启动，复用转存任务的取消标记（`taskDAO.IsCancelled(taskId)`）通过 `shouldStop` 传播，整理任务转 `cancelled` |
| 整理失败 | 转存任务维持原终态（整理是"附加价值"，**不回退转存成功**）；整理任务 `failed` 并写 `error_message`；`auto_scrape` 跳过 |
| 整理 `partial_failed` | 转存任务维持原终态；整理任务 `completed`/带失败明细；刮削仅对 `Success && !Skipped` 的文件 |
| 刮削 unsupported（115） | 刮削任务 `completed` + 元数据 `scrape_status=skipped`；**不报错、不阻断** |
| 刮削本地失败 | 刮削任务 `failed` 但转存/整理不受影响 |

---

## 5. 任务列表（有序 · 含依赖 · 按实现顺序）

| 任务ID | 任务名 | 源文件 | 依赖 | 优先级 |
|--------|--------|--------|------|--------|
| **T01** | 数据结构与任务类型基座 | `internal/domain/models.go`、`task.go` | — | P0 |
| **T02** | OrganizeService 增加 source 直传入口 | `internal/service/organize_service.go`、`organize_execute.go`、+测试 | T01 | P0 |
| **T03** | 刮削适配器（Local/Cloud115 分流，115 降级） | `internal/service/scrape_adapter.go`、`scrape_service.go`（暴露 NFO 生成方法） | T01 | P1 |
| **T04** | ShareTransferService 依赖注入 + 元数据落盘 + 纯函数 | `internal/service/share_transfer_service.go`、`share_transfer_post.go`（纯函数部分） | T01 | P0 |
| **T05** | 链式编排 goroutine（organize→scrape 触发、进度、失败处理） | `internal/service/share_transfer_post.go`（编排部分） | T02, T03, T04 | P0 |
| **T06** | DI 接线与 controller 注释 | `auth.go`、`internal/controller/resource_controller.go` | T04, T05 | P1 |
| **T07** | 单元测试（纯函数 + 编排 mock + source 入口） | `internal/service/share_transfer_service_test.go`、`organize_service_adhoc_test.go` | T02, T04, T05 | P1 |

依赖关系：`T01 → T02 → T05`；`T01 → T03 → T05`；`T01 → T04 → T05`；`T05 → T06 → T07`。T02 与 T03 在 T01 后可并行。

---

## 6. 依赖包 / 外部调用

| 依赖 | 用途 | 备注 |
|------|------|------|
| `github.com/google/uuid` | 生成 organize/scrape 任务 ID | 已在 `share_transfer_service.go` 使用 |
| `dao.TaskRedisDAO` | 任务登记 / 进度 / 元数据（`Create/UpdateMetadata/UpdateProgress/UpdateStatus/SetError`） | 复用既有，无需新依赖 |
| `OrganizeService`（`internal/service`） | 复用整理管线 | 需注入 |
| `ScrapeService`（`internal/service`） | 本地刮削 / NFO 内容生成 | 经 `PostTransferScraper` 接口注入 |
| `Cloud115Client` | 列出目标目录取 fileIDs；（`Cloud115Scraper` 后续）NFO 上传 | 已在 `ShareTransferService` 持有 |
| `MediaSourceService` | 当 `organize_source_id>0` 时取复用源 | 已在 `OrganizeService` 持有，编排层按需调用 |
| 外部：115 开放 API（列表/移动/重命名） | 整理落地；刮削上传（后续） | 现有 `client` 已封装 |
| 外部：TMDB API | 刮削识别（本地路径） | 由 `ScrapeService`/`TmdbService` 内部处理，本期 115 不触发 |

**不引入**任何新第三方库；115 刮削后续如需上传，复用现有 `client` 的上传能力。

---

## 7. 跨文件共享约定（Shared Knowledge）

1. **元数据字段命名**：统一小写蛇形，键名全局唯一：`auto_organize`、`auto_scrape`、`organize_source_id`、`organize_target_path`、`organize_task_id`、`scrape_task_id`、`parent_task_id`、`stage`(`organize`/`scrape`)。
2. **链式关联**：organize / scrape 任务元数据必带 `parent_task_id = 转存任务ID`；转存任务元数据回指 `organize_task_id` / `scrape_task_id`。任务中心前端按 `parent_task_id` 聚合即呈现为链。
3. **media_type 推断**（`inferMediaTypeFromName(name string) string`）：命中 `S\d{1,2}E\d{2}` / `第\d+季` / `Season` → `tv`；否则含明确剧集目录特征且多视频 → `tv`；单文件或电影特征 → `movie`；兜底 `all`。`all` 时由 `previewFile` 按文件逐条识别，安全。
4. **临时 MediaSource 构造规则**（`buildAdhocOrganizeSource`）：
   - `SourceType = cloud115`；`Cloud115ID = &req.TargetCloud115Id`；
   - `Path = TargetDirectory`；`OrganizeTargetPath = OrganizeTargetPath or TargetDirectory`；
   - `MediaType = 复用源.MediaType 或 inferMediaTypeFromName(shareFolderName)`；
   - `ConflictPolicy = req.ConflictStrategy 或 "skip"`；`OperationMode = "move"`（115 强制 move，参考 `resolveWatchOrganizeDefaults` 对 cloud115 的处理）；
   - `ID = 0`（不落库）；`Name = "转存整理-"+shareFolderName`。
5. **进度上报**：organize/scrape 任务复用 `UpdateProgress(total, processed, success, failed)` 与 `UpdateMetadata`；整理进度通过 `OrganizeExecutionProgress` 回调桥接到 `taskDAO.UpdateProgress`。
6. **取消传播**：organize/scrape 的 `shouldStop` 回调读 `taskDAO.IsCancelled(parentTaskID)`（即转存任务的取消标记），保证用户在转存任务上点"取消"能联动中止后续步骤。
7. **错误不阻断原则**：整理失败只影响整理任务；刮削（尤其 115）失败/不支持只影响刮削任务与元数据备注；**绝不**回滚已成功的转存。
8. **开关缺省语义**：`auto_organize/auto_scrape` 缺省（false）→ 完全走原流程；任何新代码路径必须在 `if req.AutoOrganize` / `if req.AutoScrape` 守卫内。

---

## 8. 待明确事项 / 需产品·用户拍板

> 以下为设计前提中的开放决策，已给出**推荐方案**与理由，请确认或推翻。

1. **【最关键】115 云盘刮削是否本期实现？**
   - 现状：NFO/海报写在本地文件系统，`ScrapeService` 对 cloud115 直接拒绝（F5）。本期若要"真刮削"，需：生成 NFO 内容（已有 `GenerateMovieNFO/GenerateEpisodeNFO`）→ 通过 115 API 作为文件上传到对应目录。工作量中～大，且涉及 115 上传风控。
   - **推荐**：本期 `auto_scrape` 对 115 走**优雅降级**（任务 `completed` + `skipped` 备注，不报错不阻断），并在转存页/任务详情给出"115 暂不支持刮削"提示；本地源（若用户把 115 挂载为本地源）仍走真实刮削。后续单列迭代实现 115 NFO 上传。
   - 备选：本期直接禁止 115 转存勾选 `auto_scrape`（前端置灰 + 后端校验返回提示）。

2. **media_type 来源**
   - **推荐**：`organize_source_id>0` 时取自该源配置；否则 `inferMediaTypeFromName(shareFolderName)`，兜底 `all`。理由：零新增 UI 字段即可跑通，且 `all` 下逐文件识别安全。

3. **整理目标路径（`OrganizeTargetPath`）**
   - **推荐**：缺省 = `TargetDirectory`（就地整理：文件按电影/剧集规则归到 `TargetDirectory/电影(或影片)` 或 `TargetDirectory/剧名/ Season x/`）。若传 `organize_source_id`，则用该源 `OrganizeTargetPath`。可选新增 UI 字段 `organize_target_path` 覆盖。
   - 风险：就地整理会在 `TargetDirectory` 下新建子目录，需确认用户是否接受"转存目录被重组织"。

4. **链式任务在任务中心的呈现**
   - **推荐**：三任务独立存在（share_transfer / share_transfer_organize / share_transfer_scrape），靠 `parent_task_id` 关联；前端"任务中心"按 `parent_task_id` 分组展示为一条链，并汇总各阶段状态。不改造 `TaskRedisDAO`（无父子原生支持）。
   - 备选：把整理/刮削作为转存任务的"子进度"嵌在同一任务元数据里（不单独建任务）。不推荐——会丢失独立取消/重试能力，且与 watch 既有"独立 auto_organize 任务"模式不一致。

5. **部分失败时是否仍触发整理**
   - **推荐**：是。仅对目标目录中实际落盘的视频文件整理（通过列表目标目录取 fileIDs 实现），与 watch "只处理新文件" 语义一致。

6. **是否需要在转存页补充「媒体源」选择器**
   - 后端**不强制**：不传 `organize_source_id` 也能跑（方案 B）。但传了能获得更准的 media_type/命名/目标规则。**推荐前端后续补充**该下拉（读取 `/v1/media-source` 列表，筛选 `cloud115` 且 `enabled`）。

---

## 附：与现有 watch 自动整理的差异对照

| 维度 | watch 自动整理 | 本方案（转存后整理） |
|------|----------------|----------------------|
| 触发源 | 115 轮询发现新文件 / fsnotify | 转存任务终态（completed/partial_failed） |
| 媒体源 | 必有（`source` 来自 `GetWatchEnabled`） | 复用源或临时源 |
| 任务类型 | `watch_auto_organize` | `share_transfer_organize` / `share_transfer_scrape` |
| 文件集合 | 轮询 diff 出的新 pickcode | 列表目标目录取出的视频 pickcode |
| 刮削 | 115 跳过（同 F4/F5） | 同左；本地源真刮削，115 降级 |
| 复用 | `organizeService.OrganizeDirectory(source.ID, …)` | `organizeService.OrganizeDirectoryForSource(source, …)` |
