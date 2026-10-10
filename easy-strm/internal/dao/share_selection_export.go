package dao

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
)

// ResolveSelection 保持有效选择，手选失效报错；仅 auto 失效时按确定性候补重选。
func (d *ShareRecordDAO) ResolveSelection(ctx context.Context, key string) (domain.ShareSelection, int, error) {
	transaction, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ShareSelection{}, 0, err
	}
	defer transaction.Rollback()
	if _, err = transaction.ExecContext(ctx, shareSelectionLockSQL); err != nil {
		return domain.ShareSelection{}, 0, err
	}
	selection, err := scanShareSelection(transaction.QueryRowContext(ctx, `SELECT `+shareSelectionColumns+shareSelectionFrom+` WHERE s.media_item_key=$1 FOR UPDATE OF s`, key))
	if err != nil {
		return selection, 0, err
	}
	if !selection.Valid {
		if selection.Mode == "manual" {
			return selection, 0, ErrShareSelectionUnavailable
		}
		var candidate int64
		err = transaction.QueryRowContext(ctx, `SELECT c.candidate_id FROM t_share_export_candidate_item i JOIN t_share_export_candidate c USING(candidate_id)
 JOIN t_share_media_file f ON f.id=c.source_file_id JOIN t_share_record r ON r.id=f.share_id JOIN t_share_media m ON m.id=f.media_id
 WHERE i.work_key=$1 AND i.season_number=$2 AND i.episode_number=$3 AND NOT i.revoked AND c.available AND c.state='active' AND f.available AND NOT r.share_cancelled AND f.status='identified' AND m.work_key=i.work_key
 AND ((m.media_type='movie' AND i.season_number=0 AND i.episode_number=0) OR EXISTS(SELECT 1 FROM t_share_media_file_episode e WHERE e.file_id=f.id AND e.season_number=i.season_number AND e.episode_number=i.episode_number))
 ORDER BY c.file_size DESC,c.share_id,c.source_file_id,c.candidate_id LIMIT 1`, selection.WorkKey, selection.Season, selection.Episode).Scan(&candidate)
		if err == sql.ErrNoRows {
			return selection, 0, ErrShareSelectionUnavailable
		}
		if err != nil {
			return selection, 0, err
		}
		if _, err = transaction.ExecContext(ctx, `UPDATE t_share_media_selection SET selected_candidate_id=$2,selection_revision=selection_revision+1,updated_at=now() WHERE media_item_key=$1 AND selection_mode='auto'`, key, candidate); err != nil {
			return selection, 0, err
		}
		selection.CandidateID = candidate
		selection.Revision++
		selection.Valid = true
		if _, err = transaction.ExecContext(ctx, `SELECT share_export_enqueue(ARRAY[$1]::text[],'selection-fallback')`, selection.WorkKey); err != nil {
			return selection, 0, err
		}
	}
	var source int
	if err = transaction.QueryRowContext(ctx, `SELECT source_file_id FROM t_share_export_candidate WHERE candidate_id=$1`, selection.CandidateID).Scan(&source); err != nil {
		return selection, 0, err
	}
	return selection, source, transaction.Commit()
}

// WorkSelectionKeys 读取含失效选择的全部条目，零候选也不能静默确认待办。
func (d *ShareRecordDAO) WorkSelectionKeys(ctx context.Context, work string) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT media_item_key FROM t_share_media_selection WHERE work_key=$1 ORDER BY season_number,episode_number`, work)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// AnchorSelectionPath 在文件发布前固定路径，发布或凭证失败的重试仍使用同一路径。
func (d *ShareRecordDAO) AnchorSelectionPath(ctx context.Context, key, relative string) (string, error) {
	transaction, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer transaction.Rollback()
	if _, err = transaction.ExecContext(ctx, shareSelectionLockSQL); err != nil {
		return "", err
	}
	var stable string
	if err = transaction.QueryRowContext(ctx, `SELECT stable_relative_path FROM t_share_media_selection WHERE media_item_key=$1 FOR UPDATE`, key).Scan(&stable); err != nil {
		return "", err
	}
	if stable == "" {
		if _, err = transaction.ExecContext(ctx, `UPDATE t_share_media_selection SET stable_relative_path=$2,updated_at=now() WHERE media_item_key=$1 AND stable_relative_path=''`, key, relative); err != nil {
			return "", err
		}
		stable = relative
	}
	return stable, transaction.Commit()
}

// SaveSelectionExport 在凭证事务中确认锚定路径，旧版本完成不确认新选择。
func (d *StrmExportDAO) SaveSelectionExport(ctx context.Context, state ExportState, selection domain.ShareSelection, relative string) error {
	transaction, err := d.Conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err = transaction.ExecContext(ctx, shareSelectionLockSQL); err != nil {
		return err
	}
	var revision int64
	var stable string
	if err = transaction.QueryRowContext(ctx, `SELECT selection_revision,stable_relative_path FROM t_share_media_selection WHERE media_item_key=$1 FOR UPDATE`, selection.ItemKey).Scan(&revision, &stable); err != nil {
		return err
	}
	if stable != "" && stable != relative {
		return ErrShareExportChanged
	}
	if err = saveStrmExportState(ctx, transaction, state); err != nil {
		return err
	}
	if _, err = transaction.ExecContext(ctx, `UPDATE t_share_media_selection SET stable_relative_path=CASE WHEN stable_relative_path='' THEN $2 ELSE stable_relative_path END,
 exported_revision=CASE WHEN selection_revision=$3 THEN $3 ELSE exported_revision END,updated_at=now() WHERE media_item_key=$1`, selection.ItemKey, relative, selection.Revision); err != nil {
		return err
	}
	if err = transaction.Commit(); err != nil {
		return err
	}
	if revision != selection.Revision {
		return ErrShareExportChanged
	}
	return nil
}
