package dao

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

const shareBaselineOwnerLockSQL = `SELECT pg_try_advisory_xact_lock(hashtextextended('share:default',34982))`
const shareBaselineSelectionLockSQL = `SELECT pg_try_advisory_xact_lock(34983,1)`
const shareBaselineTablesLockSQL = `LOCK TABLE t_share_media_file,t_share_media_file_episode,t_share_media,t_share_record,
 t_share_export_candidate,t_share_export_candidate_item,t_share_media_selection,
 t_share_export_source_state,t_share_export_dirty_work,t_share_export_consumer,
 t_system_config,t_media_category,t_strm_export_state,t_strm_export_history,t_share_strm,t_strm_file
 IN SHARE ROW EXCLUSIVE MODE NOWAIT`
const shareBaselineConsumerSQL = `SELECT config_revision,prepared_revision,completed_revision,config_fingerprint,baseline_state,protocol_version,legacy_outputs_reconciled FROM t_share_export_consumer WHERE consumer='share:default' FOR UPDATE NOWAIT`

const shareBaselineCountsSQL = `SELECT
 (SELECT count(*) FROM t_share_media_selection),
 (SELECT count(*) FROM t_strm_export_state WHERE owner_key='share:default' AND state='active'),
 (SELECT count(*) FROM t_share_export_dirty_work WHERE revision>acked_revision),
 (SELECT COALESCE(sum(revision),0) FROM t_share_export_dirty_work),
 (SELECT COALESCE(sum(selection_revision),0) FROM t_share_media_selection),
 (SELECT count(*) FROM t_share_media_selection s LEFT JOIN t_strm_export_state output
 ON output.owner_key='share:default' AND output.export_key=s.media_item_key AND output.state='active'
 LEFT JOIN t_share_strm mapping ON mapping.id::text=output.share_strm_id
 WHERE s.stable_relative_path='' OR s.stable_relative_path LIKE '/%' OR s.stable_relative_path ~ '(^|/)\.\.(/|$)'
 OR output.export_key IS NULL OR output.output_path<>rtrim($1,'/')||'/'||s.stable_relative_path
 OR COALESCE(mapping.payload->>'file_name','')='' OR COALESCE(mapping.payload->>'media_id','')='' OR COALESCE(mapping.payload->>'share_code','')=''
 OR (COALESCE(mapping.payload->>'file_id','')='' AND COALESCE(mapping.payload->>'file_path','')='')
 OR NOT EXISTS(SELECT 1 FROM t_share_media WHERE id=(mapping.payload->>'media_id')::integer)
 OR NOT EXISTS(SELECT 1` + shareSelectionLiveJoin + ` WHERE i.candidate_id=s.selected_candidate_id
 AND i.work_key=s.work_key AND i.season_number=s.season_number AND i.episode_number=s.episode_number
 AND (mapping.payload->>'media_id')::integer=live.id AND ` + shareSelectionLivePredicate + `))
 + (SELECT count(*) FROM t_share_export_dirty_work WHERE revision<acked_revision OR work_key='' OR (revision>acked_revision AND scope<>'baseline'))
 + (SELECT count(*) FROM t_strm_export_state WHERE owner_key='share:default' AND state='active' AND (output_path='' OR COALESCE(share_strm_id,'')='' OR content_fingerprint='' OR mapping_fingerprint=''))
 + (SELECT count(*) FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record r ON r.id=f.share_id
 WHERE f.available AND NOT r.share_cancelled AND f.status='identified' AND
 (m.media_type IS NULL OR m.media_type NOT IN ('movie','tv')
 OR (m.media_type='movie' AND NOT EXISTS(SELECT 1 FROM t_share_media_selection s WHERE s.work_key=m.work_key AND s.season_number=0 AND s.episode_number=0))
 OR (m.media_type='tv' AND (NOT EXISTS(SELECT 1 FROM t_share_media_file_episode e WHERE e.file_id=f.id)
 OR EXISTS(SELECT 1 FROM t_share_media_file_episode e WHERE e.file_id=f.id AND NOT EXISTS(
 SELECT 1 FROM t_share_media_selection s WHERE s.work_key=m.work_key AND s.season_number=e.season_number AND s.episode_number=e.episode_number))))))`

