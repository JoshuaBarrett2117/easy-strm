package dao

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"github.com/lib/pq"
	"os"
	"path"
	"strings"
)

type selection115Reader interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}

const selection115Columns = `f.id,f.strm_config_id,f.file_name,f.file_path,f.file_size,COALESCE(f.local_strm_path,''),cfg.cloud115_id,COALESCE(cfg.net_disk_path,''),EXISTS(SELECT 1 FROM t_share_media metadata WHERE metadata.work_key=$3 AND metadata.result->>'success'='true' AND btrim(metadata.result->>'title')<>'' AND metadata.result->>'media_type'=split_part($3,':',2))`
const selection115From = ` FROM t_strm_file f JOIN t_strm_config cfg ON cfg.id=f.strm_config_id WHERE f.strm_config_id>=0`
const selection115WorkSQL = `SELECT ` + selection115Columns + selection115From + ` AND (strpos(lower(f.file_path),$1)>0 OR strpos(lower(f.file_path),$2)>0) ORDER BY f.file_size DESC NULLS LAST,f.id ASC`
const selection115RefSQL = `SELECT ` + selection115Columns + selection115From + ` AND f.strm_config_id=$1 AND f.id=$2 ORDER BY f.file_size DESC NULLS LAST,f.id ASC`

type selection115Item struct {
	candidate       domain.ShareCandidate
	season, episode int
}

func readSelection115(ctx context.Context, reader selection115Reader, work, query string, args ...interface{}) ([]selection115Item, error) {
	rows, err := reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []selection115Item{}
	for rows.Next() {
		var fileID, configID int64
		var filename, relative, local, root string
		var size sql.NullInt64
		var cloudID sql.NullInt64
		var metadata bool
		if err = rows.Scan(&fileID, &configID, &filename, &relative, &size, &local, &cloudID, &root, &metadata); err != nil {
			return nil, err
		}
		if configID < 0 || fileID <= 0 {
			continue
		}
		season, episode, matched := domain.ParseStrm115Item(work, relative)
		if !matched {
			continue
		}
		relative = domain.NormalizeStrm115Path(relative)
		root = domain.NormalizeStrm115Path(root)
		networkPath := strings.HasPrefix(relative, "//")
		relative = strings.TrimPrefix(relative, "/")
		candidate := domain.ShareCandidate{SourceKind: "strm115", ExternalRef: fmt.Sprintf("%d:%d", configID, fileID), FileName: filename, FileSize: size.Int64, CloudPath: strings.TrimRight(root, "/") + "/" + relative, Cloud115ID: int(cloudID.Int64), State: "active"}
		_, buildErr := domain.BuildStrm115Entry("https://validation.invalid", candidate)
		if local != "" && metadata && buildErr == nil && !networkPath && !strings.Contains(relative, ":") && path.Clean(relative) == relative && root != "" && strings.HasPrefix(root, "/") && !strings.Contains(root, ":") {
			info, statErr := os.Stat(domain.NormalizeStrm115Path(local))
			candidate.Available = statErr == nil && info.Mode().IsRegular()
		}
		if !candidate.Available {
			candidate.State = "unavailable"
		}
		values = append(values, selection115Item{candidate: candidate, season: season, episode: episode})
	}
	return values, rows.Err()
}

func selection115Work(ctx context.Context, reader selection115Reader, work string) ([]selection115Item, error) {
	parts := strings.Split(work, ":")
	if len(parts) != 3 || (parts[1] != "movie" && parts[1] != "tv") || parts[0] != "tmdb" {
		return []selection115Item{}, nil
	}
	return readSelection115(ctx, reader, work, selection115WorkSQL, "[tmdbid="+parts[2]+"]", "{tmdb-"+parts[2]+"}", work)
}

