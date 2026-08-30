package dao

import (
	"database/sql"
	"fmt"
	"strings"

	"easy-strm/internal/domain"
)

// EmbyServerDAO 管理 Emby 多实例连接配置。
type EmbyServerDAO struct {
	db *sql.DB
}

// NewEmbyServerDAO 创建 Emby 实例 DAO。
func NewEmbyServerDAO(db *sql.DB) *EmbyServerDAO {
	return &EmbyServerDAO{db: db}
}

const embyServerColumns = `id, name, base_url, api_key, enabled, is_default, create_time, update_time`

func scanEmbyServer(scanner interface{ Scan(...interface{}) error }) (*domain.EmbyServer, error) {
	server := &domain.EmbyServer{}
	if err := scanner.Scan(&server.ID, &server.Name, &server.BaseURL, &server.APIKey, &server.Enabled, &server.IsDefault, &server.CreateTime, &server.UpdateTime); err != nil {
		return nil, err
	}
	server.APIKeyMask = maskEmbyAPIKey(server.APIKey)
	return server, nil
}

func maskEmbyAPIKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "********"
	}
	return value[:4] + "********" + value[len(value)-4:]
}

// List 查询全部 Emby 实例。
func (d *EmbyServerDAO) List() ([]*domain.EmbyServer, error) {
	rows, err := d.db.Query(`SELECT ` + embyServerColumns + ` FROM t_emby_server ORDER BY is_default DESC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("查询 Emby 实例失败: %w", err)
	}
	defer rows.Close()
	servers := make([]*domain.EmbyServer, 0)
	for rows.Next() {
		server, scanErr := scanEmbyServer(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("读取 Emby 实例失败: %w", scanErr)
		}
		servers = append(servers, server)
	}
	return servers, rows.Err()
}

// GetByID 按 ID 查询 Emby 实例，not found 时返回 nil。
func (d *EmbyServerDAO) GetByID(id int) (*domain.EmbyServer, error) {
	server, err := scanEmbyServer(d.db.QueryRow(`SELECT `+embyServerColumns+` FROM t_emby_server WHERE id=$1`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询 Emby 实例失败: %w", err)
	}
	return server, nil
}

// GetDefault 查询默认 Emby 实例。
func (d *EmbyServerDAO) GetDefault() (*domain.EmbyServer, error) {
	server, err := scanEmbyServer(d.db.QueryRow(`SELECT ` + embyServerColumns + ` FROM t_emby_server WHERE enabled=true ORDER BY is_default DESC, id ASC LIMIT 1`))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询默认 Emby 实例失败: %w", err)
	}
	return server, nil
}

// Create 新增 Emby 实例，并保证最多只有一个默认实例。
func (d *EmbyServerDAO) Create(name, baseURL, apiKey string, enabled, isDefault bool) (*domain.EmbyServer, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM t_emby_server`).Scan(&count); err != nil {
		return nil, err
	}
	if count == 0 {
		isDefault = true
	}
	if isDefault {
		if _, err = tx.Exec(`UPDATE t_emby_server SET is_default=false WHERE is_default=true`); err != nil {
			return nil, err
		}
	}
	server, err := scanEmbyServer(tx.QueryRow(`INSERT INTO t_emby_server(name, base_url, api_key, enabled, is_default) VALUES($1,$2,$3,$4,$5) RETURNING `+embyServerColumns, name, strings.TrimRight(baseURL, "/"), apiKey, enabled, isDefault))
	if err != nil {
		return nil, fmt.Errorf("新增 Emby 实例失败: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return server, nil
}

// Update 更新 Emby 实例；apiKey 为空时保留原值。
func (d *EmbyServerDAO) Update(id int, name, baseURL, apiKey string, enabled, isDefault bool) (*domain.EmbyServer, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if !isDefault {
		var otherDefaults int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM t_emby_server WHERE id<>$1 AND is_default=true`, id).Scan(&otherDefaults); err != nil { return nil, err }
		if otherDefaults == 0 { isDefault = true }
	}
	if isDefault {
		if _, err = tx.Exec(`UPDATE t_emby_server SET is_default=false WHERE id<>$1 AND is_default=true`, id); err != nil {
			return nil, err
		}
	}
	server, err := scanEmbyServer(tx.QueryRow(`UPDATE t_emby_server SET name=$2, base_url=$3, api_key=CASE WHEN $4='' THEN api_key ELSE $4 END, enabled=$5, is_default=$6, update_time=CURRENT_TIMESTAMP WHERE id=$1 RETURNING `+embyServerColumns, id, name, strings.TrimRight(baseURL, "/"), apiKey, enabled, isDefault))
	if err != nil {
		return nil, fmt.Errorf("更新 Emby 实例失败: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return server, nil
}

// Delete 删除连接配置，数据库外键只解除关联，不触碰 Emby 数据。
func (d *EmbyServerDAO) Delete(id int) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var wasDefault bool
	if err = tx.QueryRow(`SELECT is_default FROM t_emby_server WHERE id=$1`, id).Scan(&wasDefault); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("Emby 实例不存在")
		}
		return err
	}
	if _, err = tx.Exec(`UPDATE t_media_source SET emby_server_id=NULL, emby_library_id='', update_time=CURRENT_TIMESTAMP WHERE emby_server_id=$1`, id); err != nil { return err }
	result, err := tx.Exec(`DELETE FROM t_emby_server WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("删除 Emby 实例失败: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("Emby 实例不存在")
	}
	if wasDefault {
		if _, err = tx.Exec(`UPDATE t_emby_server SET is_default=true WHERE id=(SELECT id FROM t_emby_server ORDER BY enabled DESC, id ASC LIMIT 1)`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetMediaSourceBinding 查询媒体源绑定的 Emby 实例和媒体库。
func (d *EmbyServerDAO) GetMediaSourceBinding(sourceID int) (*domain.EmbyServer, string, error) {
	var serverID sql.NullInt64
	var libraryID string
	if err := d.db.QueryRow(`SELECT emby_server_id, COALESCE(emby_library_id, '') FROM t_media_source WHERE id=$1`, sourceID).Scan(&serverID, &libraryID); err != nil {
		if err == sql.ErrNoRows {
			return nil, "", fmt.Errorf("媒体源不存在")
		}
		return nil, "", err
	}
	if libraryID == "" {
		return nil, "", nil
	}
	if serverID.Valid {
		server, err := d.GetByID(int(serverID.Int64))
		return server, libraryID, err
	}
	server, err := d.GetDefault()
	return server, libraryID, err
}

// BindMediaSource 绑定媒体源到指定 Emby 实例和媒体库。
func (d *EmbyServerDAO) BindMediaSource(sourceID, serverID int, libraryID string) error {
	result, err := d.db.Exec(`UPDATE t_media_source SET emby_server_id=$2, emby_library_id=$3, update_time=CURRENT_TIMESTAMP WHERE id=$1`, sourceID, serverID, libraryID)
	if err != nil {
		return fmt.Errorf("绑定媒体源失败: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("媒体源不存在")
	}
	return nil
}
