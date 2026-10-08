package dao

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"
)

// ErrStrmOutputBusy 表示输出目录与正在执行的任务重叠，可等待后重试。
var ErrStrmOutputBusy = errors.New("输出目录或其父子目录正在执行任务")

// StrmExportDAO 在专用连接上持有目录树的共享/独占锁。
type StrmExportDAO struct{ Conn *sql.Conn }

// LockStrmOutput 按根到叶顺序锁定祖先，目录本身独占；进程退出自动释放。
func LockStrmOutput(ctx context.Context, db *sql.DB, paths []string) (*StrmExportDAO, error) {
	return lockStrmOutput(ctx, db, paths, false)
}

// LockStrmOutputShared 普通导出共享目录树，具体写入路径另行独占。
func LockStrmOutputShared(ctx context.Context, db *sql.DB, paths []string) (*StrmExportDAO, error) {
	return lockStrmOutput(ctx, db, paths, true)
}

// TryShareOwner 同一分享 owner 的全量与增量任务互斥，不受输出根目录变化影响。
func (d *StrmExportDAO) TryShareOwner(ctx context.Context) error {
	var locked bool
	err := d.Conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended('share:default',34982))`).Scan(&locked)
	if err != nil {
		return err
	}
	if !locked {
		return ErrStrmOutputBusy
	}
	return nil
}

func lockStrmOutput(ctx context.Context, db *sql.DB, paths []string, shared bool) (*StrmExportDAO, error) {
	c, e := db.Conn(ctx)
	if e != nil {
		return nil, e
	}
	d := &StrmExportDAO{c}
	for i, p := range paths {
		fn := "pg_try_advisory_lock_shared"
		if i == len(paths)-1 && !shared {
			fn = "pg_try_advisory_lock"
		}
		var ok bool
		if e = c.QueryRowContext(ctx, "SELECT "+fn+"(hashtextextended($1, 34981))", p).Scan(&ok); e != nil || !ok {
			d.Close()
			if e != nil {
				return nil, e
			}
			return nil, ErrStrmOutputBusy
		}
	}
	return d, nil
}

// LockPath 在已有目录共享锁内独占一个规范路径，取消时退出等待。
func (d *StrmExportDAO) LockPath(ctx context.Context, p string) (func(), error) {
	for {
		var ok bool
		err := d.Conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1, 34981))", p).Scan(&ok)
		if err != nil {
			return nil, err
		}
		if ok {
			return func() {
				d.Conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(hashtextextended($1, 34981))", p)
			}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// ExclusiveDirectory 升级整目录清空前先释放共享占用，避免多个清空相互等待。
func (d *StrmExportDAO) ExclusiveDirectory(ctx context.Context, paths []string) error {
	if _, err := d.Conn.ExecContext(ctx, "SELECT pg_advisory_unlock_all()"); err != nil {
		return err
	}
	for {
		busy := false
		for i, p := range paths {
			fn := "pg_try_advisory_lock_shared"
			if i == len(paths)-1 {
				fn = "pg_try_advisory_lock"
			}
			var ok bool
			if err := d.Conn.QueryRowContext(ctx, "SELECT "+fn+"(hashtextextended($1, 34981))", p).Scan(&ok); err != nil {
				return err
			}
			if !ok {
				busy = true
				break
			}
		}
		if !busy {
			return nil
		}
		if _, err := d.Conn.ExecContext(ctx, "SELECT pg_advisory_unlock_all()"); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// SharedDirectory 清空结束后原子降级叶目录，普通写入继续只独占各自路径。
func (d *StrmExportDAO) SharedDirectory(ctx context.Context, root string) error {
	if _, err := d.Conn.ExecContext(ctx, "SELECT pg_advisory_lock_shared(hashtextextended($1, 34981))", root); err != nil {
		return err
	}
	_, err := d.Conn.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtextextended($1, 34981))", root)
	return err
}

// ExportSnapshot 保留本次扫描前的清单版本；收尾不能修改其他任务刷新过的行。
type ExportSnapshot map[string]string

// Snapshot 获取指定归属的清单快照。
func (d *StrmExportDAO) Snapshot(ctx context.Context, owner string) (ExportSnapshot, error) {
	rows, err := d.Conn.QueryContext(ctx, "SELECT export_key,last_seen_run_id FROM t_strm_export_state WHERE owner_key=$1", owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := ExportSnapshot{}
	for rows.Next() {
		var k, r string
		if err = rows.Scan(&k, &r); err != nil {
			return nil, err
		}
		out[k] = r
	}
	return out, rows.Err()
}

// FinishSnapshot 仅标记本次未见且运行版本仍等于快照的行。
func (d *StrmExportDAO) FinishSnapshot(ctx context.Context, owner string, snapshot ExportSnapshot, seen map[string]bool) error {
	for k, run := range snapshot {
		if seen[k] {
			continue
		}
		if _, err := d.Conn.ExecContext(ctx, "UPDATE t_strm_export_state SET state='stale' WHERE owner_key=$1 AND export_key=$2 AND last_seen_run_id=$3", owner, k, run); err != nil {
			return err
		}
	}
	return nil
}

// Close 释放连接持有的全部会话锁。
func (d *StrmExportDAO) Close() {
	if _, err := d.Conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock_all()"); err != nil {
		// 释放失败时丢弃物理连接，不能让带会话锁的连接重新进入池。
		_ = d.Conn.Raw(func(interface{}) error { return driver.ErrBadConn })
	}
	d.Conn.Close()
}

// ExportState 保存最近一次成功结果。
type ExportState struct{ Owner, Key, Path, Content, Mapping, Playback, Run string }

// CheckPath 查询现有归属及旧清单，缺失与失效记录仍保护尚存文件。
func (d *StrmExportDAO) CheckPath(ctx context.Context, p, owner, key string) (bool, error) {
	rows, e := d.Conn.QueryContext(ctx, strmExportCheckPathSQL, p)
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

const strmExportCheckPathSQL = `SELECT owner_key,export_key FROM t_strm_export_state WHERE output_path=$1 AND state<>'missing'
 UNION ALL SELECT owner_key,'' FROM t_strm_export_history WHERE output_path=$1
 UNION ALL SELECT CASE WHEN strm_config_id=-1 THEN 'share:default' ELSE 'cloud115:'||strm_config_id::text END,''
 FROM t_strm_file WHERE local_strm_path=$1
 AND NOT EXISTS(SELECT 1 FROM t_strm_export_state WHERE output_path=$1)`

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
