-- easy-strm 数据库基线（版本 1）。
-- 由第 0 阶段（dc23fb3）启动代码 InitDB（db.go 内联 DDL + *_migration.go 嵌入脚本）在空库上实际执行后的结构导出，
-- 去掉 schema 限定名以便在任意 schema 下执行；末尾为当时启动写入的默认数据。
-- 已有生产库（存在业务表但无版本表）不会执行本文件，只校验结构完整后标记为已应用。
-- 不要修改本文件；结构变更请新增 NNNNN_xxx.sql 迁移。
-- t_media_category 默认数据按旧 db.go 中定义的顺序插入（旧代码的 INSERT…SELECT 在空库上实际插入顺序与定义不一致，分类按 id 顺序匹配，以定义顺序为准）。

CREATE FUNCTION update_cloud115_timestamp() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN NEW.update_time = CURRENT_TIMESTAMP; RETURN NEW; END; $$;

CREATE FUNCTION update_cron_task_timestamp() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN NEW.update_time = CURRENT_TIMESTAMP; RETURN NEW; END; $$;

CREATE FUNCTION update_strm_config_timestamp() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN NEW.update_time = CURRENT_TIMESTAMP; RETURN NEW; END; $$;

CREATE FUNCTION update_system_config_timestamp() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN NEW.update_time = CURRENT_TIMESTAMP; RETURN NEW; END; $$;

CREATE FUNCTION update_user_timestamp() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN NEW.update_time = CURRENT_TIMESTAMP; RETURN NEW; END; $$;

CREATE TABLE t_cloud_115 (
    id integer NOT NULL,
    name character varying(50) NOT NULL,
    cookie text,
    cookie_source character varying(100) DEFAULT ''::character varying NOT NULL,
    refresh_token text,
    access_token text,
    expires_in integer,
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    transfer_account_id integer DEFAULT 0,
    transfer_directory character varying(500) DEFAULT ''::character varying,
    account_type character varying(20) DEFAULT 'resource'::character varying,
    quota_used bigint DEFAULT 0,
    priority integer DEFAULT 5,
    status character varying(20) DEFAULT 'active'::character varying,
    cooling_start_time timestamp without time zone,
    transfer_method character varying(50) DEFAULT ''::character varying,
    alist_url character varying(500),
    alist_token character varying(255)
);

CREATE SEQUENCE t_cloud_115_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_cloud_115_id_seq OWNED BY t_cloud_115.id;

CREATE TABLE t_cron_task (
    id integer NOT NULL,
    task_name character varying(100) NOT NULL,
    task_type character varying(50) NOT NULL,
    cloud115_id integer,
    strm_config_id integer,
    cron_expr character varying(50) NOT NULL,
    status character varying(20) DEFAULT 'enabled'::character varying,
    last_run_time timestamp without time zone,
    next_run_time timestamp without time zone,
    last_run_status character varying(20),
    last_run_message text,
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    task_key text,
    handler text DEFAULT ''::text NOT NULL,
    params jsonb DEFAULT '{}'::jsonb NOT NULL,
    timezone text DEFAULT 'Local'::text NOT NULL,
    builtin boolean DEFAULT false NOT NULL
);

CREATE SEQUENCE t_cron_task_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_cron_task_id_seq OWNED BY t_cron_task.id;

CREATE TABLE t_cron_task_run (
    id bigint NOT NULL,
    cron_task_id integer,
    task_id text NOT NULL,
    trigger_type text NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    ended_at timestamp with time zone,
    status text NOT NULL,
    message text DEFAULT ''::text NOT NULL
);

CREATE SEQUENCE t_cron_task_run_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_cron_task_run_id_seq OWNED BY t_cron_task_run.id;

CREATE TABLE t_emby_monitor_item (
    server_id integer NOT NULL,
    item_id character varying(200) NOT NULL,
    item_type character varying(50) DEFAULT ''::character varying NOT NULL,
    name character varying(500) DEFAULT ''::character varying NOT NULL,
    series_name character varying(500) DEFAULT ''::character varying NOT NULL,
    production_year integer DEFAULT 0 NOT NULL,
    image_tag character varying(300) DEFAULT ''::character varying NOT NULL,
    date_created timestamp with time zone NOT NULL,
    first_seen_at timestamp with time zone NOT NULL,
    is_baseline boolean DEFAULT false NOT NULL
);

