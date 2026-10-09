package dao

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestShareExportReconciliationPreparationAtomic(t *testing.T) {
	for _, scenario := range []string{"success", "changed", "fanout", "marker", "commit"} {
		t.Run(scenario, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			mock.ExpectBegin()
			revision := int64(7)
			if scenario == "changed" {
				revision++
			}
			mock.ExpectQuery(`SELECT config_revision FROM t_share_export_consumer.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(revision))
			if scenario != "changed" {
				fanout := mock.ExpectExec(regexp.QuoteMeta(shareExportFanoutSQL))
				if scenario == "fanout" {
					fanout.WillReturnError(errors.New("fanout"))
				} else {
					fanout.WillReturnResult(sqlmock.NewResult(0, 1))
				}
				if scenario != "fanout" {
					marker := mock.ExpectExec(`UPDATE t_share_export_consumer SET legacy_outputs_reconciled=true,config_fingerprint`).WithArgs("fingerprint")
					if scenario == "marker" {
						marker.WillReturnError(errors.New("marker"))
					} else {
						marker.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
			}
			if scenario == "success" {
				mock.ExpectCommit()
			} else if scenario == "commit" {
				mock.ExpectCommit().WillReturnError(errors.New("commit"))
			} else {
				mock.ExpectRollback()
			}
			err = NewShareExportCheckpointDAO(database).CompleteReconciliation(context.Background(), domain.ShareExportInput{ConfigRevision: 7}, "fingerprint")
			if (err == nil) != (scenario == "success") {
				t.Fatalf("%s: %v", scenario, err)
			}
			if scenario == "changed" && !errors.Is(err, ErrShareExportChanged) {
				t.Fatal(err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
