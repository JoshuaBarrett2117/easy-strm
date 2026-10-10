package dao

import (
	"context"
	"database/sql"
	"github.com/lib/pq"
)

// ShareDeletePath 包含导出文件路径及是否有明确的播放映射关联。
type ShareDeletePath struct {
	Path   string
	Linked bool
}

// ShareDeleteURL 获取待删除分享链接，用于兼容没有分享ID的历史播放映射。
func (d *ShareRecordDAO) ShareDeleteURL(ctx context.Context, id int) (string, error) {
	var url string
	err := d.db.QueryRowContext(ctx, "SELECT url FROM t_share_record WHERE id=$1", id).Scan(&url)
	return url, err
}

// ShareDeleteEntries 获取同一分享码的所有播放映射，包括清空媒体记录后留下的映射。
func (d *ShareRecordDAO) ShareDeleteEntries(ctx context.Context, code string) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT id FROM t_share_strm WHERE payload->>'share_code'=$1
 AND NOT EXISTS(SELECT 1 FROM t_strm_export_state e JOIN t_share_media_selection s ON s.media_item_key=e.export_key
 WHERE e.owner_key='share:default' AND e.share_strm_id=t_share_strm.id)`, code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ShareDeletePaths 查询当前及历史分享导出清单，历史路径由Service校验文件内容后确定归属。
func (d *ShareRecordDAO) ShareDeletePaths(ctx context.Context, entries []string) ([]ShareDeletePath, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT output_path,bool_or(linked) FROM (
 SELECT output_path,COALESCE(share_strm_id=ANY($1::text[]),FALSE) AS linked FROM t_strm_export_state WHERE owner_key='share:default'
 UNION ALL SELECT output_path,reason IN (SELECT 'share-delete:'||unnest($1::text[])) FROM t_strm_export_history WHERE owner_key='share:default'
 UNION ALL SELECT local_strm_path,FALSE FROM t_strm_file WHERE strm_config_id=-1
 ) paths WHERE COALESCE(output_path,'')<>'' GROUP BY output_path`, pq.Array(entries))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	paths := []ShareDeletePath{}
	for rows.Next() {
		var p ShareDeletePath
		if err = rows.Scan(&p.Path, &p.Linked); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// RememberShareDeletePaths 在删除文件前登记清理归属，部分失败或事务失败后仍能找回历史路径。
func (d *ShareRecordDAO) RememberShareDeletePaths(ctx context.Context, entry string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	_, err := d.db.ExecContext(ctx, `INSERT INTO t_strm_export_history(owner_key,output_path,reason)
 SELECT 'share:default',unnest($1::text[]),$2 ON CONFLICT(owner_key,output_path) DO UPDATE SET reason=EXCLUDED.reason`, pq.Array(paths), "share-delete:"+entry)
	return err
}

func deleteShareStrmRecords(ctx context.Context, tx *sql.Tx, entries, paths []string) error {
	if len(paths) > 0 {
		for _, query := range []string{
			"DELETE FROM t_strm_file WHERE strm_config_id=-1 AND local_strm_path=ANY($1::text[])",
			"DELETE FROM t_strm_export_history WHERE owner_key='share:default' AND output_path=ANY($1::text[])",
			"DELETE FROM t_strm_export_state WHERE owner_key='share:default' AND output_path=ANY($1::text[])",
		} {
			if _, err := tx.ExecContext(ctx, query, pq.Array(paths)); err != nil {
				return err
			}
		}
	}
	if len(entries) > 0 {
		_, err := tx.ExecContext(ctx, "DELETE FROM t_share_strm WHERE id=ANY($1::text[])", pq.Array(entries))
		return err
	}
	return nil
}
