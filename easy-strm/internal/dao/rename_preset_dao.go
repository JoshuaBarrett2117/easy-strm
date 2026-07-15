package dao

import (
	"database/sql"
	"fmt"
	"time"
)

// RenamePresetDAO 更名预设数据访问层
type RenamePresetDAO struct{}

// NewRenamePresetDAO 创建更名预设DAO实例
func NewRenamePresetDAO() *RenamePresetDAO {
	return &RenamePresetDAO{}
}

// RenamePreset 更名预设模型
type RenamePreset struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	MediaType  string    `json:"media_type"`
	Template   string    `json:"template"`
	Enabled    bool      `json:"enabled"`
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}

// GetByID 根据ID获取更名预设
func (d *RenamePresetDAO) GetByID(id int) (*RenamePreset, error) {
	preset := &RenamePreset{}
	err := DB.QueryRow(
		`SELECT id, name, media_type, template, enabled, create_time, update_time
		 FROM t_rename_preset WHERE id = $1`,
		id,
	).Scan(
		&preset.ID, &preset.Name, &preset.MediaType, &preset.Template,
		&preset.Enabled, &preset.CreateTime, &preset.UpdateTime,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("RenamePresetDAO[GetByID] 查询失败: %v", err)
	}
	return preset, nil
}

// GetByMediaType 根据媒体类型获取预设列表
func (d *RenamePresetDAO) GetByMediaType(mediaType string) ([]*RenamePreset, error) {
	rows, err := DB.Query(
		`SELECT id, name, media_type, template, enabled, create_time, update_time
		 FROM t_rename_preset
		 WHERE media_type = $1 AND enabled = true
		 ORDER BY id ASC`,
		mediaType,
	)
	if err != nil {
		return nil, fmt.Errorf("RenamePresetDAO[GetByMediaType] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*RenamePreset
	for rows.Next() {
		preset := &RenamePreset{}
		err := rows.Scan(
			&preset.ID, &preset.Name, &preset.MediaType, &preset.Template,
			&preset.Enabled, &preset.CreateTime, &preset.UpdateTime,
		)
		if err != nil {
			return nil, fmt.Errorf("RenamePresetDAO[GetByMediaType] 扫描失败: %v", err)
		}
		list = append(list, preset)
	}
	return list, nil
}

// GetAll 获取所有预设
func (d *RenamePresetDAO) GetAll() ([]*RenamePreset, error) {
	rows, err := DB.Query(
		`SELECT id, name, media_type, template, enabled, create_time, update_time
		 FROM t_rename_preset ORDER BY media_type, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("RenamePresetDAO[GetAll] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*RenamePreset
	for rows.Next() {
		preset := &RenamePreset{}
		err := rows.Scan(
			&preset.ID, &preset.Name, &preset.MediaType, &preset.Template,
			&preset.Enabled, &preset.CreateTime, &preset.UpdateTime,
		)
		if err != nil {
			return nil, fmt.Errorf("RenamePresetDAO[GetAll] 扫描失败: %v", err)
		}
		list = append(list, preset)
	}
	return list, nil
}

// Create 创建预设
func (d *RenamePresetDAO) Create(preset *RenamePreset) error {
	err := DB.QueryRow(
		`INSERT INTO t_rename_preset (name, media_type, template, enabled)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, create_time, update_time`,
		preset.Name, preset.MediaType, preset.Template, preset.Enabled,
	).Scan(&preset.ID, &preset.CreateTime, &preset.UpdateTime)

	if err != nil {
		return fmt.Errorf("RenamePresetDAO[Create] 创建失败: %v", err)
	}
	return nil
}

// Update 更新预设
func (d *RenamePresetDAO) Update(preset *RenamePreset) error {
	_, err := DB.Exec(
		`UPDATE t_rename_preset SET name = $2, media_type = $3, template = $4, enabled = $5, update_time = NOW()
		 WHERE id = $1`,
		preset.ID, preset.Name, preset.MediaType, preset.Template, preset.Enabled,
	)

	if err != nil {
		return fmt.Errorf("RenamePresetDAO[Update] 更新失败: %v", err)
	}
	return nil
}

// Delete 删除预设
func (d *RenamePresetDAO) Delete(id int) error {
	_, err := DB.Exec("DELETE FROM t_rename_preset WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("RenamePresetDAO[Delete] 删除失败: %v", err)
	}
	return nil
}
