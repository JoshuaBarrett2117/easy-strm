package controller

import (
	"encoding/json"
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

// TmdbController TMDB 控制器
type TmdbController struct {
	tmdbService     *service.TmdbService
	cacheDAO        *dao.TmdbCacheDAO
	systemConfigDAO *dao.SystemConfigDAO
}

// NewTmdbController 创建 TMDB 控制器实例
func NewTmdbController(tmdbService *service.TmdbService) *TmdbController {
	controller := &TmdbController{
		tmdbService:     tmdbService,
		cacheDAO:        dao.NewTmdbCacheDAO(),
		systemConfigDAO: dao.NewSystemConfigDAO(),
	}
	tmdbService.SetFilenameRecognitionRuleStore(controller.systemConfigDAO)
	return controller
}

// ParseFilename 使用整理链路的当前规则解析文件名，不访问 TMDB。
// POST /api/media/tmdb/parse-filename
func (c *TmdbController) ParseFilename(ctx *gin.Context) {
	var req struct {
		Filename string `json:"filename" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "文件名不能为空")
		return
	}
	req.Filename = strings.TrimSpace(req.Filename)
	if req.Filename == "" {
		ErrorResp(ctx, http.StatusBadRequest, "文件名不能为空")
		return
	}
	SuccessResp(ctx, c.tmdbService.ParseFilename(req.Filename))
}

// GetFilenameRecognitionRules 获取当前文件名识别规则。
// GET /api/media/tmdb/filename-rules
func (c *TmdbController) GetFilenameRecognitionRules(ctx *gin.Context) {
	SuccessResp(ctx, c.tmdbService.GetFilenameRecognitionRules())
}

// UpdateFilenameRecognitionRules 校验并保存文件名识别规则。
// PUT /api/media/tmdb/filename-rules
func (c *TmdbController) UpdateFilenameRecognitionRules(ctx *gin.Context) {
	var req struct {
		Rules []service.FilenameRecognitionRule `json:"rules" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的文件名识别规则")
		return
	}
	result, err := c.tmdbService.SaveFilenameRecognitionRules(req.Rules)
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, result)
}

