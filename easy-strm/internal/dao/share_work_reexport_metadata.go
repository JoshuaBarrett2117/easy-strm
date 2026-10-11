package dao

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/lib/pq"
)

// WorkReexportTarget 固定已验证的来源、选择和文件中实际保留的播放 ID，不携带伪造指纹。
type WorkReexportTarget struct {
	Selection    domain.ShareSelection
	Source       domain.ShareStrmSource
	URLMappingID string
}

// WorkReexportReceipt 提供事务锁定的旧凭证与两种旧映射，供 Service 验证真实文件。
type WorkReexportReceipt struct {
	State      ExportState
	Mapping    domain.ShareStrmEntry
	URLMapping domain.ShareStrmEntry
	Entry      domain.ShareStrmEntry
}

const workReexportStateSQL = `SELECT output_path,content_fingerprint,mapping_fingerprint,COALESCE(share_strm_id,''),last_seen_run_id
 FROM t_strm_export_state WHERE owner_key='share:default' AND export_key=$1 AND state='active' FOR UPDATE`
const workReexportSelectionSQL = `SELECT selection_revision,selected_candidate_id,stable_relative_path,work_key
 FROM t_share_media_selection WHERE media_item_key=$1 FOR UPDATE`
const workReexportMappingSQL = `SELECT payload FROM t_share_strm WHERE id=$1 FOR UPDATE`
const workReexportInsertSQL = `INSERT INTO t_share_strm(id,payload) VALUES($1,$2) ON CONFLICT(id) DO NOTHING`
const workReexportConfirmSQL = `UPDATE t_share_media_selection SET exported_revision=$2,updated_at=now()
 WHERE media_item_key=$1 AND selection_revision=$2 AND selected_candidate_id=$3 AND stable_relative_path=$4 AND work_key=$5`
const workReexportRemainingSQL = `SELECT EXISTS(SELECT 1 FROM t_strm_export_state WHERE state='active' AND share_strm_id=ANY($1::text[]))`
const workReexportConfigSQL = `SELECT config_revision FROM t_share_export_consumer WHERE consumer='share:default' FOR SHARE`

