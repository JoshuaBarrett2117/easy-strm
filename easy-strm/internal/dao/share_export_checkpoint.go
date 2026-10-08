package dao

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/lib/pq"
)

// ErrShareExportSchemaMissing 要求 DBA 先手动安装 v43，消费者不会自行迁移。
var ErrShareExportSchemaMissing = errors.New("分享增量检查点未安装，请 DBA 手动应用 v43 后重试")

// ErrShareExportChanged 表示来源或配置在处理期间改变，待办及计划键必须保留。
var ErrShareExportChanged = errors.New("分享来源或配置已改变，请重试增量任务")

// ErrShareExportBaselineRequired 表示需先通过既有完整分享导出建立历史输出对账凭证。
var ErrShareExportBaselineRequired = errors.New("请先运行既有完整分享导出进行全量对账，再执行分享增量重建")

// ShareExportCheckpointDAO 持久化独立来源检查点、作品待办与配置基线。
type ShareExportCheckpointDAO struct {
	db         *sql.DB
	outputConn *sql.Conn
}

// NewShareExportCheckpointDAO 注入数据库；构造过程不连接、不执行迁移。
func NewShareExportCheckpointDAO(database *sql.DB) *ShareExportCheckpointDAO {
	return &ShareExportCheckpointDAO{db: database}
}

// ForOutputConnection 让旧全量恢复记账复用已持有 owner 锁的连接，避免单连接池自等待。
func (store *ShareExportCheckpointDAO) ForOutputConnection(connection *sql.Conn) *ShareExportCheckpointDAO {
	return &ShareExportCheckpointDAO{db: store.db, outputConn: connection}
}

func (store *ShareExportCheckpointDAO) execRecovery(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if store.outputConn != nil {
		return store.outputConn.ExecContext(ctx, query, args...)
	}
	return store.db.ExecContext(ctx, query, args...)
}

// CheckSchema 在任何文件副作用前检查三表及 producer，缺失时安全失败。
func (d *ShareExportCheckpointDAO) CheckSchema(ctx context.Context) error {
	var present bool
	err := d.db.QueryRowContext(ctx, `SELECT to_regclass('t_share_export_source_state') IS NOT NULL
 AND to_regclass('t_share_export_dirty_work') IS NOT NULL AND to_regclass('t_share_export_consumer') IS NOT NULL
 AND to_regprocedure('share_export_enqueue(text[],text)') IS NOT NULL
 AND (SELECT count(*) FROM pg_trigger WHERE tgname IN ('share_export_insert','share_export_update','share_export_delete','share_export_truncate')
 AND tgrelid IN (to_regclass('t_share_media_file'),to_regclass('t_share_media'),to_regclass('t_share_record'),to_regclass('t_share_media_file_episode'),to_regclass('t_system_config'),to_regclass('t_media_category'),to_regclass('t_strm_export_state')) AND tgenabled='O')=28
 AND to_regprocedure('share_export_checkpoint_invalidated()') IS NOT NULL
 AND (SELECT count(*) FROM pg_trigger WHERE tgname IN ('share_export_delete','share_export_truncate')
 AND tgrelid IN (to_regclass('t_share_export_source_state'),to_regclass('t_share_export_dirty_work')) AND tgenabled='O')=4`).Scan(&present)
	if err != nil {
		return fmt.Errorf("检查分享增量 schema：%w", err)
	}
	if !present {
		return ErrShareExportSchemaMissing
	}
	return nil
}

