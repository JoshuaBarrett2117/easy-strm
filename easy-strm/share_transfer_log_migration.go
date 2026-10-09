package main

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"time"
)

//go:embed migrations/migrate_v46_share_transfer_log.sql
var shareTransferLogMigrationSQL string

// 原 v14 文件未接入启动链；只嵌入 v46，以有界事务补齐缺表，不运行旧索引或 DBA 专用 v43。
func applyShareTransferLogMigration(ctx context.Context, database *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启转存日志迁移事务失败: %w", err)
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, shareTransferLogMigrationSQL); err != nil {
		return fmt.Errorf("执行转存日志迁移失败: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("提交转存日志迁移失败: %w", err)
	}
	return nil
}
