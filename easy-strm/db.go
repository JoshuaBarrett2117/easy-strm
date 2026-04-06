package main

import (
	"database/sql"
	"fmt"
	"path/filepath"
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
	ID                int        `json:"id"`
	Name              string     `json:"name"`
	Cookie            string     `json:"cookie"`
	RefreshToken      string     `json:"refresh_token"`
	AccessToken       string     `json:"access_token"`
	ExpiresIn         int        `json:"expires_in"`
	TransferAccountID int        `json:"transfer_account_id"`
	TransferDirectory string     `json:"transfer_directory"`
	AccountType       string     `json:"account_type"`
	QuotaUsed         int64      `json:"quota_used"`
	Priority          int        `json:"priority"`
	Status            string     `json:"status"`
	CoolingStartTime  *time.Time `json:"cooling_start_time"`
	TransferMethod    string     `json:"transfer_method"`
	AlistUrl          string     `json:"alist_url"`
	AlistToken        string     `json:"alist_token"`
	CreateTime        time.Time  `json:"create_time"`
	UpdateTime        time.Time  `json:"update_time"`
}

// StrmConfig STRM文件配置信息
type StrmConfig struct {
	ID               int       `json:"id"`
	Cloud115Id       int       `json:"cloud115_id"`       // 115账号ID
	NetDiskPath      string    `json:"net_disk_path"`     // 网盘目录
	LocalPath        string    `json:"local_path"`        // 本地目录
	Cron             string    `json:"cron"`              // cron表达式
	Extension        string    `json:"extension"`         // STRM文件后缀名
	DirTreeFile      string    `json:"dir_tree_file"`     // 本地目录树文件路径
	SyncMode         string    `json:"sync_mode"`         // 同步模式: manual/instant/cron
	SourceAccount    int       `json:"source_account"`    // 源账号ID
	TargetAccount    int       `json:"target_account"`    // 目标账号ID
	TargetDirectory  string    `json:"target_directory"`  // 目标目录
	AutoCleanup      bool      `json:"auto_cleanup"`      // 自动清理
	CleanupThreshold int       `json:"cleanup_threshold"` // 清理阈值
	CleanupPolicy    string    `json:"cleanup_policy"`    // 清理策略
	MaxConcurrency   int       `json:"max_concurrency"`   // 最大并发数
	CreateTime       time.Time `json:"create_time"`
	UpdateTime       time.Time `json:"update_time"`
}

// SystemConfig 系统配置信息
type SystemConfig struct {
	ID         int       `json:"id"`
	ConfigKey  string    `json:"config_key"` // 配置键
	ConfigVal  string    `json:"config_val"` // 配置值
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}

// StrmFile 已生成的STRM文件记录
type StrmFile struct {
	ID            int       `json:"id"`
	StrmConfigID  int       `json:"strm_config_id"`  // 关联的STRM配置ID
	FileName      string    `json:"file_name"`       // 文件名
	FilePath      string    `json:"file_path"`       // 相对路径
	PickCode      string    `json:"pick_code"`       // 115文件pickcode
	Sha1          string    `json:"sha1"`            // 文件SHA1
	FileSize      int64     `json:"file_size"`       // 文件大小
	LocalStrmPath string    `json:"local_strm_path"` // 本地STRM文件路径
	CreateTime    time.Time `json:"create_time"`
	UpdateTime    time.Time `json:"update_time"`
}

// CronTask 定时任务配置
type CronTask struct {
	ID             int        `json:"id"`
	TaskName       string     `json:"task_name"`        // 任务名：账号id+"增量更新任务"
	TaskType       string     `json:"task_type"`        // 任务类型：incremental_sync
	Cloud115ID     int        `json:"cloud115_id"`      // 关联的115账号ID
	StrmConfigID   int        `json:"strm_config_id"`   // 关联的STRM配置ID
	CronExpr       string     `json:"cron_expr"`        // cron表达式
	Status         string     `json:"status"`           // enabled/disabled
	LastRunTime    *time.Time `json:"last_run_time"`    // 上次执行时间
	NextRunTime    *time.Time `json:"next_run_time"`    // 下次执行时间
	LastRunStatus  string     `json:"last_run_status"`  // 上次执行状态
	LastRunMessage string     `json:"last_run_message"` // 上次执行消息
	CreateTime     time.Time  `json:"create_time"`
	UpdateTime     time.Time  `json:"update_time"`
}

var db *sql.DB

