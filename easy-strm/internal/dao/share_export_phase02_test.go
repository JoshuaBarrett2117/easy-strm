package dao

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

// TestStrmExportLegacyPathOwnership 验证未进入新清单的逐路径历史归属仍能阻止接管。
func TestStrmExportLegacyPathOwnership(t *testing.T) {
	for _, owner := range []string{"cloud115:9", "share:default"} {
		t.Run(owner, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			conn, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if !strings.Contains(strmExportCheckPathSQL, "FROM t_strm_file WHERE local_strm_path=$1") || !strings.Contains(strmExportCheckPathSQL, "strm_config_id=-1") {
				t.Fatal("缺少逐路径旧清单所有者检查")
			}
			mock.ExpectQuery(regexp.QuoteMeta(strmExportCheckPathSQL)).WithArgs("/synthetic/owned.strm").WillReturnRows(sqlmock.NewRows([]string{"owner", "key"}).AddRow(owner, ""))
			owned, err := (&StrmExportDAO{Conn: conn}).CheckPath(context.Background(), "/synthetic/owned.strm", "share:default", "new-key")
			if owner == "share:default" {
				if err != nil || !owned {
					t.Fatalf("本归属历史路径未识别：%v %v", owned, err)
				}
			} else if err == nil || owned {
				t.Fatal("其他 owner 的历史路径被接管")
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestShareExportCheckpointPrepareRejectsUntrusted 验证旧全量凭证不能由 fanout 自行建立。
func TestShareExportCheckpointPrepareRejectsUntrusted(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT config_revision,prepared_revision.*legacy_outputs_reconciled.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"revision", "prepared", "fingerprint", "state", "protocol", "reconciled"}).AddRow(2, 0, "", "required", 1, false))
	mock.ExpectRollback()
	if err = NewShareExportCheckpointDAO(db).Prepare(context.Background(), domain.ShareExportInput{ConfigRevision: 2}, "new"); !errors.Is(err, ErrShareExportBaselineRequired) {
		t.Fatalf("不可信 fanout：%v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestShareExportCheckpointReadInputSnapshot 验证配置、分类和可信度来自同一只读事务。
func TestShareExportCheckpointReadInputSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT config_revision,prepared_revision").WillReturnRows(sqlmock.NewRows([]string{"revision", "prepared", "fingerprint", "state", "protocol", "reconciled"}).AddRow(3, 2, "old", "required", 1, true))
	mock.ExpectQuery("SELECT config_key,config_val").WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow("share_strm_settings", `{"output_path":"/synthetic","base_url":"https://example.invalid","cloud115_id":1}`).AddRow("movie_naming_template", "original"))
	mock.ExpectQuery("SELECT id,name,media_type,target_path,match_rules").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "path", "rules", "enabled"}).AddRow(7, "synthetic", "movie", "movies", `{}`, true))
	mock.ExpectCommit()
	input, err := NewShareExportCheckpointDAO(db).ReadInput(context.Background())
	if err != nil || input.ConfigRevision != 3 || !input.LegacyOutputsReconciled || input.Settings.OutputPath != "/synthetic" || input.Templates["movie_naming_template"] != "original" || len(input.Categories) != 1 || input.Categories[0].ID != 7 {
		t.Fatalf("输入快照：%+v %v", input, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestShareExportCheckpointSourceUpsert 验证来源观察和真实去重后的键集合被提交，旧 stale 键不清空。
func TestShareExportCheckpointSourceUpsert(t *testing.T) {
	for _, stateName := range []string{"active", "stale"} {
		t.Run(stateName, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT config_revision.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(4))
			mock.ExpectQuery("SELECT revision,acked_revision.*FOR UPDATE").WithArgs("work").WillReturnRows(sqlmock.NewRows([]string{"revision", "acked"}).AddRow(9, 8))
			mock.ExpectExec("INSERT INTO t_share_export_source_state.*export_keys=CASE WHEN EXCLUDED.state='stale' THEN source.export_keys ELSE EXCLUDED.export_keys END").WithArgs(1, 2, 3, "work", 5, 6, true, false, "1:1,1:2", `["K1","K2"]`, "run", stateName).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("UPDATE t_share_export_source_state SET state='stale'").WithArgs("work", "{1}", "run").WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(regexp.QuoteMeta(shareExportAckSQL)).WithArgs("work", int64(9), "run").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			states := []domain.ShareExportSourceState{{SourceFileID: 1, ShareID: 2, MediaID: 3, WorkKey: "work", FileVersion: 5, ShareVersion: 6, Available: true, EpisodeSignature: "1:1,1:2", ExportKeys: []string{"K2", "K1", "K2"}, State: stateName}}
			if _, err = NewShareExportCheckpointDAO(db).CompleteWork(context.Background(), domain.ShareExportDirty{WorkKey: "work", Revision: 9}, 4, "run", states, ExportSnapshot{}); err != nil {
				t.Fatal(err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestShareExportCheckpointDeadlockRetainsPending 验证真实 SQLSTATE 的事务失败向上传播且不误 ack。
func TestShareExportCheckpointDeadlockRetainsPending(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	failure := &pq.Error{Code: "40P01", Message: "synthetic deadlock"}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT config_revision.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(1))
	mock.ExpectQuery("SELECT revision,acked_revision.*FOR UPDATE").WithArgs("work").WillReturnError(failure)
	mock.ExpectRollback()
	if _, err = NewShareExportCheckpointDAO(db).CompleteWork(context.Background(), domain.ShareExportDirty{WorkKey: "work", Revision: 7}, 1, "run", nil, nil); !errors.Is(err, failure) {
		t.Fatalf("死锁被吞掉：%v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
