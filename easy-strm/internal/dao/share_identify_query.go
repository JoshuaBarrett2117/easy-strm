package dao

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"github.com/lib/pq"
)

// ShareIdentifyFilter 定义任务候选筛选，强制刷新与失败重试使用不同语义。
type ShareIdentifyFilter struct {
	RecordIDs, MediaIDs                   []int
	PendingOnly, FailedOnly, ForceRefresh bool
}

// ListIdentifyMedia 在SQL中限定分享、文件和状态，不加载无关分享或媒体实体。
func (d *ShareRecordDAO) ListIdentifyMedia(ctx context.Context, filter ShareIdentifyFilter) ([]domain.ShareRecord, error) {
	where := "f.available AND NOT s.share_cancelled AND f.status NOT IN ('ignored','masked')"
	args := []interface{}{}
	if len(filter.RecordIDs) > 0 {
		args = append(args, pq.Array(filter.RecordIDs))
		where += fmt.Sprintf(" AND s.id=ANY($%d)", len(args))
	}
	if len(filter.MediaIDs) > 0 {
		args = append(args, pq.Array(filter.MediaIDs))
		where += fmt.Sprintf(" AND f.id=ANY($%d)", len(args))
	}
	switch {
	case filter.FailedOnly:
		where += " AND f.status='failed'"
	case filter.PendingOnly:
		where += " AND f.status IN ('pending','')"
	case !filter.ForceRefresh:
		where += " AND (f.status<>'identified' OR f.result IS NULL OR f.result::text='null' OR (s.media_type<>'auto' AND COALESCE(f.result->>'media_type','')<>s.media_type))"
	}
	rows, err := d.db.QueryContext(ctx, "SELECT s.id,s.media_type,f.id,f.file_name,f.metadata_source,f.status,f.result,f.version FROM t_share_record s JOIN t_share_media_file f ON f.share_id=s.id WHERE "+where+" ORDER BY s.id,f.id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShareIdentifyRows(rows)
}

// ListIdentifyContext 仅为候选分享读取文件名证据及已确认身份，不读取链接、密码或媒体详情视图。
func (d *ShareRecordDAO) ListIdentifyContext(ctx context.Context, recordIDs []int) ([]domain.ShareRecord, error) {
	if len(recordIDs) == 0 {
		return nil, nil
	}
	rows, err := d.db.QueryContext(ctx, `SELECT s.id,s.media_type,f.id,f.file_name,f.metadata_source,f.status,
 CASE WHEN f.status='identified' THEN f.result ELSE NULL END,f.version
 FROM t_share_record s JOIN t_share_media_file f ON f.share_id=s.id
 WHERE s.id=ANY($1) AND f.available AND NOT s.share_cancelled AND f.status NOT IN ('ignored','masked') ORDER BY s.id,f.id`, pq.Array(recordIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShareIdentifyRows(rows)
}

func scanShareIdentifyRows(rows *sql.Rows) ([]domain.ShareRecord, error) {
	records := []domain.ShareRecord{}
	indexes := map[int]int{}
	for rows.Next() {
		var id int
		var kind string
		var media domain.ShareMedia
		var raw []byte
		if err := rows.Scan(&id, &kind, &media.ID, &media.FileName, &media.MetadataSource, &media.Status, &raw, &media.Version); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &media.Result); err != nil {
				return nil, fmt.Errorf("解析分享媒体识别结果失败: %w", err)
			}
		}
		media.ShareID = id
		media.MediaType = kind
		media.Available = true
		index, ok := indexes[id]
		if !ok {
			index = len(records)
			indexes[id] = index
			records = append(records, domain.ShareRecord{ID: id, MediaType: kind})
		}
		records[index].Media = append(records[index].Media, media)
	}
	return records, rows.Err()
}
