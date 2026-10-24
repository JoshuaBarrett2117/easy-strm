package service

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

type round2TVFixture struct {
	id                                                   int
	title, original, first, last, seasonDate, seasonName string
	season                                               int
}

func round2TVService(t *testing.T, fixtures []round2TVFixture, queries ...string) *TmdbService {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	dao.InitTaskRedisDAO(client)
	t.Cleanup(func() { dao.InitTaskRedisDAO(nil); client.Close() })
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/search/tv" {
			if request.URL.Query().Get("first_air_date_year") != "" {
				t.Error("season year restricted premiere search")
			}
			query := request.URL.Query().Get("query")
			allowed := false
			for _, title := range queries {
				allowed = allowed || query == title
			}
			if len(queries) == 0 {
				for _, fixture := range fixtures {
					allowed = allowed || query == fixture.title || query == fixture.original
				}
			}
			results := []any{}
			if allowed {
				for _, fixture := range fixtures {
					results = append(results, map[string]any{"id": fixture.id, "name": fixture.title, "original_name": fixture.original, "first_air_date": fixture.first})
				}
			}
			t.Logf("HTTP search/tv query=%q accepted=%v results=%d", query, allowed, len(results))
			json.NewEncoder(writer).Encode(map[string]any{"results": results})
			return
		}
		for _, fixture := range fixtures {
			season := map[string]any{"season_number": fixture.season, "name": fixture.seasonName, "air_date": fixture.seasonDate}
			if request.URL.Path == fmt.Sprintf("/tv/%d", fixture.id) {
				json.NewEncoder(writer).Encode(map[string]any{"id": fixture.id, "name": fixture.title, "original_name": fixture.original, "first_air_date": fixture.first, "last_air_date": fixture.last, "seasons": []any{season}})
				return
			}
			if request.URL.Path == fmt.Sprintf("/tv/%d/season/%d", fixture.id, fixture.season) {
				json.NewEncoder(writer).Encode(season)
				return
			}
		}
		if strings.Contains(request.URL.Path, "/season/") {
			http.NotFound(writer, request)
			return
		}
		fmt.Fprint(writer, `{"results":[],"translations":[]}`)
	}))
	t.Cleanup(server.Close)
	service := NewTmdbService("mock-only", nil)
	service.baseURL, service.httpClient = server.URL, server.Client()
	return service
}

// TestRound2StrictSearchFixture 验证夹具拒绝季标记、集号和其他污染查询，不能凭空回传黄石候选。
func TestRound2StrictSearchFixture(t *testing.T) {
	service := round2TVService(t, []round2TVFixture{
		{73586, "黄石", "Yellowstone", "2018-06-20", "2024-12-15", "2020-06-21", "Season 3", 3},
	}, "Yellowstone", "黄石")
	for _, query := range []string{"Yellowstone", "黄石", "Yellowstone Season3", "Yellowstone Season03", "Yellowstone SEASON 3", "黄石 第3季", "Season3", "01", "S03E01"} {
		t.Run(query, func(t *testing.T) {
			results, err := service.searchTVContext(context.Background(), query, 0)
			want := 0
			if query == "Yellowstone" || query == "黄石" {
				want = 1
			}
			if err != nil || len(results) != want {
				t.Fatalf("query=%q results=%+v want=%d err=%v", query, results, want, err)
			}
		})
	}
}

const round2SupernaturalPath = "欧美剧 中英双语特效字幕 蓝光原盘Remux@ADE/[邪恶力量][全15季]Supernatural.S01-S15.2009.1080p.BluRay.Remux.VC-1.DD.5.1-ADE[1.68TB]/Supernatural.S03.2007.1080p.BluRay.Remux.VC-1.DD.5.1-ADE/Supernatural.S03E13.2007.1080p.BluRay.Remux.VC-1.DD.5.1-ADE.mkv"

