package dao

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"fmt"
	"github.com/go-redis/redis/v8"
	"time"
)

// PlaybackRecordDAO 使用 PostgreSQL 保存调用记录，Redis 仅缓存归属地。
type PlaybackRecordDAO struct{ client *redis.Client }

// NewPlaybackRecordDAO 创建播放记录访问对象。
func NewPlaybackRecordDAO(client *redis.Client) *PlaybackRecordDAO {
	return &PlaybackRecordDAO{client: client}
}

// Save 原子追加记录并裁剪容量。
func (d *PlaybackRecordDAO) Save(ctx context.Context, record domain.PlaybackRecord) error {
	if DB == nil {
		return fmt.Errorf("数据库未初始化")
	}
	_, err := DB.ExecContext(ctx, `INSERT INTO t_strm_playback_record (record_id, resource_name, poster, direct_url, called_at, caller_ip, location, request_method) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, record.ID, record.Name, record.Poster, record.URL, record.Time, record.IP, record.Location, record.Method)
	return err
}

// List 返回按调用时间倒序排列的最近记录。
func (d *PlaybackRecordDAO) List(ctx context.Context) ([]domain.PlaybackRecord, error) {
	if DB == nil {
		return []domain.PlaybackRecord{}, nil
	}
	rows, err := DB.QueryContext(ctx, `SELECT record_id, resource_name, poster, direct_url, called_at, caller_ip, location, request_method FROM t_strm_playback_record ORDER BY called_at DESC LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]domain.PlaybackRecord, 0)
	for rows.Next() {
		var r domain.PlaybackRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.Poster, &r.URL, &r.Time, &r.IP, &r.Location, &r.Method); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// Metadata 通过原账号和 pickcode 找回文件名，并匹配已识别的海报。
func (d *PlaybackRecordDAO) Metadata(ctx context.Context, account int, pickcode, name string) (string, string, error) {
	var fileName string
	err := DB.QueryRowContext(ctx, `SELECT f.file_name FROM t_strm_file f JOIN t_strm_config c ON c.id=f.strm_config_id WHERE c.cloud115_id=$1 AND f.pick_code=$2 ORDER BY f.id DESC LIMIT 1`, account, pickcode).Scan(&fileName)
	if err != nil && err != sql.ErrNoRows {
		return name, "", err
	}
	if fileName != "" {
		name = fileName
	}
	var title, poster string
	err = DB.QueryRowContext(ctx, `SELECT title, COALESCE(poster_path,'') FROM t_identify_cache WHERE file_hash=$1`, FileHash(name)).Scan(&title, &poster)
	if err == sql.ErrNoRows {
		return name, "", nil
	}
	if err != nil {
		return name, "", err
	}
	if title != "" {
		name = title
	}
	return name, poster, nil
}

// GetLocation 读取 IP 归属地缓存。
func (d *PlaybackRecordDAO) GetLocation(ctx context.Context, ip string) string {
	value, _ := d.client.Get(ctx, "easy_strm:playback:geo:"+ip).Result()
	return value
}

// SetLocation 缓存归属地，减少外部查询。
func (d *PlaybackRecordDAO) SetLocation(ctx context.Context, ip, location string) error {
	return d.client.Set(ctx, "easy_strm:playback:geo:"+ip, location, 24*time.Hour).Err()
}