func selection115Candidate(ctx context.Context, reader selection115Reader, selection domain.ShareSelection) (domain.ShareCandidate, error) {
	configID, fileID, err := domain.ParseStrm115Ref(selection.ExternalRef)
	if err != nil {
		return domain.ShareCandidate{}, err
	}
	values, err := readSelection115(ctx, reader, selection.WorkKey, selection115RefSQL, configID, fileID, selection.WorkKey)
	if err != nil {
		return domain.ShareCandidate{}, err
	}
	for _, value := range values {
		if value.season == selection.Season && value.episode == selection.Episode && value.candidate.Available {
			return value.candidate, nil
		}
	}
	return domain.ShareCandidate{}, ErrShareSelectionUnavailable
}

// SelectionStrm115Candidate 点查并重新验证目录选择，不读取分享凭据或修改目录元数据。
func (d *ShareRecordDAO) SelectionStrm115Candidate(ctx context.Context, selection domain.ShareSelection) (domain.ShareCandidate, error) {
	if selection.SourceKind != "strm115" {
		return domain.ShareCandidate{}, ErrShareSelectionUnavailable
	}
	return selection115Candidate(ctx, d.db, selection)
}

// RecheckSelectionStrm115 只读核对发布快照，不启动逐条解析事务或执行自动候补。
func (d *ShareRecordDAO) RecheckSelectionStrm115(ctx context.Context, selection domain.ShareSelection) error {
	var valid bool
	err := d.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM t_share_media_selection WHERE media_item_key=$1 AND source_kind='strm115' AND external_ref=$2 AND selection_revision=$3 AND stable_relative_path=$4)`, selection.ItemKey, selection.ExternalRef, selection.Revision, selection.RelativePath).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrShareExportChanged
	}
	return nil
}

// Strm115SelectionSources 按作品读取目录选择及既有元数据，不依赖分享来源仍然可用。
func (d *ShareRecordDAO) Strm115SelectionSources(ctx context.Context, query domain.ShareLibraryQuery, after string) ([]domain.Strm115SelectionSource, error) {
	where, args := libraryWhere(query)
	filter := `s.work_key IN (SELECT work_key FROM (SELECT work_key,tmdb_id,title,original_title,media_type library_media_type,media_year,rating,genre_ids,country_codes,TRUE available FROM t_share_media) metadata WHERE ` + where + `)`
	if query.WorkKey != "" && isOnlyStrmWorkQuery(query) {
		filter = `s.work_key=$1`
		args = []interface{}{query.WorkKey}
	}
	if len(query.FileIDs) > 0 {
		args = append(args, pq.Array(query.FileIDs))
		filter += ` AND s.work_key IN (SELECT media.work_key FROM t_share_media_file file JOIN t_share_media media ON media.id=file.media_id WHERE file.id=ANY($` + fmt.Sprint(len(args)) + `::integer[]))`
	}
	args = append(args, after)
	sqlQuery := `SELECT s.media_item_key,s.work_key,s.season_number,s.episode_number,s.selection_mode,s.selection_revision,s.exported_revision,s.stable_relative_path,s.external_ref,m.id,m.result FROM t_share_media_selection s LEFT JOIN LATERAL (SELECT id,result FROM t_share_media WHERE work_key=s.work_key ORDER BY id LIMIT 1) m ON true WHERE s.source_kind='strm115' AND ` + filter + ` AND s.media_item_key>$` + fmt.Sprint(len(args)) + ` ORDER BY s.media_item_key LIMIT 100`
	rows, err := d.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	values := []domain.Strm115SelectionSource{}
	for rows.Next() {
		var value domain.Strm115SelectionSource
		selection := &value.Selection
		var media sql.NullInt64
		var raw []byte
		if err = rows.Scan(&selection.ItemKey, &selection.WorkKey, &selection.Season, &selection.Episode, &selection.Mode, &selection.Revision, &selection.ExportedRevision, &selection.RelativePath, &selection.ExternalRef, &media, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		selection.SourceKind = "strm115"
		if !media.Valid || media.Int64 <= 0 || json.Unmarshal(raw, &value.Source.Result) != nil || !value.Source.Result.Success || strings.TrimSpace(value.Source.Result.Title) == "" || (value.Source.Result.MediaType != "movie" && value.Source.Result.MediaType != "tv") || !strings.HasPrefix(selection.WorkKey, "tmdb:"+value.Source.Result.MediaType+":") {
			rows.Close()
			return nil, ErrShareSelectionUnavailable
		}
		value.Source.MediaID = int(media.Int64)
		value.Source.WorkKey = selection.WorkKey
		value.Source.Available = true
		value.Source.Episodes = []domain.ShareEpisode{{SeasonNumber: selection.Season, EpisodeNumber: selection.Episode}}
		value.Source.Result.SeasonNumber = selection.Season
		value.Source.Result.EpisodeNumber = selection.Episode
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
		if _, err = selection115Candidate(ctx, d.db, values[index].Selection); err != nil {
			return nil, err
		}
		values[index].Selection.Valid = true
	}
	return values, nil
}

func changeSelection115(ctx context.Context, transaction *sql.Tx, change domain.ShareSelectionChange, work string) (string, error) {
	selection, err := scanShareSelection(transaction.QueryRowContext(ctx, `SELECT `+shareSelectionColumns+shareSelectionFrom+` WHERE s.media_item_key=$1`, change.ItemKey))
	if err != nil {
		return "", err
	}
	selection.SourceKind, selection.ExternalRef = "strm115", change.ExternalRef
	if _, err = selection115Candidate(ctx, transaction, selection); err != nil {
		return "", err
	}
	result, err := transaction.ExecContext(ctx, `UPDATE t_share_media_selection SET source_kind='strm115',external_ref=$2,selected_candidate_id=NULL,selection_mode='manual',selection_revision=selection_revision+1,updated_at=now() WHERE media_item_key=$1 AND selection_revision=$3`, change.ItemKey, change.ExternalRef, change.ExpectedRevision)
	if err != nil {
		return "", err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if count != 1 {
		return "", ErrShareSelectionConflict
	}
	if _, err = transaction.ExecContext(ctx, `SELECT share_export_enqueue(ARRAY[$1]::text[],'selection')`, work); err != nil {
		return "", err
	}
	return work, transaction.Commit()
}

const selection115PreflightSQL = `SELECT s.media_item_key,s.work_key,s.season_number,s.episode_number,s.external_ref,s.source_kind FROM t_share_media_selection s WHERE s.source_kind<>'share' AND ($1='' OR s.work_key=$1) ORDER BY s.work_key,s.season_number,s.episode_number`

func preflightSelection115(ctx context.Context, reader selection115Reader, work string) error {
	rows, err := reader.QueryContext(ctx, selection115PreflightSQL, work)
	if err != nil {
		return err
	}
	selections := []domain.ShareSelection{}
	for rows.Next() {
		var selection domain.ShareSelection
		if err = rows.Scan(&selection.ItemKey, &selection.WorkKey, &selection.Season, &selection.Episode, &selection.ExternalRef, &selection.SourceKind); err != nil {
			rows.Close()
			return err
		}
		selections = append(selections, selection)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	cache := map[string][]selection115Item{}
	for _, selection := range selections {
		if selection.SourceKind != "strm115" {
			return ErrShareSelectionUnavailable
		}
		if _, _, err = domain.ParseStrm115Ref(selection.ExternalRef); err != nil {
			return ErrShareSelectionUnavailable
		}
		values, found := cache[selection.WorkKey]
		if !found {
			values, err = selection115Work(ctx, reader, selection.WorkKey)
			if err != nil {
				return err
			}
			cache[selection.WorkKey] = values
		}
		valid := false
		for _, value := range values {
			if value.candidate.ExternalRef == selection.ExternalRef && value.season == selection.Season && value.episode == selection.Episode && value.candidate.Available {
				valid = true
				break
			}
		}
		if !valid {
			return ErrShareSelectionUnavailable
		}
	}
	return nil
}
