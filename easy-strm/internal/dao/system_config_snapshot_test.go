package dao

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func expectHotConfigRows(mock sqlmock.Sqlmock, value string) {
	mock.ExpectQuery("SELECT config_key,config_val FROM t_system_config WHERE config_key=ANY").WithArgs(`{"global_api_enabled","global_api_key"}`).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow("global_api_enabled", "true").AddRow("global_api_key", value))
}

func TestSystemConfigSnapshotInFlightWriteCannotRestoreRevokedKey(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	defer InvalidateSystemConfigCache(database)
	oldDatabase := DB
	DB = database
	defer func() { DB = oldDatabase }()
	mock.MatchExpectationsInOrder(false)
	mock.ExpectQuery("SELECT config_key,config_val").WillDelayFor(60 * time.Millisecond).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow("global_api_key", "旧密钥"))
	mock.ExpectExec("INSERT INTO t_system_config").WithArgs("global_api_key", "新密钥").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT config_key,config_val").WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow("global_api_key", "新密钥"))
	keys := []string{"global_api_key"}
	done := make(chan map[string]string, 1)
	failed := make(chan error, 1)
	go func() {
		values, err := ReadSystemConfigSnapshot(context.Background(), database, keys)
		done <- values
		failed <- err
	}()
	deadline := time.After(time.Second)
	for {
		hotSystemConfigs.Lock()
		entry := hotSystemConfigs.entries[systemConfigSnapshotKey{database: database, keys: "global_api_key"}]
		hotSystemConfigs.Unlock()
		if entry != nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("配置查询未开始")
		case <-time.After(time.Millisecond):
		}
	}
	if err = NewSystemConfigDAO().Upsert("global_api_key", "新密钥"); err != nil {
		t.Fatal(err)
	}
	select {
	case values := <-done:
		if err = <-failed; err != nil || values["global_api_key"] != "新密钥" {
			t.Fatalf("在途旧查询污染吊销后快照：%v %v", values, err)
		}
	case <-time.After(time.Second):
		t.Fatal("配置查询未结束")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSystemConfigSnapshotCopyAndInvalidation(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	defer InvalidateSystemConfigCache(database)
	keys := []string{"global_api_enabled", "global_api_key"}
	expectHotConfigRows(mock, "旧密钥")
	first, err := ReadSystemConfigSnapshot(context.Background(), database, keys)
	if err != nil {
		t.Fatal(err)
	}
	first["global_api_key"] = "外部修改"
	second, err := ReadSystemConfigSnapshot(context.Background(), database, keys)
	if err != nil || second["global_api_key"] != "旧密钥" {
		t.Fatal("配置快照泄露了可变 map")
	}
	InvalidateSystemConfigCache(database)
	expectHotConfigRows(mock, "新密钥")
	third, err := ReadSystemConfigSnapshot(context.Background(), database, keys)
	if err != nil || third["global_api_key"] != "新密钥" {
		t.Fatal("密钥吊销未即时生效")
	}
	InvalidateSystemConfigCache(database)
	mock.ExpectQuery("SELECT config_key,config_val").WillReturnError(errors.New("模拟查询故障"))
	if stale, err := ReadSystemConfigSnapshot(context.Background(), database, keys); err == nil || stale != nil {
		t.Fatal("配置故障不得使用旧密钥放行")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = ReadSystemConfigSnapshot(ctx, database, keys); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消错误未保留：%v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