// ReadInput 在一致只读快照中仅读取导出白名单配置与有序分类。
func (d *ShareExportCheckpointDAO) ReadInput(ctx context.Context) (domain.ShareExportInput, error) {
	input := domain.ShareExportInput{Templates: map[string]string{}}
	transaction, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return input, err
	}
	defer transaction.Rollback()
	err = transaction.QueryRowContext(ctx, `SELECT config_revision,prepared_revision,config_fingerprint,baseline_state,protocol_version,legacy_outputs_reconciled FROM t_share_export_consumer WHERE consumer='share:default'`).Scan(&input.ConfigRevision, &input.PreparedRevision, &input.Fingerprint, &input.BaselineState, &input.ProtocolVersion, &input.LegacyOutputsReconciled)
	if err != nil {
		return input, err
	}
	rows, err := transaction.QueryContext(ctx, `SELECT config_key,config_val FROM t_system_config WHERE config_key IN ('share_strm_settings','movie_naming_template','tv_naming_template') ORDER BY config_key`)
	if err != nil {
		return input, err
	}
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			break
		}
		if key == "share_strm_settings" {
			err = json.Unmarshal([]byte(value), &input.Settings)
		} else {
			input.Templates[key] = value
		}
		if err != nil {
			break
		}
	}
	err = errors.Join(err, rows.Err(), rows.Close())
	if err != nil {
		return input, err
	}
	rows, err = transaction.QueryContext(ctx, `SELECT id,name,media_type,target_path,match_rules,COALESCE(enabled,false) FROM t_media_category ORDER BY id`)
	if err != nil {
		return input, err
	}
	for rows.Next() {
		category := &domain.MediaCategory{}
		var rules []byte
		if err = rows.Scan(&category.ID, &category.Name, &category.MediaType, &category.TargetPath, &rules, &category.Enabled); err != nil {
			break
		}
		category.MatchRules = json.RawMessage(rules)
		input.Categories = append(input.Categories, category)
	}
	err = errors.Join(err, rows.Err(), rows.Close())
	if err != nil {
		return input, err
	}
	return input, transaction.Commit()
}

const shareExportFanoutSQL = `SELECT share_export_enqueue(ARRAY(
 SELECT work_key FROM t_share_media WHERE work_key<>''
 UNION SELECT work_key FROM t_share_export_source_state WHERE work_key<>''
 UNION SELECT work_key FROM t_share_export_dirty_work ORDER BY work_key),'baseline')`

