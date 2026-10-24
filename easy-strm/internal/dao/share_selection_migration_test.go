package dao

import (
	"os"
	"strings"
	"testing"
)

func TestSelection115MigrationOfflineContract(t *testing.T) {
	for _, name := range []string{"migrate_v51_share_selection_strm115.sql", "migrate_v51_share_selection_strm115_rollback.sql"} {
		data, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		script := string(data)
		for _, required := range []string{"BEGIN;", "SET LOCAL lock_timeout = '5s';", "SET LOCAL statement_timeout = '30s';", "SET ROLE easy_strm_app;", "to_regclass('t_share_media_selection')", "LOCK TABLE t_share_media_selection IN ACCESS EXCLUSIVE MODE;", "pg_attribute", "RAISE EXCEPTION", "COMMIT;\nRESET ROLE;"} {
			if !strings.Contains(script, required) {
				t.Errorf("%s missing %s", name, required)
			}
		}
		for _, forbidden := range []string{"ALTER OWNER", "CREATE INDEX", "CREATE TABLE", "GRANT ", "DELETE FROM", "UPDATE t_share_media_selection"} {
			if strings.Contains(script, forbidden) {
				t.Errorf("%s contains %s", name, forbidden)
			}
		}
		if strings.HasSuffix(name, "_rollback.sql") {
			if strings.Index(script, "LOCK TABLE") > strings.Index(script, "IF source_column THEN") {
				t.Fatal("rollback must lock before inspecting selections")
			}
			for _, required := range []string{"IF source_column THEN", "ELSIF external_column THEN", "EXECUTE 'SELECT EXISTS", "selected_candidate_id IS NULL", "DROP COLUMN IF EXISTS", "SET NOT NULL"} {
				if !strings.Contains(script, required) {
					t.Errorf("rollback missing %s", required)
				}
			}
		} else {
			for _, required := range []string{"ADD COLUMN IF NOT EXISTS source_kind text NOT NULL DEFAULT 'share'", "ADD COLUMN IF NOT EXISTS external_ref text NOT NULL DEFAULT ''", "DROP NOT NULL", "confmatchtype='s'", "convalidated", "constraint_row.conkey", "constraint_row.confkey", "pg_get_constraintdef", "IS DISTINCT FROM", "has_column_privilege", "CHECK ((source_kind='share' AND selected_candidate_id IS NOT NULL) OR (source_kind='strm115' AND external_ref<>''))"} {
				if !strings.Contains(script, required) {
					t.Errorf("forward missing %s", required)
				}
			}
		}
	}
}
