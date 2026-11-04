package dao

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestShareDirectLinkMappingRevisionRead(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := NewShareRecordDAO(database)
	query := regexp.QuoteMeta("SELECT payload, xmin::text || ':' || EXTRACT(EPOCH FROM updated_at)::text FROM t_share_strm WHERE id=$1")
	columns := []string{"payload", "revision"}
	for _, revision := range []string{"100:1793000000.123456", "101:1793000001.123456"} {
		mock.ExpectQuery(query).WithArgs("fixture").WillReturnRows(sqlmock.NewRows(columns).AddRow(`{"id":"fixture","file_id":"file"}`, revision))
		entry, actual, err := store.GetStrmPlaybackEntry(context.Background(), "fixture")
		if err != nil || entry.ID != "fixture" || actual != revision {
			t.Fatal("atomic payload/revision read failed")
		}
	}
	mock.ExpectQuery(query).WithArgs("missing").WillReturnRows(sqlmock.NewRows(columns))
	if _, _, err := store.GetStrmPlaybackEntry(context.Background(), "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("missing mapping semantics lost")
	}
	mock.ExpectQuery(query).WithArgs("malformed").WillReturnRows(sqlmock.NewRows(columns).AddRow(`{`, "revision"))
	if _, _, err := store.GetStrmPlaybackEntry(context.Background(), "malformed"); err == nil {
		t.Fatal("malformed mapping accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := store.GetStrmPlaybackEntry(ctx, "canceled"); !errors.Is(err, context.Canceled) {
		t.Fatal("mapping read ignored cancellation")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareDirectLinkConfigContext(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previous := DB
	DB = database
	t.Cleanup(func() { DB = previous; database.Close() })
	reader := NewSystemConfigDAO()
	query := regexp.QuoteMeta("SELECT id, config_key, config_val, create_time, update_time FROM t_system_config WHERE config_key = $1")
	columns := []string{"id", "config_key", "config_val", "create_time", "update_time"}
	mock.ExpectQuery(query).WithArgs("share_strm_settings").WillReturnRows(sqlmock.NewRows(columns).AddRow(1, "share_strm_settings", "{}", time.Now(), time.Now()))
	if config, err := reader.GetByKeyContext(context.Background(), "share_strm_settings"); err != nil || config == nil || config.ConfigVal != "{}" {
		t.Fatal("contextual config read failed")
	}
	mock.ExpectQuery(query).WithArgs("missing").WillReturnRows(sqlmock.NewRows(columns))
	if config, err := reader.GetByKeyContext(context.Background(), "missing"); err != nil || config != nil {
		t.Fatal("missing config semantics changed")
	}
	failure := errors.New("isolated query failed")
	mock.ExpectQuery(query).WithArgs("failure").WillReturnError(failure)
	if _, err := reader.GetByKeyContext(context.Background(), "failure"); !errors.Is(err, failure) {
		t.Fatal("query error chain lost")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.GetByKeyContext(ctx, "canceled"); !errors.Is(err, context.Canceled) {
		t.Fatal("config read ignored caller cancellation")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
