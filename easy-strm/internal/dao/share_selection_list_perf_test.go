package dao

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

const frozenSelectionScalarColumns = `s.media_item_key,s.work_key,s.season_number,s.episode_number,s.selected_candidate_id,s.selection_mode,s.selection_revision,s.exported_revision,s.stable_relative_path,s.source_kind,s.external_ref`
const frozenSelectionValidSQL = `
 s.source_kind='share' AND EXISTS(SELECT 1 FROM t_share_export_candidate_item i JOIN t_share_export_candidate c USING(candidate_id)
 JOIN t_share_media_file f ON f.id=c.source_file_id JOIN t_share_record r ON r.id=f.share_id JOIN t_share_media live ON live.id=f.media_id
 WHERE i.candidate_id=s.selected_candidate_id AND i.work_key=s.work_key AND i.season_number=s.season_number AND i.episode_number=s.episode_number
 AND NOT i.revoked AND c.available AND c.state='active' AND f.available AND NOT r.share_cancelled AND f.status='identified' AND live.work_key=i.work_key
 AND ((live.media_type='movie' AND i.season_number=0 AND i.episode_number=0) OR (live.media_type='tv' AND EXISTS(SELECT 1 FROM t_share_media_file_episode e WHERE e.file_id=f.id AND e.season_number=i.season_number AND e.episode_number=i.episode_number))))`
const frozenSelectionColumns = frozenSelectionScalarColumns + `,COALESCE(m.title,s.work_key),` + frozenSelectionValidSQL
const frozenSelectionFilter = ` FROM t_share_media_selection s LEFT JOIN t_share_media m ON m.work_key=s.work_key WHERE ($1='' OR strpos(lower(COALESCE(m.title,'')||s.media_item_key),lower($1))>0)`
const frozenSelectionOrder = ` ORDER BY s.work_key,s.season_number,s.episode_number`
const frozenSelectionOffset = frozenSelectionOrder + ` LIMIT $2 OFFSET $3`
const frozenSelectionCursor = ` AND (s.work_key,s.season_number,s.episode_number)>($3,$4,$5)` + frozenSelectionOrder + ` LIMIT $2`

func frozenSelectionListSQL(cursor bool) string {
	suffix := frozenSelectionOffset
	if cursor {
		suffix = frozenSelectionCursor
	}
	return `SELECT ` + frozenSelectionColumns + frozenSelectionFilter + suffix
}

// TestSelectionListCancellation 验证开始前及计数、分页请求中取消后不返回部分结果。
func TestSelectionListCancellation(test *testing.T) {
	for _, stage := range []string{"before_begin", "count", "query"} {
		test.Run(stage, func(test *testing.T) {
			database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				test.Fatal(err)
			}
			defer database.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			if stage == "before_begin" {
				cancel()
			} else {
				mock.ExpectBegin()
				if stage == "count" {
					mock.ExpectQuery(`SELECT count(*)` + frozenSelectionFilter).WithArgs("").WillDelayFor(time.Second).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				} else {
					mock.ExpectQuery(`SELECT count(*)` + frozenSelectionFilter).WithArgs("").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
					mock.ExpectQuery(expectedSelectionListSQL(false)).WithArgs("", 2, 0).WillDelayFor(time.Second).WillReturnRows(selectionListMockRows())
				}
				mock.ExpectRollback()
			}
			values, total, err := NewShareRecordDAO(database).ListSelections(ctx, domain.ShareSelectionQuery{Page: 1, PageSize: 2})
			if values != nil || total != 0 || err == nil {
				test.Fatalf("values=%v total=%d err=%v", values, total, err)
			}
			if stage == "before_begin" && !errors.Is(err, context.Canceled) {
				test.Fatal(err)
			}
			if stage != "before_begin" && !errors.Is(err, sqlmock.ErrCancelled) && !errors.Is(err, context.DeadlineExceeded) {
				test.Fatal(err)
			}
			deadline := time.Now().Add(time.Second)
			for mock.ExpectationsWereMet() != nil && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

func expectedSelectionListSQL(cursor bool) string {
	suffix := frozenSelectionOffset
	if cursor {
		suffix = frozenSelectionCursor
	}
	return `WITH selection_page AS MATERIALIZED (SELECT ` + frozenSelectionScalarColumns + `,COALESCE(m.title,s.work_key) AS title` + frozenSelectionFilter + suffix +
		`) SELECT ` + frozenSelectionScalarColumns + `,s.title,` + frozenSelectionValidSQL + ` FROM selection_page s` + frozenSelectionOrder
}

func selectionListMockRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"key", "work", "season", "episode", "candidate", "mode", "revision", "exported", "path", "kind", "ref", "title", "valid"})
}

