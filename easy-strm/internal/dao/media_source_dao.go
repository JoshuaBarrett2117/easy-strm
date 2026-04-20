package dao

import (
	"database/sql"
	"fmt"

	"easy-strm/internal/domain"
)

type MediaSourceDAO struct{}

func NewMediaSourceDAO() *MediaSourceDAO {
	return &MediaSourceDAO{}
}

const mediaSourceColumns = `id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time`

func scanMediaSource(scanner interface{ Scan(...interface{}) error }) (*domain.MediaSource, error) {
	source := &domain.MediaSource{}
	err := scanner.Scan(
		&source.ID, &source.Name, &source.SourceType, &source.Path, &source.WatchPath,
		&source.Cloud115ID, &source.Priority, &source.Enabled,
		&source.OrganizeTargetPath,
		&source.MediaType, &source.ConflictPolicy, &source.OperationMode,
		&source.AutoOrganize, &source.WatchEnabled, &source.WatchInterval,
		&source.EmbyLibraryID,
		&source.CreateTime, &source.UpdateTime,
	)
	return source, err
}

func (d *MediaSourceDAO) GetByID(id int) (*domain.MediaSource, error) {
	source, err := scanMediaSource(DB.QueryRow(
		`SELECT `+mediaSourceColumns+` FROM t_media_source WHERE id = $1`,
		id,
	))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("MediaSourceDAO[GetByID] 查询失败: %v", err)
	}
	return source, nil
}

func (d *MediaSourceDAO) GetAll(sortField, sortOrder string) ([]*domain.MediaSource, error) {
	orderClause := "priority ASC, id ASC"
	if sortField != "" {
		orderClause = fmt.Sprintf("%s %s", sortField, sortOrder)
		if sortOrder == "" {
			orderClause = fmt.Sprintf("%s ASC", sortField)
		}
	}

	rows, err := DB.Query(
		fmt.Sprintf(`SELECT %s FROM t_media_source ORDER BY %s`, mediaSourceColumns, orderClause),
	)
	if err != nil {
		return nil, fmt.Errorf("MediaSourceDAO[GetAll] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*domain.MediaSource
	for rows.Next() {
		source, err := scanMediaSource(rows)
		if err != nil {
			return nil, fmt.Errorf("MediaSourceDAO[GetAll] 扫描失败: %v", err)
		}
		list = append(list, source)
	}
	return list, nil
}

func (d *MediaSourceDAO) GetByType(sourceType string) ([]*domain.MediaSource, error) {
	rows, err := DB.Query(
		`SELECT `+mediaSourceColumns+` FROM t_media_source WHERE source_type = $1 ORDER BY priority ASC, id ASC`,
		sourceType,
	)
	if err != nil {
		return nil, fmt.Errorf("MediaSourceDAO[GetByType] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*domain.MediaSource
	for rows.Next() {
		source, err := scanMediaSource(rows)
		if err != nil {
			return nil, fmt.Errorf("MediaSourceDAO[GetByType] 扫描失败: %v", err)
		}
		list = append(list, source)
	}
	return list, nil
}

func (d *MediaSourceDAO) GetEnabled() ([]*domain.MediaSource, error) {
	rows, err := DB.Query(
		`SELECT `+mediaSourceColumns+` FROM t_media_source WHERE enabled = true ORDER BY priority ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("MediaSourceDAO[GetEnabled] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*domain.MediaSource
	for rows.Next() {
		source, err := scanMediaSource(rows)
		if err != nil {
			return nil, fmt.Errorf("MediaSourceDAO[GetEnabled] 扫描失败: %v", err)
		}
		list = append(list, source)
	}
	return list, nil
}

func (d *MediaSourceDAO) Create(name, sourceType, path, watchPath string, cloud115ID *int, priority int, enabled bool, organizeTargetPath, mediaType, conflictPolicy, operationMode string, autoOrganize, watchEnabled bool, watchInterval int, embyLibraryID string) (*domain.MediaSource, error) {
	source, err := scanMediaSource(DB.QueryRow(
		`INSERT INTO t_media_source (name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING `+mediaSourceColumns,
		name, sourceType, path, watchPath, cloud115ID, priority, enabled, organizeTargetPath, mediaType, conflictPolicy, operationMode, autoOrganize, watchEnabled, watchInterval, embyLibraryID,
	))
	if err != nil {
		return nil, fmt.Errorf("MediaSourceDAO[Create] 创建失败: %v", err)
	}
	return source, nil
}

func (d *MediaSourceDAO) Update(id int, name, sourceType, path, watchPath string, cloud115ID *int, priority int, enabled bool, organizeTargetPath, mediaType, conflictPolicy, operationMode string, autoOrganize, watchEnabled bool, watchInterval int, embyLibraryID string) (*domain.MediaSource, error) {
	source, err := scanMediaSource(DB.QueryRow(
		`UPDATE t_media_source SET name=$2, source_type=$3, path=$4, watch_path=$5, cloud115_id=$6, priority=$7, enabled=$8, organize_target_path=$9, media_type=$10, conflict_policy=$11, operation_mode=$12, auto_organize=$13, watch_enabled=$14, watch_interval=$15, emby_library_id=$16
		WHERE id=$1
		RETURNING `+mediaSourceColumns,
		id, name, sourceType, path, watchPath, cloud115ID, priority, enabled, organizeTargetPath, mediaType, conflictPolicy, operationMode, autoOrganize, watchEnabled, watchInterval, embyLibraryID,
	))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("MediaSourceDAO[Update] 媒体源不存在")
		}
		return nil, fmt.Errorf("MediaSourceDAO[Update] 更新失败: %v", err)
	}
	return source, nil
}

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

// GetWatchEnabled 获取所有启用监控的媒体源
// 用于 WatchService 启动时加载需要监控的媒体源列表
func (d *MediaSourceDAO) GetWatchEnabled() ([]*domain.MediaSource, error) {
	rows, err := DB.Query(
		`SELECT ` + mediaSourceColumns + ` FROM t_media_source WHERE watch_enabled = true AND enabled = true ORDER BY priority ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("MediaSourceDAO[GetWatchEnabled] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*domain.MediaSource
	for rows.Next() {
		source, err := scanMediaSource(rows)
		if err != nil {
			return nil, fmt.Errorf("MediaSourceDAO[GetWatchEnabled] 扫描失败: %v", err)
		}
		list = append(list, source)
	}
	return list, nil
}
