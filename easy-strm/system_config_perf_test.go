package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestProxyConfigCancellationDoesNotSendRequest(t *testing.T) {
	transport := &proxyFallbackTransport{
		loadProxyConfigContext: func(ctx context.Context) (*url.URL, []string, error) {
			return nil, nil, ctx.Err()
		},
		direct: proxyRoundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("请求取消时不得发送请求")
			return nil, nil
		}),
	}
	request, err := http.NewRequest("GET", "http://example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = transport.RoundTrip(request.WithContext(ctx)); !errors.Is(err, context.Canceled) {
		t.Fatalf("请求取消未传至配置加载：%v", err)
	}
}

func TestProxyConfigSystemSnapshotFailureSendsDirectHTTP(test *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	defer database.Close()
	oldDatabase := db
	db = database
	defer func() { dao.InvalidateSystemConfigCache(database); db = oldDatabase }()
	mock.ExpectQuery("SELECT config_key,config_val").WithArgs(`{"proxy_domains","proxy_url"}`).WillReturnRows(
		sqlmock.NewRows([]string{"key", "value"}).AddRow("proxy_url", "http://unused.invalid:7890").AddRow("proxy_domains", "127.0.0.1"),
	)
	proxyURL, domains, err := loadProxyConfigFromSystemContext(context.Background())
	if err != nil || !shouldUseProxy(proxyURL, "127.0.0.1", domains) {
		test.Fatalf("successful snapshot did not configure proxy: %v", err)
	}
	dao.InvalidateSystemConfigCache(database)
	mock.ExpectQuery("SELECT config_key,config_val").WithArgs(`{"proxy_domains","proxy_url"}`).WillReturnError(errors.New("synthetic snapshot read failure"))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := NewProxyAwareHTTPClient(5 * time.Second)
	defer client.CloseIdleConnections()
	client.Transport.(*proxyFallbackTransport).proxy = proxyRoundTripFunc(func(*http.Request) (*http.Response, error) {
		test.Error("snapshot read failure must not reuse invalidated proxy configuration")
		return nil, errors.New("unexpected proxy request")
	})
	response, err := client.Get(server.URL)
	if err != nil {
		test.Fatalf("production snapshot loader failure must send direct HTTP: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		test.Fatalf("direct status=%d", response.StatusCode)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}
