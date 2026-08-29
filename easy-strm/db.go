package main

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

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

	// 创建通知配置表，确保全新数据库无需手工执行历史迁移脚本即可使用通知模块。
	createNotificationConfigTableSQL := `
	CREATE TABLE IF NOT EXISTS t_notification_config (
		id SERIAL PRIMARY KEY,
		channel VARCHAR(20) NOT NULL UNIQUE,
		config JSONB NOT NULL DEFAULT '{}',
		enabled BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err = db.Exec(createNotificationConfigTableSQL)
	if err != nil {
		Error("Failed to create notification_config table: %v", err)
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
		task_name VARCHAR(100) NOT NULL,
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

	// task_name 仅用于展示；任务身份由 STRM 配置和任务类型共同确定。
	_, err = db.Exec(`ALTER TABLE t_cron_task DROP CONSTRAINT IF EXISTS t_cron_task_task_name_key`)
	if err != nil {
		Error("Failed to drop cron task name unique constraint: %v", err)
		return err
	}

	// 全量任务名称使用 STRM 配置 ID，保持展示名称稳定并避免同名目录产生歧义。
	_, err = db.Exec(`
		UPDATE t_cron_task
		SET task_name = 'STRM全量生成-' || strm_config_id::text
		WHERE task_type = 'full_generate'
		  AND task_name IS DISTINCT FROM 'STRM全量生成-' || strm_config_id::text
	`)
	if err != nil {
		Error("Failed to normalize full generate cron task names: %v", err)
		return err
	}
	_, err = db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_cron_task_config_type
		ON t_cron_task(strm_config_id, task_type)
	`)
	if err != nil {
		Error("Failed to create cron task identity index: %v", err)
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
			('电影（官方）', 'movie', '{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if year %} ({{ year }}){% endif %}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}', true),
			('电影（简洁）', 'movie', '{{ title }}{{ fileExt }}', true),
			('剧集（官方）', 'tv', '{{ title }}{% if year %} ({{ year }}){% endif %}/Season {{ "%02d"|format(season|int) }}/{{ title }} - S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}', true),
			('剧集（简洁）', 'tv', '{{ title }}/S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{{ fileExt }}', true)
		) AS v(name, media_type, template, enabled)
		WHERE NOT EXISTS (SELECT 1 FROM t_rename_preset WHERE name = v.name)
	`)
	if err != nil {
		Warn("Failed to insert default rename presets: %v", err)
	}

	_, err = db.Exec(`
		UPDATE t_system_config
		SET config_val = CASE
			WHEN config_key = 'movie_naming_template' AND config_val = '{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if en_title and en_title != title %} - {{ en_title }}{% endif %}{% if year %} ({{ year }}){% endif %}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}'
				THEN '{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if year %} ({{ year }}){% endif %}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}'
			WHEN config_key = 'tv_naming_template' AND config_val = '{{ title }}{% if year %} ({{ year }}){% endif %}/Season {{ "%02d"|format(season|int) }}/{{ title }}{% if en_title and en_title != title %} - {{ en_title }}{% endif %} - S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}'
				THEN '{{ title }}{% if year %} ({{ year }}){% endif %}/Season {{ "%02d"|format(season|int) }}/{{ title }} - S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}'
			ELSE config_val
		END,
		update_time = CURRENT_TIMESTAMP
		WHERE config_key IN ('movie_naming_template', 'tv_naming_template')
	`)
	if err != nil {
		Warn("Failed to migrate legacy naming templates in system config: %v", err)
	}

	_, err = db.Exec(`
		UPDATE t_rename_preset
		SET template = CASE
			WHEN media_type = 'movie' AND template = '{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if en_title and en_title != title %} - {{ en_title }}{% endif %}{% if year %} ({{ year }}){% endif %}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}'
				THEN '{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if year %} ({{ year }}){% endif %}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}'
			WHEN media_type = 'tv' AND template = '{{ title }}{% if year %} ({{ year }}){% endif %}/Season {{ "%02d"|format(season|int) }}/{{ title }}{% if en_title and en_title != title %} - {{ en_title }}{% endif %} - S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}'
				THEN '{{ title }}{% if year %} ({{ year }}){% endif %}/Season {{ "%02d"|format(season|int) }}/{{ title }} - S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}'
			ELSE template
		END,
		update_time = CURRENT_TIMESTAMP
		WHERE name IN ('电影（官方）', '剧集（官方）')
	`)
	if err != nil {
		Warn("Failed to migrate legacy naming templates in presets: %v", err)
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

	// ============================================
	// V9: 媒体源增加整理目的地目录字段
	// ============================================
	alterMediaSourceTargetPathSQL := `
