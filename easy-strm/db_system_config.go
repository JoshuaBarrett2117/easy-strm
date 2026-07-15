package main

func GetSystemConfigByKey(key string) (*SystemConfig, error) {
	Debug("Getting system config by key: %s", key)
	config := &SystemConfig{}
	err := db.QueryRow("SELECT id, config_key, config_val, create_time, update_time FROM t_system_config WHERE config_key = $1", key).Scan(
		&config.ID, &config.ConfigKey, &config.ConfigVal, &config.CreateTime, &config.UpdateTime)
	if err != nil {
		Debug("System config not found by key: %s", key)
		return nil, err
	}
	Debug("Found system config by key %s: %s", key, config.ConfigVal)
	return config, nil
}

// UpsertSystemConfig 创建或更新系统配置

func UpsertSystemConfig(key, value string) (*SystemConfig, error) {
	Debug("Upserting system config: %s = %s", key, value)
	config := &SystemConfig{}
	err := db.QueryRow(`
		INSERT INTO t_system_config (config_key, config_val)
		VALUES ($1, $2)
		ON CONFLICT (config_key) DO UPDATE SET config_val = $2
		RETURNING id, config_key, config_val, create_time, update_time
	`, key, value).Scan(&config.ID, &config.ConfigKey, &config.ConfigVal, &config.CreateTime, &config.UpdateTime)
	if err != nil {
		Error("Failed to upsert system config %s: %v", key, err)
		return nil, err
	}
	Info("Upserted system config: %s = %s", key, value)
	return config, nil
}

// GetAllSystemConfig 获取所有系统配置

func GetAllSystemConfig() ([]*SystemConfig, error) {
	Debug("Getting all system configs")
	rows, err := db.Query("SELECT id, config_key, config_val, create_time, update_time FROM t_system_config")
	if err != nil {
		Error("Failed to get all system configs: %v", err)
		return nil, err
	}
	defer rows.Close()

	var configList []*SystemConfig
	for rows.Next() {
		config := &SystemConfig{}
		err := rows.Scan(&config.ID, &config.ConfigKey, &config.ConfigVal, &config.CreateTime, &config.UpdateTime)
		if err != nil {
			Error("Failed to scan system config row: %v", err)
			return nil, err
		}
		configList = append(configList, config)
	}

	if err = rows.Err(); err != nil {
		Error("Error iterating system config rows: %v", err)
		return nil, err
	}

	Debug("Found %d system configs", len(configList))
	return configList, nil
}