func InitDB(config *Config) error {
	// 构建数据库连接字符串
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable client_encoding=UTF8",
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
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cloud_115' AND column_name = 'account_type') THEN
			ALTER TABLE t_cloud_115 ADD COLUMN account_type VARCHAR(20) DEFAULT 'resource';
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cloud_115' AND column_name = 'quota_used') THEN
			ALTER TABLE t_cloud_115 ADD COLUMN quota_used BIGINT DEFAULT 0;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cloud_115' AND column_name = 'priority') THEN
			ALTER TABLE t_cloud_115 ADD COLUMN priority INTEGER DEFAULT 5;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cloud_115' AND column_name = 'status') THEN
			ALTER TABLE t_cloud_115 ADD COLUMN status VARCHAR(20) DEFAULT 'active';
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cloud_115' AND column_name = 'cooling_start_time') THEN
			ALTER TABLE t_cloud_115 ADD COLUMN cooling_start_time TIMESTAMP;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cloud_115' AND column_name = 'transfer_method') THEN
			ALTER TABLE t_cloud_115 ADD COLUMN transfer_method VARCHAR(50) DEFAULT '';
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cloud_115' AND column_name = 'alist_url') THEN
			ALTER TABLE t_cloud_115 ADD COLUMN alist_url VARCHAR(500);
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cloud_115' AND column_name = 'alist_token') THEN
			ALTER TABLE t_cloud_115 ADD COLUMN alist_token VARCHAR(255);
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
		-- V3.0 新增字段
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'sync_mode') THEN
			ALTER TABLE t_strm_config ADD COLUMN sync_mode VARCHAR(20) DEFAULT 'manual';
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'source_account') THEN
			ALTER TABLE t_strm_config ADD COLUMN source_account INT DEFAULT 0;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'target_account') THEN
			ALTER TABLE t_strm_config ADD COLUMN target_account INT DEFAULT 0;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'target_directory') THEN
			ALTER TABLE t_strm_config ADD COLUMN target_directory VARCHAR(500);
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'auto_cleanup') THEN
			ALTER TABLE t_strm_config ADD COLUMN auto_cleanup BOOLEAN DEFAULT FALSE;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'cleanup_threshold') THEN
			ALTER TABLE t_strm_config ADD COLUMN cleanup_threshold INT DEFAULT 0;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'cleanup_policy') THEN
			ALTER TABLE t_strm_config ADD COLUMN cleanup_policy VARCHAR(50);
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_config' AND column_name = 'max_concurrency') THEN
			ALTER TABLE t_strm_config ADD COLUMN max_concurrency INT DEFAULT 1;
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

	// 初始化 TMDB API Key 配置（如果不存在）
	// 业务背景：TMDB 识别功能需要 API Key，默认为空，用户需自行配置
	_, err = db.Exec(`
		INSERT INTO t_system_config (config_key, config_val)
		VALUES ('tmdb_api_key', '')
		ON CONFLICT (config_key) DO NOTHING
	`)
	if err != nil {
		Error("Failed to initialize tmdb_api_key config: %v", err)
		return err
	}

	// 创建STRM文件记录表（如果不存在）
	createStrmFileTableSQL := `
	CREATE TABLE IF NOT EXISTS t_strm_file (
		id SERIAL PRIMARY KEY,
		strm_config_id INTEGER NOT NULL,
		file_name VARCHAR(500) NOT NULL,
		file_path VARCHAR(1000) NOT NULL,
		pick_code VARCHAR(50),
		sha1 VARCHAR(40),
		file_size BIGINT,
		local_strm_path VARCHAR(1000),
		create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (strm_config_id) REFERENCES t_strm_config(id) ON DELETE CASCADE
	);
	`
	_, err = db.Exec(createStrmFileTableSQL)
	if err != nil {
		Error("Failed to create strm_file table: %v", err)
		return err
	}

	// 为t_strm_file表添加缺失的字段
	alterStrmFileTableSQL := `
	DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_file' AND column_name = 'create_time') THEN
			ALTER TABLE t_strm_file ADD COLUMN create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_strm_file' AND column_name = 'update_time') THEN
			ALTER TABLE t_strm_file ADD COLUMN update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
		END IF;
	END $$;
	`
	_, err = db.Exec(alterStrmFileTableSQL)
	if err != nil {
		Error("Failed to alter strm_file table: %v", err)
		return err
	}

	// 修改 sha1 字段长度以存储完整路径
	_, err = db.Exec(`ALTER TABLE t_strm_file ALTER COLUMN sha1 TYPE VARCHAR(1000)`)
	if err != nil {
		Warn("Failed to alter sha1 column length: %v", err)
	}

	// 创建STRM文件记录表的唯一索引（strm_config_id + file_path）
	_, err = db.Exec(`
		DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_strm_file_config_path') THEN
				CREATE UNIQUE INDEX idx_strm_file_config_path ON t_strm_file(strm_config_id, file_path);
			END IF;
		END $$;
	`)
	if err != nil {
		Error("Failed to create strm_file index: %v", err)
		return err
	}

	// 创建定时任务表（如果不存在）
	createCronTaskTableSQL := `
	CREATE TABLE IF NOT EXISTS t_cron_task (
		id SERIAL PRIMARY KEY,
		task_name VARCHAR(100) UNIQUE NOT NULL,
		task_type VARCHAR(50) NOT NULL,
		cloud115_id INTEGER NOT NULL,
		strm_config_id INTEGER NOT NULL,
		cron_expr VARCHAR(50) NOT NULL,
		status VARCHAR(20) DEFAULT 'enabled',
		last_run_time TIMESTAMP,
		next_run_time TIMESTAMP,
		last_run_status VARCHAR(20),
		last_run_message TEXT,
		create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (cloud115_id) REFERENCES t_cloud_115(id) ON DELETE CASCADE,
		FOREIGN KEY (strm_config_id) REFERENCES t_strm_config(id) ON DELETE CASCADE
	);
	`
	_, err = db.Exec(createCronTaskTableSQL)
	if err != nil {
		Error("Failed to create cron_task table: %v", err)
		return err
	}

	// 为t_cron_task表添加缺失的字段
	alterCronTaskTableSQL := `
	DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cron_task' AND column_name = 'create_time') THEN
			ALTER TABLE t_cron_task ADD COLUMN create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cron_task' AND column_name = 'update_time') THEN
			ALTER TABLE t_cron_task ADD COLUMN update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_cron_task' AND column_name = 'last_run_message') THEN
			ALTER TABLE t_cron_task ADD COLUMN last_run_message TEXT;
		END IF;
	END $$;
	`
	_, err = db.Exec(alterCronTaskTableSQL)
	if err != nil {
		Error("Failed to alter cron_task table: %v", err)
		return err
	}

	// 添加定时任务表更新时间函数
	_, err = db.Exec(`CREATE OR REPLACE FUNCTION update_cron_task_timestamp() RETURNS TRIGGER AS $$ BEGIN NEW.update_time = CURRENT_TIMESTAMP; RETURN NEW; END; $$ LANGUAGE plpgsql;`)
	if err != nil {
		Error("Failed to create update_cron_task_timestamp function: %v", err)
		return err
	}

	// 检查并添加定时任务表更新时间触发器
	addCronTaskTriggerSQL := `
	DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = 't_cron_task'::regclass AND tgname = 'update_cron_task_timestamp_trigger') THEN
			CREATE TRIGGER update_cron_task_timestamp_trigger
			BEFORE UPDATE ON t_cron_task
			FOR EACH ROW
			EXECUTE FUNCTION update_cron_task_timestamp();
		END IF;
	END $$;
	`
	_, err = db.Exec(addCronTaskTriggerSQL)
	if err != nil {
		Error("Failed to add cron_task update trigger: %v", err)
		return err
	}

	// ============================================
	// MediaManager 模块表（v6）
	// ============================================

	// 创建媒体源配置表
	createMediaSourceTableSQL := `
	CREATE TABLE IF NOT EXISTS t_media_source (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		source_type VARCHAR(20) NOT NULL,
		path VARCHAR(500) NOT NULL,
		cloud115_id INTEGER,
		priority INT DEFAULT 10,
		enabled BOOLEAN DEFAULT TRUE,
		create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (cloud115_id) REFERENCES t_cloud_115(id) ON DELETE SET NULL
	);
	`
	_, err = db.Exec(createMediaSourceTableSQL)
	if err != nil {
		Error("Failed to create t_media_source table: %v", err)
		return err
	}

	// 创建媒体源表索引
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_media_source_type ON t_media_source(source_type)`)
	if err != nil {
		Warn("Failed to create idx_media_source_type: %v", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_media_source_enabled ON t_media_source(enabled)`)
	if err != nil {
		Warn("Failed to create idx_media_source_enabled: %v", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_media_source_priority ON t_media_source(priority)`)
	if err != nil {
		Warn("Failed to create idx_media_source_priority: %v", err)
	}

	// 创建 TMDB 缓存表
	createTmdbCacheTableSQL := `
	CREATE TABLE IF NOT EXISTS t_tmdb_cache (
		id SERIAL PRIMARY KEY,
		query_key VARCHAR(500) NOT NULL,
		media_type VARCHAR(20) NOT NULL,
		tmdb_id INTEGER NOT NULL,
		title VARCHAR(500),
		original_title VARCHAR(500),
		year INTEGER,
		poster_path VARCHAR(500),
		overview TEXT,
		vote_average DECIMAL(3,1),
		release_date VARCHAR(20),
		first_air_date VARCHAR(20),
		season_number INTEGER,
		episode_number INTEGER,
		raw_data JSONB,
		expire_at TIMESTAMP NOT NULL,
		create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (query_key, media_type)
	);
	`
	_, err = db.Exec(createTmdbCacheTableSQL)
	if err != nil {
		Error("Failed to create t_tmdb_cache table: %v", err)
		return err
	}

	// 创建 TMDB 缓存表索引
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tmdb_cache_query_key ON t_tmdb_cache(query_key)`)
	if err != nil {
		Warn("Failed to create idx_tmdb_cache_query_key: %v", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tmdb_cache_tmdb_id ON t_tmdb_cache(tmdb_id)`)
	if err != nil {
		Warn("Failed to create idx_tmdb_cache_tmdb_id: %v", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tmdb_cache_expire_at ON t_tmdb_cache(expire_at)`)
	if err != nil {
		Warn("Failed to create idx_tmdb_cache_expire_at: %v", err)
	}

	// 创建更名预设表
	createRenamePresetTableSQL := `
	CREATE TABLE IF NOT EXISTS t_rename_preset (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		media_type VARCHAR(20) NOT NULL,
		template VARCHAR(500) NOT NULL,
		enabled BOOLEAN DEFAULT TRUE,
		create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err = db.Exec(createRenamePresetTableSQL)
	if err != nil {
		Error("Failed to create t_rename_preset table: %v", err)
		return err
	}

	// 创建更名预设表索引
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_rename_preset_media_type ON t_rename_preset(media_type)`)
	if err != nil {
		Warn("Failed to create idx_rename_preset_media_type: %v", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_rename_preset_enabled ON t_rename_preset(enabled)`)
	if err != nil {
		Warn("Failed to create idx_rename_preset_enabled: %v", err)
	}

	// 插入默认更名预设（如果不存在）
	_, err = db.Exec(`
		INSERT INTO t_rename_preset (name, media_type, template, enabled)
		SELECT * FROM (VALUES
			('电影（官方）', 'movie', '{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if en_title and en_title != title %} - {{ en_title }}{% endif %}{% if year %} ({{ year }}){% endif %}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}', true),
			('电影（简洁）', 'movie', '{{ title }}{{ fileExt }}', true),
			('剧集（官方）', 'tv', '{{ title }}{% if year %} ({{ year }}){% endif %}/Season {{ "%02d"|format(season|int) }}/{{ title }}{% if en_title and en_title != title %} - {{ en_title }}{% endif %} - S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}', true),
			('剧集（简洁）', 'tv', '{{ title }}/S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{{ fileExt }}', true)
		) AS v(name, media_type, template, enabled)
		WHERE NOT EXISTS (SELECT 1 FROM t_rename_preset WHERE name = v.name)
	`)
	if err != nil {
		Warn("Failed to insert default rename presets: %v", err)
	}

	// 创建媒体分类策略表
	createMediaCategoryTableSQL := `
	CREATE TABLE IF NOT EXISTS t_media_category (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		media_type VARCHAR(20) NOT NULL,
		target_path VARCHAR(1000) NOT NULL,
		match_rules JSONB,
		enabled BOOLEAN DEFAULT TRUE,
		create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err = db.Exec(createMediaCategoryTableSQL)
	if err != nil {
		Error("Failed to create t_media_category table: %v", err)
		return err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_media_category_type ON t_media_category(media_type)`)
	if err != nil {
		Warn("Failed to create idx_media_category_type: %v", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_media_category_enabled ON t_media_category(enabled)`)
	if err != nil {
		Warn("Failed to create idx_media_category_enabled: %v", err)
	}

	// 初始化图中分类策略；default=true 的未分类只在其他规则未命中后兜底。
	_, err = db.Exec(`
		WITH defaults(name, media_type, target_path, match_rules, enabled) AS (
			VALUES
			('动画电影', 'movie', '/电影/动画电影', '{"genre_ids":[16]}'::jsonb, true),
			('华语电影', 'movie', '/电影/华语电影', '{"languages":["zh","cn"]}'::jsonb, true),
			('外语电影', 'movie', '/电影/外语电影', '{"languages":["en","ja","ko","fr","de","es","it","ru","nl","pt","th","hi"]}'::jsonb, true),
			('未分类', 'movie', '/电影/未分类', '{"default":true}'::jsonb, true),
			('国漫', 'tv', '/电视剧/国漫', '{"genre_ids":[16],"countries":["CN","TW","HK"]}'::jsonb, true),
			('日番', 'tv', '/电视剧/日番', '{"genre_ids":[16],"countries":["JP"]}'::jsonb, true),
			('纪录片', 'tv', '/电视剧/纪录片', '{"genre_ids":[99]}'::jsonb, true),
			('儿童', 'tv', '/电视剧/儿童', '{"genre_ids":[10762]}'::jsonb, true),
			('综艺', 'tv', '/电视剧/综艺', '{"genre_ids":[10764,10767]}'::jsonb, true),
			('国产剧', 'tv', '/电视剧/国产剧', '{"countries":["CN","TW","HK"]}'::jsonb, true),
			('欧美剧', 'tv', '/电视剧/欧美剧', '{"countries":["US","FR","GB","UK","DE","ES","IT","NL","PT","RU"]}'::jsonb, true),
			('日韩剧', 'tv', '/电视剧/日韩剧', '{"countries":["JP","KP","KR","TH","IN","SG"]}'::jsonb, true),
			('未分类', 'tv', '/电视剧/未分类', '{"default":true}'::jsonb, true)
		),
		updated AS (
			UPDATE t_media_category c
			SET target_path = d.target_path,
				match_rules = d.match_rules,
				enabled = d.enabled,
				update_time = NOW()
			FROM defaults d
			WHERE c.name = d.name AND c.media_type = d.media_type
			RETURNING c.name, c.media_type
		)
		INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled)
		SELECT name, media_type, target_path, match_rules, enabled FROM defaults v
		WHERE NOT EXISTS (
			SELECT 1 FROM t_media_category
			WHERE name = v.name AND media_type = v.media_type
		)
	`)
	if err != nil {
		Warn("Failed to insert default media categories: %v", err)
	}

	// 创建媒体文件缓存表
	createMediaFileCacheTableSQL := `
	CREATE TABLE IF NOT EXISTS t_media_file_cache (
		id SERIAL PRIMARY KEY,
		source_id INTEGER NOT NULL,
		file_path VARCHAR(1000) NOT NULL,
		file_name VARCHAR(500) NOT NULL,
		file_size BIGINT,
		sha1 VARCHAR(40),
		tmdb_id INTEGER,
		media_type VARCHAR(20),
		season_number INTEGER,
		episode_number INTEGER,
		tmdb_data JSONB,
		identified_at TIMESTAMP,
		create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (source_id) REFERENCES t_media_source(id) ON DELETE CASCADE,
		UNIQUE (source_id, file_path)
	);
	`
	_, err = db.Exec(createMediaFileCacheTableSQL)
	if err != nil {
		Error("Failed to create t_media_file_cache table: %v", err)
		return err
	}

	// 创建媒体文件缓存表索引
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_media_file_cache_source_id ON t_media_file_cache(source_id)`)
	if err != nil {
		Warn("Failed to create idx_media_file_cache_source_id: %v", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_media_file_cache_tmdb_id ON t_media_file_cache(tmdb_id)`)
	if err != nil {
		Warn("Failed to create idx_media_file_cache_tmdb_id: %v", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_media_file_cache_media_type ON t_media_file_cache(media_type)`)
	if err != nil {
		Warn("Failed to create idx_media_file_cache_media_type: %v", err)
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

// VerifyPassword 验证密码（前端已发送MD5哈希值，直接比较）
func VerifyPassword(hashedPassword, password string) error {
	if hashedPassword != password {
		Debug("Password verification failed: hashedPassword=%s, inputPassword=%s", hashedPassword, password)
		return fmt.Errorf("invalid password")
	}
	return nil
}

// GetCloud115ByID 根据ID获取115云账号
func GetCloud115ByID(id int) (*Cloud115, error) {
	Debug("Getting cloud_115 by ID: %d", id)
	cloud115 := &Cloud115{}
	err := db.QueryRow(`SELECT id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time,
		COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time FROM t_cloud_115 WHERE id = $1`, id).Scan(
		&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken,
		&cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID,
		&cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed,
		&cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime,
		&cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken,
		&cloud115.CreateTime, &cloud115.UpdateTime)
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
	err := db.QueryRow(`SELECT id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time,
		COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time FROM t_cloud_115 WHERE name = $1`, name).Scan(
		&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken,
		&cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID,
		&cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed,
		&cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime,
		&cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken,
		&cloud115.CreateTime, &cloud115.UpdateTime)
	if err != nil {
		Debug("Cloud_115 not found by name: %s", name)
		return nil, err
	}
	Debug("Found cloud_115 by name %s: ID %d", name, cloud115.ID)
	return cloud115, nil
}

// GetAllCloud115 获取所有115云账号
func GetAllCloud115(sortField, sortOrder string) ([]*Cloud115, error) {
	Debug("Getting all cloud_115 accounts with sort: %s %s", sortField, sortOrder)

	if sortField == "" {
		sortField = "id"
	}
	if sortOrder == "" {
		sortOrder = "asc"
	}

	query := fmt.Sprintf(`SELECT id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time,
		COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time FROM t_cloud_115 ORDER BY %s %s`, sortField, sortOrder)
	rows, err := db.Query(query)
	if err != nil {
		Error("Failed to get all cloud_115 accounts: %v", err)
		return nil, err
	}
	defer rows.Close()

	var cloud115List []*Cloud115
	for rows.Next() {
		cloud115 := &Cloud115{}
		err := rows.Scan(&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken,
			&cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID,
			&cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed,
			&cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime,
			&cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken,
			&cloud115.CreateTime, &cloud115.UpdateTime)
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
func CreateCloud115(name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, transferMethod string, alistUrl string, alistToken string) (*Cloud115, error) {
	Debug("Creating new cloud_115 account: %s", name)
	if accountType == "" {
		accountType = "resource"
	}
	if priority == 0 {
		priority = 5
	}
	if transferMethod == "" {
		transferMethod = "115driver"
	}
	cloud115 := &Cloud115{}
	err := db.QueryRow(
		`INSERT INTO t_cloud_115 (name, cookie, refresh_token, access_token, expires_in, transfer_account_id, transfer_directory, account_type, priority, status, transfer_method, alist_url, alist_token)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active', $10, $11, $12)
		RETURNING id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time, COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time`,
		name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory, accountType, priority, transferMethod, alistUrl, alistToken,
	).Scan(&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken, &cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID, &cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed, &cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime, &cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken, &cloud115.CreateTime, &cloud115.UpdateTime)
	if err != nil {
		Error("Failed to create cloud_115 account %s: %v", name, err)
		return nil, err
	}
	Info("Created new cloud_115 account: %s (ID: %d)", name, cloud115.ID)
	return cloud115, nil
}

// UpdateCloud115 更新115云账号
func UpdateCloud115(id int, name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, status string, transferMethod string, alistUrl string, alistToken string) (*Cloud115, error) {
	Debug("Updating cloud_115 account with ID: %d", id)
	cloud115 := &Cloud115{}
	err := db.QueryRow(
		`UPDATE t_cloud_115 SET name = $1, cookie = $2, refresh_token = $3, access_token = $4, expires_in = $5, transfer_account_id = $6, transfer_directory = $7, account_type = $8, priority = $9, status = $10, transfer_method = $11, alist_url = $12, alist_token = $13 WHERE id = $14
		RETURNING id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time, COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time`,
		name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory, accountType, priority, status, transferMethod, alistUrl, alistToken, id,
	).Scan(&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken, &cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID, &cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed, &cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime, &cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken, &cloud115.CreateTime, &cloud115.UpdateTime)
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
	err := db.QueryRow(`SELECT id, cloud115_id, net_disk_path, local_path, cron, extension,
		COALESCE(dir_tree_file, ''), COALESCE(sync_mode, 'manual'), COALESCE(source_account, 0),
		COALESCE(target_account, 0), COALESCE(target_directory, ''), auto_cleanup,
		COALESCE(cleanup_threshold, 0), COALESCE(cleanup_policy, ''), COALESCE(max_concurrency, 1),
		create_time, update_time FROM t_strm_config WHERE id = $1`, id).Scan(
		&strmConfig.ID, &strmConfig.Cloud115Id, &strmConfig.NetDiskPath, &strmConfig.LocalPath,
		&strmConfig.Cron, &strmConfig.Extension, &strmConfig.DirTreeFile, &strmConfig.SyncMode,
		&strmConfig.SourceAccount, &strmConfig.TargetAccount, &strmConfig.TargetDirectory,
		&strmConfig.AutoCleanup, &strmConfig.CleanupThreshold, &strmConfig.CleanupPolicy,
		&strmConfig.MaxConcurrency, &strmConfig.CreateTime, &strmConfig.UpdateTime)
	if err != nil {
		Error("Failed to get strm config by ID %d: %v", id, err)
		return nil, err
	}
	Debug("Found strm config by ID %d", id)
	return strmConfig, nil
}

// GetAllStrmConfig 获取所有STRM配置
func GetAllStrmConfig(sortField, sortOrder string) ([]*StrmConfig, error) {
	Debug("Getting all strm configs with sort: %s %s", sortField, sortOrder)

	if sortField == "" {
		sortField = "id"
	}
	if sortOrder == "" {
		sortOrder = "asc"
	}

	query := fmt.Sprintf(`SELECT id, cloud115_id, net_disk_path, local_path, cron, extension,
		COALESCE(dir_tree_file, ''), COALESCE(sync_mode, 'manual'), COALESCE(source_account, 0),
		COALESCE(target_account, 0), COALESCE(target_directory, ''), auto_cleanup,
		COALESCE(cleanup_threshold, 0), COALESCE(cleanup_policy, ''), COALESCE(max_concurrency, 1),
		create_time, update_time FROM t_strm_config ORDER BY %s %s`, sortField, sortOrder)
	rows, err := db.Query(query)
	if err != nil {
		Error("Failed to get all strm configs: %v", err)
		return nil, err
	}
	defer rows.Close()

	var strmConfigList []*StrmConfig
	for rows.Next() {
		strmConfig := &StrmConfig{}
		err := rows.Scan(&strmConfig.ID, &strmConfig.Cloud115Id, &strmConfig.NetDiskPath, &strmConfig.LocalPath,
			&strmConfig.Cron, &strmConfig.Extension, &strmConfig.DirTreeFile, &strmConfig.SyncMode,
			&strmConfig.SourceAccount, &strmConfig.TargetAccount, &strmConfig.TargetDirectory,
			&strmConfig.AutoCleanup, &strmConfig.CleanupThreshold, &strmConfig.CleanupPolicy,
			&strmConfig.MaxConcurrency, &strmConfig.CreateTime, &strmConfig.UpdateTime)
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
func CreateStrmConfig(cloud115Id int, netDiskPath, localPath, cron, extension string, syncMode string, sourceAccount, targetAccount int, targetDirectory string, autoCleanup bool, cleanupThreshold int, cleanupPolicy string, maxConcurrency int) (*StrmConfig, error) {
	Debug("Creating new strm config")
	strmConfig := &StrmConfig{}
	if syncMode == "" {
		syncMode = "manual"
	}
	if maxConcurrency == 0 {
		maxConcurrency = 1
	}
	err := db.QueryRow(
		`INSERT INTO t_strm_config (cloud115_id, net_disk_path, local_path, cron, extension, sync_mode, source_account, target_account, target_directory, auto_cleanup, cleanup_threshold, cleanup_policy, max_concurrency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, cloud115_id, net_disk_path, local_path, cron, extension, COALESCE(dir_tree_file, ''), sync_mode,
		source_account, target_account, target_directory, auto_cleanup, cleanup_threshold, cleanup_policy, max_concurrency, create_time, update_time`,
		cloud115Id, netDiskPath, localPath, cron, extension, syncMode, sourceAccount, targetAccount, targetDirectory, autoCleanup, cleanupThreshold, cleanupPolicy, maxConcurrency,
	).Scan(&strmConfig.ID, &strmConfig.Cloud115Id, &strmConfig.NetDiskPath, &strmConfig.LocalPath, &strmConfig.Cron, &strmConfig.Extension, &strmConfig.DirTreeFile, &strmConfig.SyncMode, &strmConfig.SourceAccount, &strmConfig.TargetAccount, &strmConfig.TargetDirectory, &strmConfig.AutoCleanup, &strmConfig.CleanupThreshold, &strmConfig.CleanupPolicy, &strmConfig.MaxConcurrency, &strmConfig.CreateTime, &strmConfig.UpdateTime)
	if err != nil {
		Error("Failed to create strm config: %v", err)
		return nil, err
	}
	Info("Created new strm config (ID: %d)", strmConfig.ID)

	if cron != "" {
		taskName := fmt.Sprintf("STRM全量生成-%s", filepath.Base(netDiskPath))
		cronTask, err := CreateCronTask(taskName, "full_generate", cloud115Id, strmConfig.ID, cron)
		if err != nil {
			Warn("Failed to create cron task for strm config: %v", err)
		} else {
			Info("Created cron task (ID: %d) for strm config (ID: %d)", cronTask.ID, strmConfig.ID)
			if scheduler != nil {
				if err := scheduler.AddTask(cronTask); err != nil {
					Warn("Failed to add cron task to scheduler: %v", err)
				}
			}
		}
	}

	return strmConfig, nil
}

// UpdateStrmConfig 更新STRM配置
func UpdateStrmConfig(id, cloud115Id int, netDiskPath, localPath, cron, extension string, syncMode string, sourceAccount, targetAccount int, targetDirectory string, autoCleanup bool, cleanupThreshold int, cleanupPolicy string, maxConcurrency int) (*StrmConfig, error) {
	Debug("Updating strm config with ID: %d", id)
	strmConfig := &StrmConfig{}
	if syncMode == "" {
		syncMode = "manual"
	}
	if maxConcurrency == 0 {
		maxConcurrency = 1
	}
	err := db.QueryRow(
		`UPDATE t_strm_config SET cloud115_id = $1, net_disk_path = $2, local_path = $3, cron = $4, extension = $5, sync_mode = $6, source_account = $7, target_account = $8, target_directory = $9, auto_cleanup = $10, cleanup_threshold = $11, cleanup_policy = $12, max_concurrency = $13 WHERE id = $14
		RETURNING id, cloud115_id, net_disk_path, local_path, cron, extension, COALESCE(dir_tree_file, ''), sync_mode, source_account, target_account, target_directory, auto_cleanup, cleanup_threshold, cleanup_policy, max_concurrency, create_time, update_time`,
		cloud115Id, netDiskPath, localPath, cron, extension, syncMode, sourceAccount, targetAccount, targetDirectory, autoCleanup, cleanupThreshold, cleanupPolicy, maxConcurrency, id,
	).Scan(&strmConfig.ID, &strmConfig.Cloud115Id, &strmConfig.NetDiskPath, &strmConfig.LocalPath, &strmConfig.Cron, &strmConfig.Extension, &strmConfig.DirTreeFile, &strmConfig.SyncMode, &strmConfig.SourceAccount, &strmConfig.TargetAccount, &strmConfig.TargetDirectory, &strmConfig.AutoCleanup, &strmConfig.CleanupThreshold, &strmConfig.CleanupPolicy, &strmConfig.MaxConcurrency, &strmConfig.CreateTime, &strmConfig.UpdateTime)
	if err != nil {
		Error("Failed to update strm config with ID %d: %v", id, err)
		return nil, err
	}
	Info("Updated strm config (ID: %d)", strmConfig.ID)

	existingTask, _ := GetCronTaskByStrmConfigID(strmConfig.ID)

	if cron != "" {
		taskName := fmt.Sprintf("STRM全量生成-%s", filepath.Base(netDiskPath))
		if existingTask != nil {
			_, err = UpdateCronTask(existingTask.ID, taskName, "full_generate", cron, existingTask.Status)
			if err != nil {
				Warn("Failed to update cron task: %v", err)
			} else {
				if scheduler != nil {
					updatedTask, _ := GetCronTaskByID(existingTask.ID)
					if updatedTask != nil {
						if err := scheduler.UpdateTask(updatedTask); err != nil {
							Warn("Failed to update cron task in scheduler: %v", err)
						} else {
							// 重新获取任务数据，因为 scheduler.UpdateTask 会更新 next_run_time
							if finalTask, err := GetCronTaskByID(existingTask.ID); err == nil {
								Debug("Cron task next run time updated: %v", finalTask.NextRunTime)
							}
						}
					}
				}
			}
		} else {
			cronTask, err := CreateCronTask(taskName, "full_generate", cloud115Id, strmConfig.ID, cron)
			if err != nil {
				Warn("Failed to create cron task: %v", err)
			} else {
				Info("Created cron task (ID: %d) for strm config(ID: %d)", cronTask.ID, strmConfig.ID)
				if scheduler != nil {
					if err := scheduler.AddTask(cronTask); err != nil {
						Warn("Failed to add cron task to scheduler: %v", err)
					}
				}
			}
		}
	} else {
		if existingTask != nil {
			if scheduler != nil {
				scheduler.RemoveTask(existingTask.ID)
			}
			err := DeleteCronTaskByName(existingTask.TaskName)
			if err != nil {
				Warn("Failed to delete cron task: %v", err)
			}
		}
	}

	return strmConfig, nil
}

// DeleteStrmConfig 删除STRM配置
func DeleteStrmConfig(id int) error {
	Debug("Deleting strm config with ID: %d", id)

	existingTask, _ := GetCronTaskByStrmConfigID(id)
	if existingTask != nil {
		if scheduler != nil {
			scheduler.RemoveTask(existingTask.ID)
		}
		err := DeleteCronTaskByName(existingTask.TaskName)
		if err != nil {
			Warn("Failed to delete cron task: %v", err)
		}
	}

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

// ========== StrmFile CRUD ==========

// GetStrmFileByID 根据ID获取STRM文件记录
func GetStrmFileByID(id int) (*StrmFile, error) {
	Debug("Getting strm file by ID: %d", id)
	strmFile := &StrmFile{}
	err := db.QueryRow("SELECT id, strm_config_id, file_name, file_path, COALESCE(pick_code, ''), COALESCE(sha1, ''), COALESCE(file_size, 0), COALESCE(local_strm_path, ''), create_time, update_time FROM t_strm_file WHERE id = $1", id).Scan(
		&strmFile.ID, &strmFile.StrmConfigID, &strmFile.FileName, &strmFile.FilePath, &strmFile.PickCode, &strmFile.Sha1, &strmFile.FileSize, &strmFile.LocalStrmPath, &strmFile.CreateTime, &strmFile.UpdateTime)
	if err != nil {
		Error("Failed to get strm file by ID %d: %v", id, err)
		return nil, err
	}
	return strmFile, nil
}

// GetStrmFilesByConfigID 根据STRM配置ID获取所有STRM文件记录
func GetStrmFilesByConfigID(strmConfigID int) ([]*StrmFile, error) {
	Debug("Getting strm files by config ID: %d", strmConfigID)
	rows, err := db.Query("SELECT id, strm_config_id, file_name, file_path, COALESCE(pick_code, ''), COALESCE(sha1, ''), COALESCE(file_size, 0), COALESCE(local_strm_path, ''), create_time, update_time FROM t_strm_file WHERE strm_config_id = $1", strmConfigID)
	if err != nil {
		Error("Failed to get strm files by config ID %d: %v", strmConfigID, err)
		return nil, err
	}
	defer rows.Close()

	var strmFileList []*StrmFile
	for rows.Next() {
		strmFile := &StrmFile{}
		err := rows.Scan(&strmFile.ID, &strmFile.StrmConfigID, &strmFile.FileName, &strmFile.FilePath, &strmFile.PickCode, &strmFile.Sha1, &strmFile.FileSize, &strmFile.LocalStrmPath, &strmFile.CreateTime, &strmFile.UpdateTime)
		if err != nil {
			Error("Failed to scan strm file row: %v", err)
			return nil, err
		}
		strmFileList = append(strmFileList, strmFile)
	}

	return strmFileList, nil
}

// GetStrmFileByPath 根据配置ID和文件路径获取STRM文件记录
func GetStrmFileByPath(strmConfigID int, filePath string) (*StrmFile, error) {
	Debug("Getting strm file by config ID %d and path: %s", strmConfigID, filePath)
	strmFile := &StrmFile{}
	err := db.QueryRow("SELECT id, strm_config_id, file_name, file_path, COALESCE(pick_code, ''), COALESCE(sha1, ''), COALESCE(file_size, 0), COALESCE(local_strm_path, ''), create_time, update_time FROM t_strm_file WHERE strm_config_id = $1 AND file_path = $2", strmConfigID, filePath).Scan(
		&strmFile.ID, &strmFile.StrmConfigID, &strmFile.FileName, &strmFile.FilePath, &strmFile.PickCode, &strmFile.Sha1, &strmFile.FileSize, &strmFile.LocalStrmPath, &strmFile.CreateTime, &strmFile.UpdateTime)
	if err != nil {
		return nil, err
	}
	return strmFile, nil
}

// CreateStrmFile 创建STRM文件记录
func CreateStrmFile(strmConfigID int, fileName, filePath, pickCode, sha1 string, fileSize int64, localStrmPath string) (*StrmFile, error) {
	Debug("Creating strm file: %s", fileName)
	strmFile := &StrmFile{}
	err := db.QueryRow(
		"INSERT INTO t_strm_file (strm_config_id, file_name, file_path, pick_code, sha1, file_size, local_strm_path) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, strm_config_id, file_name, file_path, pick_code, sha1, file_size, local_strm_path, create_time, update_time",
		strmConfigID, fileName, filePath, pickCode, sha1, fileSize, localStrmPath,
	).Scan(&strmFile.ID, &strmFile.StrmConfigID, &strmFile.FileName, &strmFile.FilePath, &strmFile.PickCode, &strmFile.Sha1, &strmFile.FileSize, &strmFile.LocalStrmPath, &strmFile.CreateTime, &strmFile.UpdateTime)
	if err != nil {
		Error("Failed to create strm file: %v", err)
		return nil, err
	}
	Info("Created strm file: %s (ID: %d)", fileName, strmFile.ID)
	return strmFile, nil
}

// UpsertStrmFile 创建或更新STRM文件记录
func UpsertStrmFile(strmConfigID int, fileName, filePath, pickCode, sha1 string, fileSize int64, localStrmPath string) (*StrmFile, error) {
	Debug("Upserting strm file: %s", fileName)
	strmFile := &StrmFile{}
	err := db.QueryRow(`
		INSERT INTO t_strm_file (strm_config_id, file_name, file_path, pick_code, sha1, file_size, local_strm_path) 
		VALUES ($1, $2, $3, $4, $5, $6, $7) 
		ON CONFLICT (strm_config_id, file_path) DO UPDATE SET file_name = $2, pick_code = $4, sha1 = $5, file_size = $6, local_strm_path = $7
		RETURNING id, strm_config_id, file_name, file_path, pick_code, sha1, file_size, local_strm_path, create_time, update_time
	`, strmConfigID, fileName, filePath, pickCode, sha1, fileSize, localStrmPath).Scan(&strmFile.ID, &strmFile.StrmConfigID, &strmFile.FileName, &strmFile.FilePath, &strmFile.PickCode, &strmFile.Sha1, &strmFile.FileSize, &strmFile.LocalStrmPath, &strmFile.CreateTime, &strmFile.UpdateTime)
	if err != nil {
		Error("Failed to upsert strm file: %v", err)
		return nil, err
	}
	return strmFile, nil
}

// DeleteStrmFile 删除STRM文件记录
func DeleteStrmFile(id int) error {
	Debug("Deleting strm file with ID: %d", id)
	_, err := db.Exec("DELETE FROM t_strm_file WHERE id = $1", id)
	if err != nil {
		Error("Failed to delete strm file with ID %d: %v", id, err)
		return err
	}
	return nil
}

// DeleteStrmFileByPath 根据配置ID和文件路径删除STRM文件记录
func DeleteStrmFileByPath(strmConfigID int, filePath string) error {
	Debug("Deleting strm file by config ID %d and path: %s", strmConfigID, filePath)
	_, err := db.Exec("DELETE FROM t_strm_file WHERE strm_config_id = $1 AND file_path = $2", strmConfigID, filePath)
	if err != nil {
		Error("Failed to delete strm file: %v", err)
		return err
	}
	return nil
}

// DeleteStrmFilesByConfigID 删除指定配置的所有STRM文件记录
func DeleteStrmFilesByConfigID(strmConfigID int) error {
	Debug("Deleting all strm files for config ID: %d", strmConfigID)
	_, err := db.Exec("DELETE FROM t_strm_file WHERE strm_config_id = $1", strmConfigID)
	if err != nil {
		Error("Failed to delete strm files for config ID %d: %v", strmConfigID, err)
		return err
	}
	return nil
}

// CountStrmFilesByConfigID 统计指定配置的STRM文件数量
func CountStrmFilesByConfigID(strmConfigID int) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM t_strm_file WHERE strm_config_id = $1", strmConfigID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ========== CronTask CRUD ==========

// GetCronTaskByID 根据ID获取定时任务
func GetCronTaskByID(id int) (*CronTask, error) {
	Debug("Getting cron task by ID: %d", id)
	task := &CronTask{}
	err := db.QueryRow("SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status, last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''), create_time, update_time FROM t_cron_task WHERE id = $1", id).Scan(
		&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID, &task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime, &task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
	if err != nil {
		Error("Failed to get cron task by ID %d: %v", id, err)
		return nil, err
	}
	return task, nil
}

// GetCronTaskByName 根据任务名获取定时任务
func GetCronTaskByName(taskName string) (*CronTask, error) {
	Debug("Getting cron task by name: %s", taskName)
	task := &CronTask{}
	err := db.QueryRow("SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status, last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''), create_time, update_time FROM t_cron_task WHERE task_name = $1", taskName).Scan(
		&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID, &task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime, &task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
	if err != nil {
		return nil, err
	}
	return task, nil
}

// GetCronTaskByStrmConfigID 根据STRM配置ID获取定时任务
func GetCronTaskByStrmConfigID(strmConfigID int) (*CronTask, error) {
	Debug("Getting cron task by strm config ID: %d", strmConfigID)
	task := &CronTask{}
	err := db.QueryRow("SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status, last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''), create_time, update_time FROM t_cron_task WHERE strm_config_id = $1", strmConfigID).Scan(
		&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID, &task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime, &task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
	if err != nil {
		return nil, err
	}
	return task, nil
}

// GetAllCronTasks 获取所有定时任务
func GetAllCronTasks() ([]*CronTask, error) {
	Debug("Getting all cron tasks")
	rows, err := db.Query("SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status, last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''), create_time, update_time FROM t_cron_task ORDER BY id")
	if err != nil {
		Error("Failed to get all cron tasks: %v", err)
		return nil, err
	}
	defer rows.Close()

	var taskList []*CronTask
	for rows.Next() {
		task := &CronTask{}
		err := rows.Scan(&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID, &task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime, &task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
		if err != nil {
			Error("Failed to scan cron task row: %v", err)
			return nil, err
		}
		taskList = append(taskList, task)
	}

	return taskList, nil
}

// GetEnabledCronTasks 获取所有启用的定时任务
func GetEnabledCronTasks() ([]*CronTask, error) {
	Debug("Getting enabled cron tasks")
	rows, err := db.Query("SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status, last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''), create_time, update_time FROM t_cron_task WHERE status = 'enabled'")
	if err != nil {
		Error("Failed to get enabled cron tasks: %v", err)
		return nil, err
	}
	defer rows.Close()

	var taskList []*CronTask
	for rows.Next() {
		task := &CronTask{}
		err := rows.Scan(&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID, &task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime, &task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
		if err != nil {
			Error("Failed to scan cron task row: %v", err)
			return nil, err
		}
		taskList = append(taskList, task)
	}

	return taskList, nil
}

// CreateCronTask 创建定时任务
func CreateCronTask(taskName, taskType string, cloud115ID, strmConfigID int, cronExpr string) (*CronTask, error) {
	Debug("Creating cron task: %s", taskName)
	task := &CronTask{}
	err := db.QueryRow(
		"INSERT INTO t_cron_task (task_name, task_type, cloud115_id, strm_config_id, cron_expr, status) VALUES ($1, $2, $3, $4, $5, 'enabled') RETURNING id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status, last_run_time, next_run_time, last_run_status, last_run_message, create_time, update_time",
		taskName, taskType, cloud115ID, strmConfigID, cronExpr,
	).Scan(&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID, &task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime, &task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
	if err != nil {
		Error("Failed to create cron task: %v", err)
		return nil, err
	}
	Info("Created cron task: %s (ID: %d)", taskName, task.ID)
	return task, nil
}

// UpdateCronTask 更新定时任务
func UpdateCronTask(id int, taskName, taskType, cronExpr, status string) (*CronTask, error) {
	Debug("Updating cron task with ID: %d", id)
	task := &CronTask{}
	err := db.QueryRow(
		"UPDATE t_cron_task SET task_name = $1, task_type = $2, cron_expr = $3, status = $4 WHERE id = $5 RETURNING id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status, last_run_time, next_run_time, last_run_status, last_run_message, create_time, update_time",
		taskName, taskType, cronExpr, status, id,
	).Scan(&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID, &task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime, &task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
	if err != nil {
		Error("Failed to update cron task with ID %d: %v", id, err)
		return nil, err
	}
	Info("Updated cron task (ID: %d)", task.ID)
	return task, nil
}

// UpdateCronTaskRunInfo 更新定时任务执行信息
func UpdateCronTaskRunInfo(id int, lastRunTime, nextRunTime *time.Time, lastRunStatus, lastRunMessage string) error {
	Debug("Updating cron task run info for ID: %d", id)
	_, err := db.Exec(
		"UPDATE t_cron_task SET last_run_time = $1, next_run_time = $2, last_run_status = $3, last_run_message = $4 WHERE id = $5",
		lastRunTime, nextRunTime, lastRunStatus, lastRunMessage, id,
	)
	if err != nil {
		Error("Failed to update cron task run info: %v", err)
		return err
	}
	return nil
}

// DeleteCronTask 删除定时任务
func DeleteCronTask(id int) error {
	Debug("Deleting cron task with ID: %d", id)
	result, err := db.Exec("DELETE FROM t_cron_task WHERE id = $1", id)
	if err != nil {
		Error("Failed to delete cron task with ID %d: %v", id, err)
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no cron task found with ID %d", id)
	}

	Info("Deleted cron task with ID: %d", id)
	return nil
}

// DeleteCronTaskByName 根据任务名删除定时任务
func DeleteCronTaskByName(taskName string) error {
	Debug("Deleting cron task by name: %s", taskName)
	_, err := db.Exec("DELETE FROM t_cron_task WHERE task_name = $1", taskName)
	if err != nil {
		Error("Failed to delete cron task by name: %v", err)
		return err
	}
	return nil
}

// NotificationConfig 通知配置信息
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
func getDBInstance() *sql.DB {
	return db
}
