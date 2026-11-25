// Package dbmigrate 是 easy-strm 唯一的数据库结构来源：基线 + 版本化迁移，全部嵌入二进制并在启动时执行。
//
// 版本记录在 goose_db_version 表。版本 1 是基线（baseline.sql）：
//   - 空库：执行基线 SQL，建出与旧启动代码一致的完整结构和默认数据；
//   - 已有库（存在业务表但没有版本表，即旧的 db.go 内联 DDL + *_migration.go 时代建成的库）：
//     不执行基线，只校验结构完整（表和列都在）后把版本 1 标记为已应用。
//
// 版本 2 起是 migrations/ 下的 SQL 文件，必须幂等（IF NOT EXISTS 等），
// 因为生产库可能已经手工执行过对应的历史脚本。
package dbmigrate

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"easy-strm/internal/pkg/logger"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// BaselineVersion 是基线迁移的版本号。
const BaselineVersion int64 = 1

// VersionTable 是记录已应用迁移的表名（goose 默认表名，便于直接用 goose CLI 查看）。
const VersionTable = "goose_db_version"

//go:embed baseline.sql
var baselineSQL string

//go:embed migrations/*.sql
var migrationFiles embed.FS

// legacyMarkerTables 任何一个存在即视为“旧启动代码建成的已有库”。
var legacyMarkerTables = []string{"t_user", "t_system_config", "t_cloud_115", "t_strm_config"}

const probeSchema = "easy_strm_baseline_probe"

// Up 在启动时执行全部待执行迁移；多实例同时启动时由 PostgreSQL advisory lock 串行化。
func Up(ctx context.Context, db *sql.DB) error {
	provider, err := newProvider(db, true)
	if err != nil {
		return err
	}
	entry := logger.WithContext(ctx, "db_migrate")
	current, target, err := provider.GetVersions(ctx)
	if err != nil {
		return fmt.Errorf("读取数据库迁移版本失败: %w", err)
	}
	if current >= target {
		entry.Log(logger.DEBUG, "数据库结构已是最新", logger.Fields{"version": current}, nil)
		return nil
	}
	started := time.Now()
	results, err := provider.Up(ctx)
	var partial *goose.PartialError
	if errors.As(err, &partial) {
		results = append(append([]*goose.MigrationResult{}, partial.Applied...), partial.Failed)
	}
	for _, result := range results {
		if result == nil || result.Source == nil {
			continue
		}
		fields := logger.Fields{"version": result.Source.Version, "duration_ms": result.Duration.Milliseconds()}
		if result.Source.Path != "" {
			fields["file"] = result.Source.Path
		}
		if result.Error != nil {
			entry.Log(logger.ERROR, "数据库迁移失败", fields, result.Error)
			continue
		}
		entry.Log(logger.INFO, "数据库迁移已应用", fields, nil)
	}
	if err != nil {
		if partial != nil && partial.Failed != nil && partial.Failed.Source != nil {
			return fmt.Errorf("数据库迁移 %d 失败（已回滚该版本，之前的版本已提交）: %w", partial.Failed.Source.Version, partial.Err)
		}
		return fmt.Errorf("数据库迁移失败: %w", err)
	}
	entry.Log(logger.INFO, "数据库迁移完成", logger.Fields{"from_version": current, "to_version": target, "duration_ms": time.Since(started).Milliseconds()}, nil)
	return nil
}

// Status 返回当前库版本与代码内最新版本，供健康检查/排障使用。
func Status(ctx context.Context, db *sql.DB) (current, target int64, err error) {
	provider, err := newProvider(db, false)
	if err != nil {
		return 0, 0, err
	}
	return provider.GetVersions(ctx)
}

func newProvider(db *sql.DB, withLock bool) (*goose.Provider, error) {
	migrations, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	options := []goose.ProviderOption{
		goose.WithTableName(VersionTable),
		goose.WithDisableGlobalRegistry(true),
		goose.WithGoMigrations(goose.NewGoMigration(BaselineVersion, &goose.GoFunc{RunTx: applyBaseline}, nil)),
	}
	if withLock {
		locker, err := lock.NewPostgresSessionLocker()
		if err != nil {
			return nil, err
		}
		options = append(options, goose.WithSessionLocker(locker))
	}
	return goose.NewProvider(goose.DialectPostgres, db, migrations, options...)
}

