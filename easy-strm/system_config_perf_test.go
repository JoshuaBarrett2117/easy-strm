package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestRootConfigWriteInvalidatesProxySnapshot(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	oldDatabase := db
	db = database
	defer func() { dao.InvalidateSystemConfigCache(database); db = oldDatabase }()
	expectProxy := func(address string) {
		mock.ExpectQuery("SELECT config_key,config_val").WithArgs(`{"proxy_domains","proxy_url"}`).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow("proxy_url", address).AddRow("proxy_domains", "example.invalid"))
	}
	expectProxy("http://localhost:1001")
	first, domains, err := loadProxyConfigFromSystemContext(context.Background())
	if err != nil || first.String() != "http://localhost:1001" || !shouldUseProxy(first, "example.invalid", domains) {
		t.Fatalf("代理快照读取错误：%v", err)
	}
	if _, _, err = loadProxyConfigFromSystemContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("INSERT INTO t_system_config").WithArgs("proxy_url", "http://localhost:1002").WillReturnRows(sqlmock.NewRows([]string{"id", "key", "value", "created", "updated"}).AddRow(1, "proxy_url", "http://localhost:1002", time.Now(), time.Now()))
	if _, err = UpsertSystemConfig("proxy_url", "http://localhost:1002"); err != nil {
		t.Fatal(err)
	}
	expectProxy("http://localhost:1002")
	second, _, err := loadProxyConfigFromSystemContext(context.Background())
	if err != nil || second.String() != "http://localhost:1002" {
		t.Fatal("根入口保存后仍使用旧代理")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProxyConfigFailureAndCancellationDoNotSendRequest(t *testing.T) {
	transport := &proxyFallbackTransport{
		loadProxyConfigContext: func(ctx context.Context) (*url.URL, []string, error) {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			return nil, nil, errors.New("模拟配置故障")
		},
		direct: proxyRoundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("配置不可读时不得发送请求")
			return nil, nil
		}),
	}
	request, err := http.NewRequest("GET", "http://example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = transport.RoundTrip(request); err == nil || !strings.Contains(err.Error(), "配置故障") {
		t.Fatalf("配置错误被吞掉：%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = transport.RoundTrip(request.WithContext(ctx)); !errors.Is(err, context.Canceled) {
		t.Fatalf("请求取消未传至配置加载：%v", err)
	}
}
