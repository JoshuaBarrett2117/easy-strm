package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

type shareWorkScopeKey struct{}

func (s *TmdbService) invalidateShareWork(ctx context.Context, media domain.ShareMedia) {
	ctx = context.WithValue(ctx, shareWorkScopeKey{}, media.ShareID)
	keys := make([]string, 0, 9)
	for _, source := range []string{"auto", "tmdb", "metatube"} {
		for _, kind := range []string{"auto", "tv", "movie"} {
			input := shareEpisodeInput(ctx, media.FileName, kind)
			keys = append(keys, s.shareWorkKey(ctx, input, source, kind))
		}
	}
	if err := (dao.ShareWorkCacheDAO{}).Delete(ctx, keys...); err != nil {
		logger.Warnf("手动修正后的作品缓存失效失败 | file=%q | error=%v", media.FileName, err)
	}
}

func bypassShareRecognitionCache(ctx context.Context) bool {
	round, ok := ctx.Value(recognitionRoundKey{}).(*recognitionRound)
	return ok && round.bypassWorkCache
}

func withShareWorkRound(ctx context.Context, forceRefresh bool) context.Context {
	if _, ok := ctx.Value(recognitionRoundKey{}).(*recognitionRound); ok {
		return ctx
	}
	ctx = WithRecognitionRound(ctx)
	ctx.Value(recognitionRoundKey{}).(*recognitionRound).bypassWorkCache = forceRefresh
	return ctx
}

type shareWorkAttempt struct {
	done  chan struct{}
	value *dao.ShareWorkIdentity
	err   error
}

// analyzeShareQuery 使用生效的通用季集规则；原盘和无集号目录继续使用分享标题清洗。
func (s *TmdbService) analyzeShareQuery(filename, forcedType string) ShareMediaQuery {
	q := AnalyzeShareFilename(filename)
	if forcedType == "movie" {
		name := path.Base(strings.ReplaceAll(filename, "\\", "/"))
		q.Titles, q.Year = cleanShareTitles(strings.TrimSuffix(name, path.Ext(name)))
	}
	parsed := s.parseFilenameForMediaType(filename, forcedType)
	if parsed.MediaType == "tv" && parsed.Episode > 0 && parsed.Title != "" {
		q.MediaType = "tv"
		q.Titles, q.Year = cleanShareTitles(parsed.Title)
		if q.Year == 0 {
			q.Year = parsed.Year
		}
	}
	if forcedType == "tv" || forcedType == "movie" {
		q.MediaType = forcedType
	}
	return q
}

func (s *TmdbService) shareWorkKey(ctx context.Context, filename, source, forcedType string) string {
	q := s.analyzeShareQuery(filename, forcedType)
	parent := path.Dir(strings.ReplaceAll(filename, "\\", "/"))
	// 季目录不构成作品边界，其他目录上下文仍保留，避免合集内同名作品串号。
	if shareSeasonRE.ReplaceAllString(path.Base(parent), "") == "" {
		parent = path.Dir(parent)
	}
	rules, _ := json.Marshal(s.GetFilenameRecognitionRules().Rules)
	payload, _ := json.Marshal([]interface{}{"share-work-v1", ctx.Value(shareWorkScopeKey{}), parent, q.Titles, q.Year, q.MediaType, q.TmdbID, normalizeMetadataSourcePolicy(source), s.baseURL, s.language, fmt.Sprintf("%x", sha256.Sum256(rules)), s.adultContentEnabled, s.metatubeDefaultEnabled, s.metatubeURL})
	return string(payload)
}

