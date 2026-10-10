package dao

import (
	"context"
	"encoding/json"

	"easy-strm/internal/domain"
)

const shareExportSourcesBatchSQL = `INSERT INTO t_share_export_source_state AS source(source_file_id,share_id,media_id,work_key,file_version,share_version,available,cancelled,episode_sig,export_keys,last_seen_run_id,state)
 SELECT x."SourceFileID",x."ShareID",NULLIF(x."MediaID",0),x."WorkKey",x."FileVersion",x."ShareVersion",x."Available",x."Cancelled",x."EpisodeSignature",x."ExportKeys",$2,x."State"
 FROM jsonb_to_recordset($1::jsonb) AS x("SourceFileID" integer,"ShareID" integer,"MediaID" integer,"WorkKey" text,"FileVersion" integer,"ShareVersion" integer,"Available" boolean,"Cancelled" boolean,"EpisodeSignature" text,"ExportKeys" jsonb,"State" text)
 ORDER BY x."SourceFileID"
 ON CONFLICT(source_file_id) DO UPDATE SET share_id=EXCLUDED.share_id,media_id=EXCLUDED.media_id,work_key=EXCLUDED.work_key,file_version=EXCLUDED.file_version,share_version=EXCLUDED.share_version,available=EXCLUDED.available,cancelled=EXCLUDED.cancelled,episode_sig=EXCLUDED.episode_sig,
 export_keys=CASE WHEN EXCLUDED.state='stale' THEN source.export_keys ELSE EXCLUDED.export_keys END,last_seen_run_id=EXCLUDED.last_seen_run_id,state=EXCLUDED.state,updated_at=now()`

const shareExportStaleBatchSQL = `WITH targets AS MATERIALIZED (
 SELECT target.export_key,target.run,
 EXISTS(SELECT 1 FROM t_share_export_source_state WHERE state='active' AND work_key<>$1 AND export_keys @> to_jsonb(target.export_key))
 OR EXISTS(SELECT 1 FROM t_share_export_dirty_work WHERE pending_export_keys<>'[]'::jsonb AND work_key<>$1 AND pending_export_keys @> to_jsonb(target.export_key)) protected
 FROM jsonb_to_recordset($2::jsonb) AS target(export_key text,run text)
 ), updated AS (
 UPDATE t_strm_export_state output SET state='stale' FROM targets target
 WHERE output.owner_key='share:default' AND output.export_key=target.export_key AND output.last_seen_run_id=target.run AND NOT target.protected
 RETURNING output.export_key
 ) SELECT (SELECT count(*) FROM updated),(SELECT count(*) FROM targets WHERE NOT protected)`

type shareExportStaleTarget struct {
	Key string `json:"export_key"`
	Run string `json:"run"`
}

func buildShareExportSourcesBatch(ctx context.Context, states []domain.ShareExportSourceState) (string, error) {
	values := make([]domain.ShareExportSourceState, 0, len(states))
	for _, state := range states {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := json.Unmarshal([]byte(shareExportKeysJSON(state.ExportKeys)), &state.ExportKeys); err != nil {
			return "", err
		}
		values = append(values, state)
	}
	data, err := json.Marshal(values)
	return string(data), err
}
