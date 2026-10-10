package controller

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"easy-strm/internal/dao"
	"easy-strm/internal/service"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestShareRecordCursorValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/shares", NewShareRecordController(nil).List)
	for _, query := range []string{"cursor=bad", "after_id=-1", "after_id=x", "after_id=2", "only_unsynced=bad"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/shares?"+query, nil))
		if response.Code != 400 {
			t.Fatalf("%s: %d", query, response.Code)
		}
	}
}

func TestShareRecordCursorNoTotalAndDatabaseFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		database, mock, _ := sqlmock.New()
		router := gin.New()
		router.GET("/shares", NewShareRecordController(service.NewShareRecordService(dao.NewShareRecordDAO(database), nil, nil, nil)).List)
		query := mock.ExpectQuery(`SELECT s.id.*s.id<\$1.*NOT EXISTS.*ORDER BY s.id DESC LIMIT \$2\) s`).WithArgs(700, 200)
		if fail {
			query.WillReturnError(errors.New("mock query failure"))
		} else {
			query.WillReturnRows(sqlmock.NewRows([]string{"id", "type", "name", "url", "password", "note", "version", "created", "updated", "cancelled", "files", "identified", "failed", "pending", "unavailable", "media", "ignored"}))
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/shares?cursor=true&after_id=700&only_unsynced=true&page_size=200", nil))
		if (!fail && (response.Code != 200 || !strings.Contains(response.Body.String(), `"total":0`))) || (fail && response.Code != 500) {
			t.Fatalf("%d %s", response.Code, response.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		database.Close()
	}
}
