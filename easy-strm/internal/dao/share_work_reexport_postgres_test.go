package dao

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const workReexportPostgresSchemaSQL = `
CREATE TEMP TABLE t_share_media(id integer PRIMARY KEY,work_key text NOT NULL UNIQUE,media_type text NOT NULL,title text NOT NULL,result jsonb NOT NULL);
CREATE TEMP TABLE t_share_record(id integer PRIMARY KEY,name text NOT NULL DEFAULT '',url text NOT NULL,password text NOT NULL DEFAULT '',version integer NOT NULL DEFAULT 1,share_cancelled boolean NOT NULL DEFAULT false);
CREATE TEMP TABLE t_share_media_file(id integer PRIMARY KEY,share_id integer NOT NULL REFERENCES t_share_record(id),media_id integer REFERENCES t_share_media(id),file_name text NOT NULL,file_id text NOT NULL DEFAULT '',file_size bigint NOT NULL DEFAULT 0,version integer NOT NULL DEFAULT 1,available boolean NOT NULL DEFAULT true,status text NOT NULL DEFAULT 'identified');
CREATE UNIQUE INDEX ON t_share_media_file(share_id,file_id) WHERE file_id<>'';
CREATE UNIQUE INDEX ON t_share_media_file(share_id,file_name) WHERE file_id='';
CREATE TEMP TABLE t_share_media_file_episode(file_id integer REFERENCES t_share_media_file(id),season_number integer NOT NULL CHECK(season_number>=0),episode_number integer NOT NULL CHECK(episode_number>0),PRIMARY KEY(file_id,season_number,episode_number));
CREATE TEMP TABLE t_share_export_candidate(candidate_id bigint PRIMARY KEY,source_file_id integer UNIQUE REFERENCES t_share_media_file(id),available boolean NOT NULL DEFAULT true,state text NOT NULL DEFAULT 'active');
CREATE TEMP TABLE t_share_export_candidate_item(candidate_id bigint REFERENCES t_share_export_candidate(candidate_id),work_key text NOT NULL CHECK(work_key<>''),season_number integer NOT NULL CHECK(season_number>=0),episode_number integer NOT NULL CHECK(episode_number>=0),revoked boolean NOT NULL DEFAULT false,PRIMARY KEY(candidate_id,work_key,season_number,episode_number));
CREATE TEMP TABLE t_share_media_selection(work_key text NOT NULL CONSTRAINT reexport_nonempty_work CHECK(work_key<>''),season_number integer NOT NULL CHECK(season_number>=0),episode_number integer NOT NULL CHECK(episode_number>=0),media_item_key text GENERATED ALWAYS AS (work_key||':'||season_number||':'||episode_number) STORED PRIMARY KEY,selected_candidate_id bigint NOT NULL,selection_mode text NOT NULL DEFAULT 'auto' CHECK(selection_mode IN ('auto','manual')),selection_revision bigint NOT NULL DEFAULT 1 CHECK(selection_revision>=1),exported_revision bigint NOT NULL DEFAULT 0 CHECK(exported_revision>=0 AND exported_revision<=selection_revision),stable_relative_path text NOT NULL DEFAULT '',updated_at timestamptz NOT NULL DEFAULT now(),CONSTRAINT reexport_member FOREIGN KEY(selected_candidate_id,work_key,season_number,episode_number) REFERENCES t_share_export_candidate_item(candidate_id,work_key,season_number,episode_number));
ALTER TABLE t_share_media_selection ADD COLUMN source_kind text NOT NULL DEFAULT 'share', ADD COLUMN external_ref text NOT NULL DEFAULT '';
CREATE INDEX ON t_share_media_selection(stable_relative_path);
CREATE TEMP TABLE t_strm_export_state(owner_key text NOT NULL,export_key text NOT NULL,output_path text NOT NULL,content_fingerprint text NOT NULL DEFAULT '',mapping_fingerprint text NOT NULL DEFAULT '',share_strm_id text,state text NOT NULL DEFAULT 'active',last_seen_run_id text NOT NULL DEFAULT '',exported_at timestamptz,PRIMARY KEY(owner_key,export_key));
CREATE UNIQUE INDEX ON t_strm_export_state(output_path) WHERE state='active';
CREATE INDEX ON t_strm_export_state(share_strm_id);
CREATE INDEX ON t_strm_export_state(output_path);
CREATE TEMP TABLE t_strm_export_history(owner_key text NOT NULL,output_path text NOT NULL,reason text NOT NULL,PRIMARY KEY(owner_key,output_path));
CREATE TEMP TABLE t_share_strm(id uuid PRIMARY KEY,payload jsonb NOT NULL,updated_at timestamptz NOT NULL DEFAULT now());
CREATE TEMP TABLE t_share_export_dirty_work(work_key text PRIMARY KEY,revision bigint NOT NULL,acked_revision bigint NOT NULL,scope text NOT NULL);
CREATE TEMP TABLE t_share_export_consumer(consumer text PRIMARY KEY,config_revision bigint NOT NULL);
CREATE TEMP TABLE t_strm_file(strm_config_id integer NOT NULL,local_strm_path text);
INSERT INTO t_share_media VALUES(7,'tmdb:movie:7','movie','Movie','{"success":true,"media_type":"movie","title":"Movie"}'),(8,'tmdb:tv:8','tv','TV','{"success":true,"media_type":"tv","title":"TV"}');
INSERT INTO t_share_record(id,url,password) VALUES(1,'https://115.com/s/isolated','fixture'),(2,'https://115.com/s/isolated','fixture');
INSERT INTO t_share_media_file(id,share_id,media_id,file_name,file_id) VALUES(11,1,7,'Shared/combined.mkv','same-physical-file'),(12,2,8,'Shared/combined.mkv','same-physical-file');
INSERT INTO t_share_media_file_episode VALUES(12,1,1);
INSERT INTO t_share_export_candidate(candidate_id,source_file_id) VALUES(1,11),(2,12);
INSERT INTO t_share_export_candidate_item(candidate_id,work_key,season_number,episode_number) VALUES(1,'tmdb:movie:7',0,0),(2,'tmdb:tv:8',1,1);
INSERT INTO t_share_media_selection(work_key,season_number,episode_number,selected_candidate_id,stable_relative_path) VALUES('tmdb:movie:7',0,0,1,'Movie/movie.strm'),('tmdb:tv:8',1,1,2,'TV/episode.strm');
INSERT INTO t_share_export_dirty_work VALUES('tmdb:movie:7',1,0,'baseline'),('tmdb:tv:8',1,0,'baseline');
INSERT INTO t_share_export_consumer VALUES('share:default',1);`

