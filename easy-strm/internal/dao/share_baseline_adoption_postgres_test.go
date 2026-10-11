package dao

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"easy-strm/internal/domain"
)

func baselineIsolatedPostgres(test *testing.T) *sql.DB {
	test.Helper()
	bin := os.Getenv("ESTRM_BASELINE_PG_BIN")
	if bin == "" {
		test.Skip("设置 ESTRM_BASELINE_PG_BIN 后仅启动 TMPDIR 下的全新 PostgreSQL；不接受外部 DSN")
	}
	if os.Getenv("TMPDIR") == "" {
		test.Fatal("隔离测试必须明确 TMPDIR，禁止回退到 /tmp")
	}
	root, err := os.MkdirTemp(os.Getenv("TMPDIR"), "bp-")
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() { _ = os.RemoveAll(root) })
	data, socket := filepath.Join(root, "data"), root
	output, err := exec.Command(filepath.Join(bin, "initdb"), "-D", data, "-A", "trust", "-U", "baseline_test", "--no-locale", "-E", "UTF8").CombinedOutput()
	if err != nil {
		test.Fatalf("isolated initdb: %v: %s", err, output)
	}
	log, err := os.Create(filepath.Join(root, "postgres.log"))
	if err != nil {
		test.Fatal(err)
	}
	server := exec.Command(filepath.Join(bin, "postgres"), "-D", data, "-k", socket, "-p", "55439", "-c", "listen_addresses=")
	server.Stdout, server.Stderr = log, log
	if err := server.Start(); err != nil {
		_ = log.Close()
		test.Fatal(err)
	}
	test.Cleanup(func() {
		_ = server.Process.Signal(syscall.SIGTERM)
		_ = server.Wait()
		_ = log.Close()
	})
	database, err := sql.Open("postgres", fmt.Sprintf("host='%s' port=55439 user=baseline_test dbname=postgres sslmode=disable connect_timeout=1", strings.ReplaceAll(socket, "'", "\\'")))
	if err != nil {
		test.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	test.Cleanup(func() { _ = database.Close() })
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err = database.Ping(); err == nil {
			return database
		}
		if time.Now().After(deadline) {
			test.Fatalf("isolated PostgreSQL startup: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

const baselineFixtureSchemaSQL = `
CREATE TEMP TABLE t_share_media(id integer PRIMARY KEY,work_key text,media_type text) ON COMMIT DROP;
CREATE TEMP TABLE t_share_record(id integer PRIMARY KEY,url text,password text,version integer,share_cancelled boolean) ON COMMIT DROP;
CREATE TEMP TABLE t_share_media_file(id integer PRIMARY KEY,share_id integer,media_id integer,file_name text,file_id text,version integer,available boolean,status text) ON COMMIT DROP;
CREATE TEMP TABLE t_share_media_file_episode(file_id integer,season_number integer,episode_number integer) ON COMMIT DROP;
CREATE TEMP TABLE t_share_export_candidate(candidate_id bigint,source_file_id integer,available boolean,state text) ON COMMIT DROP;
CREATE TEMP TABLE t_share_export_candidate_item(candidate_id bigint,work_key text,season_number integer,episode_number integer,revoked boolean) ON COMMIT DROP;
CREATE TEMP TABLE t_share_media_selection(media_item_key text PRIMARY KEY,work_key text,season_number integer,episode_number integer,selected_candidate_id bigint,stable_relative_path text,selection_revision bigint,exported_revision bigint,updated_at timestamptz DEFAULT now()) ON COMMIT DROP;
CREATE TEMP TABLE t_strm_export_state(owner_key text,export_key text,state text,output_path text,share_strm_id text,content_fingerprint text,mapping_fingerprint text,PRIMARY KEY(owner_key,export_key)) ON COMMIT DROP;
CREATE TEMP TABLE t_share_strm(id uuid PRIMARY KEY,payload jsonb) ON COMMIT DROP;
CREATE TEMP TABLE t_share_export_dirty_work(work_key text,revision bigint,acked_revision bigint,scope text) ON COMMIT DROP;
CREATE TEMP TABLE t_share_export_source_state(source_file_id integer PRIMARY KEY,share_id integer,media_id integer,work_key text,file_version integer,share_version integer,available boolean,cancelled boolean,episode_sig text,export_keys jsonb,last_seen_run_id text,state text,updated_at timestamptz DEFAULT now()) ON COMMIT DROP;
INSERT INTO t_share_media VALUES(7,'work','movie'),(8,'other','movie');
INSERT INTO t_share_record VALUES(1,'https://115.com/s/selected?password=url-code','explicit',3,false);
INSERT INTO t_share_media_file VALUES(11,1,7,'Show/episode.mkv','file',2,true,'identified');
INSERT INTO t_share_export_candidate VALUES(1,11,true,'active');
INSERT INTO t_share_export_candidate_item VALUES(1,'work',0,0,false);
INSERT INTO t_share_media_selection VALUES('work:0:0','work',0,0,1,'movie.strm',1,0);
INSERT INTO t_share_export_dirty_work VALUES('work',2,0,'baseline');`

func baselineFixtureTransaction(test *testing.T, database *sql.DB) *sql.Tx {
	test.Helper()
	transaction, err := database.Begin()
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() { _ = transaction.Rollback() })
	baselineFixtureExec(test, transaction, baselineFixtureSchemaSQL)
	source := domain.ShareStrmSource{ID: 11, MediaID: 7, URL: "https://115.com/s/selected?password=url-code", Password: "explicit", FileName: "Show/episode.mkv", RemoteFileID: "file"}
	entry, err := domain.BuildShareStrmEntry(source)
	if err != nil {
		test.Fatal(err)
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		test.Fatal(err)
	}
	baselineFixtureExec(test, transaction, `INSERT INTO t_share_strm VALUES($1,$2)`, entry.ID, string(payload))
	baselineFixtureExec(test, transaction, `INSERT INTO t_strm_export_state VALUES('share:default','work:0:0','active','/isolated/output/movie.strm',$1,'content','mapping')`, entry.ID)
	return transaction
}

func baselineFixtureExec(test *testing.T, transaction *sql.Tx, query string, args ...interface{}) {
	test.Helper()
	if _, err := transaction.Exec(query, args...); err != nil {
		test.Fatalf("fixture SQL: %v", err)
	}
}

func baselineFixtureCounts(test *testing.T, transaction *sql.Tx, query string) ([6]int64, error) {
	test.Helper()
	baselineFixtureExec(test, transaction, "SAVEPOINT counts")
	var counts [6]int64
	err := transaction.QueryRow(query, "/isolated/output").Scan(&counts[0], &counts[1], &counts[2], &counts[3], &counts[4], &counts[5])
	baselineFixtureExec(test, transaction, "ROLLBACK TO SAVEPOINT counts")
	return counts, err
}

func baselineFixtureMappingBag(test *testing.T, transaction *sql.Tx, query string, grouped bool) map[string]int64 {
	test.Helper()
	rows, err := transaction.Query(query)
	if err != nil {
		test.Fatal(err)
	}
	defer rows.Close()
	bag := map[string]int64{}
	for rows.Next() {
		values := make([]interface{}, 8)
		targets := make([]interface{}, 8)
		for index := range values {
			targets[index] = &values[index]
		}
		weight := int64(1)
		if grouped {
			targets = append(targets, &weight)
		}
		if err := rows.Scan(targets...); err != nil {
			test.Fatal(err)
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			test.Fatal(err)
		}
		bag[string(encoded)] += weight
	}
	if err := rows.Err(); err != nil {
		test.Fatal(err)
	}
	return bag
}

func baselineFixtureSources(test *testing.T, transaction *sql.Tx, query string) string {
	test.Helper()
	baselineFixtureExec(test, transaction, "SAVEPOINT sources")
	baselineFixtureExec(test, transaction, query, "fixture-run")
	var snapshot string
	err := transaction.QueryRow(`SELECT COALESCE(jsonb_agg(to_jsonb(source) ORDER BY source_file_id),'[]'::jsonb)::text FROM t_share_export_source_state source`).Scan(&snapshot)
	if err != nil {
		test.Fatal(err)
	}
	baselineFixtureExec(test, transaction, "ROLLBACK TO SAVEPOINT sources")
	return snapshot
}

const baselineMultiEpisodeFixtureSQL = `
UPDATE t_share_media SET media_type='tv' WHERE id=7;
UPDATE t_share_export_candidate_item SET season_number=1,episode_number=1;
INSERT INTO t_share_export_candidate_item VALUES(1,'work',1,2,false);
UPDATE t_share_media_selection SET season_number=1,episode_number=1;
INSERT INTO t_share_media_selection SELECT 'work:1:2',work_key,1,2,selected_candidate_id,'second.strm',1,0 FROM t_share_media_selection;
INSERT INTO t_strm_export_state SELECT owner_key,'work:1:2',state,'/isolated/output/second.strm',share_strm_id,content_fingerprint,mapping_fingerprint FROM t_strm_export_state;
INSERT INTO t_share_media_file_episode VALUES(11,1,2),(11,1,1),(11,1,1);`

func TestShareBaselinePostgresEquivalence(test *testing.T) {
	database := baselineIsolatedPostgres(test)
	scenarios := []struct{ name, mutation string }{
		{"valid", ""},
		{"empty-path", `UPDATE t_share_media_selection SET stable_relative_path=''`},
		{"absolute-path", `UPDATE t_share_media_selection SET stable_relative_path='/movie.strm'`},
		{"parent-path", `UPDATE t_share_media_selection SET stable_relative_path='folder/../movie.strm'`},
		{"missing-output", `DELETE FROM t_strm_export_state`},
		{"wrong-owner", `UPDATE t_strm_export_state SET owner_key='other'`},
		{"inactive-output", `UPDATE t_strm_export_state SET state='missing'`},
		{"different-path", `UPDATE t_strm_export_state SET output_path='/other/movie.strm'`},
		{"missing-mapping", `DELETE FROM t_share_strm`},
		{"malformed-mapping-reference", `UPDATE t_strm_export_state SET share_strm_id='not-a-uuid'`},
		{"uppercase-mapping-reference", `UPDATE t_strm_export_state SET share_strm_id=upper(share_strm_id)`},
		{"compact-mapping-reference", `UPDATE t_strm_export_state SET share_strm_id=replace(share_strm_id,'-','')`},
		{"braced-mapping-reference", `UPDATE t_strm_export_state SET share_strm_id='{'||share_strm_id||'}'`},
		{"padded-mapping-reference", `UPDATE t_strm_export_state SET share_strm_id=' '||share_strm_id||' '`},
		{"unknown-media", `UPDATE t_share_strm SET payload=jsonb_set(payload,'{media_id}','999')`},
		{"benign-media-only", `UPDATE t_share_strm SET payload=jsonb_set(payload,'{media_id}','8')`},
		{"remaining-path", `UPDATE t_share_strm SET payload=jsonb_set(payload,'{media_id}','8'); UPDATE t_strm_export_state SET output_path='/other'`},
		{"empty-file-identifiers", `UPDATE t_share_strm SET payload=payload-'file_id'-'file_path'`},
		{"missing-file-name", `UPDATE t_share_strm SET payload=payload-'file_name'`},
		{"missing-share-code", `UPDATE t_share_strm SET payload=payload-'share_code'`},
		{"missing-media-id", `UPDATE t_share_strm SET payload=payload-'media_id'`},
		{"empty-media-id", `UPDATE t_share_strm SET payload=jsonb_set(payload,'{media_id}','""')`},
		{"malformed-media-id", `UPDATE t_share_strm SET payload=jsonb_set(payload,'{media_id}','"invalid"')`},
		{"malformed-id-already-invalid-path", `UPDATE t_share_strm SET payload=jsonb_set(payload,'{media_id}','"invalid"'); UPDATE t_share_media_selection SET stable_relative_path=''`},
		{"malformed-id-missing-name", `UPDATE t_share_strm SET payload=jsonb_set(payload-'file_name','{media_id}','"invalid"')`},
		{"fractional-media-id", `UPDATE t_share_strm SET payload=jsonb_set(payload,'{media_id}','7.5')`},
		{"overflow-media-id", `UPDATE t_share_strm SET payload=jsonb_set(payload,'{media_id}','2147483648')`},
		{"string-media-id", `UPDATE t_share_strm SET payload=jsonb_set(payload,'{media_id}','"7"')`},
		{"revoked-item", `UPDATE t_share_export_candidate_item SET revoked=true`},
		{"unavailable-candidate", `UPDATE t_share_export_candidate SET available=false`},
		{"stale-candidate", `UPDATE t_share_export_candidate SET state='stale'`},
		{"unavailable-file", `UPDATE t_share_media_file SET available=false`},
		{"cancelled-share", `UPDATE t_share_record SET share_cancelled=true`},
		{"unidentified-file", `UPDATE t_share_media_file SET status='pending'`},
		{"wrong-work", `UPDATE t_share_media SET work_key='wrong' WHERE id=7`},
		{"wrong-selected-candidate", `UPDATE t_share_media_selection SET selected_candidate_id=2`},
		{"wrong-item-work", `UPDATE t_share_export_candidate_item SET work_key='other'`},
		{"wrong-item-season", `UPDATE t_share_export_candidate_item SET season_number=2`},
		{"wrong-item-episode", `UPDATE t_share_export_candidate_item SET episode_number=2`},
		{"backwards-dirty", `UPDATE t_share_export_dirty_work SET revision=-1`},
		{"empty-dirty-work", `UPDATE t_share_export_dirty_work SET work_key=''`},
		{"non-baseline-dirty", `UPDATE t_share_export_dirty_work SET scope='other'`},
		{"empty-output-path", `UPDATE t_strm_export_state SET output_path=''`},
		{"empty-mapping-reference", `UPDATE t_strm_export_state SET share_strm_id=''`},
		{"empty-content", `UPDATE t_strm_export_state SET content_fingerprint=''`},
		{"empty-mapping-fingerprint", `UPDATE t_strm_export_state SET mapping_fingerprint=''`},
		{"unsupported-media", `UPDATE t_share_media SET media_type='other' WHERE id=7`},
		{"unrepresented-movie", `DELETE FROM t_share_media_selection`},
		{"tv-no-episodes", `UPDATE t_share_media SET media_type='tv' WHERE id=7`},
		{"tv-unrepresented-episode", baselineMultiEpisodeFixtureSQL + `INSERT INTO t_share_media_file_episode VALUES(11,1,3)`},
		{"multi-episode-and-duplicate-episodes", baselineMultiEpisodeFixtureSQL},
		{"duplicate-item-members", `INSERT INTO t_share_export_candidate_item SELECT * FROM t_share_export_candidate_item`},
		{"duplicate-candidate-members", `INSERT INTO t_share_export_candidate SELECT * FROM t_share_export_candidate`},
		{"duplicate-output-join", `ALTER TABLE t_strm_export_state DROP CONSTRAINT t_strm_export_state_pkey; INSERT INTO t_strm_export_state SELECT * FROM t_strm_export_state`},
		{"duplicate-selection-tuples", `INSERT INTO t_share_media_selection SELECT 'work:copy',work_key,season_number,episode_number,selected_candidate_id,'copy.strm',1,0 FROM t_share_media_selection; INSERT INTO t_strm_export_state SELECT owner_key,'work:copy',state,'/isolated/output/copy.strm',share_strm_id,content_fingerprint,mapping_fingerprint FROM t_strm_export_state`},
		{"two-live-media-identities", `INSERT INTO t_share_media_file SELECT 12,share_id,8,file_name,file_id,version,available,status FROM t_share_media_file; UPDATE t_share_media SET work_key='work' WHERE id=8; INSERT INTO t_share_export_candidate VALUES(1,12,true,'active')`},
		{"unselected-contributor", `INSERT INTO t_share_media_file SELECT 12,share_id,media_id,file_name,file_id,version,available,status FROM t_share_media_file`},
		{"stale-source-retains-keys", `UPDATE t_share_media_file SET available=false; INSERT INTO t_share_export_source_state(source_file_id,export_keys) VALUES(11,'["old-key"]')`},
		{"null-episode", baselineMultiEpisodeFixtureSQL + `INSERT INTO t_share_media_file_episode VALUES(11,NULL,3),(11,1,NULL)`},
	}
	for _, column := range []struct{ table, field string }{
		{"t_share_media_selection", "stable_relative_path"}, {"t_strm_export_state", "output_path"},
		{"t_strm_export_state", "share_strm_id"}, {"t_strm_export_state", "content_fingerprint"}, {"t_strm_export_state", "mapping_fingerprint"},
		{"t_share_media", "media_type"}, {"t_share_media", "work_key"},
		{"t_share_export_dirty_work", "revision"}, {"t_share_export_dirty_work", "acked_revision"}, {"t_share_export_dirty_work", "work_key"}, {"t_share_export_dirty_work", "scope"},
		{"t_share_export_candidate_item", "revoked"}, {"t_share_export_candidate", "available"}, {"t_share_export_candidate", "state"},
		{"t_share_media_file", "available"}, {"t_share_media_file", "status"}, {"t_share_record", "share_cancelled"},
	} {
		scenarios = append(scenarios, struct{ name, mutation string }{"null-" + column.table + "-" + column.field, fmt.Sprintf("UPDATE %s SET %s=NULL", column.table, column.field)})
	}
	for _, scenario := range scenarios {
		test.Run(scenario.name, func(test *testing.T) {
			transaction := baselineFixtureTransaction(test, database)
			if scenario.mutation != "" {
				baselineFixtureExec(test, transaction, scenario.mutation)
			}
			for _, queries := range [][2]string{{legacyShareBaselineCountsSQL, shareBaselineCountsSQL}, {legacyShareBaselineBenignCountsSQL, shareBaselineBenignCountsSQL}} {
				oldCounts, oldErr := baselineFixtureCounts(test, transaction, queries[0])
				newCounts, newErr := baselineFixtureCounts(test, transaction, queries[1])
				if (oldErr != nil) != (newErr != nil) || (oldErr == nil && oldCounts != newCounts) {
					test.Fatalf("old=%v error=%v; new=%v error=%v", oldCounts, oldErr, newCounts, newErr)
				}
				if scenario.name == "valid" && (newErr != nil || newCounts != [6]int64{1, 1, 1, 2, 1, 0}) {
					test.Fatal("valid fixture was not accepted")
				}
			}
			oldBag := baselineFixtureMappingBag(test, transaction, legacyShareBaselineMappingsSQL, false)
			newBag := baselineFixtureMappingBag(test, transaction, shareBaselineMappingsSQL, true)
			if !reflect.DeepEqual(oldBag, newBag) {
				test.Fatalf("mapping multiset differs: old=%v new=%v", oldBag, newBag)
			}
			oldSources := baselineFixtureSources(test, transaction, legacyShareBaselineSourcesSQL)
			newSources := baselineFixtureSources(test, transaction, shareBaselineSourcesSQL)
			if oldSources != newSources {
				test.Fatalf("source checkpoint differs: old=%s new=%s", oldSources, newSources)
			}
			if scenario.name == "string-media-id" {
				if err := validateShareBaselineMappings(context.Background(), transaction, 1, 0); err == nil {
					test.Fatal("numeric-looking JSON string weakened decoding")
				}
			}
			if scenario.name == "duplicate-item-members" || scenario.name == "duplicate-candidate-members" || scenario.name == "duplicate-output-join" {
				if err := validateShareBaselineMappings(context.Background(), transaction, 1, 0); err == nil {
					test.Fatal("invalid duplicate multiplicity was deduplicated away")
				}
			}
			if scenario.name == "multi-episode-and-duplicate-episodes" || scenario.name == "duplicate-selection-tuples" {
				if err := validateShareBaselineMappings(context.Background(), transaction, 2, 0); err != nil {
					test.Fatalf("valid repeated mapping was rejected: %v", err)
				}
			}
			if scenario.name == "benign-media-only" || scenario.name == "remaining-path" {
				request := adoptionRequest()
				request.Expected["selection_count"], request.Expected["output_count"], request.Expected["selection_revision_sum"] = 1, 1, 1
				request.AllowKnownBenignInvalid, request.BenignAttestation = 1, "isolated-fixture-review"
				var result domain.ShareBaselineAdoptionResult
				err := validateShareBaselineCounts(context.Background(), transaction, request, &result)
				if (err == nil) != (scenario.name == "benign-media-only") {
					test.Fatalf("benign/remaining acceptance differs: %v", err)
				}
			}
		})
	}
}

func TestShareBaselineExportRewrittenSQL(test *testing.T) {
	if original := os.Getenv("ESTRM_BASELINE_LEGACY_DUMP"); original != "" {
		raw, err := os.ReadFile(original)
		if err != nil {
			test.Fatal(err)
		}
		_, counts, found := strings.Cut(string(raw), "===COUNTS===\n")
		counts, mappings, split := strings.Cut(counts, "===MAPPINGS===\n")
		mappings, _, _ = strings.Cut(mappings, "--- PASS:")
		if !found || !split || strings.TrimSpace(counts) != strings.TrimSpace(legacyShareBaselineCountsSQL) || strings.TrimSpace(mappings) != strings.TrimSpace(legacyShareBaselineMappingsSQL) {
			test.Fatal("legacy reference queries differ from the supplied original SQL dump")
		}
	}
	output := os.Getenv("ESTRM_BASELINE_SQL_OUTPUT")
	if output == "" {
		return
	}
	var document bytes.Buffer
	document.WriteString("-- 更新日期：2026-10-11；维护者：Codex。实际 DAO SQL 导出；须 DBA 审核，未执行生产库。\n-- 仅定义语句，不执行采纳；写语句只能在完整锁/快照/取消提交守卫协议内执行。\n")
	for _, statement := range []struct{ name, types, query string }{
		{"counts", "(text)", shareBaselineCountsSQL}, {"benign_counts", "(text)", shareBaselineBenignCountsSQL},
		{"mappings", "", shareBaselineMappingsSQL}, {"sources", "(text)", shareBaselineSourcesSQL},
		{"stale_sources", "(text)", shareBaselineStaleSourcesSQL}, {"selection_ack", "", shareBaselineSelectionAckSQL},
		{"ack", "(text)", shareBaselineAckSQL}, {"trust", "", shareBaselineTrustSQL},
		{"finish", "(bigint)", shareExportFinishBaselineSQL}, {"watermark", "(text)", shareWatermarkMirrorSQL},
	} {
		fmt.Fprintf(&document, "\nPREPARE baseline_%s%s AS\n%s;\n", statement.name, statement.types, statement.query)
	}
	if err := os.WriteFile(output, document.Bytes(), 0600); err != nil {
		test.Fatal(err)
	}
}

func TestShareBaselinePostgresFullFlowScale(test *testing.T) {
	if os.Getenv("ESTRM_BASELINE_SCALE") != "1" {
		test.Skip("仅显式启用全新隔离集群的 19041 作品/155023 选择合成验收")
	}
	database := baselineIsolatedPostgres(test)
	transaction, err := database.Begin()
	if err != nil {
		test.Fatal(err)
	}
	defer transaction.Rollback()
	baselineFixtureExec(test, transaction, strings.ReplaceAll(baselineFixtureSchemaSQL, " ON COMMIT DROP", ""))
	baselineFixtureExec(test, transaction, `
TRUNCATE t_share_media,t_share_media_file,t_share_export_candidate,t_share_export_candidate_item,t_share_media_selection,t_share_export_dirty_work;
ALTER TABLE t_share_export_dirty_work ADD COLUMN last_seen_run_id text, ADD COLUMN pending_export_keys jsonb DEFAULT '[]', ADD COLUMN updated_at timestamptz DEFAULT now();
CREATE TEMP TABLE t_share_export_consumer(consumer text PRIMARY KEY,config_revision bigint,prepared_revision bigint,completed_revision bigint,config_fingerprint text,baseline_state text,protocol_version integer,legacy_outputs_reconciled boolean,updated_at timestamptz DEFAULT now());
INSERT INTO t_share_export_consumer VALUES('share:default',3,3,0,'fixture-fingerprint','building',1,false);
CREATE TEMP TABLE t_system_config(config_key text PRIMARY KEY,config_val text,update_time timestamptz);
CREATE TEMP TABLE t_media_category(id integer);
CREATE TEMP TABLE t_strm_export_history(id integer);
CREATE TEMP TABLE t_strm_file(id integer);
CREATE TEMP TABLE fixture_entries(source_id integer,media_id integer,file_name text,file_id text,payload jsonb);
`)
	const works, selections = 19041, 155023
	type fixtureEntry struct {
		SourceID int                   `json:"source_id"`
		MediaID  int                   `json:"media_id"`
		FileName string                `json:"file_name"`
		FileID   string                `json:"file_id"`
		Payload  domain.ShareStrmEntry `json:"payload"`
	}
	entries := make([]fixtureEntry, 0, works)
	for index := 1; index <= works; index++ {
		source := domain.ShareStrmSource{ID: 100000 + index, MediaID: index, FileName: "Show/" + strconv.Itoa(index) + ".mkv", RemoteFileID: strconv.Itoa(index), URL: "https://115.com/s/selected?password=url-code", Password: "explicit"}
		entry, buildErr := domain.BuildShareStrmEntry(source)
		if buildErr != nil {
			test.Fatal(buildErr)
		}
		entries = append(entries, fixtureEntry{source.ID, source.MediaID, source.FileName, source.RemoteFileID, entry})
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		test.Fatal(err)
	}
	baselineFixtureExec(test, transaction, `INSERT INTO fixture_entries SELECT * FROM jsonb_to_recordset($1::jsonb) AS entry(source_id integer,media_id integer,file_name text,file_id text,payload jsonb)`, string(encoded))
	baselineFixtureExec(test, transaction, `
INSERT INTO t_share_media SELECT media_id,'work-'||media_id,'tv' FROM fixture_entries;
INSERT INTO t_share_media_file SELECT source_id,1,media_id,file_name,file_id,2,true,'identified' FROM fixture_entries;
INSERT INTO t_share_strm SELECT (payload->>'id')::uuid,payload FROM fixture_entries;
INSERT INTO t_share_export_candidate SELECT media_id,source_id,true,'active' FROM fixture_entries;
INSERT INTO t_share_media_file_episode SELECT 100001+(ordinal-1)%19041,1,1+(ordinal-1)/19041 FROM generate_series(1,155023) ordinal;
INSERT INTO t_share_export_candidate_item SELECT f.media_id,m.work_key,e.season_number,e.episode_number,false FROM t_share_media_file_episode e JOIN t_share_media_file f ON f.id=e.file_id JOIN t_share_media m ON m.id=f.media_id;
INSERT INTO t_share_media_selection SELECT work_key||':'||season_number||':'||episode_number,work_key,season_number,episode_number,candidate_id,work_key||'/'||episode_number||'.strm',1,0 FROM t_share_export_candidate_item;
INSERT INTO t_strm_export_state SELECT 'share:default',s.media_item_key,'active','/isolated/output/'||s.stable_relative_path,entry.payload->>'id','content','mapping' FROM t_share_media_selection s JOIN fixture_entries entry ON entry.media_id=s.selected_candidate_id;
INSERT INTO t_share_export_dirty_work(work_key,revision,acked_revision,scope) SELECT work_key,2,0,'baseline' FROM t_share_media;
CREATE INDEX ON t_share_export_candidate_item(candidate_id,work_key,season_number,episode_number);
CREATE INDEX ON t_share_export_candidate(candidate_id);
CREATE INDEX ON t_share_media_file_episode(file_id,season_number,episode_number);
CREATE INDEX ON t_share_media_selection(work_key,season_number,episode_number);
ANALYZE t_share_media; ANALYZE t_share_record; ANALYZE t_share_media_file; ANALYZE t_share_media_file_episode;
ANALYZE t_share_export_candidate; ANALYZE t_share_export_candidate_item; ANALYZE t_share_media_selection;
ANALYZE t_strm_export_state; ANALYZE t_share_strm; ANALYZE t_share_export_dirty_work;
`)
	if err := transaction.Commit(); err != nil {
		test.Fatal(err)
	}
	request := adoptionRequest()
	request.Expected["selection_count"], request.Expected["output_count"] = selections, selections
	request.Expected["selection_revision_sum"] = selections
	request.Expected["pending_count"], request.Expected["dirty_revision_sum"] = works, 2*works
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	started := time.Now()
	guarded := false
	result, err := NewShareExportCheckpointDAO(database).AdoptBaseline(ctx, request, "fixture-fingerprint", "fixture-scale", func() error { guarded = true; return ctx.Err() })
	elapsed := time.Since(started)
	if err != nil || !guarded || result.BaselineState != "ready" || result.CheckpointSources != works || result.AcknowledgedWorks != works || result.SelectionCount != selections || result.OutputCount != selections {
		test.Fatalf("synthetic full DAO flow elapsed=%s result=%+v error=%v", elapsed, result, err)
	}
	var pending, unexported, audits int64
	if err := database.QueryRow(`SELECT (SELECT count(*) FROM t_share_export_dirty_work WHERE revision>acked_revision),(SELECT count(*) FROM t_share_media_selection WHERE exported_revision<>selection_revision),(SELECT count(*) FROM t_system_config WHERE config_key='share_strm_baseline_adoption:fixture-scale')`).Scan(&pending, &unexported, &audits); err != nil || pending != 0 || unexported != 0 || audits != 1 {
		test.Fatalf("atomic full-flow postconditions: pending=%d unexported=%d audits=%d error=%v", pending, unexported, audits, err)
	}
	test.Logf("ISOLATED SYNTHETIC ONLY: works=%d selections=%d validation+checkpoint+ack+finish+audit+commit=%s; not production timing", works, selections, elapsed)
}
