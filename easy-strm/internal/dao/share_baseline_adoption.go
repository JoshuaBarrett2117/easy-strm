package dao

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
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

const shareBaselineEpisodeExistsSQL = `EXISTS(SELECT 1 FROM t_share_media_file_episode e WHERE e.file_id=f.id AND e.season_number=i.season_number AND e.episode_number=i.episode_number)`

var shareBaselineLivePredicate = strings.Replace(shareSelectionLivePredicate, shareBaselineEpisodeExistsSQL, `e.file_id IS NOT NULL`, 1)

const shareBaselineEpisodeSetSQL = `episode_set AS MATERIALIZED (
 SELECT DISTINCT file_id,season_number,episode_number FROM t_share_media_file_episode
)`

const shareBaselineOwnedOutputsSQL = `owned_outputs AS MATERIALIZED (
 SELECT output.owner_key,output.export_key,output.state,output.output_path,output.share_strm_id,mapping.id mapping_id,mapping.payload
 FROM t_strm_export_state output LEFT JOIN t_share_strm mapping
 ON mapping.id=CASE WHEN output.share_strm_id ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN output.share_strm_id::uuid END
 AND mapping.id::text=output.share_strm_id
 WHERE output.owner_key='share:default' AND output.state='active'
)`

var shareBaselineLiveRowsSQL = shareBaselineEpisodeSetSQL + `, candidate_keys AS MATERIALIZED (
 SELECT DISTINCT selected_candidate_id FROM t_share_media_selection
), live_rows AS MATERIALIZED (
 SELECT i.candidate_id,i.work_key,i.season_number,i.episode_number,f.id source_id,live.id,f.file_name,f.file_id,f.media_id,r.url,r.password` + shareSelectionLiveJoin + `
 JOIN candidate_keys selected ON selected.selected_candidate_id=i.candidate_id
 LEFT JOIN episode_set e ON e.file_id=f.id AND e.season_number=i.season_number AND e.episode_number=i.episode_number
 WHERE ` + shareBaselineLivePredicate + `
)`

var shareBaselineCountsSQL = shareBaselineCountsPrefixSQL + shareBaselineMediaIDMatchSQL + shareBaselineCountsSuffixSQL
var shareBaselineBenignCountsSQL = shareBaselineCountsPrefixSQL + shareBaselineCountsSuffixSQL

const shareBaselineMediaIDMatchSQL = ` AND mapping.payload_media_id=live.id`

var shareBaselineCountsPrefixSQL = `WITH ` + shareBaselineLiveRowsSQL + `, ` + shareBaselineOwnedOutputsSQL + `, selection_items AS MATERIALIZED (
 SELECT DISTINCT work_key,season_number,episode_number FROM t_share_media_selection
), eligible_sources AS MATERIALIZED (
 SELECT f.id,m.work_key,m.media_type FROM t_share_media_file f
 JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record r ON r.id=f.share_id
 WHERE f.available AND NOT r.share_cancelled AND f.status='identified'
), episode_files AS MATERIALIZED (
 SELECT DISTINCT file_id FROM episode_set
), uncovered_episode_files AS MATERIALIZED (
 SELECT DISTINCT e.file_id FROM eligible_sources m JOIN episode_set e ON e.file_id=m.id
 LEFT JOIN selection_items s ON s.work_key=m.work_key AND s.season_number=e.season_number AND s.episode_number=e.episode_number
 WHERE s.work_key IS NULL
), selection_outputs AS (
 SELECT s.selected_candidate_id,s.work_key,s.season_number,s.episode_number,output.mapping_id,output.payload,
 (s.stable_relative_path='' OR s.stable_relative_path LIKE '/%' OR s.stable_relative_path ~ '(^|/)\.\.(/|$)'
 OR output.export_key IS NULL OR output.output_path<>rtrim($1,'/')||'/'||s.stable_relative_path
 OR COALESCE(output.payload->>'file_name','')='' OR COALESCE(output.payload->>'media_id','')='' OR COALESCE(output.payload->>'share_code','')=''
 OR (COALESCE(output.payload->>'file_id','')='' AND COALESCE(output.payload->>'file_path','')='')) invalid_basic
 FROM t_share_media_selection s
 LEFT JOIN owned_outputs output ON output.owner_key='share:default' AND output.export_key=s.media_item_key AND output.state='active'
), checked_outputs AS MATERIALIZED (
 SELECT selected_candidate_id,work_key,season_number,episode_number,mapping_id,invalid_basic,
 CASE WHEN invalid_basic IS TRUE THEN NULL ELSE (payload->>'media_id')::integer END payload_media_id
 FROM selection_outputs
), live_matches AS MATERIALIZED (
 SELECT DISTINCT mapping.selected_candidate_id,mapping.work_key,mapping.season_number,mapping.episode_number,mapping.mapping_id
 FROM checked_outputs mapping
 JOIN live_rows live ON live.candidate_id=mapping.selected_candidate_id AND live.work_key=mapping.work_key
 AND live.season_number=mapping.season_number AND live.episode_number=mapping.episode_number
 WHERE true`

