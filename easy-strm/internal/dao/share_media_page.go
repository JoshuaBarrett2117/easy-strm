package dao

import (
	"context"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

func (d *ShareRecordDAO) listSummaries(ctx context.Context, q domain.ShareRecordQuery, where string, args []interface{}, total int) (domain.ShareRecordPage, error) {
	args = append(args, q.PageSize, (q.Page-1)*q.PageSize)
	parents := fmt.Sprintf(`SELECT id,media_type,name,url,password,note,version,created_at,updated_at,share_cancelled FROM t_share_record s%s ORDER BY s.id DESC LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args))
	query := `SELECT s.id,s.media_type,s.name,s.url,s.password,s.note,s.version,s.created_at::text,s.updated_at::text,s.share_cancelled,
	 a.file_count,a.identified,a.failed,a.pending,a.unavailable,a.media_count,a.ignored
	 FROM (` + parents + `) s CROSS JOIN LATERAL (
	  SELECT count(*) file_count,
	   count(*) FILTER (WHERE available AND status='identified' AND media_id IS NOT NULL) identified,
	   count(*) FILTER (WHERE available AND status='failed') failed,
	   count(*) FILTER (WHERE available AND status='pending') pending,
	   count(*) FILTER (WHERE NOT available) unavailable,
	   count(DISTINCT media_id) FILTER (WHERE available AND status='identified') media_count,
	   count(*) FILTER (WHERE status='ignored') ignored
	  FROM t_share_media_file WHERE share_id=s.id
	 ) a ORDER BY s.id DESC`
	out := domain.ShareRecordPage{Data: []domain.ShareRecord{}, Total: total}
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var r domain.ShareRecord
		if err = rows.Scan(&r.ID, &r.MediaType, &r.Name, &r.URL, &r.Password, &r.Note, &r.Version, &r.CreatedAt, &r.UpdatedAt, &r.ShareCancelled, &r.FileCount, &r.IdentifiedCount, &r.FailedCount, &r.PendingCount, &r.UnavailableCount, &r.MediaCount, &r.MaskedCount); err != nil {
			return out, err
		}
		r.Media = []domain.ShareMedia{}
		out.Data = append(out.Data, r)
	}
	return out, rows.Err()
}

// ListMedia 按媒体主键聚合海报墙；同一作品在一个分享中只返回一次。
func (d *ShareRecordDAO) ListMedia(ctx context.Context, id, page, size int, duplicates bool) (domain.ShareMediaPage, error) {
	out := domain.ShareMediaPage{Data: []domain.ShareMedia{}}
	query := `WITH ranked AS (
	 SELECT f.id,m.id media_id,f.share_id,f.file_name,m.metadata_source,'identified' status,m.result,f.version,
	  count(*) OVER(PARTITION BY m.id) file_count,row_number() OVER(PARTITION BY m.id ORDER BY f.id) ordinal
	 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id
	 WHERE f.share_id=$1 AND f.available AND f.status='identified'
	), cards AS (SELECT * FROM ranked WHERE ordinal=1)
	SELECT (SELECT count(*) FROM cards),(SELECT count(*) FROM ranked)-(SELECT count(*) FROM cards),
	 COALESCE((SELECT json_agg(p) FROM (SELECT id,media_id,share_id,file_name,metadata_source,status,result,version,file_count,TRUE available
	 FROM cards ORDER BY media_id LIMIT $2 OFFSET $3)p),'[]'::json)`
	var data []byte
	if err := d.db.QueryRowContext(ctx, query, id, size, (page-1)*size).Scan(&out.Total, &out.DuplicateCount, &data); err != nil {
		return out, err
	}
	return out, json.Unmarshal(data, &out.Data)
}

// ListFiles 返回分享下的真实文件、识别状态、媒体关联与季集映射。
func (d *ShareRecordDAO) ListFiles(ctx context.Context, id, page, size int) (domain.ShareMediaPage, error) {
	out := domain.ShareMediaPage{Data: []domain.ShareMedia{}}
	var data []byte
	err := d.db.QueryRowContext(ctx, `SELECT count(*),COALESCE((SELECT json_agg(p) FROM (
	 SELECT f.id,f.media_id,f.share_id,f.file_id remote_file_id,f.file_name,f.file_size,f.sha1,f.pick_code,f.available,
	  f.metadata_source,f.status,f.result,f.error,f.version,
	  COALESCE((SELECT json_agg(json_build_object('season_number',e.season_number,'episode_number',e.episode_number)
	   ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),'[]'::json) episodes
	 FROM t_share_media_file f WHERE f.share_id=$1 ORDER BY f.id LIMIT $2 OFFSET $3)p),'[]'::json)
	 FROM t_share_media_file WHERE share_id=$1`, id, size, (page-1)*size).Scan(&out.Total, &data)
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(data, &out.Data)
}

// ListReviewItems 返回跨分享的失败和待识别媒体，供手动核对中心分页使用。
func (d *ShareRecordDAO) ListReviewItems(ctx context.Context, statuses []string, keyword string, shareID, mediaID, page, size int) (domain.ShareReviewPage, error) {
	out := domain.ShareReviewPage{Data: []domain.ShareReviewItem{}}
	where := []string{"f.available", "f.status = ANY($1::text[])"}
	args := []interface{}{pq.Array(statuses)}
	if shareID > 0 {
		args = append(args, shareID)
		where = append(where, fmt.Sprintf("f.share_id=$%d", len(args)))
	}
	if keyword != "" {
		args = append(args, "%"+keyword+"%")
		placeholder := fmt.Sprintf("$%d", len(args))
		where = append(where, fmt.Sprintf("(s.name ILIKE %s OR f.file_name ILIKE %s OR COALESCE(f.error,'') ILIKE %s OR COALESCE(f.result->>'title','') ILIKE %s)", placeholder, placeholder, placeholder, placeholder))
	}
	if mediaID > 0 {
		args = append(args, mediaID)
		where = append(where, fmt.Sprintf("f.id=$%d", len(args)))
	}
	whereSQL := strings.Join(where, " AND ")
	if err := d.db.QueryRowContext(ctx, "SELECT count(*) FROM t_share_media_file f JOIN t_share_record s ON s.id=f.share_id WHERE "+whereSQL, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	dataArgs := append([]interface{}{}, args...)
	dataArgs = append(dataArgs, size, (page-1)*size)
	query := fmt.Sprintf(`SELECT COALESCE(json_agg(p),'[]'::json) FROM (
	 SELECT f.id,f.share_id,s.name share_name,s.media_type,f.file_name,f.file_size,f.available,f.metadata_source,f.status,f.result,f.error,f.version,
	  COALESCE((SELECT json_agg(json_build_object('season_number',e.season_number,'episode_number',e.episode_number) ORDER BY e.season_number,e.episode_number)
	   FROM t_share_media_file_episode e WHERE e.file_id=f.id),'[]'::json) episodes
	 FROM t_share_media_file f JOIN t_share_record s ON s.id=f.share_id
	 WHERE %s
	 ORDER BY CASE WHEN f.status='failed' THEN 0 ELSE 1 END, f.updated_at DESC, f.id DESC
	 LIMIT $%d OFFSET $%d
	)p`, whereSQL, len(dataArgs)-1, len(dataArgs))
	var data []byte
	if err := d.db.QueryRowContext(ctx, query, dataArgs...).Scan(&data); err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out.Data); err != nil {
		return out, err
	}
	return out, nil
}
