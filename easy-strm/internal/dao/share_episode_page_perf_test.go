package dao

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestShareEpisodeJSONOnlyAggregatesPageAndKeepsTotal(t *testing.T) {
	matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		for _, required := range []string{"page AS MATERIALIZED", "LIMIT $2 OFFSET $3", "JOIN page p ON p.id=e.file_id GROUP BY e.file_id", "COALESCE(episodes.episodes,'[]'::json)"} {
			if !strings.Contains(actual, required) {
				return fmt.Errorf("缺少本页聚合或空集兼容：%s", required)
			}
		}
		return nil
	})
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	dao := NewShareRecordDAO(database)
	mock.ExpectQuery("文件页").WithArgs(7, 20, 40).WillReturnRows(sqlmock.NewRows([]string{"total", "data"}).AddRow(700, `[{"id":44,"available":false,"status":"failed","episodes":[]},{"id":45,"episodes":[{"season_number":0,"episode_number":1},{"season_number":1,"episode_number":2}]}]`))
	files, err := dao.ListFiles(context.Background(), 7, 3, 20)
	if err != nil || files.Total != 700 || len(files.Data) != 2 || files.Data[0].Available || len(files.Data[0].Episodes) != 0 || len(files.Data[1].Episodes) != 2 || files.Data[1].Episodes[0].SeasonNumber != 0 {
		t.Fatalf("文件、特别篇或总数契约改变：%+v %v", files, err)
	}
	mock.ExpectQuery("来源页").WithArgs("作品", 20, 200).WillReturnRows(sqlmock.NewRows([]string{"total", "data"}).AddRow(100, `[]`))
	sources, err := dao.LibrarySources(context.Background(), "作品", 11, 20)
	raw, marshalErr := json.Marshal(sources.Data)
	if err != nil || marshalErr != nil || sources.Total != 100 || string(raw) != "[]" {
		t.Fatalf("越界页必须保留精确总数：%+v %v", sources, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
