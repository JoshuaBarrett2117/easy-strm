package dao

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestIdentifyBatchEpisodesAtomic(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[fail], func(t *testing.T) {
			db, m, _ := sqlmock.New()
			defer db.Close()
			m.ExpectBegin()
			m.ExpectExec("UPDATE t_share_media_file SET status").WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectQuery("SELECT media_id").WillReturnRows(sqlmock.NewRows([]string{"media_id"}).AddRow(nil))
			m.ExpectQuery("INSERT INTO t_share_media").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11))
			m.ExpectExec("UPDATE t_share_media_file SET media_id").WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec("DELETE FROM t_share_media_file_episode").WillReturnResult(sqlmock.NewResult(0, 0))
			insert := m.ExpectExec(`INSERT INTO t_share_media_file_episode.*VALUES\(\$1,\$2,\$3\),\(\$4,\$5,\$6\)`).WithArgs(7, 1, 1, 7, 1, 2)
			if fail {
				insert.WillReturnError(errors.New("insert failed"))
				m.ExpectRollback()
			} else {
				insert.WillReturnResult(sqlmock.NewResult(0, 2))
				m.ExpectCommit()
			}
			err := NewShareRecordDAO(db).Identify(context.Background(), domain.ShareMedia{ID: 7, Version: 2}, "identified", &domain.TmdbIdentifyResult{Success: true, Title: "Show", MediaType: "tv", TmdbID: 1}, "", domain.ShareEpisode{SeasonNumber: 1, EpisodeNumber: 1}, domain.ShareEpisode{SeasonNumber: 1, EpisodeNumber: 2})
			if (err != nil) != fail {
				t.Fatalf("err=%v", err)
			}
			if err = m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
