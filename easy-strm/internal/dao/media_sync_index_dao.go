package dao

import (
	"database/sql"
	"fmt"
	"strings"

	"easy-strm/internal/domain"
)

type MediaSyncIndexDAO struct{}

func NewMediaSyncIndexDAO() *MediaSyncIndexDAO {
	return &MediaSyncIndexDAO{}
}

const mediaSyncIndexColumns = `id, source_id, source_type, source_file_id, source_path, source_name, source_pick_code, source_sha1, source_size, source_modified_time, target_path, strm_path, metadata_path, media_server_type, media_server_library_id, tmdb_id, media_type, identity_status, sync_status, last_change_type, last_task_id, created_at, updated_at`

func scanMediaSyncIndex(scanner interface{ Scan(...interface{}) error }) (*domain.MediaSyncIndex, error) {
	item := &domain.MediaSyncIndex{}
	err := scanner.Scan(
		&item.ID,
		&item.SourceID,
		&item.SourceType,
		&item.SourceFileID,
		&item.SourcePath,
		&item.SourceName,
		&item.SourcePickCode,
		&item.SourceSHA1,
		&item.SourceSize,
		&item.SourceModifiedTime,
		&item.TargetPath,
		&item.StrmPath,
		&item.MetadataPath,
		&item.MediaServerType,
		&item.MediaServerLibraryID,
		&item.TmdbID,
		&item.MediaType,
		&item.IdentityStatus,
		&item.SyncStatus,
		&item.LastChangeType,
		&item.LastTaskID,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

func (d *MediaSyncIndexDAO) Upsert(item *domain.MediaSyncIndex) (*domain.MediaSyncIndex, error) {
	if item == nil {
		return nil, fmt.Errorf("MediaSyncIndexDAO[Upsert] 索引不能为空")
	}
	item.SourceFileID = strings.TrimSpace(item.SourceFileID)
	if item.SourceID <= 0 || item.SourceFileID == "" {
		return nil, fmt.Errorf("MediaSyncIndexDAO[Upsert] source_id 和 source_file_id 不能为空")
	}

	row := DB.QueryRow(
		`INSERT INTO t_media_sync_index (
			source_id, source_type, source_file_id, source_path, source_name, source_pick_code, source_sha1,
			source_size, source_modified_time, target_path, strm_path, metadata_path, media_server_type,
			media_server_library_id, tmdb_id, media_type, identity_status, sync_status, last_change_type, last_task_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		ON CONFLICT (source_id, source_file_id) DO UPDATE SET
			source_type = EXCLUDED.source_type,
			source_path = EXCLUDED.source_path,
			source_name = EXCLUDED.source_name,
			source_pick_code = EXCLUDED.source_pick_code,
			source_sha1 = EXCLUDED.source_sha1,
			source_size = EXCLUDED.source_size,
			source_modified_time = EXCLUDED.source_modified_time,
			target_path = EXCLUDED.target_path,
			strm_path = EXCLUDED.strm_path,
			metadata_path = EXCLUDED.metadata_path,
			media_server_type = EXCLUDED.media_server_type,
			media_server_library_id = EXCLUDED.media_server_library_id,
			tmdb_id = EXCLUDED.tmdb_id,
			media_type = EXCLUDED.media_type,
			identity_status = EXCLUDED.identity_status,
			sync_status = EXCLUDED.sync_status,
			last_change_type = EXCLUDED.last_change_type,
			last_task_id = EXCLUDED.last_task_id,
			updated_at = CURRENT_TIMESTAMP
		RETURNING `+mediaSyncIndexColumns,
		item.SourceID,
		item.SourceType,
		item.SourceFileID,
		item.SourcePath,
		item.SourceName,
		item.SourcePickCode,
		item.SourceSHA1,
		item.SourceSize,
		item.SourceModifiedTime,
		item.TargetPath,
		item.StrmPath,
		item.MetadataPath,
		item.MediaServerType,
		item.MediaServerLibraryID,
		item.TmdbID,
		item.MediaType,
		defaultString(item.IdentityStatus, domain.IdentityStatusUnknown),
		defaultString(item.SyncStatus, domain.SyncStatusActive),
		defaultString(item.LastChangeType, "created"),
		item.LastTaskID,
	)
	if err := row.Err(); err != nil {
		return nil, fmt.Errorf("MediaSyncIndexDAO[Upsert] 写入失败: %v", err)
	}
	result, err := scanMediaSyncIndex(row)
	if err != nil {
		return nil, fmt.Errorf("MediaSyncIndexDAO[Upsert] 扫描失败: %v", err)
	}
	return result, nil
}

func (d *MediaSyncIndexDAO) GetBySourceFileID(sourceID int, sourceFileID string) (*domain.MediaSyncIndex, error) {
	item, err := scanMediaSyncIndex(DB.QueryRow(
		`SELECT `+mediaSyncIndexColumns+` FROM t_media_sync_index WHERE source_id=$1 AND source_file_id=$2`,
		sourceID,
		sourceFileID,
	))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("MediaSyncIndexDAO[GetBySourceFileID] 查询失败: %v", err)
	}
	return item, nil
}

func (d *MediaSyncIndexDAO) GetByID(id int) (*domain.MediaSyncIndex, error) {
	item, err := scanMediaSyncIndex(DB.QueryRow(
		`SELECT `+mediaSyncIndexColumns+` FROM t_media_sync_index WHERE id=$1`,
		id,
	))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("MediaSyncIndexDAO[GetByID] 查询失败: %v", err)
	}
	return item, nil
}

