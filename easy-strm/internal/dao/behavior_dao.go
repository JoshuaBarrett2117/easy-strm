package dao

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"time"
)

// BehaviorSession 在账号独占锁内访问检查点与索引，便于单元测试替换。
type BehaviorSession interface {
	Close() error
	Checkpoint(context.Context) (int64, error)
	Advance(context.Context, int64) error
	Configuration(context.Context) (string, int, []domain.BehaviorRoot, error)
	Get(context.Context, string) (*domain.BehaviorIndex, error)
	Tree(context.Context, string) ([]domain.BehaviorIndex, error)
	Pending(context.Context) ([]domain.BehaviorIndex, error)
	Put(context.Context, domain.BehaviorIndex) error
	Delete(context.Context, string) error
	LockPaths(context.Context, []string) (func(), error)
}

// BehaviorRepository 为不同cron定义取得同一账号的跨进程锁。
type BehaviorRepository interface {
	Lock(context.Context, int) (BehaviorSession, error)
}

// BehaviorDAO 通过注入数据库访问行为同步数据，不执行迁移。
type BehaviorDAO struct{ db *sql.DB }

// NewBehaviorDAO 创建行为同步仓储，构造时无数据库操作。
func NewBehaviorDAO(database *sql.DB) *BehaviorDAO { return &BehaviorDAO{db: database} }

type behaviorSession struct {
	conn     *sql.Conn
	configID int
	cloudID  int
	db       *sql.DB
}

// Lock 使用独立连接持有账号锁；等待可取消，连接归还前必定释放锁。
// 入参为 STRM 配置 ID，内部解析所属 115 账号并按账号加锁（同一账号下多配置串行），
// 检查点维度则为 (strm_config_id, cloud115_id)。账号锁使用专用 classid 34985。
func (store *BehaviorDAO) Lock(ctx context.Context, strmConfigID int) (BehaviorSession, error) {
	if strmConfigID <= 0 {
		return nil, fmt.Errorf("STRM配置ID无效")
	}
	var ready bool
	if err := store.db.QueryRowContext(ctx, `SELECT to_regclass('t_behavior_checkpoint') IS NOT NULL AND to_regclass('t_strm_file_index') IS NOT NULL AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='t_behavior_checkpoint' AND column_name='strm_config_id')`).Scan(&ready); err != nil {
		return nil, err
	}
	if !ready {
		return nil, fmt.Errorf("行为同步表未升级到 v50（checkpoint 缺少 strm_config_id 维度），请 DBA 手工应用 migrate_v50_behavior_sync_config.sql")
	}
	var cloudID int
	if err := store.db.QueryRowContext(ctx, `SELECT cloud115_id FROM t_strm_config WHERE id=$1`, strmConfigID).Scan(&cloudID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("STRM配置%d不存在", strmConfigID)
		}
		return nil, err
	}
	conn, err := store.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	session := &behaviorSession{conn: conn, configID: strmConfigID, cloudID: cloudID, db: store.db}
	for {
		var locked bool
		err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(34985,$1)`, cloudID).Scan(&locked)
		if err != nil {
			conn.Close()
			return nil, err
		}
		if locked {
			return session, nil
		}
		select {
		case <-ctx.Done():
			conn.Close()
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (store *behaviorSession) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := store.conn.ExecContext(ctx, `SELECT pg_advisory_unlock(34985,$1)`, store.cloudID)
	if err != nil {
		_ = store.conn.Raw(func(any) error { return driver.ErrBadConn })
	}
	return errors.Join(err, store.conn.Close())
}

func (store *behaviorSession) Checkpoint(ctx context.Context) (int64, error) {
	_, err := store.conn.ExecContext(ctx, `INSERT INTO t_behavior_checkpoint(strm_config_id,cloud115_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, store.configID, store.cloudID)
	if err != nil {
		return 0, err
	}
	var cursor int64
	err = store.conn.QueryRowContext(ctx, `SELECT last_event_id FROM t_behavior_checkpoint WHERE strm_config_id=$1 AND cloud115_id=$2`, store.configID, store.cloudID).Scan(&cursor)
	return cursor, err
}

func (store *behaviorSession) Advance(ctx context.Context, cursor int64) error {
	_, err := store.conn.ExecContext(ctx, `UPDATE t_behavior_checkpoint SET last_event_id=$3,last_run_at=CURRENT_TIMESTAMP,update_time=CURRENT_TIMESTAMP WHERE strm_config_id=$1 AND cloud115_id=$2`, store.configID, store.cloudID, cursor)
	return err
}

