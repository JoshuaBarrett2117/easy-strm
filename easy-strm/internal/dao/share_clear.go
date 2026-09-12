package dao

import "context"

// ClearMedia 原子清除指定分享的全部媒体行，不删除分享配置或其他分享的数据。
func (d *ShareRecordDAO) ClearMedia(ctx context.Context, id int) (int64, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var found int
	if err = tx.QueryRowContext(ctx, "SELECT id FROM t_share_record WHERE id=$1 FOR UPDATE", id).Scan(&found); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM t_share_media_file WHERE share_id=$1", id)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM t_share_media m WHERE NOT EXISTS (SELECT 1 FROM t_share_media_file f WHERE f.media_id=m.id)"); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

// ClearAllMedia 清除全部分享的媒体候选及识别结果，不删除分享配置。
func (d *ShareRecordDAO) ClearAllMedia(ctx context.Context) (int64, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "DELETE FROM t_share_media_file")
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM t_share_media"); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}
