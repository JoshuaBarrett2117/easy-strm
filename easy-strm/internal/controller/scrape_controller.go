package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"
)

// ScrapeController NFO刮削控制器
type ScrapeController struct {
	scrapeService   *service.ScrapeService
	organizeService *service.OrganizeService
}

// NewScrapeController 创建NFO刮削控制器实例
func NewScrapeController(scrapeService *service.ScrapeService, organizeService *service.OrganizeService) *ScrapeController {
	return &ScrapeController{scrapeService: scrapeService, organizeService: organizeService}
}

// ScrapeFile 为单个文件刮削NFO
// POST /api/media/scrape/file
func (c *ScrapeController) ScrapeFile(ctx *gin.Context) {
	var req struct {
		SourceID int    `json:"source_id" binding:"required"`
		FilePath string `json:"file_path" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	_, nfoPath, generatedFiles, err := c.scrapeService.ScrapeFile(req.SourceID, req.FilePath)
	if err != nil {
		logger.Errorf("[ScrapeController] 单文件刮削失败: source_id=%d, file=%s, err=%v", req.SourceID, req.FilePath, err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, gin.H{
		"nfo_path":        nfoPath,
		"generated_files": append([]string{nfoPath}, generatedFiles...),
		"image_paths":     generatedFiles,
		"success":         true,
	})
}

// ScrapeFiles 为多个文件刮削NFO
// POST /api/media/scrape/files
func (c *ScrapeController) ScrapeFiles(ctx *gin.Context) {
	var req struct {
		SourceID  int      `json:"source_id" binding:"required"`
		FilePaths []string `json:"file_paths" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	if len(req.FilePaths) == 0 {
		ErrorResp(ctx, http.StatusBadRequest, "文件列表不能为空")
		return
	}

	results, err := c.scrapeService.ScrapeFiles(req.SourceID, req.FilePaths)
	if err != nil {
		logger.Errorf("[ScrapeController] 批量刮削失败: source_id=%d, err=%v", req.SourceID, err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	successCount := 0
	failedCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			failedCount++
		}
	}

	SuccessResp(ctx, gin.H{
		"results": results,
		"total":   len(results),
		"success": successCount,
		"failed":  failedCount,
	})
}

// ScrapeDirectory 递归扫描目录后批量刮削视频文件
// POST /api/media/scrape/directory
func (c *ScrapeController) ScrapeDirectory(ctx *gin.Context) {
	var req struct {
		SourceID   int      `json:"source_id" binding:"required"`
		SourcePath string   `json:"source_path"`
		FileIDs    []string `json:"file_ids"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	if c.organizeService == nil {
		ErrorResp(ctx, http.StatusInternalServerError, "目录扫描服务未初始化")
		return
	}

	candidates, err := c.organizeService.ListOrganizeCandidates(req.SourceID, req.SourcePath, "all", req.FileIDs)
	if err != nil {
		logger.Errorf("[ScrapeController] 递归扫描目录失败: source_id=%d, err=%v", req.SourceID, err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	filePaths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		filePaths = append(filePaths, candidate.FilePath)
	}

	results, err := c.scrapeService.ScrapeFiles(req.SourceID, filePaths)
	if err != nil {
		logger.Errorf("[ScrapeController] 目录刮削失败: source_id=%d, err=%v", req.SourceID, err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	successCount := 0
	failedCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			failedCount++
		}
	}

	SuccessResp(ctx, gin.H{
		"results": results,
		"total":   len(results),
		"success": successCount,
		"failed":  failedCount,
	})
}