// TestRound2SupernaturalIdentity 覆盖同名新旧剧、重叠年份、无关标题与具名季，并反转候选顺序。
func TestRound2SupernaturalIdentity(t *testing.T) {
	modern := round2TVFixture{1622, "邪恶力量", "Supernatural", "2005-09-13", "2020-11-19", "2007-10-04", "Season 3", 3}
	old := round2TVFixture{13836, "超自然故事集", "Supernatural", "1977-06-11", "1977-08-06", "1977-06-11", "Season 3", 3}
	for _, mode := range []string{"old", "overlap", "unrelated", "named", "named_old"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%v", mode, reverse), func(t *testing.T) {
				other := old
				if mode != "old" && mode != "named_old" {
					other.first, other.last, other.seasonDate = "2005-01-01", "2010-12-31", "2007-01-01"
				}
				if mode == "unrelated" || strings.HasPrefix(mode, "named") {
					other.title, other.original = "Unrelated Show", "Unrelated Show"
				}
				if strings.HasPrefix(mode, "named") {
					other.seasonName = "Supernatural"
				}
				fixtures := []round2TVFixture{modern, other}
				if reverse {
					fixtures[0], fixtures[1] = fixtures[1], fixtures[0]
				}
				service := round2TVService(t, fixtures)
				input := shareEpisodeInput(context.Background(), round2SupernaturalPath, "tv")
				query := service.analyzeShareQuery(input, "tv")
				if query.Year != 2009 || query.fileYear != 2007 || query.Season != 3 || query.SeasonYear != 2007 || query.YearSource != "work_directory" {
					t.Fatalf("query=%+v", query)
				}
				// named：无关作品只有季名撞名，主标题命中的候选优先，因此不再判冲突。
				want := mode != "overlap"
				result, err := service.IdentifyShareFile(context.Background(), round2SupernaturalPath, "tmdb", "tv")
				if err != nil || result == nil || result.Success != want || want && (result.TmdbID != 1622 || result.SeasonNumber != 3 || result.EpisodeNumber != 13) || !want && !strings.Contains(result.FailureReason, "冲突") {
					t.Fatalf("result=%+v err=%v", result, err)
				}
				query.Evidence = &shareRecognitionEvidence{}
				candidates := []domain.TmdbSearchResult{}
				for _, fixture := range fixtures {
					candidates = append(candidates, domain.TmdbSearchResult{TmdbID: fixture.id, Title: fixture.title, OriginalTitle: fixture.original, MediaType: "tv", Year: shareDateYear(fixture.first)})
				}
				selected, err := service.selectShareTVCandidate(WithRecognitionRound(context.Background()), query, candidates)
				if err != nil || (selected != nil) != want || selected != nil && selected.TmdbID != 1622 {
					t.Fatalf("selected=%+v err=%v", selected, err)
				}
			})
		}
	}
}

// TestRound2YearEvidencePropagation 验证分批补全年份不丢失已核验候选，歧义只来自有效同名身份。
func TestRound2YearEvidencePropagation(t *testing.T) {
	service := round2TVService(t, []round2TVFixture{
		{1622, "邪恶力量", "Supernatural", "2005-09-13", "2020-11-19", "2007-10-04", "Season 3", 3},
		{13836, "超自然故事集", "Supernatural", "1977-06-11", "1977-08-06", "1977-06-11", "Season 3", 3},
	})
	modern := domain.TmdbSearchResult{TmdbID: 1622, Title: "邪恶力量", OriginalTitle: "Supernatural", MediaType: "tv", Year: 2005}
	old := domain.TmdbSearchResult{TmdbID: 13836, Title: "超自然故事集", OriginalTitle: "Supernatural", MediaType: "tv", Year: 1977}
	query := service.analyzeShareQuery(round2SupernaturalPath, "tv")
	ctx := WithRecognitionRound(context.Background())
	query, err := service.enrichShareYearEvidence(ctx, query, []domain.TmdbSearchResult{modern})
	if err != nil {
		t.Fatal(err)
	}
	query, err = service.enrichShareYearEvidence(ctx, query, []domain.TmdbSearchResult{old})
	if err != nil {
		t.Fatal(err)
	}
	if !shareCandidateYearMatches(query, modern) || shareCandidateYearMatches(query, old) {
		t.Fatalf("lost enriched years: %+v", query.tvYears)
	}
	evidence := &shareRecognitionEvidence{}
	evidence.recordMatches(query, []domain.TmdbSearchResult{old, modern}, nil)
	if evidence.ambiguous {
		t.Fatal("old broadcast interval is not ambiguity")
	}
	overlap := old
	overlap.TmdbID = 999
	query.tvYears[shareCandidateIdentity(overlap)] = shareTVYears{first: 2005, last: 2010}
	evidence.recordMatches(query, []domain.TmdbSearchResult{overlap, modern}, nil)
	if !evidence.ambiguous {
		t.Fatal("overlapping same-title identities must remain ambiguous")
	}
}

