package service

import (
	"context"
	"strings"

	"easy-strm/internal/domain"
)

// IdentifyAssistOptions 定义统一识别流程的调用策略。
// 明确的媒体类型和元数据源优先于文件名规则及AI建议。
type IdentifyAssistOptions struct {
	MediaType      string
	MetadataSource string
	AllowAI        bool
	AIScene        string
	UseCache       bool
	ShareMode      bool
}

// IdentifyWithAssist 先执行常规数据源识别，仅在内容无匹配时调用一次AI并严格复查候选。
// AI只提供标题、原名、年份和类型，不会直接产生任何媒体身份或详情数据。
func (s *TmdbService) IdentifyWithAssist(ctx context.Context, input string, options IdentifyAssistOptions) (*domain.TmdbIdentifyResult, error) {
	ctx = WithRecognitionRound(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if options.ShareMode {
		query := AnalyzeShareFilename(input)
		if query.Container || query.TmdbID > 0 {
			return s.identifyShareWithAssist(ctx, input, options.MetadataSource, options.MediaType)
		}
	}
	mediaType := strings.ToLower(strings.TrimSpace(options.MediaType))
	if mediaType != "movie" && mediaType != "tv" {
		mediaType = ""
	}
	metadataSource := normalizeMetadataSourcePolicy(options.MetadataSource)

	var result *domain.TmdbIdentifyResult
	var err error
	result, err = s.identifyWithoutCache(ctx, input, metadataSource, mediaType)
	if err != nil {
		return nil, err
	}
	if result.Success {
		result.RecognitionMethod = "source"
		return result, nil
	}
	result.RecognitionMethod = "rule"
	result.FailureReason = result.Message
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if !options.AllowAI || s.aiRecognition == nil || !isContentIdentifyFailure(result.Message) {
		return result, nil
	}

	query := AnalyzeShareFilename(input)
	originalYear := query.Year
	if mediaType != "" {
		query.MediaType = mediaType
	}
	result.QueryBeforeAI = append([]string(nil), query.Titles...)
	scenes := identifyAssistScenes(query, options.AIScene)
	var hint *domain.AIRecognitionHint
	for _, scene := range scenes {
		var called bool
		hint, called, err = s.aiRecognition.Assist(ctx, input, scene)
		if !called {
			continue
		}
		result.AIUsed = true
		result.AIScene = scene
		if err != nil {
			result.FailureReason = "AI辅助失败：" + err.Error()
			result.Message = result.FailureReason
			return result, nil
		}
		break
	}
	if !result.AIUsed || hint == nil {
		return result, nil
	}

	query.Titles = nil
	query.Titles = appendImportCandidate(query.Titles, hint.Title)
	query.Titles = appendImportCandidate(query.Titles, hint.OriginalTitle)
	if originalYear == 0 && hint.Year > 0 {
		query.Year = hint.Year
	}
	query.YearFromDirectory = false
	result.AIHint = hint
	result.QueryAfterAI = append([]string(nil), query.Titles...)
	if mediaType == "" && query.MediaType == "unknown" {
		query.MediaType = hint.MediaType
	}
	best, searchErr := s.searchShareQuery(ctx, query, metadataSource)
	if searchErr != nil {
		// 数据源异常和内容无匹配必须区分；异常不伪装成AI核验失败。
		return nil, searchErr
	}
	if best == nil {
		result.Message = "AI建议未通过数据源标题、年份和类型核验"
		result.FailureReason = result.Message
		return result, nil
	}

	season, episode := result.SeasonNumber, result.EpisodeNumber
	result.Success = true
	result.Message = "AI建议经数据源核验通过"
	result.FailureReason = ""
	result.RecognitionMethod = "ai"
	result.MediaType = best.MediaType
	result.TmdbID = best.TmdbID
	result.Title = best.Title
	result.OriginalTitle = best.OriginalTitle
	result.Year = best.Year
	result.PosterPath = best.PosterPath
	result.MetadataSource = best.MetadataSource
	result.MetadataID = best.MetadataID
	result.MetadataProvider = best.MetadataProvider
	result.Candidates = []domain.TmdbSearchResult{*best}
	result.SeasonNumber = season
	result.EpisodeNumber = episode
	if options.UseCache {
		cacheKey := s.buildCacheKeyForSource(input, result.MediaType, metadataSource)
		s.cacheResult(cacheKey, result.MediaType, *best, season, episode)
	}
	return result, nil
}

func (s *TmdbService) identifyWithoutCache(ctx context.Context, input, metadataSource, mediaType string) (*domain.TmdbIdentifyResult, error) {
	parsed := s.parseFilename(input)
	if mediaType != "" {
		parsed = s.parseFilenameForMediaType(input, mediaType)
	}
	result := &domain.TmdbIdentifyResult{
		Filename: input, MediaType: parsed.MediaType, Quality: parsed.Quality, Source: parsed.Source,
		Codec: parsed.Codec, SeasonNumber: parsed.Season, EpisodeNumber: parsed.Episode,
	}
	if parsed.Title == "" {
		result.Message = "无法从文件名中解析出标题"
		return result, nil
	}
	query := AnalyzeShareFilename(input)
	query.Titles = appendImportCandidate(query.Titles, parsed.Title)
	if mediaType != "" {
		query.MediaType = mediaType
	} else if parsed.MediaType == "tv" {
		query.MediaType = "tv"
	}
	best, err := s.searchShareQuery(ctx, query, metadataSource)
	if err != nil {
		result.Message = err.Error()
		return result, nil
	}
	if best == nil {
		result.Message = "未找到匹配的媒体信息"
		return result, nil
	}
	result.Success = true
	result.MediaType = best.MediaType
	result.TmdbID, result.Title, result.OriginalTitle, result.Year = best.TmdbID, best.Title, best.OriginalTitle, best.Year
	result.PosterPath, result.Candidates = best.PosterPath, []domain.TmdbSearchResult{*best}
	result.MetadataSource, result.MetadataID, result.MetadataProvider = best.MetadataSource, best.MetadataID, best.MetadataProvider
	return result, nil
}

func identifyAssistScenes(query ShareMediaQuery, explicit string) []string {
	if explicit != "" {
		return []string{explicit}
	}
	result := make([]string, 0, 3)
	if query.Complex {
		result = append(result, "complex_title")
	}
	if query.MediaType == "unknown" {
		result = append(result, "uncertain_type")
	}
	return append(result, "no_match")
}

func isContentIdentifyFailure(message string) bool {
	return strings.Contains(message, "未找到匹配") || strings.Contains(message, "无法从文件名中解析")
}
