package service

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrLibraryNotTV 表示剧集详情接口收到非电视剧作品。
var ErrLibraryNotTV = errors.New("该作品不是电视剧")

// ValidateLibraryQuery 规范分页并检查搜索范围，避免错误筛选悄悄失效。
func ValidateLibraryQuery(q *domain.ShareLibraryQuery) error {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 24
	}
	if q.Page < 1 || q.Page > 1000000 || q.PageSize < 1 || q.PageSize > 100 || q.TmdbID < 0 {
		return fmt.Errorf("分页或TMDB ID无效")
	}
	if q.MediaType != "" && q.MediaType != "movie" && q.MediaType != "tv" {
		return fmt.Errorf("媒体分类无效")
	}
	if q.YearMin < 0 || q.YearMax < 0 || q.YearMin > 9999 || q.YearMax > 9999 || (q.YearMax > 0 && q.YearMin > q.YearMax) {
		return fmt.Errorf("年份范围无效")
	}
	for _, v := range []*float64{q.RatingMin, q.RatingMax} {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 || *v > 10) {
			return fmt.Errorf("评分应在0至10之间")
		}
	}
	if q.RatingMin != nil && q.RatingMax != nil && *q.RatingMin > *q.RatingMax {
		return fmt.Errorf("评分范围无效")
	}
	if q.Genres != "" {
		for _, v := range strings.Split(q.Genres, ",") {
			n, e := strconv.Atoi(v)
			if e != nil || n <= 0 || n > 2147483647 {
				return fmt.Errorf("题材类型无效")
			}
		}
	}
	if q.Countries != "" {
		for _, v := range strings.Split(q.Countries, ",") {
			if len(v) != 2 || strings.ToUpper(v)[0] < 'A' || strings.ToUpper(v)[0] > 'Z' || strings.ToUpper(v)[1] < 'A' || strings.ToUpper(v)[1] > 'Z' {
				return fmt.Errorf("国家代码无效")
			}
		}
	}
	if q.Sort != "" && q.Sort != "created" && q.Sort != "year" && q.Sort != "rating" && q.Sort != "title" {
		return fmt.Errorf("排序字段无效")
	}
	if q.Direction != "" && q.Direction != "asc" && q.Direction != "desc" {
		return fmt.Errorf("排序方向无效")
	}
	return nil
}

// Library 返回资源库作品分页。
func (s *ShareRecordService) Library(ctx context.Context, q domain.ShareLibraryQuery) (domain.ShareLibraryPage, error) {
	return s.dao.Library(ctx, q)
}

// LibrarySources 返回作品的分享文件来源。
func (s *ShareRecordService) LibrarySources(ctx context.Context, key string, page, size int) (domain.ShareLibraryPage, error) {
	return s.dao.LibrarySources(ctx, key, page, size)
}

// LibraryTVDetail 返回剧集的完整季目录，并合并本地文件覆盖统计。
func (s *ShareRecordService) LibraryTVDetail(ctx context.Context, key string) (domain.ShareLibraryTVDetail, error) {
	media, err := s.libraryTVMedia(ctx, key)
	if err != nil {
		return domain.ShareLibraryTVDetail{}, err
	}
	stats, err := s.dao.LibraryTVSeasonStats(ctx, key)
	if err != nil {
		return domain.ShareLibraryTVDetail{}, err
	}
	out := domain.ShareLibraryTVDetail{WorkKey: key, TmdbID: media.TmdbID, Title: media.Title, Seasons: make([]domain.ShareLibraryTVSeasonSummary, 0)}
	bySeason := make(map[int]domain.ShareLibraryTVSeasonSummary, len(stats))
	for _, item := range stats {
		item.Name = librarySeasonName(item.SeasonNumber)
		item.EpisodeCount = item.MatchedEpisodeCount
		bySeason[item.SeasonNumber] = item
	}

	if media.TmdbID <= 0 || media.TmdbID > math.MaxInt32 || s.tmdb == nil {
		out.Warning = "缺少可用的 TMDB 身份，当前仅展示已关联的季集"
	} else {
		var payload struct {
			Name     string `json:"name"`
			Overview string `json:"overview"`
			Seasons  []struct {
				SeasonNumber int    `json:"season_number"`
				Name         string `json:"name"`
				Overview     string `json:"overview"`
				AirDate      string `json:"air_date"`
				PosterPath   string `json:"poster_path"`
				EpisodeCount int    `json:"episode_count"`
			} `json:"seasons"`
		}
		detail, detailErr := s.tmdb.GetTVDetail(int(media.TmdbID))
		if detailErr != nil {
			out.Warning = "TMDB 季目录加载失败，当前仅展示已关联的季集：" + detailErr.Error()
		} else if err = remarshalLibraryMetadata(detail, &payload); err != nil {
			out.Warning = "TMDB 季目录格式无效，当前仅展示已关联的季集"
		} else {
			out.MetadataComplete = true
			if payload.Name != "" {
				out.Title = payload.Name
			}
			out.Overview = payload.Overview
			for _, season := range payload.Seasons {
				item := bySeason[season.SeasonNumber]
				item.SeasonNumber = season.SeasonNumber
				item.Name = season.Name
				if item.Name == "" {
					item.Name = librarySeasonName(season.SeasonNumber)
				}
				item.Overview = season.Overview
				item.AirDate = season.AirDate
				item.PosterPath = season.PosterPath
				item.EpisodeCount = season.EpisodeCount
				bySeason[season.SeasonNumber] = item
			}
		}
	}
	for _, item := range bySeason {
		out.Seasons = append(out.Seasons, item)
	}
	sort.Slice(out.Seasons, func(i, j int) bool { return out.Seasons[i].SeasonNumber < out.Seasons[j].SeasonNumber })
	return out, nil
}

