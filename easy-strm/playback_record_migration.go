package main

import _ "embed"

//go:embed migrations/migrate_v21_strm_playback_record.sql
var playbackRecordMigrationSQL string

//go:embed migrations/migrate_v35_playback_poster.sql
var playbackPosterMigrationSQL string

// migratePlaybackRecords 创建 STRM 播放记录表；启动时幂等执行。
func migratePlaybackRecords() error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(playbackRecordMigrationSQL); err != nil {
		return err
	}
	if _, err = tx.Exec(playbackPosterMigrationSQL); err != nil {
		return err
	}
	return tx.Commit()
}