type workReexportPostgresFixture struct {
	connection *sql.Conn
	root       string
	works      []string
	targets    []WorkReexportTarget
	legacy     []domain.ShareStrmEntry
	selections []domain.ShareSelection
}

func newWorkReexportPostgresFixture(test *testing.T, database *sql.DB) *workReexportPostgresFixture {
	test.Helper()
	connection, err := database.Conn(context.Background())
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() {
		_, dropErr := connection.ExecContext(context.Background(), `DROP TABLE IF EXISTS t_share_media_selection,t_share_export_candidate_item,t_share_export_candidate,t_share_media_file_episode,t_share_media_file,t_share_media,t_share_record,t_strm_export_state,t_strm_export_history,t_share_strm,t_share_export_dirty_work,t_share_export_consumer,t_strm_file CASCADE; SELECT pg_advisory_unlock_all()`)
		if dropErr != nil {
			test.Error(dropErr)
		}
		_ = connection.Close()
	})
	fixture := &workReexportPostgresFixture{connection: connection, root: test.TempDir(), works: []string{"tmdb:movie:7", "tmdb:tv:8"}}
	fixture.exec(test, workReexportPostgresSchemaSQL)
	fixture.exec(test, `SELECT pg_advisory_lock(hashtextextended('share:default',34982))`)
	for _, work := range fixture.works {
		rows, queryErr := connection.QueryContext(context.Background(), shareWorkReexportSelectionsSQL, work)
		if queryErr != nil {
			test.Fatal(queryErr)
		}
		for rows.Next() {
			var value domain.ShareWorkReexportSelection
			selection := &value.Selection
			if err = rows.Scan(&selection.ItemKey, &selection.WorkKey, &selection.Season, &selection.Episode, &selection.CandidateID, &selection.Mode, &selection.Revision, &selection.ExportedRevision, &selection.RelativePath, &selection.SourceKind, &selection.ExternalRef, &selection.Title, &selection.Valid, &value.SourceID, &value.MediaID); err != nil {
				test.Fatal(err)
			}
			if !selection.Valid {
				test.Fatal("fixture 必须是真实有效的 movie/tv 选择")
			}
			fixture.targets = append(fixture.targets, WorkReexportTarget{Selection: *selection, Source: domain.ShareStrmSource{ID: value.SourceID}})
			fixture.selections = append(fixture.selections, *selection)
		}
		if err = errors.Join(rows.Err(), rows.Close()); err != nil {
			test.Fatal(err)
		}
	}
	transaction, err := connection.BeginTx(context.Background(), nil)
	if err != nil {
		test.Fatal(err)
	}
	defer transaction.Rollback()
	for index := range fixture.targets {
		target := &fixture.targets[index]
		target.Source, err = readWorkReexportSource(context.Background(), transaction, target.Source.ID)
		if err != nil {
			test.Fatal(err)
		}
		entry, buildErr := domain.BuildShareStrmEntry(target.Source)
		if buildErr != nil {
			test.Fatal(buildErr)
		}
		target.URLMappingID = entry.ID
		fixture.legacy = append(fixture.legacy, entry)
	}
	if err = transaction.Commit(); err != nil {
		test.Fatal(err)
	}
	if len(fixture.targets) != 2 || fixture.targets[0].Source.Result.MediaType != "movie" || fixture.targets[0].Source.Episodes == nil || len(fixture.targets[0].Source.Episodes) != 0 || fixture.targets[1].Source.Result.MediaType != "tv" || len(fixture.targets[1].Source.Episodes) != 1 || fixture.legacy[0].ID != fixture.legacy[1].ID {
		test.Fatal("必须从真实 SQL 解码出 movie 的 []，与 tv 共享同一个物理文件及历史 mapping")
	}
	payload, err := json.Marshal(fixture.legacy[0])
	if err != nil {
		test.Fatal(err)
	}
	fixture.exec(test, `INSERT INTO t_share_strm(id,payload) VALUES($1,$2)`, fixture.legacy[0].ID, string(payload))
	for _, target := range fixture.targets {
		path := filepath.Join(fixture.root, target.Selection.RelativePath)
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			test.Fatal(err)
		}
		content := []byte("https://fixture.invalid/share-strm/" + target.URLMappingID + "\n")
		if err = os.WriteFile(path, content, 0644); err != nil {
			test.Fatal(err)
		}
		fixture.exec(test, `INSERT INTO t_strm_export_state(owner_key,export_key,output_path,content_fingerprint,mapping_fingerprint,share_strm_id,last_seen_run_id) VALUES('share:default',$1,$2,$3,$4,$5,'before')`, target.Selection.ItemKey, path, workReexportPostgresContent(path, content), fmt.Sprintf("%x", sha256.Sum256(payload)), target.URLMappingID)
	}
	return fixture
}