// LibraryTVSeason 返回单季完整集目录，并把分享文件挂载到对应集下。
func (s *ShareRecordService) LibraryTVSeason(ctx context.Context, key string, season int) (domain.ShareLibraryTVSeasonDetail, error) {
	media, err := s.libraryTVMedia(ctx, key)
	if err != nil {
		return domain.ShareLibraryTVSeasonDetail{}, err
	}
	files, err := s.dao.LibraryTVSeasonFiles(ctx, key, season)
	if err != nil {
		return domain.ShareLibraryTVSeasonDetail{}, err
	}
	out := domain.ShareLibraryTVSeasonDetail{SeasonNumber: season, Name: librarySeasonName(season), Episodes: make([]domain.ShareLibraryEpisode, 0)}
	byEpisode := make(map[int]domain.ShareLibraryEpisode, len(files))
	for episode, mappedFiles := range files {
		byEpisode[episode] = domain.ShareLibraryEpisode{EpisodeNumber: episode, Name: fmt.Sprintf("第 %d 集", episode), Files: mappedFiles}
	}

	if media.TmdbID <= 0 || media.TmdbID > math.MaxInt32 || s.tmdb == nil {
		out.Warning = "缺少可用的 TMDB 身份，当前仅展示已关联的集"
	} else {
		var payload struct {
			Name       string `json:"name"`
			Overview   string `json:"overview"`
			AirDate    string `json:"air_date"`
			PosterPath string `json:"poster_path"`
			Episodes   []struct {
				EpisodeNumber int    `json:"episode_number"`
				Name          string `json:"name"`
				Overview      string `json:"overview"`
				AirDate       string `json:"air_date"`
				StillPath     string `json:"still_path"`
				Runtime       int    `json:"runtime"`
			} `json:"episodes"`
		}
		detail, detailErr := s.tmdb.GetTVSeasonDetail(int(media.TmdbID), season)
		if detailErr != nil {
			out.Warning = "TMDB 分集目录加载失败，当前仅展示已关联的集：" + detailErr.Error()
		} else if err = remarshalLibraryMetadata(detail, &payload); err != nil {
			out.Warning = "TMDB 分集目录格式无效，当前仅展示已关联的集"
		} else {
			out.MetadataComplete = true
			if payload.Name != "" {
				out.Name = payload.Name
			}
			out.Overview, out.AirDate, out.PosterPath = payload.Overview, payload.AirDate, payload.PosterPath
			for _, episode := range payload.Episodes {
				item := byEpisode[episode.EpisodeNumber]
				item.EpisodeNumber = episode.EpisodeNumber
				item.Name = episode.Name
				if item.Name == "" {
					item.Name = fmt.Sprintf("第 %d 集", episode.EpisodeNumber)
				}
				item.Overview, item.AirDate, item.StillPath, item.Runtime = episode.Overview, episode.AirDate, episode.StillPath, episode.Runtime
				if item.Files == nil {
					item.Files = make([]domain.ShareLibraryFile, 0)
				}
				byEpisode[episode.EpisodeNumber] = item
			}
		}
	}
	for _, item := range byEpisode {
		if item.Files == nil {
			item.Files = make([]domain.ShareLibraryFile, 0)
		}
		out.Episodes = append(out.Episodes, item)
	}
	sort.Slice(out.Episodes, func(i, j int) bool { return out.Episodes[i].EpisodeNumber < out.Episodes[j].EpisodeNumber })
	return out, nil
}

