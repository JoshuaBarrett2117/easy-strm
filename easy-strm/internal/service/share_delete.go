package service

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// SetStrmDeleteGuard 复用导出任务锁，防止分享删除与STRM生成并发。
func (s *ShareRecordService) SetStrmDeleteGuard(exporter *ShareStrmService) {
	s.strmExportMu = &exporter.exportMu
}

// Delete 删除分享及其同步记录、播放映射和已生成STRM；磁盘清理失败时保留数据库以便重试。
func (s *ShareRecordService) Delete(ctx context.Context, id int) error {
	if id <= 0 {
		return fmt.Errorf("分享ID无效")
	}
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if s.activeSyncID != "" {
		return fmt.Errorf("有分享同步任务正在运行，请等待任务结束后再删除")
	}
	if !s.identifyMu.TryLock() {
		return fmt.Errorf("有分享识别任务正在运行，请等待任务结束后再删除")
	}
	defer s.identifyMu.Unlock()
	if s.strmExportMu != nil {
		if !s.strmExportMu.TryLock() {
			return fmt.Errorf("分享STRM导出正在运行，请等待任务结束后再删除")
		}
		defer s.strmExportMu.Unlock()
	}
	rawURL, err := s.dao.ShareDeleteURL(ctx, id)
	if err != nil {
		return err
	}
	matches := shareCodeRe.FindStringSubmatch(rawURL)
	if len(matches) < 2 {
		return fmt.Errorf("分享链接无效，无法确定STRM归属")
	}
	entries, err := s.dao.ShareDeleteEntries(ctx, matches[1])
	if err != nil {
		return err
	}
	paths := []string{}
	if len(entries) > 0 {
		candidates, err := s.dao.ShareDeletePaths(ctx, entries)
		if err != nil {
			return err
		}
		ids := make(map[string]bool, len(entries))
		for _, entry := range entries {
			ids[entry] = true
		}
		// 先验证全部候选，再删除文件；历史改名文件也通过播放URL精确匹配。
		for _, candidate := range candidates {
			if err = ctx.Err(); err != nil {
				return err
			}
			matched, err := shareDeleteFileMatches(candidate.Path, ids)
			if err != nil {
				return fmt.Errorf("检查STRM失败，分享尚未删除：%w", err)
			}
			if matched || candidate.Linked {
				paths = append(paths, candidate.Path)
			}
		}
		if err = s.dao.RememberShareDeletePaths(ctx, entries[0], paths); err != nil {
			return err
		}
		for _, p := range paths {
			if err = ctx.Err(); err != nil {
				return err
			}
			matched, err := shareDeleteFileMatches(p, ids)
			if err != nil {
				return err
			}
			if matched {
				if err = os.Remove(p); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("删除STRM失败，保留分享记录供重试：%w", err)
				}
			}
		}
	}
	return s.dao.DeleteWithStrm(ctx, id, entries, paths)
}

func shareDeleteFileMatches(p string, ids map[string]bool) (bool, error) {
	if !filepath.IsAbs(p) || !strings.EqualFold(filepath.Ext(p), ".strm") {
		return false, fmt.Errorf("STRM清单路径无效：%s", p)
	}
	if _, err := NormalizeStrmOutputPath(p); err != nil {
		return false, err
	}
	info, err := os.Stat(p)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Size() > 64*1024 {
		return false, fmt.Errorf("STRM清单对应的不是有效播放文件：%s", p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return false, err
	}
	u, err := url.Parse(strings.TrimSpace(string(data)))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false, nil
	}
	index := strings.LastIndex(u.Path, "/share-strm/")
	return index >= 0 && ids[u.Path[index+len("/share-strm/"):]], nil
}
