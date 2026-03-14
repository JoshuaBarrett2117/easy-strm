package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

type User struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	Password   string    `json:"password"`
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}

// Cloud115 115云账号信息
type Cloud115 struct {
	ID                int       `json:"id"`
	Name              string    `json:"name"`
	Cookie            string    `json:"cookie"`
	RefreshToken      string    `json:"refresh_token"`
	AccessToken       string    `json:"access_token"`
	ExpiresIn         int       `json:"expires_in"`
	TransferAccountID int       `json:"transfer_account_id"` // 转存目标账号ID
	TransferDirectory string    `json:"transfer_directory"`  // 转存目录路径
	CreateTime        time.Time `json:"create_time"`
	UpdateTime        time.Time `json:"update_time"`
}

// StrmConfig STRM文件配置信息
type StrmConfig struct {
	ID          int       `json:"id"`
	Cloud115Id  int       `json:"cloud115_id"`   // 115账号ID
	NetDiskPath string    `json:"net_disk_path"` // 网盘目录
	LocalPath   string    `json:"local_path"`    // 本地目录
	Cron        string    `json:"cron"`          // cron表达式
	Extension   string    `json:"extension"`     // STRM文件后缀名
	DirTreeFile string    `json:"dir_tree_file"` // 本地目录树文件路径（可选，如果指定则不从API获取）
	CreateTime  time.Time `json:"create_time"`
	UpdateTime  time.Time `json:"update_time"`
}

// SystemConfig 系统配置信息
type SystemConfig struct {
	ID         int       `json:"id"`
	ConfigKey  string    `json:"config_key"` // 配置键
	ConfigVal  string    `json:"config_val"` // 配置值
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}

var db *sql.DB

