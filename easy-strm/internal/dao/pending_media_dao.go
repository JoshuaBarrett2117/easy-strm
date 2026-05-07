package dao

import (
	"database/sql"
	"fmt"
	"strings"

	"easy-strm/internal/domain"
)

type PendingMediaDAO struct{}

func NewPendingMediaDAO() *PendingMediaDAO {
	return &PendingMediaDAO{}
}

const pendingMediaColumns = `id, source_kind, source_id, source_file_id, source_path, title, year, media_type, season, episode, tmdb_id, status, reason, related_task_id, created_at, updated_at`

func scanPendingMedia(scanner interface{ Scan(...interface{}) error }) (*domain.PendingMediaItem, error) {
	item := &domain.PendingMediaItem{}
	err := scanner.Scan(
		&item.ID,
		&item.SourceKind,
		&item.SourceID,
		&item.SourceFileID,
		&item.SourcePath,
		&item.Title,
		&item.Year,
		&item.MediaType,
		&item.Season,
		&item.Episode,
		&item.TmdbID,
		&item.Status,
		&item.Reason,
		&item.RelatedTaskID,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

func (d *PendingMediaDAO) Create(item *domain.PendingMediaItem) (*domain.PendingMediaItem, error) {
	if item == nil {
		return nil, fmt.Errorf("PendingMediaDAO[Create] 待处理项不能为空")
	}
	result, err := scanPendingMedia(DB.QueryRow(
		`INSERT INTO t_pending_media_item (source_kind, source_id, source_file_id, source_path, title, year, media_type, season, episode, tmdb_id, status, reason, related_task_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING `+pendingMediaColumns,
		defaultString(item.SourceKind, "manual"),
		item.SourceID,
		item.SourceFileID,
		item.SourcePath,
		item.Title,
		item.Year,
		item.MediaType,
		item.Season,
		item.Episode,
		item.TmdbID,
		defaultString(item.Status, "pending"),
		item.Reason,
		item.RelatedTaskID,
	))
	if err != nil {
		return nil, fmt.Errorf("PendingMediaDAO[Create] 创建失败: %v", err)
	}
	return result, nil
}

func (d *PendingMediaDAO) CreateOrUpdateOpen(item *domain.PendingMediaItem) (*domain.PendingMediaItem, error) {
	if item == nil {
		return nil, fmt.Errorf("PendingMediaDAO[CreateOrUpdateOpen] 待处理项不能为空")
	}
	existing, err := scanPendingMedia(DB.QueryRow(
		`SELECT `+pendingMediaColumns+` FROM t_pending_media_item
		WHERE source_id=$1 AND source_file_id=$2 AND status NOT IN ('completed', 'ignored')
		ORDER BY updated_at DESC, id DESC LIMIT 1`,
		item.SourceID,
		item.SourceFileID,
	))
	if err == nil && existing != nil {
		updated, updateErr := scanPendingMedia(DB.QueryRow(
			`UPDATE t_pending_media_item
			SET source_kind=$2, source_path=$3, title=$4, year=$5, media_type=$6,
				season=$7, episode=$8, tmdb_id=$9, status=$10, reason=$11,
				related_task_id=$12, updated_at=CURRENT_TIMESTAMP
			WHERE id=$1
			RETURNING `+pendingMediaColumns,
			existing.ID,
			defaultString(item.SourceKind, "sync"),
			item.SourcePath,
			item.Title,
			item.Year,
			item.MediaType,
			item.Season,
			item.Episode,
			item.TmdbID,
			defaultString(item.Status, "pending"),
			item.Reason,
			item.RelatedTaskID,
		))
		if updateErr != nil {
			return nil, fmt.Errorf("PendingMediaDAO[CreateOrUpdateOpen] 更新失败: %v", updateErr)
		}
		return updated, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("PendingMediaDAO[CreateOrUpdateOpen] 查询失败: %v", err)
	}
	return d.Create(item)
}

func (d *PendingMediaDAO) List(status, mediaType string, sourceID int) ([]*domain.PendingMediaItem, error) {
	query := `SELECT ` + pendingMediaColumns + ` FROM t_pending_media_item WHERE 1=1`
	args := []interface{}{}
	if strings.TrimSpace(status) != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND status=$%d", len(args))
	}
	if strings.TrimSpace(mediaType) != "" {
		args = append(args, mediaType)
		query += fmt.Sprintf(" AND media_type=$%d", len(args))
	}
	if sourceID > 0 {
		args = append(args, sourceID)
		query += fmt.Sprintf(" AND source_id=$%d", len(args))
	}
	query += ` ORDER BY updated_at DESC, id DESC`

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("PendingMediaDAO[List] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*domain.PendingMediaItem
	for rows.Next() {
		item, err := scanPendingMedia(rows)
		if err != nil {
			return nil, fmt.Errorf("PendingMediaDAO[List] 扫描失败: %v", err)
		}
		list = append(list, item)
	}
	return list, nil
}

func (d *PendingMediaDAO) GetByID(id int) (*domain.PendingMediaItem, error) {
	item, err := scanPendingMedia(DB.QueryRow(
		`SELECT `+pendingMediaColumns+` FROM t_pending_media_item WHERE id=$1`,
		id,
	))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("PendingMediaDAO[GetByID] 查询失败: %v", err)
	}
	return item, nil
}

func (d *PendingMediaDAO) UpdateIdentify(id, tmdbID, year, season, episode int, title, mediaType string) (*domain.PendingMediaItem, error) {
	item, err := scanPendingMedia(DB.QueryRow(
		`UPDATE t_pending_media_item
		SET tmdb_id=$2, year=$3, season=$4, episode=$5, title=$6, media_type=$7, status='identified', updated_at=CURRENT_TIMESTAMP
		WHERE id=$1
		RETURNING `+pendingMediaColumns,
		id,
		tmdbID,
		year,
		season,
		episode,
		title,
		mediaType,
	))
	if err != nil {
		return nil, fmt.Errorf("PendingMediaDAO[UpdateIdentify] 更新失败: %v", err)
	}
	return item, nil
}

func (d *PendingMediaDAO) UpdateStatus(id int, status, reason, relatedTaskID string) (*domain.PendingMediaItem, error) {
	item, err := scanPendingMedia(DB.QueryRow(
		`UPDATE t_pending_media_item
		SET status=$2, reason=$3, related_task_id=COALESCE(NULLIF($4, ''), related_task_id), updated_at=CURRENT_TIMESTAMP
		WHERE id=$1
		RETURNING `+pendingMediaColumns,
		id,
		status,
		reason,
		relatedTaskID,
	))
	if err != nil {
		return nil, fmt.Errorf("PendingMediaDAO[UpdateStatus] 更新失败: %v", err)
	}
	return item, nil
}
