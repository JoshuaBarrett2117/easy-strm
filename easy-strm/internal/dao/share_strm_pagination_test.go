package dao

import (
	"context"
	"database/sql/driver"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

var strmPaginationColumns = []string{"id", "share_id", "share_name", "work_key", "url", "password", "file_name", "remote_id", "result", "episodes"}

func strmPaginationRows(ids []int) *sqlmock.Rows {
	rows := sqlmock.NewRows(strmPaginationColumns)
	for _, id := range ids {
		rows.AddRow(id, 9, "分享", "work", "url", "code", fmt.Sprintf("video%d.mkv", id), "remote", `{"success":true,"media_type":"tv","_media_id":42,"_file_version":3,"_share_version":4,"_file_size":12345,"_available":true}`, `[{"season_number":1,"episode_number":2}]`)
	}
	return rows
}

func assertStrmPaginationSQL(t *testing.T, query, ending string) {
	t.Helper()
	if regexp.MustCompile(`(?i)count\s*\(\s*\*\s*\)\s*over\s*\(`).MatchString(query) {
		t.Error("来源查询不能物化 COUNT OVER")
	}
	if !strings.HasSuffix(query, ending) {
		t.Errorf("游标和分页顺序不稳定: %s", query)
	}
	for _, filter := range []string{"f.available", "NOT s.share_cancelled", "f.status='identified'"} {
		if !strings.Contains(query, filter) {
			t.Errorf("丢失来源过滤: %s", filter)
		}
	}
}

func TestShareStrmPaginationPages(t *testing.T) {
	for _, mode := range []string{"library", "work_fast_path", "work_direct"} {
		for _, count := range []int{0, 7, 100, 101} {
			t.Run(fmt.Sprintf("%s/%d", mode, count), func(t *testing.T) {
				db, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				store := NewShareRecordDAO(db)
				query, args := buildShareStrmSourcesQuery(domain.ShareLibraryQuery{MediaType: "tv"}, 11)
				values := []driver.Value{args[0], args[1]}
				if mode != "library" {
					query, values = shareStrmWorkSourcesSQL, []driver.Value{"work", 11}
				}
				assertStrmPaginationSQL(t, query, "AND f.id>$2 ORDER BY f.id LIMIT 101")
				ids := make([]int, count)
				for index := range ids {
					ids[index] = 12 + index*3
				}
				mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(values...).WillReturnRows(strmPaginationRows(ids)).RowsWillBeClosed()
				var sources []domain.ShareStrmSource
				if mode == "work_direct" {
					sources, err = store.StrmWorkSources(context.Background(), "work", 11)
				} else {
					filter := domain.ShareLibraryQuery{MediaType: "tv"}
					if mode == "work_fast_path" {
						filter = domain.ShareLibraryQuery{WorkKey: "work"}
					}
					sources, err = store.StrmSources(context.Background(), filter, 11)
				}
				if err != nil || len(sources) != min(count, 100) {
					t.Fatalf("来源页: len=%d err=%v", len(sources), err)
				}
				for index, source := range sources {
					if source.ID != ids[index] || source.Remaining != 0 || source.MediaID != 42 || source.FileVersion != 3 || source.ShareVersion != 4 || source.FileSize != 12345 || !source.Available || source.Episodes[0].EpisodeNumber != 2 {
						t.Fatalf("映射或前瞻泄漏: %+v", source)
					}
				}
				if err = mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestShareStrmPaginationGapIDs(t *testing.T) {
	for _, work := range []bool{false, true} {
		t.Run(fmt.Sprint(work), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			ids := make([]int, 205)
			for index := range ids {
				ids[index] = 7 + index*13
			}
			filter := domain.ShareLibraryQuery{}
			if work {
				filter.WorkKey = "work"
			}
			for start := 0; start <= len(ids); start = min(start+100, len(ids)) {
				after := 0
				if start > 0 {
					after = ids[start-1]
				}
				query, _ := buildShareStrmSourcesQuery(filter, after)
				values := []driver.Value{after}
				if work {
					query, values = shareStrmWorkSourcesSQL, []driver.Value{"work", after}
				}
				mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(values...).WillReturnRows(strmPaginationRows(ids[start:min(start+101, len(ids))])).RowsWillBeClosed()
				if start == len(ids) {
					break
				}
			}
			store := NewShareRecordDAO(db)
			got := []int{}
			for after := 0; ; {
				page, readErr := store.StrmSources(context.Background(), filter, after)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if len(page) == 0 {
					break
				}
				for _, source := range page {
					got = append(got, source.ID)
					after = source.ID
				}
			}
			if !reflect.DeepEqual(got, ids) {
				t.Fatalf("游标丢失或重复来源: %v", got)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShareStrmPaginationAllFilters(t *testing.T) {
	minimum, maximum := 6.5, 9.5
	filter := domain.ShareLibraryQuery{WorkKey: "work", Keyword: "title", TmdbID: 42, MediaType: "tv", YearMin: 2000, YearMax: 2026, RatingMin: &minimum, RatingMax: &maximum, Genres: "1,2", Countries: "cn,us", Available: true, FileIDs: []int{4, 8}}
	query, _ := buildShareStrmSourcesQuery(filter, 17)
	assertStrmPaginationSQL(t, query, "AND f.id>$12 ORDER BY f.id LIMIT 101")
	for _, fragment := range []string{"work_key=$1", "title ILIKE '%'||$2||'%' OR original_title ILIKE '%'||$2||'%'", "tmdb_id=$3", "library_media_type=$4", "media_year >= $5", "media_year <= $6", "rating >= $7", "rating <= $8", "genre_ids && string_to_array($9,',')::integer[]", "country_codes && string_to_array($10,',')", "AND available)", "selected_file.id=ANY($11::integer[])"} {
		if !strings.Contains(query, fragment) {
			t.Errorf("筛选条件或参数顺序丢失: %s", fragment)
		}
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs("work", "title", 42, "tv", 2000, 2026, minimum, maximum, "1,2", "CN,US", "{4,8}", 17).WillReturnRows(strmPaginationRows(nil)).RowsWillBeClosed()
	if _, err = NewShareRecordDAO(db).StrmSources(context.Background(), filter, 17); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareStrmPaginationErrors(t *testing.T) {
	fault := errors.New("来源读取失败")
	for _, scenario := range []string{"query", "scan", "result", "identity", "episodes", "row", "lookahead_row"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			expected := mock.ExpectQuery(regexp.QuoteMeta(shareStrmWorkSourcesSQL)).WithArgs("work", 0)
			rows := strmPaginationRows([]int{1, 2})
			switch scenario {
			case "query":
				expected.WillReturnError(fault)
			case "scan":
				rows = sqlmock.NewRows([]string{"id"}).AddRow(1)
			case "result", "identity", "episodes":
				raw, episodes := `{"success":true}`, `[]`
				if scenario == "result" {
					raw = `{`
				} else if scenario == "identity" {
					raw = `{"_media_id":"invalid"}`
				} else {
					episodes = `{`
				}
				rows = sqlmock.NewRows(strmPaginationColumns).AddRow(1, 9, "分享", "work", "url", "", "video.mkv", "remote", raw, episodes)
			case "row":
				rows.RowError(1, fault)
			case "lookahead_row":
				ids := make([]int, 101)
				for index := range ids {
					ids[index] = index + 1
				}
				rows = strmPaginationRows(ids).RowError(100, fault)
			}
			if scenario != "query" {
				expected.WillReturnRows(rows).RowsWillBeClosed()
			}
			if _, err = NewShareRecordDAO(db).StrmWorkSources(context.Background(), "work", 0); err == nil {
				t.Fatal("未上报来源读取错误")
			} else if (scenario == "query" || scenario == "row" || scenario == "lookahead_row") && !errors.Is(err, fault) {
				t.Fatal(err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShareStrmPaginationSingleFile(t *testing.T) {
	assertStrmPaginationSQL(t, shareStrmFileSourceSQL, "AND f.id=$1")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(shareStrmFileSourceSQL)).WithArgs(7).WillReturnRows(strmPaginationRows([]int{7})).RowsWillBeClosed()
	source, err := NewShareRecordDAO(db).GetStrmSource(context.Background(), 7)
	if err != nil || source.ID != 7 || source.MediaID != 42 || source.Remaining != 0 {
		t.Fatalf("单文件映射: %+v %v", source, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
