package main

import (
	"strings"
	"testing"

	"github.com/lib/pq"
)

// TestBuildPostgresDSNDisablesParallelQuery 离线验证连接池使用的 DSN 包含禁用并行查询的启动参数。
func TestBuildPostgresDSNDisablesParallelQuery(t *testing.T) {
	config := &Config{}
	config.PostgreSQL.Host = "localhost"
	config.PostgreSQL.Port = 5432
	config.PostgreSQL.User = "test_user"
	config.PostgreSQL.Password = "test_password"
	config.PostgreSQL.Database = "test_database"

	dsn := buildPostgresDSN(config)
	if !strings.Contains(" "+dsn+" ", " max_parallel_workers_per_gather=0 ") {
		t.Fatalf("missing PostgreSQL startup parameter max_parallel_workers_per_gather=0: %s", dsn)
	}
	if _, err := pq.NewConnector(dsn); err != nil {
		t.Fatalf("lib/pq rejected DSN without connecting: %v", err)
	}
}

// TestBuildPostgresDSNPreservesConnectionSettings 只解析 DSN，不拨号或调用会执行建表的 InitDB。
func TestBuildPostgresDSNPreservesConnectionSettings(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		host     string
		user     string
		password string
		database string
		want     string
	}{
		{
			name: "ordinary",
			host: "localhost", user: "test_user", password: "test_password", database: "test_database",
			want: "host='localhost' port=6543 user='test_user' password='test_password' dbname='test_database' sslmode=disable client_encoding=UTF8 max_parallel_workers_per_gather=0",
		},
		{
			name: "empty-password-ipv6",
			host: "::1", user: "test_user", database: "test_database",
			want: "host='::1' port=6543 user='test_user' password='' dbname='test_database' sslmode=disable client_encoding=UTF8 max_parallel_workers_per_gather=0",
		},
		{
			name: "whitespace-unicode",
			host: "/tmp/pg socket", user: "test user", password: " 测试\t口令\n ", database: "test database",
			want: "host='/tmp/pg socket' port=6543 user='test user' password=' 测试\t口令\n ' dbname='test database' sslmode=disable client_encoding=UTF8 max_parallel_workers_per_gather=0",
		},
		{
			name: "quotes-backslashes",
			host: `/tmp/pg'\socket`, user: `user'\name`, password: `pass'\word\`, database: `db'\name`,
			want: `host='/tmp/pg\'\\socket' port=6543 user='user\'\\name' password='pass\'\\word\\' dbname='db\'\\name' sslmode=disable client_encoding=UTF8 max_parallel_workers_per_gather=0`,
		},
		{
			name: "parameter-like-values",
			host: "localhost sslmode=require", user: "user client_encoding=LATIN1", password: "' max_parallel_workers_per_gather=8", database: "db options=-cwork_mem=1GB",
			want: `host='localhost sslmode=require' port=6543 user='user client_encoding=LATIN1' password='\' max_parallel_workers_per_gather=8' dbname='db options=-cwork_mem=1GB' sslmode=disable client_encoding=UTF8 max_parallel_workers_per_gather=0`,
		},
		{
			name: "empty-values",
			want: "host='' port=6543 user='' password='' dbname='' sslmode=disable client_encoding=UTF8 max_parallel_workers_per_gather=0",
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			config := &Config{}
			config.PostgreSQL.Host = scenario.host
			config.PostgreSQL.Port = 6543
			config.PostgreSQL.User = scenario.user
			config.PostgreSQL.Password = scenario.password
			config.PostgreSQL.Database = scenario.database

			dsn := buildPostgresDSN(config)
			if dsn != scenario.want {
				t.Fatalf("DSN = %q, want %q", dsn, scenario.want)
			}
			if _, err := pq.NewConnector(dsn); err != nil {
				t.Fatalf("lib/pq rejected DSN without connecting: %v", err)
			}
		})
	}
}
