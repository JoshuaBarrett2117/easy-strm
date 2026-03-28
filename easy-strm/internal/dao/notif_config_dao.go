package dao

import (
	"database/sql"
	"fmt"

	"easy-strm/internal/domain"
)

type NotificationConfigDAO struct{}

func NewNotificationConfigDAO() *NotificationConfigDAO {
	return &NotificationConfigDAO{}
}

func (n *NotificationConfigDAO) GetAll() ([]*domain.NotificationConfig, error) {
	rows, err := db.Query("SELECT id, channel, config, enabled, created_at, updated_at FROM t_notification_config ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("NotificationConfigDAO[GetAll] 查询失败: %v", err)
	}
	defer rows.Close()

	var configs []*domain.NotificationConfig
	for rows.Next() {
		config := &domain.NotificationConfig{}
		err := rows.Scan(&config.ID, &config.Channel, &config.Config, &config.Enabled, &config.CreatedAt, &config.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("NotificationConfigDAO[GetAll] 扫描失败: %v", err)
		}
		configs = append(configs, config)
	}
	return configs, nil
}

func (n *NotificationConfigDAO) GetByChannel(channel string) (*domain.NotificationConfig, error) {
	config := &domain.NotificationConfig{}
	err := db.QueryRow(
		"SELECT id, channel, config, enabled, created_at, updated_at FROM t_notification_config WHERE channel = $1",
		channel,
	).Scan(&config.ID, &config.Channel, &config.Config, &config.Enabled, &config.CreatedAt, &config.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("NotificationConfigDAO[GetByChannel] 查询失败: %v", err)
	}
	return config, nil
}

func (n *NotificationConfigDAO) Upsert(channel, configJSON string, enabled bool) (*domain.NotificationConfig, error) {
	result := &domain.NotificationConfig{}
	err := db.QueryRow(
		`INSERT INTO t_notification_config (channel, config, enabled) VALUES ($1, $2, $3)
		ON CONFLICT (channel) DO UPDATE SET config = $2, enabled = $3, updated_at = CURRENT_TIMESTAMP
		RETURNING id, channel, config, enabled, created_at, updated_at`,
		channel, configJSON, enabled,
	).Scan(&result.ID, &result.Channel, &result.Config, &result.Enabled, &result.CreatedAt, &result.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("NotificationConfigDAO[Upsert] 失败: %v", err)
	}
	return result, nil
}

func (n *NotificationConfigDAO) Delete(channel string) error {
	result, err := db.Exec("DELETE FROM t_notification_config WHERE channel = $1", channel)
	if err != nil {
		return fmt.Errorf("NotificationConfigDAO[Delete] 删除失败: %v", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("NotificationConfigDAO[Delete] 配置不存在")
	}
	return nil
}