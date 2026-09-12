package main

import _ "embed"

//go:embed migrations/migrate_v22_share_record_media.sql
var shareRecordMigrationSQL string

//go:embed migrations/migrate_v23_share_media_type.sql
var shareMediaTypeMigrationSQL string

//go:embed migrations/migrate_v24_share_cancelled.sql
var shareCancelledMigrationSQL string

//go:embed migrations/migrate_v25_share_auto_type.sql
var shareAutoTypeMigrationSQL string

//go:embed migrations/migrate_v26_share_library.sql
var shareLibraryMigrationSQL string

//go:embed migrations/migrate_v29_share_strm.sql
var shareStrmMigrationSQL string

//go:embed migrations/migrate_v30_filename_recognition_rules.sql
var filenameRecognitionMigrationSQL string

//go:embed migrations/migrate_v31_share_strm_file.sql
var shareStrmFileMigrationSQL string

//go:embed migrations/migrate_v32_share_media_entity.sql
var shareMediaEntityMigrationSQL string

//go:embed migrations/migrate_v33_share_media_master.sql
var shareMediaMasterMigrationSQL string

//go:embed migrations/migrate_v34_strm_export.sql
var strmExportMigrationSQL string

// migrateShareRecords 原子迁移旧版单文件分享记录；脚本随二进制分发。
func migrateShareRecords() error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 旧脚本依赖文件字段，拆分完成后禁止重放到媒体实体表。
	var split bool
	if err = tx.QueryRow("SELECT to_regclass('t_share_media_file') IS NOT NULL").Scan(&split); err != nil {
		return err
	}
	if !split {
		if _, err = tx.Exec(shareRecordMigrationSQL); err != nil {
			return err
		}
		if _, err = tx.Exec(shareMediaTypeMigrationSQL); err != nil {
			return err
		}
		if _, err = tx.Exec(shareCancelledMigrationSQL); err != nil {
			return err
		}
		if _, err = tx.Exec(shareAutoTypeMigrationSQL); err != nil {
			return err
		}
		if _, err = tx.Exec(shareLibraryMigrationSQL); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(shareStrmMigrationSQL); err != nil {
		return err
	}
	if _, err = tx.Exec(filenameRecognitionMigrationSQL); err != nil {
		return err
	}
	if _, err = tx.Exec(shareStrmFileMigrationSQL); err != nil {
		return err
	}
	var globalMaster bool
	if split {
		if err = tx.QueryRow("SELECT NOT EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid='t_share_media'::regclass AND attname='share_id' AND NOT attisdropped)").Scan(&globalMaster); err != nil {
			return err
		}
	}
	if !globalMaster {
		if _, err = tx.Exec(shareMediaEntityMigrationSQL); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(shareMediaMasterMigrationSQL); err != nil {
		return err
	}
	if _, err = tx.Exec(strmExportMigrationSQL); err != nil {
		return err
	}
	return tx.Commit()
}
