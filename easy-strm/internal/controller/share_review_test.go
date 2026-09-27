package controller

import (
	"easy-strm/internal/dao"
	"easy-strm/internal/service"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestShareReviewValidation(t *testing.T) {
	r := gin.New()
	r.GET("/review", NewShareRecordController(nil).ListReviewItems)
	for _, q := range []string{"page=x", "page=0", "page_size=101", "share_id=x", "share_id=-1", "media_id=x", "status=identified"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/review?"+q, nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d", q, w.Code)
		}
	}
}

func TestShareReviewResponseAndFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := gin.New()
	r.GET("/review", NewShareRecordController(service.NewShareRecordService(dao.NewShareRecordDAO(db), nil, nil, nil)).ListReviewItems)
	mock.ExpectQuery("SELECT count").WithArgs(`{"failed","pending"}`).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	mock.ExpectQuery("SELECT COALESCE").WithArgs(`{"failed","pending"}`, 20, 0).WillReturnRows(sqlmock.NewRows([]string{"data"}).AddRow(`[]`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/review", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Fatal(w.Code, w.Body.String())
	}
	mock.ExpectQuery("SELECT count").WillReturnError(errors.New("private database error"))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/review", nil))
	if w.Code != 500 || strings.Contains(w.Body.String(), "private") {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
