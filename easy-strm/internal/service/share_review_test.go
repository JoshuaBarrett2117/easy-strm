package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestReviewManualSaveConflictAndEpisodes(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		s := NewShareRecordService(dao.NewShareRecordDAO(db), nil, nil, nil)
		m := domain.ShareMedia{ID: 9, ShareID: 7, Version: 2, FileName: "Show.S00E01E02.mkv", Result: &domain.TmdbIdentifyResult{TmdbID: 123, Title: "Show", MediaType: "tv"}}
		if s.ManualIdentify(context.Background(), m) == nil {
			t.Fatal("missing episodes accepted")
		}
		m.Episodes = []domain.ShareEpisode{{SeasonNumber: 0, EpisodeNumber: 1}, {SeasonNumber: 0, EpisodeNumber: 2}}
		mock.ExpectQuery("SELECT f.share_id,f.version,f.file_name").WithArgs(9).WillReturnRows(sqlmock.NewRows([]string{"share_id", "version", "file_name", "metadata_source", "status", "available", "media_type", "result", "episodes"}).AddRow(7, 2, "Show.S00E01E02.mkv", "tmdb", "failed", true, "tv", nil, `[]`))
		mock.ExpectBegin()
		count := int64(1)
		if conflict {
			count = 0
		}
		mock.ExpectExec("UPDATE t_share_media_file SET status").WithArgs("identified", sqlmock.AnyArg(), "", 9, 2).WillReturnResult(sqlmock.NewResult(0, count))
		if conflict {
			mock.ExpectRollback()
		} else {
			mock.ExpectQuery("SELECT media_id").WithArgs(9).WillReturnRows(sqlmock.NewRows([]string{"media_id"}).AddRow(nil))
			mock.ExpectQuery("INSERT INTO t_share_media").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(15))
			mock.ExpectExec("UPDATE t_share_media_file SET media_id").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("DELETE FROM t_share_media_file_episode").WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec("INSERT INTO t_share_media_file_episode").WillReturnResult(sqlmock.NewResult(0, 1))

			mock.ExpectCommit()
		}
		err = s.ManualIdentify(context.Background(), m)
		if (err != nil) != conflict {
			t.Fatalf("conflict=%v: %v", conflict, err)
		}
		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}

func TestReviewParsesPendingEpisodes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT count").WithArgs(`{"failed","pending"}`).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery("SELECT COALESCE").WithArgs(`{"failed","pending"}`, 20, 0).WillReturnRows(sqlmock.NewRows([]string{"data"}).AddRow(`[{"id":1,"file_name":"Example.S00E02.mkv","media_type":"auto","status":"pending"}]`))
	s := NewShareRecordService(dao.NewShareRecordDAO(db), NewTmdbService("", nil), nil, nil)
	result, err := s.ListReviewItems(context.Background(), "", "", 0, 0, 1, 20)
	if err != nil || len(result.Data) != 1 || len(result.Data[0].Episodes) != 1 || result.Data[0].Episodes[0].EpisodeNumber != 2 || result.Data[0].MediaType != "tv" {
		t.Fatalf("%+v %v", result, err)
	}
	for _, status := range []string{"identified", "failed,invalid"} {
		if _, err := s.ListReviewItems(context.Background(), status, "", 0, 0, 1, 20); err == nil {
			t.Fatal("invalid status accepted")
		}
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
