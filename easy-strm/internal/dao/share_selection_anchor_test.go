package dao

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSelectionPathAnchorFirstWriterExistingAndFailure(t *testing.T) {
	for _, scenario := range []string{"first-writer", "existing-path", "failure"} {
		t.Run(scenario, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			mock.ExpectBegin()
			mock.ExpectExec(regexp.QuoteMeta(shareSelectionLockSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
			stable := ""
			if scenario == "existing-path" {
				stable = "first-title.strm"
			}
			mock.ExpectQuery("SELECT stable_relative_path.*FOR UPDATE").WithArgs("w:0:0").WillReturnRows(sqlmock.NewRows([]string{"path"}).AddRow(stable))
			if stable == "" {
				expectation := mock.ExpectExec("UPDATE t_share_media_selection SET stable_relative_path").WithArgs("w:0:0", "new-title.strm")
				if scenario == "failure" {
					expectation.WillReturnError(errors.New("fixture failure"))
				} else {
					expectation.WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}
			if scenario == "failure" {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			path, err := NewShareRecordDAO(database).AnchorSelectionPath(context.Background(), "w:0:0", "new-title.strm")
			if scenario == "failure" {
				if err == nil {
					t.Fatal("吞掉路径持久化失败")
				}
			} else {
				expected := "new-title.strm"
				if scenario == "existing-path" {
					expected = stable
				}
				if err != nil || path != expected {
					t.Fatalf("path=%s err=%v", path, err)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
