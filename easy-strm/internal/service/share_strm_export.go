package service

import (
	"context"
	"database/sql"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

type shareStrmConflict struct {
	shareIDs   map[int]bool
	labels     map[string]int
	fileCounts map[int]int
	sources    map[int]domain.ShareStrmSource
	winners    map[string]int
}

// ErrShareStrmDedupSkipped 表示该来源仅因同作品同季集已选出更优来源而跳过落盘。
var ErrShareStrmDedupSkipped = errors.New("分享STRM因去重策略跳过")

const shareStrmProgressBatchSize = 100

type shareExportRecoveryContext struct{}

type shareReconciliationCompletionContext struct{}

// export 仅以本地t_share_media及关联分享记录生成STRM，不访问115分享或元数据网络接口。
func (s *ShareStrmService) export(ctx context.Context, cfg domain.ShareStrmSettings, q domain.ShareLibraryQuery, id string) (exportErr error) {
	completion, scheduled := ctx.Value(shareReconciliationCompletionContext{}).(func(context.Context) error)
	var recovery *dao.ShareExportCheckpointDAO
	var recoveryRevision int64
	if s.exportDB != nil {
		candidate := dao.NewShareExportCheckpointDAO(s.exportDB)
		if err := candidate.CheckSchema(ctx); err == nil {
			var revisionErr error
			recoveryRevision, revisionErr = candidate.RecoveryRevision(ctx)
			if revisionErr != nil {
				return revisionErr
			}
			recovery = candidate
		} else if scheduled || !errors.Is(err, dao.ErrShareExportSchemaMissing) {
			return err
		}
	}
	if s.tasks != nil {
		if e := s.updateExportMetadata(ctx, id, map[string]interface{}{"share_export": true, "export_query": q, "output_path": cfg.OutputPath}); e != nil {
			return e
		}
	}
	output, _ := ctx.Value(shareExportOutputContext{}).(*StrmOutput)
	if s.exportDB != nil && output == nil {
		var e error
		output, e = WaitStrmOutput(ctx, s.exportDB, cfg.OutputPath, "share:default", id, func() bool { return s.tasks != nil && s.tasks.IsCancelled(id) })
		if e != nil {
			return e
		}
		defer output.Store.Close()
		ctx = context.WithValue(ctx, shareExportOutputContext{}, output)
	}
	if recovery != nil && output != nil {
		recovery = recovery.ForOutputConnection(output.Store.Conn)
		ctx = context.WithValue(ctx, shareExportRecoveryContext{}, recovery)
	}
	cats, err := s.categories()
	if err != nil {
		return err
	}
	conflicts, err := s.shareStrmConflicts(ctx, q)
	if err != nil {
		return err
	}
	after, processed, written, failed, skipped, skippedDedupe := 0, 0, 0, 0, 0, 0
	total, pending := 0, 0
	sourceErrors := []string{}
	flushProgress := func() error {
		if pending == 0 || s.tasks == nil {
			return nil
		}
		pending = 0
		progressErr := s.tasks.UpdateProgress(id, max(total, processed), processed, processed-failed-skipped, failed)
		metadata := map[string]interface{}{"exported_files": written, "written": written, "skipped_sources": skipped, "skipped_dedupe": skippedDedupe, "output_path": cfg.OutputPath, "errors": sourceErrors, "conflict_policy": "按可用性、文件大小、分享ID、文件ID确定性选优；其余同作品同季集来源去重跳过", "share_export": true, "export_query": q}
		if output != nil {
			metadata["added"] = output.Added
			metadata["updated"] = output.Updated
			metadata["skipped"] = output.Skipped
			metadata["conflicts"] = output.Conflicts
			metadata["exported_files"] = output.Added + output.Updated
		}
		return errors.Join(progressErr, s.updateExportMetadata(ctx, id, metadata))
	}
	defer func() {
		exportErr = errors.Join(exportErr, flushProgress())
	}()
	seen := map[string]bool{}
	for {
		rows, err := s.store.StrmSources(ctx, q, after)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		total = processed + max(rows[0].Remaining, len(rows))
		for _, source := range rows {
			if err = ctx.Err(); err != nil {
				return err
			}
			if s.tasks != nil && s.tasks.IsCancelled(id) {
				return context.Canceled
			}
			after = source.ID
			processed++
			created, sourceErr := s.exportLocalStrm(ctx, cfg, source, cats, seen, conflicts)
			if created {
				written++
			}
			if errors.Is(sourceErr, ErrShareStrmDedupSkipped) {
				skippedDedupe++
			} else if errors.Is(sourceErr, ErrShareUnitSkipped) || errors.Is(sourceErr, sql.ErrNoRows) {
				skipped++
			} else if sourceErr != nil {
				failed++
				if len(sourceErrors) < 100 {
					sourceErrors = append(sourceErrors, fmt.Sprintf("来源%d：%v", source.ID, sourceErr))
				}
			}
			pending++
			if pending >= shareStrmProgressBatchSize {
				if err = flushProgress(); err != nil {
					return err
				}
			}
		}
	}
	if failed > 0 {
		return fmt.Errorf("导出结束：生成%d个STRM，%d个本地记录失败，详情见任务记录", written, failed)
	}
	if err = flushProgress(); err != nil {
		return err
	}
	q.Page = 0
	q.PageSize = 0
	q.Sort = ""
	q.Direction = ""
	q.Available = false
	if output != nil && isFullShareStrmQuery(q) {
		if scheduled {
			if err = ctx.Err(); err != nil {
				return err
			}
			if s.tasks != nil && s.tasks.IsCancelled(id) {
				return context.Canceled
			}
		}
		if err = output.Store.FinishSnapshot(ctx, "share:default", output.Snapshot, output.Seen); err != nil {
			return err
		}
		if scheduled {
			return completion(ctx)
		}
		if recovery != nil {
			return recovery.MarkLegacyReconciled(ctx, recoveryRevision)
		}
	}
	return nil
}

func isFullShareStrmQuery(q domain.ShareLibraryQuery) bool {
	return q.WorkKey == "" && q.Keyword == "" && q.TmdbID == 0 && q.MediaType == "" && q.YearMin == 0 && q.YearMax == 0 && q.RatingMin == nil && q.RatingMax == nil && q.Genres == "" && q.Countries == "" && len(q.FileIDs) == 0
}

func shareStrmIdentity(source domain.ShareStrmSource, episode domain.ShareEpisode) string {
	return fmt.Sprintf("%s:%d:%d", source.WorkKey, episode.SeasonNumber, episode.EpisodeNumber)
}

func shareStrmSourceID(source domain.ShareStrmSource) int {
	if source.ShareID > 0 {
		return source.ShareID
	}
	return -source.ID
}

func (s *ShareStrmService) shareStrmSourceLabel(source domain.ShareStrmSource) string {
	label := s.organizer.sanitizeFolderName(strings.NewReplacer("/", " ", "\\", " ").Replace(source.ShareName))
	label = strings.Trim(label, " .-")
	if label == "" {
		label = fmt.Sprintf("分享%d", shareStrmSourceID(source))
	}
	return label
}

// shareStrmConflicts 先统计同作品同季集涉及的不同分享，保证首个来源也能得到稳定后缀。
func (s *ShareStrmService) shareStrmConflicts(ctx context.Context, q domain.ShareLibraryQuery) (map[string]*shareStrmConflict, error) {
	result := map[string]*shareStrmConflict{}
	for after := 0; ; {
		rows, err := s.store.StrmSources(ctx, q, after)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return result, nil
		}
		for _, source := range rows {
			after = source.ID
			episodes := source.Episodes
			if source.Result.MediaType == "movie" {
				episodes = []domain.ShareEpisode{{}}
			}
			for _, episode := range episodes {
				identity := shareStrmIdentity(source, episode)
				conflict := result[identity]
				if conflict == nil {
					conflict = &shareStrmConflict{shareIDs: map[int]bool{}, labels: map[string]int{}, fileCounts: map[int]int{}, sources: map[int]domain.ShareStrmSource{}, winners: map[string]int{}}
					result[identity] = conflict
				}
				shareID := shareStrmSourceID(source)
				conflict.sources[source.ID] = source
				if !conflict.shareIDs[shareID] {
					conflict.shareIDs[shareID] = true
					conflict.labels[s.shareStrmSourceLabel(source)]++
				}
				conflict.fileCounts[shareID]++
				winnerID := conflict.winners[identity]
				if winnerID == 0 || betterShareStrmSource(source, conflict.sources[winnerID]) {
					conflict.winners[identity] = source.ID
				}
			}
		}
	}
}

