package main

import (
	"database/sql"
	"time"
)

type NotificationConfig struct {
	ID        int       `json:"id"`
	Channel   string    `json:"channel"` // 通知渠道: telegram, serverchan, email
	Config    string    `json:"config"`  // JSON配置
	Enabled   bool      `json:"enabled"` // 是否启用
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetAllNotificationConfig 获取所有通知配置

func GetAllNotificationConfig() ([]*NotificationConfig, error) {
	Debug("Getting all notification configs")
	rows, err := db.Query("SELECT id, channel, config, enabled, created_at, updated_at FROM t_notification_config ORDER BY id")
	if err != nil {
		Error("Failed to query notification configs: %v", err)
		return nil, err
	}
	defer rows.Close()

	var configs []*NotificationConfig
	for rows.Next() {
		config := &NotificationConfig{}
		err := rows.Scan(&config.ID, &config.Channel, &config.Config, &config.Enabled, &config.CreatedAt, &config.UpdatedAt)
		if err != nil {
			Error("Failed to scan notification config: %v", err)
			return nil, err
		}
		configs = append(configs, config)
	}
	Debug("Found %d notification configs", len(configs))
	return configs, nil
}

// GetNotificationConfigByChannel 根据渠道获取通知配置

func GetNotificationConfigByChannel(channel string) (*NotificationConfig, error) {
	Debug("Getting notification config by channel: %s", channel)
	config := &NotificationConfig{}
	err := db.QueryRow("SELECT id, channel, config, enabled, created_at, updated_at FROM t_notification_config WHERE channel = $1", channel).Scan(
		&config.ID, &config.Channel, &config.Config, &config.Enabled, &config.CreatedAt, &config.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			Debug("No notification config found for channel: %s", channel)
			return nil, nil
		}
		Error("Failed to get notification config for channel %s: %v", channel, err)
		return nil, err
	}
	return config, nil
}

// UpsertNotificationConfig 创建或更新通知配置

func UpsertNotificationConfig(channel, configJSON string, enabled bool) (*NotificationConfig, error) {
	Debug("Upserting notification config for channel: %s", channel)
	result := &NotificationConfig{}
	err := db.QueryRow(
		`INSERT INTO t_notification_config (channel, config, enabled) VALUES ($1, $2, $3)
		ON CONFLICT (channel) DO UPDATE SET config = $2, enabled = $3, updated_at = CURRENT_TIMESTAMP
		RETURNING id, channel, config, enabled, created_at, updated_at`,
		channel, configJSON, enabled,
	).Scan(&result.ID, &result.Channel, &result.Config, &result.Enabled, &result.CreatedAt, &result.UpdatedAt)
	if err != nil {
		Error("Failed to upsert notification config for channel %s: %v", channel, err)
		return nil, err
	}
	Info("Upserted notification config for channel: %s", channel)
	return result, nil
}

// DeleteNotificationConfig 删除通知配置

func DeleteNotificationConfig(channel string) error {
	Debug("Deleting notification config for channel: %s", channel)
	result, err := db.Exec("DELETE FROM t_notification_config WHERE channel = $1", channel)
	if err != nil {
		Error("Failed to delete notification config for channel %s: %v", channel, err)
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	Debug("Deleted %d notification config(s) for channel: %s", rowsAffected, channel)
	return nil
}

// getDBInstance 获取全局数据库连接实例
