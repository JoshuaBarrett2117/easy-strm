package controller

import (
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

			} else if tc.status == 500 {
				mock.ExpectQuery("SELECT url FROM t_share_record").WillReturnError(fmt.Errorf("读取分享失败"))
			}
			s, _ := newControllerOperationService(t, db)
			r := gin.New()
			r.DELETE("/shares/:id", NewShareRecordController(s).Delete)
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
