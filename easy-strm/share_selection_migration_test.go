package main

import (
	"os"
	"strings"
	"testing"
)

func TestV47ManualMigrationConstraints(t *testing.T) {
	raw, err := os.ReadFile("migrations/migrate_v47_share_source_selection.sql")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, required := range []string{"D4 维护窗口", "RESET 必须先重新扫描", "easy_strm.v47_statement_timeout", "lock_timeout", "statement_timeout", "share_media_selection_member_fk", "ON DELETE RESTRICT", "revoked BOOLEAN", "share_export_candidate_discovery_seq", "e.file_id=f.id", "first_seen_seq,c.candidate_id", "share_export_require_baseline()"} {
		if !strings.Contains(script, required) {
			t.Fatalf("missing %s", required)
		}
	}
	startup, err := os.ReadFile("db.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(startup), "migrate_v47") {
		t.Fatal("v47 wired to startup")
	}
	rollback, err := os.ReadFile("migrations/migrate_v47_share_source_selection_rollback.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rollback), "t_share_export_recovery") || strings.Contains(string(rollback), "DELETE FROM") {
		t.Fatal("rollback touches business data")
	}
}
