package service

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"easy-strm/internal/domain"
)

type metaTubeRef struct {
	Provider string
	ID       string
}

// searchMetaTube 使用 MetaTube 官方电影搜索接口，并适配为现有识别候选结构。
func (s *TmdbService) searchMetaTube(query string, year int, _ string) ([]domain.TmdbSearchResult, error) {
	endpoint := s.metatubeURL + "/v1/movies/search?q=" + url.QueryEscape(query) + "&fallback=true"
	var payload struct {
		Data []struct {
			ID          string  `json:"id"`
			Provider    string  `json:"provider"`
			Number      string  `json:"number"`
			Title       string  `json:"title"`
			ReleaseDate string  `json:"release_date"`
			CoverURL    string  `json:"cover_url"`
			BigCoverURL string  `json:"big_cover_url"`
			ThumbURL    string  `json:"thumb_url"`
			BigThumbURL string  `json:"big_thumb_url"`
			Score       float64 `json:"score"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := s.metaTubeJSON(endpoint, &payload); err != nil {
		return nil, fmt.Errorf("MetaTube 搜索失败: %v", err)
	}
	if payload.Error != nil {
		return nil, fmt.Errorf("MetaTube 搜索失败: %s", payload.Error.Message)
	}
	results := make([]domain.TmdbSearchResult, 0, 3)
	for _, item := range payload.Data {
		if len(results) >= 3 {
			break
		}
		itemYear := yearFromDate(item.ReleaseDate)
		if year > 0 && itemYear > 0 && itemYear != year {
			continue
		}
		syntheticID := metaTubeSyntheticID(item.Provider, item.ID)
		s.metatubeRefs.Store(syntheticID, metaTubeRef{Provider: item.Provider, ID: item.ID})
		results = append(results, domain.TmdbSearchResult{
			TmdbID: syntheticID, Title: item.Title, OriginalTitle: item.Number, Year: itemYear,
			PosterPath: metaTubeImageURL(s.metatubeURL, item.BigCoverURL, item.CoverURL, item.BigThumbURL, item.ThumbURL), VoteAverage: item.Score, MediaType: "movie", ReleaseDate: item.ReleaseDate,
			MetadataSource: "metatube", MetadataID: item.ID, MetadataProvider: item.Provider,
		})
	}
	return results, nil
}

// metaTubeDetail 获取 MetaTube 影片详情，并转换成现有 NFO 生成器可消费的 TMDB 形状。
func (s *TmdbService) metaTubeDetail(ref metaTubeRef) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s/v1/movies/%s/%s?lazy=true", s.metatubeURL, url.PathEscape(ref.Provider), url.PathEscape(ref.ID))
	var payload struct {
		Data  map[string]interface{} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := s.metaTubeJSON(endpoint, &payload); err != nil {
		return nil, fmt.Errorf("MetaTube 详情获取失败: %v", err)
	}
	if payload.Error != nil {
		return nil, fmt.Errorf("MetaTube 详情获取失败: %s", payload.Error.Message)
	}
	if payload.Data == nil {
		return nil, fmt.Errorf("MetaTube 详情为空")
	}
	detail := normalizeMetaTubeDetail(payload.Data, metaTubeSyntheticID(ref.Provider, ref.ID))
	detail["poster_path"] = metaTubeImageURL(s.metatubeURL, stringValue(detail["poster_path"]))
	detail["backdrop_path"] = metaTubeImageURL(s.metatubeURL, stringValue(detail["backdrop_path"]))
	return detail, nil
}

func (s *TmdbService) metaTubeJSON(endpoint string, target interface{}) error {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if s.metatubeToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.metatubeToken)
		req.Header.Set("X-API-Key", s.metatubeToken)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func normalizeMetaTubeDetail(source map[string]interface{}, syntheticID int) map[string]interface{} {
	result := map[string]interface{}{
		"id": syntheticID, "title": stringValue(source["title"]), "original_title": stringValue(source["number"]),
		"overview": stringValue(source["summary"]), "release_date": stringValue(source["release_date"]),
		"poster_path": firstString(source["big_cover_url"], source["cover_url"]), "backdrop_path": firstString(source["big_thumb_url"], source["thumb_url"]),
		"runtime": intValue(source["runtime"]), "vote_average": floatValue(source["score"]),
	}
	result["genres"] = namedItems(source["genres"])
	result["production_companies"] = namedItems([]interface{}{source["maker"], source["label"]})
	cast := make([]map[string]interface{}, 0)
	for _, name := range stringSlice(source["actors"]) {
		cast = append(cast, map[string]interface{}{"name": name, "character": ""})
	}
	crew := make([]map[string]interface{}, 0)
	if director := stringValue(source["director"]); director != "" {
		crew = append(crew, map[string]interface{}{"name": director, "job": "Director"})
	}
	result["credits"] = map[string]interface{}{"cast": cast, "crew": crew}
	return result
}

// metaTubeImageURL 将 MetaTube 返回的绝对、协议相对或相对图片地址统一为浏览器可加载的地址。
func metaTubeImageURL(baseURL string, values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "//") {
			return "http:" + value
		}
		if parsed, err := url.Parse(value); err == nil && parsed.IsAbs() {
			return value
		}
		if strings.HasPrefix(value, "/") {
			return strings.TrimRight(baseURL, "/") + value
		}
		return strings.TrimRight(baseURL, "/") + "/" + value
	}
	return ""
}

func metaTubeSyntheticID(provider, id string) int {
	return int(crc32.ChecksumIEEE([]byte(provider+":"+id)) & 0x7fffffff)
}
func stringValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
func firstString(values ...interface{}) string {
	for _, v := range values {
		if s := stringValue(v); s != "" {
			return s
		}
	}
	return ""
}
func intValue(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}
func floatValue(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}
func yearFromDate(v string) int {
	if len(v) < 4 {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(v[:4]))
	return n
}
func stringSlice(v interface{}) []string {
	arr, _ := v.([]interface{})
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s := stringValue(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}
func namedItems(v interface{}) []map[string]interface{} {
	var values []interface{}
	switch x := v.(type) {
	case []interface{}:
		values = x
	default:
		values = []interface{}{x}
	}
	out := make([]map[string]interface{}, 0, len(values))
	for _, item := range values {
		if s := stringValue(item); s != "" {
			out = append(out, map[string]interface{}{"name": s})
		}
	}
	return out
}