// TestSelectionListMaterializesPage 验证分页先物化，原有效性表达式及详情投影逐字保留。
func TestSelectionListMaterializesPage(test *testing.T) {
	if shareSelectionColumns != frozenSelectionColumns {
		test.Fatal("detail/export projection changed from frozen HEAD SQL")
	}
	for _, scenario := range []struct {
		name  string
		query domain.ShareSelectionQuery
		args  []driver.Value
		empty bool
	}{
		{"first_page", domain.ShareSelectionQuery{Page: 1, PageSize: 2}, []driver.Value{"", 2, 0}, false},
		{"deep_offset", domain.ShareSelectionQuery{Page: 1000000, PageSize: 100}, []driver.Value{"", 100, 99999900}, false},
		{"literal_keyword", domain.ShareSelectionQuery{Page: 2, PageSize: 20, Keyword: `%_\'Film`}, []driver.Value{`%_\'Film`, 20, 20}, false},
		{"cursor_count", domain.ShareSelectionQuery{Cursor: true, PageSize: 2, AfterWork: "work", AfterSeason: 1, AfterEpisode: 2}, []driver.Value{"", 2, "work", 1, 2}, false},
		{"cursor_skip_count", domain.ShareSelectionQuery{Cursor: true, SkipCount: true, PageSize: 2, Keyword: "%_", AfterWork: "work", AfterSeason: 1, AfterEpisode: 2}, []driver.Value{"%_", 2, "work", 1, 2}, false},
		{"offset_skip_count", domain.ShareSelectionQuery{SkipCount: true, Page: 3, PageSize: 2}, []driver.Value{"", 2, 4}, false},
		{"empty_page", domain.ShareSelectionQuery{Page: 99, PageSize: 2}, []driver.Value{"", 2, 196}, true},
		{"empty_cursor", domain.ShareSelectionQuery{Cursor: true, PageSize: 2, AfterWork: "zzz"}, []driver.Value{"", 2, "zzz", 0, 0}, true},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				test.Fatal(err)
			}
			defer database.Close()
			mock.ExpectBegin()
			expectedTotal := 0
			if !scenario.query.SkipCount {
				expectedTotal = 123
				mock.ExpectQuery(`SELECT count(*)` + frozenSelectionFilter).WithArgs(scenario.query.Keyword).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(expectedTotal))
			}
			rows := selectionListMockRows()
			expected := []domain.ShareSelection{}
			if !scenario.empty {
				rows.AddRow("w:1:2", "w", 1, 2, 9, "manual", 3, 2, "tv/ep.strm", "share", "", "Title", false).
					AddRow("w:1:3", "w", 1, 3, 10, "auto", 4, 4, "tv/next.strm", "share", "", "Title", true)
				expected = append(expected,
					domain.ShareSelection{SourceKind: "share", ItemKey: "w:1:2", WorkKey: "w", Season: 1, Episode: 2, CandidateID: 9, Mode: "manual", Revision: 3, ExportedRevision: 2, RelativePath: "tv/ep.strm", Title: "Title", Valid: false},
					domain.ShareSelection{SourceKind: "share", ItemKey: "w:1:3", WorkKey: "w", Season: 1, Episode: 3, CandidateID: 10, Mode: "auto", Revision: 4, ExportedRevision: 4, RelativePath: "tv/next.strm", Title: "Title", Valid: true})
			}
			mock.ExpectQuery(expectedSelectionListSQL(scenario.query.Cursor)).WithArgs(scenario.args...).WillReturnRows(rows).RowsWillBeClosed()
			mock.ExpectCommit()
			actual, total, err := NewShareRecordDAO(database).ListSelections(context.Background(), scenario.query)
			if err != nil || total != expectedTotal || !reflect.DeepEqual(actual, expected) {
				test.Fatalf("rows=%+v total=%d err=%v; want=%+v total=%d", actual, total, err, expected, expectedTotal)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

// TestSelectionListFailures 验证各失败阶段释放行与事务，保留原始错误和空结果语义。
func TestSelectionListFailures(test *testing.T) {
	for _, stage := range []string{"begin", "count", "count_scan", "query", "scan", "rows", "close", "commit"} {
		test.Run(stage, func(test *testing.T) {
			database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				test.Fatal(err)
			}
			defer database.Close()
			failure := errors.New("selection " + stage + " failed")
			begin := mock.ExpectBegin()
			if stage == "begin" {
				begin.WillReturnError(failure)
			} else {
				count := mock.ExpectQuery(`SELECT count(*)` + frozenSelectionFilter).WithArgs("")
				if stage == "count" {
					count.WillReturnError(failure)
				} else if stage == "count_scan" {
					count.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow("not an integer"))
				} else {
					count.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7))
					query := mock.ExpectQuery(expectedSelectionListSQL(false)).WithArgs("", 2, 0)
					if stage == "query" {
						query.WillReturnError(failure)
					} else {
						rows := selectionListMockRows()
						if stage == "scan" {
							rows.AddRow("first", "work", 0, 0, 1, "auto", 1, 0, "first.strm", "share", "", "title", true).
								AddRow("key", "work", "not an integer", 0, 1, "auto", 1, 0, "", "share", "", "title", true)
						} else if stage == "rows" {
							rows.AddRow("first", "work", 0, 0, 1, "auto", 1, 0, "first.strm", "share", "", "title", true).
								AddRow("key", "work", 0, 1, 1, "auto", 1, 0, "", "share", "", "title", true).RowError(1, failure)
						} else if stage == "close" {
							rows.CloseError(failure)
						}
						query.WillReturnRows(rows).RowsWillBeClosed()
					}
				}
				if stage == "commit" {
					mock.ExpectCommit().WillReturnError(failure)
				} else {
					mock.ExpectRollback()
				}
			}
			values, total, err := NewShareRecordDAO(database).ListSelections(context.Background(), domain.ShareSelectionQuery{Page: 1, PageSize: 2})
			if err == nil {
				test.Fatal("missing failure")
			}
			if stage == "scan" || stage == "count_scan" {
				if !strings.Contains(err.Error(), "converting driver.Value") {
					test.Fatal(err)
				}
			} else if !errors.Is(err, failure) {
				test.Fatal(err)
			}
			if stage == "commit" {
				if values == nil || total != 7 {
					test.Fatalf("commit failure semantics changed: %v %d", values, total)
				}
			} else if values != nil || total != 0 {
				test.Fatalf("partial result on failure: %v %d", values, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}
