package main

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestShareExportMigrationExplicitTransaction(t *testing.T) {
	for _, failure := range []string{"none", "begin", "exec", "commit"} {
		t.Run(failure, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if failure == "begin" {
				mock.ExpectBegin().WillReturnError(errors.New("begin failed"))
			} else {
				mock.ExpectBegin()
				if failure == "exec" {
					mock.ExpectExec("CREATE TABLE IF NOT EXISTS t_share_export_source_state").WillReturnError(errors.New("migration failed"))
					mock.ExpectRollback()
				} else {
					mock.ExpectExec("CREATE TABLE IF NOT EXISTS t_share_export_source_state").WillReturnResult(sqlmock.NewResult(0, 0))
					if failure == "commit" {
						mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
					} else {
						mock.ExpectCommit()
					}
				}
			}
			err = applyShareExportCheckpointMigration(context.Background(), db)
			if (failure == "none") != (err == nil) {
				t.Fatalf("%s: %v", failure, err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShareExportMigrationNotInStartupAndConditions(t *testing.T) {
	startup, err := os.ReadFile("share_record_migration.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(startup), "shareExportCheckpointMigrationSQL") || strings.Contains(string(startup), "applyShareExportCheckpointMigration") {
		t.Fatal("v43 被加入启动迁移")
	}
	for _, required := range []string{"acked_revision>=0 AND acked_revision<=revision", "pending_export_keys <> '[]'::jsonb", "WHERE revision > acked_revision", "jsonb_path_ops", "REFERENCING OLD TABLE AS old_rows NEW TABLE AS new_rows", "o.version,o.available,o.media_id,o.file_name,o.file_id,o.status,o.result,o.metadata_source,o.share_id", "share_strm_settings", "movie_naming_template", "tv_naming_template"} {
		if !strings.Contains(shareExportCheckpointMigrationSQL, required) {
			t.Fatalf("迁移未固化条件：%s", required)
		}
	}
	if shareExportColumnTriggerPattern.MatchString(shareExportCheckpointMigrationSQL) || strings.Contains(shareExportCheckpointMigrationSQL, "DELETE FROM t_share_export_dirty_work") || strings.Contains(shareExportCheckpointMigrationSQL, "FOR EACH ROW") {
		t.Fatal("迁移包含被禁止的 trigger/ack 写法")
	}
	for _, required := range []string{"DO $prerequisites$", "t_share_operation_queue", "share_export_checkpoint_invalidated", "legacy_outputs_reconciled=FALSE", "ORDER BY dirty.work_key FOR UPDATE OF dirty", "source_match_sql", "ON CONFLICT DO NOTHING"} {
		if !strings.Contains(shareExportCheckpointMigrationSQL, required) {
			t.Fatalf("迁移未固化评审要求：%s", required)
		}
	}
}

var shareExportColumnTriggerPattern = regexp.MustCompile(`(?i)(?:AFTER|BEFORE|INSTEAD\s+OF)\s+UPDATE\s+OF|['"]UPDATE\s+OF`)

// TestShareExportMigrationColumnTriggerGuard 区分被禁止的触发器列清单与合法的有序行锁。
func TestShareExportMigrationColumnTriggerGuard(t *testing.T) {
	for _, scenario := range []struct {
		sql       string
		forbidden bool
	}{
		{"CREATE TRIGGER x AFTER UPDATE OF available ON f REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT", true},
		{"CREATE TRIGGER x BEFORE UPDATE OF media_id ON f", true},
		{"ARRAY['UPDATE OF available']", true},
		{"SELECT dirty.work_key ORDER BY dirty.work_key FOR UPDATE OF dirty", false},
	} {
		if shareExportColumnTriggerPattern.MatchString(scenario.sql) != scenario.forbidden {
			t.Fatalf("错误识别触发器/行锁：%s", scenario.sql)
		}
	}
}

// TestShareExportMigrationRepeatedExplicitCalls 验证显式工具允许复跑相同 SQL；真实状态幂等由 DBA 验证。
func TestShareExportMigrationRepeatedExplicitCalls(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for attempt := 0; attempt < 2; attempt++ {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(shareExportCheckpointMigrationSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		if err = applyShareExportCheckpointMigration(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestShareExportMigrationNoRuntimeCaller 阻止任何非测试启动文件调用显式 v43 工具。
func TestShareExportMigrationNoRuntimeCaller(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") || entry.Name() == "share_export_migration.go" {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && (identifier.Name == "applyShareExportCheckpointMigration" || identifier.Name == "shareExportCheckpointMigrationSQL") {
				t.Errorf("启动文件 %s 引用了显式 v43 工具", entry.Name())
			}
			return true
		})
	}
}

// TestShareExportRollbackRepeatMissingCheckpointTables 验证第二次 rollback 跳过已移除的检查点关系。
func TestShareExportRollbackRepeatMissingCheckpointTables(t *testing.T) {
	sql, err := os.ReadFile("migrations/migrate_v43_share_export_checkpoint_rollback.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sql), "IF to_regclass(relation) IS NULL THEN CONTINUE; END IF;") {
		t.Fatal("第二次 rollback 对已删除关系执行 DROP TRIGGER ON 将失败，即使 trigger 带 IF EXISTS")
	}
	if strings.Contains(string(sql), "CASCADE") || strings.Contains(string(sql), "DROP INDEX") {
		t.Fatal("rollback 不得级联或删除预建共享索引")
	}
}
