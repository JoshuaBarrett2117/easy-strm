package dao

import (
	"context"
	"database/sql"
	"fmt"
)

// StrmExportDAO 在专用连接上持有目录树的共享/独占锁。
type StrmExportDAO struct{ Conn *sql.Conn }

// LockStrmOutput 按根到叶顺序锁定祖先，目录本身独占；进程退出自动释放。
func LockStrmOutput(ctx context.Context, db *sql.DB, paths []string) (*StrmExportDAO, error) {
	c, e := db.Conn(ctx)
	if e != nil {
		return nil, e
	}
	d := &StrmExportDAO{c}
	for i, p := range paths {
		fn := "pg_try_advisory_lock_shared"
		if i == len(paths)-1 {
			fn = "pg_try_advisory_lock"
		}
		var ok bool
		if e = c.QueryRowContext(ctx, "SELECT "+fn+"(hashtextextended($1, 34981))", p).Scan(&ok); e != nil || !ok {
			d.Close()
			if e != nil {
				return nil, e
			}
			return nil, fmt.Errorf("输出目录或其父子目录正在执行任务")
		}
	}
	return d, nil
}

// Close 释放连接持有的全部会话锁。
func (d *StrmExportDAO) Close() {
	d.Conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock_all()")
	d.Conn.Close()
}

// ExportState 保存最近一次成功结果。
type ExportState struct{ Owner, Key, Path, Content, Mapping, Playback, Run string }

// CheckPath 查询现有归属及旧清单，缺失与失效记录仍保护尚存文件。
func (d *StrmExportDAO) CheckPath(ctx context.Context, p, owner, key string) (bool, error) {
	rows, e := d.Conn.QueryContext(ctx, `SELECT owner_key,export_key FROM t_strm_export_state WHERE output_path=$1 AND state<>'missing'
 UNION ALL SELECT owner_key,'' FROM t_strm_export_history WHERE output_path=$1`, p)
	if e != nil {
		return false, e
	}
	defer rows.Close()
	own := false
	for rows.Next() {
		var o, k string
		if e = rows.Scan(&o, &k); e != nil {
			return false, e
		}
		if o != owner || (k != "" && k != key) {
			return false, fmt.Errorf("输出路径归属冲突：%s", p)
		}
		own = true
	}
	return own, rows.Err()
}

// Save 保存新结果并保留改名前的路径归属。
func (d *StrmExportDAO) Save(ctx context.Context, s ExportState) error {
	tx, e := d.Conn.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `INSERT INTO t_strm_export_history SELECT owner_key,output_path,'路径变化' FROM t_strm_export_state WHERE owner_key=$1 AND export_key=$2 AND output_path<>$3 ON CONFLICT DO NOTHING`, s.Owner, s.Key, s.Path)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO t_strm_export_state(owner_key,export_key,output_path,content_fingerprint,mapping_fingerprint,share_strm_id,last_seen_run_id,exported_at) VALUES($1,$2,$3,$4,$5,$6,$7,now()) ON CONFLICT(owner_key,export_key) DO UPDATE SET output_path=$3,content_fingerprint=$4,mapping_fingerprint=$5,share_strm_id=$6,last_seen_run_id=$7,state='active',exported_at=CASE WHEN t_strm_export_state.content_fingerprint<>$4 OR t_strm_export_state.mapping_fingerprint<>$5 THEN now() ELSE t_strm_export_state.exported_at END`, s.Owner, s.Key, s.Path, s.Content, s.Mapping, s.Playback, s.Run)
	if e != nil {
		return e
	}
	return tx.Commit()
}

// Finish 标记完整扫描未见的导出项，不删除磁盘文件。
func (d *StrmExportDAO) Finish(ctx context.Context, owner, run string) error {
	_, e := d.Conn.ExecContext(ctx, "UPDATE t_strm_export_state SET state='stale' WHERE owner_key=$1 AND last_seen_run_id<>$2", owner, run)
	return e
}

// LegacyPaths 获取历史文件清单用于运行系统上的路径归一化。
func (d *StrmExportDAO) LegacyPaths(ctx context.Context) (map[string][]string, error) {
	rows, e := d.Conn.QueryContext(ctx, "SELECT strm_config_id,local_strm_path FROM t_strm_file WHERE COALESCE(local_strm_path,'')<>''")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var id int
		var p string
		if e = rows.Scan(&id, &p); e != nil {
			return nil, e
		}
		owner := fmt.Sprintf("cloud115:%d", id)
		if id == -1 {
			owner = "share:default"
		}
		out[p] = append(out[p], owner)
	}
	return out, rows.Err()
}

// RememberLegacy 保留所有历史归属，冲突不任意选边。
func (d *StrmExportDAO) RememberLegacy(ctx context.Context, p, owner string) error {
	_, e := d.Conn.ExecContext(ctx, "INSERT INTO t_strm_export_history SELECT $1,$2,'历史清单' WHERE NOT EXISTS(SELECT 1 FROM t_strm_export_state WHERE output_path=$2) ON CONFLICT DO NOTHING", owner, p)
	return e
}

// Reserve 在文件写入前保留归属，写后登记失败仍可重试；不提前推进成功指纹。
func (d *StrmExportDAO) Reserve(ctx context.Context, owner, p string) error {
	_, e := d.Conn.ExecContext(ctx, "INSERT INTO t_strm_export_history VALUES($1,$2,'写入预留') ON CONFLICT DO NOTHING", owner, p)
	return e
}

// KnownPaths 包含独立导出状态和历史路径，防止漏标清空后的记录。
func (d *StrmExportDAO) KnownPaths(ctx context.Context) ([]string, error) {
	rows, e := d.Conn.QueryContext(ctx, "SELECT output_path FROM t_strm_export_state UNION SELECT output_path FROM t_strm_export_history")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var p string
		if e = rows.Scan(&p); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ClearStarted 记录清理授权，恢复时读取相同任务的阶段。
func (d *StrmExportDAO) ClearStarted(ctx context.Context, id, p string) (bool, error) {
	_, e := d.Conn.ExecContext(ctx, "INSERT INTO t_strm_clear_run(task_id,output_path) VALUES($1,$2) ON CONFLICT DO NOTHING", id, p)
	if e != nil {
		return false, e
	}
	var done bool
	e = d.Conn.QueryRowContext(ctx, "SELECT completed FROM t_strm_clear_run WHERE task_id=$1 AND output_path=$2", id, p).Scan(&done)
	return done, e
}

// MarkMissing 清空成功后只释放目录内的归属记录。
func (d *StrmExportDAO) MarkMissing(ctx context.Context, paths []string, id string) error {
	tx, e := d.Conn.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, p := range paths {
		if _, e = tx.ExecContext(ctx, `INSERT INTO t_strm_export_state(owner_key,export_key,output_path,state) SELECT owner_key,'legacy:'||output_path,output_path,'missing' FROM t_strm_export_history WHERE output_path=$1 ON CONFLICT DO NOTHING`, p); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE t_strm_export_state SET state='missing' WHERE output_path=$1", p); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "DELETE FROM t_strm_export_history WHERE output_path=$1", p); e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, "UPDATE t_strm_clear_run SET completed=TRUE WHERE task_id=$1", id)
	if e != nil {
		return e
	}
	return tx.Commit()
}
