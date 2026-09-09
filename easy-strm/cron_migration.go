package main

import _ "embed"

//go:embed migrations/migrate_v27_unified_cron.sql
var unifiedCronSQL string

// migrateUnifiedCron 幂等升级现有定时任务，保留用户配置。
func migrateUnifiedCron() error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(unifiedCronSQL); err != nil {
		return err
	}
	return tx.Commit()
}