func (fixture *workReexportPostgresFixture) exec(test *testing.T, query string, args ...interface{}) {
	test.Helper()
	if _, err := fixture.connection.ExecContext(context.Background(), query, args...); err != nil {
		test.Fatalf("isolated fixture SQL: %v", err)
	}
}

func workReexportPostgresContent(path string, content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(append([]byte(path+"\x00"), content...)))
}

func (fixture *workReexportPostgresFixture) snapshot(test *testing.T, tables ...string) []string {
	test.Helper()
	values := make([]string, len(tables))
	for index, table := range tables {
		if err := fixture.connection.QueryRowContext(context.Background(), `SELECT COALESCE(jsonb_agg(to_jsonb(row) ORDER BY to_jsonb(row)::text),'[]'::jsonb)::text FROM `+table+` row`).Scan(&values[index]); err != nil {
			test.Fatal(err)
		}
	}
	return values
}

func (fixture *workReexportPostgresFixture) counts(test *testing.T) [6]int64 {
	test.Helper()
	var counts [6]int64
	if err := fixture.connection.QueryRowContext(context.Background(), shareBaselineCountsSQL, fixture.root).Scan(&counts[0], &counts[1], &counts[2], &counts[3], &counts[4], &counts[5]); err != nil {
		test.Fatal(err)
	}
	return counts
}

