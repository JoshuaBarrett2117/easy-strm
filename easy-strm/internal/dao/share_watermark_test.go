package dao

import (
	"context"
	"easy-strm/internal/pkg/logger"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestWatermarkFinishAtomicAndSameRevision(t *testing.T) {
	for _, scenario := range []string{"success", "same-revision", "pending", "changed", "mirror-failure", "older-mirror", "commit-failure", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			database, mock, _ := sqlmock.New()
			defer database.Close()
			ctx, cancel := context.WithCancel(logger.WithTaskID(context.Background(), "run-2"))
			defer cancel()
			mock.ExpectBegin()
			current := int64(9)
			if scenario == "changed" {
				current = 10
			}
			mock.ExpectQuery("SELECT config_revision.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(current))
			if scenario != "changed" {
				count := int64(1)
				if scenario == "pending" {
					count = 0
				}
				mock.ExpectExec("UPDATE t_share_export_consumer SET completed_revision").WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, count))
				if count == 1 {
					expect := mock.ExpectExec(regexp.QuoteMeta(shareWatermarkMirrorSQL)).WithArgs("run-2")
					if scenario == "mirror-failure" {
						expect.WillReturnError(errors.New("mirror failed"))
					} else {
						mirrorCount := int64(1)
						if scenario == "older-mirror" {
							mirrorCount = 0
						}
						expect.WillReturnResult(sqlmock.NewResult(0, mirrorCount))
					}
				}
			}
			if scenario == "success" || scenario == "same-revision" {
				mock.ExpectCommit()
			} else if scenario == "commit-failure" {
				mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
			} else if scenario == "cancelled" {
				mock.ExpectCommit().WillReturnError(context.Canceled)
			} else {
				mock.ExpectRollback()
			}
			ready, err := NewShareExportCheckpointDAO(database).FinishBaseline(ctx, 9)
			if (scenario == "success" || scenario == "same-revision") != (ready && err == nil) {
				t.Fatalf("ready=%v err=%v", ready, err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
	if !strings.Contains(shareWatermarkMirrorSQL, "<=") || strings.Contains(shareWatermarkMirrorSQL, "max(id)") {
		t.Fatal("mirror not monotonic revision snapshot")
	}
}

func TestWatermarkGenericStoreHiddenAndImmutable(t *testing.T) {
	store := NewSystemConfigDAO()
	if value, err := store.GetByKey("share_strm_incremental_watermark"); err != nil || value != nil {
		t.Fatalf("value=%v err=%v", value, err)
	}
	if err := store.Upsert("share_strm_incremental_watermark", "x"); err == nil {
		t.Fatal("watermark writable")
	}
	if err := store.BatchUpsert(map[string]string{"share_strm_incremental_watermark": "x"}); err == nil {
		t.Fatal("batch watermark writable")
	}
}
