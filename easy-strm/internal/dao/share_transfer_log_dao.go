package dao

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

const shareTransferLogBatchSize = 200 // 批量插入每批次条数

// ShareTransferLogDAO 分享转存日志数据访问层
type ShareTransferLogDAO struct {
	db *sql.DB
}

// NewShareTransferLogDAO 创建分享转存日志DAO实例
// 参数:
//   - db: PostgreSQL数据库连接
//
// 返回:
//   - *ShareTransferLogDAO: DAO实例
func NewShareTransferLogDAO(db *sql.DB) *ShareTransferLogDAO {
	return &ShareTransferLogDAO{db: db}
}

// BatchInsert 批量插入分享转存日志，每批次最多200条
// 参数:
//   - ctx: 上下文
//   - logs: 日志记录列表
//
// 返回:
//   - error: 插入失败时返回错误
func (d *ShareTransferLogDAO) BatchInsert(ctx context.Context, logs []domain.ShareTransferLog) error {
	if len(logs) == 0 {
		return nil
	}

	for i := 0; i < len(logs); i += shareTransferLogBatchSize {
		end := i + shareTransferLogBatchSize
		if end > len(logs) {
			end = len(logs)
		}
		batch := logs[i:end]

		if err := d.insertBatch(ctx, batch); err != nil {
			return fmt.Errorf("ShareTransferLogDAO[BatchInsert] 第%d批插入失败: %v", i/shareTransferLogBatchSize+1, err)
		}
	}

	logger.Infof("ShareTransferLogDAO[BatchInsert] 批量插入完成，共%d条", len(logs))
	return nil
}

// insertBatch 执行单批次插入
func (d *ShareTransferLogDAO) insertBatch(ctx context.Context, logs []domain.ShareTransferLog) error {
	if len(logs) == 0 {
		return nil
	}

	// 构建 VALUES 占位符
	valuePlaceholders := make([]string, 0, len(logs))
	args := make([]interface{}, 0, len(logs)*13)

	for idx, log := range logs {
		base := idx * 13
		valuePlaceholders = append(valuePlaceholders, fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10, base+11, base+12, base+13,
		))
		args = append(args,
			log.TaskId, log.ShareCode, log.ShareFolderName, log.FileName,
			log.FilePickCode, log.FileSize, log.FileSha1, log.Cloud115Id,
			log.TargetDirectory, log.Status, log.ErrorMessage, log.IsSecondTransfer,
			"", // create_time 使用数据库默认值
		)
	}

	query := fmt.Sprintf(
		`INSERT INTO t_share_transfer_log
		(task_id, share_code, share_folder_name, file_name, file_pick_code,
		 file_size, file_sha1, cloud115_id, target_directory, status, error_message,
		 is_second_transfer, create_time)
		VALUES %s`,
		strings.Join(valuePlaceholders, ","),
	)

	_, err := d.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("ShareTransferLogDAO[insertBatch] SQL执行失败: %v", err)
	}

	return nil
}

// GetByTaskId 按任务ID查询所有日志
// 参数:
//   - ctx: 上下文
//   - taskId: 任务ID
//
// 返回:
//   - []domain.ShareTransferLog: 日志列表
//   - error: 查询失败时返回错误
func (d *ShareTransferLogDAO) GetByTaskId(ctx context.Context, taskId string) ([]domain.ShareTransferLog, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, task_id, share_code, share_folder_name, file_name,
		 file_pick_code, file_size, file_sha1, cloud115_id,
		 target_directory, status, error_message, is_second_transfer,
		 COALESCE(create_time::text, ''), COALESCE(update_time::text, '')
		 FROM t_share_transfer_log
		 WHERE task_id = $1
		 ORDER BY id ASC`,
		taskId,
	)
	if err != nil {
		return nil, fmt.Errorf("ShareTransferLogDAO[GetByTaskId] 查询失败: %v", err)
	}
	defer rows.Close()

	var logs []domain.ShareTransferLog
	for rows.Next() {
		var l domain.ShareTransferLog
		if err := rows.Scan(
			&l.ID, &l.TaskId, &l.ShareCode, &l.ShareFolderName, &l.FileName,
			&l.FilePickCode, &l.FileSize, &l.FileSha1, &l.Cloud115Id,
			&l.TargetDirectory, &l.Status, &l.ErrorMessage, &l.IsSecondTransfer,
			&l.CreateTime, &l.UpdateTime,
		); err != nil {
			return nil, fmt.Errorf("ShareTransferLogDAO[GetByTaskId] 扫描行失败: %v", err)
		}
		logs = append(logs, l)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ShareTransferLogDAO[GetByTaskId] 遍历结果集失败: %v", err)
	}

	return logs, nil
}

// UpdateStatus 更新单条日志的状态
// 参数:
//   - ctx: 上下文
//   - taskId: 任务ID
//   - pickCode: 文件pickcode（唯一标识）
//   - status: 新状态
//   - errMsg: 错误信息（为空时保持不变）
//
// 返回:
//   - error: 更新失败时返回错误
func (d *ShareTransferLogDAO) UpdateStatus(ctx context.Context, taskId, pickCode, status, errMsg string) error {
	query := `UPDATE t_share_transfer_log
		SET status = $1, error_message = $2, update_time = CURRENT_TIMESTAMP
		WHERE task_id = $3 AND file_pick_code = $4`

	_, err := d.db.ExecContext(ctx, query, status, errMsg, taskId, pickCode)
	if err != nil {
		return fmt.Errorf("ShareTransferLogDAO[UpdateStatus] 更新失败: %v", err)
	}

	return nil
}
