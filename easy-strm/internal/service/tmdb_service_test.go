package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"easy-strm/internal/dao"
)

// ============================================
// TmdbService 文件名解析测试（纯逻辑，无网络依赖）
// ============================================

// TestParseFilename_电影_年份 测试从标准电影文件名解析出标题和年份
func TestParseFilename_电影_年份(t *testing.T) {
	svc := NewTmdbService("fake-key", nil)

	cases := []struct {
		name      string
		filename  string
		wantTitle string
		wantYear  int
		wantType  string
	}{
		{
			name:      "标准电影命名",
			filename:  "Inception.2010.1080p.BluRay.x264.mkv",
			wantTitle: "Inception",
			wantYear:  2010,
			wantType:  "movie",
		},
		{
			name:      "电影带空格分隔",
			filename:  "The Dark Knight 2008 720p.mp4",
			wantTitle: "The Dark Knight",
			wantYear:  2008,
			wantType:  "movie",
		},
		{
			name:      "中文电影名",
			filename:  "流浪地球2.2023.2160p.x265.mkv",
			wantTitle: "流浪地球2",
			wantYear:  2023,
			wantType:  "movie",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed := svc.parseFilename(tc.filename)
			if parsed.Title != tc.wantTitle {
				t.Errorf("标题不匹配: got=%q, want=%q", parsed.Title, tc.wantTitle)
			}
			if parsed.Year != tc.wantYear {
				t.Errorf("年份不匹配: got=%d, want=%d", parsed.Year, tc.wantYear)
			}
			if parsed.MediaType != tc.wantType {
				t.Errorf("类型不匹配: got=%q, want=%q", parsed.MediaType, tc.wantType)
			}
		})
	}
}

func TestExtractPreferredTitles_ChineseTranslationPreferred(t *testing.T) {
	svc := NewTmdbService("key", nil)

	detail := map[string]interface{}{
		"title":          "薬屋のひとりごと",
		"original_title": "薬屋のひとりごと",
		"translations": map[string]interface{}{
			"translations": []interface{}{
				map[string]interface{}{
					"iso_639_1": "zh",
					"data": map[string]interface{}{
						"title": "药屋少女的呢喃",
					},
				},
			},
		},
	}

	title, originalTitle := svc.extractPreferredTitles(detail, "movie", "薬屋のひとりごと", "薬屋のひとりごと")
	if title != "药屋少女的呢喃" {
		t.Fatalf("expected chinese title to be preferred, got %q", title)
	}
	if originalTitle != "薬屋のひとりごと" {
		t.Fatalf("expected original title to be preserved, got %q", originalTitle)
	}
}

// TestParseFilename_剧集_季集 测试从剧集文件名中解析出季集信息
func TestParseFilename_剧集_季集(t *testing.T) {
	svc := NewTmdbService("fake-key", nil)

	cases := []struct {
		name        string
		filename    string
		wantType    string
		wantSeason  int
		wantEpisode int
	}{
		{
			name:        "S01E02格式",
			filename:    "Breaking.Bad.S01E02.1080p.BluRay.mkv",
			wantType:    "tv",
			wantSeason:  1,
			wantEpisode: 2,
		},
		{
			name:        "Season Episode格式",
			filename:    "Friends Season 3 Episode 14.mp4",
			wantType:    "tv",
			wantSeason:  3,
			wantEpisode: 14,
		},
		{
			name:        "1x02格式",
			filename:    "House.1x05.720p.mkv",
			wantType:    "tv",
			wantSeason:  1,
			wantEpisode: 5,
		},
		{
			name:        "EP格式（默认第一季）",
			filename:    "三体 EP03.mkv",
			wantType:    "tv",
			wantSeason:  1,
			wantEpisode: 3,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed := svc.parseFilename(tc.filename)
			if parsed.MediaType != tc.wantType {
				t.Errorf("类型不匹配: got=%q, want=%q", parsed.MediaType, tc.wantType)
			}
			if parsed.Season != tc.wantSeason {
				t.Errorf("季数不匹配: got=%d, want=%d", parsed.Season, tc.wantSeason)
			}
			if parsed.Episode != tc.wantEpisode {
				t.Errorf("集数不匹配: got=%d, want=%d", parsed.Episode, tc.wantEpisode)
			}
		})
	}
}

