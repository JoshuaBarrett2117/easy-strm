package dao

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

const shareSyncBatchSize = 512

const shareSyncBatchSQL = `WITH input AS (
 SELECT * FROM unnest($3::text[],$4::text[],$5::bigint[],$6::text[],$7::text[],
 $8::text[],$9::text[],$10::boolean[],$11::integer[])
 AS batch(remote_id,file_name,file_size,sha1,pick_code,first_name,first_sha,internal_changed,version_bumps)
), matched AS MATERIALIZED (
 SELECT input.*,existing.id,existing.sha1 AS old_sha,existing.file_name AS old_name,existing.media_id AS old_media_id,
 existing.file_size AS old_size,existing.pick_code AS old_pick,existing.available AS old_available
 FROM input LEFT JOIN LATERAL (
  SELECT f.id,f.sha1,f.file_name,f.media_id,f.file_size,f.pick_code,f.available FROM t_share_media_file f
  WHERE f.share_id=$1 AND ((input.remote_id<>'' AND f.file_id=input.remote_id)
   OR (input.remote_id='' AND f.file_id='' AND md5(f.file_name)=md5(input.first_name) AND f.file_name=input.first_name))
  ORDER BY f.id LIMIT 1 FOR UPDATE OF f
 ) existing ON TRUE
), changes AS MATERIALIZED (
 SELECT matched.*,
 internal_changed OR (old_sha<>'' AND first_sha<>'' AND old_sha<>first_sha) AS content_changed,
 version_bumps+CASE WHEN (old_sha<>'' AND first_sha<>'' AND old_sha<>first_sha)
  OR old_name IS DISTINCT FROM first_name THEN 1 ELSE 0 END AS existing_version_bumps,
 ROW(old_name,old_size,old_sha,old_pick,old_available)
  IS DISTINCT FROM ROW(file_name,file_size,sha1,pick_code,TRUE) AS payload_changed
 FROM matched
), updated AS (
 UPDATE t_share_media_file f SET file_name=c.file_name,file_size=c.file_size,sha1=c.sha1,pick_code=c.pick_code,
 available=TRUE,last_seen_at=now(),last_seen_scan_token=$2,updated_at=now(),
 status=CASE WHEN c.content_changed THEN 'pending' ELSE f.status END,
 media_id=CASE WHEN c.content_changed THEN NULL ELSE f.media_id END,
 result=CASE WHEN c.content_changed THEN NULL ELSE f.result END,
 error=CASE WHEN c.content_changed THEN '' ELSE f.error END,
 version=f.version+c.existing_version_bumps
 FROM changes c WHERE f.id=c.id AND (c.payload_changed OR c.existing_version_bumps>0 OR c.content_changed)
 RETURNING f.id
), seen_only AS (
 UPDATE t_share_media_file f SET available=TRUE,last_seen_at=now(),last_seen_scan_token=$2,updated_at=now()
 FROM changes c WHERE f.id=c.id AND NOT (c.payload_changed OR c.existing_version_bumps>0 OR c.content_changed)
 RETURNING f.id
), inserted AS (
 INSERT INTO t_share_media_file(share_id,file_id,file_name,file_size,sha1,pick_code,metadata_source,
 status,available,last_seen_at,last_seen_scan_token,version)
 SELECT $1,remote_id,file_name,file_size,sha1,pick_code,$12,'pending',TRUE,now(),$2,1+version_bumps
 FROM changes WHERE id IS NULL RETURNING id
), cleared AS (
 DELETE FROM t_share_media_file_episode e USING changes c,updated u
 WHERE e.file_id=c.id AND u.id=c.id AND c.content_changed RETURNING e.file_id
)
SELECT (SELECT count(*) FROM updated)+(SELECT count(*) FROM seen_only)+(SELECT count(*) FROM inserted),
 COALESCE((SELECT array_agg(DISTINCT old_media_id) FROM changes
 WHERE content_changed AND old_media_id IS NOT NULL),'{}'::integer[])`

const shareSyncOrphanSQL = `DELETE FROM t_share_media m WHERE m.id=ANY($1::integer[])
 AND NOT EXISTS (SELECT 1 FROM t_share_media_file f WHERE f.media_id=m.id)`

type shareSyncInput struct {
	file            domain.ShareMedia
	firstName       string
	firstSHA        string
	internalChanged bool
	versionBumps    int64
}

// SyncFiles 分批提交完整扫描，成功收尾才原子标记未见文件失效；失败保留已提交批次，返回零及错误。
// 同分享扫描使用会话锁互斥，每批检查分享版本；重试使用独立令牌，不沿用失败扫描的已见集合。
func (d *ShareRecordDAO) SyncFiles(ctx context.Context, shareID int, scanToken string, files []domain.ShareMedia, versions ...int) (count int, err error) {
	if err = ctx.Err(); err != nil {
		return 0, err
	}
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	var locked bool
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(34984,$1)", shareID).Scan(&locked); err != nil {
		_ = conn.Raw(func(interface{}) error { return driver.ErrBadConn })
		_ = conn.Close()
		return 0, err
	}
	if !locked {
		_ = conn.Close()
		return 0, fmt.Errorf("该分享正在同步，请稍后重试")
	}
	log := logger.WithContext(ctx, "share_sync")
	defer func() {
		err = errors.Join(err, releaseShareSyncSession(conn, shareID))
		if err != nil && ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
		}
		if err != nil {
			count = 0
			log.Log(logger.WARN, "分享同步未成功确认；已提交批次保留，请完整重试", logger.Fields{"share_id": shareID}, err)
		}
	}()
	version, err := shareSyncVersion(ctx, conn, shareID, versions)
	if err != nil {
		return 0, err
	}
	attemptToken := scanToken + ":" + uuid.NewString()
	started := time.Now()
	log.Log(logger.INFO, "开始分批同步分享文件", logger.Fields{"share_id": shareID, "input_files": len(files), "batch_size": shareSyncBatchSize}, nil)
	for start := 0; start < len(files); start += shareSyncBatchSize {
		end := min(start+shareSyncBatchSize, len(files))
		input, prepareErr := prepareShareSyncInput(ctx, files[start:end])
		if prepareErr != nil {
			return 0, prepareErr
		}
		if err = syncShareFileBatch(ctx, conn, shareID, version, attemptToken, input); err != nil {
			return 0, err
		}
		log.Log(logger.INFO, "分享文件批次已提交", logger.Fields{"share_id": shareID, "batch": start/shareSyncBatchSize + 1,
			"processed_files": end, "input_files": len(files), "batch_unique_files": len(input), "elapsed_ms": time.Since(started).Milliseconds()}, nil)
	}
	if err = finalizeShareSync(ctx, conn, shareID, version, attemptToken); err != nil {
		return 0, err
	}
	log.Log(logger.INFO, "分享扫描收尾已提交", logger.Fields{"share_id": shareID, "input_files": len(files), "elapsed_ms": time.Since(started).Milliseconds()}, nil)
	return len(files), nil
}

