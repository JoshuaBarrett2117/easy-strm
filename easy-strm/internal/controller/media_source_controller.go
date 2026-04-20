package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"
)

// MediaSourceController 媒体源控制器
type MediaSourceController struct {
	mediaSourceService *service.MediaSourceService
	cloud115Service    *service.Cloud115Service
	watchService       watchLifecycle
	client             service.Cloud115Client
	tmdbCacheDAO       *dao.TmdbCacheDAO
}

type watchLifecycle interface {
	StartWatching(sourceID int) error
	StopWatching(sourceID int)
}

// NewMediaSourceController 创建媒体源控制器实例
func NewMediaSourceController(mediaSourceService *service.MediaSourceService, cloud115Service *service.Cloud115Service, watchService watchLifecycle, client service.Cloud115Client) *MediaSourceController {
	return &MediaSourceController{
		mediaSourceService: mediaSourceService,
		cloud115Service:    cloud115Service,
		watchService:       watchService,
		client:             client,
		tmdbCacheDAO:       dao.NewTmdbCacheDAO(),
	}
}

// GetList 获取媒体源列表
// GET /api/media/sources
func (c *MediaSourceController) GetList(ctx *gin.Context) {
	sortField := ctx.DefaultQuery("sort_field", "priority")
	sortOrder := ctx.DefaultQuery("sort_order", "asc")

	list, err := c.mediaSourceService.GetAll(sortField, sortOrder)
	if err != nil {
		logger.Errorf("MediaSourceController[GetList] 获取列表失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取媒体源列表失败")
		return
	}

	SuccessResp(ctx, gin.H{
		"data":  list,
		"total": len(list),
	})
}

// GetByID 根据ID获取媒体源
// GET /api/media/sources/:id
func (c *MediaSourceController) GetByID(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体源ID")
		return
	}

	source, err := c.mediaSourceService.GetByID(id)
	if err != nil || source == nil {
		logger.Errorf("MediaSourceController[GetByID] 获取详情失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "媒体源不存在")
		return
	}

	SuccessResp(ctx, source)
}

