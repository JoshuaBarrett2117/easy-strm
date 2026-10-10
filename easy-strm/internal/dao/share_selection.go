package dao

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"strings"
)

// ErrShareSelectionConflict 表示旧版本或非有效成员，必须刷新后重试。
var ErrShareSelectionConflict = errors.New("来源选择版本已变化或候选已失效，请刷新后重试")

// ErrShareSelectionUnavailable 表示没有有效选择，必须保留旧输出。
var ErrShareSelectionUnavailable = errors.New("选定来源不可用或识别已改变，保留原文件；请明确改选有效来源后重试")

const shareSelectionLockSQL = `SELECT pg_advisory_xact_lock(34983,1)`
const shareSelectionColumns = `s.media_item_key,s.work_key,s.season_number,s.episode_number,s.selected_candidate_id,s.selection_mode,s.selection_revision,s.exported_revision,s.stable_relative_path,COALESCE(m.title,s.work_key),
 EXISTS(SELECT 1 FROM t_share_export_candidate_item i JOIN t_share_export_candidate c USING(candidate_id)
 JOIN t_share_media_file f ON f.id=c.source_file_id JOIN t_share_record r ON r.id=f.share_id JOIN t_share_media live ON live.id=f.media_id
 WHERE i.candidate_id=s.selected_candidate_id AND i.work_key=s.work_key AND i.season_number=s.season_number AND i.episode_number=s.episode_number
 AND NOT i.revoked AND c.available AND c.state='active' AND f.available AND NOT r.share_cancelled AND f.status='identified' AND live.work_key=s.work_key
 AND ((live.media_type='movie' AND s.season_number=0 AND s.episode_number=0) OR (live.media_type='tv' AND EXISTS(SELECT 1 FROM t_share_media_file_episode e WHERE e.file_id=f.id AND e.season_number=s.season_number AND e.episode_number=s.episode_number))))`
const shareSelectionFrom = ` FROM t_share_media_selection s LEFT JOIN t_share_media m ON m.work_key=s.work_key`

func scanShareSelection(row interface{ Scan(...interface{}) error }) (domain.ShareSelection, error) {
	var value domain.ShareSelection
	err := row.Scan(&value.ItemKey, &value.WorkKey, &value.Season, &value.Episode, &value.CandidateID, &value.Mode, &value.Revision, &value.ExportedRevision, &value.RelativePath, &value.Title, &value.Valid)
	return value, err
}

// CheckSelectionSchema 在任何改选或文件副作用前要求 DBA 手工安装 v47。
func (d *ShareRecordDAO) CheckSelectionSchema(ctx context.Context) error {
	var ready bool
	err := d.db.QueryRowContext(ctx, `SELECT to_regclass('t_share_export_candidate') IS NOT NULL AND to_regclass('t_share_export_candidate_item') IS NOT NULL AND to_regclass('t_share_media_selection') IS NOT NULL`).Scan(&ready)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("来源选择未安装，请 DBA 手工应用 v47，再显式全量导出")
	}
	return nil
}

// ListSelections 在一致只读快照中返回分页与总数；读取接口绝不执行回填。
func (d *ShareRecordDAO) ListSelections(ctx context.Context, q domain.ShareSelectionQuery) ([]domain.ShareSelection, int, error) {
	transaction, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer transaction.Rollback()
	filter := shareSelectionFrom + ` WHERE ($1='' OR strpos(lower(COALESCE(m.title,'')||s.media_item_key),lower($1))>0)`
	var total int
	if err = transaction.QueryRowContext(ctx, `SELECT count(*)`+filter, q.Keyword).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := transaction.QueryContext(ctx, `SELECT `+shareSelectionColumns+filter+` ORDER BY s.work_key,s.season_number,s.episode_number LIMIT $2 OFFSET $3`, q.Keyword, q.PageSize, (q.Page-1)*q.PageSize)
	if err != nil {
		return nil, 0, err
	}
	values := []domain.ShareSelection{}
	for rows.Next() {
		value, scanErr := scanShareSelection(rows)
		if scanErr != nil {
			rows.Close()
			return nil, 0, scanErr
		}
		values = append(values, value)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return nil, 0, err
	}
	return values, total, transaction.Commit()
}

