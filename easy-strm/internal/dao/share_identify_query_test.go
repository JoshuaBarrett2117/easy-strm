package dao

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestShareIdentifySQLFilters(t *testing.T) {
	for _, tc := range []struct {
		name, filter    string
		pending, failed bool
	}{{"pending", `f.status IN \('pending',''\)`, true, false}, {"failed", `f.status='failed'`, false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			db, m, _ := sqlmock.New()
			defer db.Close()
			m.ExpectQuery(`SELECT .*FROM t_share_record s JOIN t_share_media_file f ON f.share_id=s.id WHERE f.available AND NOT s.share_cancelled AND f.status NOT IN \('ignored','masked'\) AND s.id=ANY\(\$1\) AND f.id=ANY\(\$2\) AND `+tc.filter).WithArgs("{7}", "{11}").WillReturnRows(sqlmock.NewRows([]string{"id", "media_type", "fid", "name", "source", "status", "result", "version"}).AddRow(7, "tv", 11, "Show S01E01.mkv", "tmdb", tc.name, nil, 3))
			rows, err := NewShareRecordDAO(db).ListIdentifyMedia(context.Background(), ShareIdentifyFilter{RecordIDs: []int{7}, MediaIDs: []int{11}, PendingOnly: tc.pending, FailedOnly: tc.failed})
			if err != nil || len(rows) != 1 || rows[0].Media[0].Version != 3 {
				t.Fatalf("rows=%+v err=%v", rows, err)
			}
			if err = m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