const shareBaselineMappingsSQL = `SELECT selected.id,selected.file_name,selected.file_id,selected.media_id,selected.url,selected.password,mapping.id::text,mapping.payload
 FROM t_share_media_selection s
 JOIN t_strm_export_state output ON output.owner_key='share:default' AND output.export_key=s.media_item_key AND output.state='active'
 JOIN t_share_strm mapping ON mapping.id::text=output.share_strm_id
 JOIN LATERAL (SELECT f.id,f.file_name,f.file_id,f.media_id,r.url,r.password` + shareSelectionLiveJoin + `
 WHERE i.candidate_id=s.selected_candidate_id AND i.work_key=s.work_key
 AND i.season_number=s.season_number AND i.episode_number=s.episode_number
 AND ` + shareSelectionLivePredicate + `) selected ON true`

const shareBaselineSourcesSQL = `INSERT INTO t_share_export_source_state AS source
 (source_file_id,share_id,media_id,work_key,file_version,share_version,available,cancelled,episode_sig,export_keys,last_seen_run_id,state)
 SELECT f.id,f.share_id,f.media_id,m.work_key,f.version,r.version,f.available,r.share_cancelled,
 COALESCE((SELECT string_agg(e.season_number::text||':'||e.episode_number::text,',' ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),''),
 COALESCE(keys.export_keys,'[]'::jsonb),$1,CASE WHEN f.available AND NOT r.share_cancelled AND f.status='identified' THEN 'active' ELSE 'stale' END
 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record r ON r.id=f.share_id
 LEFT JOIN LATERAL (SELECT jsonb_agg(s.media_item_key ORDER BY s.media_item_key) export_keys
 FROM t_share_media_selection s WHERE f.available AND NOT r.share_cancelled AND f.status='identified' AND s.work_key=m.work_key
 AND ((m.media_type='movie' AND s.season_number=0 AND s.episode_number=0)
 OR (m.media_type='tv' AND EXISTS(SELECT 1 FROM t_share_media_file_episode e
 WHERE e.file_id=f.id AND e.season_number=s.season_number AND e.episode_number=s.episode_number)))) keys ON true
 ORDER BY f.id` + shareExportSourcesConflictSQL
const shareBaselineStaleSourcesSQL = `UPDATE t_share_export_source_state source SET state='stale',last_seen_run_id=$1,updated_at=now()
 WHERE NOT EXISTS(SELECT 1 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record r ON r.id=f.share_id WHERE f.id=source.source_file_id)`
const shareBaselineSelectionAckSQL = `UPDATE t_share_media_selection SET exported_revision=selection_revision,updated_at=now() WHERE exported_revision<>selection_revision`
const shareBaselineAckSQL = shareExportAckSetSQL + `$1` + shareExportAckTailSQL + ` WHERE revision>acked_revision AND scope='baseline'`
const shareBaselineTrustSQL = `UPDATE t_share_export_consumer SET legacy_outputs_reconciled=true,updated_at=now() WHERE consumer='share:default'`
const shareBaselineAuditSQL = `INSERT INTO t_system_config(config_key,config_val,update_time) VALUES('share_strm_baseline_adoption:'||($1::text::jsonb->>'task_id'),$1::text,now())`