// SelectionDetail 获取当前选择与含撤销成员的候选列表，不返回 URL/密码。
func (d *ShareRecordDAO) SelectionDetail(ctx context.Context, key string) (domain.ShareSelectionDetail, error) {
	value := domain.ShareSelectionDetail{Candidates: []domain.ShareCandidate{}}
	transaction, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return value, err
	}
	defer transaction.Rollback()
	value.Selection, err = scanShareSelection(transaction.QueryRowContext(ctx, `SELECT `+shareSelectionColumns+shareSelectionFrom+` WHERE s.media_item_key=$1`, key))
	if err != nil {
		return value, err
	}
	rows, err := transaction.QueryContext(ctx, `SELECT c.candidate_id,COALESCE(c.source_file_id,0),c.share_id,COALESCE(r.name,''),COALESCE(f.file_name,''),c.file_size,c.first_seen_seq,
 c.available AND c.state='active' AND NOT i.revoked AND COALESCE(f.available,false) AND NOT COALESCE(r.share_cancelled,true) AND COALESCE(f.status='identified',false) AND live.work_key=i.work_key,
 c.state,i.revoked FROM t_share_export_candidate_item i JOIN t_share_export_candidate c USING(candidate_id)
 LEFT JOIN t_share_media_file f ON f.id=c.source_file_id LEFT JOIN t_share_record r ON r.id=c.share_id LEFT JOIN t_share_media live ON live.id=f.media_id
 WHERE i.work_key=$1 AND i.season_number=$2 AND i.episode_number=$3 ORDER BY c.first_seen_seq,c.candidate_id`, value.Selection.WorkKey, value.Selection.Season, value.Selection.Episode)
	if err != nil {
		return value, err
	}
	for rows.Next() {
		var candidate domain.ShareCandidate
		if err = rows.Scan(&candidate.ID, &candidate.SourceID, &candidate.ShareID, &candidate.ShareName, &candidate.FileName, &candidate.FileSize, &candidate.FirstSeen, &candidate.Available, &candidate.State, &candidate.Revoked); err != nil {
			rows.Close()
			return value, err
		}
		value.Candidates = append(value.Candidates, candidate)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return value, err
	}
	return value, transaction.Commit()
}