func (s *TmdbService) identifyShareWithAssist(ctx context.Context, filename, source, forcedType string) (*domain.TmdbIdentifyResult, error) {
	round := ctx.Value(recognitionRoundKey{}).(*recognitionRound)
	key := s.shareWorkKey(ctx, filename, source, forcedType)
	round.mu.Lock()
	attempt := round.works[key]
	reused := attempt != nil
	if attempt == nil {
		attempt = &shareWorkAttempt{done: make(chan struct{})}
		round.works[key] = attempt
	}
	round.mu.Unlock()
	if reused {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-attempt.done:
		}
	} else {
		// 无论取消或上层恢复异常，等待相同作品的协程均可退出。
		func() {
			defer close(attempt.done)
			attempt.value, attempt.err = s.resolveShareWork(ctx, filename, source, forcedType, key, round.bypassWorkCache)
			if attempt.err != nil {
				round.mu.Lock()
				delete(round.works, key)
				round.mu.Unlock()
			}
		}()
	}
	if attempt.err != nil {
		return nil, attempt.err
	}
	if attempt.value == nil || attempt.value.Result == nil {
		return nil, fmt.Errorf("作品识别未返回结果")
	}
	result := cloneShareWorkResult(attempt.value.Result)
	result.Filename = filename
	parsed := s.parseFilenameForMediaType(filename, forcedType)
	result.SeasonNumber, result.EpisodeNumber = parsed.Season, parsed.Episode
	result.Quality, result.Source, result.Codec = parsed.Quality, parsed.Source, parsed.Codec
	if result.MediaType == "tv" && parsed.Episode > 0 {
		found := false
		for _, season := range attempt.value.Seasons {
			found = found || season == parsed.Season
		}
		if !found && result.Success {
			result.Message += fmt.Sprintf("；作品已识别，季信息待核对（S%02d）", parsed.Season)
		}
	}
	if reused {
		logger.Infof("[ShareIdentify] 作品身份复用 | file=%q | title=%q | media_type=%s | tmdb_id=%d | season=%d | episode=%d | success=%v", filename, result.Title, result.MediaType, result.TmdbID, result.SeasonNumber, result.EpisodeNumber, result.Success)
	}
	return result, nil
}

func (s *TmdbService) resolveShareWork(ctx context.Context, filename, source, forcedType, key string, bypass bool) (*dao.ShareWorkIdentity, error) {
	cache := dao.ShareWorkCacheDAO{}
	if !bypass {
		round := ctx.Value(recognitionRoundKey{}).(*recognitionRound)
		round.mu.Lock()
		seed := round.seeds[key]
		conflict := round.workConflicts[key]
		round.mu.Unlock()
		if conflict {
			return &dao.ShareWorkIdentity{Result: &domain.TmdbIdentifyResult{Filename: filename, MediaType: s.analyzeShareQuery(filename, forcedType).MediaType, Message: "同作品存在不同的已确认身份，请手动核对", FailureReason: "历史作品身份冲突"}}, nil
		}
		if seed != nil {
			value := &dao.ShareWorkIdentity{Result: cloneShareWorkResult(seed)}
			s.enrichShareWork(ctx, value)
			if err := cache.Save(ctx, key, value); err != nil {
				logger.Warnf("作品缓存保存失败: %v", err)
			}
			logger.Infof("[ShareIdentify] 复用已保存作品身份 | file=%q | tmdb_id=%d", filename, value.Result.TmdbID)
			return value, nil
		}
		value, err := cache.Get(ctx, key)
		if err != nil {
			logger.Warnf("[ShareIdentify] 作品缓存读取失败 | file=%q | error=%v", filename, err)
		}
		if value != nil {
			logger.Infof("[ShareIdentify] 作品身份缓存命中 | file=%q | tmdb_id=%d", filename, value.Result.TmdbID)
			if !isIdentifyMetadataComplete(value.Result) || value.Result.MediaType == "tv" && !value.SeasonsKnown {
				s.enrichShareWork(ctx, value)
			}
			if err := cache.Save(ctx, key, value); err != nil {
				logger.Warnf("作品缓存保存失败: %v", err)
			}
			return value, nil
		}
	}
	q := s.analyzeShareQuery(filename, forcedType)
	logger.Infof("[ShareIdentify] 首次识别作品 | file=%q | titles=%q | media_type=%s | reason=作品身份未确认", filename, q.Titles, q.MediaType)
	result, err := s.identifyShareWithAssistUncached(ctx, filename, source, forcedType)
	if err != nil {
		return nil, err
	}
	value := &dao.ShareWorkIdentity{Result: result}
	if result.Success {
		s.enrichShareWork(ctx, value)
		if err := cache.Save(ctx, key, value); err != nil {
			logger.Warnf("[ShareIdentify] 作品缓存保存失败 | file=%q | error=%v", filename, err)
		}
	}
	return value, nil
}

