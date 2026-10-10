package dao

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestOfflineSnapshotSingleStatementAndBoundary(t *testing.T) {
	matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		for _, required := range []string{"SELECT count(*) FROM filtered", "FROM filtered ORDER BY id DESC LIMIT $3 OFFSET $4", "cloud115_id=$1", "status=$2", "statement_timestamp()"} {
			if !strings.Contains(actual, required) {
				return errors.New("快照过滤、稳定排序或总数输入被改变")
			}
		}
		return nil
	})
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery("快照").WithArgs(7, "failed", 20, 40).WillReturnRows(sqlmock.NewRows([]string{"total", "data", "snapshot_at"}).AddRow(67, `[{"id":24,"cloud115_id":7,"status":"failed","create_time":"创建","update_time":"更新"}]`, "2026-10-11T00:00:00Z"))
	result, err := NewOfflineDownloadTaskDAO(database).SnapshotPage(context.Background(), 7, "failed", 3, 20)
	if err != nil || result.Total != 67 || len(result.Data) != 1 || result.Data[0].ID != 24 || result.Data[0].CreateTime != "创建" {
		t.Fatalf("字段或分页总数被改变：%+v %v", result, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