func (store *behaviorSession) Configuration(ctx context.Context) (string, int, []domain.BehaviorRoot, error) {
	var cookie string
	if err := store.conn.QueryRowContext(ctx, `SELECT COALESCE(cookie,'') FROM t_cloud_115 WHERE id=$1`, store.cloudID).Scan(&cookie); err != nil {
		return "", 0, nil, err
	}
	var root domain.BehaviorRoot
	if err := store.conn.QueryRowContext(ctx, `SELECT id,net_disk_path,local_path FROM t_strm_config WHERE id=$1`, store.configID).Scan(&root.ID, &root.CloudPath, &root.LocalPath); err != nil {
		return "", 0, nil, err
	}
	return cookie, store.cloudID, []domain.BehaviorRoot{root}, nil
}

func (store *behaviorSession) LockPaths(ctx context.Context, paths []string) (func(), error) {
	if store.db.Stats().MaxOpenConnections == 1 {
		return nil, fmt.Errorf("行为同步至少需要两个数据库连接")
	}
	return lockStrmPaths(ctx, store.db, paths)
}

const behaviorColumns = `cloud115_id,file_id,parent_id,strm_path,is_dir,size,sha1,owned,content_hash,pending_path,pending_content`

func scanBehavior(scanner interface{ Scan(...any) error }) (domain.BehaviorIndex, error) {
	var item domain.BehaviorIndex
	err := scanner.Scan(&item.CloudID, &item.FileID, &item.ParentID, &item.Path, &item.IsDir, &item.Size, &item.SHA1, &item.Owned, &item.ContentHash, &item.PendingPath, &item.PendingContent)
	return item, err
}

func (store *behaviorSession) Get(ctx context.Context, id string) (*domain.BehaviorIndex, error) {
	item, err := scanBehavior(store.conn.QueryRowContext(ctx, `SELECT `+behaviorColumns+` FROM t_strm_file_index WHERE cloud115_id=$1 AND file_id=$2`, store.cloudID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &item, err
}

func (store *behaviorSession) query(ctx context.Context, query string, args ...any) ([]domain.BehaviorIndex, error) {
	rows, err := store.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.BehaviorIndex
	for rows.Next() {
		item, err := scanBehavior(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if len(items) > 20000 {
		return nil, fmt.Errorf("索引子树超过20000条，保留检查点")
	}
	return items, rows.Err()
}

func (store *behaviorSession) Tree(ctx context.Context, id string) ([]domain.BehaviorIndex, error) {
	return store.query(ctx, `WITH RECURSIVE subtree AS (SELECT `+behaviorColumns+` FROM t_strm_file_index WHERE cloud115_id=$1 AND file_id=$2 UNION SELECT child.cloud115_id,child.file_id,child.parent_id,child.strm_path,child.is_dir,child.size,child.sha1,child.owned,child.content_hash,child.pending_path,child.pending_content FROM t_strm_file_index child JOIN subtree parent ON child.cloud115_id=parent.cloud115_id AND child.parent_id=parent.file_id) SELECT `+behaviorColumns+` FROM subtree LIMIT 20001`, store.cloudID, id)
}

func (store *behaviorSession) Pending(ctx context.Context) ([]domain.BehaviorIndex, error) {
	return store.query(ctx, `SELECT `+behaviorColumns+` FROM t_strm_file_index WHERE cloud115_id=$1 AND pending_path<>'' LIMIT 20001`, store.cloudID)
}

func (store *behaviorSession) Put(ctx context.Context, item domain.BehaviorIndex) error {
	_, err := store.conn.ExecContext(ctx, `INSERT INTO t_strm_file_index(`+behaviorColumns+`) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(cloud115_id,file_id) DO UPDATE SET parent_id=EXCLUDED.parent_id,strm_path=EXCLUDED.strm_path,is_dir=EXCLUDED.is_dir,size=EXCLUDED.size,sha1=EXCLUDED.sha1,owned=EXCLUDED.owned,content_hash=EXCLUDED.content_hash,pending_path=EXCLUDED.pending_path,pending_content=EXCLUDED.pending_content,updated_at=CURRENT_TIMESTAMP`, store.cloudID, item.FileID, item.ParentID, item.Path, item.IsDir, item.Size, item.SHA1, item.Owned, item.ContentHash, item.PendingPath, item.PendingContent)
	return err
}

func (store *behaviorSession) Delete(ctx context.Context, id string) error {
	_, err := store.conn.ExecContext(ctx, `DELETE FROM t_strm_file_index WHERE cloud115_id=$1 AND file_id=$2`, store.cloudID, id)
	return err
}