// seedShareWorks 只复用与当前解析标题、年份、类型一致的已确认身份；冲突组重新核验。
func (s *TmdbService) seedShareWorks(ctx context.Context, records []domain.ShareRecord) {
	round := ctx.Value(recognitionRoundKey{}).(*recognitionRound)
	if round.bypassWorkCache {
		return
	}
	round.mu.Lock()
	defer round.mu.Unlock()
	conflicts := map[string]bool{}
	for _, record := range records {
		for _, media := range record.Media {
			r := media.Result
			if !media.Available || media.Status != "identified" || r == nil || !r.Success || r.TmdbID <= 0 || r.MediaType != "tv" {
				continue
			}
			scoped := context.WithValue(ctx, shareWorkScopeKey{}, record.ID)
			input := shareEpisodeInput(scoped, media.FileName, record.MediaType)
			q := s.analyzeShareQuery(input, record.MediaType)
			candidate := domain.TmdbSearchResult{TmdbID: r.TmdbID, MediaType: r.MediaType, Title: r.Title, OriginalTitle: r.OriginalTitle, Year: r.Year}
			if q.MediaType != "tv" || selectVerifiedShareCandidate(q, []domain.TmdbSearchResult{candidate}) == nil {
				continue
			}
			key := s.shareWorkKey(scoped, input, media.MetadataSource, record.MediaType)
			if conflicts[key] {
				continue
			}
			if old := round.seeds[key]; old != nil && old.TmdbID != r.TmdbID {
				delete(round.seeds, key)
				conflicts[key] = true
				round.workConflicts[key] = true
				continue
			}
			seed := cloneShareWorkResult(r)
			seed.SeasonNumber, seed.EpisodeNumber = 0, 0
			seed.Message = strings.Split(seed.Message, "；作品已识别，季信息待核对")[0]
			round.seeds[key] = seed
		}
	}
}

func (s *TmdbService) enrichShareWork(ctx context.Context, value *dao.ShareWorkIdentity) {
	r := value.Result
	if r.MediaType != "tv" {
		s.EnsureIdentifyMetadata(r)
		return
	}
	logger.Infof("[ShareIdentify] 作品详情补全 | media_type=tv | tmdb_id=%d | reason=补全元数据与季目录", r.TmdbID)
	detail, err := s.shareTVDetail(ctx, r.TmdbID)
	if err != nil {
		logger.Warnf("作品详情补全失败 | tmdb_id=%d | error=%v", r.TmdbID, err)
		return
	}
	applyDetailMetadata(r, detail, "tv")
	if seasons, ok := detail["seasons"].([]interface{}); ok {
		value.SeasonsKnown = true
		value.Seasons = nil
		for _, raw := range seasons {
			if item, ok := raw.(map[string]interface{}); ok {
				if number, ok := item["season_number"].(float64); ok {
					value.Seasons = append(value.Seasons, int(number))
				}
			}
		}
	}
}

func cloneShareWorkResult(result *domain.TmdbIdentifyResult) *domain.TmdbIdentifyResult {
	copied := *result
	copied.GenreIDs = append([]int(nil), result.GenreIDs...)
	copied.Countries = append([]string(nil), result.Countries...)
	copied.QueryBeforeAI = append([]string(nil), result.QueryBeforeAI...)
	copied.QueryAfterAI = append([]string(nil), result.QueryAfterAI...)
	copied.AIHint = cloneRecognitionHint(result.AIHint)
	if result.VoteAverage != nil {
		rating := *result.VoteAverage
		copied.VoteAverage = &rating
	}
	copied.Candidates = append([]domain.TmdbSearchResult(nil), result.Candidates...)
	for i := range copied.Candidates {
		copied.Candidates[i].GenreIDs = append([]int(nil), result.Candidates[i].GenreIDs...)
		copied.Candidates[i].Countries = append([]string(nil), result.Candidates[i].Countries...)
	}
	return &copied
}

type shareDetailAttempt struct {
	done   chan struct{}
	detail map[string]interface{}
	err    error
}

// shareTVDetail 在一轮任务内共享只读详情，包括文件显式ID核验使用的同一份响应。
func (s *TmdbService) shareTVDetail(ctx context.Context, id int) (map[string]interface{}, error) {
	round := ctx.Value(recognitionRoundKey{}).(*recognitionRound)
	key := fmt.Sprintf("%s:%s:tv:%d", s.baseURL, s.language, id)
	round.mu.Lock()
	attempt := round.details[key]
	reused := attempt != nil
	if attempt == nil {
		attempt = &shareDetailAttempt{done: make(chan struct{})}
		round.details[key] = attempt
	}
	round.mu.Unlock()
	if reused {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-attempt.done:
		}
	} else {
		func() { defer close(attempt.done); attempt.detail, attempt.err = s.getTVDetailContext(ctx, id) }()
	}
	return attempt.detail, attempt.err
}
