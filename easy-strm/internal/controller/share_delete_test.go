package controller

import (
	"easy-strm/internal/dao"
	"easy-strm/internal/service"
	"fmt"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestDeleteShareRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		id     string
		status int
	}{{"bad", 400}, {"0", 400}, {"7", 200}, {"8", 500}} {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			if tc.status == 200 {
				mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"url"}).AddRow("https://115.com/s/abc"))
				mock.ExpectQuery("SELECT id FROM t_share_strm").WithArgs("abc").WillReturnRows(sqlmock.NewRows([]string{"id"}))
				mock.ExpectBegin()
				mock.ExpectExec("DELETE FROM t_share_record").WithArgs(7).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("DELETE FROM t_share_media").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			} else if tc.status == 500 {
				mock.ExpectQuery("SELECT url FROM t_share_record").WillReturnError(fmt.Errorf("读取分享失败"))
			}
			r := gin.New()
			r.DELETE("/shares/:id", NewShareRecordController(service.NewShareRecordService(dao.NewShareRecordDAO(db), nil, nil, nil)).Delete)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("DELETE", "/shares/"+tc.id, nil))
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