func betterShareStrmSource(candidate, current domain.ShareStrmSource) bool {
	if candidate.Available != current.Available {
		return candidate.Available
	}
	if candidate.FileSize != current.FileSize {
		return candidate.FileSize > current.FileSize
	}
	if shareStrmSourceID(candidate) != shareStrmSourceID(current) {
		return shareStrmSourceID(candidate) < shareStrmSourceID(current)
	}
	return candidate.ID < current.ID
}

func (s *ShareStrmService) exportLocalStrm(ctx context.Context, cfg domain.ShareStrmSettings, source domain.ShareStrmSource, cats []*domain.MediaCategory, seen map[string]bool, conflictMaps ...map[string]*shareStrmConflict) (bool, error) {
	resources := []shareResource{{key: shareKey(source.ShareID)}, {key: fmt.Sprintf("share-config:%d", source.ShareID)}, {key: fileKey(source.ID), exclusive: true}}
	matchesForLock := shareCodeRe.FindStringSubmatch(source.URL)
	if len(matchesForLock) > 1 {
		resources = append(resources, shareResource{key: "mapping-code:" + matchesForLock[1]}, shareResource{key: "mapping:" + uuid.NewSHA1(uuid.NameSpaceURL, []byte(matchesForLock[1]+":path:"+shareCandidatePath(source.FileName))).String(), exclusive: true})
	}
	sourceEpisodes := source.Episodes
	if source.Result.MediaType == "movie" {
		sourceEpisodes = []domain.ShareEpisode{{}}
	}
	for _, ep := range sourceEpisodes {
		resources = append(resources, shareResource{key: "identity:" + shareStrmIdentity(source, ep), exclusive: true})
	}
	release, err := s.Coordinator().acquire(ctx, []int{source.ShareID}, resources...)
	if err != nil {
		return false, err
	}
	defer release()
	if reader, ok := s.store.(shareStrmSourceReader); ok {
		fresh, err := reader.GetStrmSource(ctx, source.ID)
		if err != nil {
			return false, err
		}
		if source.FileVersion != fresh.FileVersion || source.ShareVersion != fresh.ShareVersion || source.WorkKey != fresh.WorkKey {
			return false, ErrShareUnitSkipped
		}
		source = fresh
		conflicts, err := s.shareStrmConflicts(ctx, domain.ShareLibraryQuery{WorkKey: source.WorkKey})
		if err != nil {
			return false, err
		}
		conflictMaps = []map[string]*shareStrmConflict{conflicts}
	}
	output, _ := ctx.Value(shareExportOutputContext{}).(*StrmOutput)

	attempt, _ := ctx.Value(shareIncrementalAttemptContext{}).(*shareIncrementalAttempt)
	if attempt != nil && attempt.mappedKeys == nil {
		attempt.mappedKeys = map[string]string{}
	}

	filePath := shareCandidatePath(source.FileName)
	file := domain.ShareFileInfo{Name: path.Base(filePath), Path: filePath}
	if len(selectShareMediaFiles([]domain.ShareFileInfo{file})) == 0 {
		return false, fmt.Errorf("本地记录缺少具体视频文件，无法导出：%s", source.FileName)
	}
	episodes := source.Episodes
	if source.Result.MediaType == "movie" {
		episodes = []domain.ShareEpisode{{}}
	} else if len(episodes) == 0 {
		return false, fmt.Errorf("电视剧文件缺少季集映射：%s", source.FileName)
	}
	matches := shareCodeRe.FindStringSubmatch(source.URL)
	if len(matches) < 2 {
		return false, fmt.Errorf("本地记录的115分享地址无效")
	}
	entry := domain.ShareStrmEntry{
		ID:         uuid.NewSHA1(uuid.NameSpaceURL, []byte(matches[1]+":path:"+filePath)).String(),
		ShareCode:  matches[1],
		Password:   source.Password,
		FileID:     source.RemoteFileID,
		FileName:   file.Name,
		FilePath:   filePath,
		MediaID:    source.MediaID,
		Title:      source.Result.Title,
		PosterPath: source.Result.PosterPath,
		Episodes:   source.Episodes,
	}
	if entry.Password == "" {
		entry.Password = extractSharePassword(source.URL)
	}
	if err := s.store.SaveStrmEntry(ctx, entry); err != nil {
		return false, err
	}
	written := 0
	dedupeSkipped := false
	conflicts := map[string]*shareStrmConflict{}
	if len(conflictMaps) > 0 && conflictMaps[0] != nil {
		conflicts = conflictMaps[0]
	}
	for _, episode := range episodes {
		current := source
		current.Result.SeasonNumber = episode.SeasonNumber
		current.Result.EpisodeNumber = episode.EpisodeNumber
		identity := shareStrmIdentity(source, episode)
		conflict := conflicts[identity]
		shareID := shareStrmSourceID(source)
		if cfg.DedupeExport && conflict != nil && conflict.winners[identity] != source.ID {
			dedupeSkipped = true
			if attempt != nil {
				winner := conflict.sources[conflict.winners[identity]]
				attempt.keys[source.ID] = append(attempt.keys[source.ID], shareStrmExportKey(winner, episode, conflict, true))
			}
			continue
		}
		suffix := ""
		seenIdentity := identity
		if !cfg.DedupeExport && conflict != nil && len(conflict.shareIDs) > 1 {
			suffix = s.shareStrmSourceLabel(source)
			seenIdentity += fmt.Sprintf(":share:%d", shareID)
			if conflict.labels[suffix] > 1 {
				suffix += fmt.Sprintf("-分享%d", shareID)
			}
		}
		if !cfg.DedupeExport && conflict != nil && conflict.fileCounts[shareID] > 1 {
			if suffix != "" {
				suffix += "-"
			}
			suffix += fmt.Sprintf("文件%d", source.ID)
			seenIdentity += fmt.Sprintf(":file:%d", source.ID)
		}
		relative, pathErr := s.strmRelativePath(current, file, cats, suffix)
		if pathErr != nil {
			return written > 0, pathErr
		}
		exportKey := shareStrmExportKey(source, episode, conflict, cfg.DedupeExport)
		if seen[seenIdentity] {
			if attempt != nil {
				if mappedKey, ok := attempt.mappedKeys[seenIdentity]; ok {
					exportKey = mappedKey
				}
				attempt.mappedKeys[seenIdentity] = exportKey
				attempt.keys[source.ID] = append(attempt.keys[source.ID], exportKey)
			}
			continue
		}
		localPath := filepath.Join(cfg.OutputPath, relative)
		normalized, pathErr := NormalizeStrmOutputPath(localPath)
		if pathErr != nil {
			return written > 0, pathErr
		}
		pathRelease, pathErr := s.Coordinator().acquire(ctx, nil, shareResource{key: "path:" + normalized, exclusive: true})
		if pathErr != nil {
			return written > 0, pathErr
		}
		var unlock func()
		if output != nil {
			unlock, pathErr = output.Store.LockPath(ctx, normalized)
			if pathErr != nil {
				pathRelease()
				return written > 0, pathErr
			}
		}
		finishPath := func() {
			if unlock != nil {
				unlock()
			}
			pathRelease()
		}
		var writeErr error
		if output != nil {
			raw, _ := json.Marshal(entry)
			if attempt != nil {
				if writeErr = attempt.record(ctx, exportKey); writeErr != nil {
					finishPath()
					return written > 0, writeErr
				}
			} else if recovery, ok := ctx.Value(shareExportRecoveryContext{}).(*dao.ShareExportCheckpointDAO); ok {
				if writeErr = recovery.RecordExternalPlannedKeys(ctx, source.WorkKey, []string{exportKey}); writeErr != nil {
					finishPath()
					return written > 0, writeErr
				}
			}
			_, writeErr = output.Write(ctx, exportKey, localPath, cfg.BaseURL+"/share-strm/"+entry.ID+"\n", string(raw), entry.ID)
		} else {
			writeErr = writeShareStrm(localPath, cfg.BaseURL+"/share-strm/"+entry.ID)
		}
		if err := writeErr; err != nil {
			finishPath()
			return written > 0, err
		}
		if attempt != nil {
			attempt.mappedKeys[seenIdentity] = exportKey
			attempt.keys[source.ID] = append(attempt.keys[source.ID], exportKey)
		}
		if err := s.store.SaveExportedStrmFile(ctx, domain.StrmFile{StrmConfigID: -1, FileName: file.Name, FilePath: localPath, LocalStrmPath: localPath}); err != nil {
			finishPath()
			return written > 0, fmt.Errorf("STRM已写入，但登记文件清单失败：%w", err)
		}
		finishPath()
		seen[seenIdentity] = true
		written++
	}
	if dedupeSkipped && written == 0 {
		return false, ErrShareStrmDedupSkipped
	}
	return written > 0, nil
}

