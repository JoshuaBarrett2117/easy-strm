package dao

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"github.com/lib/pq"
)

// ShareOperationDAO 保存清理队列，执行事务与完成日志在同一数据库提交。
type ShareOperationDAO struct{ db *sql.DB }

// NewShareOperationDAO 创建分享操作队列访问器。
func NewShareOperationDAO(db *sql.DB) *ShareOperationDAO { return &ShareOperationDAO{db: db} }

const shareOperationColumns = "sequence,task_id,operation,request_key,share_ids,file_id,status,phase,result,error,created_at"

func scanShareOperation(row interface{ Scan(...interface{}) error }) (domain.ShareOperation, error) {
	var v domain.ShareOperation
	var ids pq.Int64Array
	var raw []byte
	err := row.Scan(&v.Sequence, &v.TaskID, &v.Kind, &v.Key, &ids, &v.FileID, &v.Status, &v.Phase, &raw, &v.Error, &v.CreatedAt)
	if err != nil {
		return v, err
	}
	v.ShareIDs = []int{}
	for _, id := range ids {
		v.ShareIDs = append(v.ShareIDs, int(id))
	}
	err = json.Unmarshal(raw, &v.Result)
	return v, err
}

// Enqueue 幂等登记相同的活动请求，返回原任务或新任务的持久化参数。
func (d *ShareOperationDAO) Enqueue(ctx context.Context, v domain.ShareOperation) (domain.ShareOperation, error) {
	if v.ShareIDs == nil {
		v.ShareIDs = []int{}
	}
	return scanShareOperation(d.db.QueryRowContext(ctx, `INSERT INTO t_share_operation_queue(task_id,operation,request_key,share_ids,file_id)
	 VALUES($1,$2,$3,$4,$5) ON CONFLICT(request_key) WHERE status IN ('pending','running')
	 DO UPDATE SET request_key=EXCLUDED.request_key RETURNING `+shareOperationColumns, v.TaskID, v.Kind, v.Key, pq.Array(v.ShareIDs), v.FileID))
}

// Recoverable 返回活动任务及尚未同步到任务中心的终态，按提交顺序恢复。
func (d *ShareOperationDAO) Recoverable(ctx context.Context) ([]domain.ShareOperation, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT "+shareOperationColumns+" FROM t_share_operation_queue WHERE status IN ('pending','running') OR NOT published ORDER BY sequence")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ShareOperation{}
	for rows.Next() {
		v, e := scanShareOperation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Get 获取持久化状态，供取消和任务恢复判定。
func (d *ShareOperationDAO) Get(ctx context.Context, id string) (domain.ShareOperation, error) {
	return scanShareOperation(d.db.QueryRowContext(ctx, "SELECT "+shareOperationColumns+" FROM t_share_operation_queue WHERE task_id=$1", id))
}

// Start 原子进入不可取消的执行阶段，重启恢复可继续原running任务。
func (d *ShareOperationDAO) Start(ctx context.Context, id string) (bool, error) {
	r, e := d.db.ExecContext(ctx, "UPDATE t_share_operation_queue SET status='running',phase='执行清理',published=FALSE,updated_at=now() WHERE task_id=$1 AND status IN ('pending','running')", id)
	if e != nil {
		return false, e
	}
	n, e := r.RowsAffected()
	return n == 1, e
}

// Cancel 只允许取消等待任务，和Start的状态更新互斥。
func (d *ShareOperationDAO) Cancel(ctx context.Context, id string) error {
	r, e := d.db.ExecContext(ctx, "UPDATE t_share_operation_queue SET status='cancelled',phase='已取消',published=FALSE,updated_at=now() WHERE task_id=$1 AND status='pending'", id)
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		return fmt.Errorf("操作已开始执行或已经结束，不能取消")
	}
	return nil
}

// Finish 保存失败或无需修改数据的完成结果；已提交的完成日志不会被覆盖。
func (d *ShareOperationDAO) Finish(ctx context.Context, id, status, message string, result map[string]interface{}) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = d.db.ExecContext(ctx, "UPDATE t_share_operation_queue SET status=$2,phase=$2,error=$3,result=$4,published=FALSE,updated_at=now() WHERE task_id=$1 AND status IN ('pending','running')", id, status, message, string(raw))
	return err
}

