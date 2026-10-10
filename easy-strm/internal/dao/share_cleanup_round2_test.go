package dao

import (
	"reflect"
	"strings"
	"testing"
)

func approvedCleanupOptions() ShareCleanupOptions {
	return ShareCleanupOptions{Mode: "REBUILD", Scope: "all-share", Apply: true, Confirm: "DESTROY_SHARE_SELECTIONS_AND_DERIVED_RECORDS", ConfirmSelectionsDestroyed: true, Quiesced: true, BackupAttestation: "isolated-fixture"}
}

func TestCleanupExactDBAWhitelistOrderAndPredicates(t *testing.T) {
	expected := []ShareCleanupTable{
		{"t_share_media_selection", "TRUE"},
		{"t_share_export_candidate_item", "TRUE"},
		{"t_share_export_candidate", "TRUE"},
		{"t_share_strm", "TRUE"},
		{"t_strm_export_state", "owner_key='share:default'"},
		{"t_strm_export_history", "owner_key='share:default'"},
		{"t_strm_clear_run", "task_id IN (SELECT task_id FROM t_share_operation_queue WHERE operation='share_clear')"},
		{"t_strm_file", "strm_config_id=-1"},
		{"t_share_export_source_state", "TRUE"},
		{"t_share_export_dirty_work", "TRUE"},
	}
	if actual := ShareCleanupTables("REBUILD"); !reflect.DeepEqual(actual, expected) {
		t.Fatalf("DBA A.1/A.3 whitelist mismatch: %+v", actual)
	}
	script, err := BuildShareCleanupSQL(approvedCleanupOptions())
	if err != nil {
		t.Fatal(err)
	}
	var deletes []string
	for _, line := range strings.Split(script, "\n") {
		if strings.HasPrefix(line, "DELETE FROM ") {
			deletes = append(deletes, line)
		}
	}
	if len(deletes) != len(expected) {
		t.Fatalf("unexpected delete count: %v", deletes)
	}
	for index, table := range expected {
		if deletes[index] != "DELETE FROM "+table.Table+" WHERE "+table.Predicate+";" {
			t.Fatalf("delete %d: %s", index, deletes[index])
		}
	}
	for _, preserved := range []string{"t_share_media_file_episode", "t_share_media_file", "t_share_media", "t_share_record", "t_share_export_recovery_", "t_system_config", "t_media_source", "t_cloud115_cookie_source", "t_cron_task", "t_cron_task_run"} {
		if strings.Contains(strings.Join(deletes, "\n"), "DELETE FROM "+preserved+" ") || (strings.HasSuffix(preserved, "_") && strings.Contains(strings.Join(deletes, "\n"), "DELETE FROM "+preserved)) {
			t.Fatalf("preserved table deleted: %s", preserved)
		}
	}
	for _, required := range []string{"share_export_require_baseline()", "legacy_outputs_reconciled=false"} {
		if !strings.Contains(script, required) {
			t.Fatal(required)
		}
	}
}

func TestCleanupRejectsResetAndEveryMissingConfirmation(t *testing.T) {
	for _, mode := range []string{"RESET", "", "rebuild", "all"} {
		options := approvedCleanupOptions()
		options.Mode = mode
		for _, apply := range []bool{false, true} {
			options.Apply = apply
			if script, err := BuildShareCleanupSQL(options); err == nil || script != "" {
				t.Fatalf("mode %q accepted", mode)
			}
		}
	}
	for _, missing := range []string{"scope", "confirm", "selections", "backup", "quiesced"} {
		t.Run(missing, func(t *testing.T) {
			options := approvedCleanupOptions()
			switch missing {
			case "scope":
				options.Scope = ""
			case "confirm":
				options.Confirm = "incorrect"
			case "selections":
				options.ConfirmSelectionsDestroyed = false
			case "backup":
				options.BackupAttestation = " \t"
			case "quiesced":
				options.Quiesced = false
			}
			if script, err := BuildShareCleanupSQL(options); err == nil || script != "" {
				t.Fatal("unguarded apply generated")
			}
		})
	}
}
