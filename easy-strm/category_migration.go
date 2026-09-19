package main

import _ "embed"

//go:embed migrations/migrate_v37_other_media_categories.sql
var otherMediaCategoriesMigrationSQL string

// migrateOtherMediaCategories 为已有数据库补充通用的其他电影和其他剧集分类。
func migrateOtherMediaCategories() error {
	_, err := db.Exec(otherMediaCategoriesMigrationSQL)
	return err
}
