package dao

import (
	"context"
	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"strings"
	"testing"
)

func TestLibraryCombinedQuery(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	low, high := 7.5, 9.0
	q := domain.ShareLibraryQuery{Keyword: "电影", TmdbID: 100, MediaType: "movie", YearMin: 2020, YearMax: 2026, RatingMin: &low, RatingMax: &high, Genres: "18,35", Countries: "cn,us", Available: true, Sort: "rating", Direction: "desc", Page: 2, PageSize: 24}
	where, args := libraryWhere(q)
	if !strings.Contains(where, "genre_ids &&") || !strings.Contains(where, "country_codes &&") || !strings.HasSuffix(where, "AND available") || len(args) != 9 {
		t.Fatalf("错误的组合条件: %s %v", where, args)
	}
	mock.ExpectQuery(`WITH stats AS[\s\S]*ORDER BY rating DESC NULLS LAST,work_key ASC LIMIT \$10 OFFSET \$11`).WithArgs("电影", 100, "movie", 2020, 2026, low, high, "18,35", "CN,US", 24, 24).WillReturnRows(sqlmock.NewRows([]string{"total", "data"}).AddRow(25, `[{"work_key":"tmdb:movie:100"}]`))
	got, err := NewShareRecordDAO(database).Library(context.Background(), q)
	if err != nil || got.Total != 25 {
		t.Fatalf("%+v %v", got, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLibrarySourcesEmptyPage(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery(`WITH sources AS`).WithArgs("tmdb:tv:1", 20, 40).WillReturnRows(sqlmock.NewRows([]string{"total", "data"}).AddRow(1, `[]`))
	got, err := NewShareRecordDAO(database).LibrarySources(context.Background(), "tmdb:tv:1", 3, 20)
	if err != nil || got.Total != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLibraryTVSeasonStatsAndFiles(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	d := NewShareRecordDAO(database)
	mock.ExpectQuery(`SELECT e.season_number,count\(DISTINCT e.episode_number\),count\(DISTINCT f.id\)`).
		WithArgs("tmdb:tv:1").
		WillReturnRows(sqlmock.NewRows([]string{"season", "episodes", "files"}).AddRow(0, 1, 1).AddRow(1, 2, 2))
	stats, err := d.LibraryTVSeasonStats(context.Background(), "tmdb:tv:1")
	if err != nil || len(stats) != 2 || stats[0].SeasonNumber != 0 || stats[1].MatchedEpisodeCount != 2 {
		t.Fatalf("季统计错误: %+v %v", stats, err)
	}
	mock.ExpectQuery(`SELECT e.episode_number,f.id,f.share_id`).WithArgs("tmdb:tv:1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"episode", "id", "share_id", "remote", "file", "size", "available", "status", "name", "url", "password", "cancelled"}).
			AddRow(1, 10, 7, "remote-10", "Show.S01E01E02.mkv", 1024, true, "identified", "来源一", "https://example.com/1", "code", false).
			AddRow(1, 11, 8, "remote-11", "Show.S01E01.mkv", 2048, false, "identified", "来源二", "https://example.com/2", "", true).
			AddRow(2, 10, 7, "remote-10", "Show.S01E01E02.mkv", 1024, true, "identified", "来源一", "https://example.com/1", "code", false))
	files, err := d.LibraryTVSeasonFiles(context.Background(), "tmdb:tv:1", 1)
	if err != nil || len(files[1]) != 2 || len(files[2]) != 1 || files[1][1].Available || !files[1][1].ShareCancelled {
		t.Fatalf("季文件映射错误: %+v %v", files, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