// ChangeSelection 原子验证成员、CAS 改选并入队，绝不在事务内写文件。
func (d *ShareRecordDAO) ChangeSelection(ctx context.Context, change domain.ShareSelectionChange) error {
	transaction, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err = transaction.ExecContext(ctx, shareSelectionLockSQL); err != nil {
		return err
	}
	var revision int64
	var work string
	err = transaction.QueryRowContext(ctx, `SELECT selection_revision,work_key FROM t_share_media_selection WHERE media_item_key=$1 FOR UPDATE`, change.ItemKey).Scan(&revision, &work)
	if err != nil {
		return err
	}
	if revision != change.ExpectedRevision {
		return ErrShareSelectionConflict
	}
	var valid bool
	err = transaction.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM t_share_media_selection s JOIN t_share_export_candidate_item i ON i.work_key=s.work_key AND i.season_number=s.season_number AND i.episode_number=s.episode_number
 JOIN t_share_export_candidate c USING(candidate_id) JOIN t_share_media_file f ON f.id=c.source_file_id JOIN t_share_record r ON r.id=f.share_id JOIN t_share_media m ON m.id=f.media_id
 WHERE s.media_item_key=$1 AND c.candidate_id=$2 AND NOT i.revoked AND c.available AND c.state='active' AND f.available AND NOT r.share_cancelled AND f.status='identified' AND m.work_key=s.work_key
 AND ((m.media_type='movie' AND s.season_number=0 AND s.episode_number=0) OR EXISTS(SELECT 1 FROM t_share_media_file_episode e WHERE e.file_id=f.id AND e.season_number=s.season_number AND e.episode_number=s.episode_number)))`, change.ItemKey, change.CandidateID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrShareSelectionConflict
	}
	result, err := transaction.ExecContext(ctx, `UPDATE t_share_media_selection SET selected_candidate_id=$2,selection_mode='manual',selection_revision=selection_revision+1,updated_at=now() WHERE media_item_key=$1 AND selection_revision=$3`, change.ItemKey, change.CandidateID, change.ExpectedRevision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrShareSelectionConflict
	}
	if _, err = transaction.ExecContext(ctx, `SELECT share_export_enqueue(ARRAY[$1]::text[],'selection')`, work); err != nil {
		return err
	}
	return transaction.Commit()
}

// SyncSelections 显式从应用库刷新候选，work 非空时仅同步该作品及其旧候选的新归属。
func (d *ShareRecordDAO) SyncSelections(ctx context.Context, work string) error {
	transaction, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err = transaction.ExecContext(ctx, shareSelectionLockSQL); err != nil {
		return err
	}
	for _, query := range shareSelectionSyncSQL {
		args := []interface{}{}
		if strings.Contains(query, "$1") {
			args = append(args, work)
		}
		if _, err = transaction.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("刷新来源选择：%w", err)
		}
	}
	return transaction.Commit()
}

var shareSelectionSyncSQL = []string{
	`CREATE TEMP TABLE selection_observed ON COMMIT DROP AS
 SELECT f.id source_file_id,f.share_id,COALESCE(f.file_id,'') remote_file_id,CASE WHEN COALESCE(f.file_id,'')='' THEN md5(f.file_name) ELSE '' END name_hash,
 f.version file_version,r.version share_version,COALESCE(f.file_size,0) file_size,f.available AND NOT r.share_cancelled available,
 CASE WHEN f.status='identified' AND f.available AND NOT r.share_cancelled AND m.work_key IS NOT NULL THEN 'active' ELSE 'stale' END state,
 COALESCE(m.work_key,'') work_key,m.media_type,COALESCE((SELECT string_agg(e.season_number||':'||e.episode_number,',' ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),'') episode_sig
 FROM t_share_media_file f JOIN t_share_record r ON r.id=f.share_id LEFT JOIN t_share_media m ON m.id=f.media_id
 WHERE ($1='' OR m.work_key=$1 OR EXISTS(SELECT 1 FROM t_share_export_candidate c JOIN t_share_export_candidate_item i USING(candidate_id) WHERE c.source_file_id=f.id AND i.work_key=$1))`,
	`DO $identity$ BEGIN
 IF EXISTS(SELECT 1 FROM selection_observed o JOIN t_share_export_candidate c ON c.source_file_id=o.source_file_id
 WHERE (c.share_id,c.remote_file_id,c.name_hash) IS DISTINCT FROM (o.share_id,o.remote_file_id,o.name_hash)) THEN
 RAISE EXCEPTION '候选源身份已变化；禁止静默重定向，请 DBA 核对旧手选与新源身份后重建';
 END IF; END $identity$`,
	`CREATE TEMP TABLE selection_changed ON COMMIT DROP AS
 SELECT DISTINCT key work_key FROM (
 SELECT o.work_key key FROM selection_observed o LEFT JOIN t_share_export_candidate c ON c.share_id=o.share_id AND c.remote_file_id=o.remote_file_id AND c.name_hash=o.name_hash
 WHERE c.candidate_id IS NULL OR (c.source_file_id,c.file_version,c.share_version,c.file_size,c.available,c.state,c.episode_sig) IS DISTINCT FROM (o.source_file_id,o.file_version,o.share_version,o.file_size,o.available,o.state,o.episode_sig)
 OR NOT EXISTS(SELECT 1 FROM t_share_export_candidate_item i WHERE i.candidate_id=c.candidate_id AND i.work_key=o.work_key AND NOT i.revoked)
 UNION SELECT i.work_key FROM t_share_export_candidate_item i JOIN t_share_export_candidate c USING(candidate_id) LEFT JOIN selection_observed o ON o.source_file_id=c.source_file_id
 WHERE ($1='' OR i.work_key=$1 OR o.source_file_id IS NOT NULL) AND (o.source_file_id IS NULL AND c.state<>'removed' OR o.source_file_id IS NOT NULL AND ((c.file_version,c.share_version,c.available,c.state,c.episode_sig) IS DISTINCT FROM (o.file_version,o.share_version,o.available,o.state,o.episode_sig) OR i.work_key<>o.work_key))
 ) affected WHERE key<>''`,
	`INSERT INTO t_share_export_candidate(share_id,remote_file_id,source_file_id,name_hash,file_version,share_version,file_size,available,state,episode_sig)
 SELECT share_id,remote_file_id,source_file_id,name_hash,file_version,share_version,file_size,available,state,episode_sig FROM selection_observed ORDER BY source_file_id
 ON CONFLICT DO NOTHING`,
	`UPDATE t_share_export_candidate c SET source_file_id=o.source_file_id,file_version=o.file_version,share_version=o.share_version,file_size=o.file_size,available=o.available,state=o.state,episode_sig=o.episode_sig,updated_at=now()
 FROM selection_observed o WHERE c.share_id=o.share_id AND c.remote_file_id=o.remote_file_id AND c.name_hash=o.name_hash`,
	`UPDATE t_share_export_candidate c SET state='removed',available=false,updated_at=now() WHERE NOT EXISTS(SELECT 1 FROM t_share_media_file f WHERE f.id=c.source_file_id)
 AND ($1='' OR EXISTS(SELECT 1 FROM t_share_export_candidate_item i WHERE i.candidate_id=c.candidate_id AND i.work_key=$1)) AND c.state<>'removed'`,
	`UPDATE t_share_export_candidate_item i SET revoked=true,updated_at=now() WHERE NOT i.revoked AND ($1='' OR i.work_key=$1 OR EXISTS(SELECT 1 FROM selection_observed o JOIN t_share_export_candidate c ON c.source_file_id=o.source_file_id WHERE c.candidate_id=i.candidate_id))
 AND NOT EXISTS(SELECT 1 FROM selection_observed o JOIN t_share_export_candidate c ON c.source_file_id=o.source_file_id WHERE c.candidate_id=i.candidate_id AND o.work_key=i.work_key
 AND ((o.media_type='movie' AND i.season_number=0 AND i.episode_number=0) OR (o.media_type='tv' AND EXISTS(SELECT 1 FROM t_share_media_file_episode e WHERE e.file_id=o.source_file_id AND e.season_number=i.season_number AND e.episode_number=i.episode_number))))`,
	`INSERT INTO t_share_export_candidate_item(candidate_id,work_key,season_number,episode_number)
 SELECT c.candidate_id,o.work_key,0,0 FROM selection_observed o JOIN t_share_export_candidate c ON c.source_file_id=o.source_file_id WHERE o.media_type='movie' AND o.work_key<>''
 UNION SELECT c.candidate_id,o.work_key,e.season_number,e.episode_number FROM selection_observed o JOIN t_share_export_candidate c ON c.source_file_id=o.source_file_id JOIN t_share_media_file_episode e ON e.file_id=o.source_file_id WHERE o.media_type='tv' AND o.work_key<>''
 ON CONFLICT(candidate_id,work_key,season_number,episode_number) DO UPDATE SET revoked=false,updated_at=now()`,
	`DELETE FROM t_share_export_candidate_item i WHERE i.revoked AND NOT EXISTS(SELECT 1 FROM t_share_media_selection s WHERE s.selected_candidate_id=i.candidate_id AND s.work_key=i.work_key AND s.season_number=i.season_number AND s.episode_number=i.episode_number) AND ($1='' OR i.work_key=$1 OR i.work_key IN(SELECT work_key FROM selection_changed))`,
	`INSERT INTO t_share_media_selection(work_key,season_number,episode_number,selected_candidate_id)
 SELECT DISTINCT ON(i.work_key,i.season_number,i.episode_number) i.work_key,i.season_number,i.episode_number,c.candidate_id FROM t_share_export_candidate_item i JOIN t_share_export_candidate c USING(candidate_id)
 WHERE NOT i.revoked AND c.state='active' AND c.available AND ($1='' OR i.work_key=$1 OR i.work_key IN(SELECT work_key FROM selection_changed))
 ORDER BY i.work_key,i.season_number,i.episode_number,c.first_seen_seq,c.candidate_id ON CONFLICT DO NOTHING`,
	`SELECT share_export_enqueue(ARRAY(SELECT work_key FROM selection_changed ORDER BY work_key),'selection-sync') WHERE $1 IS NOT NULL`,
}