// Published 标记终态已经同步到任务中心，重启仍会恢复未同步的结果。
func (d *ShareOperationDAO) Published(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, "UPDATE t_share_operation_queue SET published=TRUE WHERE task_id=$1 AND status NOT IN ('pending','running')", id)
	return err
}

type shareOperationTaskContext struct{}

// WithShareOperationTask 让清理DAO将完成日志与数据变更原子提交。
func WithShareOperationTask(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, shareOperationTaskContext{}, id)
}
func completeShareOperationTx(ctx context.Context, tx *sql.Tx, count int64) error {
	id, _ := ctx.Value(shareOperationTaskContext{}).(string)
	if id == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `UPDATE t_share_operation_queue SET status='completed',phase='清理完成',result=jsonb_build_object('deleted',$2::bigint),published=FALSE,updated_at=now() WHERE task_id=$1 AND status='running'`, id, count)
	return err
}

// ShareIDs 固定全量清空的提交范围，不包含之后新增的分享。
func (d *ShareRecordDAO) ShareIDs(ctx context.Context) ([]int, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT id FROM t_share_record ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// FileShareID 获取文件的父分享，用于资源占用和文件删除的边界校验。
func (d *ShareRecordDAO) FileShareID(ctx context.Context, id int) (int, error) {
	var shareID int
	err := d.db.QueryRowContext(ctx, "SELECT share_id FROM t_share_media_file WHERE id=$1", id).Scan(&shareID)
	return shareID, err
}

// FileIdentity 获取单文件删除的稳定网盘身份，供旧批次跳过已删除目标。
func (d *ShareRecordDAO) FileIdentity(ctx context.Context, id int) (domain.ShareMedia, error) {
	var v domain.ShareMedia
	v.ID = id
	err := d.db.QueryRowContext(ctx, "SELECT share_id,file_name,file_id FROM t_share_media_file WHERE id=$1", id).Scan(&v.ShareID, &v.FileName, &v.RemoteFileID)
	return v, err
}

// FileCandidate 在文件资源取得后读取当前版本及有效记录，供前台识别和核对校验。
func (d *ShareRecordDAO) FileCandidate(ctx context.Context, id int) (domain.ShareMedia, error) {
	v := domain.ShareMedia{ID: id}
	var result, episodes []byte
	err := d.db.QueryRowContext(ctx, `SELECT f.share_id,f.version,f.file_name,f.metadata_source,f.status,f.available,s.media_type,f.result,
	 COALESCE((SELECT json_agg(json_build_object('season_number',e.season_number,'episode_number',e.episode_number)) FROM t_share_media_file_episode e WHERE e.file_id=f.id),'[]'::json)
	 FROM t_share_media_file f JOIN t_share_record s ON s.id=f.share_id WHERE f.id=$1 AND NOT s.share_cancelled`, id).Scan(&v.ShareID, &v.Version, &v.FileName, &v.MetadataSource, &v.Status, &v.Available, &v.MediaType, &result, &episodes)
	if err != nil {
		return v, err
	}
	if len(result) > 0 {
		if err = json.Unmarshal(result, &v.Result); err != nil {
			return v, err
		}
	}
	err = json.Unmarshal(episodes, &v.Episodes)
	return v, err
}

// SyncResourceIDs 列出完整扫描会更新可用状态的文件，仅在提交阶段获取文件占用。
func (d *ShareRecordDAO) SyncResourceIDs(ctx context.Context, id int) ([]int, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT id FROM t_share_media_file WHERE share_id=$1 ORDER BY id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// HasOtherShareCode 判断共享历史映射是否仍有其他分享记录引用。
func (d *ShareRecordDAO) HasOtherShareCode(ctx context.Context, id int, code string) (bool, error) {
	var found bool
	err := d.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM t_share_record WHERE id<>$1 AND substring(url FROM '/s/([A-Za-z0-9_]+)')=$2)`, id, code).Scan(&found)
	return found, err
}
