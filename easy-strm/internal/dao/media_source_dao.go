package dao

import (
	"database/sql"
	"fmt"

	"easy-strm/internal/domain"
)

// MediaSourceDAO 媒体源数据访问层
type MediaSourceDAO struct{}

// NewMediaSourceDAO 创建媒体源DAO实例
func NewMediaSourceDAO() *MediaSourceDAO {
	return &MediaSourceDAO{}
}

// GetByID 根据ID获取媒体源
// 参数:
//   - id: 媒体源ID
// 返回:
//   - *domain.MediaSource: 媒体源信息
//   - error: 错误信息
func (d *MediaSourceDAO) GetByID(id int) (*domain.MediaSource, error) {
	source := &domain.MediaSource{}
	err := DB.QueryRow(
		`SELECT id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time
		FROM t_media_source WHERE id = $1`,
		id,
	).Scan(
		&source.ID, &source.Name, &source.SourceType, &source.Path,
		&source.Cloud115ID, &source.Priority, &source.Enabled,
		&source.CreateTime, &source.UpdateTime,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("MediaSourceDAO[GetByID] 查询失败: %v", err)
	}
	return source, nil
}

// GetAll 获取所有媒体源
// 参数:
//   - sortField: 排序字段
//   - sortOrder: 排序方向 (asc/desc)
// 返回:
//   - []*domain.MediaSource: 媒体源列表
//   - error: 错误信息
func (d *MediaSourceDAO) GetAll(sortField, sortOrder string) ([]*domain.MediaSource, error) {
	orderClause := "priority ASC, id ASC"
	if sortField != "" {
		orderClause = fmt.Sprintf("%s %s", sortField, sortOrder)
		if sortOrder == "" {
			orderClause = fmt.Sprintf("%s ASC", sortField)
		}
	}

	rows, err := DB.Query(
		fmt.Sprintf(`SELECT id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time
		FROM t_media_source ORDER BY %s`, orderClause),
	)
	if err != nil {
		return nil, fmt.Errorf("MediaSourceDAO[GetAll] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*domain.MediaSource
	for rows.Next() {
		source := &domain.MediaSource{}
		err := rows.Scan(
			&source.ID, &source.Name, &source.SourceType, &source.Path,
			&source.Cloud115ID, &source.Priority, &source.Enabled,
			&source.CreateTime, &source.UpdateTime,
		)
		if err != nil {
			return nil, fmt.Errorf("MediaSourceDAO[GetAll] 扫描失败: %v", err)
		}
		list = append(list, source)
	}
	return list, nil
}

// GetByType 根据类型获取媒体源列表
// 参数:
//   - sourceType: 媒体源类型 (local/cloud115)
// 返回:
//   - []*domain.MediaSource: 媒体源列表
//   - error: 错误信息
func (d *MediaSourceDAO) GetByType(sourceType string) ([]*domain.MediaSource, error) {
	rows, err := DB.Query(
		`SELECT id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time
		FROM t_media_source WHERE source_type = $1 ORDER BY priority ASC, id ASC`,
		sourceType,
	)
	if err != nil {
		return nil, fmt.Errorf("MediaSourceDAO[GetByType] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*domain.MediaSource
	for rows.Next() {
		source := &domain.MediaSource{}
		err := rows.Scan(
			&source.ID, &source.Name, &source.SourceType, &source.Path,
			&source.Cloud115ID, &source.Priority, &source.Enabled,
			&source.CreateTime, &source.UpdateTime,
		)
		if err != nil {
			return nil, fmt.Errorf("MediaSourceDAO[GetByType] 扫描失败: %v", err)
		}
		list = append(list, source)
	}
	return list, nil
}

// GetEnabled 获取所有启用的媒体源
// 返回:
//   - []*domain.MediaSource: 启用的媒体源列表
//   - error: 错误信息
func (d *MediaSourceDAO) GetEnabled() ([]*domain.MediaSource, error) {
	rows, err := DB.Query(
		`SELECT id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time
		FROM t_media_source WHERE enabled = true ORDER BY priority ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("MediaSourceDAO[GetEnabled] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*domain.MediaSource
	for rows.Next() {
		source := &domain.MediaSource{}
		err := rows.Scan(
			&source.ID, &source.Name, &source.SourceType, &source.Path,
			&source.Cloud115ID, &source.Priority, &source.Enabled,
			&source.CreateTime, &source.UpdateTime,
		)
		if err != nil {
			return nil, fmt.Errorf("MediaSourceDAO[GetEnabled] 扫描失败: %v", err)
		}
		list = append(list, source)
	}
	return list, nil
}

// Create 创建媒体源
// 参数:
//   - name: 媒体源名称
//   - sourceType: 媒体源类型
//   - path: 路径
//   - cloud115ID: 115账号ID（可选）
//   - priority: 优先级
//   - enabled: 是否启用
// 返回:
//   - *domain.MediaSource: 创建的媒体源
//   - error: 错误信息
func (d *MediaSourceDAO) Create(name, sourceType, path string, cloud115ID *int, priority int, enabled bool) (*domain.MediaSource, error) {
	source := &domain.MediaSource{}
	err := DB.QueryRow(
		`INSERT INTO t_media_source (name, source_type, path, cloud115_id, priority, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time`,
		name, sourceType, path, cloud115ID, priority, enabled,
	).Scan(
		&source.ID, &source.Name, &source.SourceType, &source.Path,
		&source.Cloud115ID, &source.Priority, &source.Enabled,
		&source.CreateTime, &source.UpdateTime,
	)
	if err != nil {
		return nil, fmt.Errorf("MediaSourceDAO[Create] 创建失败: %v", err)
	}
	return source, nil
}

// Update 更新媒体源
// 参数:
//   - id: 媒体源ID
//   - name: 媒体源名称
//   - sourceType: 媒体源类型
//   - path: 路径
//   - cloud115ID: 115账号ID（可选）
//   - priority: 优先级
//   - enabled: 是否启用
// 返回:
//   - *domain.MediaSource: 更新后的媒体源
//   - error: 错误信息
func (d *MediaSourceDAO) Update(id int, name, sourceType, path string, cloud115ID *int, priority int, enabled bool) (*domain.MediaSource, error) {
	source := &domain.MediaSource{}
	err := DB.QueryRow(
		`UPDATE t_media_source SET name=$2, source_type=$3, path=$4, cloud115_id=$5, priority=$6, enabled=$7
		WHERE id=$1
		RETURNING id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time`,
		id, name, sourceType, path, cloud115ID, priority, enabled,
	).Scan(
		&source.ID, &source.Name, &source.SourceType, &source.Path,
		&source.Cloud115ID, &source.Priority, &source.Enabled,
		&source.CreateTime, &source.UpdateTime,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("MediaSourceDAO[Update] 媒体源不存在")
		}
		return nil, fmt.Errorf("MediaSourceDAO[Update] 更新失败: %v", err)
	}
	return source, nil
}

// Delete 删除媒体源
// 参数:
//   - id: 媒体源ID
// 返回:
//   - error: 错误信息
func (d *MediaSourceDAO) Delete(id int) error {
	result, err := DB.Exec("DELETE FROM t_media_source WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("MediaSourceDAO[Delete] 删除失败: %v", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("MediaSourceDAO[Delete] 媒体源不存在")
	}
	return nil
}

// UpdateEnabled 更新媒体源启用状态
// 参数:
//   - id: 媒体源ID
//   - enabled: 是否启用
// 返回:
//   - error: 错误信息
func (d *MediaSourceDAO) UpdateEnabled(id int, enabled bool) error {
	result, err := DB.Exec(
		`UPDATE t_media_source SET enabled=$2 WHERE id=$1`,
		id, enabled,
	)
	if err != nil {
		return fmt.Errorf("MediaSourceDAO[UpdateEnabled] 更新状态失败: %v", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("MediaSourceDAO[UpdateEnabled] 媒体源不存在")
	}
	return nil
}
