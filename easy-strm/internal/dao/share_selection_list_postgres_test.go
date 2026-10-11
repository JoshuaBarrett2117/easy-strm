package dao

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"easy-strm/internal/domain"
	"github.com/lib/pq"
)

const selectionListFixtureSQL = `
CREATE TABLE t_share_media(id integer PRIMARY KEY,work_key text,media_type text,title text);
CREATE TABLE t_share_record(id integer PRIMARY KEY,share_cancelled boolean);
CREATE TABLE t_share_media_file(id integer PRIMARY KEY,share_id integer,media_id integer,available boolean,status text);
CREATE TABLE t_share_media_file_episode(file_id integer,season_number integer,episode_number integer);
CREATE TABLE t_share_export_candidate(candidate_id bigint PRIMARY KEY,source_file_id integer,available boolean,state text);
CREATE TABLE t_share_export_candidate_item(candidate_id bigint,work_key text,season_number integer,episode_number integer,revoked boolean);
CREATE TABLE t_share_media_selection(media_item_key text PRIMARY KEY,work_key text,season_number integer,episode_number integer,selected_candidate_id bigint,selection_mode text,selection_revision bigint,exported_revision bigint,stable_relative_path text);
INSERT INTO t_share_media SELECT number,'work:'||lpad(number::text,2,'0'),'movie','Title '||number FROM generate_series(1,25) number;
INSERT INTO t_share_record SELECT number,false FROM generate_series(1,25) number;
INSERT INTO t_share_media_file SELECT number,number,number,true,'identified' FROM generate_series(1,25) number;
INSERT INTO t_share_export_candidate SELECT number,number,true,'active' FROM generate_series(1,25) number;
INSERT INTO t_share_export_candidate_item SELECT id,work_key,0,0,false FROM t_share_media;
INSERT INTO t_share_media_selection SELECT work_key||':0:0',work_key,0,0,id,CASE WHEN id%2=0 THEN 'manual' ELSE 'auto' END,id+3,id,'movie/'||id||'.strm' FROM t_share_media;
UPDATE t_share_export_candidate_item SET revoked=true WHERE candidate_id=2;
UPDATE t_share_export_candidate SET available=false WHERE candidate_id=3;
UPDATE t_share_export_candidate SET state='removed' WHERE candidate_id=4;
UPDATE t_share_media_file SET available=false WHERE id=5;
UPDATE t_share_record SET share_cancelled=true WHERE id=6;
UPDATE t_share_media_file SET status='pending' WHERE id=7;
UPDATE t_share_media_file SET media_id=1 WHERE id=8;
DELETE FROM t_share_media_file WHERE id=9;
DELETE FROM t_share_record WHERE id=10;
UPDATE t_share_media SET title=NULL WHERE id=11;
DELETE FROM t_share_media WHERE id=12;
DELETE FROM t_share_export_candidate WHERE candidate_id=13;
DELETE FROM t_share_export_candidate_item WHERE candidate_id=14;
UPDATE t_share_media SET title='' WHERE id=15;
UPDATE t_share_media SET title=$title$Literal %_\'Film$title$ WHERE id=16;
UPDATE t_share_media SET media_type='tv',title='Series' WHERE id=17;
DELETE FROM t_share_export_candidate_item WHERE candidate_id=17;
DELETE FROM t_share_media_selection WHERE selected_candidate_id=17;
INSERT INTO t_share_export_candidate_item VALUES(17,'work:17',1,1,false),(17,'work:17',1,2,false),(17,'work:17',2,1,false),(17,'work:17',2,2,true);
INSERT INTO t_share_media_file_episode VALUES(17,1,1),(17,1,1),(17,2,1);
INSERT INTO t_share_media_selection SELECT 'work:17:'||season_number||':'||episode_number,work_key,season_number,episode_number,17,'manual',41+episode_number,40,'tv/'||season_number||'-'||episode_number||'.strm' FROM t_share_export_candidate_item WHERE candidate_id=17;
INSERT INTO t_share_export_candidate_item VALUES(18,'work:18',1,1,false);
INSERT INTO t_share_media_selection VALUES('work:18:1:1','work:18',1,1,18,'auto',50,49,'invalid-movie-episode.strm');
UPDATE t_share_media SET media_type='other' WHERE id=19;
UPDATE t_share_media_file SET status=NULL WHERE id=20;
UPDATE t_share_export_candidate_item SET revoked=NULL WHERE candidate_id=21;
UPDATE t_share_export_candidate SET available=NULL WHERE candidate_id=22;
UPDATE t_share_media_file SET available=NULL WHERE id=23;
UPDATE t_share_record SET share_cancelled=NULL WHERE id=24;
UPDATE t_share_media SET work_key=NULL WHERE id=25;`

type selectionSnapshotConnector struct {
	driver.Connector
	snapshot string
}

type selectionSnapshotConn struct {
	driver.Conn
	snapshot string
}

func (connector selectionSnapshotConnector) Connect(ctx context.Context) (driver.Conn, error) {
	connection, err := connector.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return selectionSnapshotConn{Conn: connection, snapshot: connector.snapshot}, nil
}

