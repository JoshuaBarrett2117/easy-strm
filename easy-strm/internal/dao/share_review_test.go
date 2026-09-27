package dao

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestReviewFiltersAndPagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT count.*f.available AND f.status = ANY.*f.share_id=\$2.*s.name ILIKE \$3.*f.id=\$4`).WithArgs(`{"failed"}`, 7, "%测试%", 9).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT COALESCE.*ORDER BY CASE WHEN f.status='failed' THEN 0 ELSE 1 END, f.updated_at DESC, f.id DESC.*LIMIT \$5 OFFSET \$6`).WithArgs(`{"failed"}`, 7, "%测试%", 9, 10, 10).WillReturnRows(sqlmock.NewRows([]string{"data"}).AddRow(`[{"id":9,"share_id":7,"share_name":"测试","status":"failed","version":2,"episodes":[{"season_number":0,"episode_number":1}]}]`))
	result, err := NewShareRecordDAO(db).ListReviewItems(context.Background(), []string{"failed"}, "测试", 7, 9, 2, 10)
	if err != nil || result.Total != 1 || len(result.Data) != 1 || result.Data[0].Episodes[0].SeasonNumber != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