// WorkReexportMetadata 在 owner 锁连接上的同一事务内完成整批拆分，失败回滚全部映射与凭证，不写文件或删除历史。
// 事务显式回滚以保留 owner 连接，避免任务取消触发自动回滚与连接回收竞态；全部 SQL 和验证仍使用任务 ctx。
func (store *StrmExportDAO) WorkReexportMetadata(ctx context.Context, root string, works []string, revision int64, run string, targets []WorkReexportTarget, verify func(context.Context, []WorkReexportReceipt) error) error {
	if store == nil || store.Conn == nil || len(targets) == 0 || len(targets) > domain.ShareWorkReexportMaxSelections || verify == nil {
		return fmt.Errorf("重导事务需要 owner 锁连接、明确目标与文件验证")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	transaction, err := store.Conn.BeginTx(context.WithoutCancel(ctx), &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err = transaction.ExecContext(ctx, shareSelectionLockSQL); err != nil {
		return err
	}
	var currentRevision int64
	if err = transaction.QueryRowContext(ctx, workReexportConfigSQL).Scan(&currentRevision); err != nil {
		return err
	}
	if currentRevision != revision {
		return ErrShareExportChanged
	}
	legacy := make([]domain.ShareStrmEntry, len(targets))
	selections := make([]domain.ShareSelection, len(targets))
	keys := map[string]bool{}
	urls := make([]string, len(targets))
	for index, target := range targets {
		if keys[target.Selection.ItemKey] {
			return ErrShareExportChanged
		}
		keys[target.Selection.ItemKey] = true
		legacy[index], err = domain.BuildShareStrmEntry(target.Source)
		if err != nil {
			return err
		}
		selections[index] = target.Selection
		urls[index] = target.URLMappingID
	}
	if err = workReexportPreflight(ctx, transaction, root, works, legacy, selections, urls); err != nil {
		return err
	}
	receipts := make([]WorkReexportReceipt, len(targets))
	oldMappings := map[string]bool{}
	newMappings := map[string]bool{}
	for index, target := range targets {
		receipt, readErr := lockWorkReexportReceipt(ctx, transaction, root, target)
		if readErr != nil {
			return readErr
		}
		receipts[index] = receipt
		oldMappings[receipt.State.Playback], oldMappings[target.URLMappingID], oldMappings[legacy[index].ID] = true, true, true
		newMappings[receipt.Entry.ID] = true
	}
	if err = verify(ctx, receipts); err != nil {
		return err
	}
	for index, receipt := range receipts {
		if err = saveWorkReexportReceipt(ctx, transaction, receipt, targets[index].Selection, run); err != nil {
			return err
		}
	}
	remaining := []string{}
	for mapping := range oldMappings {
		if !newMappings[mapping] {
			remaining = append(remaining, mapping)
		}
	}
	sort.Strings(remaining)
	var active bool
	if err = transaction.QueryRowContext(ctx, workReexportRemainingSQL, pq.Array(remaining)).Scan(&active); err != nil {
		return err
	}
	if active {
		return fmt.Errorf("历史 mapping 仍有未处理 active 引用：%w", ErrShareExportChanged)
	}
	if err = verify(ctx, receipts); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return transaction.Commit()
}

func lockWorkReexportReceipt(ctx context.Context, transaction *sql.Tx, root string, target WorkReexportTarget) (WorkReexportReceipt, error) {
	var receipt WorkReexportReceipt
	selection := target.Selection
	var revision, candidate int64
	var relative, work string
	if err := transaction.QueryRowContext(ctx, workReexportSelectionSQL, selection.ItemKey).Scan(&revision, &candidate, &relative, &work); err != nil {
		return receipt, fmt.Errorf("selection %s: %w", selection.ItemKey, errors.Join(err, ErrShareExportChanged))
	}
	if revision != selection.Revision || candidate != selection.CandidateID || relative != selection.RelativePath || work != selection.WorkKey {
		return receipt, ErrShareExportChanged
	}
	fresh, err := readWorkReexportSource(ctx, transaction, target.Source.ID)
	if err != nil {
		return receipt, fmt.Errorf("source %d: %w", target.Source.ID, errors.Join(err, ErrShareExportChanged))
	}
	if !reflect.DeepEqual(fresh, target.Source) {
		return receipt, ErrShareExportChanged
	}
	receipt.State.Owner, receipt.State.Key = "share:default", selection.ItemKey
	if err = transaction.QueryRowContext(ctx, workReexportStateSQL, selection.ItemKey).Scan(&receipt.State.Path, &receipt.State.Content, &receipt.State.Mapping, &receipt.State.Playback, &receipt.State.Run); err != nil {
		return receipt, fmt.Errorf("active state %s: %w", selection.ItemKey, errors.Join(err, ErrShareExportChanged))
	}
	if receipt.State.Path != filepath.Join(root, relative) {
		return receipt, ErrShareExportChanged
	}
	receipt.Mapping, err = readWorkReexportMapping(ctx, transaction, receipt.State.Playback)
	if err != nil {
		return receipt, err
	}
	receipt.URLMapping, err = readWorkReexportMapping(ctx, transaction, target.URLMappingID)
	if err != nil {
		return receipt, err
	}
	receipt.Entry, err = domain.BuildShareWorkReexportEntry(target.Source, selection.ItemKey)
	return receipt, err
}

func saveWorkReexportReceipt(ctx context.Context, transaction *sql.Tx, receipt WorkReexportReceipt, selection domain.ShareSelection, run string) error {
	raw, err := json.Marshal(receipt.Entry)
	if err != nil {
		return err
	}
	if _, err = transaction.ExecContext(ctx, workReexportInsertSQL, receipt.Entry.ID, string(raw)); err != nil {
		return err
	}
	actual, err := readWorkReexportMapping(ctx, transaction, receipt.Entry.ID)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, receipt.Entry) {
		return fmt.Errorf("新映射 ID 已被不同 payload 占用：%w", ErrShareExportChanged)
	}
	state := receipt.State
	state.Playback, state.Mapping, state.Run = receipt.Entry.ID, fmt.Sprintf("%x", sha256.Sum256(raw)), run
	if err = saveStrmExportState(ctx, transaction, state); err != nil {
		return err
	}
	result, err := transaction.ExecContext(ctx, workReexportConfirmSQL, selection.ItemKey, selection.Revision, selection.CandidateID, selection.RelativePath, selection.WorkKey)
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
	var actualState ExportState
	actualState.Owner, actualState.Key = state.Owner, state.Key
	if err = transaction.QueryRowContext(ctx, workReexportStateSQL, state.Key).Scan(&actualState.Path, &actualState.Content, &actualState.Mapping, &actualState.Playback, &actualState.Run); err != nil {
		return err
	}
	if actualState != state {
		return fmt.Errorf("写后 state 校验失败：%w", ErrShareExportChanged)
	}
	return nil
}

func readWorkReexportMapping(ctx context.Context, transaction *sql.Tx, id string) (domain.ShareStrmEntry, error) {
	var entry domain.ShareStrmEntry
	if id == "" {
		return entry, fmt.Errorf("播放 mapping ID 为空：%w", ErrShareExportChanged)
	}
	var raw []byte
	if err := transaction.QueryRowContext(ctx, workReexportMappingSQL, id).Scan(&raw); err != nil {
		return entry, errors.Join(err, ErrShareExportChanged)
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		return entry, err
	}
	if entry.ID != id {
		return entry, ErrShareExportChanged
	}
	return entry, nil
}

func readWorkReexportSource(ctx context.Context, transaction *sql.Tx, id int) (domain.ShareStrmSource, error) {
	var source domain.ShareStrmSource
	var result, episodes []byte
	err := transaction.QueryRowContext(ctx, shareStrmFileSourceSQL+` FOR SHARE OF f,s,m`, id).Scan(&source.ID, &source.ShareID, &source.ShareName, &source.WorkKey, &source.URL, &source.Password, &source.FileName, &source.RemoteFileID, &result, &episodes)
	if err != nil {
		return source, err
	}
	if err = json.Unmarshal(result, &source.Result); err != nil {
		return source, err
	}
	var metadata struct {
		MediaID      int   `json:"_media_id"`
		FileVersion  int   `json:"_file_version"`
		ShareVersion int   `json:"_share_version"`
		FileSize     int64 `json:"_file_size"`
		Available    bool  `json:"_available"`
	}
	if err = json.Unmarshal(result, &metadata); err != nil {
		return source, err
	}
	source.MediaID, source.FileVersion, source.ShareVersion, source.FileSize, source.Available = metadata.MediaID, metadata.FileVersion, metadata.ShareVersion, metadata.FileSize, metadata.Available
	err = json.Unmarshal(episodes, &source.Episodes)
	return source, err
}