func shareStrmExportKey(source domain.ShareStrmSource, episode domain.ShareEpisode, conflict *shareStrmConflict, dedupe ...bool) string {
	key := fmt.Sprintf("%d:%d:%d", source.MediaID, episode.SeasonNumber, episode.EpisodeNumber)
	shareID := shareStrmSourceID(source)
	if (len(dedupe) == 0 || !dedupe[0]) && conflict != nil && len(conflict.shareIDs) > 1 {
		key += fmt.Sprintf(":share:%d", shareID)
	}
	if (len(dedupe) == 0 || !dedupe[0]) && conflict != nil && conflict.fileCounts[shareID] > 1 {
		key += fmt.Sprintf(":file:%d", source.ID)
	}
	return key
}

// resolveStrmFileID 首次播放只逐级读取目标路径所在目录，不遍历分享中的其他目录。
func (s *ShareStrmService) resolveStrmFileID(ctx context.Context, entry domain.ShareStrmEntry) (string, error) {
	normalized := shareCandidatePath(entry.FilePath)
	if entry.FilePath == "" || normalized == "." || strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("播放映射缺少视频路径，请重新导出")
	}
	parts := strings.Split(strings.TrimPrefix(normalized, "/"), "/")
	cid := "0"
	transfer := &ShareTransferService{client: s.client}
	for i, name := range parts {
		files, _, err := transfer.fetchShareDirectory(ctx, entry.ShareCode, entry.Password, cid)
		if err != nil {
			return "", err
		}
		matches := []domain.ShareFileInfo{}
		for _, f := range files {
			info := convertToShareFileInfo(f)
			if info.Name == name {
				matches = append(matches, info)
			}
		}
		if len(matches) != 1 {
			return "", fmt.Errorf("分享路径不存在或名称不唯一：%s", strings.Join(parts[:i+1], "/"))
		}
		found := matches[0]
		if i == len(parts)-1 {
			if !found.IsDir && found.Fid != "" {
				return found.Fid, nil
			}
			return "", fmt.Errorf("本地路径对应的不是视频文件：%s", entry.FilePath)
		}
		if !found.IsDir || found.DirID == "" {
			return "", fmt.Errorf("分享路径中间项不是目录：%s", name)
		}
		cid = found.DirID
	}
	return "", fmt.Errorf("无法定位分享文件")
}

type shareExportOutputContext struct{}
type shareStrmSourceReader interface {
	GetStrmSource(context.Context, int) (domain.ShareStrmSource, error)
}
