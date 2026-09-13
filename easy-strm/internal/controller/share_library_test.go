package controller

import (
	"database/sql"
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

func TestLibraryTVControllers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	c := NewShareRecordController(service.NewShareRecordService(dao.NewShareRecordDAO(database), nil, nil, nil))
	r := gin.New()
	r.GET("/tv-detail", c.LibraryTVDetail)
	r.GET("/tv-seasons", c.LibraryTVSeason)
	for _, target := range []string{"/tv-detail", "/tv-seasons?work_key=x", "/tv-seasons?work_key=x&season_number=-1", "/tv-seasons?work_key=x&season_number=bad"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d %s", target, w.Code, w.Body.String())
		}
	}
	mock.ExpectQuery(`SELECT work_key,tmdb_id,title,media_type,metadata_source`).WithArgs("missing").WillReturnError(errors.New("query failed"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/tv-detail?work_key=missing", nil))
	if w.Code != 500 {
		t.Fatalf("数据库错误状态码: %d", w.Code)
	}
	mock.ExpectQuery(`SELECT work_key,tmdb_id,title,media_type,metadata_source`).WithArgs("absent").WillReturnError(sql.ErrNoRows)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/tv-detail?work_key=absent", nil))
	if w.Code != 404 {
		t.Fatalf("作品不存在状态码: %d", w.Code)
	}
	mock.ExpectQuery(`SELECT work_key,tmdb_id,title,media_type,metadata_source`).WithArgs("movie").
		WillReturnRows(sqlmock.NewRows([]string{"work_key", "tmdb_id", "title", "media_type", "metadata_source"}).AddRow("movie", 1, "电影", "movie", "tmdb"))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/tv-detail?work_key=movie", nil))
	if w.Code != 400 {
		t.Fatalf("电影误用状态码: %d %s", w.Code, w.Body.String())
	}
	mock.ExpectQuery(`SELECT work_key,tmdb_id,title,media_type,metadata_source`).WithArgs("tv").
		WillReturnRows(sqlmock.NewRows([]string{"work_key", "tmdb_id", "title", "media_type", "metadata_source"}).AddRow("tv", nil, "剧集", "tv", "metatube"))
	mock.ExpectQuery(`SELECT e.season_number,count\(DISTINCT e.episode_number\)`).WithArgs("tv").
		WillReturnRows(sqlmock.NewRows([]string{"season", "episodes", "files"}))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/tv-detail?work_key=tv", nil))
	if w.Code != 200 {
		t.Fatalf("成功响应状态码: %d %s", w.Code, w.Body.String())
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