func (s *ShareRecordService) libraryTVMedia(ctx context.Context, key string) (domain.ShareLibraryTVMedia, error) {
	media, err := s.dao.LibraryTVMedia(ctx, key)
	if err != nil {
		return media, err
	}
	if media.MediaType != "tv" {
		return media, ErrLibraryNotTV
	}
	return media, nil
}

func librarySeasonName(season int) string {
	if season == 0 {
		return "特别篇"
	}
	return fmt.Sprintf("第 %d 季", season)
}

func remarshalLibraryMetadata(value map[string]interface{}, target interface{}) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

// LibraryOptions 返回可用筛选项。
func (s *ShareRecordService) LibraryOptions(ctx context.Context) (json.RawMessage, error) {
	return s.dao.LibraryOptions(ctx)
}

// StartLibraryEnrichment 按作品补齐历史元数据；每次执行只重试仍缺失的项目。
func (s *ShareRecordService) StartLibraryEnrichment() (string, error) {
	if s.tasks == nil || s.tmdb == nil {
		return "", fmt.Errorf("任务或元数据服务未初始化")
	}
	if !s.enrichMu.TryLock() {
		return "", fmt.Errorf("历史补全任务正在执行，请在任务中心查看")
	}
	id := fmt.Sprintf("share_metadata_%d", time.Now().UnixNano())
	if err := s.tasks.Create(id, "share_identify", "补全历史元数据"); err != nil {
		s.enrichMu.Unlock()
		return "", err
	}
	go func() {
		defer s.enrichMu.Unlock()
		defer func() {
			if v := recover(); v != nil {
				_ = s.tasks.SetError(id, fmt.Sprint(v))
			}
		}()
		_ = s.tasks.UpdateStatus(id, "running")
		total, err := s.dao.CountLibraryIncomplete(context.Background())
		if err != nil {
			_ = s.tasks.SetError(id, err.Error())
			return
		}
		after := ""
		processed, success, failed := 0, 0, 0
		errors := []string{}
		for {
			if s.tasks.IsCancelled(id) {
				_ = s.tasks.UpdateStatus(id, "cancelled")
				return
			}
			key, media, err := s.dao.LibraryIncomplete(context.Background(), after)
			if err == sql.ErrNoRows {
				break
			}
			if err != nil {
				_ = s.tasks.SetError(id, err.Error())
				return
			}
			after = key
			if len(media) == 0 {
				continue
			}
			r := *media[0].Result
			detailErr := s.tmdb.EnrichIdentifyMetadata(&r)
			for _, m := range media {
				if s.tasks.IsCancelled(id) {
					_ = s.tasks.UpdateStatus(id, "cancelled")
					return
				}
				processed++
				mergeLibraryMetadata(m.Result, &r)
				err = detailErr
				if err == nil {
					err = s.dao.UpdateMediaMetadata(context.Background(), m)
				}
				if err == nil && (m.Result.VoteAverage == nil || len(m.Result.GenreIDs) == 0 || len(m.Result.Countries) == 0 || m.Result.Year == 0 || m.Result.Title == "") {
					err = fmt.Errorf("元数据源未提供完整年份、标题、评分、题材或国家信息")
				}
				if err != nil {
					failed++
					if len(errors) < 100 {
						errors = append(errors, fmt.Sprintf("媒体%d: %v", m.ID, err))
					}
				} else {
					success++
				}
			}
			_ = s.tasks.UpdateProgress(id, max(total, processed), processed, success, failed)
			_ = s.tasks.UpdateMetadata(id, map[string]interface{}{"errors": errors, "last_work_key": key})
		}
		if failed > 0 {
			_ = s.tasks.SetError(id, fmt.Sprintf("补全结束：成功%d，失败%d，可重新执行重试缺失项", success, failed))
		} else {
			_ = s.tasks.UpdateStatus(id, "completed")
		}
	}()
	return id, nil
}

// mergeLibraryMetadata 只补齐缺失信息，不以空详情抹除已有字段。
func mergeLibraryMetadata(dst, src *domain.TmdbIdentifyResult) {
	if dst.Year == 0 {
		dst.Year = src.Year
	}
	if dst.Title == "" {
		dst.Title = src.Title
	}
	if dst.VoteAverage == nil && src.VoteAverage != nil {
		v := *src.VoteAverage
		dst.VoteAverage = &v
	}
	if len(dst.GenreIDs) == 0 && len(src.GenreIDs) > 0 {
		dst.GenreIDs = append([]int{}, src.GenreIDs...)
	}
	if len(dst.Countries) == 0 && len(src.Countries) > 0 {
		dst.Countries = append([]string{}, src.Countries...)
	}
}