// TestRound2SupernaturalAutoEntry 使用原始完整路径核验未指定类型、自动类型和强制剧集入口。
func TestRound2SupernaturalAutoEntry(t *testing.T) {
	for _, source := range []string{"tmdb", "auto"} {
		for _, forcedType := range []string{"", "auto", "tv"} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("source=%s/type=%q/reverse=%v", source, forcedType, reverse), func(t *testing.T) {
					fixtures := []round2TVFixture{
						{13836, "超自然故事集", "Supernatural", "1977-06-11", "1977-08-06", "1977-06-11", "Season 3", 3},
						{1622, "邪恶力量", "Supernatural", "2005-09-13", "2020-11-19", "2007-10-04", "Season 3", 3},
					}
					if reverse {
						fixtures[0], fixtures[1] = fixtures[1], fixtures[0]
					}
					service := round2TVService(t, fixtures, "Supernatural", "邪恶力量")
					query := AnalyzeShareFilename(round2SupernaturalPath)
					t.Logf("AnalyzeShareFilename query=%+v", query)
					if query.MediaType != "tv" || query.Year != 2009 || query.SeasonYear != 2007 || query.fileYear != 2007 || query.YearSource != "work_directory" {
						t.Errorf("directory evidence lost: %+v", query)
					}
					input := shareEpisodeInput(context.Background(), round2SupernaturalPath, forcedType)
					t.Logf("input=%q parsed=%+v query=%+v", input, service.parseFilenameForMediaType(input, forcedType), service.analyzeShareQuery(input, forcedType))
					result, err := service.IdentifyShareFile(context.Background(), round2SupernaturalPath, source, forcedType)
					t.Logf("IdentifyShareFile source=%q forcedType=%q result=%+v err=%v", source, forcedType, result, err)
					if err != nil || result == nil || !result.Success || result.MediaType != "tv" || result.TmdbID != 1622 || result.SeasonNumber != 3 || result.EpisodeNumber != 13 || result.Filename != round2SupernaturalPath {
						t.Errorf("wrong auto identity: result=%+v err=%v", result, err)
					}
				})
			}
		}
	}
}

