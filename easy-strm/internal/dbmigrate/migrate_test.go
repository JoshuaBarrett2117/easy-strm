package dbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// TestMigrationFilesAreSequentialAndAnnotated 约束迁移文件命名、版本连续以及 goose 注解完整。
func TestMigrationFilesAreSequentialAndAnnotated(t *testing.T) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`^(\d{5})_[a-z0-9_]+\.sql$`)
	var versions []int
	for _, entry := range entries {
		match := pattern.FindStringSubmatch(entry.Name())
		if match == nil {
			t.Fatalf("迁移文件名必须是 NNNNN_snake_case.sql：%s", entry.Name())
		}
		version, _ := strconv.Atoi(match[1])
		versions = append(versions, version)
		content, err := fs.ReadFile(migrationFiles, "migrations/"+entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		text := string(content)
		for _, required := range []string{"-- +goose Up", "-- +goose StatementBegin", "-- +goose StatementEnd"} {
			if !strings.Contains(text, required) {
				t.Errorf("%s 缺少 %q", entry.Name(), required)
			}
		}
		// goose 自带事务；文件内再写 BEGIN/COMMIT 会提前提交，破坏失败回滚。
		if regexp.MustCompile(`(?m)^\s*(BEGIN|COMMIT|ROLLBACK)\s*;`).MatchString(text) {
			t.Errorf("%s 不得自带 BEGIN/COMMIT", entry.Name())
		}
	}
	sort.Ints(versions)
	for index, version := range versions {
		if version != int(BaselineVersion)+1+index {
			t.Fatalf("迁移版本必须从 %d 起连续，实际 %v", BaselineVersion+1, versions)
		}
	}
}

// TestBaselineIsSchemaNeutral 基线必须能在任意 schema 下执行（存量库校验依赖这一点）。
func TestBaselineIsSchemaNeutral(t *testing.T) {
	if strings.Contains(baselineSQL, "public.") {
		t.Fatal("基线不得包含 public. 限定名")
	}
	for _, forbidden := range []string{"search_path", "\\restrict", "OWNER TO", "GRANT "} {
		if strings.Contains(baselineSQL, forbidden) {
			t.Fatalf("基线不得包含 %q", forbidden)
		}
	}
	for _, table := range []string{"t_user", "t_cloud_115", "t_strm_config", "t_system_config", "t_cron_task", "t_share_media", "t_share_transfer_log", "t_emby_server"} {
		if !strings.Contains(baselineSQL, "CREATE TABLE "+table+" (") {
			t.Errorf("基线缺少 %s", table)
		}
	}
}

// 以下测试需要本机 PostgreSQL 二进制目录（含 initdb/postgres），例如
// ESTRM_MIGRATE_PG_BIN=/usr/lib/postgresql/17/bin TMPDIR=/tmp go test ./internal/dbmigrate
func isolatedPostgres(t *testing.T) func(name string) *sql.DB {
	t.Helper()
	bin := os.Getenv("ESTRM_MIGRATE_PG_BIN")
	if bin == "" {
		t.Skip("设置 ESTRM_MIGRATE_PG_BIN 后在 TMPDIR 下启动隔离 PostgreSQL 运行迁移集成测试")
	}
	root, err := os.MkdirTemp(os.Getenv("TMPDIR"), "migrate-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	data := filepath.Join(root, "data")
	if output, err := exec.Command(filepath.Join(bin, "initdb"), "-D", data, "-A", "trust", "-U", "migrate_test", "--no-locale", "-E", "UTF8").CombinedOutput(); err != nil {
		t.Fatalf("initdb: %v: %s", err, output)
	}
	logFile, err := os.Create(filepath.Join(root, "postgres.log"))
	if err != nil {
		t.Fatal(err)
	}
	server := exec.Command(filepath.Join(bin, "postgres"), "-D", data, "-k", root, "-p", "55441", "-c", "listen_addresses=")
	server.Stdout, server.Stderr = logFile, logFile
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = server.Process.Signal(syscall.SIGTERM)
		_ = server.Wait()
		_ = logFile.Close()
	})
	open := func(name string) *sql.DB {
		database, err := sql.Open("postgres", fmt.Sprintf("host=%s port=55441 user=migrate_test dbname=%s sslmode=disable", root, name))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = database.Close() })
		return database
	}
	admin := open("postgres")
	deadline := time.Now().Add(15 * time.Second)
	for admin.Ping() != nil {
		if time.Now().After(deadline) {
			t.Fatal("isolated PostgreSQL 启动超时")
		}
		time.Sleep(50 * time.Millisecond)
	}
	return func(name string) *sql.DB {
		if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
			t.Fatal(err)
		}
		return open(name)
	}
}