CREATE TABLE t_emby_monitor_state (
    server_id integer NOT NULL,
    baseline_completed_at timestamp with time zone,
    last_session_poll_at timestamp with time zone,
    last_item_poll_at timestamp with time zone,
    plugin_available boolean DEFAULT false NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    update_time timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE t_emby_playback_event (
    id bigint NOT NULL,
    server_id integer NOT NULL,
    session_id character varying(200) NOT NULL,
    user_id character varying(200) DEFAULT ''::character varying NOT NULL,
    user_name character varying(300) DEFAULT ''::character varying NOT NULL,
    item_id character varying(200) NOT NULL,
    item_type character varying(50) DEFAULT ''::character varying NOT NULL,
    item_name character varying(500) DEFAULT ''::character varying NOT NULL,
    series_id character varying(200) DEFAULT ''::character varying NOT NULL,
    series_name character varying(500) DEFAULT ''::character varying NOT NULL,
    image_tag character varying(300) DEFAULT ''::character varying NOT NULL,
    client_name character varying(300) DEFAULT ''::character varying NOT NULL,
    device_name character varying(300) DEFAULT ''::character varying NOT NULL,
    playback_method character varying(100) DEFAULT ''::character varying NOT NULL,
    started_at timestamp with time zone NOT NULL,
    ended_at timestamp with time zone,
    watched_seconds bigint DEFAULT 0 NOT NULL,
    last_seen_at timestamp with time zone NOT NULL,
    is_paused boolean DEFAULT false NOT NULL
);

CREATE SEQUENCE t_emby_playback_event_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_emby_playback_event_id_seq OWNED BY t_emby_playback_event.id;

CREATE TABLE t_emby_server (
    id integer NOT NULL,
    name character varying(100) NOT NULL,
    base_url character varying(1000) NOT NULL,
    api_key text DEFAULT ''::text NOT NULL,
    proxy_port integer DEFAULT 0 NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT ck_emby_server_proxy_port CHECK (((proxy_port >= 0) AND (proxy_port <= 65535)))
);

CREATE SEQUENCE t_emby_server_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_emby_server_id_seq OWNED BY t_emby_server.id;

CREATE TABLE t_identify_cache (
    id integer NOT NULL,
    file_hash character varying(64) NOT NULL,
    file_name character varying(500) NOT NULL,
    media_type character varying(20) NOT NULL,
    tmdb_id integer,
    title character varying(255),
    original_title character varying(255),
    year integer,
    season_number integer DEFAULT 0,
    episode_number integer DEFAULT 0,
    poster_path character varying(500),
    is_manual boolean DEFAULT false,
    source_id integer,
    created_at timestamp without time zone DEFAULT now(),
    updated_at timestamp without time zone DEFAULT now(),
    recognition_method character varying(20) DEFAULT 'source'::character varying NOT NULL,
    metadata_source character varying(20) DEFAULT ''::character varying NOT NULL,
    metadata_id character varying(255) DEFAULT ''::character varying NOT NULL,
    metadata_provider character varying(100) DEFAULT ''::character varying NOT NULL,
    ai_used boolean DEFAULT false NOT NULL,
    ai_scene character varying(50) DEFAULT ''::character varying NOT NULL,
    failure_reason text DEFAULT ''::text NOT NULL
);

CREATE SEQUENCE t_identify_cache_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_identify_cache_id_seq OWNED BY t_identify_cache.id;

CREATE TABLE t_media_category (
    id integer NOT NULL,
    name character varying(100) NOT NULL,
    media_type character varying(20) NOT NULL,
    target_path character varying(1000) NOT NULL,
    match_rules jsonb,
    enabled boolean DEFAULT true,
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE SEQUENCE t_media_category_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_media_category_id_seq OWNED BY t_media_category.id;

CREATE TABLE t_media_file_cache (
    id integer NOT NULL,
    source_id integer NOT NULL,
    file_path character varying(1000) NOT NULL,
    file_name character varying(500) NOT NULL,
    file_size bigint,
    sha1 character varying(40),
    tmdb_id integer,
    media_type character varying(20),
    season_number integer,
    episode_number integer,
    tmdb_data jsonb,
    identified_at timestamp without time zone,
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE SEQUENCE t_media_file_cache_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_media_file_cache_id_seq OWNED BY t_media_file_cache.id;

CREATE TABLE t_media_source (
    id integer NOT NULL,
    name character varying(100) NOT NULL,
    source_type character varying(20) NOT NULL,
    path character varying(500) NOT NULL,
    cloud115_id integer,
    priority integer DEFAULT 10,
    enabled boolean DEFAULT true,
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    organize_target_path character varying(1000) DEFAULT ''::character varying,
    watch_path character varying(1000) DEFAULT ''::character varying,
    media_type character varying(20) DEFAULT 'all'::character varying,
    metadata_source character varying(20) DEFAULT 'auto'::character varying NOT NULL,
    conflict_policy character varying(20) DEFAULT 'skip'::character varying,
    operation_mode character varying(20) DEFAULT 'move'::character varying,
    auto_organize boolean DEFAULT false,
    watch_enabled boolean DEFAULT false,
    watch_interval integer DEFAULT 1800,
    emby_library_id character varying(100) DEFAULT ''::character varying,
    emby_server_id integer
);

CREATE SEQUENCE t_media_source_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_media_source_id_seq OWNED BY t_media_source.id;

CREATE TABLE t_notification_config (
    id integer NOT NULL,
    channel character varying(20) NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    enabled boolean DEFAULT false,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE SEQUENCE t_notification_config_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_notification_config_id_seq OWNED BY t_notification_config.id;

CREATE TABLE t_offline_download_task (
    id bigint NOT NULL,
    task_id character varying(64) DEFAULT ''::character varying NOT NULL,
    cloud115_id integer NOT NULL,
    url text NOT NULL,
    info_hash character varying(64) DEFAULT ''::character varying NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    size bigint DEFAULT 0 NOT NULL,
    status character varying(32) DEFAULT 'pending'::character varying NOT NULL,
    percent double precision DEFAULT 0 NOT NULL,
    error_message text DEFAULT ''::text NOT NULL,
    save_dir_id character varying(64) DEFAULT ''::character varying NOT NULL,
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE SEQUENCE t_offline_download_task_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_offline_download_task_id_seq OWNED BY t_offline_download_task.id;

CREATE TABLE t_rename_preset (
    id integer NOT NULL,
    name character varying(100) NOT NULL,
    media_type character varying(20) NOT NULL,
    template character varying(500) NOT NULL,
    enabled boolean DEFAULT true,
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE SEQUENCE t_rename_preset_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_rename_preset_id_seq OWNED BY t_rename_preset.id;

CREATE TABLE t_share_media (
    id integer NOT NULL,
    metadata_source character varying(20) DEFAULT 'auto'::character varying NOT NULL,
    result jsonb,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    work_key text DEFAULT ''::text NOT NULL,
    tmdb_id bigint,
    title text DEFAULT ''::text NOT NULL,
    library_media_type text DEFAULT ''::text NOT NULL,
    media_year integer,
    genre_ids integer[] DEFAULT '{}'::integer[] NOT NULL,
    country_codes text[] DEFAULT '{}'::text[] NOT NULL,
    rating numeric,
    metadata_provider text DEFAULT ''::text NOT NULL,
    external_id text DEFAULT ''::text NOT NULL,
    media_type text DEFAULT ''::text NOT NULL,
    original_title text DEFAULT ''::text NOT NULL,
    poster_path text DEFAULT ''::text NOT NULL
);

CREATE SEQUENCE t_share_media_entity_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_share_media_entity_id_seq OWNED BY t_share_media.id;

CREATE TABLE t_share_media_file (
    id integer NOT NULL,
    share_id integer NOT NULL,
    file_name text NOT NULL,
    metadata_source character varying(20) DEFAULT 'auto'::character varying NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    result jsonb,
    error text DEFAULT ''::text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    media_id integer,
    file_id text DEFAULT ''::text NOT NULL,
    file_size bigint DEFAULT 0 NOT NULL,
    sha1 text DEFAULT ''::text NOT NULL,
    pick_code text DEFAULT ''::text NOT NULL,
    available boolean DEFAULT true NOT NULL,
    last_seen_at timestamp without time zone,
    last_seen_scan_token text DEFAULT ''::text NOT NULL
);

CREATE TABLE t_share_media_file_episode (
    file_id integer NOT NULL,
    season_number integer NOT NULL,
    episode_number integer NOT NULL,
    CONSTRAINT t_share_media_file_episode_episode_number_check CHECK ((episode_number > 0)),
    CONSTRAINT t_share_media_file_episode_season_number_check CHECK ((season_number >= 0))
);

CREATE SEQUENCE t_share_media_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_share_media_id_seq OWNED BY t_share_media_file.id;

CREATE TABLE t_share_operation_queue (
    sequence bigint NOT NULL,
    task_id text NOT NULL,
    operation text NOT NULL,
    request_key text NOT NULL,
    share_ids integer[] NOT NULL,
    file_id integer DEFAULT 0 NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    phase text DEFAULT '等待目标资源'::text NOT NULL,
    result jsonb DEFAULT '{}'::jsonb NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    published boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT t_share_operation_queue_operation_check CHECK ((operation = ANY (ARRAY['share_delete'::text, 'share_clear'::text, 'share_media_delete'::text]))),
    CONSTRAINT t_share_operation_queue_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text, 'cancelled'::text])))
);

CREATE SEQUENCE t_share_operation_queue_sequence_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_share_operation_queue_sequence_seq OWNED BY t_share_operation_queue.sequence;

CREATE TABLE t_share_record (
    id integer NOT NULL,
    url text NOT NULL,
    password text DEFAULT ''::text NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    media_type character varying(10) DEFAULT 'auto'::character varying NOT NULL,
    share_cancelled boolean DEFAULT false NOT NULL,
    CONSTRAINT t_share_record_media_type_check CHECK (media_type IN ('auto','movie','tv'))
);

CREATE SEQUENCE t_share_record_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_share_record_id_seq OWNED BY t_share_record.id;

CREATE TABLE t_share_strm (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE t_share_transfer_log (
    id integer NOT NULL,
    task_id character varying(64) NOT NULL,
    share_code character varying(32) DEFAULT ''::character varying NOT NULL,
    share_folder_name character varying(512) DEFAULT ''::character varying NOT NULL,
    file_name character varying(512) DEFAULT ''::character varying NOT NULL,
    file_pick_code character varying(64) DEFAULT ''::character varying NOT NULL,
    file_size bigint DEFAULT 0 NOT NULL,
    file_sha1 character varying(128) DEFAULT ''::character varying NOT NULL,
    cloud115_id integer DEFAULT 0 NOT NULL,
    target_directory character varying(1024) DEFAULT ''::character varying NOT NULL,
    status character varying(32) DEFAULT 'pending'::character varying NOT NULL,
    error_message text DEFAULT ''::text NOT NULL,
    is_second_transfer boolean DEFAULT false NOT NULL,
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE SEQUENCE t_share_transfer_log_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_share_transfer_log_id_seq OWNED BY t_share_transfer_log.id;

CREATE TABLE t_strm_clear_run (
    task_id text NOT NULL,
    output_path text NOT NULL,
    completed boolean DEFAULT false NOT NULL
);

CREATE TABLE t_strm_config (
    id integer NOT NULL,
    cloud115_id integer NOT NULL,
    net_disk_path character varying(255) NOT NULL,
    local_path character varying(255) NOT NULL,
    cron character varying(50) NOT NULL,
    extension text DEFAULT '.strm'::text,
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    dir_tree_file text DEFAULT ''::text,
    sync_mode character varying(20) DEFAULT 'manual'::character varying,
    source_account integer DEFAULT 0,
    target_account integer DEFAULT 0,
    target_directory character varying(500),
    auto_cleanup boolean DEFAULT false,
    cleanup_threshold integer DEFAULT 0,
    cleanup_policy character varying(50),
    max_concurrency integer DEFAULT 1,
    behavior_sync_enabled boolean DEFAULT false NOT NULL,
    behavior_sync_interval integer DEFAULT 600 NOT NULL
);

CREATE SEQUENCE t_strm_config_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_strm_config_id_seq OWNED BY t_strm_config.id;

CREATE TABLE t_strm_export_history (
    owner_key text NOT NULL,
    output_path text NOT NULL,
    reason text NOT NULL
);

CREATE TABLE t_strm_export_state (
    owner_key text NOT NULL,
    export_key text NOT NULL,
    output_path text NOT NULL,
    content_fingerprint text DEFAULT ''::text NOT NULL,
    mapping_fingerprint text DEFAULT ''::text NOT NULL,
    share_strm_id text,
    state text DEFAULT 'active'::text NOT NULL,
    last_seen_run_id text DEFAULT ''::text NOT NULL,
    exported_at timestamp with time zone
);

CREATE TABLE t_strm_file (
    id integer NOT NULL,
    strm_config_id integer NOT NULL,
    file_name character varying(500) NOT NULL,
    file_path character varying(1000) NOT NULL,
    pick_code character varying(50),
    sha1 character varying(1000),
    file_size bigint,
    local_strm_path character varying(1000),
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    strm_config_ref_id integer GENERATED ALWAYS AS (NULLIF(strm_config_id, '-1'::integer)) STORED
);

CREATE SEQUENCE t_strm_file_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_strm_file_id_seq OWNED BY t_strm_file.id;

CREATE TABLE t_strm_playback_record (
    id bigint NOT NULL,
    record_id character varying(64) NOT NULL,
    resource_name text DEFAULT ''::text NOT NULL,
    poster text DEFAULT ''::text NOT NULL,
    direct_url text DEFAULT ''::text NOT NULL,
    called_at timestamp with time zone NOT NULL,
    caller_ip character varying(64) DEFAULT ''::character varying NOT NULL,
    location character varying(255) DEFAULT ''::character varying NOT NULL,
    request_method character varying(16) DEFAULT 'GET'::character varying NOT NULL
);

CREATE SEQUENCE t_strm_playback_record_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_strm_playback_record_id_seq OWNED BY t_strm_playback_record.id;

CREATE TABLE t_system_config (
    id integer NOT NULL,
    config_key character varying(100) NOT NULL,
    config_val text NOT NULL,
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE SEQUENCE t_system_config_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_system_config_id_seq OWNED BY t_system_config.id;

CREATE TABLE t_tmdb_cache (
    id integer NOT NULL,
    query_key character varying(500) NOT NULL,
    media_type character varying(20) NOT NULL,
    tmdb_id integer NOT NULL,
    title character varying(500),
    original_title character varying(500),
    year integer,
    poster_path character varying(500),
    overview text,
    vote_average numeric(3,1),
    release_date character varying(20),
    first_air_date character varying(20),
    season_number integer,
    episode_number integer,
    raw_data jsonb,
    expire_at timestamp without time zone NOT NULL,
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE SEQUENCE t_tmdb_cache_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_tmdb_cache_id_seq OWNED BY t_tmdb_cache.id;

CREATE TABLE t_user (
    id integer NOT NULL,
    name character varying(50) NOT NULL,
    password character varying(255) NOT NULL,
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE SEQUENCE t_user_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE t_user_id_seq OWNED BY t_user.id;

CREATE VIEW v_share_media_detail AS
 SELECT f.id,
    f.id AS local_file_id,
    f.file_id,
    f.media_id,
    f.share_id,
    f.file_name,
    f.file_size,
    f.sha1,
    f.pick_code,
    f.metadata_source,
    f.status,
    f.result,
    f.error,
    f.version,
    f.available,
    f.last_seen_at,
    f.last_seen_scan_token,
    f.created_at,
    f.updated_at,
    m.work_key,
    m.tmdb_id,
    m.title,
    m.original_title,
    m.poster_path,
    m.media_type AS library_media_type,
    m.media_year,
    m.genre_ids,
    m.country_codes,
    m.rating,
    COALESCE(( SELECT json_agg(json_build_object('season_number', e.season_number, 'episode_number', e.episode_number) ORDER BY e.season_number, e.episode_number) AS json_agg
           FROM t_share_media_file_episode e
          WHERE (e.file_id = f.id)), '[]'::json) AS episodes
   FROM (t_share_media_file f
     LEFT JOIN t_share_media m ON ((m.id = f.media_id)));

ALTER TABLE ONLY t_cloud_115 ALTER COLUMN id SET DEFAULT nextval('t_cloud_115_id_seq'::regclass);

ALTER TABLE ONLY t_cron_task ALTER COLUMN id SET DEFAULT nextval('t_cron_task_id_seq'::regclass);

ALTER TABLE ONLY t_cron_task_run ALTER COLUMN id SET DEFAULT nextval('t_cron_task_run_id_seq'::regclass);

ALTER TABLE ONLY t_emby_playback_event ALTER COLUMN id SET DEFAULT nextval('t_emby_playback_event_id_seq'::regclass);

ALTER TABLE ONLY t_emby_server ALTER COLUMN id SET DEFAULT nextval('t_emby_server_id_seq'::regclass);

ALTER TABLE ONLY t_identify_cache ALTER COLUMN id SET DEFAULT nextval('t_identify_cache_id_seq'::regclass);

ALTER TABLE ONLY t_media_category ALTER COLUMN id SET DEFAULT nextval('t_media_category_id_seq'::regclass);

ALTER TABLE ONLY t_media_file_cache ALTER COLUMN id SET DEFAULT nextval('t_media_file_cache_id_seq'::regclass);

ALTER TABLE ONLY t_media_source ALTER COLUMN id SET DEFAULT nextval('t_media_source_id_seq'::regclass);

ALTER TABLE ONLY t_notification_config ALTER COLUMN id SET DEFAULT nextval('t_notification_config_id_seq'::regclass);

ALTER TABLE ONLY t_offline_download_task ALTER COLUMN id SET DEFAULT nextval('t_offline_download_task_id_seq'::regclass);

ALTER TABLE ONLY t_rename_preset ALTER COLUMN id SET DEFAULT nextval('t_rename_preset_id_seq'::regclass);

ALTER TABLE ONLY t_share_media ALTER COLUMN id SET DEFAULT nextval('t_share_media_entity_id_seq'::regclass);

ALTER TABLE ONLY t_share_media_file ALTER COLUMN id SET DEFAULT nextval('t_share_media_id_seq'::regclass);

ALTER TABLE ONLY t_share_operation_queue ALTER COLUMN sequence SET DEFAULT nextval('t_share_operation_queue_sequence_seq'::regclass);

ALTER TABLE ONLY t_share_record ALTER COLUMN id SET DEFAULT nextval('t_share_record_id_seq'::regclass);

ALTER TABLE ONLY t_share_transfer_log ALTER COLUMN id SET DEFAULT nextval('t_share_transfer_log_id_seq'::regclass);

ALTER TABLE ONLY t_strm_config ALTER COLUMN id SET DEFAULT nextval('t_strm_config_id_seq'::regclass);

ALTER TABLE ONLY t_strm_file ALTER COLUMN id SET DEFAULT nextval('t_strm_file_id_seq'::regclass);

ALTER TABLE ONLY t_strm_playback_record ALTER COLUMN id SET DEFAULT nextval('t_strm_playback_record_id_seq'::regclass);

ALTER TABLE ONLY t_system_config ALTER COLUMN id SET DEFAULT nextval('t_system_config_id_seq'::regclass);

ALTER TABLE ONLY t_tmdb_cache ALTER COLUMN id SET DEFAULT nextval('t_tmdb_cache_id_seq'::regclass);

ALTER TABLE ONLY t_user ALTER COLUMN id SET DEFAULT nextval('t_user_id_seq'::regclass);

ALTER TABLE ONLY t_cloud_115
    ADD CONSTRAINT t_cloud_115_name_key UNIQUE (name);

ALTER TABLE ONLY t_cloud_115
    ADD CONSTRAINT t_cloud_115_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_cron_task
    ADD CONSTRAINT t_cron_task_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_cron_task_run
    ADD CONSTRAINT t_cron_task_run_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_cron_task_run
    ADD CONSTRAINT t_cron_task_run_task_id_key UNIQUE (task_id);

ALTER TABLE ONLY t_emby_monitor_item
    ADD CONSTRAINT t_emby_monitor_item_pkey PRIMARY KEY (server_id, item_id);

ALTER TABLE ONLY t_emby_monitor_state
    ADD CONSTRAINT t_emby_monitor_state_pkey PRIMARY KEY (server_id);

ALTER TABLE ONLY t_emby_playback_event
    ADD CONSTRAINT t_emby_playback_event_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_emby_server
    ADD CONSTRAINT t_emby_server_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_identify_cache
    ADD CONSTRAINT t_identify_cache_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_media_category
    ADD CONSTRAINT t_media_category_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_media_file_cache
    ADD CONSTRAINT t_media_file_cache_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_media_file_cache
    ADD CONSTRAINT t_media_file_cache_source_id_file_path_key UNIQUE (source_id, file_path);

ALTER TABLE ONLY t_media_source
    ADD CONSTRAINT t_media_source_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_notification_config
    ADD CONSTRAINT t_notification_config_channel_key UNIQUE (channel);

ALTER TABLE ONLY t_notification_config
    ADD CONSTRAINT t_notification_config_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_offline_download_task
    ADD CONSTRAINT t_offline_download_task_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_rename_preset
    ADD CONSTRAINT t_rename_preset_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_share_media_file_episode
    ADD CONSTRAINT t_share_media_file_episode_pkey PRIMARY KEY (file_id, season_number, episode_number);

ALTER TABLE ONLY t_share_media_file
    ADD CONSTRAINT t_share_media_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_share_media
    ADD CONSTRAINT t_share_media_pkey1 PRIMARY KEY (id);

ALTER TABLE ONLY t_share_operation_queue
    ADD CONSTRAINT t_share_operation_queue_pkey PRIMARY KEY (sequence);

ALTER TABLE ONLY t_share_operation_queue
    ADD CONSTRAINT t_share_operation_queue_task_id_key UNIQUE (task_id);

ALTER TABLE ONLY t_share_record
    ADD CONSTRAINT t_share_record_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_share_strm
    ADD CONSTRAINT t_share_strm_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_share_transfer_log
    ADD CONSTRAINT t_share_transfer_log_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_strm_clear_run
    ADD CONSTRAINT t_strm_clear_run_pkey PRIMARY KEY (task_id);

ALTER TABLE ONLY t_strm_config
    ADD CONSTRAINT t_strm_config_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_strm_export_history
    ADD CONSTRAINT t_strm_export_history_pkey PRIMARY KEY (owner_key, output_path);

ALTER TABLE ONLY t_strm_export_state
    ADD CONSTRAINT t_strm_export_state_pkey PRIMARY KEY (owner_key, export_key);

ALTER TABLE ONLY t_strm_file
    ADD CONSTRAINT t_strm_file_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_strm_playback_record
    ADD CONSTRAINT t_strm_playback_record_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_strm_playback_record
    ADD CONSTRAINT t_strm_playback_record_record_id_key UNIQUE (record_id);

ALTER TABLE ONLY t_system_config
    ADD CONSTRAINT t_system_config_config_key_key UNIQUE (config_key);

ALTER TABLE ONLY t_system_config
    ADD CONSTRAINT t_system_config_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_tmdb_cache
    ADD CONSTRAINT t_tmdb_cache_pkey PRIMARY KEY (id);

ALTER TABLE ONLY t_tmdb_cache
    ADD CONSTRAINT t_tmdb_cache_query_key_media_type_key UNIQUE (query_key, media_type);

ALTER TABLE ONLY t_user
    ADD CONSTRAINT t_user_name_key UNIQUE (name);

ALTER TABLE ONLY t_user
    ADD CONSTRAINT t_user_pkey PRIMARY KEY (id);

CREATE INDEX idx_cron_run_task ON t_cron_task_run USING btree (cron_task_id, id DESC);

CREATE UNIQUE INDEX idx_cron_task_config_type ON t_cron_task USING btree (strm_config_id, task_type);

CREATE UNIQUE INDEX idx_cron_task_key ON t_cron_task USING btree (task_key);

CREATE INDEX idx_emby_monitor_item_created ON t_emby_monitor_item USING btree (server_id, date_created DESC);

CREATE INDEX idx_emby_monitor_item_seen ON t_emby_monitor_item USING btree (server_id, first_seen_at DESC);

CREATE INDEX idx_emby_playback_server_started ON t_emby_playback_event USING btree (server_id, started_at DESC);

CREATE INDEX idx_identify_cache_created_at ON t_identify_cache USING btree (created_at);

CREATE INDEX idx_identify_cache_file_hash ON t_identify_cache USING btree (file_hash);

CREATE INDEX idx_identify_cache_is_manual ON t_identify_cache USING btree (is_manual);

CREATE INDEX idx_media_category_enabled ON t_media_category USING btree (enabled);

CREATE INDEX idx_media_category_type ON t_media_category USING btree (media_type);

CREATE INDEX idx_media_file_cache_media_type ON t_media_file_cache USING btree (media_type);

CREATE INDEX idx_media_file_cache_source_id ON t_media_file_cache USING btree (source_id);

CREATE INDEX idx_media_file_cache_tmdb_id ON t_media_file_cache USING btree (tmdb_id);

CREATE INDEX idx_media_source_enabled ON t_media_source USING btree (enabled);

CREATE INDEX idx_media_source_priority ON t_media_source USING btree (priority);

CREATE INDEX idx_media_source_type ON t_media_source USING btree (source_type);

CREATE INDEX idx_offline_download_task_account_status ON t_offline_download_task USING btree (cloud115_id, status);

CREATE INDEX idx_offline_download_task_task_id ON t_offline_download_task USING btree (task_id);

CREATE INDEX idx_rename_preset_enabled ON t_rename_preset USING btree (enabled);

CREATE INDEX idx_rename_preset_media_type ON t_rename_preset USING btree (media_type);

CREATE INDEX idx_share_library_created ON t_share_media_file USING btree (created_at);

CREATE INDEX idx_share_media_share_id ON t_share_media_file USING btree (share_id);

CREATE INDEX idx_share_media_status ON t_share_media_file USING btree (status);

CREATE INDEX idx_share_operation_recovery ON t_share_operation_queue USING btree (status, sequence);

CREATE INDEX idx_share_transfer_log_task_id_id ON t_share_transfer_log USING btree (task_id, id);

CREATE INDEX idx_share_transfer_log_task_pick ON t_share_transfer_log USING btree (task_id, file_pick_code);

CREATE UNIQUE INDEX idx_strm_file_config_path ON t_strm_file USING btree (strm_config_id, file_path);

CREATE INDEX idx_strm_playback_record_called_at ON t_strm_playback_record USING btree (called_at DESC);

CREATE INDEX idx_tmdb_cache_expire_at ON t_tmdb_cache USING btree (expire_at);

CREATE INDEX idx_tmdb_cache_query_key ON t_tmdb_cache USING btree (query_key);

CREATE INDEX idx_tmdb_cache_tmdb_id ON t_tmdb_cache USING btree (tmdb_id);

CREATE INDEX share_media_file_available_status_idx ON t_share_media_file USING btree (share_id, available, status, id);

CREATE INDEX share_media_file_owner_idx ON t_share_media_file USING btree (media_id, id);

CREATE INDEX share_media_file_path_idx ON t_share_media_file USING btree (share_id, md5(file_name));

CREATE UNIQUE INDEX share_media_file_path_unique ON t_share_media_file USING btree (share_id, file_name) WHERE (file_id = ''::text);

CREATE UNIQUE INDEX share_media_file_remote_unique ON t_share_media_file USING btree (share_id, file_id) WHERE (file_id <> ''::text);

CREATE UNIQUE INDEX share_media_identity_unique ON t_share_media USING btree (metadata_source, metadata_provider, media_type, external_id);

CREATE UNIQUE INDEX strm_export_active_path ON t_strm_export_state USING btree (output_path) WHERE (state = 'active'::text);

CREATE INDEX t_share_media_country_codes_idx ON t_share_media USING gin (country_codes);

CREATE INDEX t_share_media_created_at_idx ON t_share_media USING btree (created_at);

CREATE INDEX t_share_media_genre_ids_idx ON t_share_media USING gin (genre_ids);

CREATE INDEX t_share_media_media_year_idx ON t_share_media USING btree (media_year);

CREATE INDEX t_share_media_rating_idx ON t_share_media USING btree (rating);

CREATE INDEX t_share_media_title_idx ON t_share_media USING btree (title);

CREATE INDEX t_share_media_tmdb_id_idx ON t_share_media USING btree (tmdb_id);

CREATE INDEX t_share_media_work_key_idx ON t_share_media USING btree (work_key);

CREATE UNIQUE INDEX uk_emby_playback_active_session ON t_emby_playback_event USING btree (server_id, session_id) WHERE (ended_at IS NULL);

CREATE UNIQUE INDEX uk_emby_server_default ON t_emby_server USING btree (is_default) WHERE (is_default = true);

CREATE UNIQUE INDEX uk_emby_server_proxy_port ON t_emby_server USING btree (proxy_port) WHERE (proxy_port <> 0);

CREATE UNIQUE INDEX uq_identify_cache_file_hash ON t_identify_cache USING btree (file_hash);

CREATE UNIQUE INDEX uq_share_operation_active ON t_share_operation_queue USING btree (request_key) WHERE (status = ANY (ARRAY['pending'::text, 'running'::text]));

CREATE TRIGGER update_cloud115_timestamp_trigger BEFORE UPDATE ON t_cloud_115 FOR EACH ROW EXECUTE FUNCTION update_cloud115_timestamp();

CREATE TRIGGER update_cron_task_timestamp_trigger BEFORE UPDATE ON t_cron_task FOR EACH ROW EXECUTE FUNCTION update_cron_task_timestamp();

CREATE TRIGGER update_strm_config_timestamp_trigger BEFORE UPDATE ON t_strm_config FOR EACH ROW EXECUTE FUNCTION update_strm_config_timestamp();

CREATE TRIGGER update_system_config_timestamp_trigger BEFORE UPDATE ON t_system_config FOR EACH ROW EXECUTE FUNCTION update_system_config_timestamp();

CREATE TRIGGER update_user_timestamp_trigger BEFORE UPDATE ON t_user FOR EACH ROW EXECUTE FUNCTION update_user_timestamp();

ALTER TABLE ONLY t_share_media_file
    ADD CONSTRAINT share_media_file_media_fk FOREIGN KEY (media_id) REFERENCES t_share_media(id) ON DELETE SET NULL;

ALTER TABLE ONLY t_cron_task
    ADD CONSTRAINT t_cron_task_cloud115_id_fkey FOREIGN KEY (cloud115_id) REFERENCES t_cloud_115(id) ON DELETE CASCADE;

ALTER TABLE ONLY t_cron_task_run
    ADD CONSTRAINT t_cron_task_run_cron_task_id_fkey FOREIGN KEY (cron_task_id) REFERENCES t_cron_task(id) ON DELETE SET NULL;

ALTER TABLE ONLY t_cron_task
    ADD CONSTRAINT t_cron_task_strm_config_id_fkey FOREIGN KEY (strm_config_id) REFERENCES t_strm_config(id) ON DELETE CASCADE;

ALTER TABLE ONLY t_emby_monitor_item
    ADD CONSTRAINT t_emby_monitor_item_server_id_fkey FOREIGN KEY (server_id) REFERENCES t_emby_server(id) ON DELETE CASCADE;

ALTER TABLE ONLY t_emby_monitor_state
    ADD CONSTRAINT t_emby_monitor_state_server_id_fkey FOREIGN KEY (server_id) REFERENCES t_emby_server(id) ON DELETE CASCADE;

ALTER TABLE ONLY t_emby_playback_event
    ADD CONSTRAINT t_emby_playback_event_server_id_fkey FOREIGN KEY (server_id) REFERENCES t_emby_server(id) ON DELETE CASCADE;

ALTER TABLE ONLY t_media_file_cache
    ADD CONSTRAINT t_media_file_cache_source_id_fkey FOREIGN KEY (source_id) REFERENCES t_media_source(id) ON DELETE CASCADE;

ALTER TABLE ONLY t_media_source
    ADD CONSTRAINT t_media_source_cloud115_id_fkey FOREIGN KEY (cloud115_id) REFERENCES t_cloud_115(id) ON DELETE SET NULL;

ALTER TABLE ONLY t_media_source
    ADD CONSTRAINT t_media_source_emby_server_id_fkey FOREIGN KEY (emby_server_id) REFERENCES t_emby_server(id) ON DELETE SET NULL;

ALTER TABLE ONLY t_share_media_file_episode
    ADD CONSTRAINT t_share_media_file_episode_file_id_fkey FOREIGN KEY (file_id) REFERENCES t_share_media_file(id) ON DELETE CASCADE;

ALTER TABLE ONLY t_share_media_file
    ADD CONSTRAINT t_share_media_share_id_fkey FOREIGN KEY (share_id) REFERENCES t_share_record(id) ON DELETE CASCADE;

ALTER TABLE ONLY t_strm_config
    ADD CONSTRAINT t_strm_config_cloud115_id_fkey FOREIGN KEY (cloud115_id) REFERENCES t_cloud_115(id);

ALTER TABLE ONLY t_strm_file
    ADD CONSTRAINT t_strm_file_config_ref_fkey FOREIGN KEY (strm_config_ref_id) REFERENCES t_strm_config(id) ON DELETE CASCADE;

COMMENT ON COLUMN t_media_source.organize_target_path IS '整理目的地目录（根目录），如 /已整理';
COMMENT ON COLUMN t_media_source.watch_path IS '监控目录；115 自动监控时使用该目录而不是源路径';
COMMENT ON COLUMN t_media_source.media_type IS '自动整理默认媒体类型（all/movie/tv）';
COMMENT ON COLUMN t_media_source.metadata_source IS '元数据来源策略（auto/tmdb/metatube）';
COMMENT ON COLUMN t_media_source.conflict_policy IS '自动整理默认冲突策略（skip/overwrite/suffix）';
COMMENT ON COLUMN t_media_source.operation_mode IS '自动整理默认操作方式（move/copy/hardlink/symlink）';
COMMENT ON COLUMN t_media_source.auto_organize IS '是否在监控到新增文件后自动整理';
COMMENT ON COLUMN t_media_source.watch_enabled IS '是否开启媒体源监控';
COMMENT ON COLUMN t_media_source.watch_interval IS '监控轮询间隔（秒）';
COMMENT ON COLUMN t_media_source.emby_library_id IS '关联的 Emby 媒体库 ID';

-- 默认数据
INSERT INTO t_system_config (config_key, config_val) VALUES ('log_save_day_limit', '1');
INSERT INTO t_system_config (config_key, config_val) VALUES ('tmdb_api_key', '');
INSERT INTO t_system_config (config_key, config_val) VALUES ('filename_recognition_rules', E'[{"id": "tv_sxe_compact", "name": "紧凑型 SxxExx 合并剧集", "enabled": true, "example": "举重妖精金福珠S01E01.mkv", "pattern": "(?i)^(?P<title>.+?)\\\\s*S(?P<season>\\\\d{1,2})E(?P<episode>\\\\d{1,3})(?P<episodes>(?:\\\\s*E\\\\d{1,3})*)\\\\s*$", "priority": 9, "media_type": "tv", "description": "支持中文标题紧贴 S01E01，以及 S01E01E02 连续合并集。", "default_season": 0}, {"id": "tv_sxe", "name": "SxxExx 标准剧集", "enabled": true, "example": "你是谁 - S01E01E02 .mp4", "pattern": "(?i)^(?P<title>.+?)\\\\s*S(?P<season>\\\\d{1,2})E(?P<episode>\\\\d{1,3})(?P<episodes>(?:\\\\s*E\\\\d{1,3})*)(?:\\\\s+.*)?$", "priority": 10, "media_type": "tv", "description": "支持片名紧贴S01E01、特别篇S00及S01E01E02合并集；episodes保留后续全部集号。", "default_season": 0}, {"id": "tv_x", "name": "数字 x 数字剧集", "enabled": true, "example": "Breaking Bad - 1x02 - Cat''s in the Bag.mkv", "pattern": "(?i)^(?P<title>.+?)\\\\s+(?P<season>\\\\d{1,2})x(?P<episode>\\\\d{1,3})(?:\\\\s+.*)?$", "priority": 20, "media_type": "tv", "description": "适用于 1x02、02x15 等命名。", "default_season": 0}, {"id": "tv_words", "name": "Season Episode 剧集", "enabled": true, "example": "The Office Season 2 Episode 3.mp4", "pattern": "(?i)^(?P<title>.+?)\\\\s+Season\\\\s*(?P<season>\\\\d{1,2})\\\\s+Episode\\\\s*(?P<episode>\\\\d{1,3})(?:\\\\s+.*)?$", "priority": 30, "media_type": "tv", "description": "适用于英文 Season 2 Episode 3 命名。", "default_season": 0}, {"id": "tv_chinese", "name": "中文季集", "enabled": true, "example": "庆余年 第2季 第05集.mp4", "pattern": "^(?P<title>.+?)\\\\s*第(?P<season>\\\\d{1,2})季\\\\s*第(?P<episode>\\\\d{1,3})集(?:\\\\s+.*)?$", "priority": 40, "media_type": "tv", "description": "适用于第2季第5集等中文命名，季数需使用阿拉伯数字。", "default_season": 0}, {"id": "tv_episode", "name": "EP 单集", "enabled": true, "example": "葬送的芙莉莲 EP08.mp4", "pattern": "(?i)^(?P<title>.+?)\\\\s+(?:EP?|Episode)\\\\s*(?P<episode>\\\\d{1,3})(?:\\\\s+.*)?$", "priority": 50, "media_type": "tv", "description": "仅包含集数时按默认季识别，默认第 1 季。", "default_season": 1}, {"id": "movie_year", "name": "电影标题与年份", "enabled": true, "example": "Inception.2010.1080p.BluRay.mkv", "pattern": "^(?P<title>.+?)\\\\s+(?P<year>19\\\\d{2}|20\\\\d{2})(?:\\\\s+.*)?$", "priority": 60, "media_type": "movie", "description": "适用于标题后包含四位发行年份的电影命名。", "default_season": 0}, {"id": "anime_number", "name": "动漫纯集数", "enabled": false, "example": "海贼王 - 1120.mp4", "pattern": "^(?P<title>.+?)\\\\s+(?P<episode>\\\\d{1,4})(?:v\\\\d+)?(?:\\\\s+.*)?$", "priority": 70, "media_type": "tv", "description": "可能与年份或标题数字冲突，确认资源命名稳定后再启用。", "default_season": 1}, {"id": "review_season_episode", "name": "分隔式季集号", "enabled": true, "example": "Douluo.Dalu.S01.EP072.2019.mkv", "pattern": "(?i)^(?P<title>.+?)\\\\s+S(?P<season>\\\\d{1,3})\\\\s+EP?\\\\s*(?P<episode>0*[1-9]\\\\d{0,2})(?:\\\\s+.*)?$", "priority": 11, "media_type": "tv", "description": "手动核对失败样本补充；仅解析明确命名，仍需数据源核验作品。", "default_season": 1}, {"id": "review_chinese_episode", "name": "中文单集及话数", "enabled": true, "example": "塞外奇侠传第15集.mp4", "pattern": "^(?P<title>[^第]+?)\\\\s*第(?P<episode>0*[1-9]\\\\d{0,2})[集话話](?:\\\\s+.*)?$", "priority": 41, "media_type": "tv", "description": "手动核对失败样本补充；仅解析明确命名，仍需数据源核验作品。", "default_season": 1}, {"id": "review_compact_ep", "name": "紧凑或带年份的EP单集", "enabled": true, "example": "如来神掌.2002.EP02.DVDRip.mkv", "pattern": "(?i)^(?P<title>.+?)(?:\\\\s+(?P<year>19\\\\d{2}|20\\\\d{2}))?\\\\s*EP\\\\s*(?P<episode>0*[1-9]\\\\d{0,2})(?:\\\\s+.*)?$", "priority": 49, "media_type": "tv", "description": "手动核对失败样本补充；仅解析明确命名，仍需数据源核验作品。", "default_season": 1}, {"id": "review_fansub_episode", "name": "字幕组片名与方括号集号", "enabled": true, "example": "[UHA-WINGS][Kimetsu no Yaiba][01][x264 1080p][CHS].mp4", "pattern": "^\\\\[[^\\\\[\\\\]]+\\\\]\\\\s*\\\\[(?P<title>[^\\\\[\\\\]]+)\\\\]\\\\s*\\\\[(?P<episode>0*[1-9]\\\\d{0,2})\\\\](?:\\\\s*\\\\[[^\\\\[\\\\]]*\\\\])*\\\\s*$", "priority": 42, "media_type": "tv", "description": "手动核对失败样本补充；仅解析明确命名，仍需数据源核验作品。", "default_season": 1}, {"id": "review_parenthesis_episode", "name": "中文片名与括号集号", "enabled": true, "example": "九阴真经 (18).mkv", "pattern": "^(?P<title>[\\\\p{Han}]{2,})\\\\s*[（(](?P<episode>0*[1-9]\\\\d{0,2})[）)]$", "priority": 43, "media_type": "tv", "description": "手动核对失败样本补充；仅解析明确命名，仍需数据源核验作品。", "default_season": 1}]');
INSERT INTO t_system_config (config_key, config_val) VALUES ('share_disc_recognition_version', '1');
INSERT INTO t_rename_preset (name, media_type, template, enabled) VALUES ('电影（官方）', 'movie', '{{ title }}{% if year %} ({{ year }}){% endif %}{% if tmdbid %} [tmdbid={{ tmdbid }}]{% endif %}/{{ title }}{% if year %} ({{ year }}){% endif %}{% if tmdbid %} [tmdbid={{ tmdbid }}]{% endif %}{% if videoFormat %} - {{ videoFormat }}{% endif %}{{ fileExt }}', true);
INSERT INTO t_rename_preset (name, media_type, template, enabled) VALUES ('电影（简洁）', 'movie', '{{ title }}{{ fileExt }}', true);
INSERT INTO t_rename_preset (name, media_type, template, enabled) VALUES ('剧集（官方）', 'tv', '{{ title }}{% if year %} ({{ year }}){% endif %}{% if tmdbid %} [tmdbid={{ tmdbid }}]{% endif %}/Season {{ season }}/{{ title }}{% if en_title and en_title != title %}.{{ en_title }}{% endif %}{% if year %}.{{ year }}{% endif %}.S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{% if videoFormat %}.{{ videoFormat }}{% endif %}{% if source %}.{{ source }}{% endif %}{% if codec %}.{{ codec }}{% endif %}{{ fileExt }}', true);
INSERT INTO t_rename_preset (name, media_type, template, enabled) VALUES ('剧集（简洁）', 'tv', '{{ title }}/S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{{ fileExt }}', true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('动画电影', 'movie', '/电影/动画电影', '{"genre_ids": [16]}'::jsonb, true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('华语电影', 'movie', '/电影/华语电影', '{"languages": ["zh", "cn"]}'::jsonb, true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('外语电影', 'movie', '/电影/外语电影', '{"languages": ["en", "ja", "ko", "fr", "de", "es", "it", "ru", "nl", "pt", "th", "hi"]}'::jsonb, true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('其他电影', 'movie', '/电影/其他电影', '{"default": true}'::jsonb, true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('未分类', 'movie', '/电影/未分类', '{"default": true}'::jsonb, true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('国漫', 'tv', '/电视剧/国漫', '{"countries": ["CN", "TW", "HK"], "genre_ids": [16]}'::jsonb, true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('日番', 'tv', '/电视剧/日番', '{"countries": ["JP"], "genre_ids": [16]}'::jsonb, true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('纪录片', 'tv', '/电视剧/纪录片', '{"genre_ids": [99]}'::jsonb, true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('儿童', 'tv', '/电视剧/儿童', '{"genre_ids": [10762]}'::jsonb, true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('综艺', 'tv', '/电视剧/综艺', '{"genre_ids": [10764, 10767]}'::jsonb, true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('国产剧', 'tv', '/电视剧/国产剧', '{"countries": ["CN", "TW", "HK"]}'::jsonb, true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('欧美剧', 'tv', '/电视剧/欧美剧', '{"countries": ["US", "FR", "GB", "UK", "DE", "ES", "IT", "NL", "PT", "RU"]}'::jsonb, true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('日韩剧', 'tv', '/电视剧/日韩剧', '{"countries": ["JP", "KP", "KR", "TH", "IN", "SG"]}'::jsonb, true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('其他剧集', 'tv', '/电视剧/其他剧集', '{"default": true}'::jsonb, true);
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled) VALUES ('未分类', 'tv', '/电视剧/未分类', '{"default": true}'::jsonb, true);
INSERT INTO t_cron_task (task_key, task_name, task_type, handler, cron_expr, status, builtin, params, timezone) VALUES ('system:cooling', '账号冷却恢复', 'cooling_recovery', 'cooling_recovery', '0 * * * * *', 'enabled', true, '{}'::jsonb, 'Local');
INSERT INTO t_cron_task (task_key, task_name, task_type, handler, cron_expr, status, builtin, params, timezone) VALUES ('system:logs', '日志清理', 'log_cleanup', 'log_cleanup', '0 0 * * * *', 'enabled', true, '{}'::jsonb, 'Local');
INSERT INTO t_cron_task (task_key, task_name, task_type, handler, cron_expr, status, builtin, params, timezone) VALUES ('system:identify-cache', '识别缓存清理', 'identify_cache_cleanup', 'identify_cache_cleanup', '0 0 3 * * *', 'enabled', true, '{"keep_days": 30}'::jsonb, 'Local');