// Create 创建媒体源
// POST /api/media/sources
func (c *MediaSourceController) Create(ctx *gin.Context) {
	var req struct {
		Name               string `json:"name" binding:"required"`
		SourceType         string `json:"source_type" binding:"required"`
		Path               string `json:"path" binding:"required"`
		WatchPath          string `json:"watch_path"`
		Cloud115ID         *int   `json:"cloud115_id"`
		Priority           int    `json:"priority"`
		Enabled            bool   `json:"enabled"`
		OrganizeTargetPath string `json:"organize_target_path"`
		MediaType          string `json:"media_type"`
		ConflictPolicy     string `json:"conflict_policy"`
		OperationMode      string `json:"operation_mode"`
		AutoOrganize       bool   `json:"auto_organize"`
		WatchEnabled       bool   `json:"watch_enabled"`
		WatchInterval      int    `json:"watch_interval"`
		EmbyLibraryID      string `json:"emby_library_id"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Warnf("MediaSourceController[Create] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	// 设置默认值
	if req.Priority == 0 {
		req.Priority = 10
	}

	// 验证本地路径（路径不存在时仅警告，不阻止创建）
	// 业务背景：用户可能先配置媒体源，稍后再创建目录
	if req.SourceType == domain.SourceTypeLocal {
		if err := c.mediaSourceService.ValidateLocalPath(req.Path); err != nil {
			logger.Warnf("MediaSourceController[Create] 本地路径不存在或无法访问，但仍允许创建: %s, 错误: %v", req.Path, err)
			// 不返回错误，允许创建媒体源
		}
		if req.WatchPath != "" && req.WatchPath != req.Path {
			if err := c.mediaSourceService.ValidateLocalPath(req.WatchPath); err != nil {
				ErrorResp(ctx, http.StatusBadRequest, "监控目录无效")
				return
			}
		}
	}

	source, err := c.mediaSourceService.Create(
		req.Name,
		req.SourceType,
		req.Path,
		req.WatchPath,
		req.Cloud115ID,
		req.Priority,
		req.Enabled,
		req.OrganizeTargetPath,
		req.MediaType,
		req.ConflictPolicy,
		req.OperationMode,
		req.AutoOrganize,
		req.WatchEnabled,
		req.WatchInterval,
		req.EmbyLibraryID,
	)
	if err != nil {
		logger.Errorf("MediaSourceController[Create] 创建失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("MediaSourceController[Create] 创建媒体源成功: %s (ID: %d)", source.Name, source.ID)
	c.syncWatchState(source)
	SuccessResp(ctx, gin.H{
		"message": "创建成功",
		"data":    source,
	})
}

// Update 更新媒体源
// PUT /api/media/sources/:id
// 支持部分字段更新：只需提供需要更新的字段即可
func (c *MediaSourceController) Update(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体源ID")
		return
	}

	// 更新请求体：所有字段均为可选，支持部分更新
	// 业务背景：用户可能只想更新名称或优先级，无需提供完整信息
	var req struct {
		Name               *string `json:"name"`
		SourceType         *string `json:"source_type"`
		Path               *string `json:"path"`
		WatchPath          *string `json:"watch_path"`
		Cloud115ID         *int    `json:"cloud115_id"`
		Priority           *int    `json:"priority"`
		Enabled            *bool   `json:"enabled"`
		OrganizeTargetPath *string `json:"organize_target_path"`
		MediaType          *string `json:"media_type"`
		ConflictPolicy     *string `json:"conflict_policy"`
		OperationMode      *string `json:"operation_mode"`
		AutoOrganize       *bool   `json:"auto_organize"`
		WatchEnabled       *bool   `json:"watch_enabled"`
		WatchInterval      *int    `json:"watch_interval"`
		EmbyLibraryID      *string `json:"emby_library_id"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Warnf("MediaSourceController[Update] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	// 获取现有媒体源信息，用于填充未提供的字段
	existingSource, err := c.mediaSourceService.GetByID(id)
	if err != nil || existingSource == nil {
		logger.Errorf("MediaSourceController[Update] 获取媒体源失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "媒体源不存在")
		return
	}

	// 合并更新数据：未提供的字段保留原值
	name := existingSource.Name
	if req.Name != nil {
		name = *req.Name
	}

	sourceType := existingSource.SourceType
	if req.SourceType != nil {
		sourceType = *req.SourceType
	}

	path := existingSource.Path
	if req.Path != nil {
		path = *req.Path
	}

	watchPath := existingSource.WatchPath
	if req.WatchPath != nil {
		watchPath = *req.WatchPath
	}

	cloud115ID := existingSource.Cloud115ID
	if req.Cloud115ID != nil {
		cloud115ID = req.Cloud115ID
	}

	priority := existingSource.Priority
	if req.Priority != nil {
		priority = *req.Priority
	}

	enabled := existingSource.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	organizeTargetPath := existingSource.OrganizeTargetPath
	if req.OrganizeTargetPath != nil {
		organizeTargetPath = *req.OrganizeTargetPath
	}

	mediaType := existingSource.MediaType
	if req.MediaType != nil {
		mediaType = *req.MediaType
	}

	conflictPolicy := existingSource.ConflictPolicy
	if req.ConflictPolicy != nil {
		conflictPolicy = *req.ConflictPolicy
	}

	operationMode := existingSource.OperationMode
	if req.OperationMode != nil {
		operationMode = *req.OperationMode
	}

	autoOrganize := existingSource.AutoOrganize
	if req.AutoOrganize != nil {
		autoOrganize = *req.AutoOrganize
	}

	watchEnabled := existingSource.WatchEnabled
	if req.WatchEnabled != nil {
		watchEnabled = *req.WatchEnabled
	}

	watchInterval := existingSource.WatchInterval
	if req.WatchInterval != nil {
		watchInterval = *req.WatchInterval
	}

	embyLibraryID := existingSource.EmbyLibraryID
	if req.EmbyLibraryID != nil {
		embyLibraryID = *req.EmbyLibraryID
	}

	// 验证本地路径（路径不存在时仅警告，不阻止更新）
	// 业务背景：用户可能先配置媒体源，稍后再创建目录
	if sourceType == domain.SourceTypeLocal {
		if err := c.mediaSourceService.ValidateLocalPath(path); err != nil {
			logger.Warnf("MediaSourceController[Update] 本地路径不存在或无法访问，但仍允许更新: %s, 错误: %v", path, err)
			// 不返回错误，允许更新媒体源
		}
		if watchPath != "" && watchPath != path {
			if err := c.mediaSourceService.ValidateLocalPath(watchPath); err != nil {
				ErrorResp(ctx, http.StatusBadRequest, "监控目录无效")
				return
			}
		}
	}

	source, err := c.mediaSourceService.Update(
		id,
		name,
		sourceType,
		path,
		watchPath,
		cloud115ID,
		priority,
		enabled,
		organizeTargetPath,
		mediaType,
		conflictPolicy,
		operationMode,
		autoOrganize,
		watchEnabled,
		watchInterval,
		embyLibraryID,
	)
	if err != nil {
		logger.Errorf("MediaSourceController[Update] 更新失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("MediaSourceController[Update] 更新媒体源成功: %s (ID: %d)", source.Name, id)
	c.syncWatchState(source)
	SuccessResp(ctx, gin.H{
		"message": "更新成功",
		"data":    source,
	})
}

// Delete 删除媒体源
// DELETE /api/media/sources/:id
func (c *MediaSourceController) Delete(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体源ID")
		return
	}

	if err := c.mediaSourceService.Delete(id); err != nil {
		logger.Errorf("MediaSourceController[Delete] 删除失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("MediaSourceController[Delete] 删除媒体源成功: ID %d", id)
	c.stopWatchState(id)
	SuccessResp(ctx, gin.H{
		"message": "删除成功",
	})
}

func (c *MediaSourceController) syncWatchState(source *domain.MediaSource) {
	if c.watchService == nil || source == nil {
		return
	}

	c.watchService.StopWatching(source.ID)
	if source.Enabled && source.WatchEnabled {
		if err := c.watchService.StartWatching(source.ID); err != nil {
			logger.Warnf("MediaSourceController[syncWatchState] 启动监控失败: source_id=%d, error=%v", source.ID, err)
		}
	}
}

func (c *MediaSourceController) stopWatchState(sourceID int) {
	if c.watchService == nil {
		return
	}
	c.watchService.StopWatching(sourceID)
}

// GetFiles 获取文件列表
// GET /api/media/files
func (c *MediaSourceController) GetFiles(ctx *gin.Context) {
	sourceID, err := strconv.Atoi(ctx.Query("source_id"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体源ID")
		return
	}

	path := ctx.DefaultQuery("path", "")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "50"))
	sortField := ctx.DefaultQuery("sort_field", "name")
	sortOrder := ctx.DefaultQuery("sort_order", "asc")
	filter := ctx.Query("filter")
	search := ctx.Query("search")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	// 获取媒体源信息
	source, err := c.mediaSourceService.GetByID(sourceID)
	if err != nil || source == nil {
		logger.Errorf("MediaSourceController[GetFiles] 获取媒体源失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "媒体源不存在")
		return
	}

	// 根据媒体源类型处理
	if source.SourceType == domain.SourceTypeCloud115 {
		// 115云盘文件列表
		result, err := c.getCloud115Files(ctx, source, path, page, pageSize, sortField, sortOrder, filter, search)
		if err != nil {
			logger.Errorf("MediaSourceController[GetFiles] 获取115文件列表失败: %v", err)
			ErrorResp(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		SuccessResp(ctx, result)
		return
	}

	// 本地文件列表
	result, err := c.mediaSourceService.GetFiles(sourceID, path, page, pageSize, sortField, sortOrder, filter, search)
	if err != nil {
		logger.Errorf("MediaSourceController[GetFiles] 获取文件列表失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, result)
}

// getCloud115Files 获取115云盘文件列表
// 使用动态初始化方式:每次调用时根据账号Cookie创建或复用driver实例
func (c *MediaSourceController) getCloud115Files(ctx *gin.Context, source *domain.MediaSource, path string, page, pageSize int, sortField, sortOrder, filter, search string) (*domain.FileListResult, error) {
	// 获取115账号信息
	cloud115, err := c.cloud115Service.GetByID(*source.Cloud115ID)
	if err != nil || cloud115 == nil {
		logger.Errorf("MediaSourceController[getCloud115Files] 115账号不存在: cloud115_id=%d, error=%v", *source.Cloud115ID, err)
		return nil, fmt.Errorf("115账号不存在")
	}

	// 解析CID(目录ID)
	// 业务逻辑：前端传递的是路径格式（如 "/" 或 "/电影"），需要转换为115云盘的目录ID
	var cid string
	
	// 处理路径格式的特殊情况
	if path == "" || path == "/" {
		// 如果path为空或为根目录"/"，使用媒体源配置的Path作为根目录ID
		cid = source.Path
		// 如果source.Path也是空或"/"，默认使用"0"（115云盘根目录）
		if cid == "" || cid == "/" {
			cid = "0"
		}
	} else {
		// 否则使用传入的path（假设前端后续会传递实际的目录ID）
		cid = path
	}
	
	// 如果仍然为空，默认为115云盘根目录(0)
	if cid == "" {
		cid = "0"
	}

	// 转换CID为整数
	cidInt, err := strconv.Atoi(cid)
	if err != nil {
		logger.Errorf("MediaSourceController[getCloud115Files] CID格式错误: %s, error=%v", cid, err)
		return nil, fmt.Errorf("目录ID格式错误: %s", cid)
	}

	// 调用115客户端获取文件列表
	// 注意: GetFileList内部会自动处理客户端初始化(使用Cookie创建或复用driver实例)
	logger.Infof("MediaSourceController[getCloud115Files] 开始获取115文件列表: cloud115_id=%d, cid=%d", cloud115.ID, cidInt)
	fileList, err := c.client.GetFileList(cidInt, 1, 0, 1000, cloud115.ID, cloud115.Cookie)
	if err != nil {
		logger.Errorf("MediaSourceController[getCloud115Files] 获取115文件列表失败: %v", err)
		return nil, fmt.Errorf("获取115文件列表失败: %v", err)
	}

	// 转换文件列表为统一格式
	// 注意: driver.FileInfo通过FileID和Type字段判断是否为目录
	var files []domain.MediaFile
	var fileIDs []string
	for _, f := range fileList.Files {
		// 判断是否为目录: FileID为空或Type为"folder"表示目录
		isDir := f.FileID == "" || f.Type == "folder"
		
		// 确定文件ID和CID
		// 对于目录：ID和CID都使用CategoryID
		// 对于文件：ID使用FileID，CID使用CategoryID（父目录ID）
		var fileID, cid string
		if isDir {
			fileID = string(f.CategoryID)
			cid = string(f.CategoryID)
		} else {
			fileID = f.FileID
			cid = string(f.CategoryID)
		}
		
		file := domain.MediaFile{
			ID:           fileID,
			Name:         f.Name,
			Path:         f.Name,
			Type:         c.fileTypeFrom115(f.Name, isDir),
			Size:         int64(f.Size),
			ModifiedTime: time.Now(), // 115 API未返回修改时间,使用当前时间
			IsDirectory:  isDir,
			PickCode:     f.PickCode,
			CID:          cid, // 目录ID，用于前端导航
			SHA1:         f.Sha1,
		}
		files = append(files, file)
		// 收集文件ID用于批量查询TMDB缓存
		fileIDs = append(fileIDs, fileID)
	}

	// 批量查询TMDB缓存填充TmdbTitle
	if len(fileIDs) > 0 {
		tmdbCacheMap, err := c.tmdbCacheDAO.GetByQueryKeys(fileIDs)
		if err != nil {
			logger.Warnf("MediaSourceController[getCloud115Files] 查询TMDB缓存失败: %v", err)
		} else {
			for i := range files {
				if cache, ok := tmdbCacheMap[files[i].ID]; ok {
					files[i].TmdbTitle = cache.Title
				}
			}
		}
	}

	// 过滤和搜索
	if search != "" || filter != "" {
		files = c.filterFiles(files, filter, search)
	}

	// 排序
	if sortField != "" {
		files = c.sortFiles(files, sortField, sortOrder)
	}

	// 分页处理
	total := len(files)
	start := (page - 1) * pageSize
	if start > total {
		start = 0
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	files = files[start:end]

	result := &domain.FileListResult{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		Files:      files,
		Breadcrumb: c.buildBreadcrumb(path),
	}

	logger.Infof("MediaSourceController[getCloud115Files] 获取115文件列表成功: CID=%s, total=%d", cid, total)
	return result, nil
}

// SearchFiles 搜索文件
// GET /api/media/files/search
// 支持本地和115云盘文件搜索
func (c *MediaSourceController) SearchFiles(ctx *gin.Context) {
	sourceID, err := strconv.Atoi(ctx.Query("source_id"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体源ID")
		return
	}

	keyword := ctx.Query("keyword")
	if keyword == "" {
		ErrorResp(ctx, http.StatusBadRequest, "搜索关键词不能为空")
		return
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "50"))

	// 获取媒体源
	source, err := c.mediaSourceService.GetByID(sourceID)
	if err != nil || source == nil {
		logger.Errorf("MediaSourceController[SearchFiles] 媒体源不存在: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "媒体源不存在")
		return
	}

	// 根据媒体源类型处理搜索
	var result *domain.FileListResult
	if source.SourceType == domain.SourceTypeCloud115 {
		// 115云盘搜索:调用getCloud115Files并传入搜索关键词
		// 注意:115 API不支持服务端搜索,采用客户端过滤方式
		result, err = c.getCloud115Files(ctx, source, "", page, pageSize, "name", "asc", "", keyword)
		if err != nil {
			logger.Errorf("MediaSourceController[SearchFiles] 115文件搜索失败: %v", err)
			ErrorResp(ctx, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		// 本地文件搜索
		result, err = c.mediaSourceService.GetFiles(sourceID, "", page, pageSize, "name", "asc", "", keyword)
		if err != nil {
			logger.Errorf("MediaSourceController[SearchFiles] 本地文件搜索失败: %v", err)
			ErrorResp(ctx, http.StatusInternalServerError, err.Error())
			return
		}
	}

	logger.Infof("MediaSourceController[SearchFiles] 搜索完成: keyword=%s, total=%d", keyword, result.Total)
	SuccessResp(ctx, result)
}

// fileTypeFrom115 从115文件判断文件类型
// 参数:
//   - name: 文件名
//   - isDir: 是否为目录
// 返回:
//   - string: 文件类型
func (c *MediaSourceController) fileTypeFrom115(name string, isDir bool) string {
	if isDir {
		return "dir"
	}

	ext := strings.ToLower(getFileExt(name))
	switch ext {
	case ".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".ts", ".m2ts":
		return "video"
	case ".mp3", ".flac", ".wav", ".aac", ".ogg", ".m4a":
		return "audio"
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp":
		return "image"
	case ".srt", ".ass", ".ssa", ".vtt":
		return "subtitle"
	default:
		return "file"
	}
}

// getFileExt 获取文件扩展名
func getFileExt(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[i:]
		}
	}
	return ""
}

// filterFiles 过滤文件列表
func (c *MediaSourceController) filterFiles(files []domain.MediaFile, filter, search string) []domain.MediaFile {
	var result []domain.MediaFile
	for _, file := range files {
		// 搜索过滤
		if search != "" && !strings.Contains(strings.ToLower(file.Name), strings.ToLower(search)) {
			continue
		}

		// 类型过滤
		if filter != "" && file.Type != filter {
			continue
		}

		result = append(result, file)
	}
	return result
}

// sortFiles 对文件列表进行排序
func (c *MediaSourceController) sortFiles(files []domain.MediaFile, sortField, sortOrder string) []domain.MediaFile {
	// 复制切片避免修改原数据
	result := make([]domain.MediaFile, len(files))
	copy(result, files)

	// 排序逻辑（简化实现）
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			// 目录优先
			if result[i].IsDirectory && !result[j].IsDirectory {
				continue
			}
			if !result[i].IsDirectory && result[j].IsDirectory {
				result[i], result[j] = result[j], result[i]
				continue
			}

			// 按指定字段排序
			var compare bool
			switch sortField {
			case "name":
				if sortOrder == "asc" {
					compare = result[i].Name > result[j].Name
				} else {
					compare = result[i].Name < result[j].Name
				}
			case "size":
				if sortOrder == "asc" {
					compare = result[i].Size > result[j].Size
				} else {
					compare = result[i].Size < result[j].Size
				}
			default:
				if sortOrder == "asc" {
					compare = result[i].Name > result[j].Name
				} else {
					compare = result[i].Name < result[j].Name
				}
			}

			if compare {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

// buildBreadcrumb 构建面包屑导航
func (c *MediaSourceController) buildBreadcrumb(path string) []domain.PathItem {
	if path == "" {
		return []domain.PathItem{}
	}

	items := []domain.PathItem{}
	parts := strings.Split(path, "/")
	currentPath := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if currentPath != "" {
			currentPath = currentPath + "/" + part
		} else {
			currentPath = part
		}
		items = append(items, domain.PathItem{
			Name: part,
			Path: currentPath,
		})
	}

	return items
}