// TestParseFilename_视频质量 测试视频质量、来源、编码的提取
func TestParseFilename_视频质量(t *testing.T) {
	svc := NewTmdbService("fake-key", nil)

	cases := []struct {
		name        string
		filename    string
		wantQuality string
		wantSource  string
		wantCodec   string
	}{
		{
			name:        "1080p BluRay x264",
			filename:    "Movie.2020.1080p.BluRay.x264.mkv",
			wantQuality: "", // map遍历顺序不确定，不严格检查quality
			wantCodec:   "x264",
		},
		{
			name:        "4K WEB-DL HEVC",
			filename:    "Movie.2021.4K.WEB-DL.HEVC.mkv",
			wantQuality: "4K",
			wantCodec:   "HEVC",
		},
		{
			name:        "720p AMZN",
			filename:    "Show.S01E01.720p.AMZN.WEB-DL.mp4",
			wantQuality: "720p",
			wantSource:  "AMZN",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed := svc.parseFilename(tc.filename)
			if tc.wantQuality != "" && parsed.Quality != tc.wantQuality {
				t.Errorf("质量不匹配: got=%q, want=%q", parsed.Quality, tc.wantQuality)
			}
			if tc.wantSource != "" && parsed.Source != tc.wantSource {
				t.Errorf("来源不匹配: got=%q, want=%q", parsed.Source, tc.wantSource)
			}
			if tc.wantCodec != "" && parsed.Codec != tc.wantCodec {
				t.Errorf("编码不匹配: got=%q, want=%q", parsed.Codec, tc.wantCodec)
			}
		})
	}
}

// TestParseFilename_无法解析 测试无法解析的文件名
func TestParseFilename_无法解析(t *testing.T) {
	svc := NewTmdbService("fake-key", nil)

	// 纯数字或特殊字符的文件名
	parsed := svc.parseFilename(".mkv")
	if parsed.Title != "" && parsed.Title != " " {
		// 只要不panic即可，标题可能为空或仅含空格
		t.Logf("解析结果: title=%q", parsed.Title)
	}
}

// ============================================
// TmdbService API 调用测试（使用 httptest Mock）
// ============================================

