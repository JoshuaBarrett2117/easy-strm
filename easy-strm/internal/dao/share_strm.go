package dao

import (
	"context"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
)

// StrmSources 按当前作品筛选条件遍历有效来源；分页采用来源ID游标。
func (d *ShareRecordDAO) StrmSources(ctx context.Context, q domain.ShareLibraryQuery, after int) ([]domain.ShareStrmSource, error) {
	where, args := libraryWhere(q)
	args = append(args, after)
	query := libraryWorks + ` SELECT m.id,m.work_key,s.url,s.password,m.file_name,m.result,count(*) OVER() FROM t_share_media m JOIN t_share_record s ON s.id=m.share_id JOIN works w ON w.work_key=m.work_key WHERE w.work_key IN (SELECT work_key FROM works WHERE ` + where + `) AND NOT s.share_cancelled AND m.status='identified' AND m.id>$` + fmt.Sprint(len(args)) + ` ORDER BY m.id LIMIT 100`
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ShareStrmSource{}
	for rows.Next() {
		var v domain.ShareStrmSource
		var raw []byte
		if err = rows.Scan(&v.ID, &v.WorkKey, &v.URL, &v.Password, &v.FileName, &raw, &v.Remaining); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &v.Result); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SaveStrmEntry 重复导出保持相同ID，并刷新分享提取码。
func (d *ShareRecordDAO) SaveStrmEntry(ctx context.Context, v domain.ShareStrmEntry) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = d.db.ExecContext(ctx, `INSERT INTO t_share_strm(id,payload) VALUES($1,$2) ON CONFLICT(id) DO UPDATE SET payload=CASE WHEN COALESCE(EXCLUDED.payload->>'file_id','')='' THEN EXCLUDED.payload || jsonb_build_object('file_id',COALESCE(t_share_strm.payload->>'file_id','')) ELSE EXCLUDED.payload END,updated_at=NOW()`, v.ID, string(raw))
	return err
}

// SaveExportedStrmFile 按导出路径维护分享STRM清单，保留首次创建时间并刷新更新时间。
func (d *ShareRecordDAO) SaveExportedStrmFile(ctx context.Context, v domain.StrmFile) error {
	_, err := d.db.ExecContext(ctx, `INSERT INTO t_strm_file(strm_config_id,file_name,file_path,local_strm_path)
		VALUES(-1,$1,$2,$3) ON CONFLICT(strm_config_id,file_path) DO UPDATE SET
		file_name=EXCLUDED.file_name,local_strm_path=EXCLUDED.local_strm_path,update_time=NOW()`, v.FileName, v.FilePath, v.LocalStrmPath)
	return err
}

// GetStrmEntry 获取持久播放映射。
func (d *ShareRecordDAO) GetStrmEntry(ctx context.Context, id string) (domain.ShareStrmEntry, error) {
	var v domain.ShareStrmEntry
	var raw []byte
	err := d.db.QueryRowContext(ctx, `SELECT payload FROM t_share_strm WHERE id=$1`, id).Scan(&raw)
	if err != nil {
		return v, err
	}
	err = json.Unmarshal(raw, &v)
	return v, err
}

// LockStrmPlayback 使用事务级锁串行化同一文件转存，连接关闭时锁自动释放。
func (d *ShareRecordDAO) LockStrmPlayback(ctx context.Context, key string) (func(), error) {
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	// 115客户端的转存调用不接收context；请求中断后仍持锁到调用返回，避免下一请求重复转存。
	tx, err := conn.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); err != nil {
		_ = tx.Rollback()
		_ = conn.Close()
		return nil, err
	}
	return func() { _ = tx.Rollback(); _ = conn.Close() }, nil
}