func InitDB(config *Config) error {
	// 构建数据库连接字符串
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.PostgreSQL.Host,
		config.PostgreSQL.Port,
		config.PostgreSQL.User,
		config.PostgreSQL.Password,
		config.PostgreSQL.Database,
	)

	Debug("Connecting to database with connection string: host=%s port=%d user=%s dbname=%s",
		config.PostgreSQL.Host, config.PostgreSQL.Port, config.PostgreSQL.User, config.PostgreSQL.Database)

	// 连接数据库
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		Error("Failed to open database connection: %v", err)
		return err
	}

	// 测试连接
	err = db.Ping()
	if err != nil {
		Error("Failed to ping database: %v", err)
		return err
	}

	// 创建用户表（如果不存在）
	createUserTableSQL := `
	CREATE TABLE IF NOT EXISTS t_user (
		id SERIAL PRIMARY KEY,
		name VARCHAR(50) UNIQUE NOT NULL,
		password VARCHAR(255) NOT NULL,
		create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err = db.Exec(createUserTableSQL)
	if err != nil {
		Error("Failed to create user table: %v", err)
		return err
	}

	// 为现有t_user表添加缺失的字段
	alterUserTableSQL := `
	DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_user' AND column_name = 'create_time') THEN
			ALTER TABLE t_user ADD COLUMN create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_user' AND column_name = 'update_time') THEN
			ALTER TABLE t_user ADD COLUMN update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
		END IF;
	END $$;
	`

	_, err = db.Exec(alterUserTableSQL)
	if err != nil {
		Error("Failed to alter user table: %v", err)
		return err
	}

	// 添加用户表更新时间函数
	_, err = db.Exec(`CREATE OR REPLACE FUNCTION update_user_timestamp() RETURNS TRIGGER AS $$ BEGIN NEW.update_time = CURRENT_TIMESTAMP; RETURN NEW; END; $$ LANGUAGE plpgsql;`)
	if err != nil {
		Error("Failed to create update_user_timestamp function: %v", err)
		return err
	}

	// 检查并添加用户表更新时间触发器
	addUserTriggerSQL := `
	DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = 't_user'::regclass AND tgname = 'update_user_timestamp_trigger') THEN
			CREATE TRIGGER update_user_timestamp_trigger
			BEFORE UPDATE ON t_user
			FOR EACH ROW
			EXECUTE FUNCTION update_user_timestamp();
		END IF;
	END $$;
	`

	_, err = db.Exec(addUserTriggerSQL)
	if err != nil {
		Error("Failed to add user update trigger: %v", err)
		return err
	}

	// 创建115云账号表（如果不存在）
	createCloud115TableSQL := `
	CREATE TABLE IF NOT EXISTS t_cloud_115 (
		id SERIAL PRIMARY KEY,
		name VARCHAR(50) UNIQUE NOT NULL,
		cookie TEXT,
		refresh_token TEXT,
		access_token TEXT,
		expires_in INTEGER,
		create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err = db.Exec(createCloud115TableSQL)
	if err != nil {
		Error("Failed to create cloud_115 table: %v", err)
		return err
	}

	// 为现有t_cloud_115表添加缺失的字段
	alterCloudTableSQL := `
	DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cloud_115' AND column_name = 'create_time') THEN
			ALTER TABLE t_cloud_115 ADD COLUMN create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cloud_115' AND column_name = 'update_time') THEN
			ALTER TABLE t_cloud_115 ADD COLUMN update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cloud_115' AND column_name = 'transfer_account_id') THEN
			ALTER TABLE t_cloud_115 ADD COLUMN transfer_account_id INTEGER DEFAULT 0;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cloud_115' AND column_name = 'transfer_directory') THEN
			ALTER TABLE t_cloud_115 ADD COLUMN transfer_directory VARCHAR(500) DEFAULT '';
		END IF;
	END $$;
	`

	_, err = db.Exec(alterCloudTableSQL)
	if err != nil {
		Error("Failed to alter cloud_115 table: %v", err)
		return err
	}

	// 添加115云账号表更新时间函数
	_, err = db.Exec(`CREATE OR REPLACE FUNCTION update_cloud115_timestamp() RETURNS TRIGGER AS $$ BEGIN NEW.update_time = CURRENT_TIMESTAMP; RETURN NEW; END; $$ LANGUAGE plpgsql;`)
	if err != nil {
		Error("Failed to create update_cloud115_timestamp function: %v", err)
		return err
	}

	// 检查并添加115云账号表更新时间触发器
	addCloudTriggerSQL := `
	DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = 't_cloud_115'::regclass AND tgname = 'update_cloud115_timestamp_trigger') THEN
			CREATE TRIGGER update_cloud115_timestamp_trigger
			BEFORE UPDATE ON t_cloud_115
			FOR EACH ROW
			EXECUTE FUNCTION update_cloud115_timestamp();
		END IF;
	END $$;
	`

	_, err = db.Exec(addCloudTriggerSQL)
	if err != nil {
		Error("Failed to add cloud115 update trigger: %v", err)
		return err
	}

	// 创建STRM配置表（如果不存在）
	createStrmConfigTableSQL := `
	CREATE TABLE IF NOT EXISTS t_strm_config (
		id SERIAL PRIMARY KEY,
		cloud115_id INTEGER NOT NULL,
		net_disk_path VARCHAR(255) NOT NULL,
		local_path VARCHAR(255) NOT NULL,
		cron VARCHAR(50) NOT NULL,
		extension TEXT DEFAULT '.strm',
		create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (cloud115_id) REFERENCES t_cloud_115(id)
	);
	`

	_, err = db.Exec(createStrmConfigTableSQL)
	if err != nil {
		Error("Failed to create strm_config table: %v", err)
		return err
	}

	// 为现有t_strm_config表添加缺失的字段
	alterStrmConfigTableSQL := `
DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'create_time') THEN
			ALTER TABLE t_strm_config ADD COLUMN create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'update_time') THEN
			ALTER TABLE t_strm_config ADD COLUMN update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'extension') THEN
			ALTER TABLE t_strm_config ADD COLUMN extension TEXT DEFAULT '.strm';
		END IF;
		-- 变更extension字段类型为TEXT（如果当前是VARCHAR类型）
		IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'extension' AND data_type = 'character varying') THEN
			ALTER TABLE t_strm_config ALTER COLUMN extension TYPE TEXT;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'cloud115_id') THEN
			ALTER TABLE t_strm_config ADD COLUMN cloud115_id INTEGER NOT NULL;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'net_disk_path') THEN
			ALTER TABLE t_strm_config ADD COLUMN net_disk_path VARCHAR(255) NOT NULL;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'local_path') THEN
			ALTER TABLE t_strm_config ADD COLUMN local_path VARCHAR(255) NOT NULL;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'dir_tree_file') THEN
			ALTER TABLE t_strm_config ADD COLUMN dir_tree_file TEXT DEFAULT '';
		END IF;
		-- 删除不需要的字段
		IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'name') THEN
			ALTER TABLE t_strm_config DROP COLUMN name;
		END IF;
		IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'status') THEN
			ALTER TABLE t_strm_config DROP COLUMN status;
		END IF;
		-- 添加外键约束
		IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 't_strm_config_cloud115_id_fkey') THEN
			ALTER TABLE t_strm_config ADD CONSTRAINT t_strm_config_cloud115_id_fkey FOREIGN KEY (cloud115_id) REFERENCES t_cloud_115(id);
		END IF;
END $$;
`

	_, err = db.Exec(alterStrmConfigTableSQL)
	if err != nil {
		Error("Failed to alter strm_config table: %v", err)
		return err
	}

	// 添加STRM配置表更新时间函数
	_, err = db.Exec(`CREATE OR REPLACE FUNCTION update_strm_config_timestamp() RETURNS TRIGGER AS $$ BEGIN NEW.update_time = CURRENT_TIMESTAMP; RETURN NEW; END; $$ LANGUAGE plpgsql;`)
	if err != nil {
		Error("Failed to create update_strm_config_timestamp function: %v", err)
		return err
	}

	// 检查并添加STRM配置表更新时间触发器
	addStrmConfigTriggerSQL := `
	DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = 't_strm_config'::regclass AND tgname = 'update_strm_config_timestamp_trigger') THEN
			CREATE TRIGGER update_strm_config_timestamp_trigger
			BEFORE UPDATE ON t_strm_config
			FOR EACH ROW
			EXECUTE FUNCTION update_strm_config_timestamp();
		END IF;
	END $$;
	`

	_, err = db.Exec(addStrmConfigTriggerSQL)
	if err != nil {
		Error("Failed to add strm_config update trigger: %v", err)
		return err
	}

	// 创建系统配置表（如果不存在）
	createSystemConfigTableSQL := `
	CREATE TABLE IF NOT EXISTS t_system_config (
		id SERIAL PRIMARY KEY,
		config_key VARCHAR(100) UNIQUE NOT NULL,
		config_val TEXT NOT NULL,
		create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err = db.Exec(createSystemConfigTableSQL)
	if err != nil {
		Error("Failed to create system_config table: %v", err)
		return err
	}

	// 添加系统配置表更新时间函数
	_, err = db.Exec(`CREATE OR REPLACE FUNCTION update_system_config_timestamp() RETURNS TRIGGER AS $$ BEGIN NEW.update_time = CURRENT_TIMESTAMP; RETURN NEW; END; $$ LANGUAGE plpgsql;`)
	if err != nil {
		Error("Failed to create update_system_config_timestamp function: %v", err)
		return err
	}

	// 检查并添加系统配置表更新时间触发器
	addSystemConfigTriggerSQL := `
	DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = 't_system_config'::regclass AND tgname = 'update_system_config_timestamp_trigger') THEN
			CREATE TRIGGER update_system_config_timestamp_trigger
			BEFORE UPDATE ON t_system_config
			FOR EACH ROW
			EXECUTE FUNCTION update_system_config_timestamp();
		END IF;
	END $$;
	`

	_, err = db.Exec(addSystemConfigTriggerSQL)
	if err != nil {
		Error("Failed to add system_config update trigger: %v", err)
		return err
	}

	// 初始化日志保留天数配置（如果不存在）
	_, err = db.Exec(`
		INSERT INTO t_system_config (config_key, config_val) 
		VALUES ('log_save_day_limit', '1') 
		ON CONFLICT (config_key) DO NOTHING
	`)
	if err != nil {
		Error("Failed to initialize log_save_day_limit config: %v", err)
		return err
	}

	Info("Database initialized successfully")
	return nil
}

// GetUserByID 根据ID获取用户
func GetUserByID(id int) (*User, error) {
	Debug("Getting user by ID: %d", id)
	user := &User{}
	err := db.QueryRow("SELECT id, name, password, create_time, update_time FROM t_user WHERE id = $1", id).Scan(&user.ID, &user.Name, &user.Password, &user.CreateTime, &user.UpdateTime)
	if err != nil {
		Error("Failed to get user by ID %d: %v", id, err)
		return nil, err
	}
	Debug("Found user by ID %d: %s", id, user.Name)
	return user, nil
}

// GetUserByName 根据用户名获取用户
func GetUserByName(name string) (*User, error) {
	Debug("Getting user by name: %s", name)
	user := &User{}
	err := db.QueryRow("SELECT id, name, password, create_time, update_time FROM t_user WHERE name = $1", name).Scan(&user.ID, &user.Name, &user.Password, &user.CreateTime, &user.UpdateTime)
	if err != nil {
		Debug("User not found by name: %s", name)
		return nil, err
	}
	Debug("Found user by name %s: ID %d", name, user.ID)
	return user, nil
}

// CreateUser 创建新用户
func CreateUser(name, password string) (*User, error) {
	Debug("Creating new user: %s", name)
	// 前端已经对密码进行了md5加密，直接存储
	user := &User{}
	err := db.QueryRow("INSERT INTO t_user (name, password) VALUES ($1, $2) RETURNING id, name, password, create_time, update_time", name, password).Scan(&user.ID, &user.Name, &user.Password, &user.CreateTime, &user.UpdateTime)
	if err != nil {
		Error("Failed to create user %s: %v", name, err)
		return nil, err
	}
	Info("Created new user: %s (ID: %d)", name, user.ID)
	return user, nil
}

// VerifyPassword 验证密码
func VerifyPassword(hashedPassword, password string) error {
	if hashedPassword != password {
		Debug("Password verification failed: passwords don't match")
		return fmt.Errorf("invalid password")
	}
	return nil
}

// GetCloud115ByID 根据ID获取115云账号
func GetCloud115ByID(id int) (*Cloud115, error) {
	Debug("Getting cloud_115 by ID: %d", id)
	cloud115 := &Cloud115{}
	err := db.QueryRow("SELECT id, name, cookie, refresh_token, access_token, expires_in, COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''), create_time, update_time FROM t_cloud_115 WHERE id = $1", id).Scan(
		&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken, &cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID, &cloud115.TransferDirectory, &cloud115.CreateTime, &cloud115.UpdateTime)
	if err != nil {
		Error("Failed to get cloud_115 by ID %d: %v", id, err)
		return nil, err
	}
	Debug("Found cloud_115 by ID %d: %s", id, cloud115.Name)
	return cloud115, nil
}

// GetCloud115ByName 根据名称获取115云账号
func GetCloud115ByName(name string) (*Cloud115, error) {
	Debug("Getting cloud_115 by name: %s", name)
	cloud115 := &Cloud115{}
	err := db.QueryRow("SELECT id, name, cookie, refresh_token, access_token, expires_in, COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''), create_time, update_time FROM t_cloud_115 WHERE name = $1", name).Scan(
		&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken, &cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID, &cloud115.TransferDirectory, &cloud115.CreateTime, &cloud115.UpdateTime)
	if err != nil {
		Debug("Cloud_115 not found by name: %s", name)
		return nil, err
	}
	Debug("Found cloud_115 by name %s: ID %d", name, cloud115.ID)
	return cloud115, nil
}

// GetAllCloud115 获取所有115云账号
func GetAllCloud115() ([]*Cloud115, error) {
	Debug("Getting all cloud_115 accounts")
	rows, err := db.Query("SELECT id, name, cookie, refresh_token, access_token, expires_in, COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''), create_time, update_time FROM t_cloud_115")
	if err != nil {
		Error("Failed to get all cloud_115 accounts: %v", err)
		return nil, err
	}
	defer rows.Close()

	var cloud115List []*Cloud115
	for rows.Next() {
		cloud115 := &Cloud115{}
		err := rows.Scan(&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken, &cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID, &cloud115.TransferDirectory, &cloud115.CreateTime, &cloud115.UpdateTime)
		if err != nil {
			Error("Failed to scan cloud_115 row: %v", err)
			return nil, err
		}
		cloud115List = append(cloud115List, cloud115)
	}

	if err = rows.Err(); err != nil {
		Error("Error iterating cloud_115 rows: %v", err)
		return nil, err
	}

	Debug("Found %d cloud_115 accounts", len(cloud115List))
	return cloud115List, nil
}

// CreateCloud115 创建115云账号
func CreateCloud115(name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string) (*Cloud115, error) {
	Debug("Creating new cloud_115 account: %s", name)
	cloud115 := &Cloud115{}
	err := db.QueryRow(
		"INSERT INTO t_cloud_115 (name, cookie, refresh_token, access_token, expires_in, transfer_account_id, transfer_directory) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, name, cookie, refresh_token, access_token, expires_in, COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''), create_time, update_time",
		name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory,
	).Scan(&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken, &cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID, &cloud115.TransferDirectory, &cloud115.CreateTime, &cloud115.UpdateTime)
	if err != nil {
		Error("Failed to create cloud_115 account %s: %v", name, err)
		return nil, err
	}
	Info("Created new cloud_115 account: %s (ID: %d)", name, cloud115.ID)
	return cloud115, nil
}

// UpdateCloud115 更新115云账号
func UpdateCloud115(id int, name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string) (*Cloud115, error) {
	Debug("Updating cloud_115 account with ID: %d", id)
	cloud115 := &Cloud115{}
	err := db.QueryRow(
		"UPDATE t_cloud_115 SET name = $1, cookie = $2, refresh_token = $3, access_token = $4, expires_in = $5, transfer_account_id = $6, transfer_directory = $7 WHERE id = $8 RETURNING id, name, cookie, refresh_token, access_token, expires_in, COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''), create_time, update_time",
		name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory, id,
	).Scan(&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken, &cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID, &cloud115.TransferDirectory, &cloud115.CreateTime, &cloud115.UpdateTime)
	if err != nil {
		Error("Failed to update cloud_115 account with ID %d: %v", id, err)
		return nil, err
	}
	Info("Updated cloud_115 account: %s (ID: %d)", cloud115.Name, cloud115.ID)
	return cloud115, nil
}

// DeleteCloud115 删除115云账号
func DeleteCloud115(id int) error {
	Debug("Deleting cloud_115 account with ID: %d", id)
	result, err := db.Exec("DELETE FROM t_cloud_115 WHERE id = $1", id)
	if err != nil {
		Error("Failed to delete cloud_115 account with ID %d: %v", id, err)
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		Error("Failed to get rows affected for delete operation: %v", err)
		return err
	}

	if rowsAffected == 0 {
		Debug("No cloud_115 account found with ID %d for deletion", id)
		return fmt.Errorf("no cloud_115 account found with ID %d", id)
	}

	Info("Deleted cloud_115 account with ID: %d", id)
	return nil
}

// GetStrmConfigByID 根据ID获取STRM配置
func GetStrmConfigByID(id int) (*StrmConfig, error) {
	Debug("Getting strm config by ID: %d", id)
	strmConfig := &StrmConfig{}
	err := db.QueryRow("SELECT id, cloud115_id, net_disk_path, local_path, cron, extension, COALESCE(dir_tree_file, ''), create_time, update_time FROM t_strm_config WHERE id = $1", id).Scan(
		&strmConfig.ID, &strmConfig.Cloud115Id, &strmConfig.NetDiskPath, &strmConfig.LocalPath, &strmConfig.Cron, &strmConfig.Extension, &strmConfig.DirTreeFile, &strmConfig.CreateTime, &strmConfig.UpdateTime)
	if err != nil {
		Error("Failed to get strm config by ID %d: %v", id, err)
		return nil, err
	}
	Debug("Found strm config by ID %d", id)
	return strmConfig, nil
}

// GetAllStrmConfig 获取所有STRM配置
func GetAllStrmConfig() ([]*StrmConfig, error) {
	Debug("Getting all strm configs")
	rows, err := db.Query("SELECT id, cloud115_id, net_disk_path, local_path, cron, extension, COALESCE(dir_tree_file, ''), create_time, update_time FROM t_strm_config")
	if err != nil {
		Error("Failed to get all strm configs: %v", err)
		return nil, err
	}
	defer rows.Close()

	var strmConfigList []*StrmConfig
	for rows.Next() {
		strmConfig := &StrmConfig{}
		err := rows.Scan(&strmConfig.ID, &strmConfig.Cloud115Id, &strmConfig.NetDiskPath, &strmConfig.LocalPath, &strmConfig.Cron, &strmConfig.Extension, &strmConfig.DirTreeFile, &strmConfig.CreateTime, &strmConfig.UpdateTime)
		if err != nil {
			Error("Failed to scan strm config row: %v", err)
			return nil, err
		}
		strmConfigList = append(strmConfigList, strmConfig)
	}

	if err = rows.Err(); err != nil {
		Error("Error iterating strm config rows: %v", err)
		return nil, err
	}

	Debug("Found %d strm configs", len(strmConfigList))
	return strmConfigList, nil
}

// CreateStrmConfig 创建STRM配置
func CreateStrmConfig(cloud115Id int, netDiskPath, localPath, cron, extension string) (*StrmConfig, error) {
	Debug("Creating new strm config")
	strmConfig := &StrmConfig{}

	err := db.QueryRow(
		"INSERT INTO t_strm_config (cloud115_id, net_disk_path, local_path, cron, extension) VALUES ($1, $2, $3, $4, $5) RETURNING id, cloud115_id, net_disk_path, local_path, cron, extension, create_time, update_time",
		cloud115Id, netDiskPath, localPath, cron, extension,
	).Scan(&strmConfig.ID, &strmConfig.Cloud115Id, &strmConfig.NetDiskPath, &strmConfig.LocalPath, &strmConfig.Cron, &strmConfig.Extension, &strmConfig.CreateTime, &strmConfig.UpdateTime)
	if err != nil {
		Error("Failed to create strm config: %v", err)
		return nil, err
	}
	Info("Created new strm config (ID: %d)", strmConfig.ID)
	return strmConfig, nil
}

// UpdateStrmConfig 更新STRM配置
func UpdateStrmConfig(id, cloud115Id int, netDiskPath, localPath, cron, extension string) (*StrmConfig, error) {
	Debug("Updating strm config with ID: %d", id)
	strmConfig := &StrmConfig{}

	err := db.QueryRow(
		"UPDATE t_strm_config SET cloud115_id = $1, net_disk_path = $2, local_path = $3, cron = $4, extension = $5 WHERE id = $6 RETURNING id, cloud115_id, net_disk_path, local_path, cron, extension, create_time, update_time",
		cloud115Id, netDiskPath, localPath, cron, extension, id,
	).Scan(&strmConfig.ID, &strmConfig.Cloud115Id, &strmConfig.NetDiskPath, &strmConfig.LocalPath, &strmConfig.Cron, &strmConfig.Extension, &strmConfig.CreateTime, &strmConfig.UpdateTime)
	if err != nil {
		Error("Failed to update strm config with ID %d: %v", id, err)
		return nil, err
	}
	Info("Updated strm config (ID: %d)", strmConfig.ID)
	return strmConfig, nil
}

// DeleteStrmConfig 删除STRM配置
func DeleteStrmConfig(id int) error {
	Debug("Deleting strm config with ID: %d", id)
	result, err := db.Exec("DELETE FROM t_strm_config WHERE id = $1", id)
	if err != nil {
		Error("Failed to delete strm config with ID %d: %v", id, err)
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		Error("Failed to get rows affected for delete operation: %v", err)
		return err
	}

	if rowsAffected == 0 {
		Debug("No strm config found with ID %d for deletion", id)
		return fmt.Errorf("no strm config found with ID %d", id)
	}

	Info("Deleted strm config with ID: %d", id)
	return nil
}

// GetSystemConfigByKey 根据配置键获取系统配置
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
