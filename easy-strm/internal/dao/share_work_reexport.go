package dao

import (
	"context"
	"easy-strm/internal/domain"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lib/pq"
)

const shareWorkReexportSelectionsSQL = `SELECT ` + shareSelectionColumns + `,COALESCE(c.source_file_id,0),COALESCE(f.media_id,0)
 FROM t_share_media_selection s
 LEFT JOIN t_share_export_candidate c ON c.candidate_id=s.selected_candidate_id
 LEFT JOIN t_share_media_file f ON f.id=c.source_file_id
 LEFT JOIN t_share_media m ON m.id=f.media_id
 WHERE s.work_key=$1 ORDER BY s.season_number,s.episode_number,s.media_item_key LIMIT 10001`

// WorkReexportSelections 只读点查一个非空作品，复用有效成员谓词，按实际文件媒体关联以避免重复作品媒体行膨胀。
func (store *ShareRecordDAO) WorkReexportSelections(ctx context.Context, work string) ([]domain.ShareWorkReexportSelection, error) {
	if strings.TrimSpace(work) == "" {
		return nil, fmt.Errorf("重导作品不能为空")
	}
	rows, err := store.db.QueryContext(ctx, shareWorkReexportSelectionsSQL, work)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []domain.ShareWorkReexportSelection{}
	for rows.Next() {
		var value domain.ShareWorkReexportSelection
		selection := &value.Selection
		if err = rows.Scan(&selection.ItemKey, &selection.WorkKey, &selection.Season, &selection.Episode, &selection.CandidateID, &selection.Mode, &selection.Revision, &selection.ExportedRevision, &selection.RelativePath, &selection.Title, &selection.Valid, &value.SourceID, &value.MediaID); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

// WorkReexportGlobalInvalid 原样复用采纳计数 SQL，只读返回全局 invalid，不修改任何采纳护栏或检查点。
func (store *ShareExportCheckpointDAO) WorkReexportGlobalInvalid(ctx context.Context, root string) (int64, error) {
	var selections, outputs, pending, dirtySum, selectionSum, invalid int64
	if store.outputConn != nil {
		err := store.outputConn.QueryRowContext(ctx, shareBaselineCountsSQL, root).Scan(&selections, &outputs, &pending, &dirtySum, &selectionSum, &invalid)
		return invalid, err
	}
	err := store.db.QueryRowContext(ctx, shareBaselineCountsSQL, root).Scan(&selections, &outputs, &pending, &dirtySum, &selectionSum, &invalid)
	return invalid, err
}

const shareWorkReexportPreflightSQL = `WITH targets AS (
 SELECT * FROM unnest($1::text[],$2::bigint[],$3::text[],$4::text[],$5::text[])
 AS target(mapping_id,media_id,item_key,output_path,relative_path)
)
SELECT EXISTS(SELECT mapping_id FROM targets GROUP BY mapping_id HAVING COUNT(DISTINCT media_id)>1)
 OR EXISTS(
 SELECT 1 FROM t_strm_export_state output
 JOIN targets target ON (output.state='active' AND output.share_strm_id=target.mapping_id)
                     OR (output.state<>'missing' AND output.output_path=target.output_path)
 LEFT JOIN t_share_media_selection selection ON output.owner_key='share:default' AND selection.media_item_key=output.export_key
 LEFT JOIN t_share_export_candidate candidate ON candidate.candidate_id=selection.selected_candidate_id
 LEFT JOIN t_share_media_file file ON file.id=candidate.source_file_id
 WHERE selection.work_key IS NULL OR NOT(selection.work_key=ANY($6::text[]))
    OR (output.output_path=target.output_path AND output.export_key<>target.item_key)
    OR (output.state='active' AND output.share_strm_id=target.mapping_id AND file.media_id IS DISTINCT FROM target.media_id)
 )
 OR EXISTS(
 SELECT 1 FROM t_share_media_selection selection
 JOIN targets target ON selection.stable_relative_path=target.relative_path
 WHERE NOT(selection.work_key=ANY($6::text[])) OR selection.media_item_key<>target.item_key
 )
 OR EXISTS(
 SELECT 1 FROM t_strm_export_history history
 WHERE history.output_path=ANY($4::text[]) AND history.owner_key<>'share:default'
 )
 OR EXISTS(
 SELECT 1 FROM t_strm_file file
 WHERE file.local_strm_path=ANY($4::text[]) AND file.strm_config_id<>-1
 AND NOT EXISTS(SELECT 1 FROM t_strm_export_state output WHERE output.output_path=file.local_strm_path)
 )`

// WorkReexportPreflight 在既有 owner 锁连接上只读核验全部预期映射和路径的引用范围，不以旧 payload 媒体判错而阻止修复。
func (store *ShareExportCheckpointDAO) WorkReexportPreflight(ctx context.Context, root string, works []string, entries []domain.ShareStrmEntry, selections []domain.ShareSelection) error {
	if store.outputConn == nil || len(works) == 0 || len(works) > domain.ShareWorkReexportMaxWorks || len(entries) == 0 || len(entries) != len(selections) || len(entries) > domain.ShareWorkReexportMaxSelections {
		return fmt.Errorf("映射范围预检需要 owner 锁连接与明确有界的目标")
	}
	mappings := make([]string, len(entries))
	media := make([]int64, len(entries))
	items := make([]string, len(entries))
	paths := make([]string, len(entries))
	relatives := make([]string, len(entries))
	for index, entry := range entries {
		selection := selections[index]
		if entry.ID == "" || entry.MediaID <= 0 || selection.ItemKey == "" || selection.RelativePath == "" {
			return fmt.Errorf("映射范围预检目标不完整")
		}
		mappings[index], media[index], items[index] = entry.ID, int64(entry.MediaID), selection.ItemKey
		paths[index], relatives[index] = filepath.Join(root, selection.RelativePath), selection.RelativePath
	}
	var collision bool
	if err := store.outputConn.QueryRowContext(ctx, shareWorkReexportPreflightSQL, pq.Array(mappings), pq.Array(media), pq.Array(items), pq.Array(paths), pq.Array(relatives), pq.Array(works)).Scan(&collision); err != nil {
		return err
	}
	if collision {
		return fmt.Errorf("预期映射或稳定路径被请求范围外引用，或映射对应不同实际媒体：%w", ErrShareExportChanged)
	}
	return nil
}