// Prepare 原子完成全作品 fanout 与指纹推进；building 重启不会重复 fanout。
func (d *ShareExportCheckpointDAO) Prepare(ctx context.Context, input domain.ShareExportInput, fingerprint string) error {
	transaction, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	var revision, prepared int64
	var stored, state string
	var protocol int
	var reconciled bool
	err = transaction.QueryRowContext(ctx, `SELECT config_revision,prepared_revision,config_fingerprint,baseline_state,protocol_version,legacy_outputs_reconciled FROM t_share_export_consumer WHERE consumer='share:default' FOR UPDATE`).Scan(&revision, &prepared, &stored, &state, &protocol, &reconciled)
	if err != nil {
		return err
	}
	if revision != input.ConfigRevision {
		return ErrShareExportChanged
	}
	if !reconciled {
		return ErrShareExportBaselineRequired
	}
	if prepared != revision || stored != fingerprint || state == "required" || protocol != 1 {
		if _, err = transaction.ExecContext(ctx, shareExportFanoutSQL); err != nil {
			return err
		}
		if _, err = transaction.ExecContext(ctx, `UPDATE t_share_export_consumer SET config_fingerprint=$1,prepared_revision=config_revision,protocol_version=1,baseline_state='building',updated_at=now() WHERE consumer='share:default'`, fingerprint); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

// RecoveryRevision 获取全量对账前的失效令牌，期间破坏检查点不能被旧 full 的凭证覆盖。
func (d *ShareExportCheckpointDAO) RecoveryRevision(ctx context.Context) (int64, error) {
	var revision int64
	err := d.db.QueryRowContext(ctx, `SELECT config_revision FROM t_share_export_consumer WHERE consumer='share:default'`).Scan(&revision)
	return revision, err
}

const shareExportExternalPlannedSQL = `INSERT INTO t_share_export_dirty_work AS dirty(work_key,scope,pending_export_keys)
 VALUES($1,'external-export',$2::jsonb) ON CONFLICT(work_key) DO UPDATE SET
 revision=CASE WHEN dirty.revision=dirty.acked_revision THEN dirty.revision+1 ELSE dirty.revision END,
 scope='external-export',pending_export_keys=(SELECT COALESCE(jsonb_agg(key ORDER BY key),'[]'::jsonb) FROM (
 SELECT jsonb_array_elements_text(dirty.pending_export_keys) key UNION SELECT jsonb_array_elements_text(EXCLUDED.pending_export_keys)) keys),updated_at=now()`

// RecordExternalPlannedKeys 为旧全量/筛选出口提交恢复键，不限制其原有来源版本边界，不确认任何作品。
func (d *ShareExportCheckpointDAO) RecordExternalPlannedKeys(ctx context.Context, work string, keys []string) error {
	_, err := d.execRecovery(ctx, shareExportExternalPlannedSQL, work, shareExportKeysJSON(keys))
	return err
}

// RequireBaseline 仅设置持久失效令牌，不在触发或请求事务中扫描作品。
func (d *ShareExportCheckpointDAO) RequireBaseline(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, `SELECT share_export_require_baseline()`)
	return err
}

// MarkLegacyReconciled 仅在旧全量 FinishSnapshot 成功且配置未改变后登记一次性凭证。
func (d *ShareExportCheckpointDAO) MarkLegacyReconciled(ctx context.Context, revision int64) error {
	result, err := d.execRecovery(ctx, `UPDATE t_share_export_consumer SET legacy_outputs_reconciled=true,updated_at=now() WHERE consumer='share:default' AND config_revision=$1`, revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrShareExportChanged
	}
	return nil
}

// DirtyBatch 按 partial 待办索引读取有限批次，排除本轮已尝试的作品。
func (d *ShareExportCheckpointDAO) DirtyBatch(ctx context.Context, attempted []string) ([]domain.ShareExportDirty, error) {
	rows, err := d.db.QueryContext(ctx, shareExportDirtySQL, pq.Array(attempted))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	batch := []domain.ShareExportDirty{}
	for rows.Next() {
		var dirty domain.ShareExportDirty
		var raw []byte
		if err = rows.Scan(&dirty.WorkKey, &dirty.Revision, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &dirty.PendingKeys); err != nil {
			return nil, err
		}
		batch = append(batch, dirty)
	}
	return batch, rows.Err()
}

const shareExportDirtySQL = `SELECT work_key,revision,pending_export_keys FROM t_share_export_dirty_work WHERE revision > acked_revision AND NOT(work_key=ANY($1::text[])) ORDER BY updated_at,work_key LIMIT 100`

const shareExportAckSQL = `UPDATE t_share_export_dirty_work SET acked_revision=revision,last_seen_run_id=$3,pending_export_keys='[]'::jsonb,updated_at=now() WHERE work_key=$1 AND revision=$2 AND acked_revision<>revision`

const shareExportPlannedSQL = `UPDATE t_share_export_dirty_work SET pending_export_keys=(
 SELECT COALESCE(jsonb_agg(key ORDER BY key),'[]'::jsonb) FROM (
 SELECT jsonb_array_elements_text(pending_export_keys) key UNION SELECT jsonb_array_elements_text($3::jsonb)) keys
 ),updated_at=now() WHERE work_key=$1 AND revision=$2 AND revision > acked_revision`

func shareExportKeysJSON(keys []string) string {
	unique := map[string]bool{}
	for _, key := range keys {
		unique[key] = true
	}
	ordered := make([]string, 0, len(unique))
	for key := range unique {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	raw, _ := json.Marshal(ordered)
	return string(raw)
}

// RecordPlannedKeys 单语句自动提交 union 计划键；CAS 失败后禁止文件副作用。
func (d *ShareExportCheckpointDAO) RecordPlannedKeys(ctx context.Context, key string, revision int64, keys []string) error {
	result, err := d.db.ExecContext(ctx, shareExportPlannedSQL, key, revision, shareExportKeysJSON(keys))
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrShareExportChanged
	}
	return nil
}

const shareExportObservedSQL = `SELECT f.id,f.share_id,COALESCE(f.media_id,0),m.work_key,f.version,s.version,f.available,s.share_cancelled,
 COALESCE((SELECT string_agg(e.season_number::text||':'||e.episode_number::text,',' ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),''),
 CASE WHEN f.available AND NOT s.share_cancelled AND f.status='identified' THEN 'active' ELSE 'stale' END
 FROM t_share_media m JOIN t_share_media_file f ON f.media_id=m.id JOIN t_share_record s ON s.id=f.share_id WHERE m.work_key=$1 ORDER BY f.id`

// ObserveWork 读取目标作品的全部来源观察，包含取消及不可用来源。
func (d *ShareExportCheckpointDAO) ObserveWork(ctx context.Context, key string) ([]domain.ShareExportSourceState, error) {
	rows, err := d.db.QueryContext(ctx, shareExportObservedSQL, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := []domain.ShareExportSourceState{}
	for rows.Next() {
		var state domain.ShareExportSourceState
		if err = rows.Scan(&state.SourceFileID, &state.ShareID, &state.MediaID, &state.WorkKey, &state.FileVersion, &state.ShareVersion, &state.Available, &state.Cancelled, &state.EpisodeSignature, &state.State); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

// WorkKeys 获取历史来源键与半成功键，不遍历输出 owner 的全部清单。
func (d *ShareExportCheckpointDAO) WorkKeys(ctx context.Context, key string) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT jsonb_array_elements_text(export_keys) FROM t_share_export_source_state WHERE work_key=$1 UNION SELECT jsonb_array_elements_text(pending_export_keys) FROM t_share_export_dirty_work WHERE work_key=$1`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			return nil, err
		}
		keys = append(keys, value)
	}
	return keys, rows.Err()
}

// TargetSnapshot 只读取本次目标键的起始版本，用于精确失效 CAS。
func (d *ShareExportCheckpointDAO) TargetSnapshot(ctx context.Context, keys []string) (ExportSnapshot, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT export_key,last_seen_run_id FROM t_strm_export_state WHERE owner_key='share:default' AND export_key=ANY($1::text[])`, pq.Array(keys))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	snapshot := ExportSnapshot{}
	for rows.Next() {
		var key, run string
		if err = rows.Scan(&key, &run); err != nil {
			return nil, err
		}
		snapshot[key] = run
	}
	return snapshot, rows.Err()
}

const shareExportStaleSQL = `UPDATE t_strm_export_state output SET state='stale'
 WHERE owner_key='share:default' AND export_key=$2 AND last_seen_run_id=$3
 AND NOT EXISTS(SELECT 1 FROM t_share_export_source_state WHERE state = 'active' AND work_key<>$1 AND export_keys @> to_jsonb($2::text))
 AND NOT EXISTS(SELECT 1 FROM t_share_export_dirty_work WHERE pending_export_keys <> '[]'::jsonb AND work_key<>$1 AND pending_export_keys @> to_jsonb($2::text))`

// CompleteWork 在同一短事务中 CAS 检查配置/作品，写检查点、差集失效及 ack。
func (d *ShareExportCheckpointDAO) CompleteWork(ctx context.Context, dirty domain.ShareExportDirty, configRevision int64, run string, states []domain.ShareExportSourceState, snapshot ExportSnapshot) (int, error) {
	transaction, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer transaction.Rollback()
	var current int64
	if err = transaction.QueryRowContext(ctx, `SELECT config_revision FROM t_share_export_consumer WHERE consumer='share:default' FOR UPDATE`).Scan(&current); err != nil {
		return 0, err
	}
	if current != configRevision {
		return 0, ErrShareExportChanged
	}
	var revision, acked int64
	if err = transaction.QueryRowContext(ctx, `SELECT revision,acked_revision FROM t_share_export_dirty_work WHERE work_key=$1 FOR UPDATE`, dirty.WorkKey).Scan(&revision, &acked); err != nil {
		return 0, err
	}
	if revision != dirty.Revision || acked == revision {
		return 0, ErrShareExportChanged
	}
	newKeys := map[string]bool{}
	ids := make([]int, 0, len(states))
	for _, state := range states {
		ids = append(ids, state.SourceFileID)
		if state.State == "active" {
			for _, key := range state.ExportKeys {
				newKeys[key] = true
			}
		}
		_, err = transaction.ExecContext(ctx, `INSERT INTO t_share_export_source_state AS source(source_file_id,share_id,media_id,work_key,file_version,share_version,available,cancelled,episode_sig,export_keys,last_seen_run_id,state)
 VALUES($1,$2,NULLIF($3,0),$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12)
 ON CONFLICT(source_file_id) DO UPDATE SET share_id=EXCLUDED.share_id,media_id=EXCLUDED.media_id,work_key=EXCLUDED.work_key,file_version=EXCLUDED.file_version,share_version=EXCLUDED.share_version,available=EXCLUDED.available,cancelled=EXCLUDED.cancelled,episode_sig=EXCLUDED.episode_sig,
 export_keys=CASE WHEN EXCLUDED.state='stale' THEN source.export_keys ELSE EXCLUDED.export_keys END,last_seen_run_id=EXCLUDED.last_seen_run_id,state=EXCLUDED.state,updated_at=now()`, state.SourceFileID, state.ShareID, state.MediaID, state.WorkKey, state.FileVersion, state.ShareVersion, state.Available, state.Cancelled, state.EpisodeSignature, shareExportKeysJSON(state.ExportKeys), run, state.State)
		if err != nil {
			return 0, err
		}
	}
	if _, err = transaction.ExecContext(ctx, `UPDATE t_share_export_source_state SET state='stale',last_seen_run_id=$3,updated_at=now() WHERE work_key=$1 AND NOT(source_file_id=ANY($2::integer[]))`, dirty.WorkKey, pq.Array(ids), run); err != nil {
		return 0, err
	}
	stale := 0
	keys := make([]string, 0, len(snapshot))
	for key := range snapshot {
		if !newKeys[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		result, execErr := transaction.ExecContext(ctx, shareExportStaleSQL, dirty.WorkKey, key, snapshot[key])
		if execErr != nil {
			return 0, execErr
		}
		count, countErr := result.RowsAffected()
		if countErr != nil {
			return 0, countErr
		}
		if count == 0 {
			var protected bool
			err = transaction.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM t_share_export_source_state WHERE state = 'active' AND work_key<>$1 AND export_keys @> to_jsonb($2::text)) OR EXISTS(SELECT 1 FROM t_share_export_dirty_work WHERE pending_export_keys <> '[]'::jsonb AND work_key<>$1 AND pending_export_keys @> to_jsonb($2::text))`, dirty.WorkKey, key).Scan(&protected)
			if err != nil {
				return 0, err
			}
			if !protected {
				return 0, ErrShareExportChanged
			}
		}
		stale += int(count)
	}
	result, err := transaction.ExecContext(ctx, shareExportAckSQL, dirty.WorkKey, dirty.Revision, run)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if count != 1 {
		return 0, ErrShareExportChanged
	}
	return stale, transaction.Commit()
}

// FinishBaseline 仅在配置一致且不存在任何待办时标记 ready；不使用 seq 或时间水位。
func (d *ShareExportCheckpointDAO) FinishBaseline(ctx context.Context, revision int64) (bool, error) {
	result, err := d.db.ExecContext(ctx, `UPDATE t_share_export_consumer SET completed_revision=config_revision,baseline_state='ready',updated_at=now() WHERE consumer='share:default' AND config_revision=$1 AND prepared_revision=config_revision AND NOT EXISTS(SELECT 1 FROM t_share_export_dirty_work WHERE revision > acked_revision)`, revision)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
