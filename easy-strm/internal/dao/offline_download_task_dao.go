package dao

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// OfflineDownloadTaskDAO 115云下载（离线下载）记录数据访问层
type OfflineDownloadTaskDAO struct {
	db *sql.DB
}

// NewOfflineDownloadTaskDAO 创建云下载记录DAO实例
// 参数:
//   - db: PostgreSQL数据库连接
//
// 返回:
//   - *OfflineDownloadTaskDAO: DAO实例
func NewOfflineDownloadTaskDAO(db *sql.DB) *OfflineDownloadTaskDAO {
	return &OfflineDownloadTaskDAO{db: db}
}

// offlineDownloadTaskColumns 查询字段列表（显式列出，避免 SELECT *）
const offlineDownloadTaskColumns = `id, task_id, cloud115_id, url, info_hash, name, size,
	status, percent, error_message, save_dir_id,
	COALESCE(create_time::text, ''), COALESCE(update_time::text, '')`

// BatchInsert 批量插入云下载记录
func (d *OfflineDownloadTaskDAO) BatchInsert(ctx context.Context, tasks []domain.OfflineDownloadTask) error {
	if len(tasks) == 0 {
		return nil
	}

	valuePlaceholders := make([]string, 0, len(tasks))
	args := make([]interface{}, 0, len(tasks)*10)
	for idx, task := range tasks {
		base := idx * 10
		valuePlaceholders = append(valuePlaceholders, fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10,
		))
		args = append(args,
			task.TaskId, task.Cloud115ID, task.Url, task.InfoHash, task.Name,
			task.Size, task.Status, task.Percent, task.ErrorMessage, task.SaveDirID,
		)
	}

	query := fmt.Sprintf(
		`INSERT INTO t_offline_download_task
		(task_id, cloud115_id, url, info_hash, name, size, status, percent, error_message, save_dir_id)
		VALUES %s`,
		strings.Join(valuePlaceholders, ","),
	)

	if _, err := d.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("OfflineDownloadTaskDAO[BatchInsert] SQL执行失败: %v", err)
	}

	logger.Infof("OfflineDownloadTaskDAO[BatchInsert] 批量插入完成，共%d条", len(tasks))
	return nil
}

// List 分页查询云下载记录
// cloud115ID<=0 表示不按账号过滤；status 为空表示不按状态过滤。
// 返回当前页记录与满足条件的总数。
func (d *OfflineDownloadTaskDAO) List(ctx context.Context, cloud115ID int, status string, page, pageSize int) ([]domain.OfflineDownloadTask, int, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	if cloud115ID > 0 {
		args = append(args, cloud115ID)
		where = append(where, fmt.Sprintf("cloud115_id = $%d", len(args)))
	}
	if status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	whereClause := strings.Join(where, " AND ")

	var total int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM t_offline_download_task WHERE %s`, whereClause)
	if err := d.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("OfflineDownloadTaskDAO[List] 统计总数失败: %v", err)
	}

	args = append(args, pageSize, (page-1)*pageSize)
	listQuery := fmt.Sprintf(
		`SELECT %s FROM t_offline_download_task WHERE %s ORDER BY id DESC LIMIT $%d OFFSET $%d`,
		offlineDownloadTaskColumns, whereClause, len(args)-1, len(args),
	)

	rows, err := d.db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("OfflineDownloadTaskDAO[List] 查询失败: %v", err)
	}
	defer rows.Close()

	tasks, err := scanOfflineDownloadTasks(rows)
	if err != nil {
		return nil, 0, fmt.Errorf("OfflineDownloadTaskDAO[List] %v", err)
	}
	return tasks, total, nil
}

// GetByID 按主键查询单条记录；记录不存在时返回 nil, nil
func (d *OfflineDownloadTaskDAO) GetByID(ctx context.Context, id int64) (*domain.OfflineDownloadTask, error) {
	query := fmt.Sprintf(`SELECT %s FROM t_offline_download_task WHERE id = $1`, offlineDownloadTaskColumns)
	var task domain.OfflineDownloadTask
	err := d.db.QueryRowContext(ctx, query, id).Scan(
		&task.ID, &task.TaskId, &task.Cloud115ID, &task.Url, &task.InfoHash, &task.Name,
		&task.Size, &task.Status, &task.Percent, &task.ErrorMessage, &task.SaveDirID,
		&task.CreateTime, &task.UpdateTime,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("OfflineDownloadTaskDAO[GetByID] 查询失败: %v", err)
	}
	return &task, nil
}

// GetByTaskID 查询指定提交批次任务的全部记录
func (d *OfflineDownloadTaskDAO) GetByTaskID(ctx context.Context, taskID string) ([]domain.OfflineDownloadTask, error) {
	query := fmt.Sprintf(`SELECT %s FROM t_offline_download_task WHERE task_id = $1 ORDER BY id ASC`, offlineDownloadTaskColumns)
	rows, err := d.db.QueryContext(ctx, query, taskID)
	if err != nil {
		return nil, fmt.Errorf("OfflineDownloadTaskDAO[GetByTaskID] 查询失败: %v", err)
	}
	defer rows.Close()

	tasks, err := scanOfflineDownloadTasks(rows)
	if err != nil {
		return nil, fmt.Errorf("OfflineDownloadTaskDAO[GetByTaskID] %v", err)
	}
	return tasks, nil
}

// GetActiveByAccount 查询指定账号下未达终态（pending/downloading）的记录
func (d *OfflineDownloadTaskDAO) GetActiveByAccount(ctx context.Context, cloud115ID int) ([]domain.OfflineDownloadTask, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM t_offline_download_task WHERE cloud115_id = $1 AND status IN ($2, $3) ORDER BY id ASC`,
		offlineDownloadTaskColumns,
	)
	rows, err := d.db.QueryContext(ctx, query, cloud115ID, domain.OfflineStatusPending, domain.OfflineStatusDownloading)
	if err != nil {
		return nil, fmt.Errorf("OfflineDownloadTaskDAO[GetActiveByAccount] 查询失败: %v", err)
	}
	defer rows.Close()

	tasks, err := scanOfflineDownloadTasks(rows)
	if err != nil {
		return nil, fmt.Errorf("OfflineDownloadTaskDAO[GetActiveByAccount] %v", err)
	}
	return tasks, nil
}

