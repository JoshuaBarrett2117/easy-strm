package dao

import (
	"context"
	"database/sql"
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
	values := []domain.ShareWorkReexportSelection{}
	for rows.Next() {
		var value domain.ShareWorkReexportSelection
		selection := &value.Selection
		var candidate sql.NullInt64
		if err = rows.Scan(&selection.ItemKey, &selection.WorkKey, &selection.Season, &selection.Episode, &candidate, &selection.Mode, &selection.Revision, &selection.ExportedRevision, &selection.RelativePath, &selection.SourceKind, &selection.ExternalRef, &selection.Title, &selection.Valid, &value.SourceID, &value.MediaID); err != nil {
			rows.Close()
			return nil, err
		}
		selection.CandidateID = candidate.Int64
		values = append(values, value)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for index := range values {
		if values[index].Selection.SourceKind == "strm115" {
			return nil, domain.ErrStrm115MetadataUnsupported
		}
	}
	return values, nil
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
 SELECT * FROM unnest($1::text[],$2::bigint[],$3::text[],$4::text[],$5::text[],$9::text[])
 AS target(mapping_id,media_id,item_key,output_path,relative_path,item_mapping_id)
), urls AS (
 SELECT * FROM unnest($3::text[],$8::text[]) AS url(item_key,url_mapping_id)
), scoped AS (
 SELECT target.*,old.share_strm_id AS old_mapping_id,url.url_mapping_id FROM targets target
 JOIN urls url ON url.item_key=target.item_key
 LEFT JOIN t_strm_export_state old ON old.owner_key='share:default'
 AND old.export_key=target.item_key AND old.state='active'
), reference_rows AS (
 SELECT DISTINCT 'state' AS kind,output.owner_key,output.export_key,COALESCE(selection.work_key,'') AS work_key,
 CASE WHEN selection.work_key IS NULL THEN 'missing_selection'
 WHEN btrim(selection.work_key)='' THEN 'empty_work_key'
 WHEN NOT(selection.work_key=ANY($6::text[])) THEN 'outside_work_scope'
 WHEN output.owner_key<>'share:default' THEN 'foreign_owner'
 WHEN output.output_path=target.output_path AND output.export_key<>target.item_key THEN 'C2_output_path'
 WHEN output.state='active' AND NOT(output.export_key=ANY($3::text[])) THEN 'unprocessed_export_key'
 ELSE '' END AS reason
 FROM t_strm_export_state output JOIN scoped target
 ON (output.state='active' AND (output.share_strm_id=target.mapping_id OR output.share_strm_id=target.item_mapping_id OR output.share_strm_id=target.old_mapping_id OR output.share_strm_id=target.url_mapping_id))
 OR (output.state<>'missing' AND output.output_path=target.output_path)
 LEFT JOIN t_share_media_selection selection ON output.owner_key='share:default' AND selection.media_item_key=output.export_key
 UNION
 SELECT 'selection','share:default',selection.media_item_key,COALESCE(selection.work_key,''),
 CASE WHEN btrim(COALESCE(selection.work_key,''))='' THEN 'empty_work_key'
 WHEN NOT(selection.work_key=ANY($6::text[])) THEN 'outside_work_scope'
 WHEN selection.media_item_key<>target.item_key THEN 'C4_stable_relative_path' ELSE '' END
 FROM t_share_media_selection selection JOIN targets target ON selection.stable_relative_path=target.relative_path
 UNION
 SELECT 'history',history.owner_key,history.output_path,'','C5_history_owner' FROM t_strm_export_history history
 WHERE history.output_path=ANY($4::text[]) AND history.owner_key<>'share:default'
 UNION
 SELECT 'strm_file','cloud115:'||file.strm_config_id::text,file.local_strm_path,'','C6_local_strm_path' FROM t_strm_file file
 WHERE file.local_strm_path=ANY($4::text[]) AND file.strm_config_id<>-1
 AND NOT EXISTS(SELECT 1 FROM t_strm_export_state output WHERE output.output_path=file.local_strm_path)
), deduplicated AS (
 SELECT kind,owner_key,export_key,work_key,string_agg(DISTINCT NULLIF(reason,''),',' ORDER BY NULLIF(reason,'')) AS reason
 FROM reference_rows GROUP BY kind,owner_key,export_key,work_key
), totals AS (
 SELECT COUNT(kind) AS total,COUNT(reason) AS rejected FROM deduplicated
), samples AS (
 SELECT work_key,export_key,COALESCE(reason,'reference_limit') AS reason FROM deduplicated
 WHERE reason IS NOT NULL OR (SELECT total FROM totals)>$7
 ORDER BY work_key,export_key,reason LIMIT 100
)
SELECT totals.total,totals.rejected,COALESCE(samples.work_key,''),COALESCE(samples.export_key,''),COALESCE(samples.reason,'')
FROM totals LEFT JOIN samples ON TRUE`

type workReexportQuery interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}

// WorkReexportPreflight 在既有 owner 锁连接上只读核验全部预期映射和路径的引用范围，不以旧 payload 媒体判错而阻止修复。
func (store *ShareExportCheckpointDAO) WorkReexportPreflight(ctx context.Context, root string, works []string, entries []domain.ShareStrmEntry, selections []domain.ShareSelection) error {
	if store.outputConn == nil {
		return fmt.Errorf("映射范围预检需要 owner 锁连接")
	}
	return workReexportPreflight(ctx, store.outputConn, root, works, entries, selections)
}

func workReexportPreflight(ctx context.Context, query workReexportQuery, root string, works []string, entries []domain.ShareStrmEntry, selections []domain.ShareSelection, urlMappings ...[]string) error {
	if len(works) == 0 || len(works) > domain.ShareWorkReexportMaxWorks || len(entries) == 0 || len(entries) != len(selections) || len(entries) > domain.ShareWorkReexportMaxSelections {
		return fmt.Errorf("映射范围预检需要 owner 锁连接与明确有界的目标")
	}
	mappings := make([]string, len(entries))
	media := make([]int64, len(entries))
	items := make([]string, len(entries))
	paths := make([]string, len(entries))
	relatives := make([]string, len(entries))
	itemMappings := make([]string, len(entries))
	for index, entry := range entries {
		selection := selections[index]
		if selection.SourceKind == "strm115" {
			return domain.ErrStrm115MetadataUnsupported
		}
		if entry.ID == "" || entry.MediaID <= 0 || selection.ItemKey == "" || selection.RelativePath == "" {
			return fmt.Errorf("映射范围预检目标不完整")
		}
		mappings[index], media[index], items[index] = entry.ID, int64(entry.MediaID), selection.ItemKey
		itemMappings[index] = domain.ShareWorkReexportMappingID(entry.ID, selection.ItemKey)
		paths[index], relatives[index] = filepath.Join(root, selection.RelativePath), selection.RelativePath
	}
	urls := make([]string, len(entries))
	if len(urlMappings) > 0 {
		if len(urlMappings[0]) != len(entries) {
			return ErrShareExportChanged
		}
		urls = urlMappings[0]
	}
	rows, err := query.QueryContext(ctx, shareWorkReexportPreflightSQL, pq.Array(mappings), pq.Array(media), pq.Array(items), pq.Array(paths), pq.Array(relatives), pq.Array(works), domain.ShareWorkReexportMaxReferences, pq.Array(urls), pq.Array(itemMappings))
	if err != nil {
		return err
	}
	defer rows.Close()
	var total, rejected int64
	samples := []string{}
	read := false
	for rows.Next() {
		var work, item, reason string
		if err = rows.Scan(&total, &rejected, &work, &item, &reason); err != nil {
			return err
		}
		read = true
		if reason != "" {
			samples = append(samples, fmt.Sprintf("work_key=%q export_key=%q reason=%q", boundedWorkReexportReference(work), boundedWorkReexportReference(item), boundedWorkReexportReference(reason)))
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if !read {
		return fmt.Errorf("共享闭包计数缺失：%w", ErrShareExportChanged)
	}
	if rejected > 0 || total > domain.ShareWorkReexportMaxReferences {
		listed := rejected
		if total > domain.ShareWorkReexportMaxReferences {
			listed = total
		}
		return fmt.Errorf("共享映射闭包拒绝 references_total=%d rejected_total=%d max_references=%d samples=%d truncated=%t: %s：%w", total, rejected, domain.ShareWorkReexportMaxReferences, len(samples), listed > int64(len(samples)), strings.Join(samples, "; "), ErrShareExportChanged)
	}
	return nil
}

func boundedWorkReexportReference(value string) string {
	if len(value) > 256 {
		return value[:256] + "[field_truncated]"
	}
	return value
}
