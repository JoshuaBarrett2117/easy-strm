package service

import (
	"context"
	"database/sql"
	"testing"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

type shareSyncParser struct {
	response *domain.ParseShareResponse
	err      error
}

func (p shareSyncParser) ParseShareLink(context.Context, string, string) (*domain.ParseShareResponse, error) {
	return p.response, p.err
}

func expectSyncRecord(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT count").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("FROM \\(SELECT .* FROM t_share_record").WithArgs(7, 1, 0).WillReturnRows(sqlmock.NewRows([]string{"id", "media_type", "name", "url", "password", "note", "version", "created_at", "updated_at", "share_cancelled", "mid", "file_name", "source", "status", "result", "error", "mversion"}).AddRow(7, "auto", "分享", "url", "", "", 1, "now", "now", false, nil, nil, nil, nil, nil, nil, nil))
}

func TestSyncShareFilesStoresOnlyRealVideos(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	expectSyncRecord(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM t_share_record").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery("SELECT id,sha1 FROM t_share_media_file").WithArgs(7, "remote-1", "目录/剧集.S01E01E02.mkv").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("INSERT INTO t_share_media_file").WithArgs(7, "remote-1", "目录/剧集.S01E01E02.mkv", int64(10), "sha", "pick", "auto", sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11))
	mock.ExpectExec("UPDATE t_share_media_file SET available=FALSE").WithArgs(7, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM t_share_media m").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	parser := shareSyncParser{response: &domain.ParseShareResponse{Files: []domain.ShareFileInfo{
		{Name: "目录", Path: "目录", IsDir: true, Type: "folder"},
		{Name: "剧集.S01E01E02.mkv", Path: "目录/剧集.S01E01E02.mkv", Fid: "remote-1", Size: 10, Sha1: "sha", PickCode: "pick", Type: "video"},
		{Name: "说明.txt", Path: "目录/说明.txt", Type: "other"},
	}}}
	s := NewShareRecordService(dao.NewShareRecordDAO(db), nil, nil, parser)
	if count, masked, err := s.SyncShareFiles(context.Background(), 7); err != nil || count != 1 || masked != 0 {
		t.Fatalf("同步结果错误: count=%d masked=%d err=%v", count, masked, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeShareEpisodesSupportsMultiEpisode(t *testing.T) {
	got, err := normalizeShareEpisodes("tv", nil, 1, 1, 1, 2, 2)
	if err != nil || len(got) != 2 || got[0].EpisodeNumber != 1 || got[1].EpisodeNumber != 2 {
		t.Fatalf("多集映射错误: %+v %v", got, err)
	}
	if _, err = normalizeShareEpisodes("tv", nil, 1, 0); err == nil {
		t.Fatal("缺少集号的电视剧被标记成功")
	}
	if got, err = normalizeShareEpisodes("movie", []domain.ShareEpisode{{SeasonNumber: 1, EpisodeNumber: 1}}, 0, 0); err != nil || len(got) != 0 {
		t.Fatalf("电影未清空季集: %+v %v", got, err)
	}
}

func TestShareVideoFileFilter(t *testing.T) {
	for _, name := range []string{"电影.mkv", "原盘.ISO", "老片.rmvb", "蓝光/正片.M2TS"} {
		if !isShareVideoFile(name) {
			t.Fatalf("支持的视频文件被过滤: %s", name)
		}
	}
	for _, name := range []string{"电视剧目录", "说明.txt", "海报.jpg"} {
		if isShareVideoFile(name) {
			t.Fatalf("非视频文件被接收: %s", name)
		}
	}
}