// AdoptBaseline 在既有 owner/selection 锁和阻断生产者的表锁内验证、补检查点、ack 与收尾；失败全部回滚。
// 可选提交守卫在 Commit 前执行，供任务原子切换至不可取消的提交阶段；守卫失败也回滚。
func (store *ShareExportCheckpointDAO) AdoptBaseline(ctx context.Context, request domain.ShareBaselineAdoptionRequest, fingerprint, taskID string, beforeCommit ...func() error) (result domain.ShareBaselineAdoptionResult, adoptionErr error) {
	result = domain.ShareBaselineAdoptionResult{ManualConfirmation: "人工确认采纳", TaskID: taskID}
	defer func() {
		if adoptionErr != nil {
			result = domain.ShareBaselineAdoptionResult{ManualConfirmation: "人工确认采纳", TaskID: taskID}
		}
	}()
	if request.ManualConfirmation != "人工确认采纳" || taskID == "" || fingerprint == "" || !filepath.IsAbs(request.ExpectedOutputPath) || filepath.Clean(request.ExpectedOutputPath) != request.ExpectedOutputPath || request.ExpectedOutputPath == "/" {
		return result, fmt.Errorf("必须显式人工确认采纳且提供任务及指纹")
	}
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return result, err
	}
	defer transaction.Rollback()
	if err = lockShareBaselineOutput(ctx, transaction, request.ExpectedOutputPath); err != nil {
		return result, err
	}
	for _, query := range []string{shareBaselineOwnerLockSQL, shareBaselineSelectionLockSQL} {
		var locked bool
		if err = transaction.QueryRowContext(ctx, query).Scan(&locked); err != nil {
			return result, err
		}
		if !locked {
			return result, ErrStrmOutputBusy
		}
	}
	if _, err = transaction.ExecContext(ctx, shareBaselineTablesLockSQL); err != nil {
		return result, fmt.Errorf("采纳拒绝：存在并发生产者、配置或导出写入，请停写并重新核验：%w", err)
	}
	ready, err := validateShareBaselineConsumer(ctx, transaction, request, fingerprint, &result)
	if err != nil || ready {
		if err == nil {
			err = guardShareBaselineCommit(ctx, beforeCommit)
		}
		return result, err
	}
	if err = validateShareBaselineCounts(ctx, transaction, request, &result); err != nil {
		return result, err
	}
	if err = completeShareBaselineAdoption(ctx, transaction, taskID, &result); err != nil {
		return result, err
	}
	audit, err := json.Marshal(struct {
		TaskID      string                              `json:"task_id"`
		RequestID   string                              `json:"request_id"`
		ConfirmedBy int                                 `json:"confirmed_by"`
		ConfirmedAt time.Time                           `json:"confirmed_at"`
		Request     domain.ShareBaselineAdoptionRequest `json:"request"`
		Result      domain.ShareBaselineAdoptionResult  `json:"result"`
	}{taskID, logger.RequestID(ctx), request.ActorUserID, time.Now().UTC(), request, result})
	if err != nil {
		return result, err
	}
	if err = execShareBaselineSingleRow(ctx, transaction, shareBaselineAuditSQL, string(audit)); err != nil {
		return result, err
	}
	if err = guardShareBaselineCommit(ctx, beforeCommit); err != nil {
		return result, err
	}
	return result, transaction.Commit()
}

func guardShareBaselineCommit(ctx context.Context, guards []func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, guard := range guards {
		if guard != nil {
			if err := guard(); err != nil {
				return err
			}
		}
	}
	return nil
}

func lockShareBaselineOutput(ctx context.Context, transaction *sql.Tx, root string) error {
	paths := []string{}
	for current := root; ; current = filepath.Dir(current) {
		paths = append([]string{current}, paths...)
		if filepath.Dir(current) == current {
			break
		}
	}
	for index, path := range paths {
		function := "pg_try_advisory_xact_lock_shared"
		if index == len(paths)-1 {
			function = "pg_try_advisory_xact_lock"
		}
		var locked bool
		if err := transaction.QueryRowContext(ctx, "SELECT "+function+"(hashtextextended($1,34981))", path).Scan(&locked); err != nil {
			return err
		}
		if !locked {
			return ErrStrmOutputBusy
		}
	}
	return nil
}

func validateShareBaselineConsumer(ctx context.Context, transaction *sql.Tx, request domain.ShareBaselineAdoptionRequest, fingerprint string, result *domain.ShareBaselineAdoptionResult) (bool, error) {
	var current, prepared, completed int64
	var stored, state string
	var protocol int
	var reconciled bool
	if err := transaction.QueryRowContext(ctx, shareBaselineConsumerSQL).Scan(&current, &prepared, &completed, &stored, &state, &protocol, &reconciled); err != nil {
		return false, err
	}
	if current != request.Expected["config_revision"] || prepared != request.Expected["prepared_revision"] || prepared != current || stored != fingerprint || protocol != 1 {
		return false, ErrShareExportChanged
	}
	if state == "ready" {
		if !reconciled || completed != current {
			return false, ErrShareExportBaselineRequired
		}
		result.BaselineState, result.CompletedRevision, result.AlreadyReady = "ready", completed, true
		return true, nil
	}
	if state != "building" || completed != request.Expected["completed_revision"] || completed >= current {
		return false, fmt.Errorf("只允许采纳已准备且未完成的 building 基线；required 必须先独立对账")
	}
	result.CompletedRevision = current
	return false, nil
}

