package main

import (
	_ "embed"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// 仅测试嵌入；生产启动链不得执行此 DBA 手工迁移。
//
//go:embed migrations/migrate_v48_cron_dir_cleanup.sql
var cronCleanupMigrationTestSQL string

func normalizeCronCleanupMigrationSQL(source string) string {
	var lines []string
	for _, line := range strings.Split(source, "\n") {
		lines = append(lines, strings.SplitN(line, "--", 2)[0])
	}
	return strings.Join(strings.Fields(strings.Join(lines, "\n")), " ")
}

// TestCronCleanupMigrationSchemaContract 固定 DBA 批准的列、外键、路径约束、单例索引和不覆盖配置的默认值。
func TestCronCleanupMigrationSchemaContract(t *testing.T) {
	const approved = `CREATE TABLE IF NOT EXISTS t_cron_cleanup_target (
 id BIGSERIAL PRIMARY KEY,
 cron_task_id INTEGER NOT NULL REFERENCES t_cron_task(id) ON DELETE CASCADE,
 cloud115_id INTEGER NOT NULL REFERENCES t_cloud_115(id) ON DELETE CASCADE,
 dir_path VARCHAR(1024) NOT NULL,
 dir_cid TEXT,
 enabled BOOLEAN NOT NULL DEFAULT TRUE,
 create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 update_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 CONSTRAINT ck_cleanup_target_path CHECK(dir_path ~ '^/[^/]' AND dir_path !~ '/$' AND dir_path !~ '//'),
 CONSTRAINT ck_cleanup_target_cid CHECK(dir_cid IS NULL OR dir_cid ~ '^[1-9][0-9]*$')
 );
 CREATE UNIQUE INDEX IF NOT EXISTS uq_cron_cleanup_target
 ON t_cron_cleanup_target(cron_task_id,cloud115_id,dir_path);
 CREATE INDEX IF NOT EXISTS idx_cron_cleanup_target_account
 ON t_cron_cleanup_target(cloud115_id);
 CREATE UNIQUE INDEX IF NOT EXISTS uq_cron_task_cleanup_directories_single
 ON t_cron_task(handler) WHERE handler = 'cleanup_directories';
 INSERT INTO t_system_config(config_key,config_val)
 VALUES('share_transfer_dir','/EmbyCache')
 ON CONFLICT(config_key) DO NOTHING;
 COMMIT;`
	script := normalizeCronCleanupMigrationSQL(cronCleanupMigrationTestSQL)
	_, actual, found := strings.Cut(script, "END $pre$; ")
	if !found || actual != normalizeCronCleanupMigrationSQL(approved) {
		t.Fatal("v48 必须完整保持 DBA 批准的表、三个索引和配置默认值契约，不能增加任务种子或其他写入")
	}
}

// TestCronCleanupMigrationPreflight 验证事务超时、所有依赖和重复任务拒绝逻辑在建表前执行，并报告任务 ID。
func TestCronCleanupMigrationPreflight(t *testing.T) {
	script := normalizeCronCleanupMigrationSQL(cronCleanupMigrationTestSQL)
	preflight, _, found := strings.Cut(script, "END $pre$; ")
	if !found {
		t.Fatal("缺少前置检查块")
	}
	const approved = `BEGIN;
 SET LOCAL lock_timeout = '10s';
 SET LOCAL statement_timeout = '2min';
 DO $pre$
 DECLARE cleanup_task_ids INTEGER[];
 BEGIN
 IF to_regclass('t_cron_task') IS NULL
 OR to_regclass('t_cloud_115') IS NULL
 OR to_regclass('t_system_config') IS NULL
 OR to_regclass('idx_cron_task_key') IS NULL THEN
 RAISE EXCEPTION 'v48 缺少前置表或 idx_cron_task_key 索引，请 DBA 先核验已有迁移';
 END IF;
 SELECT array_agg(id ORDER BY id) INTO cleanup_task_ids
 FROM t_cron_task WHERE handler = 'cleanup_directories';
 IF cardinality(cleanup_task_ids) > 1 THEN
 RAISE EXCEPTION 'v48 存在多个 cleanup_directories 任务，任务 ids=%，请 DBA 先处理', cleanup_task_ids;
 END IF;`
	if strings.TrimSpace(preflight) != normalizeCronCleanupMigrationSQL(approved) {
		t.Fatal("前置检查必须检查四个依赖，允许零个或一个任务，多个任务须报错并列出 ID")
	}
}

// TestCronCleanupMigrationManualSafety 验证手工执行和回滚约束，禁止前向迁移破坏已有数据或预置任务。
func TestCronCleanupMigrationManualSafety(t *testing.T) {
	for _, required := range []string{"先备份", "仅由 DBA 手工执行", "禁止应用启动执行", "不提供回滚脚本", "停用任务", "回退二进制", "DBA 审核", "保留系统设置和用户任务行"} {
		if !strings.Contains(cronCleanupMigrationTestSQL, required) {
			t.Errorf("缺少运维约束：%s", required)
		}
	}
	script := normalizeCronCleanupMigrationSQL(cronCleanupMigrationTestSQL)
	for _, forbidden := range []string{
		`(?i)\b(?:DROP|TRUNCATE|ALTER|UPDATE|MERGE)\b`,
		`(?i)\bDELETE\s+FROM\b`,
		`(?i)\bINSERT\s+INTO\s+t_cron_task\b`,
	} {
		if regexp.MustCompile(forbidden).MatchString(script) {
			t.Errorf("前向迁移包含禁止的写入：%s", forbidden)
		}
	}
}

// TestCronCleanupMigrationNotInProduction 静态扫描生产 Go 源码及嵌入目录通配符，不加载应用或连接数据库。
func TestCronCleanupMigrationNotInProduction(t *testing.T) {
	const migration = "migrations/migrate_v48_cron_dir_cleanup.sql"
	directive := regexp.MustCompile("(?m)^\\s*//go:embed\\s+(.+)$")
	patterns := regexp.MustCompile("`[^`]*`|\"[^\"]*\"|\\S+")
	inlineDDL := regexp.MustCompile(`(?is)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?t_cron_cleanup_target\b`)
	checked := 0
	err := filepath.WalkDir(".", func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(filename, ".go") || strings.HasSuffix(filename, "_test.go") {
			return nil
		}
		source, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		checked++
		text := string(source)
		if strings.Contains(text, "migrate_v48") || strings.Contains(text, "cronCleanupMigrationTestSQL") || inlineDDL.MatchString(text) {
			t.Errorf("生产源码 %s 引用或复制了手工迁移", filename)
		}
		relative, err := filepath.Rel(filepath.Dir(filename), migration)
		if err != nil {
			return err
		}
		if strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil // 嵌入不能跨越当前包目录向上访问。
		}
		for _, match := range directive.FindAllStringSubmatch(text, -1) {
			for _, pattern := range patterns.FindAllString(match[1], -1) {
				if strings.HasPrefix(pattern, "\"") || strings.HasPrefix(pattern, "`") {
					pattern, err = strconv.Unquote(pattern)
					if err != nil {
						return err
					}
				}
				pattern = strings.TrimPrefix(pattern, "all:")
				// 匹配目录也会递归嵌入其文件，因此逐级检查祖先目录。
				for candidate := filepath.ToSlash(relative); candidate != "."; candidate = path.Dir(candidate) {
					matched, err := path.Match(pattern, candidate)
					if err != nil {
						return err
					}
					if matched {
						t.Errorf("生产源码 %s 的嵌入模式 %q 包含手工迁移", filename, pattern)
						break
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("未检查任何生产 Go 源码")
	}
}
