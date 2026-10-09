package main

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func normalizeD1MigrationSQL(source string) string {
	var statements []string
	for _, line := range strings.Split(source, "\n") {
		statements = append(statements, strings.SplitN(line, "--", 2)[0])
	}
	return strings.Join(strings.Fields(strings.Join(statements, "\n")), " ")
}

// TestShareTransferLogMigrationSchemaContract 固定 DBA 列与两个索引契约，确认嵌入文件及危险回滚前置条件。
func TestShareTransferLogMigrationSchemaContract(t *testing.T) {
	file, err := os.ReadFile("migrations/migrate_v46_share_transfer_log.sql")
	if err != nil {
		t.Fatal(err)
	}
	if string(file) != shareTransferLogMigrationSQL {
		t.Fatal("启动嵌入 SQL 与 v46 文件不一致")
	}
	const approved = `CREATE TABLE IF NOT EXISTS t_share_transfer_log (
 id SERIAL PRIMARY KEY,
 task_id VARCHAR(64) NOT NULL,
 share_code VARCHAR(32) NOT NULL DEFAULT '',
 share_folder_name VARCHAR(512) NOT NULL DEFAULT '',
 file_name VARCHAR(512) NOT NULL DEFAULT '',
 file_pick_code VARCHAR(64) NOT NULL DEFAULT '',
 file_size BIGINT NOT NULL DEFAULT 0,
 file_sha1 VARCHAR(128) NOT NULL DEFAULT '',
 cloud115_id INTEGER NOT NULL DEFAULT 0,
 target_directory VARCHAR(1024) NOT NULL DEFAULT '',
 status VARCHAR(32) NOT NULL DEFAULT 'pending',
 error_message TEXT NOT NULL DEFAULT '',
 is_second_transfer BOOLEAN NOT NULL DEFAULT FALSE,
 create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 update_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
 );
 CREATE INDEX IF NOT EXISTS idx_share_transfer_log_task_id_id ON t_share_transfer_log (task_id, id);
 CREATE INDEX IF NOT EXISTS idx_share_transfer_log_task_pick ON t_share_transfer_log (task_id, file_pick_code);`
	actual := normalizeD1MigrationSQL(shareTransferLogMigrationSQL)
	if actual != normalizeD1MigrationSQL(approved) {
		t.Fatal("v46 必须完整保持 DBA 列契约，且只创建两个非唯一复合索引")
	}
	legacy, err := os.ReadFile("migrations/migrate_v14_share_transfer_log.sql")
	if err != nil {
		t.Fatal(err)
	}
	legacyTable := strings.SplitN(normalizeD1MigrationSQL(string(legacy)), ");", 2)[0]
	if strings.SplitN(actual, ");", 2)[0] != legacyTable {
		t.Fatal("v46 不能改变 v14 列语义")
	}
	rollback, err := os.ReadFile("migrations/migrate_v46_share_transfer_log_rollback.sql")
	if err != nil {
		t.Fatal(err)
	}
	if normalizeD1MigrationSQL(string(rollback)) != "DROP TABLE IF EXISTS t_share_transfer_log;" {
		t.Fatal("回滚只能删本表，不能使用 CASCADE 或单独删其它索引")
	}
	for _, warning := range []string{"IRREVERSIBLE LOG LOSS", "变更前已存在的表禁止", "独立 DBA 批准", "全部 HTTP、Telegram", "所有实例", "在途", "backup AND restore validation", "索引随表删除"} {
		if !strings.Contains(string(rollback), warning) {
			t.Fatalf("回滚缺少必要前置条件: %s", warning)
		}
	}
}

// TestShareTransferLogMigrationRepeatedMockExecution 只证明重复执行相同幂等 SQL 和事务调用；真实建表幂等由 DBA 演练。
func TestShareTransferLogMigrationRepeatedMockExecution(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for range 2 {
		mock.ExpectBegin()
		mock.ExpectExec(shareTransferLogMigrationSQL).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		if err := applyShareTransferLogMigration(context.Background(), database); err != nil {
			t.Fatal(err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestShareTransferLogMigrationErrors 验证事务开启、执行、回滚、提交与取消分支保留原错误。
func TestShareTransferLogMigrationErrors(t *testing.T) {
	for _, stage := range []string{"begin", "execute", "rollback", "commit", "cancelled"} {
		t.Run(stage, func(t *testing.T) {
			database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			failure := errors.New("migration refused")
			ctx := context.Background()
			switch stage {
			case "begin":
				mock.ExpectBegin().WillReturnError(failure)
			case "execute", "rollback":
				mock.ExpectBegin()
				mock.ExpectExec(shareTransferLogMigrationSQL).WillReturnError(failure)
				rollback := mock.ExpectRollback()
				if stage == "rollback" {
					rollback.WillReturnError(errors.New("rollback refused"))
				}
			case "commit":
				mock.ExpectBegin()
				mock.ExpectExec(shareTransferLogMigrationSQL).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectCommit().WillReturnError(failure)
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				failure = context.Canceled
			}
			if err := applyShareTransferLogMigration(ctx, database); !errors.Is(err, failure) {
				t.Fatalf("迁移必须保留原错误: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func d1StartupFunction(t *testing.T, path, name string) *ast.FuncDecl {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == name {
			return function
		}
	}
	t.Fatalf("%s 未定义 %s", path, name)
	return nil
}

// TestShareTransferLogMigrationStartupWiring 静态验证真实启动链的显式调用、错误返回和有界超时，不调用 InitDB 连接数据库。
func TestShareTransferLogMigrationStartupWiring(t *testing.T) {
	mainFunction := d1StartupFunction(t, "main.go", "main")
	mainCallsInitDB := false
	ast.Inspect(mainFunction, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if name, ok := call.Fun.(*ast.Ident); ok && name.Name == "InitDB" {
				mainCallsInitDB = true
			}
		}
		return true
	})
	if !mainCallsInitDB {
		t.Fatal("main 必须沿用 InitDB 启动链")
	}
	initFunction := d1StartupFunction(t, "db.go", "InitDB")
	guardedMigration := false
	ast.Inspect(initFunction, func(node ast.Node) bool {
		guard, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}
		assignment, ok := guard.Init.(*ast.AssignStmt)
		if !ok || len(assignment.Rhs) != 1 {
			return true
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		name, ok := call.Fun.(*ast.Ident)
		if !ok || name.Name != "applyShareTransferLogMigration" || len(call.Args) != 2 {
			return true
		}
		database, ok := call.Args[1].(*ast.Ident)
		if !ok || database.Name != "db" {
			t.Fatal("InitDB 必须使用现有数据库句柄")
		}
		condition, ok := guard.Cond.(*ast.BinaryExpr)
		if !ok || condition.Op != token.NEQ {
			t.Fatal("迁移调用必须检查错误")
		}
		for _, statement := range guard.Body.List {
			if returned, ok := statement.(*ast.ReturnStmt); ok && len(returned.Results) == 1 {
				if identifier, ok := returned.Results[0].(*ast.Ident); ok && identifier.Name == "err" {
					guardedMigration = true
				}
			}
		}
		return true
	})
	if !guardedMigration {
		t.Fatal("InitDB 必须显式调用 v46 并返回失败，不能只增加未使用的 helper")
	}
	helper, err := os.ReadFile("share_transfer_log_migration.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(helper), "context.WithTimeout(ctx, 30*time.Second)") {
		t.Fatal("v46 事务必须使用有界超时")
	}
}
