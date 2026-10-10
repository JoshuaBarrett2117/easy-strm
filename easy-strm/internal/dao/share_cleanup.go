package dao

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ShareCleanupOptions 是离线脚本生成参数，必须显式授权销毁手选及全部分享派生记录。
type ShareCleanupOptions struct {
	Mode                       string
	Scope                      string
	Apply                      bool
	Confirm                    string
	ConfirmSelectionsDestroyed bool
	BackupAttestation          string
	Quiesced                   bool
}

// ShareCleanupTable 定义精确删除谓词，不含账号、全局配置、恢复账本或无关输出。
type ShareCleanupTable struct{ Table, Predicate string }

// ShareCleanupTables 仅返回 DBA 批准的十表 REBUILD 清单，非法模式不返回删除目标。
func ShareCleanupTables(mode string) []ShareCleanupTable {
	if mode != "REBUILD" {
		return nil
	}
	return []ShareCleanupTable{
		{"t_share_media_selection", "TRUE"},
		{"t_share_export_candidate_item", "TRUE"},
		{"t_share_export_candidate", "TRUE"},
		{"t_share_strm", "TRUE"},
		{"t_strm_export_state", "owner_key='share:default'"},
		{"t_strm_export_history", "owner_key='share:default'"},
		{"t_strm_clear_run", "task_id IN (SELECT task_id FROM t_share_operation_queue WHERE operation='share_clear')"},
		{"t_strm_file", "strm_config_id=-1"},
		{"t_share_export_source_state", "TRUE"},
		{"t_share_export_dirty_work", "TRUE"},
	}
}

func validateShareCleanup(options ShareCleanupOptions) error {
	if options.Mode != "REBUILD" {
		return fmt.Errorf("mode 仅支持 REBUILD")
	}
	if options.Scope != "all-share" {
		return fmt.Errorf("必须显式指定 scope=all-share；不支持含糊或跨业务清理范围")
	}
	if options.Apply && (!options.ConfirmSelectionsDestroyed || !options.Quiesced || strings.TrimSpace(options.BackupAttestation) == "" || options.Confirm != "DESTROY_SHARE_SELECTIONS_AND_DERIVED_RECORDS") {
		return fmt.Errorf("apply 需要确认销毁选择、已停任务、备份证明和准确确认字符串")
	}

	return nil
}

// BuildShareCleanupSQL 默认生成只读逐表 COUNT；不连接服务、不读取配置、不执行 SQL。
func BuildShareCleanupSQL(options ShareCleanupOptions) (string, error) {
	if err := validateShareCleanup(options); err != nil {
		return "", err
	}
	var output strings.Builder
	output.WriteString("-- 更新日期：2026-10-10；维护者：Codex。仅 DBA 审核后手工执行。\n")
	output.WriteString("-- REBUILD 唯一模式：保留识别缓存、原始分享记录、全部系统配置及恢复台账；dirty_work 最后删除。\n-- v47 缺失：先停写、备份并由 DBA 安装 v47，再运行本脚本；禁止对不存在的表清理。\n-- v47 已存在：验证备份→停任务与 Emby 扫描→只读 dry-run→DBA 审核 apply→核验/回填 v47→SyncSelections→显式手动全量对账。\n")
	if options.Apply {
		output.WriteString("BEGIN;\nSET LOCAL lock_timeout='30s';\nSET LOCAL statement_timeout='5min';\n")
		output.WriteString(shareCleanupGuardSQL)
		output.WriteString("LOCK TABLE ")
		for index, table := range ShareCleanupTables(options.Mode) {
			if index > 0 {
				output.WriteString(",")
			}
			output.WriteString(table.Table)
		}
		output.WriteString(" IN EXCLUSIVE MODE;\n")
	} else {
		output.WriteString("BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;\nSET LOCAL statement_timeout='5min';\n")
	}
	for _, table := range ShareCleanupTables(options.Mode) {
		fmt.Fprintf(&output, "SELECT '%s' AS table_name,count(*) AS rows_to_delete FROM %s WHERE %s;\n", table.Table, table.Table, table.Predicate)
	}
	output.WriteString("SELECT count(*) AS manual_selections_to_destroy FROM t_share_media_selection WHERE selection_mode='manual';\nSELECT media_item_key,selection_mode,stable_relative_path FROM t_share_media_selection WHERE selection_mode='manual' ORDER BY media_item_key;\n")

	if options.Apply {
		for _, table := range ShareCleanupTables(options.Mode) {
			fmt.Fprintf(&output, "DELETE FROM %s WHERE %s;\n", table.Table, table.Predicate)
		}
		output.WriteString("SELECT share_export_require_baseline();\nUPDATE t_share_export_consumer SET legacy_outputs_reconciled=false WHERE consumer='share:default';\nCOMMIT;\n")
	} else {
		output.WriteString("ROLLBACK;\n")
	}
	return output.String(), nil
}

const shareCleanupGuardSQL = `DO $guard$
BEGIN
 IF NOT pg_try_advisory_xact_lock(hashtextextended('share:default',34982)) THEN
  RAISE EXCEPTION '分享导出正在运行；先停任务再清理';
 END IF;
 IF NOT pg_try_advisory_xact_lock(34983,1) THEN
  RAISE EXCEPTION '来源刷新或改选正在运行；先停任务再清理';
 END IF;
 IF EXISTS(SELECT 1 FROM t_share_operation_queue WHERE status IN ('pending','running'))
 OR EXISTS(SELECT 1 FROM t_cron_task_run r JOIN t_cron_task c ON c.id=r.cron_task_id WHERE r.status IN ('pending','running') AND left(c.handler,6)='share_') THEN
  RAISE EXCEPTION '仍有分享操作或调度任务；禁止清理';
 END IF;
END $guard$;
`

// ShareCleanupCount 返回清理清单统计，不将测试计数冒充生产计数。
type ShareCleanupCount struct {
	Table string
	Count int64
}

// ReadShareCleanupCounts 仅在调用方明确提供的数据库上运行一致只读 COUNT，用于 DBA 工具或 sqlmock。
func ReadShareCleanupCounts(ctx context.Context, database *sql.DB, options ShareCleanupOptions) ([]ShareCleanupCount, error) {
	if err := validateShareCleanup(options); err != nil {
		return nil, err
	}
	transaction, err := database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer transaction.Rollback()
	counts := []ShareCleanupCount{}
	for _, table := range ShareCleanupTables(options.Mode) {
		var count int64
		if err = transaction.QueryRowContext(ctx, "SELECT count(*) FROM "+table.Table+" WHERE "+table.Predicate).Scan(&count); err != nil {
			return nil, err
		}
		counts = append(counts, ShareCleanupCount{Table: table.Table, Count: count})
	}
	return counts, transaction.Commit()
}
