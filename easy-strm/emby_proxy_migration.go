package main

import _ "embed"

//go:embed migrations/migrate_v41_emby_proxy_port.sql
var embyProxyMigrationSQL string

func migrateEmbyProxyPort() error {
	_, err := db.Exec(embyProxyMigrationSQL)
	return err
}