// TestRound2YellowstoneDirectories 只允许干净片名命中搜索，分别核验公开解析与实际自动入口。
func TestRound2YellowstoneDirectories(t *testing.T) {
	parent := "[黄石][全5季]Yellowstone.S01-S05.2018.1080p.BluRay.Remux.AVC.TrueHD.5.1-ADE[510.01GB]/"
	for _, token := range []string{"Season3", "Season03", "SEASON 3", "第3季"} {
		for _, directory := range []string{"Yellowstone." + token + ".2020.1080p.BluRay.Remux", token} {
			for _, filename := range []string{"01.mkv", "S03E01.mkv", "Yellowstone.S03E01.2020.mkv"} {
				name := parent + directory + "/" + filename
				t.Run(directory+"/"+filename, func(t *testing.T) {
					t.Run("AnalyzeShareFilename", func(t *testing.T) {
						query := AnalyzeShareFilename(name)
						t.Logf("path=%q query=%+v", name, query)
						if query.MediaType != "tv" || query.Year != 2018 || len(query.Titles) == 0 {
							t.Errorf("wrong directory evidence: %+v", query)
						}
						for _, title := range query.Titles {
							if title != "Yellowstone" && title != "黄石" {
								t.Errorf("polluted title: %q", title)
							}
						}
					})
					for _, source := range []string{"tmdb", "auto"} {
						for _, forcedType := range []string{"", "auto", "tv"} {
							t.Run(fmt.Sprintf("source=%s/type=%q", source, forcedType), func(t *testing.T) {
								service := round2TVService(t, []round2TVFixture{
									{19355, "黄石公园", "Yellowstone Park", "2009-03-15", "2009-03-29", "2009-03-15", "Season 3", 3},
									{73586, "黄石", "Yellowstone", "2018-06-20", "2024-12-15", "2020-06-21", "Season 3", 3},
								}, "Yellowstone", "黄石")
								input := shareEpisodeInput(context.Background(), name, forcedType)
								query := service.analyzeShareQuery(input, forcedType)
								parsed := service.parseFilenameForMediaType(input, forcedType)
								t.Logf("input=%q query=%+v parsed=%+v", input, query, parsed)
								if query.Season != 3 || query.Year != 2018 || parsed.Season != 3 || parsed.Episode != 1 || (parsed.Title != "Yellowstone" && parsed.Title != "黄石") || strings.HasPrefix(directory, "Yellowstone.") && query.SeasonYear != 2020 {
									t.Errorf("wrong parsed evidence: query=%+v parsed=%+v", query, parsed)
								}
								if directory == token && filename != "Yellowstone.S03E01.2020.mkv" && query.SeasonYear != 0 {
									t.Errorf("work year became season year: %+v", query)
								}
								result, err := service.IdentifyShareFile(context.Background(), name, source, forcedType)
								t.Logf("IdentifyShareFile result=%+v err=%v", result, err)
								if err != nil || result == nil || !result.Success || result.TmdbID != 73586 || result.SeasonNumber != 3 || result.EpisodeNumber != 1 || result.Filename != name {
									t.Errorf("wrong identity: result=%+v err=%v", result, err)
								}
							})
						}
					}
				})
			}
		}
	}
}

// TestRound2StandaloneSeasonFilename 清除独立季标记，不将其作为查询标题或伪造集号。
func TestRound2StandaloneSeasonFilename(t *testing.T) {
	for _, token := range []string{"Season3", "Season03", "SEASON 3", "第3季"} {
		t.Run(token, func(t *testing.T) {
			name := "[黄石][全5季]Yellowstone.S01-S05.2018/Yellowstone." + token + ".2020/" + token + ".mkv"
			query := AnalyzeShareFilename(name)
			t.Logf("path=%q query=%+v", name, query)
			if query.MediaType != "tv" || query.Year != 2018 || query.SeasonYear != 2020 || !reflect.DeepEqual(query.Titles, []string{"Yellowstone"}) {
				t.Fatalf("unclean standalone season: %+v", query)
			}
		})
	}
}

// TestRound2ExplicitZeroParsing 验证明确零集及混合集保留原始集号，缺省值不生成明确集号。
func TestRound2ExplicitZeroParsing(t *testing.T) {
	service := NewTmdbService("mock-only", nil)
	for _, test := range []struct {
		file     string
		season   int
		episodes []int
	}{
		{"Twin.Peaks.S01E00.1990.mkv", 1, []int{0}},
		{"Twin.Peaks.S01E00E01.1990.mkv", 1, []int{0, 1}},
		{"Twin.Peaks.S00E00.1990.mkv", 0, []int{0}},
		{"Twin.Peaks.S01E01.1990.mkv", 1, []int{1}},
		{"Twin.Peaks.1990.mkv", 0, []int{}},
	} {
		t.Run(test.file, func(t *testing.T) {
			parsed := service.ParseFilename(test.file)
			if parsed.Season != test.season || !reflect.DeepEqual(parsed.Episodes, test.episodes) {
				t.Fatalf("parsed=%+v want=%v", parsed, test.episodes)
			}
		})
	}
}

