package dao

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
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
	err := d.db.QueryRowContext(ctx, "SELECT s.media_type FROM t_share_record s JOIN t_share_media m ON m.share_id=s.id WHERE m.id=$1", id).Scan(&value)
	return value, err
}
func (d *ShareRecordDAO) List(ctx context.Context, q domain.ShareRecordQuery) (domain.ShareRecordPage, error) {
	args := []interface{}{}
	where := ""
	if q.Keyword != "" {
		where = " WHERE (s.name ILIKE $1 OR s.url ILIKE $1)"
		args = append(args, "%"+q.Keyword+"%")
	}
	if q.ShareID>0 {
		if where==""{where=" WHERE "}else{where+=" AND "}
		args=append(args,q.ShareID);where+=fmt.Sprintf("s.id=$%d",len(args))
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
	rows, e := d.db.QueryContext(ctx, fmt.Sprintf("SELECT s.id,s.media_type,s.name,s.url,s.password,s.note,s.version,s.created_at::text,s.updated_at::text,s.share_cancelled,m.id,m.file_name,m.metadata_source,m.status,m.result,m.error,m.version FROM (SELECT s.id,s.media_type,s.name,s.url,s.password,s.note,s.version,s.created_at,s.updated_at,s.share_cancelled FROM t_share_record s%s ORDER BY s.id DESC LIMIT $%d OFFSET $%d) s LEFT JOIN t_share_media m ON m.share_id=s.id ORDER BY s.id DESC,m.id ASC", where, len(args)-1, len(args)), args...)
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
			m := domain.ShareMedia{ID: int(mid.Int64), ShareID: r.ID, FileName: mn.String, MetadataSource: ms.String, Status: mt.String, Error: me.String, Version: int(mv.Int64)}
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
		if e = tx.QueryRowContext(ctx, "INSERT INTO t_share_media(share_id,file_name,metadata_source) VALUES($1,$2,$3) RETURNING id,version", r.ID, m.FileName, m.MetadataSource).Scan(&m.ID, &m.Version); e != nil {
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
	_, e := d.db.ExecContext(ctx, "DELETE FROM t_share_record WHERE id=$1", id)
	return e
}
func (d *ShareRecordDAO) AddMedia(ctx context.Context, m *domain.ShareMedia) error {
	return d.db.QueryRowContext(ctx, "INSERT INTO t_share_media(share_id,file_name,metadata_source) VALUES($1,$2,$3) RETURNING id,version", m.ShareID, m.FileName, m.MetadataSource).Scan(&m.ID, &m.Version)
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
	err = tx.QueryRowContext(ctx, "SELECT id,version FROM t_share_media WHERE share_id=$1 AND file_name=$2 ORDER BY id LIMIT 1", m.ShareID, m.FileName).Scan(&m.ID, &m.Version)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx, "INSERT INTO t_share_media(share_id,file_name,metadata_source) VALUES($1,$2,$3) RETURNING id,version", m.ShareID, m.FileName, m.MetadataSource).Scan(&m.ID, &m.Version)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (d *ShareRecordDAO) DeleteMedia(ctx context.Context, id int) error {
	_, e := d.db.ExecContext(ctx, "DELETE FROM t_share_media WHERE id=$1", id)
	return e
}
func (d *ShareRecordDAO) Identify(ctx context.Context, m domain.ShareMedia, status string, r *domain.TmdbIdentifyResult, msg string) error {
	b, _ := json.Marshal(r)
	res, e := d.db.ExecContext(ctx, "UPDATE t_share_media SET status=$1,result=$2,error=$3,version=version+1,updated_at=now() WHERE id=$4 AND version=$5", status, b, msg, m.ID, m.Version)
	if e != nil {
		return e
	}
	count, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if count == 0 {
		return fmt.Errorf("媒体已更新或删除，请刷新后重试")
	}
	return nil
}
