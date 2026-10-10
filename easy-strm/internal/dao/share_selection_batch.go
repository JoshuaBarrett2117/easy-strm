package dao

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
)

// ShareSelectionPreparationError 保留全部失效键；错误文案只展示计数和有限示例。
type ShareSelectionPreparationError struct {
	ManualKeys []string `json:"manual_keys"`
	AutoKeys   []string `json:"auto_keys"`
}

// Error 返回计数和最多五个示例，完整键清单保留在类型字段中。
func (failure *ShareSelectionPreparationError) Error() string {
	examples := make([]string, 0, 5)
	examples = append(examples, failure.ManualKeys[:min(5, len(failure.ManualKeys))]...)
	examples = append(examples, failure.AutoKeys[:min(5-len(examples), len(failure.AutoKeys))]...)
	return fmt.Sprintf("来源准备失败：手选失效 %d 项，自动选择无候补 %d 项；示例：%s；保留原文件，请明确改选后重试", len(failure.ManualKeys), len(failure.AutoKeys), strings.Join(examples, ", "))
}

// Unwrap 保持既有不可用错误的 errors.Is 兼容性。
func (failure *ShareSelectionPreparationError) Unwrap() error { return ErrShareSelectionUnavailable }

const shareSelectionLiveJoin = ` FROM t_share_export_candidate_item i JOIN t_share_export_candidate c USING(candidate_id)
 JOIN t_share_media_file f ON f.id=c.source_file_id JOIN t_share_record r ON r.id=f.share_id JOIN t_share_media live ON live.id=f.media_id`
const shareSelectionLivePredicate = `NOT i.revoked AND c.available AND c.state='active' AND f.available AND NOT r.share_cancelled AND f.status='identified' AND live.work_key=i.work_key
 AND ((live.media_type='movie' AND i.season_number=0 AND i.episode_number=0) OR (live.media_type='tv' AND EXISTS(SELECT 1 FROM t_share_media_file_episode e WHERE e.file_id=f.id AND e.season_number=i.season_number AND e.episode_number=i.episode_number)))`

var shareSelectionPrepareSQL = []string{
	`CREATE TEMP TABLE selection_valid_member ON COMMIT DROP AS
 SELECT i.candidate_id,i.work_key,i.season_number,i.episode_number,c.share_id,c.source_file_id,c.file_size` + shareSelectionLiveJoin + ` WHERE ($1='' OR i.work_key=$1) AND ` + shareSelectionLivePredicate,
	`CREATE UNIQUE INDEX ON selection_valid_member(work_key,season_number,episode_number,candidate_id)`,
	`ANALYZE selection_valid_member`,
	`CREATE TEMP TABLE selection_fallback ON COMMIT DROP AS
 SELECT DISTINCT ON(work_key,season_number,episode_number) work_key,season_number,episode_number,candidate_id
 FROM selection_valid_member ORDER BY work_key,season_number,episode_number,file_size DESC,share_id,source_file_id,candidate_id`,
	`CREATE UNIQUE INDEX ON selection_fallback(work_key,season_number,episode_number)`,
	`ANALYZE selection_fallback`,
	`CREATE TEMP TABLE selection_resolution ON COMMIT DROP AS
	 SELECT s.media_item_key,s.work_key,s.selection_mode,s.selected_candidate_id,b.candidate_id fallback_candidate
 FROM t_share_media_selection s
 LEFT JOIN selection_valid_member v ON v.work_key=s.work_key AND v.season_number=s.season_number AND v.episode_number=s.episode_number AND v.candidate_id=s.selected_candidate_id
 LEFT JOIN selection_fallback b ON b.work_key=s.work_key AND b.season_number=s.season_number AND b.episode_number=s.episode_number
 WHERE ($1='' OR s.work_key=$1) AND v.candidate_id IS NULL`,
	`CREATE UNIQUE INDEX ON selection_resolution(media_item_key)`,
	`ANALYZE selection_resolution`,
}

const shareSelectionPrepareErrorsSQL = `SELECT media_item_key,selection_mode FROM selection_resolution
 WHERE selection_mode='manual' OR fallback_candidate IS NULL ORDER BY media_item_key`
const shareSelectionPrepareUpdateSQL = `UPDATE t_share_media_selection s
 SET selected_candidate_id=r.fallback_candidate,selection_revision=s.selection_revision+1,updated_at=now()
 FROM selection_resolution r WHERE s.media_item_key=r.media_item_key AND s.selection_mode='auto' AND r.selection_mode='auto'
	 AND s.selected_candidate_id=r.selected_candidate_id AND r.fallback_candidate IS NOT NULL RETURNING s.work_key`

func lockShareSelection(ctx context.Context, transaction *sql.Tx) error {
	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := transaction.ExecContext(lockCtx, shareSelectionLockSQL)
	return err
}

// ResolveSelections 原子解析全库或指定作品，单次物化有效成员，绝不为失效手选自动候补。
func (store *ShareRecordDAO) ResolveSelections(ctx context.Context, work string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer transaction.Rollback()
	if err = lockShareSelection(ctx, transaction); err != nil {
		return 0, err
	}
	for _, query := range shareSelectionPrepareSQL {
		if err = ctx.Err(); err != nil {
			return 0, err
		}
		args := []interface{}{}
		if strings.Contains(query, "$1") {
			args = append(args, work)
		}
		if _, err = transaction.ExecContext(ctx, query, args...); err != nil {
			return 0, fmt.Errorf("集合解析来源选择：%w", err)
		}
	}
	rows, err := transaction.QueryContext(ctx, shareSelectionPrepareErrorsSQL)
	if err != nil {
		return 0, err
	}
	invalid := &ShareSelectionPreparationError{}
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			rows.Close()
			return 0, err
		}
		var key, mode string
		if err = rows.Scan(&key, &mode); err != nil {
			rows.Close()
			return 0, err
		}
		if mode == "manual" {
			invalid.ManualKeys = append(invalid.ManualKeys, key)
		} else {
			invalid.AutoKeys = append(invalid.AutoKeys, key)
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err = rows.Close(); err != nil {
		return 0, err
	}
	if len(invalid.ManualKeys)+len(invalid.AutoKeys) != 0 {
		return 0, invalid
	}
	rows, err = transaction.QueryContext(ctx, shareSelectionPrepareUpdateSQL)
	if err != nil {
		return 0, err
	}
	changed, works := 0, map[string]bool{}
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			rows.Close()
			return 0, err
		}
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return 0, err
		}
		changed++
		works[key] = true
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err = rows.Close(); err != nil {
		return 0, err
	}
	if changed > 0 {
		keys := make([]string, 0, len(works))
		for key := range works {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if _, err = transaction.ExecContext(ctx, `SELECT share_export_enqueue($1::text[],'selection-fallback')`, pq.Array(keys)); err != nil {
			return 0, err
		}
	}
	if err = ctx.Err(); err != nil {
		return 0, err
	}
	if err = transaction.Commit(); err != nil {
		return 0, err
	}
	return changed, nil
}
