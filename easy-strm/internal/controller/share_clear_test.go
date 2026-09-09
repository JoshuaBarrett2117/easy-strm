package controller

import (
	"database/sql"
	"easy-strm/internal/dao"
	"easy-strm/internal/service"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

// TestClearShareMedia 覆盖路由参数、空分享、删除范围、不存在以及事务失败回滚。
func TestClearShareMedia(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name, id string
		count    int64
		status   int
	}{
		{"参数错误", "bad", 0, 400}, {"非正数", "0", 0, 400}, {"成功", "9", 43, 200}, {"重复清空", "9", 0, 200}, {"不存在", "9", 0, 404}, {"删除失败", "9", 0, 409},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			s := service.NewShareRecordService(dao.NewShareRecordDAO(db), nil, nil, nil)
			r := gin.New()
			r.DELETE("/shares/:id/media", NewShareRecordController(s).ClearMedia)
			if tt.status != 400 {
				mock.ExpectBegin()
				q := mock.ExpectQuery("SELECT id FROM t_share_record WHERE id=\\$1 FOR UPDATE").WithArgs(9)
				if tt.status == 404 {
					q.WillReturnError(sql.ErrNoRows)
					mock.ExpectRollback()
				} else {
					q.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
					e := mock.ExpectExec("DELETE FROM t_share_media WHERE share_id=\\$1").WithArgs(9)
					if tt.status == 409 {
						e.WillReturnError(fmt.Errorf("delete failed"))
						mock.ExpectRollback()
					} else {
						e.WillReturnResult(sqlmock.NewResult(0, tt.count))
						mock.ExpectCommit()
					}
				}
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("DELETE", "/shares/"+tt.id+"/media", nil))
			if w.Code != tt.status {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