DO $$ BEGIN
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'organize_target_path') THEN
		ALTER TABLE t_media_source ADD COLUMN organize_target_path VARCHAR(1000) DEFAULT '';
	END IF;
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'watch_path') THEN
		ALTER TABLE t_media_source ADD COLUMN watch_path VARCHAR(1000) DEFAULT '';
	END IF;
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'media_type') THEN
		ALTER TABLE t_media_source ADD COLUMN media_type VARCHAR(20) DEFAULT 'all';
	END IF;
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'conflict_policy') THEN
		ALTER TABLE t_media_source ADD COLUMN conflict_policy VARCHAR(20) DEFAULT 'skip';
	END IF;
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'operation_mode') THEN
		ALTER TABLE t_media_source ADD COLUMN operation_mode VARCHAR(20) DEFAULT 'move';
	END IF;
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'auto_organize') THEN
		ALTER TABLE t_media_source ADD COLUMN auto_organize BOOLEAN DEFAULT FALSE;
	END IF;
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'watch_enabled') THEN
		ALTER TABLE t_media_source ADD COLUMN watch_enabled BOOLEAN DEFAULT FALSE;
	END IF;
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'watch_interval') THEN
		ALTER TABLE t_media_source ADD COLUMN watch_interval INTEGER DEFAULT 1800;
	END IF;
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'emby_library_id') THEN
		ALTER TABLE t_media_source ADD COLUMN emby_library_id VARCHAR(100) DEFAULT '';
	END IF;
