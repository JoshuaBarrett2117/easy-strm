package dao

import (
	"easy-strm/internal/domain"
	"fmt"
)

type MediaCategoryDAO struct{}

func NewMediaCategoryDAO() *MediaCategoryDAO {
	return &MediaCategoryDAO{}
}

func (d *MediaCategoryDAO) GetAll() ([]*domain.MediaCategory, error) {
	rows, err := DB.Query(
		`SELECT id, name, media_type, target_path, match_rules, enabled, create_time, update_time
		 FROM t_media_category ORDER BY media_type, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("MediaCategoryDAO[GetAll] error: %v", err)
	}
	defer rows.Close()

	var list []*domain.MediaCategory
	for rows.Next() {
		cat := &domain.MediaCategory{}
		var matchRules []byte
		err := rows.Scan(
			&cat.ID, &cat.Name, &cat.MediaType, &cat.TargetPath, &matchRules,
			&cat.Enabled, &cat.CreateTime, &cat.UpdateTime,
		)
		if err != nil {
			return nil, fmt.Errorf("MediaCategoryDAO[GetAll] scan error: %v", err)
		}
		if matchRules != nil {
			cat.MatchRules = matchRules
		}
		list = append(list, cat)
	}
	return list, nil
}

func (d *MediaCategoryDAO) Create(cat *domain.MediaCategory) error {
	err := DB.QueryRow(
		`INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, create_time, update_time`,
		cat.Name, cat.MediaType, cat.TargetPath, cat.MatchRules, cat.Enabled,
	).Scan(&cat.ID, &cat.CreateTime, &cat.UpdateTime)
	if err != nil {
		return fmt.Errorf("MediaCategoryDAO[Create] error: %v", err)
	}
	return nil
}

func (d *MediaCategoryDAO) Update(cat *domain.MediaCategory) error {
	_, err := DB.Exec(
		`UPDATE t_media_category SET name = $2, media_type = $3, target_path = $4, match_rules = $5, enabled = $6, update_time = NOW()
		 WHERE id = $1`,
		cat.ID, cat.Name, cat.MediaType, cat.TargetPath, cat.MatchRules, cat.Enabled,
	)
	if err != nil {
		return fmt.Errorf("MediaCategoryDAO[Update] error: %v", err)
	}
	return nil
}

func (d *MediaCategoryDAO) Delete(id int) error {
	_, err := DB.Exec("DELETE FROM t_media_category WHERE id = $1", id)
	return err
}

func (d *MediaCategoryDAO) GetByMediaType(mediaType string) ([]*domain.MediaCategory, error) {
	rows, err := DB.Query(
		`SELECT id, name, media_type, target_path, match_rules, enabled, create_time, update_time
		 FROM t_media_category WHERE media_type = $1 AND enabled = true ORDER BY id ASC`,
		mediaType,
	)
	if err != nil {
		return nil, fmt.Errorf("MediaCategoryDAO[GetByMediaType] error: %v", err)
	}
	defer rows.Close()

	var list []*domain.MediaCategory
	for rows.Next() {
		cat := &domain.MediaCategory{}
		var matchRules []byte
		err := rows.Scan(
			&cat.ID, &cat.Name, &cat.MediaType, &cat.TargetPath, &matchRules,
			&cat.Enabled, &cat.CreateTime, &cat.UpdateTime,
		)
		if err != nil {
			return nil, err
		}
		if matchRules != nil {
			cat.MatchRules = matchRules
		}
		list = append(list, cat)
	}
	return list, nil
}