func (connection selectionSnapshotConn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	if options.Isolation != driver.IsolationLevel(sql.LevelRepeatableRead) || !options.ReadOnly {
		return nil, fmt.Errorf("DAO must request read-only repeatable read: %+v", options)
	}
	transaction, err := connection.Conn.(driver.ConnBeginTx).BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	_, err = connection.Conn.(driver.ExecerContext).ExecContext(ctx, `SET TRANSACTION SNAPSHOT `+pq.QuoteLiteral(connection.snapshot)+`;
DO $$ BEGIN
 IF current_setting('transaction_isolation')<>'repeatable read' OR current_setting('transaction_read_only')<>'on' THEN
  RAISE EXCEPTION 'DAO transaction settings changed';
 END IF;
END $$;`, nil)
	if err != nil {
		_ = transaction.Rollback()
		return nil, err
	}
	return transaction, nil
}

func frozenSelectionList(ctx context.Context, transaction *sql.Tx, query domain.ShareSelectionQuery) ([]domain.ShareSelection, int, error) {
	var total int
	if !query.SkipCount {
		if err := transaction.QueryRowContext(ctx, `SELECT count(*)`+frozenSelectionFilter, query.Keyword).Scan(&total); err != nil {
			return nil, 0, err
		}
	}
	args := []interface{}{query.Keyword, query.PageSize}
	if query.Cursor {
		args = append(args, query.AfterWork, query.AfterSeason, query.AfterEpisode)
	} else {
		args = append(args, (query.Page-1)*query.PageSize)
	}
	rows, err := transaction.QueryContext(ctx, frozenSelectionListSQL(query.Cursor), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	values := []domain.ShareSelection{}
	for rows.Next() {
		var value domain.ShareSelection
		if err := rows.Scan(&value.ItemKey, &value.WorkKey, &value.Season, &value.Episode, &value.CandidateID, &value.Mode, &value.Revision, &value.ExportedRevision, &value.RelativePath, &value.Title, &value.Valid); err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, rows.Err()
}

// TestSelectionListPostgresEquivalence 在 TMPDIR 全新集群中，将实际 DAO 与冻结 HEAD SQL 在同一只读快照逐字段对比。
func TestSelectionListPostgresEquivalence(test *testing.T) {
	database := baselineIsolatedPostgres(test)
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, selectionListFixtureSQL); err != nil {
		test.Fatal(err)
	}
	if path := os.Getenv("ESTRM_SELECTION_PERF_SQL_FILE"); path != "" {
		contents, err := os.ReadFile(path)
		if err != nil {
			test.Fatal(err)
		}
		for _, prepared := range []struct {
			name  string
			types string
			query string
		}{
			{"old_count", "text", `SELECT count(*)` + frozenSelectionFilter},
			{"new_count", "text", `SELECT count(*)` + frozenSelectionFilter},
			{"old_offset", "text, integer, integer", frozenSelectionListSQL(false)},
			{"new_offset", "text, integer, integer", expectedSelectionListSQL(false)},
			{"old_cursor", "text, integer, text, integer, integer", frozenSelectionListSQL(true)},
			{"new_cursor", "text, integer, text, integer, integer", expectedSelectionListSQL(true)},
		} {
			statement := "PREPARE selection_perf_" + prepared.name + "(" + prepared.types + ") AS " + prepared.query + ";"
			if !strings.Contains(strings.Join(strings.Fields(string(contents)), " "), strings.Join(strings.Fields(statement), " ")) {
				test.Fatalf("SQL artifact differs from tested query: %s", prepared.name)
			}
		}
		if _, err := database.ExecContext(ctx, string(contents)); err != nil {
			test.Fatalf("SQL artifact execution on disposable fixture: %v", err)
		}
		test.Log("full old/new SQL artifact executed on disposable PostgreSQL fixture")
	}
	var socket, port string
	if err := database.QueryRowContext(ctx, `SELECT current_setting('unix_socket_directories'),current_setting('port')`).Scan(&socket, &port); err != nil {
		test.Fatal(err)
	}
	transaction, err := database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		test.Fatal(err)
	}
	defer transaction.Rollback()
	var snapshot string
	if err := transaction.QueryRowContext(ctx, `SELECT pg_export_snapshot()`).Scan(&snapshot); err != nil {
		test.Fatal(err)
	}
	connector, err := pq.NewConnector(fmt.Sprintf("host='%s' port=%s user=baseline_test dbname=postgres sslmode=disable", strings.ReplaceAll(socket, "'", "\\'"), port))
	if err != nil {
		test.Fatal(err)
	}
	daoDatabase := sql.OpenDB(selectionSnapshotConnector{Connector: connector, snapshot: snapshot})
	defer daoDatabase.Close()
	daoDatabase.SetMaxOpenConns(1)
	store := NewShareRecordDAO(daoDatabase)
	allQuery := domain.ShareSelectionQuery{Page: 1, PageSize: 100}
	all, total, err := frozenSelectionList(ctx, transaction, allQuery)
	if err != nil || len(all) != 29 || total != 29 {
		test.Fatalf("fixture rows=%d total=%d err=%v", len(all), total, err)
	}
	validKeys := map[string]bool{"work:01:0:0": true, "work:11:0:0": true, "work:15:0:0": true, "work:16:0:0": true, "work:17:1:1": true, "work:17:2:1": true, "work:18:0:0": true}
	for _, value := range all {
		if value.Valid != validKeys[value.ItemKey] {
			test.Fatalf("unexpected fixture validity: %+v", value)
		}
		if (value.WorkKey == "work:11" || value.WorkKey == "work:12" || value.WorkKey == "work:25") && value.Title != value.WorkKey {
			test.Fatalf("missing title fallback: %+v", value)
		}
	}
	writer := sql.OpenDB(connector)
	defer writer.Close()
	if _, err := writer.ExecContext(ctx, `UPDATE t_share_media SET title='changed after snapshot' WHERE id=1;
UPDATE t_share_export_candidate_item SET revoked=true WHERE candidate_id=1;
INSERT INTO t_share_media_selection VALUES('aaa:0:0','aaa',0,0,1,'manual',1,0,'later.strm');`); err != nil {
		test.Fatal(err)
	}
	cases := []struct {
		name  string
		query domain.ShareSelectionQuery
	}{
		{"all_validity_cases", allQuery},
		{"first_page", domain.ShareSelectionQuery{Page: 1, PageSize: 3}},
		{"second_page", domain.ShareSelectionQuery{Page: 2, PageSize: 3}},
		{"deep_offset", domain.ShareSelectionQuery{Page: 1000000, PageSize: 100}},
		{"skip_count", domain.ShareSelectionQuery{Page: 3, PageSize: 5, SkipCount: true}},
		{"keyword_case", domain.ShareSelectionQuery{Page: 1, PageSize: 4, Keyword: "tItLe"}},
		{"keyword_second_page", domain.ShareSelectionQuery{Page: 2, PageSize: 4, Keyword: "Title"}},
		{"keyword_literal", domain.ShareSelectionQuery{Page: 1, PageSize: 3, Keyword: `%_\'fIlM`}},
		{"keyword_percent", domain.ShareSelectionQuery{Page: 1, PageSize: 3, Keyword: "%"}},
		{"keyword_underscore", domain.ShareSelectionQuery{Page: 1, PageSize: 3, Keyword: "_"}},
		{"keyword_backslash", domain.ShareSelectionQuery{Page: 1, PageSize: 3, Keyword: `\`}},
		{"keyword_quote", domain.ShareSelectionQuery{Page: 1, PageSize: 3, Keyword: "'"}},
		{"keyword_item_missing_title", domain.ShareSelectionQuery{Page: 1, PageSize: 3, Keyword: "WORK:12"}},
		{"keyword_missing", domain.ShareSelectionQuery{Page: 1, PageSize: 3, Keyword: "absent"}},
		{"cursor_count", domain.ShareSelectionQuery{Cursor: true, PageSize: 4, AfterWork: "work:16"}},
		{"cursor_skip_count", domain.ShareSelectionQuery{Cursor: true, SkipCount: true, PageSize: 4, AfterWork: "work:16"}},
		{"cursor_within_season", domain.ShareSelectionQuery{Cursor: true, PageSize: 2, AfterWork: "work:17", AfterSeason: 1, AfterEpisode: 1}},
		{"cursor_next_season", domain.ShareSelectionQuery{Cursor: true, PageSize: 2, AfterWork: "work:17", AfterSeason: 1, AfterEpisode: 2}},
		{"cursor_keyword", domain.ShareSelectionQuery{Cursor: true, PageSize: 2, Keyword: "Series", AfterWork: "work:17", AfterSeason: 1, AfterEpisode: 1}},
		{"cursor_keyword_skip_count", domain.ShareSelectionQuery{Cursor: true, SkipCount: true, PageSize: 2, Keyword: "%_", AfterWork: "work:15"}},
		{"cursor_empty", domain.ShareSelectionQuery{Cursor: true, PageSize: 3, AfterWork: "zzz"}},
		{"cursor_empty_skip_count", domain.ShareSelectionQuery{Cursor: true, SkipCount: true, PageSize: 3, AfterWork: "zzz"}},
	}
	for _, scenario := range cases {
		test.Run(scenario.name, func(test *testing.T) {
			before, beforeTotal, err := frozenSelectionList(ctx, transaction, scenario.query)
			if err != nil {
				test.Fatal(err)
			}
			after, afterTotal, err := store.ListSelections(ctx, scenario.query)
			if err != nil {
				test.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) || beforeTotal != afterTotal {
				test.Fatalf("old rows=%+v total=%d; actual DAO rows=%+v total=%d", before, beforeTotal, after, afterTotal)
			}
			if scenario.query.SkipCount && afterTotal != 0 {
				test.Fatal("SkipCount returned a nonzero total")
			}
		})
	}
}