// ResetFilenameRecognitionRules 恢复内置常用文件名识别模板。
// POST /api/media/tmdb/filename-rules/reset
func (c *TmdbController) ResetFilenameRecognitionRules(ctx *gin.Context) {
	result, err := c.tmdbService.ResetFilenameRecognitionRules()
	if err != nil {
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	SuccessResp(ctx, result)
}

// Search 搜索媒体信息
// GET /api/media/tmdb/search
// 参数:
//   - keyword: 搜索关键词
//   - year: 年份（可选）
//   - type: 媒体类型 movie/tv（可选，不传则同时搜索）
func (c *TmdbController) Search(ctx *gin.Context) {
	keyword := ctx.Query("keyword")
	if keyword == "" {
		ErrorResp(ctx, http.StatusBadRequest, "搜索关键词不能为空")
		return
	}

	year := 0
	if yearStr := ctx.Query("year"); yearStr != "" {
		year, _ = strconv.Atoi(yearStr)
	}

	mediaType := ctx.Query("type")
	metadataSource := ctx.Query("metadata_source")

	var results []domain.TmdbSearchResult
	var err error

	// 根据媒体类型搜索
	switch mediaType {
	case "movie":
		results, err = c.tmdbService.SearchMovieBySource(keyword, year, metadataSource)
	case "tv":
		results, err = c.tmdbService.SearchTV(keyword, year)
	default:
		// 同时搜索电影和剧集
		movieResults, movieErr := c.tmdbService.SearchMovieBySource(keyword, year, metadataSource)
		tvResults, tvErr := c.tmdbService.SearchTV(keyword, year)

		if movieErr != nil && tvErr != nil {
			logger.Errorf("TmdbController[Search] 搜索失败: movieErr=%v, tvErr=%v", movieErr, tvErr)
			ErrorResp(ctx, http.StatusInternalServerError, "搜索失败")
			return
		}

		// 合并结果，各取前2个
		for i := 0; i < 2 && i < len(movieResults); i++ {
			results = append(results, movieResults[i])
		}
		for i := 0; i < 2 && i < len(tvResults); i++ {
			results = append(results, tvResults[i])
		}
	}

	if err != nil {
		logger.Errorf("TmdbController[Search] 搜索失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("TmdbController[Search] 搜索完成: keyword=%s, results=%d", keyword, len(results))
	SuccessResp(ctx, gin.H{
		"data":  results,
		"total": len(results),
	})
}

// Identify 识别文件
// POST /api/media/tmdb/identify
// 用于手动关联文件与 TMDB 信息
func (c *TmdbController) Identify(ctx *gin.Context) {
	var req domain.TmdbIdentifyRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Errorf("TmdbController[Identify] 绑定请求失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	logger.Infof("TmdbController[Identify] 开始识别: file_id=%s, tmdb_id=%d, tmdb_type=%s", req.FileID, req.TmdbID, req.TmdbType)

	// 获取 TMDB 详情以获取完整信息
	var detail map[string]interface{}
	var err error
	if req.TmdbType == "movie" {
		detail, err = c.tmdbService.GetMovieDetailBySource(req.TmdbID, req.MetadataSource, req.MetadataID, req.MetadataProvider)
	} else {
		detail, err = c.tmdbService.GetTVDetail(req.TmdbID)
	}

	if err != nil {
		logger.Errorf("TmdbController[Identify] 获取 TMDB 详情失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取 TMDB 详情失败: "+err.Error())
		return
	}

	// 保存识别结果到缓存（以 file_id 作为 query_key）
	cacheKey := c.tmdbService.CacheKeyForSource(req.FileID, req.TmdbType, req.MetadataSource)
	cache := &dao.TmdbCache{
		QueryKey:  cacheKey,
		MediaType: req.TmdbType,
		TmdbID:    req.TmdbID,
		Title:     req.Title,
	}

	if preferredTitle, preferredOriginalTitle := pickPreferredTitlesFromDetail(detail, req.TmdbType, req.Title); preferredTitle != "" {
		cache.Title = preferredTitle
		cache.OriginalTitle = preferredOriginalTitle
	}

	// 从详情中提取信息
	if originalTitle, ok := detail["original_title"].(string); ok && cache.OriginalTitle == "" {
		cache.OriginalTitle = originalTitle
	}
	if originalName, ok := detail["original_name"].(string); ok && cache.OriginalTitle == "" {
		cache.OriginalTitle = originalName
	}
	if posterPath, ok := detail["poster_path"].(string); ok {
		cache.PosterPath = posterPath
	}
	if overview, ok := detail["overview"].(string); ok {
		cache.Overview = overview
	}
	if voteAverage, ok := detail["vote_average"].(float64); ok {
		cache.VoteAverage = voteAverage
	}

	// 序列化详情为 JSON
	if req.MetadataSource != "" {
		detail["metadata_source"] = req.MetadataSource
	}
	if req.MetadataID != "" {
		detail["metadata_id"] = req.MetadataID
	}
	if req.MetadataProvider != "" {
		detail["metadata_provider"] = req.MetadataProvider
	}
	rawData, err := json.Marshal(detail)
	if err != nil {
		logger.Warnf("TmdbController[Identify] 序列化详情失败: %v", err)
	} else {
		cache.RawData = rawData
	}

	// 设置过期时间（7天后）
	cache.ExpireAt = time.Now().Add(7 * 24 * time.Hour)

	// 检查是否已存在缓存
	existing, _ := c.cacheDAO.GetByQueryKey(cacheKey, req.TmdbType)
	if existing != nil {
		// 更新逻辑
		cache.ID = existing.ID
		if err := c.cacheDAO.Update(cache); err != nil {
			logger.Errorf("TmdbController[Identify] 更新缓存失败: %v", err)
			ErrorResp(ctx, http.StatusInternalServerError, "更新识别结果失败")
			return
		}
	} else {
		// 保存到数据库
		if err := c.cacheDAO.Create(cache); err != nil {
			logger.Errorf("TmdbController[Identify] 保存缓存失败: %v", err)
			ErrorResp(ctx, http.StatusInternalServerError, "保存识别结果失败")
			return
		}
	}

	logger.Infof("TmdbController[Identify] 识别完成: file_id=%s, tmdb_id=%d", req.FileID, req.TmdbID)
	SuccessResp(ctx, gin.H{
		"success": true,
		"message": "识别成功",
		"tmdb_id": req.TmdbID,
		"title":   cache.Title,
	})
}

// AutoIdentify 自动识别文件，返回 Top 3 候选供前端选择
// POST /api/media/tmdb/auto-identify
// 不写入缓存，仅返回候选列表；用户选择后再调用 Identify 端点绑定
func (c *TmdbController) AutoIdentify(ctx *gin.Context) {
	var req struct {
		Filename       string `json:"filename" binding:"required"`
		FilePath       string `json:"file_path"`
		MediaType      string `json:"media_type"` // 可选：movie | tv，不传则自动判断
		MetadataSource string `json:"metadata_source"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Errorf("TmdbController[AutoIdentify] 绑定请求失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	identifyInput := req.Filename
	if req.FilePath != "" {
		identifyInput = req.FilePath
	}

	logger.Infof("TmdbController[AutoIdentify] 开始自动识别: filename=%s", identifyInput)

	result, err := c.tmdbService.GetCandidatesWithPathBySource(identifyInput, req.MetadataSource)
	if err != nil {
		logger.Errorf("TmdbController[AutoIdentify] 获取候选失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取候选失败: "+err.Error())
		return
	}

	logger.Infof("TmdbController[AutoIdentify] 完成: filename=%s, candidates=%d", identifyInput, len(result.Candidates))
	SuccessResp(ctx, result)
}

// BatchIdentify 批量识别文件
// POST /api/media/tmdb/batch-identify
func pickPreferredTitlesFromDetail(detail map[string]interface{}, mediaType, fallbackTitle string) (string, string) {
	title := fallbackTitle
	originalTitle := ""

	if mediaType == "tv" {
		if value, ok := detail["name"].(string); ok && value != "" {
			title = value
		}
		if value, ok := detail["original_name"].(string); ok && value != "" {
			originalTitle = value
		}
	} else {
		if value, ok := detail["title"].(string); ok && value != "" {
			title = value
		}
		if value, ok := detail["original_title"].(string); ok && value != "" {
			originalTitle = value
		}
	}

	if translations, ok := detail["translations"].(map[string]interface{}); ok {
		if items, ok := translations["translations"].([]interface{}); ok {
			for _, item := range items {
				entry, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				lang, _ := entry["iso_639_1"].(string)
				if lang != "zh" {
					continue
				}
				data, ok := entry["data"].(map[string]interface{})
				if !ok {
					continue
				}
				if mediaType == "tv" {
					if value, ok := data["name"].(string); ok && value != "" {
						title = value
						break
					}
				} else {
					if value, ok := data["title"].(string); ok && value != "" {
						title = value
						break
					}
				}
			}
		}
	}

	if title == "" {
		title = originalTitle
	}
	if originalTitle == "" {
		originalTitle = title
	}
	return title, originalTitle
}

func (c *TmdbController) BatchIdentify(ctx *gin.Context) {
	var req struct {
		Filenames []string `json:"filenames" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	if len(req.Filenames) == 0 {
		ErrorResp(ctx, http.StatusBadRequest, "文件列表不能为空")
		return
	}

	results := make([]*domain.TmdbIdentifyResult, 0, len(req.Filenames))
	for _, filename := range req.Filenames {
		result, err := c.tmdbService.IdentifyFileWithPath(filename)
		if err != nil {
			logger.Warnf("TmdbController[BatchIdentify] 识别失败: filename=%s, err=%v", filename, err)
			results = append(results, &domain.TmdbIdentifyResult{
				Success:  false,
				Message:  err.Error(),
				Filename: filename,
			})
		} else {
			results = append(results, result)
		}
	}

	logger.Infof("TmdbController[BatchIdentify] 批量识别完成: total=%d", len(results))
	SuccessResp(ctx, gin.H{
		"data":  results,
		"total": len(results),
	})
}

// GetMovieDetail 获取电影详情
// GET /api/media/tmdb/movie/:id
func (c *TmdbController) GetMovieDetail(ctx *gin.Context) {
	tmdbIDStr := ctx.Param("id")
	if tmdbIDStr == "" {
		ErrorResp(ctx, http.StatusBadRequest, "TMDB ID 不能为空")
		return
	}

	tmdbID, err := strconv.Atoi(tmdbIDStr)
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的 TMDB ID")
		return
	}

	detail, err := c.tmdbService.GetMovieDetail(tmdbID)
	if err != nil {
		logger.Errorf("TmdbController[GetMovieDetail] 获取详情失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, detail)
}

// GetTVDetail 获取剧集详情
// GET /api/media/tmdb/tv/:id
func (c *TmdbController) GetTVDetail(ctx *gin.Context) {
	tmdbIDStr := ctx.Param("id")
	if tmdbIDStr == "" {
		ErrorResp(ctx, http.StatusBadRequest, "TMDB ID 不能为空")
		return
	}

	tmdbID, err := strconv.Atoi(tmdbIDStr)
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的 TMDB ID")
		return
	}

	detail, err := c.tmdbService.GetTVDetail(tmdbID)
	if err != nil {
		logger.Errorf("TmdbController[GetTVDetail] 获取详情失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, detail)
}

// UpdateAPIKey 更新 TMDB API Key
// POST /api/media/tmdb/config
func (c *TmdbController) UpdateAPIKey(ctx *gin.Context) {
	var req struct {
		APIKey   string `json:"api_key" binding:"required"`
		Language string `json:"language"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	// 持久化到数据库
	if err := c.systemConfigDAO.Upsert("tmdb_api_key", req.APIKey); err != nil {
		logger.Errorf("TmdbController[UpdateAPIKey] 保存 tmdb_api_key 到数据库失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "保存配置失败")
		return
	}
	if req.Language != "" {
		if err := c.systemConfigDAO.Upsert("tmdb_language", req.Language); err != nil {
			logger.Warnf("TmdbController[UpdateAPIKey] 保存 tmdb_language 到数据库失败: %v", err)
			// 语言保存失败不阻断主流程
		}
	}

	// 同步更新内存中的服务实例（立即生效，无需重启）
	c.tmdbService.SetAPIKey(req.APIKey)
	if req.Language != "" {
		c.tmdbService.SetLanguage(req.Language)
	}

	logger.Infof("TmdbController[UpdateAPIKey] API Key 已更新并持久化")
	SuccessResp(ctx, gin.H{
		"message": "API Key 更新成功",
	})
}

// GetConfig 获取 TMDB 配置
// GET /api/media/tmdb/config
func (c *TmdbController) GetConfig(ctx *gin.Context) {
	// 优先从数据库读取持久化配置
	dbKey, dbErr := c.systemConfigDAO.GetByKey("tmdb_api_key")
	dbLang, langErr := c.systemConfigDAO.GetByKey("tmdb_language")

	var apiKey, language string
	hasDBKey := false

	if dbErr == nil && dbKey != nil && dbKey.ConfigVal != "" {
		apiKey = dbKey.ConfigVal
		hasDBKey = true
	} else {
		// 回退到内存中的值（来自 config.yaml 或之前的 SetAPIKey 调用）
		apiKey = c.tmdbService.GetAPIKey()
	}

	if langErr == nil && dbLang != nil && dbLang.ConfigVal != "" {
		language = dbLang.ConfigVal
	} else {
		language = c.tmdbService.GetLanguage()
	}

	hasKey := hasDBKey || c.tmdbService.HasUsableAPIKey()

	// 对 API Key 进行脱敏处理（只显示前后各4位）
	maskedKey := ""
	if hasKey && len(apiKey) > 8 {
		maskedKey = apiKey[:4] + "****" + apiKey[len(apiKey)-4:]
	} else if hasKey && len(apiKey) > 0 {
		maskedKey = "****"
	}

	SuccessResp(ctx, gin.H{
		"api_key":  maskedKey,
		"language": language,
		"has_key":  hasKey,
	})
}