END $$;
`
	_, err = db.Exec(alterMediaSourceTargetPathSQL)
	if err != nil {
		Error("Failed to add organize_target_path to t_media_source: %v", err)
		return err
	}

	_, err = db.Exec(`COMMENT ON COLUMN t_media_source.organize_target_path IS '整理目的地目录（根目录），如 /已整理'`)
	if err != nil {
		Warn("Failed to add comment for organize_target_path: %v", err)
	}
	_, err = db.Exec(`COMMENT ON COLUMN t_media_source.watch_path IS '监控目录；115 自动监控时使用该目录而不是源路径'`)
	if err != nil {
		Warn("Failed to add comment for watch_path: %v", err)
	}

	_, err = db.Exec(`COMMENT ON COLUMN t_media_source.media_type IS '自动整理默认媒体类型（all/movie/tv）'`)
	if err != nil {
		Warn("Failed to add comment for media_type: %v", err)
	}
	_, err = db.Exec(`COMMENT ON COLUMN t_media_source.conflict_policy IS '自动整理默认冲突策略（skip/overwrite/suffix）'`)
	if err != nil {
		Warn("Failed to add comment for conflict_policy: %v", err)
	}
	_, err = db.Exec(`COMMENT ON COLUMN t_media_source.operation_mode IS '自动整理默认操作方式（move/copy/hardlink/symlink）'`)
	if err != nil {
		Warn("Failed to add comment for operation_mode: %v", err)
	}
	_, err = db.Exec(`COMMENT ON COLUMN t_media_source.auto_organize IS '是否在监控到新增文件后自动整理'`)
	if err != nil {
		Warn("Failed to add comment for auto_organize: %v", err)
	}
	_, err = db.Exec(`COMMENT ON COLUMN t_media_source.watch_enabled IS '是否开启媒体源监控'`)
	if err != nil {
		Warn("Failed to add comment for watch_enabled: %v", err)
	}
	_, err = db.Exec(`COMMENT ON COLUMN t_media_source.watch_interval IS '监控轮询间隔（秒）'`)
	if err != nil {
		Warn("Failed to add comment for watch_interval: %v", err)
	}
	_, err = db.Exec(`COMMENT ON COLUMN t_media_source.emby_library_id IS '关联的 Emby 媒体库 ID'`)
	if err != nil {
		Warn("Failed to add comment for emby_library_id: %v", err)
	}

	// ============================================
	// V10: 识别结果缓存表
	// ============================================
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS t_identify_cache (
		    id              SERIAL PRIMARY KEY,
		    file_hash       VARCHAR(64) NOT NULL,
		    file_name       VARCHAR(500) NOT NULL,
		    media_type      VARCHAR(20) NOT NULL,
		    tmdb_id         INTEGER,
		    title           VARCHAR(255),
		    original_title  VARCHAR(255),
		    year            INTEGER,
		    season_number   INTEGER DEFAULT 0,
		    episode_number  INTEGER DEFAULT 0,
		    poster_path     VARCHAR(500),
		    is_manual       BOOLEAN DEFAULT FALSE,
		    source_id       INTEGER,
		    created_at      TIMESTAMP DEFAULT NOW(),
		    updated_at      TIMESTAMP DEFAULT NOW()
		);
	`)
	if err != nil {
		Error("Failed to create t_identify_cache: %v", err)
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_identify_cache_file_hash ON t_identify_cache(file_hash)`)
	if err != nil {
		Warn("Failed to create idx_identify_cache_file_hash: %v", err)
	}
	_, err = db.Exec(`
		WITH duplicated_rows AS (
			SELECT id
			FROM (
				SELECT id,
				       ROW_NUMBER() OVER (
				           PARTITION BY file_hash
				           ORDER BY is_manual DESC, updated_at DESC, created_at DESC, id DESC
				       ) AS row_num
				FROM t_identify_cache
			) ranked
			WHERE ranked.row_num > 1
		)
		DELETE FROM t_identify_cache
		WHERE id IN (SELECT id FROM duplicated_rows)
	`)
	if err != nil {
		Warn("Failed to cleanup duplicated identify cache rows: %v", err)
	}
	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_identify_cache_file_hash ON t_identify_cache(file_hash)`)
	if err != nil {
		Warn("Failed to create uq_identify_cache_file_hash: %v", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_identify_cache_is_manual ON t_identify_cache(is_manual)`)
	if err != nil {
		Warn("Failed to create idx_identify_cache_is_manual: %v", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_identify_cache_created_at ON t_identify_cache(created_at)`)
	if err != nil {
		Warn("Failed to create idx_identify_cache_created_at: %v", err)
	}

	// ============================================
	// V15: 115云下载（离线下载）任务记录表
	// ============================================
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS t_offline_download_task (
		    id            BIGSERIAL PRIMARY KEY,
		    task_id       VARCHAR(64)  NOT NULL DEFAULT '',
		    cloud115_id   INTEGER      NOT NULL,
		    url           TEXT         NOT NULL,
		    info_hash     VARCHAR(64)  NOT NULL DEFAULT '',
		    name          TEXT         NOT NULL DEFAULT '',
		    size          BIGINT       NOT NULL DEFAULT 0,
		    status        VARCHAR(32)  NOT NULL DEFAULT 'pending',
		    percent       DOUBLE PRECISION NOT NULL DEFAULT 0,
		    error_message TEXT         NOT NULL DEFAULT '',
		    save_dir_id   VARCHAR(64)  NOT NULL DEFAULT '',
		    create_time   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
		    update_time   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		Error("Failed to create t_offline_download_task: %v", err)
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_offline_download_task_task_id ON t_offline_download_task(task_id)`)
	if err != nil {
		Warn("Failed to create idx_offline_download_task_task_id: %v", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_offline_download_task_account_status ON t_offline_download_task(cloud115_id, status)`)
	if err != nil {
		Warn("Failed to create idx_offline_download_task_account_status: %v", err)
	}

	Info("Database initialized successfully")
	return nil
}
