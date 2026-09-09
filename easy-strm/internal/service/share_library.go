package service

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

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
		total,err:=s.dao.CountLibraryIncomplete(context.Background())
		if err!=nil{_=s.tasks.SetError(id,err.Error());return}
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
					err = s.dao.Identify(context.Background(), m, "identified", m.Result, "")
				}
				if err == nil && (m.Result.VoteAverage == nil || len(m.Result.GenreIDs) == 0 || len(m.Result.Countries) == 0 || m.Result.Year==0 || m.Result.Title=="") {
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
			_ = s.tasks.UpdateProgress(id, max(total,processed), processed, success, failed)
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
 if dst.Year==0{dst.Year=src.Year};if dst.Title==""{dst.Title=src.Title}
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
