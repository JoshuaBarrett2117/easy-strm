package dao

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"encoding/json"
	"fmt"
)

// ReadWatermark 仅读取内部成功镜像，不暴露给配置 API，也不作为待办筛选条件。
func (d *ShareExportCheckpointDAO) ReadWatermark(ctx context.Context) (*domain.ShareIncrementalWatermark, error) {
	var raw string
	err := d.db.QueryRowContext(ctx, `SELECT config_val FROM t_system_config WHERE config_key='share_strm_incremental_watermark'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var value domain.ShareIncrementalWatermark
	if err = json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("内部水位镜像损坏：%w", err)
	}
	if value.ProtocolVersion != 1 || value.CompletedRevision > value.PreparedRevision || value.PreparedRevision > value.ConfigRevision {
		return nil, fmt.Errorf("内部水位镜像协议或版本链无效")
	}
	logger.WithContext(ctx, "share_watermark").Log(logger.INFO, "读取内部成功水位镜像", logger.Fields{"config_revision": value.ConfigRevision, "completed_revision": value.CompletedRevision}, nil)
	return &value, nil
}
