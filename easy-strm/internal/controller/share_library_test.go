package controller

import (
	"easy-strm/internal/dao"
	"easy-strm/internal/service"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestLibraryControllerValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	c := NewShareRecordController(nil)
	r.GET("/library", c.Library)
	for _, query := range []string{"page=-1", "rating_min=NaN", "rating_max=11", "year_min=2026&year_max=2020", "genres=1,x", "countries=!!", "sort=bad", "tmdb_id=x"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/library?"+query, nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d %s", query, w.Code, w.Body.String())
		}
	}
}

func TestLibraryControllerSuccessAndDBFailure(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	c := NewShareRecordController(service.NewShareRecordService(dao.NewShareRecordDAO(database), nil, nil, nil))
	r := gin.New()
	r.GET("/library", c.Library)
	mock.ExpectQuery(`WITH stats AS`).WithArgs(24, 0).WillReturnRows(sqlmock.NewRows([]string{"total", "data"}).AddRow(0, `[]`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/library", nil))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	mock.ExpectQuery(`WITH stats AS`).WillReturnError(errors.New("数据库暂时不可用"))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/library", nil))
	if w.Code != 500 {
		t.Fatalf("%d", w.Code)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
