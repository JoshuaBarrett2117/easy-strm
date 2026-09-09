package dao

import (
	"context"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"strings"
)

// 每个作品取最近一次识别结果作为展示元数据，来源计数在聚合前计算。
const libraryWorks = `WITH ranked AS (
 SELECT m.work_key,m.tmdb_id,m.title,m.library_media_type,m.media_year,m.rating,m.result,m.genre_ids,m.country_codes,m.share_id,m.created_at,s.share_cancelled,
 row_number() OVER(PARTITION BY work_key ORDER BY m.updated_at DESC,m.id DESC) ordinal
 FROM t_share_media m JOIN t_share_record s ON s.id=m.share_id WHERE work_key<>''
), stats AS (SELECT work_key,count(DISTINCT share_id) source_count,max(created_at) collected_at,bool_or(NOT share_cancelled) available FROM ranked GROUP BY work_key),
 works AS (SELECT r.work_key,r.tmdb_id,r.title,r.library_media_type,r.media_year,r.rating,r.result,r.genre_ids,r.country_codes,s.source_count,s.collected_at,s.available FROM ranked r JOIN stats s ON s.work_key=r.work_key WHERE r.ordinal=1) `

func libraryWhere(q domain.ShareLibraryQuery) (string, []interface{}) {
	parts, args := []string{"TRUE"}, []interface{}{}
	add := func(expr string, val interface{}) {
		args = append(args, val)
		parts = append(parts, fmt.Sprintf(expr, len(args)))
	}
	if q.Keyword != "" {
		add("(title ILIKE '%%'||$%[1]d||'%%' OR result->>'original_title' ILIKE '%%'||$%[1]d||'%%')", q.Keyword)
	}
	if q.TmdbID > 0 {
		add("tmdb_id=$%d", q.TmdbID)
	}
	if q.MediaType != "" {
		add("library_media_type=$%d", q.MediaType)
	}
	if q.YearMin > 0 {
		add("media_year >= $%d", q.YearMin)
	}
	if q.YearMax > 0 {
		add("media_year <= $%d", q.YearMax)
	}
	if q.RatingMin != nil {
		add("rating >= $%d", *q.RatingMin)
	}
	if q.RatingMax != nil {
		add("rating <= $%d", *q.RatingMax)
	}
	if q.Genres != "" {
		add("genre_ids && string_to_array($%d,',')::integer[]", q.Genres)
	}
	if q.Countries != "" {
		add("country_codes && string_to_array($%d,',')", strings.ToUpper(q.Countries))
	}
	if q.Available {
		parts = append(parts, "available")
	}
	return strings.Join(parts, " AND "), args
}

