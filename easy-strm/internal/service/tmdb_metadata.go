package service

import (
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// EnsureIdentifyMetadata loads genre/country/language metadata when the current
// identify result is missing it, so category matching can use precise TMDB data.
func (s *TmdbService) EnsureIdentifyMetadata(result *domain.TmdbIdentifyResult) {
	if result == nil || result.TmdbID <= 0 {
		return
	}
	if len(result.GenreIDs) > 0 && result.Language != "" && (result.MediaType != "tv" || len(result.Countries) > 0) {
		return
	}

	if s.cacheDAO != nil {
		cache, err := s.cacheDAO.GetByTmdbID(result.TmdbID, result.MediaType)
		if err == nil && cache != nil && len(cache.RawData) > 0 {
			applyCachedMetadata(result, cache.RawData)
			if len(result.GenreIDs) > 0 && result.Language != "" && (result.MediaType != "tv" || len(result.Countries) > 0) {
				return
			}
		}
	}

	var (
		detail map[string]interface{}
		err    error
	)

	if result.MediaType == "tv" {
		detail, err = s.GetTVDetail(result.TmdbID)
	} else {
		detail, err = s.GetMovieDetail(result.TmdbID)
	}
	if err != nil {
		logger.Warnf("TmdbService[EnsureIdentifyMetadata] fetch detail failed: tmdb_id=%d, err=%v", result.TmdbID, err)
		return
	}

	applyDetailMetadata(result, detail, result.MediaType)
}

func applyDetailMetadata(result *domain.TmdbIdentifyResult, detail map[string]interface{}, mediaType string) {
	if result == nil || detail == nil {
		return
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
