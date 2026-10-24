package dao

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"errors"
	"time"
)

// LockSelectionPublication 按输出路径锁之后获取会话级选择锁，保护目录发布与凭证免受并发改选。
func (d *StrmExportDAO) LockSelectionPublication(ctx context.Context) (func() error, error) {
	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := d.Conn.ExecContext(lockCtx, `SELECT pg_advisory_lock(34983,1)`); err != nil {
		d.Close()
		return nil, err
	}
	return func() error {
		unlockCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer stop()
		var unlocked bool
		err := d.Conn.QueryRowContext(unlockCtx, `SELECT pg_advisory_unlock(34983,1)`).Scan(&unlocked)
		if err != nil || !unlocked {
			d.Close()
			return errors.Join(ErrShareExportChanged, err)
		}
		return nil
	}, nil
}

// ResolveSelection 保持有效选择，手选失效报错；仅 auto 失效时按确定性候补重选。
func (d *ShareRecordDAO) ResolveSelection(ctx context.Context, key string) (domain.ShareSelection, int, error) {
	transaction, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
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
	if selection.SourceKind == "strm115" {
		_, err = selection115Candidate(ctx, transaction, selection)
		if err != nil {
			return selection, 0, err
		}
		selection.Valid = true
		return selection, 0, transaction.Commit()
	}
	if selection.SourceKind != "share" {
		return selection, 0, ErrShareSelectionUnavailable
	}
	if !selection.Valid {
		if selection.Mode == "manual" {
			return selection, 0, ErrShareSelectionUnavailable
		}
		var candidate int64
		err = transaction.QueryRowContext(ctx, `SELECT c.candidate_id`+shareSelectionLiveJoin+`
 WHERE i.work_key=$1 AND i.season_number=$2 AND i.episode_number=$3 AND `+shareSelectionLivePredicate+`
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

// AnchorSelectionPath 在文件发布前固定路径，发布或凭证失败的重试仍使用同一路径。
func (d *ShareRecordDAO) AnchorSelectionPath(ctx context.Context, key, relative string) (string, error) {
	transaction, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
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
	transaction, err := d.Conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
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
	if selection.SourceKind == "strm115" {
		var kind, ref string
		if err = transaction.QueryRowContext(ctx, `SELECT source_kind,external_ref FROM t_share_media_selection WHERE media_item_key=$1`, selection.ItemKey).Scan(&kind, &ref); err != nil {
			return err
		}
		if revision != selection.Revision || kind != selection.SourceKind || ref != selection.ExternalRef {
			return ErrShareExportChanged
		}
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