func prepareShareSyncInput(ctx context.Context, files []domain.ShareMedia) ([]shareSyncInput, error) {
	type identity struct{ remoteID, name string }
	indices := make(map[identity]int)
	input := make([]shareSyncInput, 0)
	for index, file := range files {
		if index%shareSyncBatchSize == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		key := identity{remoteID: file.RemoteFileID}
		if file.RemoteFileID == "" {
			key.name = file.FileName
		}
		if position, found := indices[key]; found {
			previous := &input[position]
			changed := previous.file.SHA1 != "" && file.SHA1 != "" && previous.file.SHA1 != file.SHA1
			previous.internalChanged = previous.internalChanged || changed
			if changed || previous.file.FileName != file.FileName {
				previous.versionBumps++
			}
			previous.file = file
		} else {
			indices[key] = len(input)
			input = append(input, shareSyncInput{file: file, firstName: file.FileName, firstSHA: file.SHA1})
		}
	}
	return input, ctx.Err()
}

func shareSyncVersion(ctx context.Context, conn *sql.Conn, shareID int, versions []int) (int, error) {
	query := "SELECT version FROM t_share_record WHERE id=$1"
	args := []interface{}{shareID}
	if len(versions) > 0 {
		query += " AND version=$2"
		args = append(args, versions[0])
	}
	var version int
	err := conn.QueryRowContext(ctx, query, args...).Scan(&version)
	return version, err
}

func lockShareSyncVersion(ctx context.Context, tx *sql.Tx, shareID, version int) error {
	var found int
	return tx.QueryRowContext(ctx, "SELECT id FROM t_share_record WHERE id=$1 AND version=$2 FOR UPDATE", shareID, version).Scan(&found)
}

func syncShareFileBatch(ctx context.Context, conn *sql.Conn, shareID, version int, token string, input []shareSyncInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockShareSyncVersion(ctx, tx, shareID, version); err != nil {
		return err
	}
	var written int
	var orphans []int64
	if err = tx.QueryRowContext(ctx, shareSyncBatchSQL, shareSyncBatchArgs(shareID, token, input)...).Scan(&written, pq.Array(&orphans)); err != nil {
		return err
	}
	if written != len(input) {
		return fmt.Errorf("分享同步批次写入数量不一致：期望 %d，实际 %d", len(input), written)
	}
	if len(orphans) > 0 {
		if _, err = tx.ExecContext(ctx, shareSyncOrphanSQL, pq.Array(orphans)); err != nil {
			return err
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

func shareSyncBatchArgs(shareID int, token string, input []shareSyncInput) []interface{} {
	remoteIDs, names, hashes, picks := []string{}, []string{}, []string{}, []string{}
	firstNames, firstHashes := []string{}, []string{}
	sizes, bumps := []int64{}, []int64{}
	changed := []bool{}
	for _, entry := range input {
		remoteIDs = append(remoteIDs, entry.file.RemoteFileID)
		names = append(names, entry.file.FileName)
		sizes = append(sizes, entry.file.FileSize)
		hashes = append(hashes, entry.file.SHA1)
		picks = append(picks, entry.file.PickCode)
		firstNames = append(firstNames, entry.firstName)
		firstHashes = append(firstHashes, entry.firstSHA)
		changed = append(changed, entry.internalChanged)
		bumps = append(bumps, entry.versionBumps)
	}
	return []interface{}{shareID, token, pq.Array(remoteIDs), pq.Array(names), pq.Array(sizes), pq.Array(hashes), pq.Array(picks),
		pq.Array(firstNames), pq.Array(firstHashes), pq.Array(changed), pq.Array(bumps), domain.MetadataSourceAuto}
}

func finalizeShareSync(ctx context.Context, conn *sql.Conn, shareID, version int, token string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockShareSyncVersion(ctx, tx, shareID, version); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE t_share_media_file SET available=FALSE,updated_at=now() WHERE share_id=$1 AND available AND last_seen_scan_token IS DISTINCT FROM $2", shareID, token); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

func releaseShareSyncSession(conn *sql.Conn, shareID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var unlocked bool
	err := conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock(34984,$1)", shareID).Scan(&unlocked)
	if err == nil && !unlocked {
		err = fmt.Errorf("分享同步会话锁释放未确认")
	}
	if err != nil {
		_ = conn.Raw(func(interface{}) error { return driver.ErrBadConn })
	}
	closeErr := conn.Close()
	if errors.Is(closeErr, sql.ErrConnDone) {
		closeErr = nil
	}
	return errors.Join(err, closeErr)
}
