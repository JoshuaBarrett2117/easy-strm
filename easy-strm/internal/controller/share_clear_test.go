package controller

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

// TestClearShareMedia 覆盖提交契约、错误参数、目标不存在及队列持久化失败。
func TestClearShareMedia(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, id string
		status   int
	}{{"参数错误", "bad", 400}, {"非正数", "0", 400}, {"接受任务", "9", 200}, {"不存在", "9", 404}, {"持久化失败", "9", 409}} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			s, store := newControllerOperationService(t, db)
			if tc.status != 400 {
				q := mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(9)
				if tc.status == 404 {
					q.WillReturnError(sql.ErrNoRows)
				} else {
					q.WillReturnRows(sqlmock.NewRows([]string{"url"}).AddRow("https://115.com/s/example"))
				}
			}
			if tc.status == 409 {
				store.err = errors.New("队列保存失败")
			}
			r := gin.New()
			r.DELETE("/shares/:id/media", NewShareRecordController(s).ClearMedia)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("DELETE", "/shares/"+tc.id+"/media", nil))
			if w.Code != tc.status {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if w.Code == 200 {
				var result struct {
					Data map[string]interface{} `json:"data"`
				}
				if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil {
					t.Fatal(e)
				}
				checkAcceptedOperation(t, result.Data)
			}
			if e := mock.ExpectationsWereMet(); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestClearSelectedShareMedia(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	defer db.Close()
	s, store := newControllerOperationService(t, db)
	for _, id := range []int{2, 3} {
		mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"url"}).AddRow("https://115.com/s/example"))
	}
	r := gin.New()
	r.POST("/shares/batch-clear", NewShareRecordController(s).ClearSelectedMedia)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/shares/batch-clear", bytes.NewBufferString(`{"share_ids":[3,2,2]}`)))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	store.mu.Lock()
	ids := store.row.ShareIDs
	store.mu.Unlock()
	if len(ids) != 2 || ids[0] != 2 || ids[1] != 3 {
		t.Fatal(ids)
	}
	bad := httptest.NewRecorder()
	r.ServeHTTP(bad, httptest.NewRequest("POST", "/shares/batch-clear", bytes.NewBufferString(`{"share_ids":[]}`)))
	if bad.Code != 400 {
		t.Fatal(bad.Code)
	}
	if e := mock.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestClearAllShareMedia(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	defer db.Close()
	s, store := newControllerOperationService(t, db)
	mock.ExpectQuery("SELECT id FROM t_share_record ORDER BY id").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(9).WillReturnRows(sqlmock.NewRows([]string{"url"}).AddRow("https://115.com/s/example"))
	r := gin.New()
	r.DELETE("/shares/media", NewShareRecordController(s).ClearAllMedia)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("DELETE", "/shares/media", nil))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	store.mu.Lock()
	ids := store.row.ShareIDs
	store.mu.Unlock()
	if len(ids) != 1 || ids[0] != 9 {
		t.Fatal(ids)
	}
	if e := mock.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
