package dao

import (
	"context"
	"database/sql"
	"fmt"

	"easy-strm/internal/domain"
)

type SystemConfigDAO struct{}

func NewSystemConfigDAO() *SystemConfigDAO {
	return &SystemConfigDAO{}
}

// GetHotConfigs 在入口预算内读取关联配置快照，不返回可变的缓存对象。
func (d *SystemConfigDAO) GetHotConfigs(ctx context.Context, keys ...string) (map[string]string, error) {
	return ReadSystemConfigSnapshot(ctx, DB, keys)
}

// GetByKey 根据Key获取配置
func (d *SystemConfigDAO) GetByKey(key string) (*domain.SystemConfig, error) {
	if key == "share_strm_incremental_watermark" {
		return nil, nil
	}
	config := &domain.SystemConfig{}
	err := DB.QueryRow(
		`SELECT id, config_key, config_val, create_time, update_time 
		 FROM t_system_config WHERE config_key = $1`,
		key,
	).Scan(&config.ID, &config.ConfigKey, &config.ConfigVal, &config.CreateTime, &config.UpdateTime)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("SystemConfigDAO[GetByKey] 查询失败: %v", err)
	}
	return config, nil
}

// GetAll 获取所有配置
func (d *SystemConfigDAO) GetAll() ([]*domain.SystemConfig, error) {
	rows, err := DB.Query("SELECT id, config_key, config_val, create_time, update_time FROM t_system_config WHERE config_key<>'share_strm_incremental_watermark'")
	if err != nil {
		return nil, fmt.Errorf("SystemConfigDAO[GetAll] 查询失败: %v", err)
	}
	defer rows.Close()

	var configs []*domain.SystemConfig
	for rows.Next() {
		config := &domain.SystemConfig{}
		err := rows.Scan(&config.ID, &config.ConfigKey, &config.ConfigVal, &config.CreateTime, &config.UpdateTime)
		if err != nil {
			return nil, fmt.Errorf("SystemConfigDAO[GetAll] 扫描失败: %v", err)
		}
		configs = append(configs, config)
	}
	return configs, nil
}

// Upsert 更新或插入配置
func (d *SystemConfigDAO) Upsert(key, val string) error {
	if key == "share_strm_incremental_watermark" {
		return fmt.Errorf("内部水位不可通过配置接口修改")
	}
	defer InvalidateSystemConfigCache(DB)
	_, err := DB.Exec(
		`INSERT INTO t_system_config (config_key, config_val, update_time)
		 VALUES ($1, $2, CURRENT_TIMESTAMP)
		 ON CONFLICT (config_key) DO UPDATE SET config_val = $2, update_time = CURRENT_TIMESTAMP`,
		key, val,
	)
	if err != nil {
		return fmt.Errorf("SystemConfigDAO[Upsert] 失败: %v", err)
	}
	return nil
}

// BatchUpsert 批量更新或插入配置
func (d *SystemConfigDAO) BatchUpsert(configs map[string]string) error {
	if _, present := configs["share_strm_incremental_watermark"]; present {
		return fmt.Errorf("内部水位不可通过配置接口修改")
	}
	defer InvalidateSystemConfigCache(DB)
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for k, v := range configs {
		_, err := tx.Exec(
			`INSERT INTO t_system_config (config_key, config_val, update_time)
			 VALUES ($1, $2, CURRENT_TIMESTAMP)
			 ON CONFLICT (config_key) DO UPDATE SET config_val = $2, update_time = CURRENT_TIMESTAMP`,
			k, v,
		)
		if err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// EnsureTable 确保表存在 (如果数据库初始化逻辑中已包含则可选)
func (d *SystemConfigDAO) EnsureTable() error {
	_, err := DB.Exec(`
		CREATE TABLE IF NOT EXISTS t_system_config (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			config_key TEXT UNIQUE,
			config_val TEXT,
			create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}