const shareBaselineCountsSuffixSQL = `)
SELECT
 (SELECT count(*) FROM t_share_media_selection),
 (SELECT count(*) FROM t_strm_export_state WHERE owner_key='share:default' AND state='active'),
 (SELECT count(*) FROM t_share_export_dirty_work WHERE revision>acked_revision),
 (SELECT COALESCE(sum(revision),0) FROM t_share_export_dirty_work),
 (SELECT COALESCE(sum(selection_revision),0) FROM t_share_media_selection),
 (SELECT count(*) FROM checked_outputs s
 LEFT JOIN t_share_media referenced ON referenced.id=s.payload_media_id
 LEFT JOIN live_matches matched ON matched.selected_candidate_id=s.selected_candidate_id AND matched.work_key=s.work_key
 AND matched.season_number=s.season_number AND matched.episode_number=s.episode_number AND matched.mapping_id=s.mapping_id
 WHERE s.invalid_basic OR referenced.id IS NULL OR matched.mapping_id IS NULL)
 + (SELECT count(*) FROM t_share_export_dirty_work WHERE revision<acked_revision OR work_key='' OR (revision>acked_revision AND scope<>'baseline'))
 + (SELECT count(*) FROM t_strm_export_state WHERE owner_key='share:default' AND state='active' AND (output_path='' OR COALESCE(share_strm_id,'')='' OR content_fingerprint='' OR mapping_fingerprint=''))
 + (SELECT count(*) FROM eligible_sources m
 LEFT JOIN selection_items s ON s.work_key=m.work_key AND s.season_number=0 AND s.episode_number=0
 LEFT JOIN episode_files episodes ON episodes.file_id=m.id
 LEFT JOIN uncovered_episode_files uncovered ON uncovered.file_id=m.id
 WHERE m.media_type IS NULL OR m.media_type NOT IN ('movie','tv')
 OR (m.media_type='movie' AND s.work_key IS NULL)
 OR (m.media_type='tv' AND (episodes.file_id IS NULL OR uncovered.file_id IS NOT NULL)))`

var shareBaselineMappingsSQL = `WITH ` + shareBaselineLiveRowsSQL + `, ` + shareBaselineOwnedOutputsSQL + `
 SELECT live.source_id,live.file_name,live.file_id,live.media_id,live.url,live.password,output.mapping_id::text,output.payload,output.export_key,count(*) multiplicity
 FROM t_share_media_selection s
 JOIN owned_outputs output ON output.owner_key='share:default' AND output.export_key=s.media_item_key AND output.state='active'
 JOIN live_rows live ON live.candidate_id=s.selected_candidate_id AND live.work_key=s.work_key
 AND live.season_number=s.season_number AND live.episode_number=s.episode_number
 WHERE output.mapping_id IS NOT NULL
 GROUP BY live.source_id,live.file_name,live.file_id,live.media_id,live.url,live.password,output.mapping_id,output.payload,output.export_key`