// UpdateByHash 按账号+info_hash更新任务名称、大小、状态与进度
func (d *OfflineDownloadTaskDAO) UpdateByHash(ctx context.Context, cloud115ID int, infoHash, name string, size int64, status string, percent float64, errMsg string) error {
	query := `UPDATE t_offline_download_task
		SET name = $1, size = $2, status = $3, percent = $4, error_message = $5, update_time = CURRENT_TIMESTAMP
		WHERE cloud115_id = $6 AND info_hash = $7`
	if _, err := d.db.ExecContext(ctx, query, name, size, status, percent, errMsg, cloud115ID, infoHash); err != nil {
		return fmt.Errorf("OfflineDownloadTaskDAO[UpdateByHash] 更新失败: %v", err)
	}
	return nil
}

// UpdateStatus 更新单条记录状态与错误信息
func (d *OfflineDownloadTaskDAO) UpdateStatus(ctx context.Context, id int64, status, errMsg string) error {
	query := `UPDATE t_offline_download_task
		SET status = $1, error_message = $2, update_time = CURRENT_TIMESTAMP
		WHERE id = $3`
	if _, err := d.db.ExecContext(ctx, query, status, errMsg, id); err != nil {
		return fmt.Errorf("OfflineDownloadTaskDAO[UpdateStatus] 更新失败: %v", err)
	}
	return nil
}

// Delete 删除单条记录
func (d *OfflineDownloadTaskDAO) Delete(ctx context.Context, id int64) error {
	if _, err := d.db.ExecContext(ctx, `DELETE FROM t_offline_download_task WHERE id = $1`, id); err != nil {
		return fmt.Errorf("OfflineDownloadTaskDAO[Delete] 删除失败: %v", err)
	}
	return nil
}

// scanOfflineDownloadTasks 扫描结果集为记录列表
func scanOfflineDownloadTasks(rows *sql.Rows) ([]domain.OfflineDownloadTask, error) {
	tasks := make([]domain.OfflineDownloadTask, 0)
	for rows.Next() {
		var task domain.OfflineDownloadTask
		if err := rows.Scan(
			&task.ID, &task.TaskId, &task.Cloud115ID, &task.Url, &task.InfoHash, &task.Name,
			&task.Size, &task.Status, &task.Percent, &task.ErrorMessage, &task.SaveDirID,
			&task.CreateTime, &task.UpdateTime,
		); err != nil {
			return nil, fmt.Errorf("扫描行失败: %v", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历结果集失败: %v", err)
	}
	return tasks, nil
}
