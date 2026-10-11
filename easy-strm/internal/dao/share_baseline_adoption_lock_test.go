package dao

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestShareBaselineAdoptionOutputWriterRaceRejectedBeforeAck(test *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	defer database.Close()
	mock.ExpectBegin()
	for _, ancestor := range []string{"/", "/isolated"} {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_xact_lock_shared(hashtextextended($1,34981))")).WithArgs(ancestor).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_xact_lock(hashtextextended($1,34981))")).WithArgs("/isolated/output").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))
	mock.ExpectRollback()
	if _, err := NewShareExportCheckpointDAO(database).AdoptBaseline(context.Background(), adoptionRequest(), "fingerprint", "task"); err == nil {
		test.Fatal("must reject concurrent output writer")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}
