package dao

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

const libraryWorks = `WITH stats AS (
 SELECT f.media_id,
  count(DISTINCT f.share_id) FILTER (WHERE f.available AND NOT s.share_cancelled) source_count,
  count(*) FILTER (WHERE f.available AND NOT s.share_cancelled) file_count,
  max(f.created_at) collected_at
 FROM t_share_media_file f JOIN t_share_record s ON s.id=f.share_id
 WHERE f.status='identified' AND f.media_id IS NOT NULL GROUP BY f.media_id
), works AS (
 SELECT m.id,m.work_key,m.tmdb_id,m.title,m.original_title,m.media_type library_media_type,m.media_year,m.rating,
  m.result,m.poster_path,m.genre_ids,m.country_codes,st.source_count,st.file_count,st.collected_at,(st.source_count>0) available
 FROM t_share_media m JOIN stats st ON st.media_id=m.id
) `

func libraryWhere(q domain.ShareLibraryQuery) (string, []interface{}) {
	parts, args := []string{"TRUE"}, []interface{}{}
	add := func(expr string, val interface{}) {
		args = append(args, val)
		parts = append(parts, fmt.Sprintf(expr, len(args)))
	}
	if q.Keyword != "" {
		add("(title ILIKE '%%'||$%[1]d||'%%' OR original_title ILIKE '%%'||$%[1]d||'%%')", q.Keyword)
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

// Library 查询全局媒体主数据，文件只负责来源和可用性统计。
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
	query := libraryWorks + `,filtered AS (SELECT * FROM works WHERE ` + where + `) SELECT (SELECT count(*) FROM filtered),
	 COALESCE((SELECT json_agg(p) FROM (SELECT work_key,tmdb_id,title,library_media_type AS media_type,media_year AS "year",rating,poster_path,genre_ids,country_codes,source_count,file_count,available,collected_at
	 FROM filtered ORDER BY ` + sort + ` ` + direction + ` NULLS LAST,work_key ASC LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args)) + `) p),'[]'::json)`
	return d.libraryPage(ctx, query, args...)
}

func (d *ShareRecordDAO) libraryPage(ctx context.Context, query string, args ...interface{}) (domain.ShareLibraryPage, error) {
	out := domain.ShareLibraryPage{}
	var raw []byte
	err := d.db.QueryRowContext(ctx, query, args...).Scan(&out.Total, &raw)
	out.Data = json.RawMessage(raw)
	return out, err
}

// LibrarySources 返回作品的真实文件来源以及持久化季集映射。
func (d *ShareRecordDAO) LibrarySources(ctx context.Context, key string, page, size int) (domain.ShareLibraryPage, error) {
	return d.libraryPage(ctx, `WITH sources AS (
	 SELECT f.id,f.share_id,f.file_id remote_file_id,f.file_name,f.file_size,f.available,f.status,s.name,s.url,s.password,s.share_cancelled,
	  COALESCE((SELECT json_agg(json_build_object('season_number',e.season_number,'episode_number',e.episode_number) ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),'[]'::json) episodes
	 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record s ON s.id=f.share_id WHERE m.work_key=$1)
	 SELECT (SELECT count(*) FROM sources),COALESCE((SELECT json_agg(p) FROM (SELECT * FROM sources ORDER BY available DESC,share_id DESC,id LIMIT $2 OFFSET $3)p),'[]'::json)`, key, size, (page-1)*size)
}

// LibraryTVMedia 查询剧集的服务端身份，避免调用方直接指定外部媒体 ID。
func (d *ShareRecordDAO) LibraryTVMedia(ctx context.Context, key string) (domain.ShareLibraryTVMedia, error) {
	var out domain.ShareLibraryTVMedia
	var tmdbID sql.NullInt64
	err := d.db.QueryRowContext(ctx, `SELECT work_key,tmdb_id,title,media_type,metadata_source FROM t_share_media WHERE work_key=$1`, key).
		Scan(&out.WorkKey, &tmdbID, &out.Title, &out.MediaType, &out.MetadataSource)
	if tmdbID.Valid {
		out.TmdbID = tmdbID.Int64
	}
	return out, err
}

// LibraryTVSeasonStats 返回本地季集映射的覆盖统计。
func (d *ShareRecordDAO) LibraryTVSeasonStats(ctx context.Context, key string) ([]domain.ShareLibraryTVSeasonSummary, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT e.season_number,count(DISTINCT e.episode_number),count(DISTINCT f.id)
	 FROM t_share_media_file_episode e JOIN t_share_media_file f ON f.id=e.file_id
	 JOIN t_share_media m ON m.id=f.media_id WHERE m.work_key=$1
	 GROUP BY e.season_number ORDER BY e.season_number`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.ShareLibraryTVSeasonSummary, 0)
	for rows.Next() {
		var item domain.ShareLibraryTVSeasonSummary
		if err = rows.Scan(&item.SeasonNumber, &item.MatchedEpisodeCount, &item.FileCount); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// LibraryTVSeasonFiles 返回一季中每集关联的真实分享文件。
func (d *ShareRecordDAO) LibraryTVSeasonFiles(ctx context.Context, key string, season int) (map[int][]domain.ShareLibraryFile, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT e.episode_number,f.id,f.share_id,f.file_id,f.file_name,f.file_size,
	 f.available,f.status,s.name,s.url,s.password,s.share_cancelled
	 FROM t_share_media_file_episode e JOIN t_share_media_file f ON f.id=e.file_id
	 JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record s ON s.id=f.share_id
	 WHERE m.work_key=$1 AND e.season_number=$2
	 ORDER BY e.episode_number,f.available DESC,s.share_cancelled ASC,f.share_id DESC,f.id`, key, season)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int][]domain.ShareLibraryFile)
	for rows.Next() {
		var episode int
		var file domain.ShareLibraryFile
		if err = rows.Scan(&episode, &file.ID, &file.ShareID, &file.RemoteFileID, &file.FileName, &file.FileSize,
			&file.Available, &file.Status, &file.Name, &file.URL, &file.Password, &file.ShareCancelled); err != nil {
			return nil, err
		}
		out[episode] = append(out[episode], file)
	}
	return out, rows.Err()
}

func (d *ShareRecordDAO) LibraryOptions(ctx context.Context) (json.RawMessage, error) {
	var raw []byte
	err := d.db.QueryRowContext(ctx, `SELECT json_build_object('genres',ARRAY(SELECT DISTINCT unnest(genre_ids) FROM t_share_media ORDER BY 1),'countries',ARRAY(SELECT DISTINCT unnest(country_codes) FROM t_share_media ORDER BY 1),'years',ARRAY(SELECT DISTINCT media_year FROM t_share_media WHERE media_year IS NOT NULL ORDER BY 1 DESC))`).Scan(&raw)
	return json.RawMessage(raw), err
}

// LibraryIncomplete 每次读取一个待补全的媒体主记录。
func (d *ShareRecordDAO) LibraryIncomplete(ctx context.Context, after string) (string, []domain.ShareMedia, error) {
	var key string
	var m domain.ShareMedia
	var raw []byte
	err := d.db.QueryRowContext(ctx, `SELECT work_key,id,version,result FROM t_share_media WHERE work_key>$1 AND (rating IS NULL OR cardinality(genre_ids)=0 OR cardinality(country_codes)=0 OR media_year IS NULL OR title='') AND (tmdb_id>0 OR external_id<>'') ORDER BY work_key LIMIT 1`, after).Scan(&key, &m.ID, &m.Version, &raw)
	if err != nil {
		return "", nil, err
	}
	if err = json.Unmarshal(raw, &m.Result); err != nil {
		return "", nil, err
	}
	return key, []domain.ShareMedia{m}, nil
}

// UpdateMediaMetadata 只更新媒体主数据，不改动任何文件识别关系。
func (d *ShareRecordDAO) UpdateMediaMetadata(ctx context.Context, m domain.ShareMedia) error {
	raw, err := json.Marshal(m.Result)
	if err != nil {
		return err
	}
	res, err := d.db.ExecContext(ctx, `UPDATE t_share_media SET title=$1,original_title=$2,media_year=NULLIF($3,0),poster_path=$4,result=$5,genre_ids=$6,country_codes=$7,rating=$8,version=version+1,updated_at=now() WHERE id=$9 AND version=$10`, m.Result.Title, m.Result.OriginalTitle, m.Result.Year, m.Result.PosterPath, raw, pq.Array(m.Result.GenreIDs), pq.Array(m.Result.Countries), m.Result.VoteAverage, m.ID, m.Version)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("媒体已更新，请重新补全")
	}
	return nil
}

func (d *ShareRecordDAO) CountLibraryIncomplete(ctx context.Context) (int, error) {
	var total int
	err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM t_share_media WHERE rating IS NULL OR cardinality(genre_ids)=0 OR cardinality(country_codes)=0 OR media_year IS NULL OR title=''`).Scan(&total)
	return total, err
}
