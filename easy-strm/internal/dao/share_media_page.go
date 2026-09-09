package dao

import (
	"context"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
)

// 路径脱敏判断与历史识别结果归一化保持一致。
const unmaskedMedia = `file_name NOT LIKE '%**%' AND file_name NOT LIKE '%＊＊%'`
const identifiedMedia = `status='identified' AND result->>'success'='true' AND ` + unmaskedMedia

func (d *ShareRecordDAO) listSummaries(ctx context.Context, q domain.ShareRecordQuery, where string, args []interface{}, total int) (domain.ShareRecordPage, error) {
	args = append(args, q.PageSize, (q.Page-1)*q.PageSize)
	parents := fmt.Sprintf(`SELECT id,media_type,name,url,password,note,version,created_at,updated_at,share_cancelled FROM t_share_record s%s ORDER BY s.id DESC LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args))
	query := `SELECT s.id,s.media_type,s.name,s.url,s.password,s.note,s.version,s.created_at::text,s.updated_at::text,s.share_cancelled,
 a.total,a.identified,a.failed,a.pending,a.masked
 FROM (` + parents + `) s
 CROSS JOIN LATERAL (SELECT count(*) total,
 count(*) FILTER (WHERE ` + identifiedMedia + `) identified,
 count(*) FILTER (WHERE status='failed' AND ` + unmaskedMedia + `) failed,
 count(*) FILTER (WHERE status NOT IN ('identified','failed','masked') AND ` + unmaskedMedia + `) pending,
 count(*) FILTER (WHERE NOT (` + unmaskedMedia + `)) masked
 FROM t_share_media WHERE share_id=s.id) a ORDER BY s.id DESC`
	out := domain.ShareRecordPage{Data: []domain.ShareRecord{}, Total: total}
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var r domain.ShareRecord
		if err = rows.Scan(&r.ID, &r.MediaType, &r.Name, &r.URL, &r.Password, &r.Note, &r.Version, &r.CreatedAt, &r.UpdatedAt, &r.ShareCancelled, &r.MediaCount, &r.IdentifiedCount, &r.FailedCount, &r.PendingCount, &r.MaskedCount); err != nil {
			return out, err
		}
		r.Media = []domain.ShareMedia{}
		out.Data = append(out.Data, r)
	}
	return out, rows.Err()
}

// 分组先于分页，保证跨页剧集仍然合并；没有可信身份的媒体各自独立。
const galleryRows = `WITH ranked AS (
 SELECT id,share_id,file_name,metadata_source,status,result,error,version,
 row_number() OVER (PARTITION BY CASE
 WHEN result->>'media_type'='tv' AND COALESCE((result->>'tmdb_id')::bigint,0)>0 THEN 'tmdb:tv:'||(result->>'tmdb_id')
 WHEN result->>'media_type'='tv' AND COALESCE(result->>'metadata_source','')<>'' AND COALESCE(result->>'metadata_id','')<>'' THEN (result->>'metadata_source')||':tv:'||(result->>'metadata_id')
 ELSE 'row:'||id::text END ORDER BY id) AS ordinal
 FROM t_share_media WHERE share_id=$1 AND ` + identifiedMedia + `)
 `

// ListMedia 只将当前页的识别详情读入内存，计数与详情使用同一数据库快照。
func (d *ShareRecordDAO) ListMedia(ctx context.Context, id, page, size int, duplicates bool) (domain.ShareMediaPage, error) {
	out := domain.ShareMediaPage{Data: []domain.ShareMedia{}}
	query := galleryRows + `SELECT (SELECT count(*) FROM ranked WHERE $2 OR ordinal=1),
 (SELECT count(*) FROM ranked WHERE ordinal>1),
 COALESCE((SELECT json_agg(p) FROM (SELECT id,share_id,file_name,metadata_source,status,result,error,version,ordinal>1 gallery_duplicate
 FROM ranked WHERE $2 OR ordinal=1 ORDER BY id LIMIT $3 OFFSET $4) p),'[]'::json)`
	var data []byte
	err := d.db.QueryRowContext(ctx, query, id, duplicates, size, (page-1)*size).Scan(&out.Total, &out.DuplicateCount, &data)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(data, &out.Data)
	return out, err
}