func validateShareBaselineCounts(ctx context.Context, transaction *sql.Tx, request domain.ShareBaselineAdoptionRequest, result *domain.ShareBaselineAdoptionResult) error {
	var pending, dirtySum, selectionSum, invalid int64
	if err := transaction.QueryRowContext(ctx, shareBaselineCountsSQL, request.ExpectedOutputPath).Scan(&result.SelectionCount, &result.OutputCount, &pending, &dirtySum, &selectionSum, &invalid); err != nil {
		return err
	}
	actual := map[string]int64{"selection_count": result.SelectionCount, "output_count": result.OutputCount, "pending_count": pending, "dirty_revision_sum": dirtySum, "selection_revision_sum": selectionSum}
	for key, value := range actual {
		if value != request.Expected[key] {
			return fmt.Errorf("采纳快照护栏不匹配：%s 实际=%d 预期=%d；请重新独立核验", key, value, request.Expected[key])
		}
	}
	if invalid != 0 || result.SelectionCount == 0 || result.OutputCount != result.SelectionCount {
		return fmt.Errorf("采纳一致性校验失败：selection=%d output=%d invalid=%d", result.SelectionCount, result.OutputCount, invalid)
	}
	if err := validateShareBaselineMappings(ctx, transaction, result.SelectionCount); err != nil {
		return err
	}
	result.AcknowledgedWorks = pending
	return nil
}

func validateShareBaselineMappings(ctx context.Context, transaction *sql.Tx, expectedCount int64) error {
	rows, err := transaction.QueryContext(ctx, shareBaselineMappingsSQL)
	if err != nil {
		return err
	}
	defer rows.Close()
	var count int64
	for rows.Next() {
		var source domain.ShareStrmSource
		var mappingID string
		var payload []byte
		if err := rows.Scan(&source.ID, &source.FileName, &source.RemoteFileID, &source.MediaID, &source.URL, &source.Password, &mappingID, &payload); err != nil {
			return err
		}
		expected, err := domain.BuildShareStrmEntry(source)
		if err != nil {
			return fmt.Errorf("采纳选中来源 %d 的映射构造失败：%w", source.ID, err)
		}
		var actual domain.ShareStrmEntry
		if err := json.Unmarshal(payload, &actual); err != nil {
			return fmt.Errorf("采纳选中来源 %d 的映射格式无效：%w", source.ID, err)
		}
		if mappingID != expected.ID || actual.ID != expected.ID || actual.ShareCode != expected.ShareCode || actual.Password != expected.Password ||
			actual.FilePath != expected.FilePath || actual.FileName != expected.FileName || actual.MediaID != expected.MediaID ||
			(expected.FileID != "" && actual.FileID != expected.FileID) {
			return fmt.Errorf("采纳映射与选中来源 %d 不一致；保留现有输出，请重新独立核验", source.ID)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != expectedCount {
		return fmt.Errorf("采纳映射覆盖不完整：实际=%d 预期=%d", count, expectedCount)
	}
	return nil
}

func completeShareBaselineAdoption(ctx context.Context, transaction *sql.Tx, taskID string, result *domain.ShareBaselineAdoptionResult) error {
	seed, err := transaction.ExecContext(ctx, shareBaselineSourcesSQL, taskID)
	if err != nil {
		return err
	}
	if result.CheckpointSources, err = seed.RowsAffected(); err != nil {
		return err
	}
	if _, err = transaction.ExecContext(ctx, shareBaselineStaleSourcesSQL, taskID); err != nil {
		return err
	}
	if _, err = transaction.ExecContext(ctx, shareBaselineSelectionAckSQL); err != nil {
		return err
	}
	ack, err := transaction.ExecContext(ctx, shareBaselineAckSQL, taskID)
	if err != nil {
		return err
	}
	count, err := ack.RowsAffected()
	if err != nil {
		return err
	}
	if count != result.AcknowledgedWorks {
		return ErrShareExportChanged
	}
	if err = execShareBaselineSingleRow(ctx, transaction, shareBaselineTrustSQL); err != nil {
		return err
	}
	finished, err := finishShareBaseline(logger.WithTaskID(ctx, taskID), transaction, result.CompletedRevision)
	if err != nil {
		return err
	}
	if !finished {
		return ErrShareExportChanged
	}
	result.BaselineState = "ready"
	return nil
}

func execShareBaselineSingleRow(ctx context.Context, transaction *sql.Tx, query string, args ...interface{}) error {
	result, err := transaction.ExecContext(ctx, query, args...)
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
