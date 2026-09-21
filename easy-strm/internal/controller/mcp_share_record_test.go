package controller

import (
	"context"
	"encoding/json"
	"testing"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"github.com/DATA-DOG/go-sqlmock"
)

func newMCPShareRegistry(t *testing.T) (*MCPRegistry, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := service.NewShareRecordService(dao.NewShareRecordDAO(db), nil, nil, nil)
	return NewCoreMCPRegistry(MCPDependencies{ShareRecordService: s}), mock
}

func TestMCPShareRecordToolsRegistered(t *testing.T) {
	r, _ := newMCPShareRegistry(t)
	for _, name := range []string{"share_records_list", "share_record_get", "share_record_create", "share_record_update", "share_record_delete"} {
		if _, ok := r.tools[name]; !ok {
			t.Fatalf("tool not registered: %s", name)
		}
	}
}

func TestMCPShareRecordReadsSanitizePassword(t *testing.T) {
	r, mock := newMCPShareRegistry(t)
	mock.ExpectQuery("SELECT count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT s.id,s.media_type").WillReturnRows(sqlmock.NewRows([]string{"id", "media_type", "name", "url", "password", "note", "version", "created_at", "updated_at", "share_cancelled", "file_count", "identified", "failed", "pending", "unavailable", "media_count", "ignored"}).AddRow(7, "movie", "name", "https://115.com/s/abc", "secret", "", 1, "now", "now", false, 0, 0, 0, 0, 0, 0, 0))
	result, err := r.Call(context.Background(), "share_records_list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	page := result.(map[string]interface{})
	if got := page["data"].([]domain.ShareRecord)[0].Password; got != "" {
		t.Fatalf("password leaked: %q", got)
	}

	mock.ExpectQuery("SELECT count").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT s.id,s.media_type").WithArgs(7, 1, 0).WillReturnRows(sqlmock.NewRows([]string{"id", "media_type", "name", "url", "password", "note", "version", "created_at", "updated_at", "share_cancelled", "media_id", "file_name", "metadata_source", "status", "result", "error", "media_version"}).AddRow(7, "movie", "name", "https://115.com/s/abc", "secret", "", 1, "now", "now", false, nil, nil, nil, nil, nil, nil, nil))
	result, err = r.Call(context.Background(), "share_record_get", json.RawMessage(`{"share_id":7}`))
	if err != nil || result.(domain.ShareRecord).Password != "" {
		t.Fatalf("get failed or leaked password: %#v %v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMCPShareRecordWritesRequireConfirmationAndCRUD(t *testing.T) {
	r, mock := newMCPShareRegistry(t)
	for _, name := range []string{"share_record_create", "share_record_update", "share_record_delete"} {
		result, err := r.Call(context.Background(), name, json.RawMessage(`{}`))
		if err != nil || result.(map[string]interface{})["confirmation_required"] != true {
			t.Fatalf("%s did not require confirmation: %#v %v", name, result, err)
		}
	}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_share_record").WithArgs("name", "https://115.com/s/abc", "secret", "", "movie").WillReturnRows(sqlmock.NewRows([]string{"id", "version", "created_at", "updated_at"}).AddRow(8, 1, "now", "now"))
	mock.ExpectCommit()
	created, err := r.Call(context.Background(), "share_record_create", json.RawMessage(`{"name":"name","url":"https://115.com/s/abc","password":"secret","media_type":"movie","confirm":true}`))
	if err != nil || created.(domain.ShareRecord).Password != "" {
		t.Fatalf("create failed: %#v %v", created, err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE t_share_record SET").WithArgs("new", "https://115.com/s/abc", "next", "", 8, 1, "tv").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(2))
	mock.ExpectCommit()
	updated, err := r.Call(context.Background(), "share_record_update", json.RawMessage(`{"share_id":8,"name":"new","url":"https://115.com/s/abc","password":"next","media_type":"tv","version":1,"confirm":true}`))
	if err != nil || updated.(domain.ShareRecord).Password != "" || updated.(domain.ShareRecord).Version != 2 {
		t.Fatalf("update failed: %#v %v", updated, err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM t_share_record").WithArgs(8).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM t_share_media").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	deleted, err := r.Call(context.Background(), "share_record_delete", json.RawMessage(`{"share_id":8,"confirm":true}`))
	if err != nil || deleted.(map[string]interface{})["deleted"] != true {
		t.Fatalf("delete failed: %#v %v", deleted, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
