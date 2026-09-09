package service

import (
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"fmt"
	"strconv"
)

// EnsureIdentifyMetadata 补齐识别元数据；已有身份在详情接口失败时仍保留。
func (s *TmdbService) EnsureIdentifyMetadata(result *domain.TmdbIdentifyResult) {
	if err := s.EnrichIdentifyMetadata(result); err != nil {
		logger.Warnf("识别详情补全失败: %v", err)
	}
}

// EnrichIdentifyMetadata 返回详情获取失败原因，供历史补全任务记录和重试。
func (s *TmdbService) EnrichIdentifyMetadata(result *domain.TmdbIdentifyResult) error {
	if result == nil {
		return fmt.Errorf("没有可补全的识别结果")
	}
	if isIdentifyMetadataComplete(result) {
		return nil
	}
	if result.MetadataSource == domain.MetadataSourceMetaTube || result.MetadataProvider != "" {
		detail, err := s.GetMovieDetailBySource(result.TmdbID, result.MetadataSource, result.MetadataID, result.MetadataProvider)
		if err != nil {
			return err
		}
		applyDetailMetadata(result, detail, result.MediaType)
		return nil
	}
	if result.TmdbID <= 0 {
		return fmt.Errorf("缺少可信媒体身份")
	}
	if s.cacheDAO != nil {
		cached, err := s.cacheDAO.GetByTmdbID(result.TmdbID, result.MediaType)
		if err == nil && cached != nil && len(cached.RawData) > 0 {
			applyCachedMetadata(result, cached.RawData)
			if isIdentifyMetadataComplete(result) {
				return nil
			}
		}
	}
	var detail map[string]interface{}
	var err error
	if result.MediaType == "tv" {
		detail, err = s.GetTVDetail(result.TmdbID)
	} else {
		detail, err = s.GetMovieDetail(result.TmdbID)
	}
	if err != nil {
		return err
	}
	applyDetailMetadata(result, detail, result.MediaType)
	return nil
}

func applyDetailMetadata(result *domain.TmdbIdentifyResult, detail map[string]interface{}, mediaType string) {
	if result == nil || detail == nil {
		return
	}
	if rating, ok := detail["vote_average"].(float64); ok && rating >= 0 && rating <= 10 {
		result.VoteAverage = &rating
	}
	if result.Year==0 {
		field:="release_date";if mediaType=="tv"{field="first_air_date"}
		if date,ok:=detail[field].(string);ok && len(date)>=4 {if year,err:=strconv.Atoi(date[:4]);err==nil && year>0{result.Year=year}}
	}
	if poster, ok := detail["poster_path"].(string); ok && poster != "" {
		if len(poster) > 0 && poster[0] == '/' {
			result.PosterPath = "https://image.tmdb.org/t/p/w500" + poster
		} else {
			result.PosterPath = poster
		}
	}
	genreIDs := extractGenreIDsFromDetail(detail)
	countries := extractCountriesFromDetail(detail, mediaType)

	if len(genreIDs) > 0 {
		result.GenreIDs = genreIDs
	}
	if len(countries) > 0 {
		result.Countries = countries
	}

	if lang, ok := detail["original_language"].(string); ok && lang != "" {
		result.Language = lang
	}
	if result.Title == "" {
		if mediaType == "tv" {
			if title, ok := detail["name"].(string); ok && title != "" {
				result.Title = title
			}
		} else if title, ok := detail["title"].(string); ok && title != "" {
			result.Title = title
		}
	}
	if result.OriginalTitle == "" {
		if mediaType == "tv" {
			if title, ok := detail["original_name"].(string); ok && title != "" {
				result.OriginalTitle = title
			}
		} else if title, ok := detail["original_title"].(string); ok && title != "" {
			result.OriginalTitle = title
		}
	}
}

func isIdentifyMetadataComplete(result *domain.TmdbIdentifyResult) bool {
	if result == nil {
		return true
	}
	if len(result.GenreIDs) == 0 || result.Language == "" || result.VoteAverage == nil || len(result.Countries) == 0 {
		return false
	}
	if result.MediaType == "tv" && len(result.Countries) == 0 {
		return false
	}
	if result.Title == "" || result.OriginalTitle == "" || result.Year==0 {
		return false
	}
	return true
}

func extractGenreIDsFromDetail(detail map[string]interface{}) []int {
	genreIDs := make([]int, 0)
	appendGenreID := func(id int) {
		for _, existing := range genreIDs {
			if existing == id {
				return
			}
		}
		genreIDs = append(genreIDs, id)
	}

	if genres, ok := detail["genres"].([]interface{}); ok {
		for _, item := range genres {
			entry, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if id, ok := entry["id"].(float64); ok {
				appendGenreID(int(id))
			}
		}
	}

	if ids, ok := detail["genre_ids"].([]interface{}); ok {
		for _, item := range ids {
			if id, ok := item.(float64); ok {
				appendGenreID(int(id))
			}
		}
	}

	return genreIDs
}

func extractCountriesFromDetail(detail map[string]interface{}, mediaType string) []string {
	countries := make([]string, 0)
	appendCountry := func(code string) {
		for _, existing := range countries {
			if existing == code {
				return
			}
		}
		countries = append(countries, code)
	}

	if mediaType == "movie" {
		if productionCountries, ok := detail["production_countries"].([]interface{}); ok {
			for _, item := range productionCountries {
				entry, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				if code, ok := entry["iso_3166_1"].(string); ok && code != "" {
					appendCountry(code)
				}
			}
		}
	}

	if originCountries, ok := detail["origin_country"].([]interface{}); ok {
		for _, item := range originCountries {
			if code, ok := item.(string); ok && code != "" {
				appendCountry(code)
			}
		}
	}

	return countries
}
