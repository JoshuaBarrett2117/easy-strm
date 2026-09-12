package dao

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"
)

// ShareRecordDAO 负责分享主表和媒体子表的数据访问。
type ShareRecordDAO struct{ db *sql.DB }

// SetShareCancelled 保存网盘确认的状态，避免并发编辑链接后将旧检查结果写入新链接。
func (d *ShareRecordDAO) SetShareCancelled(ctx context.Context, record domain.ShareRecord, cancelled bool) error {
	result, err := d.db.ExecContext(ctx, "UPDATE t_share_record SET share_cancelled=$1,version=version+1,updated_at=NOW() WHERE id=$2 AND url=$3 AND password=$4", cancelled, record.ID, record.URL, record.Password)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("分享已被修改或删除，请重新识别")
	}
	return nil
}

func NewShareRecordDAO(db *sql.DB) *ShareRecordDAO { return &ShareRecordDAO{db} }

// MediaType 获取媒体所属分享配置，供单条识别复用。
func (d *ShareRecordDAO) MediaType(ctx context.Context, id int) (string, error) {
	var value string
	err := d.db.QueryRowContext(ctx, "SELECT s.media_type FROM t_share_record s JOIN t_share_media_file f ON f.share_id=s.id WHERE f.id=$1", id).Scan(&value)
	return value, err
}
func (d *ShareRecordDAO) List(ctx context.Context, q domain.ShareRecordQuery) (domain.ShareRecordPage, error) {
	args := []interface{}{}
	where := ""
	if q.Keyword != "" {
		where = " WHERE (s.name ILIKE $1 OR s.url ILIKE $1)"
		args = append(args, "%"+q.Keyword+"%")
	}
	if q.ShareID > 0 {
		if where == "" {
			where = " WHERE "
		} else {
			where += " AND "
		}
		args = append(args, q.ShareID)
		where += fmt.Sprintf("s.id=$%d", len(args))
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 200 {
		q.PageSize = 20
	}
	var total int
	if e := d.db.QueryRowContext(ctx, "SELECT count(*) FROM t_share_record s"+where, args...).Scan(&total); e != nil {
		return domain.ShareRecordPage{}, e
	}
	if q.Summary {
		return d.listSummaries(ctx, q, where, args, total)
	}
	args = append(args, q.PageSize, (q.Page-1)*q.PageSize)
	// 先对分享主表分页，再加载该页分享的全部媒体，避免媒体行占用分享页数并截断媒体列表。
	rows, e := d.db.QueryContext(ctx, fmt.Sprintf("SELECT s.id,s.media_type,s.name,s.url,s.password,s.note,s.version,s.created_at::text,s.updated_at::text,s.share_cancelled,m.id,m.file_name,m.metadata_source,m.status,m.result,m.error,m.version FROM (SELECT s.id,s.media_type,s.name,s.url,s.password,s.note,s.version,s.created_at,s.updated_at,s.share_cancelled FROM t_share_record s%s ORDER BY s.id DESC LIMIT $%d OFFSET $%d) s LEFT JOIN v_share_media_detail m ON m.share_id=s.id AND m.available ORDER BY s.id DESC,m.id ASC", where, len(args)-1, len(args)), args...)
	if e != nil {
		return domain.ShareRecordPage{}, e
	}
	defer rows.Close()
	by := map[int]*domain.ShareRecord{}
	order := []int{}
	for rows.Next() {
		var r domain.ShareRecord
		var mid sql.NullInt64
		var mn, ms, mt, me sql.NullString
		var b []byte
		var mv sql.NullInt64
		if e = rows.Scan(&r.ID, &r.MediaType, &r.Name, &r.URL, &r.Password, &r.Note, &r.Version, &r.CreatedAt, &r.UpdatedAt, &r.ShareCancelled, &mid, &mn, &ms, &mt, &b, &me, &mv); e != nil {
			return domain.ShareRecordPage{}, e
		}
		p := by[r.ID]
		if p == nil {
			r.Media = []domain.ShareMedia{}
			by[r.ID] = &r
			order = append(order, r.ID)
			p = &r
		}
		if mid.Valid {
			m := domain.ShareMedia{ID: int(mid.Int64), ShareID: r.ID, FileName: mn.String, Available: true, MetadataSource: ms.String, Status: mt.String, Error: me.String, Version: int(mv.Int64)}
			if len(b) > 0 {
				_ = json.Unmarshal(b, &m.Result)
			}
			p.Media = append(p.Media, m)
		}
	}
	out := make([]domain.ShareRecord, 0, len(order))
	for _, id := range order {
		out = append(out, *by[id])
	}
	return domain.ShareRecordPage{Data: out, Total: total}, rows.Err()
}
func (d *ShareRecordDAO) Create(ctx context.Context, r *domain.ShareRecord) error {
	tx, e := d.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = tx.QueryRowContext(ctx, "INSERT INTO t_share_record(name,url,password,note,media_type) VALUES($1,$2,$3,$4,$5) RETURNING id,version,created_at::text,updated_at::text", r.Name, r.URL, r.Password, r.Note, r.MediaType).Scan(&r.ID, &r.Version, &r.CreatedAt, &r.UpdatedAt); e != nil {
		return e
	}
	for i := range r.Media {
		m := &r.Media[i]
		m.ShareID = r.ID
		if e = insertShareMedia(ctx, tx, m); e != nil {
			return e
		}
	}

	return tx.Commit()
}
func (d *ShareRecordDAO) Update(ctx context.Context, r *domain.ShareRecord) error {
	tx, e := d.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var next int
	if e = tx.QueryRowContext(ctx, "UPDATE t_share_record SET share_cancelled=CASE WHEN url IS DISTINCT FROM $2 OR password IS DISTINCT FROM $3 THEN FALSE ELSE share_cancelled END,name=$1,url=$2,password=$3,note=$4,media_type=$7,version=version+1,updated_at=now() WHERE id=$5 AND version=$6 RETURNING version", r.Name, r.URL, r.Password, r.Note, r.ID, r.Version, r.MediaType).Scan(&next); e != nil {
		return e
	}
	r.Version = next
	return tx.Commit()
}
func (d *ShareRecordDAO) Delete(ctx context.Context, id int) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM t_share_record WHERE id=$1", id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM t_share_media m WHERE NOT EXISTS (SELECT 1 FROM t_share_media_file f WHERE f.media_id=m.id)"); err != nil {
		return err
	}
	return tx.Commit()
}
func (d *ShareRecordDAO) AddMedia(ctx context.Context, m *domain.ShareMedia) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = insertShareMedia(ctx, tx, m); err != nil {
		return err
	}

	return tx.Commit()
}

