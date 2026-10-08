package main

import (
	"context"
	"database/sql"
	_ "embed"
)

//go:embed migrations/migrate_v43_share_export_checkpoint.sql
var shareExportCheckpointMigrationSQL string

// applyShareExportCheckpointMigration 仅供显式迁移工具和测试调用；生产必须由 DBA 手动应用 SQL。
func applyShareExportCheckpointMigration(ctx context.Context, database *sql.DB) error {
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err = transaction.ExecContext(ctx, shareExportCheckpointMigrationSQL); err != nil {
		return err
	}
	return transaction.Commit()
}
