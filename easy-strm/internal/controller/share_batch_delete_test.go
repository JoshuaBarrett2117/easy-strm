package controller

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// TestBatchDeleteShareRecords 验证批量删除的输入错误及200、404、500响应，不执行真实清理。
func TestBatchDeleteShareRecords(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"missing", `{}`, 400}, {"empty", `{"share_ids":[]}`, 400},
		{"null_list", `{"share_ids":null}`, 400}, {"null_item", `{"share_ids":[7,null]}`, 400},
		{"zero", `{"share_ids":[0]}`, 400}, {"negative", `{"share_ids":[-1]}`, 400},
		{"string", `{"share_ids":["7"]}`, 400}, {"fraction", `{"share_ids":[1.5]}`, 400},
		{"object", `{"share_ids":{}}`, 400}, {"malformed", `{"share_ids":`, 400},
		{"accepted", `{"share_ids":[8,7,8]}`, 200}, {"not_found", `{"share_ids":[7,8]}`, 404},
		{"read_failure", `{"share_ids":[7]}`, 500}, {"enqueue_failure", `{"share_ids":[7]}`, 500},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			s, store := newControllerOperationService(t, db)
			if test.status != 400 {
				query := mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(7)
				if test.name == "read_failure" {
					query.WillReturnError(errors.New("读取失败"))
				} else {
					query.WillReturnRows(sqlmock.NewRows([]string{"url"}).AddRow("https://115.com/s/example"))
				}
				if test.status == 200 || test.status == 404 {
					query = mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(8)
					if test.status == 404 {
						query.WillReturnError(sql.ErrNoRows)
					} else {
						query.WillReturnRows(sqlmock.NewRows([]string{"url"}).AddRow("https://115.com/s/example"))
					}
				}
				if test.name == "enqueue_failure" {
					store.err = errors.New("队列不可用")
				}
			}
			router := gin.New()
			router.POST("/media/share-records/batch-delete", NewShareRecordController(s).BatchDelete)
			request := httptest.NewRequest("POST", "/media/share-records/batch-delete", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.status == 400 && !strings.Contains(response.Body.String(), "请至少选择一个分享") {
				t.Fatal(response.Body.String())
			}
			if test.status == 200 {
				var payload struct {
					Data map[string]interface{} `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				checkAcceptedOperation(t, payload.Data)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
