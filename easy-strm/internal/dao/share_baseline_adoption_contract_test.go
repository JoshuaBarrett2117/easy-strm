package dao

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"strings"
	"testing"
	"time"
)

type baselineAuditArgument struct {
	allowance   int
	attestation string
	earliest    time.Time
}

func (argument baselineAuditArgument) Match(value driver.Value) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	var receipt map[string]interface{}
	if json.Unmarshal([]byte(text), &receipt) != nil {
		return false
	}
	request, ok := receipt["request"].(map[string]interface{})
	if !ok {
		return false
	}
	confirmedAt, ok := receipt["confirmed_at"].(string)
	if !ok {
		return false
	}
	confirmedTime, err := time.Parse(time.RFC3339Nano, confirmedAt)
	if err != nil || confirmedTime.IsZero() || confirmedTime.Before(argument.earliest) || confirmedTime.After(time.Now().UTC()) {
		return false
	}
	return receipt["task_id"] == "task" && receipt["confirmed_by"] == float64(1) && request["manual_confirmation"] == "人工确认采纳" && request["reason"] == "独立核验后确认" && request["dba_attestation"] == "DBA-review-42" && request["expected"] != nil && request["allow_known_benign_invalid"] == float64(argument.allowance) && request["benign_attestation"] == argument.attestation
}

func TestShareBaselineAdoptionBenignRecheckSQLContract(test *testing.T) {
	predicate := " AND (mapping.payload->>'media_id')::integer=live.id"
	if strings.Count(shareBaselineCountsSQL, predicate) != 1 || shareBaselineBenignCountsSQL != strings.Replace(shareBaselineCountsSQL, predicate, "", 1) {
		test.Fatal("benign recheck must omit ONLY the live MediaID equality predicate")
	}
	for _, fragment := range []string{
		"s.stable_relative_path=''", "s.stable_relative_path LIKE '/%'", `s.stable_relative_path ~ '(^|/)\.\.(/|$)'`,
		"output.owner_key='share:default'", "output.export_key=s.media_item_key", "output.state='active'",
		"mapping.id::text=output.share_strm_id", "output.export_key IS NULL", "output.output_path<>rtrim($1,'/')||'/'||s.stable_relative_path",
		"COALESCE(mapping.payload->>'file_name','')=''", "COALESCE(mapping.payload->>'media_id','')=''", "COALESCE(mapping.payload->>'share_code','')=''",
		"(COALESCE(mapping.payload->>'file_id','')='' AND COALESCE(mapping.payload->>'file_path','')='')",
		"NOT EXISTS(SELECT 1 FROM t_share_media WHERE id=(mapping.payload->>'media_id')::integer)",
		shareSelectionLiveJoin, shareSelectionLivePredicate, "i.candidate_id=s.selected_candidate_id", "i.work_key=s.work_key",
		"i.season_number=s.season_number", "i.episode_number=s.episode_number", "revision<acked_revision", "work_key=''", "scope<>'baseline'",
		"output_path=''", "COALESCE(share_strm_id,'')=''", "content_fingerprint=''", "mapping_fingerprint=''",
		"m.media_type IS NULL", "m.media_type NOT IN ('movie','tv')", "s.work_key=m.work_key", "e.file_id=f.id",
		"s.season_number=e.season_number", "s.episode_number=e.episode_number",
	} {
		if !strings.Contains(shareBaselineBenignCountsSQL, fragment) {
			test.Fatalf("benign recheck weakened strict predicate: %s", fragment)
		}
	}
}

func TestShareBaselineAdoptionCheckpointAndLockSQLContract(test *testing.T) {
	for _, relation := range []string{"t_share_media_file", "t_share_media_file_episode", "t_share_media", "t_share_record", "t_share_export_candidate", "t_share_export_candidate_item", "t_share_media_selection", "t_share_export_source_state", "t_share_export_dirty_work", "t_share_export_consumer", "t_system_config", "t_media_category", "t_strm_export_state", "t_strm_export_history", "t_share_strm", "t_strm_file"} {
		if !strings.Contains(shareBaselineTablesLockSQL, relation) {
			test.Fatalf("unprotected writer: %s", relation)
		}
	}
	if !strings.Contains(shareBaselineTablesLockSQL, "SHARE ROW EXCLUSIVE MODE NOWAIT") || !strings.Contains(shareBaselineOwnerLockSQL, "34982") || !strings.Contains(shareBaselineSelectionLockSQL, "34983,1") {
		test.Fatal("locking protocol changed")
	}
	for _, fragment := range []string{"f.version,r.version,f.available,r.share_cancelled", "string_agg(e.season_number::text", "jsonb_agg(s.media_item_key", "s.work_key=m.work_key", "CASE WHEN EXCLUDED.state='stale' THEN source.export_keys", "share_version=EXCLUDED.share_version"} {
		if !strings.Contains(shareBaselineSourcesSQL, fragment) {
			test.Fatalf("missing future incremental source semantics: %s", fragment)
		}
	}
	if !strings.Contains(shareBaselineCountsSQL, shareSelectionLivePredicate) || !strings.Contains(shareBaselineCountsSQL, "scope<>'baseline'") || !strings.Contains(shareBaselineAckSQL, "scope='baseline'") {
		test.Fatal("must reject non-baseline pending and preserve live selection rules")
	}
	for _, fragment := range []string{shareSelectionLiveJoin, shareSelectionLivePredicate, "i.candidate_id=s.selected_candidate_id", "i.work_key=s.work_key", "i.season_number=s.season_number AND i.episode_number=s.episode_number", "output.owner_key='share:default'", "output.export_key=s.media_item_key", "output.state='active'", "mapping.id::text=output.share_strm_id", "f.file_name,f.file_id,f.media_id,r.url,r.password"} {
		if !strings.Contains(shareBaselineMappingsSQL, fragment) {
			test.Fatalf("mapping identity must use the actual live selected source and owned output: missing %s", fragment)
		}
	}
	for _, verb := range []string{"UPDATE ", "INSERT ", "DELETE ", "TRUNCATE "} {
		if strings.Contains(shareBaselineMappingsSQL, verb) {
			test.Fatal("mapping verification must not rewrite business output")
		}
	}
	for _, query := range []string{shareBaselineSourcesSQL, shareBaselineStaleSourcesSQL, shareBaselineSelectionAckSQL, shareBaselineAckSQL, shareBaselineTrustSQL, shareBaselineAuditSQL} {
		if strings.Contains(query, "DELETE ") || strings.Contains(query, "TRUNCATE ") {
			test.Fatal("adoption must not delete business data")
		}
	}
}

func TestShareBaselineAdoptionDAORejectsImplicitInvocation(test *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	defer database.Close()
	for _, scenario := range []string{"confirmation", "task", "fingerprint", "path"} {
		request, id, fingerprint := adoptionRequest(), "task", "fingerprint"
		switch scenario {
		case "confirmation":
			request.ManualConfirmation = ""
		case "task":
			id = ""
		case "fingerprint":
			fingerprint = ""
		case "path":
			request.ExpectedOutputPath = "/"
		}
		if _, err := NewShareExportCheckpointDAO(database).AdoptBaseline(context.Background(), request, fingerprint, id); err == nil {
			test.Fatal("implicit adoption accepted")
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}