// Library 查询作品，计数和页面在同一语句快照中返回。
func (d *ShareRecordDAO) Library(ctx context.Context, q domain.ShareLibraryQuery) (domain.ShareLibraryPage, error) {
	where, args := libraryWhere(q)
	sort := map[string]string{"created": "collected_at", "year": "media_year", "rating": "rating", "title": "title"}[q.Sort]
	if sort == "" {
		sort = "collected_at"
	}
	direction := "DESC"
	if q.Direction == "asc" {
		direction = "ASC"
	}
	args = append(args, q.PageSize, (q.Page-1)*q.PageSize)
	sql := libraryWorks + ` ,filtered AS (SELECT * FROM works WHERE ` + where + `) SELECT (SELECT count(*) FROM filtered),
 COALESCE((SELECT json_agg(p) FROM (SELECT work_key,tmdb_id,title,library_media_type AS media_type,media_year AS "year",rating,result->>'poster_path' AS poster_path,genre_ids,country_codes,source_count,available,collected_at
 FROM filtered ORDER BY ` + sort + ` ` + direction + ` NULLS LAST,work_key ASC LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args)) + `) p),'[]'::json)`
	return d.libraryPage(ctx, sql, args...)
}

func (d *ShareRecordDAO) libraryPage(ctx context.Context, query string, args ...interface{}) (domain.ShareLibraryPage, error) {
	out := domain.ShareLibraryPage{}
	var raw []byte
	err := d.db.QueryRowContext(ctx, query, args...).Scan(&out.Total, &raw)
	out.Data = json.RawMessage(raw)
	return out, err
}

// LibrarySources 返回作品全部来源，并保留失效分享。
func (d *ShareRecordDAO) LibrarySources(ctx context.Context, key string, page, size int) (domain.ShareLibraryPage, error) {
	return d.libraryPage(ctx, `WITH sources AS (SELECT m.id,m.share_id,m.file_name,s.name,s.url,s.password,s.share_cancelled FROM t_share_media m JOIN t_share_record s ON s.id=m.share_id WHERE m.work_key=$1)
 SELECT (SELECT count(*) FROM sources),COALESCE((SELECT json_agg(p) FROM (SELECT * FROM sources ORDER BY share_id DESC,id LIMIT $2 OFFSET $3)p),'[]'::json)`, key, size, (page-1)*size)
}

// LibraryOptions 仅返回已收录作品的可选维度。
func (d *ShareRecordDAO) LibraryOptions(ctx context.Context) (json.RawMessage, error) {
	var raw []byte
	err := d.db.QueryRowContext(ctx, `SELECT json_build_object('genres',ARRAY(SELECT DISTINCT unnest(genre_ids) FROM t_share_media WHERE work_key<>'' ORDER BY 1),'countries',ARRAY(SELECT DISTINCT unnest(country_codes) FROM t_share_media WHERE work_key<>'' ORDER BY 1),'years',ARRAY(SELECT DISTINCT media_year FROM t_share_media WHERE work_key<>'' AND media_year IS NOT NULL ORDER BY 1 DESC))`).Scan(&raw)
	return json.RawMessage(raw), err
}

// LibraryIncomplete 分批读取待补全作品的来源，避免覆盖读取后发生的人工修正。
func (d *ShareRecordDAO) LibraryIncomplete(ctx context.Context, after string) (string, []domain.ShareMedia, error) {
	var key string
	err := d.db.QueryRowContext(ctx, `SELECT work_key FROM t_share_media WHERE work_key>$1 AND (rating IS NULL OR cardinality(genre_ids)=0 OR cardinality(country_codes)=0 OR media_year IS NULL OR title='') AND (tmdb_id>0 OR COALESCE(result->>'metadata_id','')<>'') ORDER BY work_key LIMIT 1`, after).Scan(&key)
	if err != nil {
		return "", nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT id,version,file_name,result FROM t_share_media WHERE work_key=$1 AND (rating IS NULL OR cardinality(genre_ids)=0 OR cardinality(country_codes)=0 OR media_year IS NULL OR title='') ORDER BY id`, key)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	media := []domain.ShareMedia{}
	for rows.Next() {
		var m domain.ShareMedia
		var raw []byte
		if err = rows.Scan(&m.ID, &m.Version, &m.FileName, &raw); err != nil {
			return "", nil, err
		}
		if err = json.Unmarshal(raw, &m.Result); err != nil {
			return "", nil, err
		}
		media = append(media, m)
	}
	return key, media, rows.Err()
}

// CountLibraryIncomplete 返回补全开始时的待处理数量，进度不使用不断变化的已处理数充当总数。
func (d *ShareRecordDAO) CountLibraryIncomplete(ctx context.Context)(int,error){
	var total int
	err:=d.db.QueryRowContext(ctx,`SELECT count(*) FROM t_share_media WHERE work_key<>'' AND (rating IS NULL OR cardinality(genre_ids)=0 OR cardinality(country_codes)=0 OR media_year IS NULL OR title='') AND (tmdb_id>0 OR COALESCE(result->>'metadata_id','')<>'')`).Scan(&total)
	return total,err
}
