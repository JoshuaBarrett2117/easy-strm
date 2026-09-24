package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// getImageURL 获取完整图片 URL
// 参数:
//   - path: 图片路径
//
// 返回:
//   - string: 完整 URL
func (s *TmdbService) getImageURL(path string) string {
	if path == "" {
		return ""
	}
	return s.imageBaseURL + path
}

// GetMovieDetail 获取电影详情
// 参数:
//   - tmdbID: TMDB ID
//
// 返回:
//   - map[string]interface{}: 电影详情
//   - error: 错误信息
func (s *TmdbService) GetMovieDetail(tmdbID int) (map[string]interface{}, error) {
	return s.getMovieDetailContext(context.Background(), tmdbID)
}

func (s *TmdbService) getMovieDetailContext(ctx context.Context, tmdbID int) (map[string]interface{}, error) {
	if s.MetaTubeEnabled() {
		if ref, ok := s.metatubeRefs.Load(tmdbID); ok {
			return s.metaTubeDetail(ref.(metaTubeRef))
		}
	}
	if s.apiKey == "" {
		return nil, fmt.Errorf("TMDB API Key 未配置")
	}
	if detail, ok := s.loadDetailCache("movie", tmdbID, 0, 0); ok && !bypassShareRecognitionCache(ctx) {
		observeShareMetric(ctx, "tmdb_detail_cache_hit", 0)
		return detail, nil
	}

	apiURL := fmt.Sprintf("%s/movie/%d?api_key=%s&language=%s",
		s.baseURL, tmdbID, s.apiKey, s.language)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.doShareTMDBRequest(req)
	if err != nil {
		return nil, fmt.Errorf("TMDB API 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API 返回错误: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 TMDB 响应失败: %v", err)
	}

	s.saveDetailCache("movie", tmdbID, 0, 0, result)
	return result, nil
}

// GetTVDetail 获取剧集详情
// 参数:
//   - tmdbID: TMDB ID
//
// 返回:
//   - map[string]interface{}: 剧集详情
//   - error: 错误信息
func (s *TmdbService) GetTVDetail(tmdbID int) (map[string]interface{}, error) {
	return s.getTVDetailContext(context.Background(), tmdbID)
}

func (s *TmdbService) getTVDetailContext(ctx context.Context, tmdbID int) (map[string]interface{}, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("TMDB API Key 未配置")
	}
	if detail, ok := s.loadDetailCache("tv", tmdbID, 0, 0); ok && !bypassShareRecognitionCache(ctx) {
		observeShareMetric(ctx, "tmdb_detail_cache_hit", 0)
		return detail, nil
	}

	apiURL := fmt.Sprintf("%s/tv/%d?api_key=%s&language=%s",
		s.baseURL, tmdbID, s.apiKey, s.language)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.doShareTMDBRequest(req)
	if err != nil {
		return nil, fmt.Errorf("TMDB API 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API 返回错误: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 TMDB 响应失败: %v", err)
	}

	s.saveDetailCache("tv", tmdbID, 0, 0, result)
	return result, nil
}

// GetTVSeasonDetail 获取剧集单季详情，响应包含该季完整集目录。
func (s *TmdbService) GetTVSeasonDetail(tmdbID, season int) (map[string]interface{}, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("TMDB API Key 未配置")
	}
	if detail, ok := s.loadDetailCache("tv_season", tmdbID, season, 0); ok {
		return detail, nil
	}

	apiURL := fmt.Sprintf("%s/tv/%d/season/%d?api_key=%s&language=%s",
		s.baseURL, tmdbID, season, s.apiKey, s.language)
	resp, err := s.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("TMDB API 请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API 返回错误: %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 TMDB 响应失败: %v", err)
	}
	s.saveDetailCache("tv_season", tmdbID, season, 0, result)
	return result, nil
}

// GetTVEpisodeDetail 获取剧集某一集的详情
func (s *TmdbService) GetTVEpisodeDetail(tmdbID, season, episode int) (map[string]interface{}, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("TMDB API Key 未配置")
	}
	if detail, ok := s.loadDetailCache("tv_episode", tmdbID, season, episode); ok {
		return detail, nil
	}

	apiURL := fmt.Sprintf("%s/tv/%d/season/%d/episode/%d?api_key=%s&language=%s",
		s.baseURL, tmdbID, season, episode, s.apiKey, s.language)

	resp, err := s.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("TMDB API 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API 返回错误: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 TMDB 响应失败: %v", err)
	}

	s.saveDetailCache("tv_episode", tmdbID, season, episode, result)
	return result, nil
}
