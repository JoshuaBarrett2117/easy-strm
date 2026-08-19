package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

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

	source, err := c.mediaSourceService.GetByID(sourceID)
	if err != nil || source == nil {
		logger.Errorf("MediaSourceController[GetFiles] 获取媒体源失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "媒体源不存在")
		return
	}

	if source.SourceType == domain.SourceTypeCloud115 {
		result, err := c.getCloud115Files(ctx, source, path, page, pageSize, sortField, sortOrder, filter, search)
		if err != nil {
			logger.Errorf("MediaSourceController[GetFiles] 获取115文件列表失败: %v", err)
			ErrorResp(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		SuccessResp(ctx, result)
		return
	}

	result, err := c.mediaSourceService.GetFiles(sourceID, path, page, pageSize, sortField, sortOrder, filter, search)
	if err != nil {
		logger.Errorf("MediaSourceController[GetFiles] 获取文件列表失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, result)
}

// getCloud115Files 获取115云盘文件列表。
func (c *MediaSourceController) getCloud115Files(ctx *gin.Context, source *domain.MediaSource, path string, page, pageSize int, sortField, sortOrder, filter, search string) (*domain.FileListResult, error) {
	cloud115, err := c.cloud115Service.GetByID(*source.Cloud115ID)
	if err != nil || cloud115 == nil {
		logger.Errorf("MediaSourceController[getCloud115Files] 115账号不存在: cloud115_id=%d, error=%v", *source.Cloud115ID, err)
		return nil, fmt.Errorf("115账号不存在")
	}

	cid := resolveCloud115CID(path, source.Path)
	if strings.HasPrefix(cid, "/") {
		cid, err = c.client.GetCIDByPath(cid, cloud115.ID, cloud115.Cookie)
		if err != nil {
			logger.Errorf("MediaSourceController[getCloud115Files] 绝对路径解析失败: path=%s, error=%v", cid, err)
			return nil, fmt.Errorf("115 目录不存在或无法访问: %s", resolveCloud115CID(path, source.Path))
		}
	}
	cidInt, err := strconv.Atoi(cid)
	if err != nil {
		logger.Errorf("MediaSourceController[getCloud115Files] 目录解析结果无效: %s, error=%v", cid, err)
		return nil, fmt.Errorf("115 目录解析结果无效")
	}

	logger.Infof("MediaSourceController[getCloud115Files] 开始获取115文件列表: cloud115_id=%d, cid=%d", cloud115.ID, cidInt)
	fileList, err := c.client.GetFileList(cidInt, 1, 0, 1000, cloud115.ID, cloud115.Cookie)
	if err != nil {
		logger.Errorf("MediaSourceController[getCloud115Files] 获取115文件列表失败: %v", err)
		return nil, fmt.Errorf("获取115文件列表失败: %v", err)
	}

	files := make([]domain.MediaFile, 0, len(fileList.Files))
	fileIDs := make([]string, 0, len(fileList.Files))
	for _, f := range fileList.Files {
		isDir := f.FileID == "" || f.Type == "folder"

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
			ModifiedTime: time.Now(), // 115 API 未返回修改时间，保持原行为使用当前时间。
			IsDirectory:  isDir,
			PickCode:     f.PickCode,
			CID:          cid,
			SHA1:         f.Sha1,
		}
		files = append(files, file)
		fileIDs = append(fileIDs, fileID)
	}

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

	if search != "" || filter != "" {
		files = c.filterFiles(files, filter, search)
	}
	if sortField != "" {
		files = c.sortFiles(files, sortField, sortOrder)
	}

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

	source, err := c.mediaSourceService.GetByID(sourceID)
	if err != nil || source == nil {
		logger.Errorf("MediaSourceController[SearchFiles] 媒体源不存在: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "媒体源不存在")
		return
	}

	var result *domain.FileListResult
	if source.SourceType == domain.SourceTypeCloud115 {
		result, err = c.getCloud115Files(ctx, source, "", page, pageSize, "name", "asc", "", keyword)
		if err != nil {
			logger.Errorf("MediaSourceController[SearchFiles] 115文件搜索失败: %v", err)
			ErrorResp(ctx, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
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