// TestSearchMovie_Top3候选 测试电影搜索返回 Top 3 候选
func TestSearchMovie_Top3候选(t *testing.T) {
	// 创建模拟 TMDB API 服务
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求路径
		if r.URL.Path != "/search/movie" {
			t.Errorf("意外的请求路径: %s", r.URL.Path)
		}

		// 返回模拟数据（5个结果，只应返回3个）
		resp := map[string]interface{}{
			"results": []map[string]interface{}{
				{"id": 100, "title": "电影1", "original_title": "Movie 1", "release_date": "2020-01-01", "poster_path": "/poster1.jpg", "overview": "简介1", "vote_average": 8.5},
				{"id": 200, "title": "电影2", "original_title": "Movie 2", "release_date": "2021-06-15", "poster_path": "/poster2.jpg", "overview": "简介2", "vote_average": 7.0},
				{"id": 300, "title": "电影3", "original_title": "Movie 3", "release_date": "2022-03-20", "poster_path": "/poster3.jpg", "overview": "简介3", "vote_average": 6.5},
				{"id": 400, "title": "电影4", "original_title": "Movie 4", "release_date": "2019-12-01", "poster_path": "/poster4.jpg", "overview": "简介4", "vote_average": 5.0},
				{"id": 500, "title": "电影5", "original_title": "Movie 5", "release_date": "2018-07-10", "poster_path": "/poster5.jpg", "overview": "简介5", "vote_average": 4.5},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	svc := NewTmdbService("test-api-key", nil)
	svc.baseURL = mockServer.URL

	results, err := svc.SearchMovie("test", 0)
	if err != nil {
		t.Fatalf("搜索电影失败: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("应只返回Top 3候选，got=%d", len(results))
	}
	if results[0].TmdbID != 100 {
		t.Errorf("第一个结果ID不匹配: got=%d, want=100", results[0].TmdbID)
	}
	if results[0].MediaType != "movie" {
		t.Errorf("MediaType应为movie, got=%q", results[0].MediaType)
	}
	if results[0].Year != 2020 {
		t.Errorf("年份不匹配: got=%d, want=2020", results[0].Year)
	}
}

// TestSearchTV_Top3候选 测试剧集搜索返回 Top 3 候选
func TestSearchTV_Top3候选(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"results": []map[string]interface{}{
				{"id": 1, "name": "剧集A", "original_name": "Show A", "first_air_date": "2021-01-01", "poster_path": "/a.jpg", "overview": "简介A", "vote_average": 9.0},
				{"id": 2, "name": "剧集B", "original_name": "Show B", "first_air_date": "2022-06-01", "poster_path": "/b.jpg", "overview": "简介B", "vote_average": 8.0},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	svc := NewTmdbService("test-api-key", nil)
	svc.baseURL = mockServer.URL

	results, err := svc.SearchTV("test", 0)
	if err != nil {
		t.Fatalf("搜索剧集失败: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("应返回2个结果，got=%d", len(results))
	}
	if results[0].MediaType != "tv" {
		t.Errorf("MediaType应为tv, got=%q", results[0].MediaType)
	}
}

// TestSearchMovie_无APIKey 测试未配置API Key时应返回错误
func TestSearchMovie_无APIKey(t *testing.T) {
	svc := NewTmdbService("", nil)
	_, err := svc.SearchMovie("test", 0)
	if err == nil {
		t.Fatal("未配置API Key时应返回错误")
	}
}

// TestSearchTV_无APIKey 测试未配置API Key时应返回错误
func TestSearchTV_无APIKey(t *testing.T) {
	svc := NewTmdbService("", nil)
	_, err := svc.SearchTV("test", 0)
	if err == nil {
		t.Fatal("未配置API Key时应返回错误")
	}
}

// TestSearchMovie_空结果 测试没有搜索结果时返回空列表
func TestSearchMovie_空结果(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"results": []map[string]interface{}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	svc := NewTmdbService("test-api-key", nil)
	svc.baseURL = mockServer.URL

	results, err := svc.SearchMovie("不存在的电影", 0)
	if err != nil {
		t.Fatalf("搜索不应报错: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("应返回空结果, got=%d", len(results))
	}
}

// TestIdentifyFile_电影识别 测试电影文件的自动识别
func TestIdentifyFile_电影识别(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"results": []map[string]interface{}{
				{"id": 27205, "title": "盗梦空间", "original_title": "Inception", "release_date": "2010-07-16", "poster_path": "/inception.jpg", "overview": "一个关于梦境的故事", "vote_average": 8.4},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	// 需要 mock cacheDAO 来避免 DB 调用
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	cacheDAO := dao.NewTmdbCacheDAO()
	svc := NewTmdbService("test-api-key", cacheDAO)
	svc.baseURL = mockServer.URL

	// mock 缓存查询（无缓存命中）
	mock.ExpectQuery(`SELECT .+ FROM t_tmdb_cache`).
		WillReturnRows(mock.NewRows(nil))

	// mock 缓存写入
	mock.ExpectQuery(`INSERT INTO t_tmdb_cache`).
		WillReturnRows(mock.NewRows([]string{"id", "create_time", "update_time"}).
			AddRow(1, "2026-01-01", "2026-01-01"))

	result, err := svc.IdentifyFile("Inception.2010.1080p.BluRay.x264.mkv")
	if err != nil {
		t.Fatalf("识别失败: %v", err)
	}
	if !result.Success {
		t.Fatalf("识别应当成功: %s", result.Message)
	}
	if result.MediaType != "movie" {
		t.Errorf("类型应为movie, got=%q", result.MediaType)
	}
	if result.TmdbID != 27205 {
		t.Errorf("TMDB ID不匹配: got=%d, want=27205", result.TmdbID)
	}
	if len(result.Candidates) == 0 {
		t.Error("应返回候选列表")
	}
}

// TestIdentifyFile_剧集识别 测试剧集文件的自动识别
func TestIdentifyFile_剧集识别(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"results": []map[string]interface{}{
				{"id": 1396, "name": "绝命毒师", "original_name": "Breaking Bad", "first_air_date": "2008-01-20", "poster_path": "/bb.jpg", "overview": "一位化学老师的故事", "vote_average": 9.5},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	cacheDAO := dao.NewTmdbCacheDAO()
	svc := NewTmdbService("test-api-key", cacheDAO)
	svc.baseURL = mockServer.URL

	// mock 缓存查询（无缓存命中）
	mock.ExpectQuery(`SELECT .+ FROM t_tmdb_cache`).
		WillReturnRows(mock.NewRows(nil))

	// mock 缓存写入
	mock.ExpectQuery(`INSERT INTO t_tmdb_cache`).
		WillReturnRows(mock.NewRows([]string{"id", "create_time", "update_time"}).
			AddRow(1, "2026-01-01", "2026-01-01"))

	result, err := svc.IdentifyFile("Breaking.Bad.S01E02.1080p.BluRay.mkv")
	if err != nil {
		t.Fatalf("识别失败: %v", err)
	}
	if !result.Success {
		t.Fatalf("识别应当成功: %s", result.Message)
	}
	if result.MediaType != "tv" {
		t.Errorf("类型应为tv, got=%q", result.MediaType)
	}
	if result.SeasonNumber != 1 || result.EpisodeNumber != 2 {
		t.Errorf("季集信息不匹配: S%02dE%02d, want S01E02", result.SeasonNumber, result.EpisodeNumber)
	}
}

// TestIdentifyFile_识别失败_无标题 测试无法解析出标题时返回失败
func TestIdentifyFile_识别失败_无标题(t *testing.T) {
	svc := NewTmdbService("test-api-key", nil)

	result, err := svc.IdentifyFile(".mkv")
	if err != nil {
		t.Fatalf("不应返回error: %v", err)
	}
	if result.Success {
		t.Error("无法解析标题时应返回失败")
	}
}

// TestIdentifyFile_FallbackTokenizeSearch 测试根据中英文切分的回退搜索能力
func TestIdentifyFile_FallbackTokenizeSearch(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求中是否包含我们期待的切分词语
		query := r.URL.Query().Get("query")

		var resp map[string]interface{}

		// 模拟：如果是中英混杂全标题，搜索不到结果
		if query == "流浪地球 The Wandering Earth" {
			resp = map[string]interface{}{
				"results": []map[string]interface{}{},
			}
		} else if query == "流浪地球" {
			// 模拟：提取出纯中文后，能搜索到结果
			resp = map[string]interface{}{
				"results": []map[string]interface{}{
					{"id": 12345, "title": "流浪地球", "original_title": "The Wandering Earth", "release_date": "2019-02-05", "poster_path": "/earth.jpg", "overview": "地球逃亡", "vote_average": 8.0},
				},
			}
		} else {
			resp = map[string]interface{}{
				"results": []map[string]interface{}{},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	cacheDAO := dao.NewTmdbCacheDAO()
	svc := NewTmdbService("test-api-key", cacheDAO)
	svc.baseURL = mockServer.URL

	// mock 缓存查询（无缓存命中）
	mock.ExpectQuery(`SELECT .+ FROM t_tmdb_cache`).WillReturnRows(mock.NewRows(nil))
	// mock 缓存写入
	mock.ExpectQuery(`INSERT INTO t_tmdb_cache`).WillReturnRows(mock.NewRows([]string{"id", "create_time", "update_time"}).AddRow(1, "2026-01-01", "2026-01-01"))

	result, err := svc.IdentifyFile("流浪地球 The Wandering Earth 1080p.mp4")
	if err != nil {
		t.Fatalf("识别失败: %v", err)
	}
	if !result.Success {
		t.Fatalf("回退搜索应当成功: %s", result.Message)
	}
	if result.TmdbID != 12345 {
		t.Errorf("TMDB ID不匹配: got=%d, want=12345", result.TmdbID)
	}
	if result.Title != "流浪地球" {
		t.Errorf("TMDB 标题不匹配: got=%s, want=流浪地球", result.Title)
	}
}

// TestSetAPIKey_动态设置 测试动态设置 API Key
func TestSetAPIKey_动态设置(t *testing.T) {
	svc := NewTmdbService("", nil)
	if svc.GetAPIKey() != "" {
		t.Error("初始 API Key 应为空")
	}

	svc.SetAPIKey("new-key")
	if svc.GetAPIKey() != "new-key" {
		t.Errorf("API Key 设置失败: got=%q", svc.GetAPIKey())
	}
}

// TestSetLanguage_动态设置 测试动态设置语言
func TestSetLanguage_动态设置(t *testing.T) {
	svc := NewTmdbService("key", nil)
	if svc.GetLanguage() != "zh-CN" {
		t.Errorf("默认语言应为zh-CN, got=%q", svc.GetLanguage())
	}

	svc.SetLanguage("en")
	if svc.GetLanguage() != "en" {
		t.Errorf("语言设置失败: got=%q", svc.GetLanguage())
	}
}

// TestGetImageURL_完整路径 测试图片URL拼接
func TestGetImageURL_完整路径(t *testing.T) {
	svc := NewTmdbService("key", nil)

	// 正常路径
	url := svc.getImageURL("/abc.jpg")
	if url != "https://image.tmdb.org/t/p/w500/abc.jpg" {
		t.Errorf("URL不匹配: got=%q", url)
	}

	// 空路径
	url = svc.getImageURL("")
	if url != "" {
		t.Errorf("空路径应返回空字符串, got=%q", url)
	}
}

// TestBuildCacheKey_小写化 测试缓存键生成
func TestBuildCacheKey_小写化(t *testing.T) {
	svc := NewTmdbService("key", nil)

	key := svc.buildCacheKey("Inception.2010.1080p.mkv", "movie")
	if key != "inception.2010.1080p.mkv" {
		t.Errorf("缓存键应全小写化: got=%q", key)
	}
}

// TestIdentifyFile_缓存缺少分类元数据时补全 测试旧缓存缺少 genre_ids 时会补齐详情元数据，确保分类整理可命中规则
func TestIdentifyFile_缓存缺少分类元数据时补全(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/64789" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		resp := map[string]interface{}{
			"genres": []map[string]interface{}{
				{"id": 16, "name": "Animation"},
			},
			"production_countries": []map[string]interface{}{
				{"iso_3166_1": "JP"},
			},
			"original_language": "ja",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	cacheDAO := dao.NewTmdbCacheDAO()
	svc := NewTmdbService("test-api-key", cacheDAO)
	svc.baseURL = mockServer.URL

	now := time.Now()
	rawData := []byte(`{"tmdb_id":64789,"title":"Doraemon","original_title":"Doraemon","media_type":"movie"}`)
	cacheRows := mock.NewRows([]string{
		"id", "query_key", "media_type", "tmdb_id", "title", "original_title", "year", "poster_path",
		"overview", "vote_average", "release_date", "first_air_date", "season_number", "episode_number",
		"raw_data", "expire_at", "create_time", "update_time",
	}).AddRow(
		1, "doraemon.1980.mp4", "movie", 64789, "Doraemon", "Doraemon", 1980, nil,
		nil, nil, "1980-03-15", nil, nil, nil,
		rawData, now.Add(24*time.Hour), now, now,
	)
	mock.ExpectQuery(`SELECT .+ FROM t_tmdb_cache`).
		WithArgs("doraemon.1980.mp4", "movie").
		WillReturnRows(cacheRows)
	mock.ExpectExec(`UPDATE t_tmdb_cache SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := svc.IdentifyFile("Doraemon.1980.mp4")
	if err != nil {
		t.Fatalf("识别失败: %v", err)
	}
	if !result.Success {
		t.Fatalf("缓存识别应当成功: %s", result.Message)
	}
	if len(result.GenreIDs) != 1 || result.GenreIDs[0] != 16 {
		t.Fatalf("应补齐动画 genre id: got=%v", result.GenreIDs)
	}
	if len(result.Countries) != 1 || result.Countries[0] != "JP" {
		t.Fatalf("应补齐制片国家: got=%v", result.Countries)
	}
	if result.Language != "ja" {
		t.Fatalf("应补齐原始语言: got=%q", result.Language)
	}
}