// applyBaseline 空库执行基线；已有库只做结构完整性校验，不重复执行任何历史 DDL。
func applyBaseline(ctx context.Context, tx *sql.Tx) error {
	entry := logger.WithContext(ctx, "db_migrate")
	legacy, err := hasLegacyTables(ctx, tx)
	if err != nil {
		return err
	}
	if !legacy {
		if _, err := tx.ExecContext(ctx, baselineSQL); err != nil {
			return fmt.Errorf("执行基线结构失败: %w", err)
		}
		entry.Log(logger.INFO, "空库已按基线建表", logger.Fields{"version": BaselineVersion}, nil)
		return nil
	}
	report, err := compareWithBaseline(ctx, tx)
	if err != nil {
		// 无权限建临时 schema 等情况：不阻塞已有库启动，只提示无法校验。
		entry.Log(logger.WARN, "已有数据库无法与基线逐列比对，跳过校验直接标记基线", logger.Fields{"version": BaselineVersion}, err)
		return nil
	}
	if len(report.MissingIndexes) > 0 {
		entry.Log(logger.WARN, "已有数据库缺少基线中的部分索引（名称可能不同），不影响启动", logger.Fields{"indexes": strings.Join(report.MissingIndexes, ",")}, nil)
	}
	if len(report.MissingColumns) > 0 {
		return fmt.Errorf("已有数据库结构早于迁移基线，缺少 %d 个表/列：%s。请先用上一版镜像（第 1 阶段之前，例如 sha-dc23fb3）启动一次完成旧式升级，再升级到本版本", len(report.MissingColumns), summarize(report.MissingColumns, 20))
	}
	entry.Log(logger.INFO, "检测到已有数据库，结构与基线一致，基线标记为已应用（未执行基线 SQL）", logger.Fields{"version": BaselineVersion}, nil)
	return nil
}

func hasLegacyTables(ctx context.Context, tx *sql.Tx) (bool, error) {
	for _, table := range legacyMarkerTables {
		var present bool
		if err := tx.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&present); err != nil {
			return false, fmt.Errorf("检测已有业务表失败: %w", err)
		}
		if present {
			return true, nil
		}
	}
	return false, nil
}

// BaselineReport 是已有库与基线结构的差异（只列已有库缺少的部分，多出的旧表/列忽略）。
type BaselineReport struct {
	MissingColumns []string
	MissingIndexes []string
}

// compareWithBaseline 在保存点内把基线建到临时 schema，与当前 schema 比对表、列和索引名，然后回滚保存点。
func compareWithBaseline(ctx context.Context, tx *sql.Tx) (report BaselineReport, err error) {
	var schema, searchPath string
	if err = tx.QueryRowContext(ctx, `SELECT current_schema(), current_setting('search_path')`).Scan(&schema, &searchPath); err != nil {
		return report, err
	}
	if _, err = tx.ExecContext(ctx, `SAVEPOINT easy_strm_baseline_probe`); err != nil {
		return report, err
	}
	defer func() {
		if _, rollbackErr := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT easy_strm_baseline_probe`); rollbackErr != nil && err == nil {
			err = rollbackErr
		}
		if _, restoreErr := tx.ExecContext(ctx, `SELECT set_config('search_path', $1, true)`, searchPath); restoreErr != nil && err == nil {
			err = restoreErr
		}
	}()
	if _, err = tx.ExecContext(ctx, `CREATE SCHEMA `+probeSchema); err != nil {
		return report, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT set_config('search_path', $1, true)`, probeSchema); err != nil {
		return report, err
	}
	if _, err = tx.ExecContext(ctx, baselineSQL); err != nil {
		return report, fmt.Errorf("在临时 schema 中构建基线失败: %w", err)
	}
	report.MissingColumns, err = queryStrings(ctx, tx, `
		SELECT table_name || '.' || column_name FROM information_schema.columns WHERE table_schema = $1
		EXCEPT
		SELECT table_name || '.' || column_name FROM information_schema.columns WHERE table_schema = $2`, probeSchema, schema)
	if err != nil {
		return report, err
	}
	report.MissingIndexes, err = queryStrings(ctx, tx, `
		SELECT indexname FROM pg_indexes WHERE schemaname = $1
		EXCEPT
		SELECT indexname FROM pg_indexes WHERE schemaname = $2`, probeSchema, schema)
	return report, err
}

func queryStrings(ctx context.Context, tx *sql.Tx, query string, args ...interface{}) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	sort.Strings(values)
	return values, rows.Err()
}

func summarize(values []string, limit int) string {
	if len(values) <= limit {
		return strings.Join(values, ", ")
	}
	return strings.Join(values[:limit], ", ") + fmt.Sprintf(" 等 %d 项", len(values))
}
