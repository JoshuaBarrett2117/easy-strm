package dao

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestShareWorkReexportDAOExactReadOnlyQueries(test *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	defer database.Close()
	store := NewShareRecordDAO(database)
	for _, work := range []string{"", " "} {
		if _, err = store.WorkReexportSelections(context.Background(), work); err == nil {
			test.Fatal("空作品必须在查询前拒绝")
		}
	}
	if strings.Contains(shareWorkReexportSelectionsSQL, "m.work_key=s.work_key") || !strings.Contains(shareWorkReexportSelectionsSQL, "m.id=f.media_id") || !strings.Contains(shareWorkReexportSelectionsSQL, "WHERE s.work_key=$1") || !strings.Contains(shareWorkReexportSelectionsSQL, shareSelectionLivePredicate) {
		test.Fatal("必须点查实际候选媒体并复用原有效成员谓词")
	}
	columns := []string{"key", "work", "season", "episode", "candidate", "mode", "revision", "exported", "path", "title", "valid", "source", "media"}
	mock.ExpectQuery(regexp.QuoteMeta(shareWorkReexportSelectionsSQL)).WithArgs("tmdb:tv:1").WillReturnRows(sqlmock.NewRows(columns).AddRow("tmdb:tv:1:1:1", "tmdb:tv:1", 1, 1, 11, "manual", 3, 2, "tv/stable.strm", "标题", true, 7, 99))
	values, err := store.WorkReexportSelections(context.Background(), "tmdb:tv:1")
	if err != nil || len(values) != 1 || values[0].MediaID != 99 || values[0].SourceID != 7 || values[0].Selection.CandidateID != 11 {
		test.Fatalf("%+v %v", values, err)
	}
	failure := errors.New("只读查询失败")
	mock.ExpectQuery(regexp.QuoteMeta(shareWorkReexportSelectionsSQL)).WithArgs("tmdb:tv:1").WillReturnError(failure)
	if _, err = store.WorkReexportSelections(context.Background(), "tmdb:tv:1"); !errors.Is(err, failure) {
		test.Fatal(err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(shareWorkReexportSelectionsSQL)).WithArgs("tmdb:tv:1").WillReturnRows(sqlmock.NewRows(columns).AddRow("key", "work", 1, 1, 11, "manual", 3, 2, "path", "title", true, 7, 99).RowError(0, failure))
	if _, err = store.WorkReexportSelections(context.Background(), "tmdb:tv:1"); !errors.Is(err, failure) {
		test.Fatal(err)
	}
	checkpoint := NewShareExportCheckpointDAO(database)
	mock.ExpectQuery(regexp.QuoteMeta(shareBaselineCountsSQL)).WithArgs("/isolated/output").WillReturnRows(sqlmock.NewRows([]string{"selections", "outputs", "pending", "dirty", "revisions", "invalid"}).AddRow(68, 68, 27, 50, 68, 68))
	invalid, err := checkpoint.WorkReexportGlobalInvalid(context.Background(), "/isolated/output")
	if err != nil || invalid != 68 {
		test.Fatalf("全局计数必须原样复用：invalid=%d err=%v", invalid, err)
	}
	connection, err := database.Conn(context.Background())
	if err != nil {
		test.Fatal(err)
	}
	defer connection.Close()
	mock.ExpectQuery(regexp.QuoteMeta(shareBaselineCountsSQL)).WithArgs("/isolated/output").WillReturnError(failure)
	if _, err = checkpoint.ForOutputConnection(connection).WorkReexportGlobalInvalid(context.Background(), "/isolated/output"); !errors.Is(err, failure) {
		test.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}

func TestShareWorkReexportSourceUsesActualMediaID(test *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery(regexp.QuoteMeta(shareStrmFileSourceSQL)).WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"id", "share", "name", "work", "url", "password", "file", "remote", "result", "episodes"}).AddRow(7, 9, "分享", "tmdb:movie:1", "https://115.com/s/example", "", "Movie.mkv", "remote", `{"success":true,"media_type":"movie","title":"电影","_media_id":99,"_available":true}`, `[]`))
	source, err := NewShareRecordDAO(database).GetStrmSource(context.Background(), 7)
	if err != nil || source.MediaID != 99 || source.ID != 7 {
		test.Fatalf("必须使用文件关联的实际媒体，而非同作品旧行：%+v %v", source, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}

func TestShareWorkReexportScopeDAOReadOnlyParameterizedPreflight(test *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	defer database.Close()
	connection, err := database.Conn(context.Background())
	if err != nil {
		test.Fatal(err)
	}
	defer connection.Close()
	store := NewShareExportCheckpointDAO(database).ForOutputConnection(connection)
	entries := []domain.ShareStrmEntry{{ID: "shared-mapping", MediaID: 99}, {ID: "shared-mapping", MediaID: 99}}
	selections := []domain.ShareSelection{{ItemKey: "tv:1:1", RelativePath: "tv/one.strm"}, {ItemKey: "tv:1:2", RelativePath: "tv/two.strm"}}
	works := []string{"tmdb:tv:1", `work'key`}
	if regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|LOCK|SET|CREATE|ALTER|DROP)\b`).MatchString(shareWorkReexportPreflightSQL) || strings.Contains(shareWorkReexportPreflightSQL, "payload") {
		test.Fatal("范围预检必须只读且不能根据旧映射 payload 拒绝修复")
	}
	for _, required := range []string{
		"unnest($1::text[],$2::bigint[],$3::text[],$4::text[],$5::text[])",
		"COUNT(DISTINCT media_id)>1", "output.state='active' AND output.share_strm_id=target.mapping_id",
		"output.state<>'missing' AND output.output_path=target.output_path",
		"selection.work_key IS NULL OR NOT(selection.work_key=ANY($6::text[]))",
		"file.id=candidate.source_file_id", "file.media_id IS DISTINCT FROM target.media_id",
		"selection.stable_relative_path=target.relative_path", "selection.media_item_key<>target.item_key",
		"output.export_key<>target.item_key", "history.owner_key<>'share:default'", "file.strm_config_id<>-1",
	} {
		if !strings.Contains(shareWorkReexportPreflightSQL, required) {
			test.Fatalf("缺失范围/实际媒体护栏：%s", required)
		}
	}
	expectQuery := func() *sqlmock.ExpectedQuery {
		return mock.ExpectQuery(regexp.QuoteMeta(shareWorkReexportPreflightSQL)).WithArgs(
			pq.Array([]string{"shared-mapping", "shared-mapping"}), pq.Array([]int64{99, 99}),
			pq.Array([]string{"tv:1:1", "tv:1:2"}), pq.Array([]string{"/isolated/tv/one.strm", "/isolated/tv/two.strm"}),
			pq.Array([]string{"tv/one.strm", "tv/two.strm"}), pq.Array(works))
	}
	for _, collision := range []bool{true, false} {
		expectQuery().WillReturnRows(sqlmock.NewRows([]string{"collision"}).AddRow(collision))
		err = store.WorkReexportPreflight(context.Background(), "/isolated", works, entries, selections)
		if (collision && !errors.Is(err, ErrShareExportChanged)) || (!collision && err != nil) {
			test.Fatalf("共享多集不能被误判为一对一，冲突必须拒绝：collision=%v err=%v", collision, err)
		}
	}
	failure := errors.New("预检读取失败")
	expectQuery().WillReturnError(failure)
	if err = store.WorkReexportPreflight(context.Background(), "/isolated", works, entries, selections); !errors.Is(err, failure) {
		test.Fatal(err)
	}
	expectQuery().WillReturnRows(sqlmock.NewRows([]string{"collision"}).AddRow("not-a-bool"))
	if err = store.WorkReexportPreflight(context.Background(), "/isolated", works, entries, selections); err == nil {
		test.Fatal("非法读取结果不能放行")
	}
	for _, invalid := range []struct {
		entries    []domain.ShareStrmEntry
		selections []domain.ShareSelection
	}{
		{nil, nil}, {entries, selections[:1]}, {[]domain.ShareStrmEntry{{}}, selections[:1]},
		{make([]domain.ShareStrmEntry, domain.ShareWorkReexportMaxSelections+1), make([]domain.ShareSelection, domain.ShareWorkReexportMaxSelections+1)},
	} {
		if err = store.WorkReexportPreflight(context.Background(), "/isolated", works, invalid.entries, invalid.selections); err == nil {
			test.Fatal("空或超限目标必须查询前拒绝")
		}
	}
	if err = NewShareExportCheckpointDAO(database).WorkReexportPreflight(context.Background(), "/isolated", works, entries, selections); err == nil {
		test.Fatal("预检不能绕过持有 owner 锁的连接")
	}
	if err = store.WorkReexportPreflight(context.Background(), "/isolated", nil, entries, selections); err == nil {
		test.Fatal("空范围不能进入查询")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}