const shareBaselineSourcesSQL = `WITH ` + shareBaselineEpisodeSetSQL + `, episode_signatures AS MATERIALIZED (
 SELECT e.file_id,string_agg(e.season_number::text||':'||e.episode_number::text,',' ORDER BY e.season_number,e.episode_number) episode_sig
 FROM t_share_media_file_episode e GROUP BY e.file_id
), sources AS MATERIALIZED (
 SELECT f.id,f.share_id,f.media_id,m.work_key,m.media_type,f.version file_version,r.version share_version,f.available,r.share_cancelled,
 f.available AND NOT r.share_cancelled AND f.status='identified' active
 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record r ON r.id=f.share_id
), source_items AS MATERIALIZED (
 SELECT f.id,s.media_item_key FROM sources f JOIN t_share_media_selection s ON s.work_key=f.work_key
 AND s.season_number=0 AND s.episode_number=0 WHERE f.active AND f.media_type='movie'
 UNION ALL
 SELECT f.id,s.media_item_key FROM sources f JOIN episode_set e ON e.file_id=f.id
 JOIN t_share_media_selection s ON s.work_key=f.work_key AND s.season_number=e.season_number AND s.episode_number=e.episode_number
 WHERE f.active AND f.media_type='tv'
), source_keys AS MATERIALIZED (
 SELECT s.id,jsonb_agg(s.media_item_key ORDER BY s.media_item_key) export_keys FROM source_items s GROUP BY s.id
)
INSERT INTO t_share_export_source_state AS source
 (source_file_id,share_id,media_id,work_key,file_version,share_version,available,cancelled,episode_sig,export_keys,last_seen_run_id,state)
 SELECT f.id,f.share_id,f.media_id,f.work_key,f.file_version,f.share_version,f.available,f.share_cancelled,
 COALESCE(episodes.episode_sig,''),COALESCE(keys.export_keys,'[]'::jsonb),$1,CASE WHEN f.active THEN 'active' ELSE 'stale' END
 FROM sources f LEFT JOIN episode_signatures episodes ON episodes.file_id=f.id LEFT JOIN source_keys keys ON keys.id=f.id
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
	var unsupported bool
	if err = transaction.QueryRowContext(ctx, selectionMetadataUnsupportedSQL).Scan(&unsupported); err != nil {
		return result, err
	}
	if unsupported {
		return result, domain.ErrStrm115MetadataUnsupported
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
	if err = transaction.Commit(); err != nil {
		return result, err
	}
	if request.AllowKnownBenignInvalid > 0 {
		logger.WithContext(logger.WithTaskID(ctx, taskID), "share_baseline_adoption").Log(logger.WARN, "人工确认采纳：已提交仅 MediaID 差异的良性放行", logger.Fields{"event": "manual_baseline_benign_allowance", "allowance_count": request.AllowKnownBenignInvalid, "actor_user_id": request.ActorUserID}, nil)
	}
	return result, nil
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
	var allowance int64
	if request.AllowKnownBenignInvalid > 0 && int64(request.AllowKnownBenignInvalid) == invalid && request.AllowKnownBenignInvalid <= domain.ShareBaselineBenignInvalidHardCap && strings.TrimSpace(request.BenignAttestation) != "" {
		allowance = invalid
	}
	if (invalid != 0 && allowance == 0) || (request.AllowKnownBenignInvalid > 0 && allowance == 0) || result.SelectionCount == 0 || result.OutputCount != result.SelectionCount {
		return fmt.Errorf("采纳一致性校验失败：selection=%d output=%d invalid=%d", result.SelectionCount, result.OutputCount, invalid)
	}
	if allowance > 0 {
		var rechecked [5]int64
		var remaining int64
		if err := transaction.QueryRowContext(ctx, shareBaselineBenignCountsSQL, request.ExpectedOutputPath).Scan(&rechecked[0], &rechecked[1], &rechecked[2], &rechecked[3], &rechecked[4], &remaining); err != nil {
			return err
		}
		if rechecked != [5]int64{result.SelectionCount, result.OutputCount, pending, dirtySum, selectionSum} {
			return ErrShareExportChanged
		}
		if remaining != 0 {
			return fmt.Errorf("采纳良性放行校验失败：仍有非 MediaID 差异 invalid=%d", remaining)
		}
	}
	if err := validateShareBaselineMappings(ctx, transaction, result.SelectionCount, allowance); err != nil {
		return err
	}
	result.AcknowledgedWorks = pending
	return nil
}

func validateShareBaselineMappings(ctx context.Context, transaction *sql.Tx, expectedCount, allowance int64) error {
	rows, err := transaction.QueryContext(ctx, shareBaselineMappingsSQL)
	if err != nil {
		return err
	}
	defer rows.Close()
	var count, mediaIDDifferences int64
	for rows.Next() {
		var source domain.ShareStrmSource
		var mappingID, exportKey string
		var payload []byte
		var multiplicity int64
		if err := rows.Scan(&source.ID, &source.FileName, &source.RemoteFileID, &source.MediaID, &source.URL, &source.Password, &mappingID, &payload, &exportKey, &multiplicity); err != nil {
			return err
		}
		if multiplicity <= 0 || multiplicity > expectedCount-count {
			return fmt.Errorf("采纳映射覆盖数量异常：已验证=%d 本组=%d 预期=%d", count, multiplicity, expectedCount)
		}
		legacyExpected, err := domain.BuildShareStrmEntry(source)
		if err != nil {
			return fmt.Errorf("采纳选中来源 %d 的映射构造失败：%w", source.ID, err)
		}
		expected := legacyExpected
		if exportKey != "" {
			itemExpected, err := domain.BuildShareWorkReexportEntry(source, exportKey)
			if err != nil {
				return fmt.Errorf("采纳选中来源 %d 的 item 映射构造失败：%w", source.ID, err)
			}
			if mappingID == itemExpected.ID {
				expected = itemExpected
			}
		}
		var actual domain.ShareStrmEntry
		if err := json.Unmarshal(payload, &actual); err != nil {
			return fmt.Errorf("采纳选中来源 %d 的映射格式无效：%w", source.ID, err)
		}
		if mappingID != expected.ID || actual.ID != expected.ID || actual.ShareCode != expected.ShareCode || actual.Password != expected.Password ||
			actual.FilePath != expected.FilePath || actual.FileName != expected.FileName || (actual.MediaID != expected.MediaID && allowance == 0) ||
			(expected.FileID != "" && actual.FileID != expected.FileID) {
			return fmt.Errorf("采纳映射与选中来源 %d 不一致；保留现有输出，请重新独立核验", source.ID)
		}
		if actual.MediaID != expected.MediaID {
			mediaIDDifferences += multiplicity
		}
		count += multiplicity
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != expectedCount {
		return fmt.Errorf("采纳映射覆盖不完整：实际=%d 预期=%d", count, expectedCount)
	}
	if allowance > 0 && mediaIDDifferences != allowance {
		return fmt.Errorf("采纳良性放行数量不匹配：仅 MediaID 差异=%d 预期=%d", mediaIDDifferences, allowance)
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
