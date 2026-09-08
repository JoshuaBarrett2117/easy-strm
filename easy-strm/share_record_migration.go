package main

import _ "embed"

//go:embed migrations/migrate_v22_share_record_media.sql
var shareRecordMigrationSQL string

//go:embed migrations/migrate_v23_share_media_type.sql
var shareMediaTypeMigrationSQL string

// migrateShareRecords 原子迁移旧版单文件分享记录；脚本随二进制分发。
func migrateShareRecords() error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(shareRecordMigrationSQL); err != nil {
		return err
	}
	if _, err = tx.Exec(shareMediaTypeMigrationSQL); err != nil {
		return err
	}
	return tx.Commit()
}