func (fixture *workReexportPostgresFixture) verify(ctx context.Context, receipts []WorkReexportReceipt) error {
	if len(receipts) != len(fixture.targets) {
		return fmt.Errorf("不完整回执")
	}
	for index, receipt := range receipts {
		content, err := os.ReadFile(receipt.State.Path)
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if workReexportPostgresContent(receipt.State.Path, content) != receipt.State.Content || string(content) != "https://fixture.invalid/share-strm/"+fixture.targets[index].URLMappingID+"\n" {
			return ErrShareExportChanged
		}
		for _, mapping := range []domain.ShareStrmEntry{receipt.Mapping, receipt.URLMapping} {
			if mapping.ShareCode != receipt.Entry.ShareCode || mapping.Password != receipt.Entry.Password || mapping.FileID != receipt.Entry.FileID || mapping.FilePath != receipt.Entry.FilePath || mapping.FileName != receipt.Entry.FileName {
				return ErrShareExportChanged
			}
		}
	}
	return nil
}

func (fixture *workReexportPostgresFixture) reexport(ctx context.Context, run string, verify func(context.Context, []WorkReexportReceipt) error) error {
	return (&StrmExportDAO{Conn: fixture.connection}).WorkReexportMetadata(ctx, fixture.root, fixture.works, 1, run, fixture.targets, verify)
}

