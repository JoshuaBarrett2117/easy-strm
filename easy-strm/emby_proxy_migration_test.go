package main

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestEmbyProxyMigration(t *testing.T) {
	for _, required := range []string{"ADD COLUMN IF NOT EXISTS proxy_port INTEGER NOT NULL DEFAULT 0", "proxy_port BETWEEN 0 AND 65535", "CREATE UNIQUE INDEX IF NOT EXISTS", "WHERE proxy_port <> 0", "conrelid='t_emby_server'::regclass"} {
		if !strings.Contains(embyProxyMigrationSQL, required) {
			t.Fatalf("迁移缺少约束: %s", required)
		}
	}
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	previous := db
	db = database
	defer func() { db = previous }()
	for range 2 {
		mock.ExpectExec(regexp.QuoteMeta(embyProxyMigrationSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
		if err := migrateEmbyProxyPort(); err != nil {
			t.Fatal(err)
		}
	}
	mock.ExpectExec(regexp.QuoteMeta(embyProxyMigrationSQL)).WillReturnError(errors.New("migration failed"))
	if err := migrateEmbyProxyPort(); err == nil {
		t.Fatal("不能吞掉迁移错误")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
