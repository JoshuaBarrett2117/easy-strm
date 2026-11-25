package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"easy-strm/internal/dbmigrate"

	_ "github.com/lib/pq"
)

var db *sql.DB

// buildPostgresDSN 将禁用并行查询作为启动参数，覆盖池扩容、连接替换及其上的事务，不影响其他应用。
func buildPostgresDSN(config *Config) string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable client_encoding=UTF8 max_parallel_workers_per_gather=0",
		quotePostgresDSNValue(config.PostgreSQL.Host),
		config.PostgreSQL.Port,
		quotePostgresDSNValue(config.PostgreSQL.User),
		quotePostgresDSNValue(config.PostgreSQL.Password),
		quotePostgresDSNValue(config.PostgreSQL.Database),
	)
}

// quotePostgresDSNValue 按 lib/pq 规则转义反斜杠和单引号，防止空白或配置值改变连接参数。
func quotePostgresDSNValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	return "'" + value + "'"
}

// InitDB 连接 PostgreSQL 并执行嵌入二进制的版本化迁移（internal/dbmigrate）。
// 结构变更只能通过新增 internal/dbmigrate/migrations/NNNNN_xxx.sql 完成，不要在这里内联 DDL。
func InitDB(config *Config) error {
	connStr := buildPostgresDSN(config)

	Debug("Connecting to database")

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		Error("Failed to open database connection: error_type=%T", err)
		return err
	}

	if err = db.Ping(); err != nil {
		Error("Failed to ping database: error_type=%T", err)
		return err
	}

	if err = dbmigrate.Up(context.Background(), db); err != nil {
		Error("Database migration failed: %v", err)
		return err
	}
	Info("Database initialized successfully")
	return nil
}