func (d *MediaSyncIndexDAO) UpdatePipelineState(item *domain.MediaSyncIndex) (*domain.MediaSyncIndex, error) {
	if item == nil || item.ID <= 0 {
		return nil, fmt.Errorf("MediaSyncIndexDAO[UpdatePipelineState] 索引不能为空")
	}
	result, err := scanMediaSyncIndex(DB.QueryRow(
		`UPDATE t_media_sync_index
		SET target_path=$2, strm_path=$3, metadata_path=$4, media_server_type=$5, media_server_library_id=$6,
			tmdb_id=$7, media_type=$8, identity_status=$9, sync_status=$10, last_change_type=$11,
			last_task_id=$12, updated_at=CURRENT_TIMESTAMP
		WHERE id=$1
		RETURNING `+mediaSyncIndexColumns,
		item.ID,
		item.TargetPath,
		item.StrmPath,
		item.MetadataPath,
		item.MediaServerType,
		item.MediaServerLibraryID,
		item.TmdbID,
		item.MediaType,
		defaultString(item.IdentityStatus, domain.IdentityStatusUnknown),
		defaultString(item.SyncStatus, domain.SyncStatusActive),
		defaultString(item.LastChangeType, "pipeline"),
		item.LastTaskID,
	))
	if err != nil {
		return nil, fmt.Errorf("MediaSyncIndexDAO[UpdatePipelineState] 更新失败: %v", err)
	}
	return result, nil
}

func (d *MediaSyncIndexDAO) ListBySource(sourceID int, status string) ([]*domain.MediaSyncIndex, error) {
	query := `SELECT ` + mediaSyncIndexColumns + ` FROM t_media_sync_index WHERE source_id=$1`
	args := []interface{}{sourceID}
	if strings.TrimSpace(status) != "" {
		query += ` AND sync_status=$2`
		args = append(args, status)
	}
	query += ` ORDER BY source_path ASC, id ASC`

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("MediaSyncIndexDAO[ListBySource] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*domain.MediaSyncIndex
	for rows.Next() {
		item, err := scanMediaSyncIndex(rows)
		if err != nil {
			return nil, fmt.Errorf("MediaSyncIndexDAO[ListBySource] 扫描失败: %v", err)
		}
		list = append(list, item)
	}
	return list, nil
}

func (d *MediaSyncIndexDAO) MarkMissingExcept(sourceID int, taskID string, activeFileIDs []string) (int64, error) {
	if len(activeFileIDs) == 0 {
		result, err := DB.Exec(
			`UPDATE t_media_sync_index SET sync_status=$2, last_change_type='deleted', last_task_id=$3, updated_at=CURRENT_TIMESTAMP WHERE source_id=$1 AND sync_status <> $2`,
			sourceID,
			domain.SyncStatusMissing,
			taskID,
		)
		if err != nil {
			return 0, fmt.Errorf("MediaSyncIndexDAO[MarkMissingExcept] 更新失败: %v", err)
		}
		return result.RowsAffected()
	}

	placeholders := make([]string, len(activeFileIDs))
	args := []interface{}{sourceID, domain.SyncStatusMissing, taskID}
	for i, id := range activeFileIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+4)
		args = append(args, id)
	}
	query := fmt.Sprintf(
		`UPDATE t_media_sync_index SET sync_status=$2, last_change_type='deleted', last_task_id=$3, updated_at=CURRENT_TIMESTAMP
		WHERE source_id=$1 AND source_file_id NOT IN (%s) AND sync_status <> $2`,
		strings.Join(placeholders, ","),
	)
	result, err := DB.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("MediaSyncIndexDAO[MarkMissingExcept] 更新失败: %v", err)
	}
	return result.RowsAffected()
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