func TestShareWorkReexportPostgres(test *testing.T) {
	database := baselineIsolatedPostgres(test)
	test.Run("movie-tv-shared-file-split-and-idempotent", func(test *testing.T) {
		fixture := newWorkReexportPostgresFixture(test, database)
		before := fixture.counts(test)
		if before[5] != 1 {
			test.Fatalf("unchanged baseline SQL 必须检出共享 payload 的 tv 错配: %v", before)
		}
		invariantTables := []string{"t_share_record", "t_share_media", "t_share_media_file", "t_share_media_file_episode", "t_share_export_candidate", "t_share_export_candidate_item", "t_strm_export_history", "t_share_export_dirty_work", "t_share_export_consumer"}
		invariants := fixture.snapshot(test, invariantTables...)
		oldMapping := fixture.snapshot(test, "t_share_strm")[0]
		fileInfos := make([]os.FileInfo, len(fixture.targets))
		oldStates := make([]ExportState, len(fixture.targets))
		for index, target := range fixture.targets {
			state := &oldStates[index]
			if err := fixture.connection.QueryRowContext(context.Background(), workReexportStateSQL, target.Selection.ItemKey).Scan(&state.Path, &state.Content, &state.Mapping, &state.Playback, &state.Run); err != nil {
				test.Fatal(err)
			}
			var err error
			fileInfos[index], err = os.Stat(state.Path)
			if err != nil {
				test.Fatal(err)
			}
		}
		for attempt := 0; attempt < 2; attempt++ {
			checkpoint := &ShareExportCheckpointDAO{outputConn: fixture.connection}
			if err := checkpoint.WorkReexportPreflight(context.Background(), fixture.root, fixture.works, fixture.legacy, fixture.selections); err != nil {
				test.Fatalf("双边闭包必须通过真实 CTE: %v", err)
			}
			calls := 0
			if err := fixture.reexport(context.Background(), "same-run", func(ctx context.Context, receipts []WorkReexportReceipt) error {
				calls++
				return fixture.verify(ctx, receipts)
			}); err != nil || calls != 2 {
				test.Fatalf("真实事务必须成功且前后验证文件: calls=%d err=%v", calls, err)
			}
			after := fixture.counts(test)
			if after[5] != 0 || !reflect.DeepEqual(before[:5], after[:5]) {
				test.Fatalf("unchanged baseline invalid 必须自然归零，其他计数不改: before=%v after=%v", before, after)
			}
			if !reflect.DeepEqual(invariants, fixture.snapshot(test, invariantTables...)) {
				test.Fatal("来源、候选、dirty、consumer、历史不可修改")
			}
			var oldAfter string
			if err := fixture.connection.QueryRowContext(context.Background(), `SELECT jsonb_build_array(to_jsonb(mapping))::text FROM t_share_strm mapping WHERE id=$1`, fixture.legacy[0].ID).Scan(&oldAfter); err != nil || oldAfter != oldMapping {
				test.Fatalf("旧 payload/updated_at 不得覆盖: %v", err)
			}
			for index, target := range fixture.targets {
				expected, err := domain.BuildShareWorkReexportEntry(target.Source, target.Selection.ItemKey)
				if err != nil {
					test.Fatal(err)
				}
				var state ExportState
				if err = fixture.connection.QueryRowContext(context.Background(), workReexportStateSQL, target.Selection.ItemKey).Scan(&state.Path, &state.Content, &state.Mapping, &state.Playback, &state.Run); err != nil {
					test.Fatal(err)
				}
				var payload []byte
				if err = fixture.connection.QueryRowContext(context.Background(), `SELECT payload FROM t_share_strm WHERE id=$1`, state.Playback).Scan(&payload); err != nil {
					test.Fatal(err)
				}
				var actual domain.ShareStrmEntry
				if err = json.Unmarshal(payload, &actual); err != nil || !reflect.DeepEqual(expected, actual) || state.Playback == fixture.legacy[0].ID || state.Path != oldStates[index].Path || state.Content != oldStates[index].Content {
					test.Fatalf("拆分映射、路径、内容凭证不正确: %v %+v", err, state)
				}
				raw, err := json.Marshal(expected)
				if err != nil || state.Mapping != fmt.Sprintf("%x", sha256.Sum256(raw)) {
					test.Fatal("mapping fingerprint 必须来自实际新 payload")
				}
				info, err := os.Stat(state.Path)
				if err != nil || !os.SameFile(info, fileInfos[index]) || info.Mode() != fileInfos[index].Mode() || !info.ModTime().Equal(fileInfos[index].ModTime()) {
					test.Fatal("文件 inode/mode/mtime 必须保留")
				}
				var exported int64
				if err = fixture.connection.QueryRowContext(context.Background(), `SELECT exported_revision FROM t_share_media_selection WHERE media_item_key=$1`, target.Selection.ItemKey).Scan(&exported); err != nil || exported != target.Selection.Revision {
					test.Fatal("必须在同事务确认选择版本")
				}
			}
			var total, distinct, legacyActive int
			if err := fixture.connection.QueryRowContext(context.Background(), `SELECT count(*),count(DISTINCT share_strm_id),count(*) FILTER(WHERE share_strm_id=$1) FROM t_strm_export_state WHERE state='active'`, fixture.legacy[0].ID).Scan(&total, &distinct, &legacyActive); err != nil || total != 2 || distinct != 2 || legacyActive != 0 {
				test.Fatalf("每 item 独立，不能遗留旧 active 引用: %v", err)
			}
			var mappings int
			if err := fixture.connection.QueryRowContext(context.Background(), `SELECT count(*) FROM t_share_strm`).Scan(&mappings); err != nil || mappings != 3 {
				test.Fatal("重复运行不可新增重复映射")
			}
			test.Logf("attempt=%d unchanged baseline counts before=%v after=%v active=%d distinct=%d old_active=%d mappings=%d", attempt+1, before, after, total, distinct, legacyActive, mappings)
		}
	})
	for _, scenario := range []string{"one-work", "unprocessed-export", "missing-selection", "empty-work", "C2_output_path", "C4_stable_relative_path", "C5_history_owner", "C6_local_strm_path", "null-playback", "empty-playback", "new-item-reference", "old-state-reference", "url-reference", "empty-selection", "empty-scope"} {
		test.Run(scenario, func(test *testing.T) {
			fixture := newWorkReexportPostgresFixture(test, database)
			reason := scenario
			switch scenario {
			case "one-work", "unprocessed-export":
				fixture.targets, fixture.legacy, fixture.selections = fixture.targets[:1], fixture.legacy[:1], fixture.selections[:1]
				if scenario == "one-work" {
					fixture.works, reason = fixture.works[:1], "outside_work_scope"
				} else {
					reason = "unprocessed_export_key"
				}
			case "missing-selection":
				fixture.exec(test, `DELETE FROM t_share_media_selection WHERE work_key='tmdb:tv:8'`)
				reason = "missing_selection"
			case "empty-work":
				fixture.exec(test, `ALTER TABLE t_share_media_selection DROP CONSTRAINT reexport_nonempty_work, DROP CONSTRAINT reexport_member; UPDATE t_share_media_selection SET work_key='' WHERE work_key='tmdb:tv:8'; UPDATE t_strm_export_state SET export_key=':1:1' WHERE export_key='tmdb:tv:8:1:1'`)
				reason = "empty_work_key"
			case "C2_output_path":
				fixture.exec(test, `INSERT INTO t_strm_export_state(owner_key,export_key,output_path,state) VALUES('share:default','tmdb:tv:8:1:1',$1,'stale') ON CONFLICT(owner_key,export_key) DO UPDATE SET output_path=$1,state='stale'`, filepath.Join(fixture.root, fixture.selections[0].RelativePath))
			case "C4_stable_relative_path":
				fixture.exec(test, `UPDATE t_share_media_selection SET stable_relative_path=$1 WHERE work_key='tmdb:tv:8'`, fixture.selections[0].RelativePath)
			case "C5_history_owner":
				fixture.exec(test, `INSERT INTO t_strm_export_history VALUES('foreign',$1,'fixture')`, filepath.Join(fixture.root, fixture.selections[0].RelativePath))
			case "C6_local_strm_path":
				fixture.exec(test, `DELETE FROM t_strm_export_state WHERE export_key='tmdb:movie:7:0:0'`)
				fixture.exec(test, `INSERT INTO t_strm_file VALUES(42,$1)`, filepath.Join(fixture.root, fixture.selections[0].RelativePath))
			case "null-playback", "empty-playback":
				var playback interface{}
				if scenario == "empty-playback" {
					playback = ""
				}
				fixture.exec(test, `UPDATE t_strm_export_state SET share_strm_id=$1 WHERE export_key='tmdb:movie:7:0:0'`, playback)
				reason = "播放 mapping ID 为空"
			case "new-item-reference", "old-state-reference", "url-reference":
				mapping := domain.ShareWorkReexportMappingID(fixture.legacy[0].ID, fixture.selections[0].ItemKey)
				if scenario != "new-item-reference" {
					mapping = "11111111-1111-5111-8111-111111111111"
					if scenario == "old-state-reference" {
						fixture.exec(test, `UPDATE t_strm_export_state SET share_strm_id=$1 WHERE export_key='tmdb:movie:7:0:0'`, mapping)
					} else {
						fixture.targets[0].URLMappingID = mapping
					}
				}
				fixture.exec(test, `INSERT INTO t_share_media_file_episode VALUES(12,1,2); INSERT INTO t_share_export_candidate_item(candidate_id,work_key,season_number,episode_number) VALUES(2,'tmdb:tv:8',1,2); INSERT INTO t_share_media_selection(work_key,season_number,episode_number,selected_candidate_id,stable_relative_path) VALUES('tmdb:tv:8',1,2,2,'extra.strm')`)
				fixture.exec(test, `INSERT INTO t_strm_export_state(owner_key,export_key,output_path,share_strm_id) VALUES('share:default','tmdb:tv:8:1:2',$1,$2)`, filepath.Join(fixture.root, "extra.strm"), mapping)
				reason = "unprocessed_export_key"
			case "empty-selection":
				fixture.targets, fixture.legacy, fixture.selections = nil, nil, nil
			case "empty-scope":
				fixture.works = nil
			}
			tables := []string{"t_share_strm", "t_strm_export_state", "t_share_media_selection", "t_strm_export_history", "t_share_export_dirty_work"}
			before := fixture.snapshot(test, tables...)
			urls := make([]string, len(fixture.targets))
			for index, target := range fixture.targets {
				urls[index] = target.URLMappingID
			}
			preflightErr := workReexportPreflight(context.Background(), fixture.connection, fixture.root, fixture.works, fixture.legacy, fixture.selections, urls)
			if scenario != "null-playback" && scenario != "empty-playback" && preflightErr == nil {
				test.Fatal("真实 CTE/入参保护必须拒绝")
			}
			if preflightErr != nil && scenario != "empty-selection" && scenario != "empty-scope" && !strings.Contains(preflightErr.Error(), reason) {
				test.Fatalf("真实 CTE 缺少拒绝原因 %s: %v", reason, preflightErr)
			}
			err := fixture.reexport(context.Background(), "rejected", fixture.verify)
			if err == nil || (scenario != "empty-selection" && scenario != "empty-scope" && !errors.Is(err, ErrShareExportChanged)) {
				test.Fatalf("真实事务必须拒绝并归一业务错误: %v", err)
			}
			if !reflect.DeepEqual(before, fixture.snapshot(test, tables...)) {
				test.Fatal("拒绝必须保持全部元数据")
			}
			test.Logf("isolated rejection: %v", err)
		})
	}
	for _, scenario := range []string{"second-insert-failure", "final-verification-failure"} {
		test.Run(scenario, func(test *testing.T) {
			fixture := newWorkReexportPostgresFixture(test, database)
			if scenario == "second-insert-failure" {
				fixture.exec(test, `CREATE FUNCTION pg_temp.reexport_fail_tv() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF (NEW.payload->>'media_id')::integer=8 THEN RAISE EXCEPTION 'isolated second mapping failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reexport_fail_tv BEFORE INSERT ON t_share_strm FOR EACH ROW EXECUTE FUNCTION pg_temp.reexport_fail_tv()`)
			}
			tables := []string{"t_share_strm", "t_strm_export_state", "t_share_media_selection", "t_strm_export_history"}
			before := fixture.snapshot(test, tables...)
			calls := 0
			err := fixture.reexport(context.Background(), "rollback", func(ctx context.Context, receipts []WorkReexportReceipt) error {
				calls++
				if calls == 2 && scenario == "final-verification-failure" {
					return ErrShareExportChanged
				}
				return fixture.verify(ctx, receipts)
			})
			if err == nil || !reflect.DeepEqual(before, fixture.snapshot(test, tables...)) || fixture.counts(test)[5] != 1 {
				test.Fatalf("中途/提交前失败必须由实际 PostgreSQL 全批回滚: %v", err)
			}
			if scenario == "second-insert-failure" {
				if !strings.Contains(err.Error(), "isolated second mapping failure") || calls != 1 {
					test.Fatalf("必须已通过电影写后回读，再在第二映射失败: calls=%d err=%v", calls, err)
				}
				fixture.exec(test, `DROP TRIGGER reexport_fail_tv ON t_share_strm; DROP FUNCTION pg_temp.reexport_fail_tv()`)
			} else if calls != 2 {
				test.Fatal("必须已写完整批次后才拒绝提交")
			}
			if err = fixture.reexport(context.Background(), "retry", fixture.verify); err != nil || fixture.counts(test)[5] != 0 {
				test.Fatalf("回滚后必须可重新显式执行: %v", err)
			}
			test.Logf("actual rollback and explicit retry passed: %s", scenario)
		})
	}
	test.Run("actual-reference-limit", func(test *testing.T) {
		fixture := newWorkReexportPostgresFixture(test, database)
		fixture.exec(test, `DELETE FROM t_strm_export_state; DELETE FROM t_share_media_selection; INSERT INTO t_share_export_candidate_item(candidate_id,work_key,season_number,episode_number) SELECT 2,'tmdb:tv:8',1,number FROM generate_series(1,10000) number ON CONFLICT DO NOTHING; INSERT INTO t_share_media_selection(work_key,season_number,episode_number,selected_candidate_id,stable_relative_path) SELECT 'tmdb:tv:8',1,number,2,'cap/'||number||'.strm' FROM generate_series(1,10000) number`)
		fixture.exec(test, `INSERT INTO t_strm_export_state(owner_key,export_key,output_path,share_strm_id) SELECT 'share:default',media_item_key,$1||'/'||stable_relative_path,md5(media_item_key)::uuid::text FROM t_share_media_selection`, fixture.root)
		rows, err := fixture.connection.QueryContext(context.Background(), `SELECT selection.media_item_key,selection.stable_relative_path,output.share_strm_id FROM t_share_media_selection selection JOIN t_strm_export_state output ON output.export_key=selection.media_item_key ORDER BY selection.media_item_key`)
		if err != nil {
			test.Fatal(err)
		}
		entries, selections := []domain.ShareStrmEntry{}, []domain.ShareSelection{}
		for rows.Next() {
			var entry domain.ShareStrmEntry
			var selection domain.ShareSelection
			if err = rows.Scan(&selection.ItemKey, &selection.RelativePath, &entry.ID); err != nil {
				test.Fatal(err)
			}
			entry.MediaID, selection.WorkKey = 8, "tmdb:tv:8"
			entries, selections = append(entries, entry), append(selections, selection)
		}
		if err = errors.Join(rows.Err(), rows.Close()); err != nil || len(entries) != domain.ShareWorkReexportMaxSelections {
			test.Fatalf("需要实际一万条 item fixture: %d %v", len(entries), err)
		}
		ctx := context.Background()
		if err = workReexportPreflight(ctx, fixture.connection, fixture.root, []string{"tmdb:tv:8"}, entries, selections); err != nil {
			test.Fatalf("一万 item 的两万条 state+selection 应通过: %v", err)
		}
		fixture.exec(test, `INSERT INTO t_strm_export_history VALUES('foreign',$1,'cap')`, filepath.Join(fixture.root, selections[0].RelativePath))
		err = workReexportPreflight(ctx, fixture.connection, fixture.root, []string{"tmdb:tv:8"}, entries, selections)
		if !errors.Is(err, ErrShareExportChanged) || !strings.Contains(err.Error(), "references_total=20001") || !strings.Contains(err.Error(), "reference_limit") || !strings.Contains(err.Error(), "samples=100 truncated=true") {
			test.Fatalf("实际超限必须由 CTE 计数拒绝，并返回有界样本: %v", err)
		}
		test.Logf("actual 10000 items / 20000 references accepted; extra history rejected: %v", err)
	})
}