func latestVersion(t *testing.T) int64 {
	t.Helper()
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	return BaselineVersion + int64(len(entries))
}

func scalar[T any](t *testing.T, database *sql.DB, query string) T {
	t.Helper()
	var value T
	if err := database.QueryRow(query).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value
}

func TestUpOnEmptyAndLegacyDatabases(t *testing.T) {
	create := isolatedPostgres(t)
	ctx := context.Background()
	latest := latestVersion(t)

	t.Run("empty", func(t *testing.T) {
		database := create("empty_db")
		if err := Up(ctx, database); err != nil {
			t.Fatal(err)
		}
		if err := Up(ctx, database); err != nil {
			t.Fatalf("重复启动应为空操作: %v", err)
		}
		current, target, err := Status(ctx, database)
		if err != nil || current != latest || target != latest {
			t.Fatalf("版本 %d/%d, err=%v", current, target, err)
		}
		if count := scalar[int](t, database, `SELECT count(*) FROM t_cron_task WHERE builtin`); count != 3 {
			t.Fatalf("内置定时任务 %d", count)
		}
		if count := scalar[int](t, database, `SELECT count(*) FROM t_media_category`); count != 15 {
			t.Fatalf("默认分类 %d", count)
		}
		for _, table := range []string{"t_share_export_consumer", "t_share_media_selection", "t_cron_cleanup_target", "t_behavior_checkpoint", "t_strm_file_index"} {
			if !scalar[bool](t, database, `SELECT to_regclass('`+table+`') IS NOT NULL`) {
				t.Errorf("缺少 %s", table)
			}
		}
	})

	t.Run("legacy", func(t *testing.T) {
		database := create("legacy_db")
		// 模拟旧启动代码建成的库：结构与基线一致但没有版本表，并带一条用户数据。
		if _, err := database.Exec(baselineSQL); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`UPDATE t_media_category SET target_path='/custom' WHERE name='国漫'`); err != nil {
			t.Fatal(err)
		}
		if err := Up(ctx, database); err != nil {
			t.Fatal(err)
		}
		if path := scalar[string](t, database, `SELECT target_path FROM t_media_category WHERE name='国漫'`); path != "/custom" {
			t.Fatalf("已有库不应重跑基线默认数据: %s", path)
		}
		if count := scalar[int](t, database, `SELECT count(*) FROM t_media_category`); count != 15 {
			t.Fatalf("已有库默认分类被重复插入: %d", count)
		}
		if version := scalar[int64](t, database, `SELECT max(version_id) FROM goose_db_version`); version != latest {
			t.Fatalf("版本 %d", version)
		}
		if scalar[bool](t, database, `SELECT to_regnamespace('`+probeSchema+`') IS NOT NULL`) {
			t.Fatal("临时校验 schema 未清理")
		}
	})

	t.Run("legacy_missing_column", func(t *testing.T) {
		database := create("legacy_old_db")
		if _, err := database.Exec(baselineSQL + `; ALTER TABLE t_strm_config DROP COLUMN behavior_sync_enabled;`); err != nil {
			t.Fatal(err)
		}
		err := Up(ctx, database)
		if err == nil || !strings.Contains(err.Error(), "t_strm_config.behavior_sync_enabled") {
			t.Fatalf("应拒绝早于基线的库: %v", err)
		}
		if scalar[bool](t, database, `SELECT EXISTS(SELECT 1 FROM goose_db_version WHERE version_id=1)`) {
			t.Fatal("失败时不得标记基线")
		}
	})
}
