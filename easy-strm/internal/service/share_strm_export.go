package service

import (
	"context"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// export 仅以本地t_share_media及关联分享记录生成STRM，不访问115分享或元数据网络接口。
func (s *ShareStrmService) export(ctx context.Context, cfg domain.ShareStrmSettings, q domain.ShareLibraryQuery, id string) error {
	if s.tasks != nil {
		if e := s.tasks.UpdateMetadata(id, map[string]interface{}{"share_export": true, "export_query": q, "output_path": cfg.OutputPath}); e != nil {
			return e
		}
	}
	if s.exportDB != nil {
		output, e := NewStrmOutput(ctx, s.exportDB, cfg.OutputPath, "share:default", id)
		if e != nil {
			return e
		}
		s.output = output
		defer func() { output.Store.Close(); s.output = nil }()
	}
	cats, err := s.categories()
	if err != nil {
		return err
	}
	after, processed, written, failed := 0, 0, 0, 0
	sourceErrors := []string{}
	seen := map[string]bool{}
	for {
		rows, err := s.store.StrmSources(ctx, q, after)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		total := processed + max(rows[0].Remaining, len(rows))
		for _, source := range rows {
			if err = ctx.Err(); err != nil {
				return err
			}
			if s.tasks.IsCancelled(id) {
				return context.Canceled
			}
			after = source.ID
			processed++
			created, sourceErr := s.exportLocalStrm(ctx, cfg, source, cats, seen)
			if created {
				written++
			}
			if sourceErr != nil {
				failed++
				if len(sourceErrors) < 100 {
					sourceErrors = append(sourceErrors, fmt.Sprintf("来源%d：%v", source.ID, sourceErr))
				}
			}
			if err = s.tasks.UpdateProgress(id, max(total, processed), processed, processed-failed, failed); err != nil {
				return err
			}
			metadata := map[string]interface{}{"exported_files": written, "output_path": cfg.OutputPath, "errors": sourceErrors, "conflict_policy": "同作品同集采用首个有效来源；其他归属路径报告冲突"}
			metadata["share_export"] = true
			metadata["export_query"] = q
			if s.output != nil {
				metadata["added"] = s.output.Added
				metadata["updated"] = s.output.Updated
				metadata["skipped"] = s.output.Skipped
				metadata["conflicts"] = s.output.Conflicts
				metadata["exported_files"] = s.output.Added + s.output.Updated
			}
			if err = s.tasks.UpdateMetadata(id, metadata); err != nil {
				return err
			}
		}
	}
	if failed > 0 {
		return fmt.Errorf("导出结束：生成%d个STRM，%d个本地记录失败，详情见任务记录", written, failed)
	}
	q.Page = 0
	q.PageSize = 0
	q.Sort = ""
	q.Direction = ""
	q.Available = false
	if s.output != nil && q == (domain.ShareLibraryQuery{}) {
		return s.output.Store.Finish(ctx, "share:default", id)
	}
	return nil
}

func (s *ShareStrmService) exportLocalStrm(ctx context.Context, cfg domain.ShareStrmSettings, source domain.ShareStrmSource, cats []*domain.MediaCategory, seen map[string]bool) (bool, error) {
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
	entry := domain.ShareStrmEntry{ID: uuid.NewSHA1(uuid.NameSpaceURL, []byte(matches[1]+":path:"+filePath)).String(), ShareCode: matches[1], Password: source.Password, FileID: source.RemoteFileID, FileName: file.Name, FilePath: filePath}
	if entry.Password == "" {
		entry.Password = extractSharePassword(source.URL)
	}
	if err := s.store.SaveStrmEntry(ctx, entry); err != nil {
		return false, err
	}
	written := 0
	for _, episode := range episodes {
		current := source
		current.Result.SeasonNumber = episode.SeasonNumber
		current.Result.EpisodeNumber = episode.EpisodeNumber
		relative, pathErr := s.strmRelativePath(current, file, cats)
		if pathErr != nil {
			return written > 0, pathErr
		}
		identity := relative
		if s.output != nil {
			identity = fmt.Sprintf("%d:%d:%d", source.MediaID, episode.SeasonNumber, episode.EpisodeNumber)
		}
		if seen[identity] {
			continue
		}
		localPath := filepath.Join(cfg.OutputPath, relative)
		var writeErr error
		if s.output != nil {
			raw, _ := json.Marshal(entry)
			_, writeErr = s.output.Write(ctx, fmt.Sprintf("%d:%d:%d", source.MediaID, episode.SeasonNumber, episode.EpisodeNumber), localPath, cfg.BaseURL+"/share-strm/"+entry.ID+"\n", string(raw), entry.ID)
		} else {
			writeErr = writeShareStrm(localPath, cfg.BaseURL+"/share-strm/"+entry.ID)
		}
		if err := writeErr; err != nil {
			return written > 0, err
		}
		if err := s.store.SaveExportedStrmFile(ctx, domain.StrmFile{StrmConfigID: -1, FileName: file.Name, FilePath: localPath, LocalStrmPath: localPath}); err != nil {
			return written > 0, fmt.Errorf("STRM已写入，但登记文件清单失败：%w", err)
		}
		seen[identity] = true
		written++
	}
	return written > 0, nil
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
