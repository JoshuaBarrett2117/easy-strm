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
	var title, poster, mediaType string
	var tmdbID sql.NullInt64
	autoHash := FileHash(name)
	tmdbHash := FileHash(domain.MetadataSourceTMDB + ":" + name)
	metaTubeHash := FileHash(domain.MetadataSourceMetaTube + ":" + name)
	err = DB.QueryRowContext(ctx, `SELECT i.title,COALESCE(i.poster_path,''),i.tmdb_id,i.media_type
		FROM t_identify_cache i LEFT JOIN t_media_source s ON s.id=i.source_id
		WHERE i.file_hash IN ($1,$2,$3) OR lower(i.file_name)=lower($4)
		ORDER BY (s.cloud115_id=$5) DESC,(i.file_hash=$1) DESC,i.is_manual DESC,i.updated_at DESC,i.id DESC LIMIT 1`, autoHash, tmdbHash, metaTubeHash, name, account).Scan(&title, &poster, &tmdbID, &mediaType)
	if err == sql.ErrNoRows {
		return name, "", nil
	}
	if err != nil {
		return name, "", err
	}
	if title != "" {
		name = title
	}
	// 兼容旧版本识别缓存漏存海报的记录，使用已缓存的 TMDB 身份回查，不发起网络请求。
	if poster == "" && tmdbID.Valid && tmdbID.Int64 > 0 {
		var cachedTitle, cachedPoster string
		cacheErr := DB.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(title,''),$3),COALESCE(poster_path,'') FROM t_tmdb_cache
			WHERE tmdb_id=$1 AND media_type=$2 AND COALESCE(poster_path,'')<>'' ORDER BY update_time DESC,id DESC LIMIT 1`, tmdbID.Int64, mediaType, name).Scan(&cachedTitle, &cachedPoster)
		if cacheErr != nil && cacheErr != sql.ErrNoRows {
			return name, "", cacheErr
		}
		if cacheErr == nil {
			name, poster = cachedTitle, cachedPoster
		}
	}
	return name, poster, nil
}

// ShareMetadata 优先读取导出映射快照，并兼容从历史映射关联分享媒体主数据。
func (d *PlaybackRecordDAO) ShareMetadata(ctx context.Context, entryID string) (string, string, error) {
	if DB == nil {
		return entryID, "", fmt.Errorf("数据库未初始化")
	}
	var title, poster string
	err := DB.QueryRowContext(ctx, `SELECT
		COALESCE(NULLIF(e.payload->>'title',''), NULLIF(metadata.title,''), NULLIF(e.payload->>'file_name',''), e.id::text),
		COALESCE(NULLIF(e.payload->>'poster_path',''), NULLIF(metadata.poster_path,''), '')
	FROM t_share_strm e
	LEFT JOIN LATERAL (
		SELECT m.title,m.poster_path
		FROM t_share_media_file f
		JOIN t_share_media m ON m.id=f.media_id
		LEFT JOIN t_share_record s ON s.id=f.share_id
		WHERE (COALESCE(e.payload->>'media_id','') ~ '^[1-9][0-9]*$' AND m.id=(e.payload->>'media_id')::integer)
		   OR (COALESCE(e.payload->>'share_code','')<>'' AND POSITION(e.payload->>'share_code' IN COALESCE(s.url,''))>0
		       AND ((COALESCE(e.payload->>'file_id','')<>'' AND f.file_id=e.payload->>'file_id')
		            OR f.file_name IN (e.payload->>'file_path',e.payload->>'file_name')))
		ORDER BY CASE WHEN COALESCE(e.payload->>'media_id','') ~ '^[1-9][0-9]*$' AND m.id=(e.payload->>'media_id')::integer THEN 0 ELSE 1 END, f.id DESC
		LIMIT 1
	) metadata ON TRUE
	WHERE e.id=$1`, entryID).Scan(&title, &poster)
	return title, poster, err
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