// TestRound2SpecialSkipIdentificationAndCache 验证复用作品身份后仍逐文件跳过零集，且普通集不受影响。
func TestRound2SpecialSkipIdentificationAndCache(t *testing.T) {
	service := round2TVService(t, []round2TVFixture{{1920, "双峰", "Twin Peaks", "1990-04-08", "2017-09-03", "1990-04-08", "Season 1", 1}})
	parent := "剧集/[双峰][全3季]Twin.Peaks.S01-S03.1990/Twin.Peaks.S01.1990.Repack/"
	ctx := WithRecognitionRound(context.Background())
	for _, name := range []string{"Twin.Peaks.S01E01.1990.mkv", "Twin.Peaks.S01E00.1990.mkv", "Twin.Peaks.S01E00E01.1990.mkv", "S01E00.mkv", "Twin.Peaks.S01E01.1990.mkv", "Twin.Peaks.S00E00.1990.mkv"} {
		t.Run(name, func(t *testing.T) {
			result, err := service.IdentifyShareFile(ctx, parent+name, "tmdb", "tv")
			if err != nil || result == nil {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if strings.Contains(name, "E00") {
				season := 1
				if strings.Contains(name, "S00") {
					season = 0
				}
				if result.Success || result.SeasonNumber != season || result.EpisodeNumber != 0 || !strings.HasPrefix(result.FailureReason, "特辑跳过") || result.Title == "" || result.TmdbID != 1920 {
					t.Fatalf("explicit zero not safely skipped: %+v", result)
				}
			} else if !result.Success || result.EpisodeNumber != 1 {
				t.Fatalf("ordinary episode failed: %+v", result)
			}
		})
	}
	result, err := service.IdentifyShareFile(context.Background(), parent+"Twin.Peaks.S01E00.1980.mkv", "tmdb", "tv")
	if err != nil || result.Success || !strings.Contains(result.FailureReason, "年份") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

// TestRound2SpecialSkipNormalization 区分明确零集、缺失季集与正常集号，不放宽持久化约束。
func TestRound2SpecialSkipNormalization(t *testing.T) {
	for _, test := range []struct {
		name            string
		explicit        []domain.ShareEpisode
		season, episode int
		more            []int
		reason          string
	}{
		{"explicit", []domain.ShareEpisode{{SeasonNumber: 1, EpisodeNumber: 0}}, 1, 0, nil, "特辑跳过"},
		{"parsed", nil, 1, 0, []int{0}, "特辑跳过"},
		{"mixed", nil, 1, 0, []int{0, 1}, "特辑跳过"},
		{"missing", nil, 1, 0, nil, "缺少季集信息"},
		{"absent", nil, 0, 0, nil, "缺少季集信息"},
		{"ordinary", nil, 1, 1, []int{1}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := normalizeShareEpisodes("tv", test.explicit, test.season, test.episode, test.more...)
			if test.reason == "" && err != nil || test.reason != "" && (err == nil || !strings.HasPrefix(err.Error(), test.reason)) {
				t.Fatalf("err=%v want=%s", err, test.reason)
			}
		})
	}
}

// TestRound2SpecialSkipManualAndReview 验证手动保存拦截零集，并在核对队列保留混合集信息。
func TestRound2SpecialSkipManualAndReview(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewShareRecordService(dao.NewShareRecordDAO(db), NewTmdbService("", nil), nil, nil)
	media := domain.ShareMedia{ID: 9, ShareID: 7, Version: 2, FileName: "Twin.Peaks.S01E00E01.1990.mkv", Result: &domain.TmdbIdentifyResult{TmdbID: 1920, Title: "双峰", MediaType: "tv", SeasonNumber: 1}}
	mock.ExpectQuery("SELECT f.share_id,f.version,f.file_name").WithArgs(9).WillReturnRows(sqlmock.NewRows([]string{"share_id", "version", "file_name", "metadata_source", "status", "available", "media_type", "result", "episodes"}).AddRow(7, 2, media.FileName, "tmdb", "failed", true, "tv", nil, `[]`))
	if _, err := service.ManualIdentifyWithCount(context.Background(), media); err == nil || !strings.HasPrefix(err.Error(), "特辑跳过") {
		t.Errorf("manual err=%v", err)
	}
	mock.ExpectQuery("SELECT count").WithArgs(`{"failed","pending"}`).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(2))
	mock.ExpectQuery("SELECT COALESCE").WithArgs(`{"failed","pending"}`, 20, 0).WillReturnRows(sqlmock.NewRows([]string{"data"}).AddRow(`[{"id":1,"file_name":"Twin.Peaks.S01E00E01.mkv","media_type":"tv","status":"failed"},{"id":2,"file_name":"Twin.Peaks.mkv","media_type":"tv","status":"pending"}]`))
	page, err := service.ListReviewItems(context.Background(), "", "", 0, 0, 1, 20)
	if err != nil || len(page.Data) != 2 || !reflect.DeepEqual(page.Data[0].Episodes, []domain.ShareEpisode{{SeasonNumber: 1, EpisodeNumber: 0}, {SeasonNumber: 1, EpisodeNumber: 1}}) || len(page.Data[1].Episodes) != 0 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type round2PrefixArgument string

type round2ManualResultArgument struct {
	filename string
}

// Match 核验手动正集修正及数据库文件名确实写入识别结果。
func (expected round2ManualResultArgument) Match(value driver.Value) bool {
	data, ok := value.([]byte)
	if !ok {
		return false
	}
	var result domain.TmdbIdentifyResult
	return json.Unmarshal(data, &result) == nil && result.Success && result.Filename == expected.filename && result.SeasonNumber == 2 && result.EpisodeNumber == 3
}

// TestRound2ManualPositiveEpisodeCorrection 允许人工修正发布编号，不用 E00 文件名覆盖明确正集映射。
func TestRound2ManualPositiveEpisodeCorrection(t *testing.T) {
	for _, test := range []struct {
		name, filename string
		explicit       bool
		withParser     bool
	}{
		{"explicit", "Twin.Peaks.S01E00.1990.mkv", true, true},
		{"result", "Twin.Peaks.S01E00.1990.mkv", false, true},
		{"stale_client", "Twin.Peaks.S01E01.1990.mkv", true, true},
		{"without_parser", "Twin.Peaks.S01E00.1990.mkv", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var parser *TmdbService
			if test.withParser {
				parser = NewTmdbService("", nil)
			}
			service := NewShareRecordService(dao.NewShareRecordDAO(db), parser, nil, nil)
			stored := "Twin.Peaks.S01E00.1990.mkv"
			media := domain.ShareMedia{ID: 9, ShareID: 7, Version: 2, FileName: test.filename, Result: &domain.TmdbIdentifyResult{TmdbID: 1920, Title: "双峰", MediaType: "tv", SeasonNumber: 2}}
			if test.explicit {
				media.Episodes = []domain.ShareEpisode{{SeasonNumber: 2, EpisodeNumber: 3}}
			} else {
				media.Result.EpisodeNumber = 3
			}
			mock.ExpectQuery("SELECT f.share_id,f.version,f.file_name").WithArgs(9).WillReturnRows(sqlmock.NewRows([]string{"share_id", "version", "file_name", "metadata_source", "status", "available", "media_type", "result", "episodes"}).AddRow(7, 2, stored, "tmdb", "failed", true, "tv", nil, `[]`))
			mock.ExpectBegin()
			mock.ExpectExec("UPDATE t_share_media_file SET status").WithArgs("identified", round2ManualResultArgument{filename: stored}, "", 9, 2).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT media_id").WithArgs(9).WillReturnRows(sqlmock.NewRows([]string{"media_id"}).AddRow(nil))
			mock.ExpectQuery("INSERT INTO t_share_media").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(15))
			mock.ExpectExec("UPDATE t_share_media_file SET media_id").WithArgs(15, "tmdb", 9).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("DELETE FROM t_share_media_file_episode").WithArgs(9).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec("INSERT INTO t_share_media_file_episode").WithArgs(9, 2, 3).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			count, err := service.ManualIdentifyWithCount(context.Background(), media)
			if err != nil || count != 1 {
				t.Fatalf("count=%d err=%v", count, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestRound2ManualUnresolvedEpisodes 仅按数据库文件名区分未修正的 E00 与普通缺失集号。
func TestRound2ManualUnresolvedEpisodes(t *testing.T) {
	for _, test := range []struct{ name, client, stored, reason string }{
		{"zero", "Twin.Peaks.S01E01.mkv", "Twin.Peaks.S01E00.mkv", "特辑跳过"},
		{"mixed_zero", "Twin.Peaks.S01E01.mkv", "Twin.Peaks.S01E00E01.mkv", "特辑跳过"},
		{"positive_not_guessed", "Twin.Peaks.S01E00.mkv", "Twin.Peaks.S01E01.mkv", "缺少季集信息"},
		{"missing", "Twin.Peaks.S01E00.mkv", "Twin.Peaks.mkv", "缺少季集信息"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			service := NewShareRecordService(dao.NewShareRecordDAO(db), nil, nil, nil)
			media := domain.ShareMedia{ID: 9, ShareID: 7, Version: 2, FileName: test.client, Result: &domain.TmdbIdentifyResult{TmdbID: 1920, Title: "双峰", MediaType: "tv", SeasonNumber: 1}}
			mock.ExpectQuery("SELECT f.share_id,f.version,f.file_name").WithArgs(9).WillReturnRows(sqlmock.NewRows([]string{"share_id", "version", "file_name", "metadata_source", "status", "available", "media_type", "result", "episodes"}).AddRow(7, 2, test.stored, "tmdb", "failed", true, "tv", nil, `[]`))
			count, err := service.ManualIdentifyWithCount(context.Background(), media)
			if count != 0 || err == nil || !strings.HasPrefix(err.Error(), test.reason) {
				t.Fatalf("count=%d err=%v want=%s", count, err, test.reason)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestRound2ManualExplicitZeroEpisode 不允许结果中的正集号覆盖显式零集或混合零集列表。
func TestRound2ManualExplicitZeroEpisode(t *testing.T) {
	for _, episodes := range [][]domain.ShareEpisode{
		{{SeasonNumber: 1, EpisodeNumber: 0}},
		{{SeasonNumber: 1, EpisodeNumber: 0}, {SeasonNumber: 1, EpisodeNumber: 1}},
	} {
		service := NewShareRecordService(nil, nil, nil, nil)
		media := domain.ShareMedia{ID: 9, ShareID: 7, Version: 2, FileName: "Twin.Peaks.S01E01.mkv", Episodes: episodes, Result: &domain.TmdbIdentifyResult{TmdbID: 1920, Title: "双峰", MediaType: "tv", SeasonNumber: 1, EpisodeNumber: 1}}
		count, err := service.ManualIdentifyWithCount(context.Background(), media)
		if count != 0 || err == nil || !strings.HasPrefix(err.Error(), "特辑跳过") {
			t.Fatalf("episodes=%+v count=%d err=%v", episodes, count, err)
		}
	}
}

// Match 核验隔离持久化调用中的中文失败原因前缀。
func (prefix round2PrefixArgument) Match(value driver.Value) bool {
	text, ok := value.(string)
	return ok && strings.HasPrefix(text, string(prefix))
}

// TestRound2SpecialSkipPersistenceHandoff 使用隔离 SQL mock 验证零集只保存失败原因，不关联剧集实体。
func TestRound2SpecialSkipPersistenceHandoff(t *testing.T) {
	for _, test := range []struct{ filename, reason string }{
		{"Twin.Peaks.S01E00.1990.mkv", "特辑跳过"},
		{"Twin.Peaks.S01E00E01.1990.mkv", "特辑跳过"},
		{"Twin.Peaks.1990.mkv", "缺少季集信息"},
	} {
		t.Run(test.filename, func(t *testing.T) {
			tmdb := round2TVService(t, []round2TVFixture{{1920, "双峰", "Twin Peaks", "1990-04-08", "2017-09-03", "1990-04-08", "Season 1", 1}})
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectQuery("SELECT f.share_id,f.version,f.file_name").WithArgs(9).WillReturnRows(sqlmock.NewRows([]string{"share_id", "version", "file_name", "metadata_source", "status", "available", "media_type", "result", "episodes"}).AddRow(7, 2, test.filename, "tmdb", "pending", true, "tv", nil, `[]`))
			mock.ExpectBegin()
			mock.ExpectExec("UPDATE t_share_media_file SET status").WithArgs("failed", sqlmock.AnyArg(), round2PrefixArgument(test.reason), 9, 2).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT media_id FROM t_share_media_file").WithArgs(9).WillReturnRows(sqlmock.NewRows([]string{"media_id"}).AddRow(nil))
			mock.ExpectExec("DELETE FROM t_share_media_file_episode").WithArgs(9).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectCommit()
			service := NewShareRecordService(dao.NewShareRecordDAO(db), tmdb, nil, nil)
			identified, err := service.identifyOne(WithRecognitionRound(context.Background()), domain.ShareMedia{ID: 9, ShareID: 7, Version: 2}, false)
			if err != nil || identified {
				t.Fatalf("identified=%v err=%v", identified, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestRound2MovieYearStrict 验证电影年份仍严格限制在文件年份正负一年内。
func TestRound2MovieYearStrict(t *testing.T) {
	for _, year := range []int{2023, 2024, 2025, 2026, 2027} {
		t.Run(fmt.Sprint(year), func(t *testing.T) {
			service := round2TVService(t, nil)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/search/movie" {
					fmt.Fprintf(writer, `{"results":[{"id":42,"title":"Movie","release_date":"%d-01-01"}]}`, year)
					return
				}
				fmt.Fprint(writer, `{"results":[],"translations":[]}`)
			}))
			defer server.Close()
			service.baseURL, service.httpClient = server.URL, server.Client()
			result, err := service.IdentifyShareFile(context.Background(), "Movie.2025.mkv", "tmdb", "movie")
			want := year >= 2024 && year <= 2026
			if err != nil || result.Success != want {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

// TestRound2SpecialSkipColdAndRedis 验证冷识别与跨轮缓存均保留零集，且不污染普通集。
func TestRound2SpecialSkipColdAndRedis(t *testing.T) {
	service := round2TVService(t, []round2TVFixture{{1920, "双峰", "Twin Peaks", "1990-04-08", "2017-09-03", "1990-04-08", "Season 1", 1}})
	for _, episode := range []string{"00", "01", "00"} {
		result, err := service.IdentifyShareFile(context.Background(), "Twin.Peaks.S01E"+episode+".1990.mkv", "tmdb", "auto")
		if err != nil || result == nil || result.Success != (episode == "01") || result.SeasonNumber != 1 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		if episode == "00" {
			data, err := json.Marshal(result)
			if err != nil || !strings.Contains(string(data), `"episode_number":0`) || !strings.HasPrefix(result.FailureReason, "特辑跳过") {
				t.Fatalf("json=%s err=%v", data, err)
			}
		}
	}
}

// TestRound2SpecialSkipPreservesErrors 验证零集检查不会吞掉取消或数据源暂时性故障。
func TestRound2SpecialSkipPreservesErrors(t *testing.T) {
	service := round2TVService(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.IdentifyShareFile(ctx, "Twin.Peaks.S01E00.1990.mkv", "tmdb", "tv"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel err=%v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/search/tv" {
			fmt.Fprint(writer, `{"results":[{"id":1920,"name":"双峰","original_name":"Twin Peaks","first_air_date":"1990-04-08"}]}`)
			return
		}
		http.Error(writer, "mock temporary failure", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	service.baseURL, service.httpClient = server.URL, server.Client()
	if result, err := service.IdentifyShareFile(context.Background(), "Twin.Peaks.S01E00.1990.mkv", "tmdb", "tv"); err == nil || result != nil {
		t.Fatalf("temporary failure became special skip: result=%+v err=%v", result, err)
	}
}
