package service

import (
	"context"
	"errors"
	"testing"

	"easy-strm/internal/dao"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestGlobalAPIKeyCacheRotationDisableAndFailClosed(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	oldDatabase := dao.DB
	dao.DB = database
	defer func() { dao.InvalidateSystemConfigCache(database); dao.DB = oldDatabase }()
	configs := dao.NewSystemConfigDAO()
	service := NewGlobalAPIService(configs)
	expectValues := func(enabled, key string) {
		mock.ExpectQuery("SELECT config_key,config_val").WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(GlobalAPIEnabledKey, enabled).AddRow(GlobalAPIKeyKey, key))
	}
	expectValues(" TRUE ", "旧密钥")
	if valid, err := service.ValidateAPIKey("旧密钥"); err != nil || !valid {
		t.Fatalf("兼容启用值被改变：%v", err)
	}
	if valid, err := service.ValidateAPIKey("错误密钥"); err != nil || valid {
		t.Fatal("错误密钥通过缓存认证")
	}
	mock.ExpectExec("INSERT INTO t_system_config").WithArgs(GlobalAPIKeyKey, "新密钥").WillReturnResult(sqlmock.NewResult(0, 1))
	if err = configs.Upsert(GlobalAPIKeyKey, "新密钥"); err != nil {
		t.Fatal(err)
	}
	expectValues("true", "新密钥")
	if valid, err := service.ValidateAPIKey("旧密钥"); err != nil || valid {
		t.Fatal("已吊销密钥仍可认证")
	}
	if valid, err := service.ValidateAPIKey("新密钥"); err != nil || !valid {
		t.Fatal("新密钥不能认证")
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO t_system_config").WithArgs(GlobalAPIEnabledKey, "false").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err = configs.BatchUpsert(map[string]string{GlobalAPIEnabledKey: "false"}); err != nil {
		t.Fatal(err)
	}
	expectValues("false", "新密钥")
	if valid, err := service.ValidateAPIKey("新密钥"); err != nil || valid {
		t.Fatal("停用 API 未即时生效")
	}
	dao.InvalidateSystemConfigCache(database)
	mock.ExpectQuery("SELECT config_key,config_val").WillReturnError(errors.New("模拟数据库故障"))
	if valid, err := service.ValidateAPIKey("新密钥"); err == nil || valid {
		t.Fatal("数据库故障不得放行认证")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if valid, err := service.ValidateAPIKeyContext(ctx, "新密钥"); valid || !errors.Is(err, context.Canceled) {
		t.Fatalf("取消未保留：%v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
