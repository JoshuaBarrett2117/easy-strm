package main

import _ "embed"

//go:embed migrations/migrate_v36_identify_trace.sql
var identifyTraceMigrationSQL string

// migrateIdentifyTrace 为已有识别缓存补充来源与AI追踪字段，脚本可重复执行。
func migrateIdentifyTrace() error {
	_, err := db.Exec(identifyTraceMigrationSQL)
	return err
}