// AddMediaCandidate 对扫描候选按分享和完整路径幂等写入；脱敏统计仍使用独立的AddMedia语义。
func (d *ShareRecordDAO) AddMediaCandidate(ctx context.Context, m *domain.ShareMedia) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int
	if err = tx.QueryRowContext(ctx, "SELECT id FROM t_share_record WHERE id=$1 FOR UPDATE", m.ShareID).Scan(&id); err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, "SELECT f.id,f.version FROM t_share_media_file f WHERE f.share_id=$1 AND md5(f.file_name)=md5($2) AND f.file_name=$2 ORDER BY f.id LIMIT 1", m.ShareID, m.FileName).Scan(&m.ID, &m.Version)
	if err == sql.ErrNoRows {
		err = insertShareMedia(ctx, tx, m)
	}

	if err != nil {
		return err
	}
	return tx.Commit()
}
func (d *ShareRecordDAO) DeleteMedia(ctx context.Context, id int) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM t_share_media_file WHERE id=$1", id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM t_share_media m WHERE NOT EXISTS (SELECT 1 FROM t_share_media_file f WHERE f.media_id=m.id)"); err != nil {
		return err
	}
	return tx.Commit()
}
func (d *ShareRecordDAO) Identify(ctx context.Context, m domain.ShareMedia, status string, r *domain.TmdbIdentifyResult, msg string, episodes ...domain.ShareEpisode) error {
	b, marshalErr := json.Marshal(r)
	if marshalErr != nil {
		return marshalErr
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, "UPDATE t_share_media_file SET status=$1,result=$2,error=$3,version=version+1,updated_at=now() WHERE id=$4 AND version=$5", status, b, msg, m.ID, m.Version)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("媒体已更新或删除，请刷新后重试")
	}
	var mediaID sql.NullInt64
	if err = tx.QueryRowContext(ctx, "SELECT media_id FROM t_share_media_file WHERE id=$1 FOR UPDATE", m.ID).Scan(&mediaID); err != nil {
		return err
	}
	matched := status == "identified" && r != nil && r.Success
	if matched {
		source, provider, externalID, workKey, identityErr := shareMediaIdentity(r)
		if identityErr != nil {
			return identityErr
		}
		if r.MediaType == "tv" && len(episodes) == 0 {
			return fmt.Errorf("缺少季集信息")
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO t_share_media(metadata_source,metadata_provider,external_id,media_type,tmdb_id,title,original_title,media_year,poster_path,result,work_key,genre_ids,country_codes,rating)
		 VALUES($1,$2,$3,$4,NULLIF($5,0),$6,$7,NULLIF($8,0),$9,$10,$11,$12,$13,$14)
		 ON CONFLICT(metadata_source,metadata_provider,media_type,external_id) DO UPDATE SET
		 tmdb_id=COALESCE(EXCLUDED.tmdb_id,t_share_media.tmdb_id),title=EXCLUDED.title,original_title=EXCLUDED.original_title,
		 media_year=COALESCE(EXCLUDED.media_year,t_share_media.media_year),poster_path=EXCLUDED.poster_path,result=EXCLUDED.result,
		 work_key=EXCLUDED.work_key,genre_ids=EXCLUDED.genre_ids,country_codes=EXCLUDED.country_codes,rating=EXCLUDED.rating,updated_at=now() RETURNING id`, source, provider, externalID, r.MediaType, r.TmdbID, r.Title, r.OriginalTitle, r.Year, r.PosterPath, b, workKey, pq.Array(r.GenreIDs), pq.Array(r.Countries), r.VoteAverage).Scan(&mediaID.Int64)
		if err == nil {
			_, err = tx.ExecContext(ctx, "UPDATE t_share_media_file SET media_id=$1,metadata_source=$2 WHERE id=$3", mediaID.Int64, source, m.ID)
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, "DELETE FROM t_share_media_file_episode WHERE file_id=$1", m.ID)
		}
		for _, episode := range episodes {
			if err != nil {
				break
			}
			if episode.SeasonNumber < 0 || episode.EpisodeNumber <= 0 {
				return fmt.Errorf("季集信息无效")
			}
			_, err = tx.ExecContext(ctx, "INSERT INTO t_share_media_file_episode(file_id,season_number,episode_number) VALUES($1,$2,$3)", m.ID, episode.SeasonNumber, episode.EpisodeNumber)
		}
	} else if mediaID.Valid {
		_, err = tx.ExecContext(ctx, "UPDATE t_share_media_file SET media_id=NULL WHERE id=$1", m.ID)
	}
	if !matched && err == nil {
		_, err = tx.ExecContext(ctx, "DELETE FROM t_share_media_file_episode WHERE file_id=$1", m.ID)
	}
	if !matched && err == nil && mediaID.Valid {
		_, err = tx.ExecContext(ctx, "DELETE FROM t_share_media master WHERE master.id=$1 AND NOT EXISTS (SELECT 1 FROM t_share_media_file f WHERE f.media_id=master.id)", mediaID.Int64)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func shareMediaIdentity(r *domain.TmdbIdentifyResult) (string, string, string, string, error) {
	if r == nil || (r.MediaType != "movie" && r.MediaType != "tv") {
		return "", "", "", "", fmt.Errorf("媒体身份无效")
	}
	source := r.MetadataSource
	if source == "" && r.TmdbID > 0 {
		source = domain.MetadataSourceTMDB
	}
	externalID := r.MetadataID
	if externalID == "" && r.TmdbID > 0 {
		externalID = fmt.Sprint(r.TmdbID)
	}
	if source == "" || externalID == "" {
		return "", "", "", "", fmt.Errorf("媒体外部身份无效")
	}
	workKeyBytes, _ := json.Marshal([]string{source, r.MetadataProvider, r.MediaType, externalID})
	workKey := string(workKeyBytes)
	return source, r.MetadataProvider, externalID, workKey, nil
}

// RecordIdentifyError 记录暂时性识别异常，不破坏已有成功关联和季集映射。
func (d *ShareRecordDAO) RecordIdentifyError(ctx context.Context, m domain.ShareMedia, msg string) error {
	res, err := d.db.ExecContext(ctx, `UPDATE t_share_media_file SET error=$1,
	 status=CASE WHEN media_id IS NULL THEN 'pending' ELSE status END,version=version+1,updated_at=now()
	 WHERE id=$2 AND version=$3`, msg, m.ID, m.Version)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("媒体已更新或删除，请刷新后重试")
	}
	return nil
}

// insertShareMedia 在调用方事务中创建待识别文件候选；匹配成功后再创建媒体实体。
func insertShareMedia(ctx context.Context, tx *sql.Tx, m *domain.ShareMedia) error {
	source := m.MetadataSource
	if source == "" {
		source = domain.MetadataSourceAuto
	}
	if err := tx.QueryRowContext(ctx, "INSERT INTO t_share_media_file(share_id,file_name,metadata_source,available) VALUES($1,$2,$3,TRUE) RETURNING id,version", m.ShareID, m.FileName, source).Scan(&m.ID, &m.Version); err != nil {
		return err
	}
	m.Available = true
	m.Status = "pending"
	m.MetadataSource = source
	return nil
}

// SyncFiles 原子同步一次完整分享扫描；只有调用本方法才会将未见历史文件标记失效。
func (d *ShareRecordDAO) SyncFiles(ctx context.Context, shareID int, scanToken string, files []domain.ShareMedia) (int, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var locked int
	if err = tx.QueryRowContext(ctx, "SELECT id FROM t_share_record WHERE id=$1 FOR UPDATE", shareID).Scan(&locked); err != nil {
		return 0, err
	}
	for i := range files {
		file := &files[i]
		var id int
		var oldSHA string
		err = tx.QueryRowContext(ctx, `SELECT id,sha1 FROM t_share_media_file WHERE share_id=$1 AND
		 ((file_id<>'' AND file_id=$2) OR ($2='' AND file_id='' AND file_name=$3)) ORDER BY id LIMIT 1`, shareID, file.RemoteFileID, file.FileName).Scan(&id, &oldSHA)
		if err == sql.ErrNoRows {
			err = tx.QueryRowContext(ctx, `INSERT INTO t_share_media_file(share_id,file_id,file_name,file_size,sha1,pick_code,metadata_source,status,available,last_seen_at,last_seen_scan_token)
			 VALUES($1,$2,$3,$4,$5,$6,$7,'pending',TRUE,now(),$8) RETURNING id`, shareID, file.RemoteFileID, file.FileName, file.FileSize, file.SHA1, file.PickCode, domain.MetadataSourceAuto, scanToken).Scan(&id)
		} else if err == nil {
			changed := oldSHA != "" && file.SHA1 != "" && oldSHA != file.SHA1
			_, err = tx.ExecContext(ctx, `UPDATE t_share_media_file SET file_name=$1,file_size=$2,sha1=$3,pick_code=$4,available=TRUE,last_seen_at=now(),last_seen_scan_token=$5,
			 status=CASE WHEN $6 THEN 'pending' ELSE status END,media_id=CASE WHEN $6 THEN NULL ELSE media_id END,
			 result=CASE WHEN $6 THEN NULL ELSE result END,error=CASE WHEN $6 THEN '' ELSE error END,version=version+1,updated_at=now() WHERE id=$7`, file.FileName, file.FileSize, file.SHA1, file.PickCode, scanToken, changed, id)
			if err == nil && changed {
				_, err = tx.ExecContext(ctx, "DELETE FROM t_share_media_file_episode WHERE file_id=$1", id)
			}
		}
		if err != nil {
			return 0, err
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE t_share_media_file SET available=FALSE,updated_at=now() WHERE share_id=$1 AND available AND last_seen_scan_token IS DISTINCT FROM $2", shareID, scanToken); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM t_share_media m WHERE NOT EXISTS (SELECT 1 FROM t_share_media_file f WHERE f.media_id=m.id)"); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(files), nil
}
