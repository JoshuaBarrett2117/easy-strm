package main

import _ "embed"

//go:embed migrations/migrate_v21_strm_playback_record.sql
var playbackRecordMigrationSQL string

// migratePlaybackRecords 创建 STRM 播放记录表；启动时幂等执行。
func migratePlaybackRecords() error {
	_, err := db.Exec(playbackRecordMigrationSQL)
	return err
}
